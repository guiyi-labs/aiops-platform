[CmdletBinding()]
param(
    [ValidateSet('Auto', 'Backend', 'Frontend', 'Manifests', 'All')]
    [string]$Scope = 'Auto',
    [string]$BaseRef = ''
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Root = Split-Path -Parent $PSScriptRoot

function Invoke-Native {
    param(
        [Parameter(Mandatory)] [string]$File,
        [Parameter(Mandatory)] [string[]]$Arguments,
        [string]$WorkingDirectory = $Root,
        [switch]$DiscardOutput
    )

    Push-Location $WorkingDirectory
    try {
        $previousErrorAction = $ErrorActionPreference
        try {
            # Windows PowerShell 5 converts native stderr into ErrorRecord objects.
            # Docker/kubectl progress is valid stderr, so rely on the exit code.
            $ErrorActionPreference = 'Continue'
            if ($DiscardOutput) { & $File @Arguments 2>&1 | Out-Null } else { & $File @Arguments }
            $exitCode = $LASTEXITCODE
        } finally {
            $ErrorActionPreference = $previousErrorAction
        }
        if ($exitCode -ne 0) {
            throw "$File $($Arguments -join ' ') failed with exit code $exitCode"
        }
    } finally {
        Pop-Location
    }
}

function Resolve-Go {
    $required = [version](([regex]::Match(
        [IO.File]::ReadAllText((Join-Path $Root 'backend\go.mod')),
        '(?m)^go\s+(\d+\.\d+)'
    )).Groups[1].Value)
    $candidates = @((Join-Path $Root '.tools\go\bin\go.exe'))
    $toolchains = Join-Path $Root '.tools\gomodcache\golang.org'
    if (Test-Path -LiteralPath $toolchains) {
        $candidates += @(Get-ChildItem -LiteralPath $toolchains -Directory -Force -ErrorAction SilentlyContinue |
            Where-Object { $_.Name -like 'toolchain@v*.windows-amd64' } |
            ForEach-Object { Join-Path $_.FullName 'bin\go.exe' })
    }
    $pathGo = Get-Command go -ErrorAction SilentlyContinue
    if ($null -ne $pathGo) { $candidates += $pathGo.Source }

    foreach ($candidate in $candidates | Select-Object -Unique) {
        if (-not (Test-Path -LiteralPath $candidate)) { continue }
        $previous = $env:GOTOOLCHAIN
        try {
            $env:GOTOOLCHAIN = 'local'
            $output = (& $candidate version 2>$null | Out-String).Trim()
        } finally {
            $env:GOTOOLCHAIN = $previous
        }
        $match = [regex]::Match($output, 'go(\d+\.\d+(?:\.\d+)?)')
        if ($match.Success -and [version]$match.Groups[1].Value -ge $required) { return $candidate }
    }
    throw "Go $required or newer is required for the fast gate"
}

function Enable-NodePath {
    if (Get-Command node -ErrorAction SilentlyContinue) { return }
    $pnpm = Get-Command pnpm -ErrorAction Stop
    $dependencies = Split-Path -Parent (Split-Path -Parent (Split-Path -Parent $pnpm.Source))
    $candidate = Join-Path $dependencies 'node\bin'
    if (Test-Path -LiteralPath (Join-Path $candidate 'node.exe')) {
        $env:Path = "$candidate;$env:Path"
    }
    Get-Command node -ErrorAction Stop | Out-Null
}

function Get-ChangedFiles {
    $tracked = if ([string]::IsNullOrWhiteSpace($BaseRef)) {
        @(& git -C $Root diff --name-only --diff-filter=ACMR HEAD)
    } else {
        @(& git -C $Root diff --name-only --diff-filter=ACMR "$BaseRef...HEAD")
    }
    if ($LASTEXITCODE -ne 0) { throw 'unable to determine tracked change scope' }
    $untracked = @(& git -C $Root ls-files --others --exclude-standard)
    if ($LASTEXITCODE -ne 0) { throw 'unable to determine untracked change scope' }
    return @($tracked + $untracked | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | Sort-Object -Unique)
}

$startedAt = Get-Date
Invoke-Native 'git' @('diff', '--check') $Root
$changed = @(Get-ChangedFiles)
$runBackend = $Scope -in @('Backend', 'All')
$runFrontend = $Scope -in @('Frontend', 'All')
$runManifests = $Scope -in @('Manifests', 'All')
if ($Scope -eq 'Auto') {
    $runBackend = @($changed | Where-Object { $_ -match '^(backend/|scripts/|docs/|\.github/|README\.md$)' }).Count -gt 0
    $runFrontend = @($changed | Where-Object { $_ -like 'frontend/*' }).Count -gt 0
    $runManifests = @($changed | Where-Object { $_ -match '^(deploy/|compose\.yaml$|\.env\.example$|\.github/)' }).Count -gt 0
    if ($changed.Count -eq 0) { $runBackend = $true }
}

if ($runBackend) {
    $go = Resolve-Go
    $env:GOTOOLCHAIN = 'local'
    if (Test-Path -LiteralPath (Join-Path $Root '.tools\gomodcache')) {
        $env:GOMODCACHE = (Resolve-Path (Join-Path $Root '.tools\gomodcache')).Path
    }
    $fastCache = Join-Path $Root '.tools\gocache-fast'
    [IO.Directory]::CreateDirectory($fastCache) | Out-Null
    $env:GOCACHE = $fastCache

    $goFiles = @($changed | Where-Object { $_ -like 'backend/*.go' -or $_ -like 'backend/*/*.go' -or $_ -like 'backend/*/*/*.go' -or $_ -like 'backend/*/*/*/*.go' })
    if ($goFiles.Count -gt 0) {
        $formatTargets = @($goFiles | ForEach-Object { Join-Path $Root $_ })
        $unformatted = @(& (Join-Path (Split-Path -Parent $go) 'gofmt.exe') -l $formatTargets)
        if ($unformatted.Count -gt 0) { throw "unformatted Go files: $($unformatted -join ', ')" }
    }

    $packages = @($goFiles | ForEach-Object {
        $directory = (Split-Path -Parent ($_ -replace '\\', '/')) -replace '\\', '/'
        './' + $directory.Substring('backend/'.Length)
    } | Sort-Object -Unique)
    if (@($changed | Where-Object { $_ -match '^(\.github/|scripts/|docs/|README\.md$)' }).Count -gt 0) {
        $packages += './internal/deployment'
    }
    if (@($changed | Where-Object { $_ -match '^backend/(go\.(mod|sum)|migrations/|Dockerfile)' }).Count -gt 0) {
        $packages = @('./...')
    }
    $packages = @($packages | Sort-Object -Unique)
    if ($packages.Count -eq 0) { $packages = @('./internal/deployment') }
    Write-Host "[fast] go vet $($packages -join ' ')"
    Invoke-Native $go (@('vet') + $packages) (Join-Path $Root 'backend')
    Write-Host "[fast] go test $($packages -join ' ')"
    Invoke-Native $go (@('test', '-count=1') + $packages) (Join-Path $Root 'backend')
}

if ($runFrontend) {
    Enable-NodePath
    $pnpm = (Get-Command pnpm -ErrorAction Stop).Source
    Write-Host '[fast] frontend typecheck and Vitest'
    Invoke-Native $pnpm @('typecheck') (Join-Path $Root 'frontend')
    Invoke-Native $pnpm @('test', '--', '--run') (Join-Path $Root 'frontend')
}

if ($runManifests) {
    Write-Host '[fast] Compose and Kustomize contracts'
    Invoke-Native 'docker' @('compose', 'config', '--quiet') $Root
    foreach ($directory in @('deploy/kubernetes', 'deploy/managed-cluster', 'deploy/demo-scenarios', 'deploy/diagnosis-e2e')) {
        Invoke-Native 'kubectl' @('kustomize', $directory) $Root -DiscardOutput
    }
    if (Get-Command helm -ErrorAction SilentlyContinue) {
        Invoke-Native 'helm' @('lint', '--strict', 'deploy/helm/aiops-platform') $Root
    }
}

$duration = [math]::Round(((Get-Date) - $startedAt).TotalSeconds, 2)
Write-Host "Fast verification passed in $duration seconds (backend=$runBackend frontend=$runFrontend manifests=$runManifests)."
