[CmdletBinding()]
param(
    [string]$Version = 'v0.3.0-rc.1',
    [string]$PreviousVersion = 'v0.2.0-rc.0',
    [string]$ReleaseDirectory = '',
    [string]$ClusterName = '',
    [string]$PreviousBackendImage = 'k8s-aiops-backend:latest',
    [string]$PreviousFrontendImage = 'k8s-aiops-frontend:latest',
    [switch]$SkipHelm,
    [switch]$KeepCluster
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$Root = Split-Path -Parent $PSScriptRoot
if (-not $ReleaseDirectory) { $ReleaseDirectory = Join-Path $Root (".artifacts\release-local\{0}" -f $Version) }
if (-not $ClusterName) { $ClusterName = "aiops-m97-$([guid]::NewGuid().ToString('N').Substring(0, 8))" }
$Context = "kind-$ClusterName"
$TemporaryDirectory = Join-Path ([IO.Path]::GetTempPath()) "aiops-m97-$ClusterName"
$KustomizeDirectory = Join-Path $TemporaryDirectory 'kubernetes'
$SecretPath = Join-Path $TemporaryDirectory 'aiops-secret.yaml'
$PostgresDockerfile = Join-Path $TemporaryDirectory 'Postgres.Dockerfile'
$EvidenceDirectory = Join-Path $Root '.artifacts\m97-release'
$EvidencePath = Join-Path $EvidenceDirectory "lifecycle-$ClusterName.json"
$PortForward = $null
$Namespace = 'aiops-system'
$HelmNamespace = 'aiops-helm-system'

function Invoke-Native {
    param([Parameter(Mandatory)][string]$FilePath, [Parameter(Mandatory)][string[]]$Arguments, [switch]$AllowFailure)
    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $output = & $FilePath @Arguments 2>&1
        $exitCode = $LASTEXITCODE
    } finally { $ErrorActionPreference = $previousErrorAction }
    if ($exitCode -ne 0 -and -not $AllowFailure) {
        throw "$FilePath exited with code $exitCode`: $($output -join [Environment]::NewLine)"
    }
    return (($output | ForEach-Object { $_.ToString() }) -join [Environment]::NewLine).Trim()
}

function Write-Utf8File {
    param([Parameter(Mandatory)][string]$Path, [Parameter(Mandatory)][string]$Contents)
    [IO.File]::WriteAllText($Path, $Contents, [Text.UTF8Encoding]::new($false))
}

function New-RandomHex {
    param([int]$Bytes = 24)
    $buffer = [byte[]]::new($Bytes)
    $generator = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $generator.GetBytes($buffer) } finally { $generator.Dispose() }
    return (($buffer | ForEach-Object { $_.ToString('x2') }) -join '')
}

function New-Secret {
    $databasePassword = New-RandomHex
    $jwtKey = New-RandomHex
    $credentialKey = [Convert]::ToBase64String(([byte[]](1..32)))
    $contents = Get-Content -Raw -Encoding UTF8 (Join-Path $Root 'deploy\kubernetes\secret.example.yaml')
    $contents = $contents.Replace('CHANGE_ME_DATABASE_PASSWORD', $databasePassword)
    $contents = $contents.Replace('CHANGE_ME_AT_LEAST_32_RANDOM_CHARACTERS', $jwtKey)
    $contents = $contents.Replace('CHANGE_ME_BASE64_ENCODED_32_BYTE_KEY', $credentialKey)
    $contents = $contents.Replace('CHANGE_ME_INITIAL_ADMIN_PASSWORD', 'm97-rehearsal-password')
    Write-Utf8File -Path $SecretPath -Contents $contents
}

function New-KustomizeRendering {
    param([Parameter(Mandatory)][string]$BackendTag, [Parameter(Mandatory)][string]$FrontendTag)
    if (Test-Path -LiteralPath $KustomizeDirectory) { Remove-Item -LiteralPath $KustomizeDirectory -Recurse -Force }
    Copy-Item -LiteralPath (Join-Path $Root 'deploy\kubernetes') -Destination $KustomizeDirectory -Recurse
    $path = Join-Path $KustomizeDirectory 'kustomization.yaml'
    $contents = Get-Content -Raw -Encoding UTF8 $path
    $contents += @"
images:
  - name: k8s-aiops-backend
    newName: k8s-aiops-backend
    newTag: $BackendTag
  - name: k8s-aiops-frontend
    newName: k8s-aiops-frontend
    newTag: $FrontendTag
  - name: pgvector/pgvector
    newName: aiops-postgres-m97
    newTag: $Version
"@
    Write-Utf8File -Path $path -Contents $contents
}

function Wait-Workloads {
    param([Parameter(Mandatory)][string]$TargetNamespace)
    Invoke-Native -FilePath 'kubectl' -Arguments @('--context', $Context, 'rollout', 'status', 'statefulset/postgres', '-n', $TargetNamespace, '--timeout=10m') | Out-Null
    Invoke-Native -FilePath 'kubectl' -Arguments @('--context', $Context, 'rollout', 'status', 'deployment/backend', '-n', $TargetNamespace, '--timeout=10m') | Out-Null
    Invoke-Native -FilePath 'kubectl' -Arguments @('--context', $Context, 'rollout', 'status', 'deployment/frontend', '-n', $TargetNamespace, '--timeout=10m') | Out-Null
}

