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
$ArtifactDirectory = Join-Path $Root '.artifacts\search-e2e'
$RunID = '{0}-{1}' -f (Get-Date -Format 'yyyyMMdd-HHmmss'), ([guid]::NewGuid().ToString('N').Substring(0, 6))
$ClusterNames = @("aiops-search-a-$RunID", "aiops-search-b-$RunID")
$DockerNetwork = "aiops-search-e2e-$RunID"
$PostgresContainer = "aiops-search-postgres-$RunID"
$BackendContainer = "aiops-search-backend-$RunID"
$BackendImage = "aiops-search-backend:$RunID"
$TemporaryDirectory = Join-Path ([IO.Path]::GetTempPath()) "aiops-search-e2e-$RunID"
$SearchNamespace = 'search-e2e'
$SearchTerm = 'search'
$FixedKinds = @('Pod', 'Deployment', 'Service', 'Ingress')

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

function Assert-SequenceEqual {
    param(
        [Parameter(Mandatory)] [object[]]$Actual,
        [Parameter(Mandatory)] [object[]]$Expected,
        [Parameter(Mandatory)] [string]$Message
    )
    Assert-Equal $Actual.Count $Expected.Count "$Message length mismatch"
    for ($index = 0; $index -lt $Expected.Count; $index++) {
        Assert-Equal ([string]$Actual[$index]) ([string]$Expected[$index]) "$Message at index $index"
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
        Invoke-WebRequest -UseBasicParsing -Uri $Uri -Headers $Headers -TimeoutSec 15 | Out-Null
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
  - name: search-e2e
    cluster:
      server: $dockerServer
      tls-server-name: $($serverUri.Host)
      certificate-authority-data: $ca
contexts:
  - name: search-e2e
    context:
      cluster: search-e2e
      user: aiops-platform
current-context: search-e2e
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
        [Parameter(Mandatory)] [string]$Suffix,
        [switch]$AddExtraPod
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

    $extraPod = ''
    if ($AddExtraPod) {
        $extraPod = @"
---
apiVersion: v1
kind: Pod
metadata:
  name: search-gamma-pod
  namespace: $SearchNamespace
spec:
  containers:
    - name: pause
      image: registry.k8s.io/pause:3.10
"@
    }
    Write-Utf8File -Path $FixtureManifest -Contents @"
apiVersion: v1
kind: Namespace
metadata:
  name: $SearchNamespace
---
apiVersion: v1
kind: Pod
metadata:
  name: search-$Suffix-pod
  namespace: $SearchNamespace
  labels:
    app: search-$Suffix
spec:
  containers:
    - name: pause
      image: registry.k8s.io/pause:3.10
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: search-$Suffix-deployment
  namespace: $SearchNamespace
spec:
  replicas: 0
  selector:
    matchLabels:
      app: search-$Suffix-deployment
  template:
    metadata:
      labels:
        app: search-$Suffix-deployment
    spec:
      containers:
        - name: pause
          image: registry.k8s.io/pause:3.10
---
apiVersion: v1
kind: Service
metadata:
  name: search-$Suffix-service
  namespace: $SearchNamespace
spec:
  selector:
    app: search-$Suffix
  ports:
    - name: http
      port: 80
      targetPort: 8080
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: search-$Suffix-ingress
  namespace: $SearchNamespace
spec:
  rules:
    - host: search-$Suffix.example.test
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: search-$Suffix-service
                port:
                  number: 80
$extraPod
"@
    Invoke-KubectlText -Kubeconfig $Kubeconfig -Arguments @('apply', '-f', $FixtureManifest) | Write-Host
}

function Get-ItemKeys {
    param([object[]]$Items)
    return @($Items | ForEach-Object { '{0}|{1}|{2}|{3}' -f $_.cluster_id, $_.kind, $_.namespace, $_.name })
}

function Assert-HealthyPeer {
    param(
        $Response,
        [Parameter(Mandatory)] [int64]$ClusterID,
        [Parameter(Mandatory)] [int]$ExpectedCount,
        [Parameter(Mandatory)] [string]$Message
    )
    $items = @($Response.items | Where-Object { [int64]$_.cluster_id -eq $ClusterID })
    Assert-Equal $items.Count $ExpectedCount "$Message result count"
    Assert-True (@($Response.failures | Where-Object { [int64]$_.cluster_id -eq $ClusterID }).Count -eq 0) "$Message exposed a failure"
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
POSTGRES_DB=search_e2e
POSTGRES_USER=search_e2e
POSTGRES_PASSWORD=$DbPassword
"@
Write-Utf8File -Path $BackendEnv -Contents @"
APP_ENV=production
HTTP_ADDR=:8080
DATABASE_URL=postgres://search_e2e:$DbPassword@postgres:5432/search_e2e?sslmode=disable
JWT_SIGNING_KEY=$JwtKey
BOOTSTRAP_ADMIN_USERNAME=search-admin
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
        '--health-cmd', 'pg_isready -U search_e2e -d search_e2e',
        '--health-interval', '2s', '--health-timeout', '2s', '--health-retries', '30',
        $PostgresImage
    ) | Out-Null
    Wait-DockerHealthy -Container $PostgresContainer

    Invoke-NativeText -File 'docker' -Arguments @(
        'build', '--label', 'aiops.search-e2e=true', '-t', $BackendImage, (Join-Path $Root 'backend')
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

    New-KindCluster -Name $ClusterNames[0] -ApiPort $KindApiPorts[0] -Kubeconfig $Kubeconfigs[0] -KindConfig $KindConfigs[0] -FixtureManifest $FixtureManifests[0] -Suffix 'alpha'
    $kindCreated[0] = $true
    New-KindCluster -Name $ClusterNames[1] -ApiPort $KindApiPorts[1] -Kubeconfig $Kubeconfigs[1] -KindConfig $KindConfigs[1] -FixtureManifest $FixtureManifests[1] -Suffix 'beta' -AddExtraPod
    $kindCreated[1] = $true

    $actor = 'system:serviceaccount:kube-system:aiops-platform'
    $rbac = [ordered]@{}
    $rbacResources = @('pods', 'deployments.apps', 'services', 'ingresses.networking.k8s.io')
    for ($index = 0; $index -lt 2; $index++) {
        $clusterRBAC = [ordered]@{}
        foreach ($resource in $rbacResources) {
            $canList = Get-AuthorizationDecision -Kubeconfig $Kubeconfigs[$index] -Arguments @('auth', 'can-i', 'list', $resource, '-n', $SearchNamespace, "--as=$actor")
            $canCreate = Get-AuthorizationDecision -Kubeconfig $Kubeconfigs[$index] -Arguments @('auth', 'can-i', 'create', $resource, '-n', $SearchNamespace, "--as=$actor")
            Assert-Equal $canList 'yes' "cluster $index observer cannot list $resource"
            Assert-Equal $canCreate 'no' "cluster $index observer unexpectedly can create $resource"
            $clusterRBAC[$resource] = [ordered]@{ list = $canList; create = $canCreate }
        }
        $rbac["cluster_$($index + 1)"] = $clusterRBAC
    }

    $login = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/auth/login" -ContentType 'application/json' -Body (@{
        username = 'search-admin'
        password = $AdminPassword
    } | ConvertTo-Json)
    $accessToken = [string]$login.access_token
    $headers = @{ Authorization = "Bearer $accessToken" }
    $searchBase = "$ApiBase/api/v1/fleet/resources/search"

    $anonymousStatus = Get-HttpStatus -Uri "${searchBase}?q=$SearchTerm"
    Assert-Equal $anonymousStatus 401 'anonymous global search was not rejected'
    $invalidStatuses = [ordered]@{
        short_query = Get-HttpStatus -Uri "${searchBase}?q=x" -Headers $headers
        unknown_kind = Get-HttpStatus -Uri "${searchBase}?q=$SearchTerm&kinds=secrets" -Headers $headers
        zero_cluster_limit = Get-HttpStatus -Uri "${searchBase}?q=$SearchTerm&cluster_limit=0" -Headers $headers
        excess_result_limit = Get-HttpStatus -Uri "${searchBase}?q=$SearchTerm&limit=101" -Headers $headers
    }
    foreach ($entry in $invalidStatuses.GetEnumerator()) {
        Assert-Equal ([int]$entry.Value) 400 "invalid search input $($entry.Key) was not rejected"
    }

    for ($index = 0; $index -lt 2; $index++) {
        $observerKubeconfig = New-ObserverKubeconfig -AdminKubeconfig $Kubeconfigs[$index]
        $created = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/clusters" -Headers $headers -ContentType 'application/json' -Body (@{
            name = "search-e2e-$($index + 1)-$RunID"
            kubeconfig = $observerKubeconfig
        } | ConvertTo-Json)
        $clusterID = [int64]$created.id
        $clusterIDs.Add($clusterID)
        Invoke-WebRequest -UseBasicParsing -Method Patch -Uri "$ApiBase/api/v1/clusters/$clusterID" -Headers $headers -ContentType 'application/json' -Body '{"enabled":true}' | Out-Null
        $probe = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/clusters/$clusterID/probe" -Headers $headers
        Assert-Equal $probe.status 'ready' "cluster $index probe did not become ready"
    }

    Assert-True ($clusterIDs[0] -lt $clusterIDs[1]) 'platform cluster IDs were not assigned in creation order'
    $expectedKeys = @(
        "$($clusterIDs[0])|Pod|$SearchNamespace|search-alpha-pod",
        "$($clusterIDs[0])|Deployment|$SearchNamespace|search-alpha-deployment",
        "$($clusterIDs[0])|Service|$SearchNamespace|search-alpha-service",
        "$($clusterIDs[0])|Ingress|$SearchNamespace|search-alpha-ingress",
        "$($clusterIDs[1])|Pod|$SearchNamespace|search-beta-pod",
        "$($clusterIDs[1])|Pod|$SearchNamespace|search-gamma-pod",
        "$($clusterIDs[1])|Deployment|$SearchNamespace|search-beta-deployment",
        "$($clusterIDs[1])|Service|$SearchNamespace|search-beta-service",
        "$($clusterIDs[1])|Ingress|$SearchNamespace|search-beta-ingress"
    )

    $baseline = Invoke-RestMethod -Uri "${searchBase}?q=$SearchTerm&namespace=$SearchNamespace&limit=100" -Headers $headers -TimeoutSec 15
    Assert-Equal ([int]$baseline.total) 9 'baseline total mismatch'
    Assert-Equal @($baseline.items).Count 9 'baseline item count mismatch'
    Assert-Equal ([int]$baseline.remaining) 0 'baseline remaining mismatch'
    Assert-Equal ([int]$baseline.clusters_total) 2 'baseline clusters_total mismatch'
    Assert-Equal ([int]$baseline.clusters_searched) 2 'omitted cluster_limit did not search both clusters'
    Assert-Equal ([int]$baseline.clusters_remaining) 0 'baseline clusters_remaining mismatch'
    Assert-True ([bool]$baseline.complete) 'complete baseline was marked partial'
    Assert-SequenceEqual -Actual @($baseline.kinds) -Expected $FixedKinds -Message 'fixed response kinds'
    Assert-SequenceEqual -Actual (Get-ItemKeys -Items @($baseline.items)) -Expected $expectedKeys -Message 'stable global search ordering'
    Assert-True (@($baseline.items | Where-Object { $_.kind -notin $FixedKinds }).Count -eq 0) 'search returned a kind outside the fixed catalog'
    Assert-Equal @($baseline.failures).Count 0 'baseline returned localized failures'
    Assert-Equal ([int]$baseline.limits.max_clusters) 20 'max cluster contract mismatch'
    Assert-Equal ([int]$baseline.limits.max_concurrent_clusters) 4 'cluster concurrency contract mismatch'
    Assert-Equal ([int]$baseline.limits.per_cluster_timeout_ms) 4000 'per-cluster timeout contract mismatch'
    Assert-Equal ([int]$baseline.limits.max_results) 100 'max result contract mismatch'
    Assert-Equal ([int]$baseline.limits.per_kind_limit) 100 'per-kind limit contract mismatch'

    $selectedKinds = Invoke-RestMethod -Uri "${searchBase}?q=$SearchTerm&namespace=$SearchNamespace&kinds=services,pods&limit=100" -Headers $headers -TimeoutSec 15
    Assert-SequenceEqual -Actual @($selectedKinds.kinds) -Expected @('Pod', 'Service') -Message 'selected kind canonical order'
    Assert-Equal ([int]$selectedKinds.total) 5 'selected kind total mismatch'
    Assert-True (@($selectedKinds.items | Where-Object { $_.kind -notin @('Pod', 'Service') }).Count -eq 0) 'selected kind search leaked another kind'

    $clusterLimited = Invoke-RestMethod -Uri "${searchBase}?q=$SearchTerm&namespace=$SearchNamespace&cluster_limit=1&limit=100" -Headers $headers -TimeoutSec 15
    Assert-Equal ([int]$clusterLimited.clusters_total) 2 'cluster-limited total coverage mismatch'
    Assert-Equal ([int]$clusterLimited.clusters_searched) 1 'cluster-limited searched coverage mismatch'
    Assert-Equal ([int]$clusterLimited.clusters_remaining) 1 'cluster-limited remaining coverage mismatch'
    Assert-Equal ([int]$clusterLimited.total) 4 'cluster-limited result total mismatch'
    Assert-True (-not [bool]$clusterLimited.complete) 'cluster omission was marked complete'
    Assert-True (@($clusterLimited.items | Where-Object { [int64]$_.cluster_id -ne $clusterIDs[0] }).Count -eq 0) 'cluster_limit did not retain the lowest cluster ID'

    $truncated = Invoke-RestMethod -Uri "${searchBase}?q=$SearchTerm&namespace=$SearchNamespace&limit=3" -Headers $headers -TimeoutSec 15
    Assert-Equal ([int]$truncated.total) 9 'truncated search lost the known total'
    Assert-Equal @($truncated.items).Count 3 'global result limit was not applied'
    Assert-Equal ([int]$truncated.remaining) 6 'global remaining count mismatch'
    Assert-True (-not [bool]$truncated.complete) 'truncated search was marked complete'
    Assert-SequenceEqual -Actual (Get-ItemKeys -Items @($truncated.items)) -Expected $expectedKeys[0..2] -Message 'truncated stable prefix'

    $secondControlPlane = "$($ClusterNames[1])-control-plane"
    Invoke-NativeText -File 'docker' -Arguments @('pause', $secondControlPlane) | Out-Null
    $secondControlPlanePaused = $true
    $timedOut = Invoke-RestMethod -Uri "${searchBase}?q=$SearchTerm&namespace=$SearchNamespace&limit=100" -Headers $headers -TimeoutSec 15
    Assert-HealthyPeer -Response $timedOut -ClusterID $clusterIDs[0] -ExpectedCount 4 -Message 'healthy peer during timeout'
    $timeoutFailures = @($timedOut.failures | Where-Object { [int64]$_.cluster_id -eq $clusterIDs[1] })
    Assert-Equal $timeoutFailures.Count 4 'paused cluster did not return four failures'
    Assert-SequenceEqual -Actual @($timeoutFailures.kind) -Expected $FixedKinds -Message 'timeout failure kind order'
    Assert-True (@($timeoutFailures | Where-Object { $_.code -ne 'TIMEOUT' }).Count -eq 0) 'paused cluster exposed a non-timeout failure'
    Assert-Equal ([int]$timedOut.total) 4 'timeout response known total mismatch'
    Assert-True (-not [bool]$timedOut.complete) 'timeout response was marked complete'

    Invoke-NativeText -File 'docker' -Arguments @('unpause', $secondControlPlane) | Out-Null
    $secondControlPlanePaused = $false
    Invoke-KubectlText -Kubeconfig $Kubeconfigs[1] -Arguments @('wait', '--for=condition=Ready', 'node', '--all', '--timeout=60s') | Out-Null
    $recovered = Invoke-RestMethod -Uri "${searchBase}?q=$SearchTerm&namespace=$SearchNamespace&limit=100" -Headers $headers -TimeoutSec 15
    Assert-Equal ([int]$recovered.total) 9 'recovery did not restore all results'
    Assert-Equal @($recovered.failures).Count 0 'recovered search retained failures'
    Assert-True ([bool]$recovered.complete) 'recovered search was not complete'
    Assert-SequenceEqual -Actual (Get-ItemKeys -Items @($recovered.items)) -Expected $expectedKeys -Message 'recovered stable ordering'

    Invoke-NativeText -File 'docker' -Arguments @('stop', '--time', '1', $secondControlPlane) | Out-Null
    $unavailable = Invoke-RestMethod -Uri "${searchBase}?q=$SearchTerm&namespace=$SearchNamespace&limit=100" -Headers $headers -TimeoutSec 15
    Assert-HealthyPeer -Response $unavailable -ClusterID $clusterIDs[0] -ExpectedCount 4 -Message 'healthy peer during outage'
    $queryFailures = @($unavailable.failures | Where-Object { [int64]$_.cluster_id -eq $clusterIDs[1] })
    Assert-Equal $queryFailures.Count 4 'stopped cluster did not return four failures'
    Assert-SequenceEqual -Actual @($queryFailures.kind) -Expected $FixedKinds -Message 'query failure kind order'
    Assert-True (@($queryFailures | Where-Object { $_.code -ne 'QUERY_FAILED' }).Count -eq 0) 'stopped cluster exposed an unexpected failure code'
    Assert-Equal ([int]$unavailable.total) 4 'outage response known total mismatch'
    Assert-True (-not [bool]$unavailable.complete) 'outage response was marked complete'

    $summary = [ordered]@{
        verified_at = (Get-Date).ToString('o')
        kind_version = Invoke-NativeText -File $Kind -Arguments @('version')
        kubernetes_version = 'v1.34.0'
        cluster_count = 2
        cluster_ids_sorted = @($clusterIDs)
        authorization = [ordered]@{ anonymous_status = $anonymousStatus; invalid_statuses = $invalidStatuses }
        rbac = $rbac
        baseline = [ordered]@{
            total = $baseline.total
            returned = @($baseline.items).Count
            stable_order = $true
            fixed_kinds = @($baseline.kinds)
            coverage = [ordered]@{ total = $baseline.clusters_total; searched = $baseline.clusters_searched; remaining = $baseline.clusters_remaining }
            complete = $baseline.complete
            limits = $baseline.limits
        }
        kind_selection = [ordered]@{ kinds = @($selectedKinds.kinds); total = $selectedKinds.total }
        cluster_limit_one = [ordered]@{ total = $clusterLimited.clusters_total; searched = $clusterLimited.clusters_searched; remaining = $clusterLimited.clusters_remaining; lowest_id_first = $true }
        result_limit_three = [ordered]@{ total = $truncated.total; returned = @($truncated.items).Count; remaining = $truncated.remaining; stable_prefix = $true }
        timeout_isolation = [ordered]@{ survivor_results = 4; failed_cluster_codes = @($timeoutFailures.code); complete = $timedOut.complete }
        recovery = [ordered]@{ total = $recovered.total; failures = @($recovered.failures).Count; complete = $recovered.complete }
        query_failure_isolation = [ordered]@{ survivor_results = 4; failed_cluster_codes = @($queryFailures.code); complete = $unavailable.complete }
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
$path = Join-Path $ArtifactDirectory ("search-e2e-{0}.json" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
Write-Utf8File -Path $path -Contents ($summary | ConvertTo-Json -Depth 12)
Write-Host "Disposable two-cluster global search verification passed. Evidence: $path"
$summary | ConvertTo-Json -Depth 12
