[CmdletBinding()]
param(
    [string]$Version = 'v0.3.0-rc.1',
    [switch]$IncludeImages,
    [switch]$StrictSupplyChain,
    [switch]$StrictSignatures,
    [string]$OutputDirectory = '',
    [string]$Repository = 'guiyi-labs/aiops-platform'
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$Root = Split-Path -Parent $PSScriptRoot
$ImageVersion = $Version.TrimStart('v')
$ArtifactRoot = Join-Path $Root '.artifacts'
$OutDir = if ($OutputDirectory) {
    if ([IO.Path]::IsPathRooted($OutputDirectory)) { $OutputDirectory } else { Join-Path $Root $OutputDirectory }
} else {
    Join-Path $ArtifactRoot ("release-local\{0}" -f $Version)
}
$WorkDir = Join-Path $ArtifactRoot ("release-work\{0}" -f $Version)
$EvidenceDir = Join-Path $ArtifactRoot 'm97-release'

function Invoke-Native {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [Parameter(Mandatory = $true)][string[]]$Arguments,
        [string]$WorkingDirectory = $Root
    )
    Push-Location $WorkingDirectory
    try {
        & $FilePath @Arguments
        $exitCode = $LASTEXITCODE
        if ($exitCode -ne 0) {
            throw "$FilePath exited with code $exitCode"
        }
    } finally {
        Pop-Location
    }
}

function Get-ToolVersion {
    param([string]$Name, [string[]]$Arguments)
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) { return 'unavailable' }
    $previousErrorAction = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $output = & $Name @Arguments 2>&1 | Select-Object -First 1
        $exitCode = $LASTEXITCODE
        if ($exitCode -ne 0) { return "error:$exitCode" }
        return [string]$output
    } finally {
        $ErrorActionPreference = $previousErrorAction
    }
}

function Write-Checksums {
    param([string]$Directory, [string]$FileName)
    $target = Join-Path $Directory $FileName
    $files = Get-ChildItem -LiteralPath $Directory -Recurse -File |
        Where-Object { $_.FullName -ne $target } |
        Sort-Object FullName
    $lines = foreach ($file in $files) {
        $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $file.FullName).Hash.ToLowerInvariant()
        $relative = $file.FullName.Substring($Directory.Length).TrimStart('\').Replace('\', '/')
        "$hash  $relative"
    }
    [IO.File]::WriteAllLines($target, $lines, [Text.UTF8Encoding]::new($false))
}

if (-not ($Version -match '^v\d+\.\d+\.\d+-rc\.\d+$')) {
    throw "Invalid release candidate version: $Version (expected vX.Y.Z-rc.N)"
}
if ($StrictSupplyChain -and -not $IncludeImages) {
    throw '-StrictSupplyChain requires -IncludeImages'
}
$dirty = (& git -C $Root status --porcelain)
if ($dirty) {
    throw 'Release packaging requires a clean worktree so revision and source archive stay aligned'
}
$sha = (& git -C $Root rev-parse HEAD).Trim()
if (-not ($sha -match '^[0-9a-f]{40}$')) { throw "Unable to resolve full Git revision: $sha" }

foreach ($directory in @($OutDir, $WorkDir)) {
    if (Test-Path -LiteralPath $directory) { Remove-Item -LiteralPath $directory -Recurse -Force }
    New-Item -ItemType Directory -Force -Path $directory | Out-Null
}
New-Item -ItemType Directory -Force -Path $EvidenceDir | Out-Null

$backendArchive = Join-Path $OutDir "aiops-platform-backend-$Version-linux-multiarch-oci.tar"
$frontendArchive = Join-Path $OutDir "aiops-platform-frontend-$Version-linux-multiarch-oci.tar"
if ($IncludeImages) {
    Invoke-Native -FilePath 'docker' -Arguments @(
        'buildx', 'build', '--platform', 'linux/amd64,linux/arm64',
        '--build-arg', "VERSION=$ImageVersion",
        '--tag', "k8s-aiops-backend:$Version",
        '--output', "type=oci,dest=$backendArchive", 'backend'
    )
    Invoke-Native -FilePath 'docker' -Arguments @(
        'buildx', 'build', '--platform', 'linux/amd64,linux/arm64',
        '--tag', "k8s-aiops-frontend:$Version",
        '--output', "type=oci,dest=$frontendArchive", 'frontend'
    )
}