function Start-HealthCheck {
    param([Parameter(Mandatory)][string]$TargetNamespace, [int]$Port = 18080)
    if ($null -ne $script:PortForward) { Stop-Process -Id $script:PortForward.Id -Force -ErrorAction SilentlyContinue }
    $script:PortForward = Start-Process -FilePath 'kubectl' -ArgumentList @('--context', $Context, '-n', $TargetNamespace, 'port-forward', 'service/backend', "$Port`:8080") -PassThru -WindowStyle Hidden
    $deadline = (Get-Date).AddMinutes(2)
    do {
        try {
            $ready = Invoke-RestMethod -Uri "http://127.0.0.1:$Port/api/v1/health/ready" -TimeoutSec 5
            if ($ready.status -eq 'ready') {
                $login = Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:$Port/api/v1/auth/login" -ContentType 'application/json' -Body (@{username='admin';password='m97-rehearsal-password'} | ConvertTo-Json -Compress)
                if ([string]::IsNullOrWhiteSpace([string]$login.access_token)) { throw 'login did not return an access token' }
                return
            }
        } catch {}
        Start-Sleep -Seconds 2
    } while ((Get-Date) -lt $deadline)
    throw "backend health/login did not pass in $TargetNamespace"
}

function Apply-Kustomize {
    param([Parameter(Mandatory)][string]$BackendTag, [Parameter(Mandatory)][string]$FrontendTag)
    New-KustomizeRendering -BackendTag $BackendTag -FrontendTag $FrontendTag
    Invoke-Native -FilePath 'kubectl' -Arguments @('--context', $Context, 'apply', '-f', (Join-Path $KustomizeDirectory 'namespace.yaml')) | Out-Null
    Invoke-Native -FilePath 'kubectl' -Arguments @('--context', $Context, 'apply', '-f', $SecretPath) | Out-Null
    Invoke-Native -FilePath 'kubectl' -Arguments @('--context', $Context, 'apply', '-k', $KustomizeDirectory) | Out-Null
    Wait-Workloads -TargetNamespace $Namespace
    Start-HealthCheck -TargetNamespace $Namespace -Port 18080
}

function Apply-Helm {
    param([Parameter(Mandatory)][string]$BackendTag, [Parameter(Mandatory)][string]$FrontendTag, [Parameter(Mandatory)][string]$ChartPath)
    $helmSecret = (Get-Content -Raw -Encoding UTF8 $SecretPath).Replace("namespace: aiops-system", "namespace: $HelmNamespace")
    $helmSecretPath = Join-Path $TemporaryDirectory 'aiops-helm-secret.yaml'
    Write-Utf8File -Path $helmSecretPath -Contents $helmSecret
    Invoke-Native -FilePath 'kubectl' -Arguments @('--context', $Context, 'create', 'namespace', $HelmNamespace) | Out-Null
    Invoke-Native -FilePath 'kubectl' -Arguments @('--context', $Context, 'apply', '-f', $helmSecretPath) | Out-Null
    $helmValues = @(
        '--set', "namespace.name=$HelmNamespace",
        '--set', 'existingSecret=aiops-secrets',
        '--set', 'backend.image.repository=k8s-aiops-backend', '--set', "backend.image.tag=$BackendTag",
        '--set', 'frontend.image.repository=k8s-aiops-frontend', '--set', "frontend.image.tag=$FrontendTag",
        '--set', 'postgres.image.repository=aiops-postgres-m97', '--set', "postgres.image.tag=$Version"
    )
    Invoke-Native -FilePath 'helm' -Arguments (@('upgrade', '--install', 'aiops', $ChartPath, '--namespace', $HelmNamespace, '--create-namespace', '--wait', '--timeout', '10m') + $helmValues) | Out-Null
    Wait-Workloads -TargetNamespace $HelmNamespace
    Start-HealthCheck -TargetNamespace $HelmNamespace -Port 18081
}

if (-not (Test-Path -LiteralPath $ReleaseDirectory)) { throw "Release directory does not exist: $ReleaseDirectory" }
if (-not (Get-Command kubectl -ErrorAction SilentlyContinue)) { throw 'kubectl is required' }
if (-not (Get-Command helm -ErrorAction SilentlyContinue) -and -not $SkipHelm) { throw 'helm is required unless -SkipHelm is set' }
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { throw 'docker is required' }
$chart = Get-ChildItem -LiteralPath $ReleaseDirectory -Filter 'aiops-platform-*.tgz' -File | Select-Object -First 1
if ($null -eq $chart -and -not $SkipHelm) { throw 'release directory has no packaged Helm chart' }

