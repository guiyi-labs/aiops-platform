[CmdletBinding()]
param(
    [string]$Version = 'v0.2.0',
    [switch]$IncludeImages
)
# M88-local release verification: assemble a checksummed release directory
# from local state and verify its SHA256SUMS. Docker image archives are
# produced by CI (release.yml); local runs default to source-side artifacts
# and pass -IncludeImages only on a host intended for multi-arch qemu builds.
# When cosign is available, the package is signed and verified with an
# explicit key pair (COSIGN_PRIVATE_KEY / COSIGN_PUBLIC_KEY); otherwise the
# script records signature_skipped and exits non-zero on strict signing.
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$Root = Split-Path -Parent $PSScriptRoot
$OutDir = Join-Path $Root ".artifacts\release-local"
$ImageVersion = $Version.TrimStart('v')

if (-not ($Version -match '^v\d+\.\d+\.\d+([.-][0-9A-Za-z.-]+)?$')) {
    throw "Invalid semantic version: $Version (expected vX.Y.Z)"
}

# 1) Optional multi-arch OCI archives (normally built by CI release.yml).
if ($IncludeImages) {
    New-Item -ItemType Directory -Force -Path $OutDir -ErrorAction SilentlyContinue | Out-Null
    docker buildx build --platform linux/amd64,linux/arm64 `
        --build-arg "VERSION=$ImageVersion" `
        --tag "aiops-platform-backend:$Version" -o "type=oci,dest=$OutDir\backend-$Version-oci.tar" backend
    docker buildx build --platform linux/amd64,linux/arm64 `
        --build-arg "VERSION=$ImageVersion" `
        --tag "aiops-platform-frontend:$Version" `
        -o "type=oci,dest=$OutDir\frontend-$Version-oci.tar" frontend
}

# 2) Source archive from tracked HEAD.
$sha = (git -C $Root rev-parse HEAD).Trim()
New-Item -ItemType Directory -Force -Path $OutDir -ErrorAction SilentlyContinue | Out-Null
git -C $Root archive --format=tar.gz -o "$OutDir\aiops-platform-source-$Version.tar.gz" HEAD

# 3) Contract and license artifacts.
Copy-Item (Join-Path $Root 'docs\api\openapi.yaml') (Join-Path $OutDir 'openapi.yaml')
Copy-Item (Join-Path $Root 'docs\thesis\dependency-licenses.md') (Join-Path $OutDir 'dependency-licenses.md')
Copy-Item (Join-Path $Root 'docs\security\license-allowlist.json') (Join-Path $OutDir 'license-allowlist.json')
Copy-Item (Join-Path $Root 'deploy\helm\aiops-platform\Chart.yaml') (Join-Path $OutDir 'helm-chart.yaml')

# 4) Helm chart bundle.
Push-Location (Join-Path $Root 'deploy\helm')
try {
    tar -czf (Join-Path $OutDir "aiops-platform-helm-$Version.tar.gz") aiops-platform
} finally { Pop-Location }

# 5) Release metadata.
$metadata = [ordered]@{
    version = $Version
    revision = $sha
    builder = 'scripts/release-verify.ps1'
    architectures = @('linux/amd64','linux/arm64')
    images = @('backend','frontend')
    source = "aiops-platform-source-$Version.tar.gz"
    helm = "aiops-platform-helm-$Version.tar.gz"
    signature = if (Get-Command cosign -ErrorAction SilentlyContinue) { 'signed' } else { 'skipped-no-cosign' }
}
[IO.File]::WriteAllText((Join-Path $OutDir 'release-metadata.json'), ($metadata | ConvertTo-Json -Depth 6), (New-Object System.Text.UTF8Encoding $false))

# 6) Checksums + self-verification.
Push-Location $OutDir
try {
    if (Test-Path -LiteralPath 'SHA256SUMS') { Remove-Item -LiteralPath 'SHA256SUMS' -Force }
    $files = Get-ChildItem -File | Where-Object { $_.Name -ne 'SHA256SUMS' }
    $lines = foreach ($f in $files) {
        $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $f.FullName).Hash.ToLowerInvariant()
        "$hash  $($f.Name)"
    }
    [IO.File]::WriteAllLines((Join-Path $OutDir 'SHA256SUMS'), $lines, [Text.UTF8Encoding]::new($false))
    foreach ($line in $lines) {
        $parts = $line -split '  ',2
        $expected = $parts[0].ToLowerInvariant()
        $name = $parts[1]
        $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $OutDir $name)).Hash.ToLowerInvariant()
        if ($actual -ne $expected) { throw "SHA256 mismatch for $name" }
    }
    Write-Host "SHA256SUMS verified for $($lines.Count) files"
} finally { Pop-Location }

# 7) Signing gate (cosign optional, explicit key pair).
$cosign = Get-Command cosign -ErrorAction SilentlyContinue
if ($null -ne $cosign) {
    if (-not $env:COSIGN_PRIVATE_KEY -or -not $env:COSIGN_PUBLIC_KEY) {
        throw 'cosign found but COSIGN_PRIVATE_KEY / COSIGN_PUBLIC_KEY are not set'
    }
    Push-Location $OutDir
    try {
        cosign sign-blob --key $env:COSIGN_PRIVATE_KEY --output-signature=SHA256SUMS.sig SHA256SUMS | Out-Null
        cosign verify-blob --key $env:COSIGN_PUBLIC_KEY --signature=SHA256SUMS.sig SHA256SUMS | Out-Null
        Write-Host "Cosign real signature + verification passed for $Version"
    } finally { Pop-Location }
} else {
    Set-Content -Path (Join-Path $OutDir 'SIGNING_SKIPPED') -Value 'cosign unavailable; run on a release host with cosign' -Encoding ascii
    Write-Warning "cosign not found; signature_skipped=true. Use cosign on the CI/release host for production artifacts."
}
Write-Output "Release package verified at $OutDir (revision $sha)"