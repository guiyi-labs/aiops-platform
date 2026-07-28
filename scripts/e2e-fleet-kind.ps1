[CmdletBinding()]
param(
    [string]$KindNodeImage = 'kindest/node:v1.34.0@sha256:7416a61b42b1662ca6ca89f02028ac133a309a2a30ba309614e8ec94d976dc5a',
    [string]$PostgresImage = 'pgvector/pgvector:0.8.1-pg17'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Root = Split-Path -Parent $PSScriptRoot
$Kind = Join-Path $Root '.tools\kind-v0.30.0.exe'
$ObserverManifest = Join-Path $Root 'deploy\managed-cluster\observer.yaml'
$ArtifactDirectory = Join-Path $Root '.artifacts\fleet-e2e'
$RunID = '{0}-{1}' -f (Get-Date -Format 'yyyyMMdd-HHmmss'), ([guid]::NewGuid().ToString('N').Substring(0, 6))
$ClusterNames = @("aiops-fleet-a-$RunID", "aiops-fleet-b-$RunID")
$DockerNetwork = "aiops-fleet-e2e-$RunID"
$PostgresContainer = "aiops-fleet-postgres-$RunID"
$BackendContainer = "aiops-fleet-backend-$RunID"
$BackendImage = "aiops-fleet-backend:$RunID"
$TemporaryDirectory = Join-Path ([IO.Path]::GetTempPath()) "aiops-fleet-e2e-$RunID"

[IO.Directory]::CreateDirectory($ArtifactDirectory) | Out-Null
[IO.Directory]::CreateDirectory($TemporaryDirectory) | Out-Null

function Invoke-NativeText {
    param(
        [Parameter(Mandatory)] [string]$File,
        [Parameter(Mandatory)] [string[]]$Arguments,
        [switch]$AllowFailure
    )

    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $output = & $File @Arguments 2>&1
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorAction
    }
    if ($exitCode -ne 0 -and -not $AllowFailure) {
        throw "$File command failed with exit code $exitCode`: $($output -join [Environment]::NewLine)"
    }
    return (($output | ForEach-Object { $_.ToString() }) -join [Environment]::NewLine).Trim()
}

function Invoke-KubectlText {
    param(
        [Parameter(Mandatory)] [string]$Kubeconfig,
        [Parameter(Mandatory)] [string[]]$Arguments,
        [switch]$AllowFailure
    )
    return Invoke-NativeText -File 'kubectl' -Arguments (@('--kubeconfig', $Kubeconfig) + $Arguments) -AllowFailure:$AllowFailure
}

function Assert-Equal {
    param($Actual, $Expected, [Parameter(Mandatory)] [string]$Message)
    if ($Actual -ne $Expected) {
        throw "$Message; expected $Expected, got $Actual"
    }
}

function Assert-True {
    param([bool]$Condition, [Parameter(Mandatory)] [string]$Message)
    if (-not $Condition) {
        throw $Message
    }
}

function Get-AuthorizationDecision {
    param(
        [Parameter(Mandatory)] [string]$Kubeconfig,
        [Parameter(Mandatory)] [string[]]$Arguments
    )
    $output = Invoke-KubectlText -Kubeconfig $Kubeconfig -Arguments $Arguments -AllowFailure
    $decisions = @($output -split [Environment]::NewLine | ForEach-Object { $_.Trim() } | Where-Object { $_ -in @('yes', 'no') })
    if ($decisions.Count -ne 1) {
        throw "kubectl auth can-i did not return exactly one decision: $output"
    }
    return $decisions[0]
}

function Get-FreePort {
    $listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
    try {
        $listener.Start()
        return ([Net.IPEndPoint]$listener.LocalEndpoint).Port
    } finally {
        $listener.Stop()
    }
}

function New-RandomHex {
    param([int]$Bytes = 32)
    $buffer = [byte[]]::new($Bytes)
    $generator = [Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        $generator.GetBytes($buffer)
    } finally {
        $generator.Dispose()
    }
    return (($buffer | ForEach-Object { $_.ToString('x2') }) -join '')
}

