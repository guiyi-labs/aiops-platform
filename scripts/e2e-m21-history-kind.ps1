<#
.SYNOPSIS
Runs the final M21 history and continuous-window acceptance against a disposable real kind cluster.

.DESCRIPTION
Requires Docker, kind, kubectl, the repository Compose backend with M21 routes, and registry access for the
pinned Metrics Server and workload images. The run normally takes 8-15 minutes because it waits for real
collector intervals, a persisted Metrics API outage, recovery, and a clean 60-second evaluation window.

The script creates a uniquely named kind cluster and platform registration, never reuses aiops-test, and removes
only resources bearing this run's identifier in finally. It writes a redacted success summary under
.artifacts/m21-history-kind; credentials, tokens and kubeconfig material remain in memory and are never archived.
Using -SkipBackendRestart omits the restart-durability assertion and therefore is not a full final acceptance.
#>
[CmdletBinding()]
param(
    [string]$ApiBase = 'http://127.0.0.1:8080',
    [string]$Username = '',
    [string]$AdminPassword = $env:AIOPS_ADMIN_PASSWORD,
    [string]$KindNodeImage = '',
    [string]$WorkloadImage = 'alpine:3.22',
    [int]$ReadyTimeoutSeconds = 180,
    [int]$CollectionTimeoutSeconds = 480,
    [int]$MinimumAvailablePoints = 2,
    [switch]$SkipBackendRestart
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Root = Split-Path -Parent $PSScriptRoot
$ArtifactDirectory = Join-Path $Root '.artifacts\m21-history-kind'
$RunID = '{0}-{1}' -f (Get-Date -Format 'yyyyMMddHHmmss'), ([guid]::NewGuid().ToString('N').Substring(0, 8))
$KindClusterName = "m21-history-$RunID"
$Context = "kind-$KindClusterName"
$Namespace = "m21-$($RunID.ToLowerInvariant())"
$TargetPodName = "history-target-$($RunID.ToLowerInvariant())"
$DecoyPodName = "history-decoy-$($RunID.ToLowerInvariant())"
$M21WorkloadImage = "aiops-m21-workload:$RunID"
$PlatformClusterName = "m21-kind-$RunID"
$MetricsManifestPath = Join-Path $Root 'deploy\metrics-server-kind\components-v0.8.0.yaml'
$MetricsPatchPath = Join-Path $Root 'deploy\metrics-server-kind\kind-patch.json'
$MetricsManifestSHA256 = 'ff64d1a13b9ac3b0635f0dd985815fb44c23eed4706c04e5db1daadf6bc0a83b'
$kindCommand = Get-Command kind -ErrorAction SilentlyContinue
$Kind = if ($null -ne $kindCommand) { $kindCommand.Source } else { Join-Path $Root '.tools\kind-v0.30.0.exe' }
if (-not (Test-Path -LiteralPath $Kind)) { throw 'kind executable is required' }

if ($MinimumAvailablePoints -lt 2 -or $MinimumAvailablePoints -gt 20) {
    throw 'MinimumAvailablePoints must be from 2 through 20'
}
if ($ReadyTimeoutSeconds -lt 30 -or $CollectionTimeoutSeconds -lt 120) {
    throw 'ready timeout must be at least 30 seconds and collection timeout at least 120 seconds'
}

function Get-RuntimeValue {
    param([Parameter(Mandatory)] [string]$Name, [string]$Fallback = '')

    $processValue = [Environment]::GetEnvironmentVariable($Name, 'Process')
    if (-not [string]::IsNullOrWhiteSpace($processValue)) { return $processValue }
    $environmentPath = Join-Path $Root '.env'
    if (Test-Path -LiteralPath $environmentPath) {
        $prefix = "$Name="
        $line = Get-Content -LiteralPath $environmentPath |
            Where-Object { $_.StartsWith($prefix, [StringComparison]::Ordinal) } |
            Select-Object -Last 1
        if ($null -ne $line) { return $line.Substring($prefix.Length) }
    }
    return $Fallback
}

function Invoke-NativeText {
    param(
        [Parameter(Mandatory)] [string]$FilePath,
        [Parameter(Mandatory)] [string[]]$Arguments,
        [string]$InputText,
        [switch]$AllowFailure
    )

    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        if ($PSBoundParameters.ContainsKey('InputText')) {
            $output = $InputText | & $FilePath @Arguments 2>&1
        } else {
            $output = & $FilePath @Arguments 2>&1
        }
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorAction
    }
    $text = (($output | ForEach-Object {
        if ($_ -is [System.Management.Automation.ErrorRecord]) { $_.Exception.Message } else { $_.ToString() }
    }) -join [Environment]::NewLine).Trim()
    if ($exitCode -ne 0 -and -not $AllowFailure) {
        throw "$FilePath $($Arguments -join ' ') failed with exit code $exitCode`: $text"
    }
    return $text
}

function Test-DockerImageExists {
    param([Parameter(Mandatory)] [string]$Image)

    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        & docker image inspect $Image *> $null
        return $LASTEXITCODE -eq 0
    } finally {
        $ErrorActionPreference = $previousErrorAction
    }
}