Invoke-Native -FilePath 'git' -Arguments @('-C', $Root, 'archive', '--format=tar.gz', "--output=$OutDir\aiops-platform-source-$Version.tar.gz", $sha)
Copy-Item (Join-Path $Root 'docs\api\openapi.yaml') (Join-Path $OutDir 'openapi.yaml')
Copy-Item (Join-Path $Root 'docs\thesis\dependency-licenses.md') (Join-Path $OutDir 'dependency-licenses.md')
Copy-Item (Join-Path $Root 'docs\security\license-allowlist.json') (Join-Path $OutDir 'license-allowlist.json')
Copy-Item (Join-Path $Root 'docs\release-candidate-operations.md') (Join-Path $OutDir 'release-candidate-operations.md')

$syft = Get-Command syft -ErrorAction SilentlyContinue
if ($syft) {
    $backendSource = if ($IncludeImages) { "oci-archive:$backendArchive" } else { 'dir:backend' }
    $frontendSource = if ($IncludeImages) { "oci-archive:$frontendArchive" } else { 'dir:frontend' }
    Invoke-Native -FilePath 'syft' -Arguments @($backendSource, '--source-name', 'k8s-aiops-backend', '--source-version', $Version, '--output', "spdx-json=$OutDir\sbom-backend-$Version-spdx.json")
    Invoke-Native -FilePath 'syft' -Arguments @($frontendSource, '--source-name', 'k8s-aiops-frontend', '--source-version', $Version, '--output', "spdx-json=$OutDir\sbom-frontend-$Version-spdx.json")
} elseif ($StrictSupplyChain) {
    throw 'syft is required by -StrictSupplyChain'
}

$helm = Get-Command helm -ErrorAction SilentlyContinue
if ($helm) {
    Invoke-Native -FilePath 'helm' -Arguments @('lint', '--strict', 'deploy/helm/aiops-platform')
    Invoke-Native -FilePath 'helm' -Arguments @(
        'package', 'deploy/helm/aiops-platform', '--version', $ImageVersion,
        '--app-version', $ImageVersion, '--destination', $OutDir
    )
} elseif ($StrictSupplyChain) {
    throw 'helm is required by -StrictSupplyChain'
} else {
    Invoke-Native -FilePath 'tar' -Arguments @('-czf', "$OutDir\aiops-platform-helm-$Version.tar.gz", '-C', 'deploy/helm', 'aiops-platform')
}

Invoke-Native -FilePath 'kubectl' -Arguments @('kustomize', 'deploy/kubernetes')
Invoke-Native -FilePath 'tar' -Arguments @('-czf', "$OutDir\aiops-platform-kustomize-$Version.tar.gz", '-C', 'deploy', 'kubernetes')

$offlineName = "aiops-platform-offline-$Version"
$offlineRoot = Join-Path $WorkDir $offlineName
foreach ($name in @('images', 'deploy', 'config', 'docs', 'sbom')) {
    New-Item -ItemType Directory -Force -Path (Join-Path $offlineRoot $name) | Out-Null
}
if ($IncludeImages) {
    Copy-Item $backendArchive (Join-Path $offlineRoot 'images')
    Copy-Item $frontendArchive (Join-Path $offlineRoot 'images')
}
Get-ChildItem -LiteralPath $OutDir -Filter 'sbom-*.json' -File | Copy-Item -Destination (Join-Path $offlineRoot 'sbom')
Get-ChildItem -LiteralPath $OutDir -Filter 'aiops-platform-*.tgz' -File | Copy-Item -Destination (Join-Path $offlineRoot 'deploy')
Copy-Item (Join-Path $OutDir "aiops-platform-kustomize-$Version.tar.gz") (Join-Path $offlineRoot 'deploy')
Copy-Item (Join-Path $Root 'deploy\kubernetes\secret.example.yaml') (Join-Path $offlineRoot 'config\secret.example.yaml')
Copy-Item (Join-Path $Root 'docs\release-candidate-operations.md') (Join-Path $offlineRoot 'docs\release-candidate-operations.md')
Write-Checksums -Directory $offlineRoot -FileName 'OFFLINE-SHA256SUMS'
Invoke-Native -FilePath 'tar' -Arguments @('-czf', "$OutDir\$offlineName.tar.gz", '-C', $WorkDir, $offlineName)