New-Item -ItemType Directory -Force -Path $TemporaryDirectory | Out-Null
New-Secret
$results = [ordered]@{schema='aiops.m97-release-lifecycle/v1'; version=$Version; previous_version=$PreviousVersion; cluster=$ClusterName; kustomize='pending'; helm=if ($SkipHelm) {'skipped'} else {'pending'} }
try {
    $kind = if (Get-Command kind -ErrorAction SilentlyContinue) { 'kind' } else { Join-Path $Root '.tools\kind-v0.30.0.exe' }
    Invoke-Native -FilePath $kind -Arguments @('create', 'cluster', '--name', $ClusterName, '--wait', '5m') | Out-Null
    Invoke-Native -FilePath 'docker' -Arguments @('tag', $PreviousBackendImage, "k8s-aiops-backend:$PreviousVersion") | Out-Null
    Invoke-Native -FilePath 'docker' -Arguments @('tag', $PreviousFrontendImage, "k8s-aiops-frontend:$PreviousVersion") | Out-Null
    Invoke-Native -FilePath 'docker' -Arguments @('buildx', 'build', '--platform', 'linux/amd64', '--load', '--build-arg', "VERSION=$($Version.TrimStart('v'))", '--tag', "k8s-aiops-backend:$Version", 'backend') | Out-Null
    Invoke-Native -FilePath 'docker' -Arguments @('buildx', 'build', '--platform', 'linux/amd64', '--load', '--tag', "k8s-aiops-frontend:$Version", 'frontend') | Out-Null
    Write-Utf8File -Path $PostgresDockerfile -Contents "FROM pgvector/pgvector:0.8.1-pg17`n"
    Invoke-Native -FilePath 'docker' -Arguments @('buildx', 'build', '--platform', 'linux/amd64', '--load', '--file', $PostgresDockerfile, '--tag', "aiops-postgres-m97:$Version", $TemporaryDirectory) | Out-Null
    Invoke-Native -FilePath $kind -Arguments @('load', 'docker-image', "aiops-postgres-m97:$Version", '--name', $ClusterName) | Out-Null
    Invoke-Native -FilePath $kind -Arguments @('load', 'docker-image', "k8s-aiops-backend:$PreviousVersion", "k8s-aiops-frontend:$PreviousVersion", "k8s-aiops-backend:$Version", "k8s-aiops-frontend:$Version", '--name', $ClusterName) | Out-Null

    Apply-Kustomize -BackendTag $PreviousVersion -FrontendTag $PreviousVersion
    Apply-Kustomize -BackendTag $Version -FrontendTag $Version
    Apply-Kustomize -BackendTag $PreviousVersion -FrontendTag $PreviousVersion
    $results.kustomize = 'passed: install/upgrade/rollback/health/login/cleanup-pending'
    Invoke-Native -FilePath 'kubectl' -Arguments @('--context', $Context, 'delete', 'namespace', $Namespace, '--wait=true', '--timeout=5m') | Out-Null

    if (-not $SkipHelm) {
        Apply-Helm -BackendTag $PreviousVersion -FrontendTag $PreviousVersion -ChartPath $chart.FullName
        Invoke-Native -FilePath 'helm' -Arguments @('upgrade', 'aiops', $chart.FullName, '--namespace', $HelmNamespace, '--wait', '--timeout', '10m', '--set', "backend.image.tag=$Version", '--set', "frontend.image.tag=$Version") | Out-Null
        Wait-Workloads -TargetNamespace $HelmNamespace
        Start-HealthCheck -TargetNamespace $HelmNamespace -Port 18081
        Invoke-Native -FilePath 'helm' -Arguments @('rollback', 'aiops', '1', '--namespace', $HelmNamespace, '--wait', '--timeout', '10m') | Out-Null
        Wait-Workloads -TargetNamespace $HelmNamespace
        Start-HealthCheck -TargetNamespace $HelmNamespace -Port 18081
        $results.helm = 'passed: install/upgrade/rollback/health/login/cleanup-pending'
        Invoke-Native -FilePath 'helm' -Arguments @('uninstall', 'aiops', '--namespace', $HelmNamespace) | Out-Null
        Invoke-Native -FilePath 'kubectl' -Arguments @('--context', $Context, 'delete', 'namespace', $HelmNamespace, '--wait=true', '--timeout=5m') | Out-Null
    }
    $results.status = 'passed'
} finally {
    if ($null -ne $PortForward) { Stop-Process -Id $PortForward.Id -Force -ErrorAction SilentlyContinue }
    if (-not $KeepCluster) { Invoke-Native -FilePath $kind -Arguments @('delete', 'cluster', '--name', $ClusterName) -AllowFailure | Out-Null }
    if (Test-Path -LiteralPath $TemporaryDirectory) { Remove-Item -LiteralPath $TemporaryDirectory -Recurse -Force }
}
$results.cleanup = if ($KeepCluster) { 'deferred-by-request' } else { 'passed' }
New-Item -ItemType Directory -Force -Path $EvidenceDirectory | Out-Null
Write-Utf8File -Path $EvidencePath -Contents ($results | ConvertTo-Json -Depth 8)
Write-Output "M97 release lifecycle rehearsal passed: $EvidencePath"