function Invoke-KubectlText {
    param([Parameter(Mandatory, Position = 0)] [string[]]$Arguments, [switch]$AllowFailure)
    return Invoke-NativeText -FilePath 'kubectl' -Arguments (@('--context', $Context) + $Arguments) -AllowFailure:$AllowFailure
}

function Invoke-KubectlInput {
    param([Parameter(Mandatory)] [string]$Body, [Parameter(Mandatory)] [string[]]$Arguments)

    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $output = $Body | & kubectl --context $Context @Arguments 2>&1
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorAction
    }
    if ($exitCode -ne 0) {
        throw "kubectl $($Arguments -join ' ') failed: $($output -join [Environment]::NewLine)"
    }
    return (($output | ForEach-Object { $_.ToString() }) -join [Environment]::NewLine).Trim()
}

function Invoke-ComposeText {
    param([Parameter(Mandatory)] [string[]]$Arguments)

    Push-Location $Root
    try {
        return Invoke-NativeText -FilePath 'docker' -Arguments (@('compose') + $Arguments)
    } finally {
        Pop-Location
    }
}

function Wait-BackendReady {
    $deadline = (Get-Date).AddSeconds($ReadyTimeoutSeconds)
    do {
        try {
            $ready = Invoke-RestMethod -Uri "$ApiBase/api/v1/health/ready" -TimeoutSec 5
            if ($ready.status -eq 'ready') { return }
        } catch {}
        Start-Sleep -Seconds 2
    } while ((Get-Date) -lt $deadline)
    throw 'platform backend did not become ready before the deadline'
}

function Wait-MetricsSamples {
    $deadline = (Get-Date).AddSeconds($ReadyTimeoutSeconds)
    do {
        try {
            $nodes = Invoke-KubectlText @('get', '--raw', '/apis/metrics.k8s.io/v1beta1/nodes') | ConvertFrom-Json
            $pods = Invoke-KubectlText @('get', '--raw', "/apis/metrics.k8s.io/v1beta1/namespaces/$Namespace/pods") | ConvertFrom-Json
            $target = @($pods.items | Where-Object { $_.metadata.name -eq $TargetPodName })
            $decoy = @($pods.items | Where-Object { $_.metadata.name -eq $DecoyPodName })
            if (@($nodes.items).Count -gt 0 -and $target.Count -eq 1 -and $decoy.Count -eq 1) {
                return [ordered]@{ nodes = @($nodes.items).Count; target_pods = $target.Count; decoy_pods = $decoy.Count }
            }
        } catch {
            # Aggregated discovery becomes available before kubelet metrics are populated.
        }
        Start-Sleep -Seconds 5
    } while ((Get-Date) -lt $deadline)
    throw 'Metrics API did not publish Node and both isolated Pod samples before the deadline'
}

function New-HistoryQuery {
    param(
        [Parameter(Mandatory)] [string]$Kind,
        [Parameter(Mandatory)] [string]$Name,
        [Parameter(Mandatory)] [string]$Metric,
        [Parameter(Mandatory)] [datetime]$From,
        [Parameter(Mandatory)] [datetime]$To,
        [string]$ResourceNamespace = '',
        [string]$Container = '',
        [hashtable]$Additional = @{}
    )

    $pairs = [ordered]@{
        resource_kind = $Kind
        name = $Name
        metric = $Metric
        from = $From.ToUniversalTime().ToString('o')
        to = $To.ToUniversalTime().ToString('o')
        limit = '1440'
    }
    if ($Kind -eq 'Pod') {
        $pairs.namespace = $ResourceNamespace
        $pairs.container = $Container
    }
    foreach ($key in $Additional.Keys) { $pairs[$key] = [string]$Additional[$key] }
    return ($pairs.GetEnumerator() | ForEach-Object {
        '{0}={1}' -f [uri]::EscapeDataString([string]$_.Key), [uri]::EscapeDataString([string]$_.Value)
    }) -join '&'
}

