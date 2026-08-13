[CmdletBinding()]
param(
    [string]$ApiBase = 'http://127.0.0.1:8080',
    [string]$Username = '',
    [string]$Password = '',
    [int]$ReadyTimeoutSeconds = 120
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Root = Split-Path -Parent $PSScriptRoot
$ArtifactDirectory = Join-Path $Root '.artifacts\metrics-history-e2e'
$RunID = '{0}-{1}' -f (Get-Date -Format 'yyyyMMddHHmmss'), ([guid]::NewGuid().ToString('N').Substring(0, 8))
[IO.Directory]::CreateDirectory($ArtifactDirectory) | Out-Null

function Get-RuntimeValue {
    param([Parameter(Mandatory)] [string]$Name, [Parameter(Mandatory)] [string]$Fallback)

    $processValue = [Environment]::GetEnvironmentVariable($Name, 'Process')
    if (-not [string]::IsNullOrWhiteSpace($processValue)) { return $processValue }
    $environmentPath = Join-Path $Root '.env'
    if (Test-Path -LiteralPath $environmentPath) {
        $prefix = "$Name="
        $line = Get-Content -LiteralPath $environmentPath | Where-Object { $_.StartsWith($prefix, [StringComparison]::Ordinal) } | Select-Object -Last 1
        if ($null -ne $line) { return $line.Substring($prefix.Length) }
    }
    return $Fallback
}

function Invoke-ComposeText {
    param([Parameter(Mandatory)] [string[]]$Arguments, [switch]$AllowFailure)

    Push-Location $Root
    try {
        $previousErrorAction = $ErrorActionPreference
        try {
            $ErrorActionPreference = 'Continue'
            $output = & docker compose @Arguments 2>&1
            $exitCode = $LASTEXITCODE
        } finally {
            $ErrorActionPreference = $previousErrorAction
        }
    } finally {
        Pop-Location
    }
    $text = (($output | ForEach-Object { $_.ToString() }) -join [Environment]::NewLine).Trim()
    if ($exitCode -ne 0 -and -not $AllowFailure) {
        throw "docker compose command failed with exit code $exitCode`: $text"
    }
    return $text
}

function Invoke-PsqlScalar {
    param([Parameter(Mandatory)] [string]$Query)

    return (Invoke-ComposeText -Arguments @('exec', '-T', 'postgres', 'psql', '--username', $script:DatabaseUser,
        '--dbname', $script:DatabaseName, '--set', 'ON_ERROR_STOP=1', '--quiet', '--tuples-only', '--no-align', '--command', $Query)).Trim()
}

function Wait-BackendReady {
    $deadline = (Get-Date).AddSeconds($ReadyTimeoutSeconds)
    do {
        try {
            $health = Invoke-RestMethod "$ApiBase/api/v1/health/ready" -TimeoutSec 5
            if ($health.status -eq 'ready') { return }
        } catch {}
        Start-Sleep -Seconds 2
    } while ((Get-Date) -lt $deadline)
    throw 'backend did not become ready before the metrics-history E2E deadline'
}

function Invoke-HistoryQuery {
    param([Parameter(Mandatory)] [int64]$ClusterID, [Parameter(Mandatory)] [string]$AccessToken,
        [Parameter(Mandatory)] [datetime]$From, [Parameter(Mandatory)] [datetime]$To)

    $query = @(
        'resource_kind=Node',
        'name=worker-a',
        'metric=cpu',
        "from=$([uri]::EscapeDataString($From.ToUniversalTime().ToString('o')))",
        "to=$([uri]::EscapeDataString($To.ToUniversalTime().ToString('o')))",
        'limit=10'
    ) -join '&'
    return Invoke-RestMethod -Uri "$ApiBase/api/v1/clusters/$ClusterID/metrics/history?$query" -Headers @{ Authorization = "Bearer $AccessToken" } -TimeoutSec 10
}

function Invoke-HistoryEvaluation {
    param([Parameter(Mandatory)] [int64]$ClusterID, [Parameter(Mandatory)] [string]$AccessToken,
        [Parameter(Mandatory)] [datetime]$From, [Parameter(Mandatory)] [datetime]$To,
        [Parameter(Mandatory)] [int64]$Threshold, [Parameter(Mandatory)] [int]$ForSeconds)

    $query = @(
        'resource_kind=Node', 'name=worker-a', 'metric=cpu',
        "from=$([uri]::EscapeDataString($From.ToUniversalTime().ToString('o')))",
        "to=$([uri]::EscapeDataString($To.ToUniversalTime().ToString('o')))",
        'operator=gte', "threshold=$Threshold", "for_seconds=$ForSeconds", 'minimum_points=2'
    ) -join '&'
    return Invoke-RestMethod -Uri "$ApiBase/api/v1/clusters/$ClusterID/metrics/history/evaluate?$query" -Headers @{ Authorization = "Bearer $AccessToken" } -TimeoutSec 10
}

function Assert-HistoryEvaluation {
    param([Parameter(Mandatory)] $Response, [Parameter(Mandatory)] [string]$State,
        [Parameter(Mandatory)] [int64]$Threshold, [Parameter(Mandatory)] [int]$ForSeconds)

    if ($Response.state -ne $State -or $Response.operator -ne 'gte' -or [int64]$Response.threshold -ne $Threshold -or
        [int]$Response.for_seconds -ne $ForSeconds -or [int]$Response.minimum_points -ne 2) {
        throw "unexpected deterministic history evaluation: state=$($Response.state)"
    }
}

function Assert-HistoryResponse {
    param([Parameter(Mandatory)] $Response, [Parameter(Mandatory)] [int64]$ClusterID)

    if ([int64]$Response.series.cluster_id -ne $ClusterID -or $Response.series.resource_kind -ne 'Node' -or
        $Response.series.resource_name -ne 'worker-a' -or $Response.series.metric_name -ne 'cpu' -or
        $Response.series.unit -ne 'nanocores') { throw 'exact series identity was not preserved' }
    if (@($Response.points).Count -ne 3 -or [int64]$Response.points[0].value -ne 100 -or
        [int64]$Response.points[1].value -ne 300 -or [int64]$Response.points[2].value -ne 400) {
        throw 'query leaked another cluster/series or returned unstable point ordering'
    }
    if ([int]$Response.coverage.collections -ne 4 -or [int]$Response.coverage.succeeded -ne 2 -or
        [int]$Response.coverage.partial -ne 1 -or [int]$Response.coverage.unavailable -ne 1 -or
        [int]$Response.coverage.points -ne 3 -or [int]$Response.coverage.missing -ne 1) {
        throw 'sparse collection coverage does not match the seeded history'
    }
    if ([bool]$Response.truncated -or [int]$Response.limits.max_window_seconds -ne 86400 -or [int]$Response.limits.max_points -ne 1440) {
        throw 'history response limits or truncation metadata are invalid'
    }
}

$DatabaseName = Get-RuntimeValue -Name 'POSTGRES_DB' -Fallback 'aiops'
$DatabaseUser = Get-RuntimeValue -Name 'POSTGRES_USER' -Fallback 'aiops'
if ([string]::IsNullOrWhiteSpace($Username)) { $Username = Get-RuntimeValue -Name 'BOOTSTRAP_ADMIN_USERNAME' -Fallback 'admin' }
if ([string]::IsNullOrWhiteSpace($Password)) { $Password = Get-RuntimeValue -Name 'BOOTSTRAP_ADMIN_PASSWORD' -Fallback 'admin123' }
$targetName = "metrics-history-target-$RunID"
$decoyName = "metrics-history-decoy-$RunID"

$targetClusterID = 0L
$decoyClusterID = 0L
$failure = $null
$cleanupComplete = $false
try {
    Write-Host '[1/5] Waiting for the existing Compose backend'
    Wait-BackendReady
    $login = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/auth/login" -ContentType 'application/json' `
        -Body (@{ username = $Username; password = $Password } | ConvertTo-Json -Compress) -TimeoutSec 10
    $accessToken = [string]$login.access_token
    if ([string]::IsNullOrWhiteSpace($accessToken)) { throw 'login did not return an access token' }

    Write-Host '[2/5] Seeding isolated exact-series, sparse-gap and cross-cluster fixtures'
    $targetClusterID = [int64](Invoke-PsqlScalar "INSERT INTO clusters (name, api_server, status) VALUES ('$targetName', 'https://127.0.0.1:6443', 'disabled') RETURNING id")
    $decoyClusterID = [int64](Invoke-PsqlScalar "INSERT INTO clusters (name, api_server, status) VALUES ('$decoyName', 'https://127.0.0.1:6443', 'disabled') RETURNING id")
    $now = (Get-Date).ToUniversalTime()
    $from = $now.AddHours(-1)
    $to = $now.AddMinutes(1)
    $run1At = $now.AddMinutes(-30).ToString('o')
    $run2At = $now.AddMinutes(-20).ToString('o')
    $run3At = $now.AddMinutes(-10).ToString('o')
    $run4At = $now.AddMinutes(-5).ToString('o')
    $decoyAt = $now.AddMinutes(-3).ToString('o')
    $expiresAt = $now.AddDays(7).ToString('o')

    $run1 = [int64](Invoke-PsqlScalar "INSERT INTO metric_collection_runs (cluster_id,status,nodes_status,nodes_sampled,nodes_total,nodes_complete,pods_status,pods_sampled,pods_total,pods_complete,failure_code,started_at,completed_at,expires_at) VALUES ($targetClusterID,'succeeded','succeeded',1,1,TRUE,'succeeded',0,0,TRUE,'','$run1At','$run1At','$expiresAt') RETURNING id")
    Invoke-PsqlScalar "INSERT INTO metric_samples (collection_run_id,cluster_id,resource_kind,resource_namespace,resource_name,resource_uid,container_name,metric_name,value,unit,source_timestamp,window_milliseconds,collected_at,expires_at) VALUES ($run1,$targetClusterID,'Node','','worker-a','uid-worker-a','','cpu',100,'nanocores','$run1At',15000,'$run1At','$expiresAt'),($run1,$targetClusterID,'Node','','worker-a','uid-worker-a','','memory',500,'bytes','$run1At',15000,'$run1At','$expiresAt')" | Out-Null
    Invoke-PsqlScalar "INSERT INTO metric_collection_runs (cluster_id,status,nodes_status,nodes_sampled,nodes_total,nodes_complete,pods_status,pods_sampled,pods_total,pods_complete,failure_code,started_at,completed_at,expires_at) VALUES ($targetClusterID,'unavailable','unavailable',0,0,FALSE,'unavailable',0,0,FALSE,'METRICS_API_UNAVAILABLE','$run2At','$run2At','$expiresAt')" | Out-Null
    $run3 = [int64](Invoke-PsqlScalar "INSERT INTO metric_collection_runs (cluster_id,status,nodes_status,nodes_sampled,nodes_total,nodes_complete,pods_status,pods_sampled,pods_total,pods_complete,failure_code,started_at,completed_at,expires_at) VALUES ($targetClusterID,'partial','succeeded',2,2,TRUE,'unavailable',0,0,FALSE,'METRICS_API_UNAVAILABLE','$run3At','$run3At','$expiresAt') RETURNING id")
    Invoke-PsqlScalar "INSERT INTO metric_samples (collection_run_id,cluster_id,resource_kind,resource_namespace,resource_name,resource_uid,container_name,metric_name,value,unit,source_timestamp,window_milliseconds,collected_at,expires_at) VALUES ($run3,$targetClusterID,'Node','','worker-a','uid-worker-a','','cpu',300,'nanocores','$run3At',15000,'$run3At','$expiresAt'),($run3,$targetClusterID,'Node','','worker-b','uid-worker-b','','cpu',999,'nanocores','$run3At',15000,'$run3At','$expiresAt')" | Out-Null
    $run4 = [int64](Invoke-PsqlScalar "INSERT INTO metric_collection_runs (cluster_id,status,nodes_status,nodes_sampled,nodes_total,nodes_complete,pods_status,pods_sampled,pods_total,pods_complete,failure_code,started_at,completed_at,expires_at) VALUES ($targetClusterID,'succeeded','succeeded',1,1,TRUE,'succeeded',0,0,TRUE,'','$run4At','$run4At','$expiresAt') RETURNING id")
    Invoke-PsqlScalar "INSERT INTO metric_samples (collection_run_id,cluster_id,resource_kind,resource_namespace,resource_name,resource_uid,container_name,metric_name,value,unit,source_timestamp,window_milliseconds,collected_at,expires_at) VALUES ($run4,$targetClusterID,'Node','','worker-a','uid-worker-a','','cpu',400,'nanocores','$run4At',15000,'$run4At','$expiresAt')" | Out-Null
    $decoyRun = [int64](Invoke-PsqlScalar "INSERT INTO metric_collection_runs (cluster_id,status,nodes_status,nodes_sampled,nodes_total,nodes_complete,pods_status,pods_sampled,pods_total,pods_complete,failure_code,started_at,completed_at,expires_at) VALUES ($decoyClusterID,'succeeded','succeeded',1,1,TRUE,'succeeded',0,0,TRUE,'','$decoyAt','$decoyAt','$expiresAt') RETURNING id")
    Invoke-PsqlScalar "INSERT INTO metric_samples (collection_run_id,cluster_id,resource_kind,resource_namespace,resource_name,resource_uid,container_name,metric_name,value,unit,source_timestamp,window_milliseconds,collected_at,expires_at) VALUES ($decoyRun,$decoyClusterID,'Node','','worker-a','uid-worker-a','','cpu',777,'nanocores','$decoyAt',15000,'$decoyAt','$expiresAt')" | Out-Null

    Write-Host '[3/5] Proving exact-series isolation and sparse missing-sample semantics'
    $beforeRestart = Invoke-HistoryQuery -ClusterID $targetClusterID -AccessToken $accessToken -From $from -To $to
    Assert-HistoryResponse -Response $beforeRestart -ClusterID $targetClusterID
    $completeFrom = $now.AddMinutes(-11)
    $firing = Invoke-HistoryEvaluation -ClusterID $targetClusterID -AccessToken $accessToken -From $completeFrom -To $to -Threshold 300 -ForSeconds 300
    Assert-HistoryEvaluation -Response $firing -State 'firing' -Threshold 300 -ForSeconds 300
    $normal = Invoke-HistoryEvaluation -ClusterID $targetClusterID -AccessToken $accessToken -From $completeFrom -To $to -Threshold 350 -ForSeconds 300
    Assert-HistoryEvaluation -Response $normal -State 'normal' -Threshold 350 -ForSeconds 300
    $insufficient = Invoke-HistoryEvaluation -ClusterID $targetClusterID -AccessToken $accessToken -From $from -To $to -Threshold 100 -ForSeconds 300
    Assert-HistoryEvaluation -Response $insufficient -State 'insufficient_data' -Threshold 100 -ForSeconds 300

    Write-Host '[4/5] Restarting the backend and proving PostgreSQL durability'
    Invoke-ComposeText -Arguments @('restart', 'backend') | Out-Null
    Wait-BackendReady
    $afterRestart = Invoke-HistoryQuery -ClusterID $targetClusterID -AccessToken $accessToken -From $from -To $to
    Assert-HistoryResponse -Response $afterRestart -ClusterID $targetClusterID
    $afterRestartEvaluation = Invoke-HistoryEvaluation -ClusterID $targetClusterID -AccessToken $accessToken -From $completeFrom -To $to -Threshold 300 -ForSeconds 300
    Assert-HistoryEvaluation -Response $afterRestartEvaluation -State 'firing' -Threshold 300 -ForSeconds 300
    $summary = [ordered]@{
        verified_at = (Get-Date).ToString('o')
        target_cluster_id = $targetClusterID
        points = 3
        collections = 4
        missing = 1
        cross_cluster_isolation = $true
        exact_series_isolation = $true
        stable_ordering = $true
        evaluation_states = @('firing', 'normal', 'insufficient_data')
        restart_durability = $true
    }
} catch {
    $failure = $_
} finally {
    Write-Host '[5/5] Removing synthetic clusters and cascaded history fixtures'
    Invoke-PsqlScalar "DELETE FROM clusters WHERE name IN ('$targetName','$decoyName')" | Out-Null
    $remainingClusters = [int](Invoke-PsqlScalar "SELECT COUNT(*) FROM clusters WHERE name IN ('$targetName','$decoyName')")
    $ids = @($targetClusterID, $decoyClusterID) | Where-Object { $_ -gt 0 }
    $remainingHistory = if ($ids.Count -gt 0) {
        [int](Invoke-PsqlScalar "SELECT COUNT(*) FROM metric_collection_runs WHERE cluster_id IN ($($ids -join ','))")
    } else { 0 }
    $cleanupComplete = $remainingClusters -eq 0 -and $remainingHistory -eq 0
}

if ($null -ne $failure) { throw $failure }
if (-not $cleanupComplete) { throw 'metrics-history E2E fixture cleanup was incomplete' }
$summary.cleanup_complete = $cleanupComplete
$evidencePath = Join-Path $ArtifactDirectory ("metrics-history-e2e-{0}.json" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
[IO.File]::WriteAllText($evidencePath, ($summary | ConvertTo-Json -Depth 10), [Text.UTF8Encoding]::new($false))
Write-Host "Metrics history E2E passed. Evidence: $evidencePath"
$summary | ConvertTo-Json -Depth 10
