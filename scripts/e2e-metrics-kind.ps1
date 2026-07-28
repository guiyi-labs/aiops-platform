[CmdletBinding()]
param(
    [string]$ApiBase = 'http://127.0.0.1:8080',
    [string]$Username = 'admin',
    [string]$AdminPassword = $env:AIOPS_ADMIN_PASSWORD,
    [string]$Context = 'kind-aiops-test',
    [switch]$KeepPlatformCluster
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Root = Split-Path -Parent $PSScriptRoot
$ArtifactDirectory = Join-Path $Root '.artifacts\metrics-e2e'
[IO.Directory]::CreateDirectory($ArtifactDirectory) | Out-Null

$MetricsServerVersion = 'v0.8.0'
$ManifestUri = "https://github.com/kubernetes-sigs/metrics-server/releases/download/$MetricsServerVersion/components.yaml"
$ManifestSHA256 = 'ff64d1a13b9ac3b0635f0dd985815fb44c23eed4706c04e5db1daadf6bc0a83b'
$RuntimeImage = 'registry.cn-hangzhou.aliyuncs.com/google_containers/metrics-server:v0.8.0'
$ManifestPath = Join-Path $Root 'deploy\metrics-server-kind\components-v0.8.0.yaml'
$KindPatchPath = Join-Path $Root 'deploy\metrics-server-kind\kind-patch.json'

function Invoke-KubectlText {
    param([Parameter(Mandatory, Position = 0)] [string[]]$Arguments)
    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $output = & kubectl --context $Context @Arguments 2>&1
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorAction
    }
    if ($exitCode -ne 0) {
        throw "kubectl $($Arguments -join ' ') failed: $($output -join "`n")"
    }
    return (($output | ForEach-Object { $_.ToString() }) -join "`n").Trim()
}

function Wait-MetricsSamples {
    $deadline = (Get-Date).AddMinutes(4)
    do {
        try {
            $nodes = Invoke-KubectlText @('get', '--raw', '/apis/metrics.k8s.io/v1beta1/nodes') | ConvertFrom-Json
            $pods = Invoke-KubectlText @('get', '--raw', '/apis/metrics.k8s.io/v1beta1/pods') | ConvertFrom-Json
            if (@($nodes.items).Count -gt 0 -and @($pods.items).Count -gt 0) {
                return [ordered]@{ node_samples = @($nodes.items).Count; pod_samples = @($pods.items).Count }
            }
        } catch {
            # Aggregated discovery can become available before kubelet samples arrive.
        }
        Start-Sleep -Seconds 5
    } while ((Get-Date) -lt $deadline)
    throw 'Metrics Server did not publish Node and Pod samples within four minutes'
}

if ([string]::IsNullOrWhiteSpace($AdminPassword)) {
    $secure = Read-Host 'AIOps administrator password' -AsSecureString
    $pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
    try {
        $AdminPassword = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer)
    } finally {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer)
    }
}

$startedAt = Get-Date
$metricsServerWasPresent = $false
$failure = $null
$summary = $null
$previousAdminPassword = $env:AIOPS_ADMIN_PASSWORD