function Invoke-HistoryQuery {
    param(
        [Parameter(Mandatory)] [int64]$ClusterID,
        [Parameter(Mandatory)] [hashtable]$Headers,
        [Parameter(Mandatory)] [string]$Kind,
        [Parameter(Mandatory)] [string]$Name,
        [Parameter(Mandatory)] [string]$Metric,
        [Parameter(Mandatory)] [datetime]$From,
        [Parameter(Mandatory)] [datetime]$To,
        [string]$ResourceNamespace = '',
        [string]$Container = ''
    )

    $query = New-HistoryQuery -Kind $Kind -Name $Name -Metric $Metric -From $From -To $To `
        -ResourceNamespace $ResourceNamespace -Container $Container
    return Invoke-RestMethod -Uri "$ApiBase/api/v1/clusters/$ClusterID/metrics/history?$query" -Headers $Headers -TimeoutSec 15
}

function Assert-HistorySeries {
    param(
        [Parameter(Mandatory)]$Response,
        [Parameter(Mandatory)] [int64]$ClusterID,
        [Parameter(Mandatory)] [string]$Kind,
        [Parameter(Mandatory)] [string]$Name,
        [Parameter(Mandatory)] [string]$Metric,
        [string]$ResourceNamespace = '',
        [string]$Container = '',
        [int]$MinimumPoints = 1
    )

    $expectedUnit = if ($Metric -eq 'cpu') { 'nanocores' } else { 'bytes' }
    if ([int64]$Response.series.cluster_id -ne $ClusterID -or
        [string]$Response.series.resource_kind -ne $Kind -or
        [string]$Response.series.resource_name -ne $Name -or
        [string]$Response.series.metric_name -ne $Metric -or
        [string]$Response.series.unit -ne $expectedUnit) {
        throw "$Kind/$Name $Metric history did not preserve exact series identity and unit"
    }
    if ($Kind -eq 'Pod' -and
        ([string]$Response.series.resource_namespace -ne $ResourceNamespace -or
         [string]$Response.series.container_name -ne $Container)) {
        throw "Pod/$ResourceNamespace/$Name/$Container history crossed an exact-series boundary"
    }
    $points = @($Response.points)
    if ($points.Count -lt $MinimumPoints) {
        throw "$Kind/$Name $Metric returned $($points.Count) points; expected at least $MinimumPoints"
    }
    $previous = [datetime]::MinValue
    foreach ($point in $points) {
        $collectedAt = [datetime]::Parse([string]$point.collected_at).ToUniversalTime()
        if ($collectedAt -lt $previous) { throw "$Kind/$Name $Metric points are not stably ordered" }
        if ([int64]$point.value -lt 0) { throw "$Kind/$Name $Metric contains a negative quantity" }
        if ([string]::IsNullOrWhiteSpace([string]$point.source_timestamp) -or [int64]$point.window_milliseconds -lt 1000) {
            throw "$Kind/$Name $Metric point metadata is incomplete"
        }
        $previous = $collectedAt
    }
    if ([int]$Response.coverage.points -ne $points.Count -or [int]$Response.coverage.collections -lt $points.Count) {
        throw "$Kind/$Name $Metric coverage does not describe its sparse point set"
    }
    return $points.Count
}

function Wait-HistoryPoints {
    param(
        [Parameter(Mandatory)] [int64]$ClusterID,
        [Parameter(Mandatory)] [hashtable]$Headers,
        [Parameter(Mandatory)] [string]$Kind,
        [Parameter(Mandatory)] [string]$Name,
        [Parameter(Mandatory)] [string]$Metric,
        [Parameter(Mandatory)] [datetime]$From,
        [string]$ResourceNamespace = '',
        [string]$Container = '',
        [int]$MinimumPoints = $MinimumAvailablePoints
    )

    $deadline = (Get-Date).AddSeconds($CollectionTimeoutSeconds)
    do {
        try {
            $response = Invoke-HistoryQuery -ClusterID $ClusterID -Headers $Headers -Kind $Kind -Name $Name `
                -Metric $Metric -From $From -To ((Get-Date).ToUniversalTime().AddMinutes(1)) `
                -ResourceNamespace $ResourceNamespace -Container $Container
            if (@($response.points).Count -ge $MinimumPoints) { return $response }
        } catch {
            # A just-enabled cluster may not have completed its first collection yet.
        }
        Start-Sleep -Seconds 5
    } while ((Get-Date) -lt $deadline)
    throw "$Kind/$Name $Metric did not reach $MinimumPoints collected points before the deadline"
}

function Wait-SparseGap {
    param(
        [Parameter(Mandatory)] [int64]$ClusterID,
        [Parameter(Mandatory)] [hashtable]$Headers,
        [Parameter(Mandatory)] [string]$NodeName,
        [Parameter(Mandatory)] [datetime]$From,
        [Parameter(Mandatory)]$Baseline
    )

    $baselineUnavailable = [int]$Baseline.coverage.unavailable + [int]$Baseline.coverage.timed_out + [int]$Baseline.coverage.failed
    $deadline = (Get-Date).AddSeconds($CollectionTimeoutSeconds)
    do {
        try {
            $response = Invoke-HistoryQuery -ClusterID $ClusterID -Headers $Headers -Kind 'Node' -Name $NodeName `
                -Metric 'cpu' -From $From -To ((Get-Date).ToUniversalTime().AddMinutes(1))
            $unavailable = [int]$response.coverage.unavailable + [int]$response.coverage.timed_out + [int]$response.coverage.failed
            if ($unavailable -gt $baselineUnavailable -and [int]$response.coverage.missing -gt 0) { return $response }
        } catch {}
        Start-Sleep -Seconds 5
    } while ((Get-Date) -lt $deadline)
    throw 'collector did not persist an explicit sparse Metrics API outage before the deadline'
}