$cosign = Get-Command cosign -ErrorAction SilentlyContinue
$hasLocalKeys = $cosign -and $env:COSIGN_PRIVATE_KEY -and $env:COSIGN_PUBLIC_KEY
$signatureMode = if ($hasLocalKeys) { 'key' } else { 'keyless' }
if ($hasLocalKeys) {
    if (-not (Test-Path -LiteralPath $env:COSIGN_PRIVATE_KEY) -or -not (Test-Path -LiteralPath $env:COSIGN_PUBLIC_KEY)) {
        throw 'COSIGN_PRIVATE_KEY and COSIGN_PUBLIC_KEY must point to local key files'
    }
    Copy-Item -LiteralPath $env:COSIGN_PUBLIC_KEY -Destination (Join-Path $OutDir 'SHA256SUMS.pub')
}

$manifestScript = Join-Path $Root 'scripts\release-manifest.mjs'
$builderId = "https://github.com/$Repository/blob/$sha/scripts/release-verify.ps1"
Invoke-Native -FilePath 'node' -Arguments @(
    $manifestScript, 'provenance', '--directory', $OutDir, '--version', $Version,
    '--revision', $sha, '--repository', $Repository, '--builder-id', $builderId,
    '--invocation-id', "local:$sha"
)
$createArguments = @(
    $manifestScript, 'create', '--directory', $OutDir, '--version', $Version,
    '--revision', $sha, '--repository', $Repository, '--signature-mode', $signatureMode
)
if ($StrictSupplyChain) { $createArguments += '--strict' }
Invoke-Native -FilePath 'node' -Arguments $createArguments

$signatureStatus = 'skipped'
if ($hasLocalKeys) {
    Invoke-Native -FilePath 'cosign' -WorkingDirectory $OutDir -Arguments @(
        'sign-blob', '--yes', '--key', $env:COSIGN_PRIVATE_KEY,
        '--output-signature', 'SHA256SUMS.sig', 'SHA256SUMS'
    )
    Invoke-Native -FilePath 'cosign' -WorkingDirectory $OutDir -Arguments @(
        'verify-blob', '--key', 'SHA256SUMS.pub', '--signature', 'SHA256SUMS.sig', 'SHA256SUMS'
    )
    $signatureStatus = 'verified-local-key'
} else {
    [IO.File]::WriteAllText((Join-Path $OutDir 'SIGNING_SKIPPED'), 'No local Cosign key pair was provided.', [Text.Encoding]::ASCII)
    if ($StrictSignatures) {
        throw '-StrictSignatures requires cosign plus COSIGN_PRIVATE_KEY and COSIGN_PUBLIC_KEY'
    }
    Write-Warning 'Signature verification skipped; this directory is a rehearsal package, not a publishable RC.'
}

$verifyArguments = @($manifestScript, 'verify', '--directory', $OutDir)
if ($StrictSupplyChain) { $verifyArguments += '--strict' }
if ($StrictSignatures) { $verifyArguments += '--require-signatures' }
Invoke-Native -FilePath 'node' -Arguments $verifyArguments

$timestamp = [DateTime]::UtcNow.ToString('yyyyMMdd-HHmmss')
$evidence = [ordered]@{
    schema = 'aiops.m97-release-rehearsal/v1'
    generated_at = [DateTime]::UtcNow.ToString('o')
    version = $Version
    revision = $sha
    release_directory = $OutDir
    strict_supply_chain = [bool]$StrictSupplyChain
    strict_signatures = [bool]$StrictSignatures
    signature_status = $signatureStatus
    release_manifest_sha256 = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $OutDir 'release-manifest.json')).Hash.ToLowerInvariant()
    checksum_root_sha256 = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $OutDir 'SHA256SUMS')).Hash.ToLowerInvariant()
    tools = [ordered]@{
        docker = Get-ToolVersion -Name 'docker' -Arguments @('--version')
        buildx = Get-ToolVersion -Name 'docker' -Arguments @('buildx', 'version')
        helm = Get-ToolVersion -Name 'helm' -Arguments @('version', '--short')
        kubectl = Get-ToolVersion -Name 'kubectl' -Arguments @('version', '--client')
        syft = Get-ToolVersion -Name 'syft' -Arguments @('version')
        cosign = Get-ToolVersion -Name 'cosign' -Arguments @('version')
        node = Get-ToolVersion -Name 'node' -Arguments @('--version')
    }
}
$evidencePath = Join-Path $EvidenceDir ("m97-release-$timestamp.json")
[IO.File]::WriteAllText($evidencePath, ($evidence | ConvertTo-Json -Depth 8), [Text.UTF8Encoding]::new($false))
Remove-Item -LiteralPath $WorkDir -Recurse -Force
Write-Output "Release candidate package verified at $OutDir"
Write-Output "Evidence: $evidencePath"