function Write-Utf8File {
    param([Parameter(Mandatory)] [string]$Path, [Parameter(Mandatory)] [string]$Contents)
    [IO.File]::WriteAllText($Path, $Contents, [Text.UTF8Encoding]::new($false))
}

function Wait-DockerHealthy {
    param([Parameter(Mandatory)] [string]$Container, [int]$TimeoutSeconds = 120)
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    do {
        $status = Invoke-NativeText -File 'docker' -Arguments @('inspect', '--format', '{{.State.Health.Status}}', $Container) -AllowFailure
        if ($status -eq 'healthy') {
            return
        }
        if ($status -eq 'unhealthy') {
            throw "$Container became unhealthy"
        }
        Start-Sleep -Seconds 2
    } while ((Get-Date) -lt $deadline)
    throw "$Container did not become healthy within $TimeoutSeconds seconds"
}

function Wait-ApiReady {
    param([Parameter(Mandatory)] [string]$ApiBase, [int]$TimeoutSeconds = 120)
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    do {
        try {
            $response = Invoke-RestMethod -Uri "$ApiBase/api/v1/health/ready" -TimeoutSec 5
            if ($response.status -eq 'ready') {
                return
            }
        } catch {
            # The isolated backend may still be applying migrations.
        }
        Start-Sleep -Seconds 2
    } while ((Get-Date) -lt $deadline)
    throw 'isolated backend did not become ready'
}

function Get-HttpStatus {
    param(
        [Parameter(Mandatory)] [string]$Uri,
        [hashtable]$Headers = @{}
    )
    try {
        Invoke-WebRequest -UseBasicParsing -Uri $Uri -Headers $Headers -TimeoutSec 10 | Out-Null
        return 200
    } catch {
        if ($null -ne $_.Exception.Response) {
            return [int]$_.Exception.Response.StatusCode
        }
        throw
    }
}

function New-ObserverKubeconfig {
    param([Parameter(Mandatory)] [string]$AdminKubeconfig)

    $token = Invoke-KubectlText -Kubeconfig $AdminKubeconfig -Arguments @(
        '-n', 'kube-system', 'create', 'token', 'aiops-platform', '--duration=20m'
    )
    $rawContext = Invoke-KubectlText -Kubeconfig $AdminKubeconfig -Arguments @(
        'config', 'view', '--raw', '--minify', '-o', 'json'
    ) | ConvertFrom-Json
    $serverUri = [Uri][string]$rawContext.clusters[0].cluster.server
    $ca = [string]$rawContext.clusters[0].cluster.'certificate-authority-data'
    Assert-True (-not [string]::IsNullOrWhiteSpace($ca)) 'kind kubeconfig does not contain embedded CA data'
    $builder = [UriBuilder]$serverUri
    $builder.Host = 'host.docker.internal'
    $dockerServer = $builder.Uri.AbsoluteUri.TrimEnd('/')

    return @"
apiVersion: v1
kind: Config
clusters:
  - name: fleet-e2e
    cluster:
      server: $dockerServer
      tls-server-name: $($serverUri.Host)
      certificate-authority-data: $ca
contexts:
  - name: fleet-e2e
    context:
      cluster: fleet-e2e
      user: aiops-platform
current-context: fleet-e2e
users:
  - name: aiops-platform
    user:
      token: $token
"@
}