function Invoke-EvaluationQuery {
    param(
        [Parameter(Mandatory)] [int64]$ClusterID,
        [Parameter(Mandatory)] [hashtable]$Headers,
        [Parameter(Mandatory)] [string]$NodeName,
        [Parameter(Mandatory)] [datetime]$From,
        [Parameter(Mandatory)] [datetime]$To,
        [Parameter(Mandatory)] [string]$Operator,
        [Parameter(Mandatory)] [string]$Threshold,
        [Parameter(Mandatory)] [int]$ForSeconds,
        [Parameter(Mandatory)] [int]$MinimumPoints
    )

    $additional = @{
        operator = $Operator
        threshold = $Threshold
        for_seconds = $ForSeconds
        minimum_points = $MinimumPoints
    }
    $query = New-HistoryQuery -Kind 'Node' -Name $NodeName -Metric 'cpu' -From $From -To $To -Additional $additional
    return Invoke-RestMethod -Uri "$ApiBase/api/v1/clusters/$ClusterID/metrics/history/evaluate?$query" -Headers $Headers -TimeoutSec 15
}

function Assert-Evaluation {
    param(
        [Parameter(Mandatory)]$Response,
        [Parameter(Mandatory)] [string]$ExpectedState,
        [Parameter(Mandatory)] [string]$ExpectedOperator,
        [Parameter(Mandatory)] [int64]$ExpectedThreshold,
        [Parameter(Mandatory)] [int]$ExpectedForSeconds,
        [Parameter(Mandatory)] [int]$ExpectedMinimumPoints,
        [Parameter(Mandatory)] [int64]$ClusterID,
        [Parameter(Mandatory)] [string]$NodeName
    )

    if ([string]$Response.state -ne $ExpectedState -or [string]$Response.operator -ne $ExpectedOperator -or
        [int64]$Response.threshold -ne $ExpectedThreshold -or [int]$Response.for_seconds -ne $ExpectedForSeconds -or
        [int]$Response.minimum_points -ne $ExpectedMinimumPoints) {
        throw "evaluation contract mismatch; expected $ExpectedState/$ExpectedOperator/$ExpectedThreshold"
    }
    if ([int64]$Response.series.cluster_id -ne $ClusterID -or [string]$Response.series.resource_kind -ne 'Node' -or
        [string]$Response.series.resource_name -ne $NodeName -or [string]$Response.series.metric_name -ne 'cpu') {
        throw 'evaluation did not echo its exact series identity'
    }
    if ($null -eq $Response.coverage -or [string]::IsNullOrWhiteSpace([string]$Response.from) -or
        [string]::IsNullOrWhiteSpace([string]$Response.to) -or [int]$Response.points_evaluated -lt 0 -or
        [int]$Response.breaching_points -lt 0 -or [int]$Response.observed_span_seconds -lt 0) {
        throw 'evaluation omitted coverage, window or deterministic evidence counters'
    }
}

foreach ($command in @('docker', 'kubectl')) {
    if ($null -eq (Get-Command $command -ErrorAction SilentlyContinue)) { throw "$command is required for the M21 kind acceptance" }
}
if (-not (Test-Path -LiteralPath $MetricsManifestPath) -or -not (Test-Path -LiteralPath $MetricsPatchPath)) {
    throw 'the pinned Metrics Server manifest and kind patch are required'
}
$actualMetricsHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $MetricsManifestPath).Hash.ToLowerInvariant()
if ($actualMetricsHash -ne $MetricsManifestSHA256) {
    throw "Metrics Server manifest checksum mismatch; expected $MetricsManifestSHA256, got $actualMetricsHash"
}
if ([string]::IsNullOrWhiteSpace($Username)) { $Username = Get-RuntimeValue -Name 'BOOTSTRAP_ADMIN_USERNAME' -Fallback 'admin' }
if ([string]::IsNullOrWhiteSpace($AdminPassword)) { $AdminPassword = Get-RuntimeValue -Name 'BOOTSTRAP_ADMIN_PASSWORD' }
if ([string]::IsNullOrWhiteSpace($AdminPassword)) {
    throw 'set AIOPS_ADMIN_PASSWORD, pass -AdminPassword, or configure BOOTSTRAP_ADMIN_PASSWORD in .env'
}

$kindCreated = $false
$workloadImageBuilt = $false
$platformClusterID = 0L
$accessToken = ''
$failure = $null
$cleanupErrors = [Collections.Generic.List[string]]::new()
$summary = $null
$historyFrom = (Get-Date).ToUniversalTime().AddMinutes(-1)

Write-Host '[0/9] Ensuring the workload image is cached (docker image inspect / docker pull)'
if (-not (Test-DockerImageExists -Image $WorkloadImage)) {
    Invoke-NativeText -FilePath 'docker' -Arguments @('pull', $WorkloadImage) | Write-Host
}
$workloadDockerfile = "FROM $WorkloadImage`n"
Write-Host "[0/9] Building a disposable single-platform workload image (docker build --platform linux/amd64): $M21WorkloadImage"
Invoke-NativeText -FilePath 'docker' -Arguments @('build', '--pull=false', '--load', '--platform', 'linux/amd64', '-t', $M21WorkloadImage, '-') -InputText $workloadDockerfile | Write-Host
$workloadImageBuilt = $true

