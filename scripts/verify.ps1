[CmdletBinding()]
param(
    [switch]$SkipComposeBuild,
    [switch]$SkipFrontendBuild
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Root = Split-Path -Parent $PSScriptRoot
$ArtifactDirectory = Join-Path $Root '.artifacts\verification'
[IO.Directory]::CreateDirectory($ArtifactDirectory) | Out-Null

function Invoke-Native {
    param(
        [Parameter(Mandatory)] [string]$File,
        [Parameter(Mandatory)] [string[]]$Arguments,
        [string]$WorkingDirectory = $Root,
        [switch]$DiscardOutput
    )

    Push-Location $WorkingDirectory
    try {
        if ($DiscardOutput) {
            & $File @Arguments 2>&1 | Out-Null
        } else {
            & $File @Arguments
        }
        if ($LASTEXITCODE -ne 0) {
            throw "$File $($Arguments -join ' ') failed with exit code $LASTEXITCODE"
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
    $pathGo = Get-Command go -ErrorAction SilentlyContinue
    if ($null -ne $pathGo) {
        $candidates += $pathGo.Source
    }

    foreach ($candidate in $candidates | Select-Object -Unique) {
        if (-not (Test-Path -LiteralPath $candidate)) {
            continue
        }
        $previous = $env:GOTOOLCHAIN
        try {
            $env:GOTOOLCHAIN = 'local'
            $output = (& $candidate version 2>$null | Out-String).Trim()
        } finally {
            $env:GOTOOLCHAIN = $previous
        }
        $match = [regex]::Match($output, 'go(\d+\.\d+(?:\.\d+)?)')
        if ($match.Success -and [version]$match.Groups[1].Value -ge $required) {
            return $candidate
        }
    }

    Write-Warning "Go $required or newer is not available on the host; backend checks will use golang:1.25-alpine."
    return $null
}

function Enable-NodePath {
    if (Get-Command node -ErrorAction SilentlyContinue) {
        return
    }
    $pnpm = Get-Command pnpm -ErrorAction Stop
    $dependencies = Split-Path -Parent (Split-Path -Parent (Split-Path -Parent $pnpm.Source))
    $candidate = Join-Path $dependencies 'node\bin'
    if (Test-Path -LiteralPath (Join-Path $candidate 'node.exe')) {
        $env:Path = "$candidate;$env:Path"
    }
    Get-Command node -ErrorAction Stop | Out-Null
}

function Wait-ContainerHealthy {
    param([Parameter(Mandatory)] [string]$Name)
    $deadline = (Get-Date).AddSeconds(120)
    do {
        $state = (& docker inspect $Name --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' 2>$null | Out-String).Trim()
        if ($LASTEXITCODE -eq 0 -and ($state -eq 'healthy' -or $state -eq 'running')) {
            return $state
        }
        Start-Sleep -Seconds 2
    } while ((Get-Date) -lt $deadline)
    throw "container $Name did not become healthy; last state: $state"
}

$startedAt = Get-Date
$go = Resolve-Go
$pnpm = (Get-Command pnpm -ErrorAction Stop).Source
Enable-NodePath

$goVerificationMode = 'host'
if ($null -ne $go) {
    $env:GOMAXPROCS = '2'
    if (Test-Path -LiteralPath (Join-Path $Root '.tools\gomodcache')) {
        $env:GOMODCACHE = (Resolve-Path (Join-Path $Root '.tools\gomodcache')).Path
    }
    if (Test-Path -LiteralPath (Join-Path $Root '.tools\gocache')) {
        $env:GOCACHE = (Resolve-Path (Join-Path $Root '.tools\gocache')).Path
    }

    Write-Host '[1/8] go vet and go test ./...'
    Invoke-Native $go @('vet', './...') (Join-Path $Root 'backend')
    Invoke-Native $go @('test', '-p=1', '-count=1', './...') (Join-Path $Root 'backend')

    Write-Host '[2/8] go build ./cmd/server, ./cmd/credential-reencrypt and ./cmd/audit-archive'
    Invoke-Native $go @('build', '-p=1', '-o', (Join-Path $ArtifactDirectory 'api.exe'), './cmd/server') (Join-Path $Root 'backend')
    Invoke-Native $go @('build', '-p=1', '-o', (Join-Path $ArtifactDirectory 'credential-reencrypt.exe'), './cmd/credential-reencrypt') (Join-Path $Root 'backend')
    Invoke-Native $go @('build', '-p=1', '-o', (Join-Path $ArtifactDirectory 'audit-archive.exe'), './cmd/audit-archive') (Join-Path $Root 'backend')
} else {
    $goVerificationMode = 'docker'
    Write-Host '[1/8] go vet and go test ./... (Docker Go toolchain)'
    Invoke-Native 'docker' @('build', '--target', 'source', '-t', 'aiops-platform-go-source:local', '.') (Join-Path $Root 'backend')
    $repositoryMount = "type=bind,source=$Root,target=/workspace,readonly"
    Invoke-Native 'docker' @(
        'run', '--rm', '--network', 'none', '--mount', $repositoryMount,
        '-w', '/workspace/backend', 'aiops-platform-go-source:local',
        'go', 'vet', './...'
    ) $Root
    Invoke-Native 'docker' @(
        'run', '--rm', '--network', 'none', '--mount', $repositoryMount,
        '-w', '/workspace/backend', 'aiops-platform-go-source:local',
        'go', 'test', '-p=1', '-count=1', './...'
    ) $Root

    Write-Host '[2/8] go build ./cmd/server, ./cmd/credential-reencrypt and ./cmd/audit-archive (Docker Go toolchain)'
    Invoke-Native 'docker' @('build', '--target', 'build', '-t', 'aiops-platform-backend-build:local', '.') (Join-Path $Root 'backend')
}

Write-Host '[3/8] pnpm typecheck'
Invoke-Native $pnpm @('typecheck') (Join-Path $Root 'frontend')

Write-Host '[4/8] pnpm test -- --run'
Invoke-Native $pnpm @('test', '--', '--run') (Join-Path $Root 'frontend')

if (-not $SkipFrontendBuild) {
    Write-Host '[5/8] pnpm build'
    Invoke-Native $pnpm @('build') (Join-Path $Root 'frontend')
} else {
    Write-Host '[5/8] pnpm build skipped'
}

Write-Host '[6/8] docker compose config and runtime'
Invoke-Native 'docker' @('compose', 'config', '--quiet') $Root
if ($SkipComposeBuild) {
    Invoke-Native 'docker' @('compose', 'up', '-d') $Root
} else {
    Invoke-Native 'docker' @('compose', 'up', '-d', '--build') $Root
}

$containerStates = [ordered]@{}
foreach ($name in @('k8s-aiops-postgres-1', 'k8s-aiops-backend-1', 'k8s-aiops-frontend-1')) {
    $containerStates[$name] = Wait-ContainerHealthy $name
}

Write-Host '[7/8] kubectl kustomize deployment gates'
$rendered = [ordered]@{}
foreach ($directory in @('deploy/kubernetes', 'deploy/managed-cluster', 'deploy/demo-scenarios', 'deploy/diagnosis-e2e')) {
    $output = & kubectl kustomize (Join-Path $Root $directory) 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "kubectl kustomize $directory failed: $($output -join "`n")"
    }
    $rendered[$directory] = @($output | Select-String '^kind:').Count
}

Write-Host '[8/8] /api/v1/health/ready and frontend proxy'
$backend = Invoke-RestMethod 'http://127.0.0.1:8080/api/v1/health/ready' -TimeoutSec 10
$frontend = Invoke-WebRequest -UseBasicParsing 'http://127.0.0.1:18080/' -TimeoutSec 10
$proxy = Invoke-RestMethod 'http://127.0.0.1:18080/api/v1/health/ready' -TimeoutSec 10
if ($backend.status -ne 'ready' -or $proxy.status -ne 'ready' -or $frontend.StatusCode -ne 200) {
    throw 'runtime health verification failed'
}

$summary = [ordered]@{
    verified_at = (Get-Date).ToString('o')
    duration_seconds = [math]::Round(((Get-Date) - $startedAt).TotalSeconds, 2)
    backend_status = $backend.status
    frontend_status = $frontend.StatusCode
    frontend_proxy_status = $proxy.status
    go_verification_mode = $goVerificationMode
    containers = $containerStates
    rendered_resources = $rendered
}
$path = Join-Path $ArtifactDirectory ("verify-{0}.json" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
[IO.File]::WriteAllText($path, ($summary | ConvertTo-Json -Depth 10), [Text.UTF8Encoding]::new($false))
Write-Host "Verification passed. Evidence: $path"
$summary | ConvertTo-Json -Depth 10