function New-KindCluster {
    param(
        [Parameter(Mandatory)] [string]$Name,
        [Parameter(Mandatory)] [int]$ApiPort,
        [Parameter(Mandatory)] [string]$Kubeconfig,
        [Parameter(Mandatory)] [string]$KindConfig,
        [Parameter(Mandatory)] [string]$FixtureManifest,
        [Parameter(Mandatory)] [int]$FixtureDeployments
    )

    Write-Utf8File -Path $KindConfig -Contents @"
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  apiServerAddress: "127.0.0.1"
  apiServerPort: $ApiPort
nodes:
  - role: control-plane
"@
    Invoke-NativeText -File $Kind -Arguments @(
        'create', 'cluster', '--name', $Name, '--image', $KindNodeImage,
        '--config', $KindConfig, '--kubeconfig', $Kubeconfig, '--wait', '120s'
    ) | Write-Host
    Invoke-KubectlText -Kubeconfig $Kubeconfig -Arguments @('apply', '-f', $ObserverManifest) | Write-Host

    $documents = [Collections.Generic.List[string]]::new()
    $documents.Add(@"
apiVersion: v1
kind: Namespace
metadata:
  name: fleet-e2e
"@)
    for ($index = 1; $index -le $FixtureDeployments; $index++) {
        $documents.Add(@"
apiVersion: apps/v1
kind: Deployment
metadata:
  name: fixture-$index
  namespace: fleet-e2e
spec:
  replicas: 0
  selector:
    matchLabels:
      app: fixture-$index
  template:
    metadata:
      labels:
        app: fixture-$index
    spec:
      containers:
        - name: pause
          image: registry.k8s.io/pause:3.10
"@)
    }
    Write-Utf8File -Path $FixtureManifest -Contents ($documents -join "`n---`n")
    Invoke-KubectlText -Kubeconfig $Kubeconfig -Arguments @('apply', '-f', $FixtureManifest) | Write-Host
}

function Get-ResourceExpectation {
    param(
        [Parameter(Mandatory)] [string]$ApiBase,
        [Parameter(Mandatory)] [hashtable]$Headers,
        [Parameter(Mandatory)] [int64]$ClusterID
    )

    $resourceBase = "$ApiBase/api/v1/clusters/$ClusterID"
    $nodes = Invoke-RestMethod -Uri "$resourceBase/nodes?limit=100" -Headers $Headers
    $pods = Invoke-RestMethod -Uri "$resourceBase/pods?limit=100" -Headers $Headers
    $deployments = Invoke-RestMethod -Uri "$resourceBase/deployments?limit=100" -Headers $Headers
    $events = Invoke-RestMethod -Uri "$resourceBase/events?limit=100" -Headers $Headers
    return [ordered]@{
        nodes = [int]$nodes.total
        pods = [int]$pods.total
        deployments = [int]$deployments.total
        events = [int]$events.total
        warnings = @($events.items | Where-Object { $_.type -eq 'Warning' }).Count
    }
}

function Assert-FleetCounts {
    param($Item, $Expected, [Parameter(Mandatory)] [string]$Label)
    Assert-Equal ([int]$Item.nodes.total) $Expected.nodes "$Label Node total mismatch"
    Assert-Equal ([int]$Item.pods.total) $Expected.pods "$Label Pod total mismatch"
    Assert-Equal ([int]$Item.deployments.total) $Expected.deployments "$Label Deployment total mismatch"
    Assert-Equal ([int]$Item.warnings.total) $Expected.events "$Label Event total mismatch"
    Assert-Equal ([int]$Item.warnings.count) $Expected.warnings "$Label Warning count mismatch"
    Assert-True ([bool]$Item.nodes.complete -and [bool]$Item.pods.complete -and [bool]$Item.deployments.complete -and [bool]$Item.warnings.complete) "$Label sampling was unexpectedly truncated"
}

if (-not (Test-Path -LiteralPath $Kind)) {
    throw "bundled kind executable is missing: $Kind"
}

$initialKindClusters = @((Invoke-NativeText -File $Kind -Arguments @('get', 'clusters')) -split [Environment]::NewLine | Where-Object { $_ })
$ApiPort = Get-FreePort
$KindApiPorts = @((Get-FreePort), (Get-FreePort))
while ($KindApiPorts[1] -eq $KindApiPorts[0] -or $KindApiPorts -contains $ApiPort) {
    $KindApiPorts = @((Get-FreePort), (Get-FreePort))
}
$ApiBase = "http://127.0.0.1:$ApiPort"
$DbPassword = New-RandomHex 24
$AdminPassword = New-RandomHex 24
$JwtKey = New-RandomHex 32
$CredentialKeyBytes = [byte[]]::new(32)
$credentialGenerator = [Security.Cryptography.RandomNumberGenerator]::Create()
try {
    $credentialGenerator.GetBytes($CredentialKeyBytes)
} finally {
    $credentialGenerator.Dispose()
}
$CredentialKey = [Convert]::ToBase64String($CredentialKeyBytes)

$PostgresEnv = Join-Path $TemporaryDirectory 'postgres.env'
$BackendEnv = Join-Path $TemporaryDirectory 'backend.env'
$Kubeconfigs = @(
    (Join-Path $TemporaryDirectory 'cluster-a.kubeconfig'),
    (Join-Path $TemporaryDirectory 'cluster-b.kubeconfig')
)
$KindConfigs = @(
    (Join-Path $TemporaryDirectory 'cluster-a.kind.yaml'),
    (Join-Path $TemporaryDirectory 'cluster-b.kind.yaml')
)
$FixtureManifests = @(
    (Join-Path $TemporaryDirectory 'cluster-a.fixtures.yaml'),
    (Join-Path $TemporaryDirectory 'cluster-b.fixtures.yaml')
)

Write-Utf8File -Path $PostgresEnv -Contents @"
POSTGRES_DB=fleet_e2e
POSTGRES_USER=fleet_e2e
POSTGRES_PASSWORD=$DbPassword
"@
Write-Utf8File -Path $BackendEnv -Contents @"
APP_ENV=production
HTTP_ADDR=:8080
DATABASE_URL=postgres://fleet_e2e:$DbPassword@postgres:5432/fleet_e2e?sslmode=disable
JWT_SIGNING_KEY=$JwtKey
BOOTSTRAP_ADMIN_USERNAME=fleet-admin
BOOTSTRAP_ADMIN_PASSWORD=$AdminPassword
CREDENTIAL_ENCRYPTION_KEY=$CredentialKey
CREDENTIAL_KEY_VERSION=v1
CLUSTER_PROBE_TIMEOUT=4s
AI_ENABLED=false
NOTIFICATION_ENABLED=false
"@

$backendCreated = $false
$kindCreated = @($false, $false)
$secondControlPlanePaused = $false
$clusterIDs = [Collections.Generic.List[int64]]::new()
$accessToken = ''
$failure = $null
$summary = $null
$cleanupFailures = [Collections.Generic.List[string]]::new()
$cleanup = [ordered]@{
    platform_records_deleted = $false
    kind_clusters_deleted = $false
    preexisting_kind_clusters_preserved = $false
    backend_container_deleted = $false
    postgres_container_deleted = $false
    docker_network_deleted = $false
    backend_image_deleted = $false
    temporary_files_deleted = $false
}

try {
    Invoke-NativeText -File 'docker' -Arguments @('network', 'create', $DockerNetwork) | Out-Null
    Invoke-NativeText -File 'docker' -Arguments @(
        'run', '-d', '--name', $PostgresContainer, '--network', $DockerNetwork,
        '--network-alias', 'postgres', '--env-file', $PostgresEnv,
        '--health-cmd', 'pg_isready -U fleet_e2e -d fleet_e2e',
        '--health-interval', '2s', '--health-timeout', '2s', '--health-retries', '30',
        $PostgresImage
    ) | Out-Null
    Wait-DockerHealthy -Container $PostgresContainer

    Invoke-NativeText -File 'docker' -Arguments @(
        'build', '--label', 'aiops.fleet-e2e=true', '-t', $BackendImage, (Join-Path $Root 'backend')
    ) | Write-Host
    Invoke-NativeText -File 'docker' -Arguments @(
        'run', '-d', '--name', $BackendContainer, '--network', $DockerNetwork,
        '--add-host', 'host.docker.internal:host-gateway', '--env-file', $BackendEnv,
        '-p', "127.0.0.1:$ApiPort`:8080",
        '--health-cmd', 'wget -q -O - http://127.0.0.1:8080/api/v1/health/ready',
        '--health-interval', '2s', '--health-timeout', '2s', '--health-retries', '40',
        $BackendImage
    ) | Out-Null
    $backendCreated = $true
    Wait-DockerHealthy -Container $BackendContainer
    Wait-ApiReady -ApiBase $ApiBase

    New-KindCluster -Name $ClusterNames[0] -ApiPort $KindApiPorts[0] -Kubeconfig $Kubeconfigs[0] -KindConfig $KindConfigs[0] -FixtureManifest $FixtureManifests[0] -FixtureDeployments 1
    $kindCreated[0] = $true
    New-KindCluster -Name $ClusterNames[1] -ApiPort $KindApiPorts[1] -Kubeconfig $Kubeconfigs[1] -KindConfig $KindConfigs[1] -FixtureManifest $FixtureManifests[1] -FixtureDeployments 3
    $kindCreated[1] = $true

    $actor = 'system:serviceaccount:kube-system:aiops-platform'
    $rbac = [ordered]@{}
    for ($index = 0; $index -lt 2; $index++) {
        $canListNodes = Get-AuthorizationDecision -Kubeconfig $Kubeconfigs[$index] -Arguments @('auth', 'can-i', 'list', 'nodes', '--all-namespaces', "--as=$actor")
        $canListEvents = Get-AuthorizationDecision -Kubeconfig $Kubeconfigs[$index] -Arguments @('auth', 'can-i', 'list', 'events', '--all-namespaces', "--as=$actor")
        $canCreateDeployments = Get-AuthorizationDecision -Kubeconfig $Kubeconfigs[$index] -Arguments @('auth', 'can-i', 'create', 'deployments', '-n', 'fleet-e2e', "--as=$actor")
        Assert-Equal $canListNodes 'yes' "cluster $index observer cannot list Nodes"
        Assert-Equal $canListEvents 'yes' "cluster $index observer cannot list Events"
        Assert-Equal $canCreateDeployments 'no' "cluster $index observer unexpectedly can create Deployments"
        $rbac["cluster_$($index + 1)"] = [ordered]@{ list_nodes = $canListNodes; list_events = $canListEvents; create_deployments = $canCreateDeployments }
    }

    $login = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/auth/login" -ContentType 'application/json' -Body (@{
        username = 'fleet-admin'
        password = $AdminPassword
    } | ConvertTo-Json)
    $accessToken = [string]$login.access_token
    $headers = @{ Authorization = "Bearer $accessToken" }

    $anonymousStatus = Get-HttpStatus -Uri "$ApiBase/api/v1/fleet/health"
    Assert-Equal $anonymousStatus 401 'anonymous fleet request was not rejected'
    $invalidLimitStatus = Get-HttpStatus -Uri "$ApiBase/api/v1/fleet/health?limit=0" -Headers $headers
    Assert-Equal $invalidLimitStatus 400 'invalid fleet limit was not rejected'

    for ($index = 0; $index -lt 2; $index++) {
        $observerKubeconfig = New-ObserverKubeconfig -AdminKubeconfig $Kubeconfigs[$index]
        $created = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/clusters" -Headers $headers -ContentType 'application/json' -Body (@{
            name = "fleet-e2e-$($index + 1)-$RunID"
            kubeconfig = $observerKubeconfig
        } | ConvertTo-Json)
        $clusterID = [int64]$created.id
        $clusterIDs.Add($clusterID)
        Invoke-WebRequest -UseBasicParsing -Method Patch -Uri "$ApiBase/api/v1/clusters/$clusterID" -Headers $headers -ContentType 'application/json' -Body '{"enabled":true}' | Out-Null
        $probe = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/clusters/$clusterID/probe" -Headers $headers
        Assert-Equal $probe.status 'ready' "cluster $index probe did not become ready"
    }

    $expectations = @(
        (Get-ResourceExpectation -ApiBase $ApiBase -Headers $headers -ClusterID $clusterIDs[0]),
        (Get-ResourceExpectation -ApiBase $ApiBase -Headers $headers -ClusterID $clusterIDs[1])
    )
    Assert-Equal ($expectations[1].deployments - $expectations[0].deployments) 2 'cluster-specific Deployment fixtures were not distinguishable'

    $fleet = Invoke-RestMethod -Uri "$ApiBase/api/v1/fleet/health?limit=20" -Headers $headers -TimeoutSec 15
    Assert-Equal ([int]$fleet.total) 2 'fleet total mismatch'
    Assert-Equal @($fleet.items).Count 2 'fleet item count mismatch'
    Assert-Equal ([int]$fleet.remaining) 0 'fleet remaining mismatch'
    Assert-Equal ([int]$fleet.limits.max_clusters) 20 'fleet max cluster limit mismatch'
    Assert-Equal ([int]$fleet.limits.max_concurrent_clusters) 4 'fleet concurrency limit mismatch'
    Assert-Equal ([int]$fleet.limits.per_cluster_timeout_ms) 4000 'fleet timeout budget mismatch'
    Assert-Equal ([int]$fleet.limits.resource_sample_limit) 100 'fleet sample limit mismatch'
    Assert-True ([int64]$fleet.items[0].cluster_id -lt [int64]$fleet.items[1].cluster_id) 'fleet items were not sorted by cluster ID'
    for ($index = 0; $index -lt 2; $index++) {
        Assert-True ($fleet.items[$index].status -in @('healthy', 'degraded')) "cluster $index baseline fleet status was $($fleet.items[$index].status)"
        Assert-Equal @($fleet.items[$index].failures).Count 0 "cluster $index baseline returned query failures"
        Assert-FleetCounts -Item $fleet.items[$index] -Expected $expectations[$index] -Label "cluster $index baseline"
    }

    $limited = Invoke-RestMethod -Uri "$ApiBase/api/v1/fleet/health?limit=1" -Headers $headers -TimeoutSec 10
    Assert-Equal ([int]$limited.total) 2 'limited fleet total mismatch'
    Assert-Equal @($limited.items).Count 1 'limited fleet item count mismatch'
    Assert-Equal ([int]$limited.remaining) 1 'limited fleet remaining mismatch'
    Assert-Equal ([int64]$limited.items[0].cluster_id) $clusterIDs[0] 'limited fleet did not retain the lowest cluster ID'

    $secondControlPlane = "$($ClusterNames[1])-control-plane"
    Invoke-NativeText -File 'docker' -Arguments @('pause', $secondControlPlane) | Out-Null
    $secondControlPlanePaused = $true
    $timedOutFleet = Invoke-RestMethod -Uri "$ApiBase/api/v1/fleet/health?limit=20" -Headers $headers -TimeoutSec 15
    Assert-Equal $timedOutFleet.items[0].status $fleet.items[0].status 'healthy cluster changed while its peer timed out'
    Assert-FleetCounts -Item $timedOutFleet.items[0] -Expected $expectations[0] -Label 'surviving cluster during timeout'
    Assert-Equal $timedOutFleet.items[1].status 'timed_out' 'paused cluster was not isolated as timed_out'
    Assert-Equal @($timedOutFleet.items[1].failures).Count 4 'timed-out cluster did not report all four bounded failures'
    Assert-True (@($timedOutFleet.items[1].failures | Where-Object { $_.code -ne 'TIMEOUT' }).Count -eq 0) 'timed-out cluster exposed a non-timeout failure code'

    Invoke-NativeText -File 'docker' -Arguments @('unpause', $secondControlPlane) | Out-Null
    $secondControlPlanePaused = $false
    Invoke-KubectlText -Kubeconfig $Kubeconfigs[1] -Arguments @('wait', '--for=condition=Ready', 'node', '--all', '--timeout=60s') | Out-Null
    $recoveredFleet = Invoke-RestMethod -Uri "$ApiBase/api/v1/fleet/health?limit=20" -Headers $headers -TimeoutSec 15
    Assert-True ($recoveredFleet.items[1].status -in @('healthy', 'degraded')) 'timed-out cluster did not recover'
    Assert-FleetCounts -Item $recoveredFleet.items[1] -Expected $expectations[1] -Label 'recovered cluster'

    Invoke-NativeText -File 'docker' -Arguments @('stop', '--time', '1', $secondControlPlane) | Out-Null
    $unavailableFleet = Invoke-RestMethod -Uri "$ApiBase/api/v1/fleet/health?limit=20" -Headers $headers -TimeoutSec 15
    Assert-Equal $unavailableFleet.items[0].status $fleet.items[0].status 'healthy cluster changed while its peer was unavailable'
    Assert-FleetCounts -Item $unavailableFleet.items[0] -Expected $expectations[0] -Label 'surviving cluster during outage'
    Assert-Equal $unavailableFleet.items[1].status 'unavailable' 'stopped cluster was not isolated as unavailable'
    Assert-Equal @($unavailableFleet.items[1].failures).Count 4 'unavailable cluster did not report four query failures'
    Assert-True (@($unavailableFleet.items[1].failures | Where-Object { $_.code -ne 'QUERY_FAILED' }).Count -eq 0) 'unavailable cluster exposed an unexpected failure code'

    $summary = [ordered]@{
        verified_at = (Get-Date).ToString('o')
        kind_version = Invoke-NativeText -File $Kind -Arguments @('version')
        cluster_count = 2
        cluster_ids_sorted = @($fleet.items | ForEach-Object { [int64]$_.cluster_id })
        limits = $fleet.limits
        authorization = [ordered]@{ anonymous_status = $anonymousStatus; invalid_limit_status = $invalidLimitStatus }
        rbac = $rbac
        baseline = @(
            [ordered]@{ status = $fleet.items[0].status; expected = $expectations[0]; observed = [ordered]@{ nodes = $fleet.items[0].nodes.total; pods = $fleet.items[0].pods.total; deployments = $fleet.items[0].deployments.total; events = $fleet.items[0].warnings.total; warnings = $fleet.items[0].warnings.count } },
            [ordered]@{ status = $fleet.items[1].status; expected = $expectations[1]; observed = [ordered]@{ nodes = $fleet.items[1].nodes.total; pods = $fleet.items[1].pods.total; deployments = $fleet.items[1].deployments.total; events = $fleet.items[1].warnings.total; warnings = $fleet.items[1].warnings.count } }
        )
        limit_one = [ordered]@{ total = $limited.total; returned = @($limited.items).Count; remaining = $limited.remaining; lowest_id_first = $true }
        timeout_isolation = [ordered]@{ survivor_status = $timedOutFleet.items[0].status; failed_status = $timedOutFleet.items[1].status; failure_codes = @($timedOutFleet.items[1].failures | ForEach-Object { $_.code }); duration_ms = $timedOutFleet.items[1].duration_ms }
        recovery = [ordered]@{ status = $recoveredFleet.items[1].status; counts_restored = $true }
        unavailable_isolation = [ordered]@{ survivor_status = $unavailableFleet.items[0].status; failed_status = $unavailableFleet.items[1].status; failure_codes = @($unavailableFleet.items[1].failures | ForEach-Object { $_.code }) }
    }
} catch {
    $failure = $_
} finally {
    if ($secondControlPlanePaused) {
        Invoke-NativeText -File 'docker' -Arguments @('unpause', "$($ClusterNames[1])-control-plane") -AllowFailure | Out-Null
        $secondControlPlanePaused = $false
    }

    if ($clusterIDs.Count -gt 0 -and -not [string]::IsNullOrWhiteSpace($accessToken) -and $backendCreated) {
        try {
            $cleanupHeaders = @{ Authorization = "Bearer $accessToken" }
            foreach ($clusterID in $clusterIDs) {
                Invoke-WebRequest -UseBasicParsing -Method Delete -Uri "$ApiBase/api/v1/clusters/$clusterID" -Headers $cleanupHeaders -TimeoutSec 10 | Out-Null
            }
            $remaining = Invoke-RestMethod -Uri "$ApiBase/api/v1/clusters" -Headers $cleanupHeaders -TimeoutSec 10
            $cleanup.platform_records_deleted = @($remaining.items).Count -eq 0
            if (-not $cleanup.platform_records_deleted) {
                throw 'temporary platform cluster records remain'
            }
        } catch {
            $cleanupFailures.Add("platform cleanup failed: $($_.Exception.Message)")
        }
    }

    $kindClustersBeforeCleanup = @((Invoke-NativeText -File $Kind -Arguments @('get', 'clusters')) -split [Environment]::NewLine | Where-Object { $_ })
    for ($index = 0; $index -lt 2; $index++) {
        if ($kindCreated[$index] -or $ClusterNames[$index] -in $kindClustersBeforeCleanup) {
            try {
                Invoke-NativeText -File $Kind -Arguments @('delete', 'cluster', '--name', $ClusterNames[$index]) | Write-Host
            } catch {
                $cleanupFailures.Add("kind cleanup failed for index $index`: $($_.Exception.Message)")
            }
        }
    }
    $remainingKindClusters = @((Invoke-NativeText -File $Kind -Arguments @('get', 'clusters')) -split [Environment]::NewLine | Where-Object { $_ })
    $cleanup.kind_clusters_deleted = @($ClusterNames | Where-Object { $_ -in $remainingKindClusters }).Count -eq 0
    $cleanup.preexisting_kind_clusters_preserved = @(Compare-Object ($initialKindClusters | Sort-Object) ($remainingKindClusters | Sort-Object)).Count -eq 0
    if (-not $cleanup.kind_clusters_deleted -or -not $cleanup.preexisting_kind_clusters_preserved) {
        $cleanupFailures.Add('kind cluster cleanup did not restore the initial cluster set')
    }

    Invoke-NativeText -File 'docker' -Arguments @('rm', '-f', $BackendContainer) -AllowFailure | Out-Null
    $cleanup.backend_container_deleted = [string]::IsNullOrWhiteSpace((Invoke-NativeText -File 'docker' -Arguments @('ps', '-aq', '--filter', "name=^/$BackendContainer`$") -AllowFailure))
    if (-not $cleanup.backend_container_deleted) { $cleanupFailures.Add('temporary backend container remains') }

    Invoke-NativeText -File 'docker' -Arguments @('rm', '-f', $PostgresContainer) -AllowFailure | Out-Null
    $cleanup.postgres_container_deleted = [string]::IsNullOrWhiteSpace((Invoke-NativeText -File 'docker' -Arguments @('ps', '-aq', '--filter', "name=^/$PostgresContainer`$") -AllowFailure))
    if (-not $cleanup.postgres_container_deleted) { $cleanupFailures.Add('temporary PostgreSQL container remains') }

    Invoke-NativeText -File 'docker' -Arguments @('network', 'rm', $DockerNetwork) -AllowFailure | Out-Null
    $cleanup.docker_network_deleted = [string]::IsNullOrWhiteSpace((Invoke-NativeText -File 'docker' -Arguments @('network', 'ls', '-q', '--filter', "name=^$DockerNetwork`$") -AllowFailure))
    if (-not $cleanup.docker_network_deleted) { $cleanupFailures.Add('temporary Docker network remains') }

    Invoke-NativeText -File 'docker' -Arguments @('image', 'rm', $BackendImage) -AllowFailure | Out-Null
    $cleanup.backend_image_deleted = [string]::IsNullOrWhiteSpace((Invoke-NativeText -File 'docker' -Arguments @('image', 'ls', '-q', $BackendImage) -AllowFailure))
    if (-not $cleanup.backend_image_deleted) { $cleanupFailures.Add('temporary backend image tag remains') }

    Remove-Item -LiteralPath $TemporaryDirectory -Recurse -Force -ErrorAction SilentlyContinue
    $cleanup.temporary_files_deleted = -not (Test-Path -LiteralPath $TemporaryDirectory)
    if (-not $cleanup.temporary_files_deleted) { $cleanupFailures.Add('temporary credential files remain') }
}

if ($cleanupFailures.Count -gt 0) {
    $cleanupError = $cleanupFailures -join '; '
    if ($null -ne $failure) {
        throw "$($failure.Exception.Message); $cleanupError"
    }
    throw $cleanupError
}
if ($null -ne $failure) {
    throw $failure
}

$summary.cleanup = $cleanup
$path = Join-Path $ArtifactDirectory ("fleet-e2e-{0}.json" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
Write-Utf8File -Path $path -Contents ($summary | ConvertTo-Json -Depth 12)
Write-Host "Disposable two-cluster fleet verification passed. Evidence: $path"
$summary | ConvertTo-Json -Depth 12