$existingKindClusters = @(Invoke-NativeText -FilePath $Kind -Arguments @('get', 'clusters') -AllowFailure -ErrorAction Stop -WarningAction SilentlyContinue -InformationAction SilentlyContinue) -split "`r?`n" |
    Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
if ($existingKindClusters -contains $KindClusterName) { throw "refusing to reuse existing kind cluster $KindClusterName" }
$AiopsTestWasPresent = $existingKindClusters -contains 'aiops-test'
$PreviousContext = (Invoke-NativeText -FilePath 'kubectl' -Arguments @('config', 'current-context') -AllowFailure).Trim()
$knownContexts = @(Invoke-NativeText -FilePath 'kubectl' -Arguments @('config', 'get-contexts', '-o', 'name') -AllowFailure) -split "`r?`n"
if ($knownContexts -notcontains $PreviousContext) { $PreviousContext = '' }

try {
    Write-Host '[1/9] Checking the isolated platform runtime and creating a disposable kind cluster'
    Wait-BackendReady
    $kindArguments = @('create', 'cluster', '--name', $KindClusterName, '--wait', "$ReadyTimeoutSeconds`s")
    if (-not [string]::IsNullOrWhiteSpace($KindNodeImage)) { $kindArguments += @('--image', $KindNodeImage) }
    Invoke-NativeText -FilePath $Kind -Arguments $kindArguments | Write-Host
    $kindCreated = $true
    Invoke-NativeText -FilePath $Kind -Arguments @('load', 'docker-image', $M21WorkloadImage, '--name', $KindClusterName) | Write-Host

    Write-Host '[2/9] Installing the pinned Metrics Server and two unique workload fixtures'
    Invoke-KubectlText @('apply', '-f', $MetricsManifestPath) | Write-Host
    Invoke-KubectlText @('-n', 'kube-system', 'patch', 'deployment', 'metrics-server', '--type=json', '--patch-file', $MetricsPatchPath) | Write-Host
    Invoke-KubectlText @('-n', 'kube-system', 'rollout', 'status', 'deployment/metrics-server', "--timeout=$ReadyTimeoutSeconds`s") | Write-Host
    Invoke-KubectlText @('wait', '--for=condition=Available', 'apiservice/v1beta1.metrics.k8s.io', "--timeout=$ReadyTimeoutSeconds`s") | Write-Host

    $workloads = @'
apiVersion: v1
kind: Namespace
metadata:
  name: __NAMESPACE__
---
apiVersion: v1
kind: Pod
metadata:
  name: __TARGET__
  namespace: __NAMESPACE__
  labels:
    app.kubernetes.io/name: m21-history-target
spec:
  automountServiceAccountToken: false
  containers:
    - name: load
      image: __IMAGE__
      imagePullPolicy: IfNotPresent
      command: ["sh", "-c", "while true; do value=$((value + 1)); done"]
      resources:
        requests: { cpu: 10m, memory: 8Mi }
        limits: { cpu: 100m, memory: 32Mi }
      securityContext:
        allowPrivilegeEscalation: false
        capabilities: { drop: ["ALL"] }
  restartPolicy: Always
---
apiVersion: v1
kind: Pod
metadata:
  name: __DECOY__
  namespace: __NAMESPACE__
  labels:
    app.kubernetes.io/name: m21-history-decoy
spec:
  automountServiceAccountToken: false
  containers:
    - name: decoy
      image: __IMAGE__
      imagePullPolicy: IfNotPresent
      command: ["sh", "-c", "while true; do sleep 5; done"]
      resources:
        requests: { cpu: 1m, memory: 4Mi }
        limits: { cpu: 20m, memory: 16Mi }
      securityContext:
        allowPrivilegeEscalation: false
        capabilities: { drop: ["ALL"] }
  restartPolicy: Always
'@
    $workloads = $workloads.Replace('__NAMESPACE__', $Namespace).Replace('__TARGET__', $TargetPodName).
        Replace('__DECOY__', $DecoyPodName).Replace('__IMAGE__', $M21WorkloadImage)
    Invoke-KubectlInput -Body $workloads -Arguments @('apply', '-f', '-') | Write-Host
    Invoke-KubectlText @('-n', $Namespace, 'wait', '--for=condition=Ready', "pod/$TargetPodName", "pod/$DecoyPodName", "--timeout=$ReadyTimeoutSeconds`s") | Write-Host
    $directSamples = Wait-MetricsSamples

    Write-Host '[3/9] Installing least-privilege observer RBAC and registering only the disposable cluster'
    Invoke-KubectlText @('apply', '-f', (Join-Path $Root 'deploy\managed-cluster\observer.yaml')) | Write-Host
    $token = Invoke-KubectlText @('-n', 'kube-system', 'create', 'token', 'aiops-platform', '--duration=1h')
    $rawContext = Invoke-KubectlText @('config', 'view', '--raw', '--minify', '-o', 'json') | ConvertFrom-Json
    $server = [string]$rawContext.clusters[0].cluster.server
    $ca = [string]$rawContext.clusters[0].cluster.'certificate-authority-data'
    if ([string]::IsNullOrWhiteSpace($server) -or [string]::IsNullOrWhiteSpace($ca) -or [string]::IsNullOrWhiteSpace($token)) {
        throw 'disposable kind context did not provide server, CA and short-lived token material'
    }
    $serverUri = [Uri]$server
    $tlsServerNameLine = ''
    if ($serverUri.IsLoopback) {
        $tlsServerNameLine = "      tls-server-name: $($serverUri.Host)`n"
        $builder = [UriBuilder]$serverUri
        $builder.Host = 'host.docker.internal'
        $server = $builder.Uri.AbsoluteUri.TrimEnd('/')
    }
    $kubeconfig = @"
apiVersion: v1
kind: Config
clusters:
  - name: m21
    cluster:
      server: $server
$tlsServerNameLine      certificate-authority-data: $ca
contexts:
  - name: m21
    context: { cluster: m21, user: aiops-platform }
current-context: m21
users:
  - name: aiops-platform
    user: { token: $token }
"@
    $login = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/auth/login" -ContentType 'application/json' `
        -Body (@{ username = $Username; password = $AdminPassword } | ConvertTo-Json -Compress) -TimeoutSec 15
    $accessToken = [string]$login.access_token
    if ([string]::IsNullOrWhiteSpace($accessToken)) { throw 'platform login did not return an access token' }
    $headers = @{ Authorization = "Bearer $accessToken" }
    $cluster = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/clusters" -Headers $headers -ContentType 'application/json' `
        -Body (@{ name = $PlatformClusterName; kubeconfig = $kubeconfig } | ConvertTo-Json) -TimeoutSec 20
    $platformClusterID = [int64]$cluster.id
    Invoke-WebRequest -UseBasicParsing -Method Patch -Uri "$ApiBase/api/v1/clusters/$platformClusterID" -Headers $headers `
        -ContentType 'application/json' -Body '{"enabled":true}' -TimeoutSec 15 | Out-Null
    $probe = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/clusters/$platformClusterID/probe" -Headers $headers -TimeoutSec 20
    if ($probe.status -ne 'ready') { throw 'disposable cluster probe did not become ready' }

    Write-Host '[4/9] Waiting for real collector history across Node and Pod CPU/memory series'
    $nodeName = [string](Invoke-KubectlText @('get', 'nodes', '-o', 'jsonpath={.items[0].metadata.name}'))
    if ([string]::IsNullOrWhiteSpace($nodeName)) { throw 'disposable cluster did not report a Node name' }
    $nodeCPU = Wait-HistoryPoints -ClusterID $platformClusterID -Headers $headers -Kind 'Node' -Name $nodeName -Metric 'cpu' -From $historyFrom
    $nodeMemory = Wait-HistoryPoints -ClusterID $platformClusterID -Headers $headers -Kind 'Node' -Name $nodeName -Metric 'memory' -From $historyFrom
    $podCPU = Wait-HistoryPoints -ClusterID $platformClusterID -Headers $headers -Kind 'Pod' -Name $TargetPodName -Metric 'cpu' `
        -From $historyFrom -ResourceNamespace $Namespace -Container 'load'
    $podMemory = Wait-HistoryPoints -ClusterID $platformClusterID -Headers $headers -Kind 'Pod' -Name $TargetPodName -Metric 'memory' `
        -From $historyFrom -ResourceNamespace $Namespace -Container 'load'
    $decoyCPU = Wait-HistoryPoints -ClusterID $platformClusterID -Headers $headers -Kind 'Pod' -Name $DecoyPodName -Metric 'cpu' `
        -From $historyFrom -ResourceNamespace $Namespace -Container 'decoy'

    $seriesCounts = [ordered]@{
        node_cpu = Assert-HistorySeries -Response $nodeCPU -ClusterID $platformClusterID -Kind 'Node' -Name $nodeName -Metric 'cpu' -MinimumPoints $MinimumAvailablePoints
        node_memory = Assert-HistorySeries -Response $nodeMemory -ClusterID $platformClusterID -Kind 'Node' -Name $nodeName -Metric 'memory' -MinimumPoints $MinimumAvailablePoints
        pod_cpu = Assert-HistorySeries -Response $podCPU -ClusterID $platformClusterID -Kind 'Pod' -Name $TargetPodName -Metric 'cpu' -ResourceNamespace $Namespace -Container 'load' -MinimumPoints $MinimumAvailablePoints
        pod_memory = Assert-HistorySeries -Response $podMemory -ClusterID $platformClusterID -Kind 'Pod' -Name $TargetPodName -Metric 'memory' -ResourceNamespace $Namespace -Container 'load' -MinimumPoints $MinimumAvailablePoints
        decoy_cpu = Assert-HistorySeries -Response $decoyCPU -ClusterID $platformClusterID -Kind 'Pod' -Name $DecoyPodName -Metric 'cpu' -ResourceNamespace $Namespace -Container 'decoy' -MinimumPoints $MinimumAvailablePoints
    }

    Write-Host '[5/9] Making Metrics API unavailable and proving an explicit sparse gap'
    Invoke-KubectlText @('-n', 'kube-system', 'scale', 'deployment/metrics-server', '--replicas=0') | Write-Host
    Invoke-KubectlText @('-n', 'kube-system', 'wait', '--for=delete', 'pod', '-l', 'k8s-app=metrics-server', "--timeout=$ReadyTimeoutSeconds`s") | Write-Host
    $gapHistory = Wait-SparseGap -ClusterID $platformClusterID -Headers $headers -NodeName $nodeName -From $historyFrom -Baseline $nodeCPU
    $gapPointCount = Assert-HistorySeries -Response $gapHistory -ClusterID $platformClusterID -Kind 'Node' -Name $nodeName -Metric 'cpu' -MinimumPoints $MinimumAvailablePoints

    Write-Host '[6/9] Restoring Metrics API and proving collector recovery without zero filling'
    Invoke-KubectlText @('-n', 'kube-system', 'scale', 'deployment/metrics-server', '--replicas=1') | Write-Host
    Invoke-KubectlText @('-n', 'kube-system', 'rollout', 'status', 'deployment/metrics-server', "--timeout=$ReadyTimeoutSeconds`s") | Write-Host
    Invoke-KubectlText @('wait', '--for=condition=Available', 'apiservice/v1beta1.metrics.k8s.io', "--timeout=$ReadyTimeoutSeconds`s") | Write-Host
    Wait-MetricsSamples | Out-Null
    $recoveryFrom = (Get-Date).ToUniversalTime()
    # Three post-recovery points guarantee at least one full 60-second evaluator window at the default 1m cadence.
    $recoveredCPU = Wait-HistoryPoints -ClusterID $platformClusterID -Headers $headers -Kind 'Node' -Name $nodeName -Metric 'cpu' `
        -From $recoveryFrom -MinimumPoints 3
    $recoveredPointCount = Assert-HistorySeries -Response $recoveredCPU -ClusterID $platformClusterID -Kind 'Node' -Name $nodeName -Metric 'cpu' -MinimumPoints 3
    if ([int]$recoveredCPU.coverage.missing -ne 0) { throw 'post-recovery evaluation window unexpectedly contains a collection gap' }
    $combinedRecoveredCPU = Invoke-HistoryQuery -ClusterID $platformClusterID -Headers $headers -Kind 'Node' -Name $nodeName `
        -Metric 'cpu' -From $historyFrom -To ((Get-Date).ToUniversalTime().AddMinutes(1))
    if ([int]$combinedRecoveredCPU.coverage.missing -lt 1) { throw 'recovered history lost the explicit sparse outage coverage' }

    Write-Host '[7/9] Verifying deterministic firing, normal and insufficient-data window evaluation'
    $evaluationDeadline = (Get-Date).AddSeconds($CollectionTimeoutSeconds)
    $firing = $null
    do {
        $evaluationTo = (Get-Date).ToUniversalTime().AddMinutes(1)
        $firing = Invoke-EvaluationQuery -ClusterID $platformClusterID -Headers $headers -NodeName $nodeName -From $recoveryFrom -To $evaluationTo `
            -Operator 'gte' -Threshold '0' -ForSeconds 60 -MinimumPoints 2
        if ([string]$firing.state -eq 'firing') { break }
        Start-Sleep -Seconds 5
    } while ((Get-Date) -lt $evaluationDeadline)
    Assert-Evaluation -Response $firing -ExpectedState 'firing' -ExpectedOperator 'gte' -ExpectedThreshold 0 -ExpectedForSeconds 60 `
        -ExpectedMinimumPoints 2 -ClusterID $platformClusterID -NodeName $nodeName
    $evaluationTo = (Get-Date).ToUniversalTime().AddMinutes(1)
    $normalThreshold = [int64]::MaxValue
    $normal = Invoke-EvaluationQuery -ClusterID $platformClusterID -Headers $headers -NodeName $nodeName -From $recoveryFrom -To $evaluationTo `
        -Operator 'gte' -Threshold ([string]$normalThreshold) -ForSeconds 60 -MinimumPoints 2
    Assert-Evaluation -Response $normal -ExpectedState 'normal' -ExpectedOperator 'gte' -ExpectedThreshold $normalThreshold -ExpectedForSeconds 60 `
        -ExpectedMinimumPoints 2 -ClusterID $platformClusterID -NodeName $nodeName
    $insufficient = Invoke-EvaluationQuery -ClusterID $platformClusterID -Headers $headers -NodeName $nodeName -From $historyFrom -To $evaluationTo `
        -Operator 'gte' -Threshold '0' -ForSeconds 60 -MinimumPoints 2
    Assert-Evaluation -Response $insufficient -ExpectedState 'insufficient_data' -ExpectedOperator 'gte' -ExpectedThreshold 0 -ExpectedForSeconds 60 `
        -ExpectedMinimumPoints 2 -ClusterID $platformClusterID -NodeName $nodeName

    Write-Host '[8/9] Restarting the Compose backend and proving PostgreSQL durability'
    if (-not $SkipBackendRestart) {
        Invoke-ComposeText @('restart', 'backend') | Write-Host
        Wait-BackendReady
        $afterRestart = Invoke-HistoryQuery -ClusterID $platformClusterID -Headers $headers -Kind 'Node' -Name $nodeName -Metric 'cpu' `
            -From $historyFrom -To ((Get-Date).ToUniversalTime().AddMinutes(1))
        $afterRestartCount = Assert-HistorySeries -Response $afterRestart -ClusterID $platformClusterID -Kind 'Node' -Name $nodeName `
            -Metric 'cpu' -MinimumPoints $recoveredPointCount
        if ([int]$afterRestart.coverage.missing -lt 1) { throw 'backend restart lost sparse coverage evidence' }
    } else {
        $afterRestartCount = $recoveredPointCount
    }

    $summary = [ordered]@{
        verified_at = (Get-Date).ToUniversalTime().ToString('o')
        mode = 'disposable-kind-isolated-registration'
        kind_cluster = $KindClusterName
        preexisting_aiops_test = $AiopsTestWasPresent
        platform_cluster_id = $platformClusterID
        direct_metrics = $directSamples
        series_points = $seriesCounts
        exact_series_isolation = $true
        stable_ordering = $true
        units_verified = @('nanocores', 'bytes')
        sparse_gap = [ordered]@{
            missing = [int]$combinedRecoveredCPU.coverage.missing
            unavailable = [int]$gapHistory.coverage.unavailable
            timed_out = [int]$gapHistory.coverage.timed_out
            failed = [int]$gapHistory.coverage.failed
            recovered_points = $recoveredPointCount
        }
        evaluation = [ordered]@{
            firing = [string]$firing.state
            normal = [string]$normal.state
            insufficient_data = [string]$insufficient.state
        }
        backend_restart_durability = -not [bool]$SkipBackendRestart
        points_after_restart = $afterRestartCount
        credential_lifetime = '1h'
        cleanup_complete = $false
    }
} catch {
    $failure = $_
} finally {
    Write-Host '[9/9] Removing only this run registration and disposable kind cluster'
    if ($platformClusterID -gt 0 -and -not [string]::IsNullOrWhiteSpace($accessToken)) {
        try {
            Invoke-WebRequest -UseBasicParsing -Method Delete -Uri "$ApiBase/api/v1/clusters/$platformClusterID" `
                -Headers @{ Authorization = "Bearer $accessToken" } -TimeoutSec 20 | Out-Null
        } catch { $cleanupErrors.Add('platform cluster registration cleanup failed') }
    }
    if ($workloadImageBuilt) {
        Write-Host '[9/9] Removing the disposable workload image (docker image rm)'
        try { Invoke-NativeText -FilePath 'docker' -Arguments @('image', 'rm', '--force', $M21WorkloadImage) -AllowFailure | Write-Host }
        catch { $cleanupErrors.Add('disposable workload image cleanup failed') }
    }
    if ($kindCreated) {
        if (-not $KindClusterName.StartsWith('m21-history-', [StringComparison]::Ordinal)) {
            $cleanupErrors.Add('refused unsafe kind cleanup target')
        } else {
            try { Invoke-NativeText -FilePath $Kind -Arguments @('delete', 'cluster', '--name', $KindClusterName) | Write-Host }
            catch { $cleanupErrors.Add('disposable kind cluster cleanup failed') }
        }
    }
    if (-not [string]::IsNullOrWhiteSpace($PreviousContext)) {
        try { Invoke-NativeText -FilePath 'kubectl' -Arguments @('config', 'use-context', $PreviousContext) -AllowFailure | Out-Null }
        catch { $cleanupErrors.Add('previous kubectl context restore failed') }
    }
    $remainingKindClusters = @(Invoke-NativeText -FilePath $Kind -Arguments @('get', 'clusters') -AllowFailure) -split "`r?`n" |
        Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
    if ($remainingKindClusters -contains $KindClusterName) { $cleanupErrors.Add('disposable kind cluster still exists') }
    if ($AiopsTestWasPresent -and $remainingKindClusters -notcontains 'aiops-test') { $cleanupErrors.Add('pre-existing aiops-test was not preserved') }
}

if ($cleanupErrors.Count -gt 0) {
    $cleanupFailure = $cleanupErrors -join '; '
    if ($null -eq $failure) { throw $cleanupFailure }
    throw "$($failure.Exception.Message); cleanup: $cleanupFailure"
}
if ($null -ne $failure) { throw $failure }

$summary.cleanup_complete = $true
$summary.aiops_test_preservation_verified = $true
[IO.Directory]::CreateDirectory($ArtifactDirectory) | Out-Null
$evidencePath = Join-Path $ArtifactDirectory ("m21-history-kind-{0}.json" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
[IO.File]::WriteAllText($evidencePath, ($summary | ConvertTo-Json -Depth 12), [Text.UTF8Encoding]::new($false))
Write-Host "M21 real kind history acceptance passed. Redacted evidence: $evidencePath"
$summary | ConvertTo-Json -Depth 12