try {
    $current = Invoke-KubectlText @('config', 'current-context')
    if ($current -ne $Context) {
        throw "unexpected kubectl context; expected $Context, got $current"
    }

    $existing = Invoke-KubectlText @('-n', 'kube-system', 'get', 'deployment', 'metrics-server', '--ignore-not-found', '-o', 'name')
    $metricsServerWasPresent = -not [string]::IsNullOrWhiteSpace($existing)

    $actualHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $ManifestPath).Hash.ToLowerInvariant()
    if ($actualHash -ne $ManifestSHA256) {
        throw "Metrics Server manifest checksum mismatch; expected $ManifestSHA256, got $actualHash"
    }

    Invoke-KubectlText @('apply', '-f', $ManifestPath) | Write-Host
    $deployment = Invoke-KubectlText @('-n', 'kube-system', 'get', 'deployment', 'metrics-server', '-o', 'json') | ConvertFrom-Json
    $arguments = @($deployment.spec.template.spec.containers[0].args)
    $currentImage = [string]$deployment.spec.template.spec.containers[0].image
    if ($arguments -notcontains '--kubelet-insecure-tls' -or $currentImage -ne $RuntimeImage) {
        Invoke-KubectlText @('-n', 'kube-system', 'patch', 'deployment', 'metrics-server', '--type=json', '--patch-file', $KindPatchPath) | Write-Host
    }
    Invoke-KubectlText @('-n', 'kube-system', 'rollout', 'status', 'deployment/metrics-server', '--timeout=180s') | Write-Host
    Invoke-KubectlText @('wait', '--for=condition=Available', 'apiservice/v1beta1.metrics.k8s.io', '--timeout=180s') | Write-Host
    $directSamples = Wait-MetricsSamples

    $env:AIOPS_ADMIN_PASSWORD = $AdminPassword
    if ($KeepPlatformCluster) {
        & powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'demo-down.ps1') -ApiBase $ApiBase -Username $Username -Context $Context | Write-Host
        if ($LASTEXITCODE -ne 0) { throw "demo-down.ps1 failed with exit code $LASTEXITCODE" }
    }

    $e2eArguments = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', (Join-Path $PSScriptRoot 'e2e-kind.ps1'), '-ApiBase', $ApiBase, '-Username', $Username, '-Context', $Context, '-RequireMetrics')
    if ($KeepPlatformCluster) { $e2eArguments += '-KeepPlatformCluster' }
    & powershell @e2eArguments | Write-Host
    if ($LASTEXITCODE -ne 0) { throw "e2e-kind.ps1 failed with exit code $LASTEXITCODE" }

    $e2eArtifact = Get-ChildItem -LiteralPath (Join-Path $Root '.artifacts\e2e-kind') -Filter 'e2e-kind-*.json' |
        Where-Object { $_.LastWriteTime -ge $startedAt } |
        Sort-Object LastWriteTime -Descending |
        Select-Object -First 1
    if ($null -eq $e2eArtifact) { throw 'metrics E2E did not produce a downstream kind artifact' }
    $e2e = Get-Content -Raw -Encoding utf8 -LiteralPath $e2eArtifact.FullName | ConvertFrom-Json
    if ($null -eq $e2e.metrics -or -not [bool]$e2e.metrics.required -or $e2e.metrics.node_samples -lt 1 -or $e2e.metrics.pod_samples -lt 1) {
        throw 'downstream platform verification did not retain real Metrics samples'
    }

    $summary = [ordered]@{
        verified_at = (Get-Date).ToString('o')
        context = $Context
        metrics_server_version = $MetricsServerVersion
        manifest_uri = $ManifestUri
        manifest_sha256 = $ManifestSHA256
        runtime_image = $RuntimeImage
        direct_samples = $directSamples
        platform_samples = $e2e.metrics
        platform_cluster_id = $e2e.platform_cluster_id
        platform_cluster_name = $e2e.platform_cluster_name
        retained = [bool]$KeepPlatformCluster
        downstream_evidence = $e2eArtifact.FullName
    }
} catch {
    $failure = $_
} finally {
    $env:AIOPS_ADMIN_PASSWORD = $previousAdminPassword
    if (-not $KeepPlatformCluster -and -not $metricsServerWasPresent -and (Test-Path -LiteralPath $ManifestPath)) {
        try {
            Invoke-KubectlText @('delete', '-f', $ManifestPath, '--ignore-not-found=true') | Write-Host
        } catch {
            Write-Warning "Metrics Server cleanup failed: $($_.Exception.Message)"
        }
    }
}

if ($null -ne $failure) { throw $failure }

$path = Join-Path $ArtifactDirectory ("metrics-e2e-{0}.json" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
[IO.File]::WriteAllText($path, ($summary | ConvertTo-Json -Depth 12), [Text.UTF8Encoding]::new($false))
Write-Host "Real Metrics available-path verification passed. Evidence: $path"
$summary | ConvertTo-Json -Depth 12
