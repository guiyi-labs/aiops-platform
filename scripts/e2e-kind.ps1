[CmdletBinding()]
param(
    [string]$ApiBase = 'http://127.0.0.1:8080',
    [string]$Username = 'admin',
    [string]$AdminPassword = $env:AIOPS_ADMIN_PASSWORD,
    [string]$Context = 'kind-aiops-test',
    [switch]$RequireMetrics,
    [switch]$KeepPlatformCluster,
    [switch]$CleanupDemoResources
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Root = Split-Path -Parent $PSScriptRoot
$ArtifactDirectory = Join-Path $Root '.artifacts\e2e-kind'
[IO.Directory]::CreateDirectory($ArtifactDirectory) | Out-Null

if ($KeepPlatformCluster -and $CleanupDemoResources) {
    throw 'KeepPlatformCluster and CleanupDemoResources cannot be used together'
}

function Invoke-KubectlText {
    param(
        [Parameter(Mandatory, Position = 0)] [string[]]$Arguments,
        [switch]$AllowDenied
    )
    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $output = & kubectl --context $Context @Arguments 2>&1
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorAction
    }
    if ($exitCode -ne 0 -and -not ($AllowDenied -and $exitCode -eq 1)) {
        throw "kubectl $($Arguments -join ' ') failed: $($output -join "`n")"
    }
    return (($output | ForEach-Object { $_.ToString() }) -join "`n").Trim()
}

function Invoke-KubectlInput {
    param(
        [Parameter(Mandatory)] [string]$Body,
        [Parameter(Mandatory)] [string[]]$Arguments
    )
    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $output = $Body | & kubectl --context $Context @Arguments 2>&1
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorAction
    }
    if ($exitCode -ne 0) {
        throw "kubectl $($Arguments -join ' ') failed: $($output -join "`n")"
    }
    return (($output | ForEach-Object { $_.ToString() }) -join "`n").Trim()
}

function Invoke-KubectlPatch {
    param(
        [Parameter(Mandatory)] [string]$Body,
        [Parameter(Mandatory)] [string[]]$Arguments
    )
    $patchPath = [IO.Path]::GetTempFileName()
    try {
        [IO.File]::WriteAllText($patchPath, $Body, [Text.UTF8Encoding]::new($false))
        return Invoke-KubectlText -Arguments ($Arguments + @("--patch-file=$patchPath"))
    } finally {
        Remove-Item -LiteralPath $patchPath -Force -ErrorAction SilentlyContinue
    }
}

function Assert-Equal {
    param($Actual, $Expected, [Parameter(Mandatory)] [string]$Message)
    if ($Actual -ne $Expected) {
        throw "$Message; expected $Expected, got $Actual"
    }
}

function Get-WaitingReason {
    param($Pod)
    if ($null -eq $Pod -or $null -eq $Pod.status) {
        return ''
    }
    $containerStatuses = $Pod.status.PSObject.Properties['containerStatuses']
    if ($null -eq $containerStatuses) {
        return ''
    }
    $statuses = @($containerStatuses.Value)
    if ($statuses.Count -eq 0 -or $null -eq $statuses[0].state) {
        return ''
    }
    $waiting = $statuses[0].state.PSObject.Properties['waiting']
    if ($null -eq $waiting -or $null -eq $waiting.Value) {
        return ''
    }
    return [string]$waiting.Value.reason
}

function Test-PodReady {
    param($Pod)
    if ($null -eq $Pod -or $null -eq $Pod.status) {
        return $false
    }
    $containerStatuses = $Pod.status.PSObject.Properties['containerStatuses']
    if ($null -eq $containerStatuses) {
        return $false
    }
    $statuses = @($containerStatuses.Value)
    return $statuses.Count -gt 0 -and [bool]$statuses[0].ready
}

function Wait-DemoState {
    $deadline = (Get-Date).AddMinutes(3)
    do {
        $pods = Invoke-KubectlText @('-n', 'aiops-demo', 'get', 'pods', '-o', 'json') | ConvertFrom-Json
        $imagePod = $pods.items | Where-Object { $_.metadata.labels.'app.kubernetes.io/name' -eq 'image-pull-backoff' } | Select-Object -First 1
        $crashPod = $pods.items | Where-Object { $_.metadata.labels.'app.kubernetes.io/name' -eq 'crash-loop-backoff' } | Select-Object -First 1
        $healthy = @($pods.items | Where-Object {
            $_.metadata.labels.'app.kubernetes.io/name' -in @('healthy-nginx', 'service-backend') -and
            (Test-PodReady $_)
        }).Count
        $imageReason = Get-WaitingReason $imagePod
        $crashReason = Get-WaitingReason $crashPod
        if ($healthy -eq 2 -and $imageReason -in @('ErrImagePull', 'ImagePullBackOff') -and $crashReason -eq 'CrashLoopBackOff') {
            return [ordered]@{
                image_pod = $imagePod.metadata.name
                image_reason = $imageReason
                crash_pod = $crashPod.metadata.name
                crash_reason = $crashReason
                healthy_workloads = $healthy
            }
        }
        Start-Sleep -Seconds 3
    } while ((Get-Date) -lt $deadline)
    throw "demo pods did not reach the expected states; image=$imageReason crash=$crashReason healthy=$healthy"
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

$clusterID = 0
$accessToken = ''
$cleanupSucceeded = $false
$failure = $null
$summary = $null
$clusterName = ''
$metricsSummary = $null
$syntheticNodeCreated = $false
$deploymentOriginalReplicas = $null
$cronJobOriginalSuspend = $null

try {
    Invoke-RestMethod "$ApiBase/api/v1/health/ready" -TimeoutSec 10 | Out-Null
    $current = Invoke-KubectlText @('config', 'current-context')
    Assert-Equal $current $Context 'unexpected kubectl context'

    Invoke-KubectlText @('apply', '-f', (Join-Path $Root 'deploy\demo-scenarios\namespace.yaml')) | Write-Host
    Invoke-KubectlText @('apply', '--dry-run=server', '-k', (Join-Path $Root 'deploy\demo-scenarios')) | Write-Host
    Invoke-KubectlText @('apply', '-k', (Join-Path $Root 'deploy\demo-scenarios')) | Write-Host
    Invoke-KubectlText @('apply', '--dry-run=server', '-k', (Join-Path $Root 'deploy\managed-cluster')) | Write-Host
    Invoke-KubectlText @('apply', '-k', (Join-Path $Root 'deploy\managed-cluster')) | Write-Host
    Invoke-KubectlText @('-n', 'aiops-demo', 'rollout', 'status', 'deployment/healthy-nginx', '--timeout=120s') | Write-Host
    Invoke-KubectlText @('-n', 'aiops-demo', 'rollout', 'status', 'deployment/service-without-endpoints', '--timeout=120s') | Write-Host
    $demo = Wait-DemoState

    # Credential is intentionally short-lived: kubectl create token ... --duration=1h
    $token = Invoke-KubectlText @('-n', 'kube-system', 'create', 'token', 'aiops-platform', '--duration=1h')
    $rawContext = Invoke-KubectlText @('config', 'view', '--raw', '--minify', '-o', 'json') | ConvertFrom-Json
    $server = $rawContext.clusters[0].cluster.server
    $ca = $rawContext.clusters[0].cluster.'certificate-authority-data'
    if ([string]::IsNullOrWhiteSpace($server) -or [string]::IsNullOrWhiteSpace($ca)) {
        throw 'current context does not contain an embedded API server and CA'
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
  - name: e2e
    cluster:
      server: $server
$tlsServerNameLine      certificate-authority-data: $ca
contexts:
  - name: e2e
    context:
      cluster: e2e
      user: aiops-platform
current-context: e2e
users:
  - name: aiops-platform
    user:
      token: $token
"@

    $login = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/auth/login" -ContentType 'application/json' -Body (@{
        username = $Username
        password = $AdminPassword
    } | ConvertTo-Json)
    $accessToken = $login.access_token
    $headers = @{ Authorization = "Bearer $accessToken" }

    $clusterNamePrefix = if ($KeepPlatformCluster) { 'demo-kind' } else { 'e2e-kind' }
    $clusterName = "$clusterNamePrefix-$((Get-Date).ToString('yyyyMMdd-HHmmss'))"
    $cluster = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/clusters" -Headers $headers -ContentType 'application/json' -Body (@{
        name = $clusterName
        kubeconfig = $kubeconfig
    } | ConvertTo-Json)
    $clusterID = [int64]$cluster.id
    Invoke-WebRequest -UseBasicParsing -Method Patch -Uri "$ApiBase/api/v1/clusters/$clusterID" -Headers $headers -ContentType 'application/json' -Body '{"enabled":true}' | Out-Null
    $probe = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/clusters/$clusterID/probe" -Headers $headers
    Assert-Equal $probe.status 'ready' 'cluster probe did not become ready'

    $resourceBase = "$ApiBase/api/v1/clusters/$clusterID"
    $pods = Invoke-RestMethod -Uri "$resourceBase/pods?namespace=aiops-demo&limit=50" -Headers $headers
    $services = Invoke-RestMethod -Uri "$resourceBase/services?namespace=aiops-demo&limit=50" -Headers $headers
    if ($pods.total -lt 4 -or $services.total -lt 2) {
        throw "resource read returned too few objects; pods=$($pods.total) services=$($services.total)"
    }

    $m17Fixtures = [ordered]@{
        statefulsets = 'demo-stateful'
        daemonsets = 'demo-node-agent'
        replicasets = 'demo-replica'
        jobs = 'demo-backup'
        cronjobs = 'demo-cleanup'
        horizontalpodautoscalers = 'healthy-nginx'
        resourcequotas = 'demo-budget'
        limitranges = 'demo-storage-bounds'
        secrets = 'demo-key-catalog'
    }
    $m17Counts = [ordered]@{}
    foreach ($resourceName in $m17Fixtures.Keys) {
        $fixtureName = $m17Fixtures[$resourceName]
        $list = Invoke-RestMethod -Uri "$resourceBase/$resourceName`?namespace=aiops-demo&limit=100" -Headers $headers
        if ($list.total -lt 1 -or @($list.items | Where-Object { $_.metadata.name -eq $fixtureName }).Count -ne 1) {
            throw "M17 $resourceName list did not contain $fixtureName"
        }
        $detail = Invoke-RestMethod -Uri "$resourceBase/$resourceName/aiops-demo/$fixtureName" -Headers $headers
        Assert-Equal $detail.metadata.name $fixtureName "M17 $resourceName detail mismatch"
        $m17Counts[$resourceName] = $list.total
    }
    $secret = Invoke-RestMethod -Uri "$resourceBase/secrets/aiops-demo/demo-key-catalog" -Headers $headers
    if (@($secret.dataKeys).Count -ne 1 -or $secret.dataKeys[0] -ne 'example-key' -or
        $null -ne $secret.PSObject.Properties['data'] -or $null -ne $secret.metadata.PSObject.Properties['annotations']) {
        throw 'Secret public contract exposed values/annotations or lost key names'
    }

    if ($RequireMetrics) {
        $nodeMetrics = Invoke-RestMethod -Uri "$resourceBase/metrics/nodes?limit=100" -Headers $headers
        $podMetrics = Invoke-RestMethod -Uri "$resourceBase/metrics/pods?limit=100" -Headers $headers
        $nodeItems = @($nodeMetrics.items)
        $podItems = @($podMetrics.items)
        if ($nodeItems.Count -lt 1 -or $podItems.Count -lt 1) {
            throw "Metrics API returned too few samples; nodes=$($nodeItems.Count) pods=$($podItems.Count)"
        }
        $nodeSample = $nodeItems | Select-Object -First 1
        $podSample = $podItems | Where-Object { @($_.containers).Count -gt 0 } | Select-Object -First 1
        if ([string]::IsNullOrWhiteSpace([string]$nodeSample.usage.cpu) -or
            [string]::IsNullOrWhiteSpace([string]$nodeSample.usage.memory) -or
            $null -eq $podSample -or
            [string]::IsNullOrWhiteSpace([string]$podSample.containers[0].usage.cpu) -or
            [string]::IsNullOrWhiteSpace([string]$podSample.containers[0].usage.memory)) {
            throw 'Metrics API samples do not contain real CPU and memory quantities'
        }
        $metricsSummary = [ordered]@{
            required = $true
            node_samples = $nodeItems.Count
            pod_samples = $podItems.Count
            node_total = $nodeMetrics.total
            pod_total = $podMetrics.total
            node_cpu = $nodeSample.usage.cpu
            node_memory = $nodeSample.usage.memory
            pod_cpu = $podSample.containers[0].usage.cpu
            pod_memory = $podSample.containers[0].usage.memory
        }
    }

    function New-Diagnosis([string]$Kind, [string]$Name) {
        return Invoke-RestMethod -Method Post -Uri "$resourceBase/diagnoses" -Headers $headers -ContentType 'application/json' -Body (@{
            resource_kind = $Kind
            namespace = 'aiops-demo'
            name = $Name
        } | ConvertTo-Json)
    }

    $imageDiagnosis = New-Diagnosis 'Pod' $demo.image_pod
    $crashDiagnosis = New-Diagnosis 'Pod' $demo.crash_pod
    $serviceDiagnosis = New-Diagnosis 'Service' 'service-without-endpoints'

    Invoke-KubectlText -Arguments @('-n', 'aiops-demo', 'delete', 'event', 'm18-pvc-provisioning-failed', '--ignore-not-found=true') | Out-Null
    $pvcFixture = Invoke-KubectlText @('-n', 'aiops-demo', 'get', 'pvc', 'm18-pending-pvc', '-o', 'json') | ConvertFrom-Json
    $eventTimestamp = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
    $eventBody = [ordered]@{
        apiVersion = 'v1'
        kind = 'Event'
        metadata = [ordered]@{ name = 'm18-pvc-provisioning-failed'; namespace = 'aiops-demo' }
        involvedObject = [ordered]@{
            apiVersion = 'v1'
            kind = 'PersistentVolumeClaim'
            namespace = 'aiops-demo'
            name = 'm18-pending-pvc'
            uid = $pvcFixture.metadata.uid
            resourceVersion = $pvcFixture.metadata.resourceVersion
        }
        reason = 'ProvisioningFailed'
        message = 'deterministic M18 fixture: requested StorageClass does not exist'
        source = [ordered]@{ component = 'aiops-e2e' }
        firstTimestamp = $eventTimestamp
        lastTimestamp = $eventTimestamp
        count = 1
        type = 'Warning'
    } | ConvertTo-Json -Depth 8
    Invoke-KubectlInput -Body $eventBody -Arguments @('create', '-f', '-') | Out-Null
    $pvcDiagnosis = New-Diagnosis 'PersistentVolumeClaim' 'm18-pending-pvc'

    $ingressDiagnosis = New-Diagnosis 'Ingress' 'm18-broken-ingress'

    $hpaStatus = '{"status":{"currentReplicas":1,"desiredReplicas":1,"conditions":[{"type":"ScalingLimited","status":"True","reason":"TooManyReplicas","message":"deterministic M18 fixture reached maxReplicas","lastTransitionTime":"2026-07-27T08:00:00Z"}]}}'
    Invoke-KubectlPatch -Body $hpaStatus -Arguments @('-n', 'aiops-demo', 'patch', 'horizontalpodautoscaler', 'm18-saturated-hpa', '--subresource=status', '--type=merge') | Out-Null
    $hpaDiagnosis = New-Diagnosis 'HorizontalPodAutoscaler' 'm18-saturated-hpa'

    Invoke-KubectlText @('apply', '-f', (Join-Path $Root 'deploy\demo-scenarios\m18-pressure-node.yaml')) | Out-Null
    $syntheticNodeCreated = $true
    $nodeStatus = '{"status":{"conditions":[{"type":"Ready","status":"True","reason":"KubeletReady","message":"deterministic M18 fixture is Ready","lastTransitionTime":"2026-07-27T08:00:00Z"},{"type":"MemoryPressure","status":"True","reason":"KubeletHasInsufficientMemory","message":"deterministic M18 fixture reports memory pressure","lastTransitionTime":"2026-07-27T08:00:00Z"}]}}'
    Invoke-KubectlPatch -Body $nodeStatus -Arguments @('patch', 'node', 'm18-pressure-node', '--subresource=status', '--type=merge') | Out-Null
    $nodeDiagnosis = New-Diagnosis 'Node' 'm18-pressure-node'

    Assert-Equal $imageDiagnosis.rule_id 'pod.image_pull_backoff.v1' 'image diagnosis rule mismatch'
    Assert-Equal $crashDiagnosis.rule_id 'pod.crash_loop_backoff.v1' 'crash diagnosis rule mismatch'
    Assert-Equal $serviceDiagnosis.rule_id 'service.no_ready_endpoints.v1' 'service diagnosis rule mismatch'
    Assert-Equal $pvcDiagnosis.rule_id 'persistentvolumeclaim.pending.v1' 'PVC diagnosis rule mismatch'
    Assert-Equal $ingressDiagnosis.rule_id 'ingress.backend_unavailable.v1' 'Ingress diagnosis rule mismatch'
    Assert-Equal $hpaDiagnosis.rule_id 'horizontalpodautoscaler.saturated.v1' 'HPA diagnosis rule mismatch'
    Assert-Equal $nodeDiagnosis.rule_id 'node.pressure.v1' 'Node pressure diagnosis rule mismatch'

    $confirmed = Invoke-RestMethod -Method Patch -Uri "$ApiBase/api/v1/diagnoses/$($imageDiagnosis.id)" -Headers $headers -ContentType 'application/json' -Body (@{
        status = 'confirmed'
        comment = 'Automated kind delivery verification'
    } | ConvertTo-Json)
    Assert-Equal $confirmed.status 'confirmed' 'diagnosis confirmation failed'

    $preview = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/diagnoses/$($imageDiagnosis.id)/remediations/preview" -Headers $headers -ContentType 'application/json' -Body (@{
        action = 'deployment.rollout_restart'
        target_name = 'image-pull-backoff'
    } | ConvertTo-Json)
    $idempotencyKey = "e2e-kind-$((Get-Date).ToString('yyyyMMddHHmmss'))"
    $executeHeaders = @{ Authorization = "Bearer $accessToken"; 'Idempotency-Key' = $idempotencyKey }
    $executionBody = @{ confirmation_token = $preview.confirmation_token } | ConvertTo-Json
    $executed = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/remediations/$($preview.id)/execute" -Headers $executeHeaders -ContentType 'application/json' -Body $executionBody
    $replayed = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/remediations/$($preview.id)/execute" -Headers $executeHeaders -ContentType 'application/json' -Body $executionBody
    Assert-Equal $executed.status 'succeeded' 'remediation execution failed'
    Assert-Equal $replayed.id $executed.id 'idempotent replay returned another plan'

    $deployment = Invoke-KubectlText @('-n', 'aiops-demo', 'get', 'deployment', 'image-pull-backoff', '-o', 'json') | ConvertFrom-Json
    Assert-Equal $deployment.spec.template.metadata.annotations.'k8s-aiops.local/remediation-id' $preview.id 'remediation annotation is missing'

    $scaleTarget = Invoke-KubectlText @('-n', 'aiops-demo', 'get', 'deployment', 'healthy-nginx', '-o', 'json') | ConvertFrom-Json
    $deploymentOriginalReplicas = [int]$scaleTarget.spec.replicas
    $scaleDesired = if ($deploymentOriginalReplicas -eq 1) { 2 } else { 1 }
    $scalePreview = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/clusters/$clusterID/operations/preview" -Headers $headers -ContentType 'application/json' -Body (@{
        action = 'deployment.scale'
        namespace = 'aiops-demo'
        target_name = 'healthy-nginx'
        desired_replicas = $scaleDesired
    } | ConvertTo-Json)
    Assert-Equal $scalePreview.change.before $deploymentOriginalReplicas 'scale preview before value mismatch'
    Assert-Equal $scalePreview.change.after $scaleDesired 'scale preview after value mismatch'
    $scaleHeaders = @{ Authorization = "Bearer $accessToken"; 'Idempotency-Key' = "e2e-kind-scale-$((Get-Date).ToString('yyyyMMddHHmmss'))" }
    $scaleBody = @{ confirmation_token = $scalePreview.confirmation_token } | ConvertTo-Json
    $scaleExecuted = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/remediations/$($scalePreview.id)/execute" -Headers $scaleHeaders -ContentType 'application/json' -Body $scaleBody
    $scaleReplayed = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/remediations/$($scalePreview.id)/execute" -Headers $scaleHeaders -ContentType 'application/json' -Body $scaleBody
    Assert-Equal $scaleExecuted.status 'succeeded' 'Deployment scale execution failed'
    Assert-Equal $scaleReplayed.id $scaleExecuted.id 'Deployment scale replay returned another plan'
    $scaledDeployment = Invoke-KubectlText @('-n', 'aiops-demo', 'get', 'deployment', 'healthy-nginx', '-o', 'json') | ConvertFrom-Json
    Assert-Equal $scaledDeployment.spec.replicas $scaleDesired 'Deployment scale state mismatch'

    $cronTarget = Invoke-KubectlText @('-n', 'aiops-demo', 'get', 'cronjob', 'demo-cleanup', '-o', 'json') | ConvertFrom-Json
    $cronJobOriginalSuspend = [bool]$cronTarget.spec.suspend
    Assert-Equal $cronJobOriginalSuspend $true 'CronJob fixture must start suspended'
    $resumePreview = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/clusters/$clusterID/operations/preview" -Headers $headers -ContentType 'application/json' -Body (@{
        action = 'cronjob.resume'
        namespace = 'aiops-demo'
        target_name = 'demo-cleanup'
    } | ConvertTo-Json)
    $resumeHeaders = @{ Authorization = "Bearer $accessToken"; 'Idempotency-Key' = "e2e-kind-resume-$((Get-Date).ToString('yyyyMMddHHmmss'))" }
    $resumeExecuted = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/remediations/$($resumePreview.id)/execute" -Headers $resumeHeaders -ContentType 'application/json' -Body (@{ confirmation_token = $resumePreview.confirmation_token } | ConvertTo-Json)
    Assert-Equal $resumeExecuted.status 'succeeded' 'CronJob resume execution failed'
    $resumedCronJob = Invoke-KubectlText @('-n', 'aiops-demo', 'get', 'cronjob', 'demo-cleanup', '-o', 'json') | ConvertFrom-Json
    Assert-Equal ([bool]$resumedCronJob.spec.suspend) $false 'CronJob resume state mismatch'

    $suspendPreview = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/clusters/$clusterID/operations/preview" -Headers $headers -ContentType 'application/json' -Body (@{
        action = 'cronjob.suspend'
        namespace = 'aiops-demo'
        target_name = 'demo-cleanup'
    } | ConvertTo-Json)
    $suspendHeaders = @{ Authorization = "Bearer $accessToken"; 'Idempotency-Key' = "e2e-kind-suspend-$((Get-Date).ToString('yyyyMMddHHmmss'))" }
    $suspendExecuted = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/remediations/$($suspendPreview.id)/execute" -Headers $suspendHeaders -ContentType 'application/json' -Body (@{ confirmation_token = $suspendPreview.confirmation_token } | ConvertTo-Json)
    Assert-Equal $suspendExecuted.status 'succeeded' 'CronJob suspend execution failed'
    $suspendedCronJob = Invoke-KubectlText @('-n', 'aiops-demo', 'get', 'cronjob', 'demo-cleanup', '-o', 'json') | ConvertFrom-Json
    Assert-Equal ([bool]$suspendedCronJob.spec.suspend) $cronJobOriginalSuspend 'CronJob fixture was not restored'

    $restoreScalePreview = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/clusters/$clusterID/operations/preview" -Headers $headers -ContentType 'application/json' -Body (@{
        action = 'deployment.scale'
        namespace = 'aiops-demo'
        target_name = 'healthy-nginx'
        desired_replicas = $deploymentOriginalReplicas
    } | ConvertTo-Json)
    $restoreScaleHeaders = @{ Authorization = "Bearer $accessToken"; 'Idempotency-Key' = "e2e-kind-scale-restore-$((Get-Date).ToString('yyyyMMddHHmmss'))" }
    $restoreScaleExecuted = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/remediations/$($restoreScalePreview.id)/execute" -Headers $restoreScaleHeaders -ContentType 'application/json' -Body (@{ confirmation_token = $restoreScalePreview.confirmation_token } | ConvertTo-Json)
    Assert-Equal $restoreScaleExecuted.status 'succeeded' 'Deployment scale restore failed'
    $restoredDeployment = Invoke-KubectlText @('-n', 'aiops-demo', 'get', 'deployment', 'healthy-nginx', '-o', 'json') | ConvertFrom-Json
    Assert-Equal $restoredDeployment.spec.replicas $deploymentOriginalReplicas 'Deployment fixture was not restored'

    $actor = 'system:serviceaccount:kube-system:aiops-platform'
    $canRead = Invoke-KubectlText -Arguments @('auth', 'can-i', 'list', 'pods', '--all-namespaces', "--as=$actor") -AllowDenied
    $canPatchDemo = Invoke-KubectlText -Arguments @('auth', 'can-i', 'patch', 'deployments', '-n', 'aiops-demo', "--as=$actor") -AllowDenied
    $canDeletePod = Invoke-KubectlText -Arguments @('auth', 'can-i', 'delete', 'pods', '-n', 'aiops-demo', "--as=$actor") -AllowDenied
    $canPatchSystem = Invoke-KubectlText -Arguments @('auth', 'can-i', 'patch', 'deployments', '-n', 'kube-system', "--as=$actor") -AllowDenied
    $canPatchCronJob = Invoke-KubectlText -Arguments @('auth', 'can-i', 'patch', 'cronjobs.batch', '-n', 'aiops-demo', "--as=$actor") -AllowDenied
    $canPatchSystemCronJob = Invoke-KubectlText -Arguments @('auth', 'can-i', 'patch', 'cronjobs.batch', '-n', 'kube-system', "--as=$actor") -AllowDenied
    $canListSecrets = Invoke-KubectlText -Arguments @('auth', 'can-i', 'list', 'secrets', '-n', 'aiops-demo', "--as=$actor") -AllowDenied
    $canCreateSecrets = Invoke-KubectlText -Arguments @('auth', 'can-i', 'create', 'secrets', '-n', 'aiops-demo', "--as=$actor") -AllowDenied
    $canListHPA = Invoke-KubectlText -Arguments @('auth', 'can-i', 'list', 'horizontalpodautoscalers.autoscaling', '-n', 'aiops-demo', "--as=$actor") -AllowDenied
    Assert-Equal $canRead 'yes' 'observer read permission is missing'
    Assert-Equal $canPatchDemo 'yes' 'namespaced remediation permission is missing'
    Assert-Equal $canDeletePod 'no' 'service account unexpectedly can delete Pods'
    Assert-Equal $canPatchSystem 'no' 'service account unexpectedly can patch kube-system'
    Assert-Equal $canPatchCronJob 'yes' 'namespaced CronJob operation permission is missing'
    Assert-Equal $canPatchSystemCronJob 'no' 'service account unexpectedly can patch kube-system CronJobs'
    Assert-Equal $canListSecrets 'yes' 'observer Secret metadata read permission is missing'
    Assert-Equal $canCreateSecrets 'no' 'observer unexpectedly can create Secrets'
    Assert-Equal $canListHPA 'yes' 'observer HPA read permission is missing'

    $summary = [ordered]@{
        verified_at = (Get-Date).ToString('o')
        context = $Context
        kubernetes_version = $probe.kubernetes_version
        cluster_status = $probe.status
        resource_counts = [ordered]@{ pods = $pods.total; services = $services.total; m17 = $m17Counts }
        metrics = $metricsSummary
        demo_state = $demo
        diagnoses = @(
            [ordered]@{ id = $imageDiagnosis.id; rule_id = $imageDiagnosis.rule_id },
            [ordered]@{ id = $crashDiagnosis.id; rule_id = $crashDiagnosis.rule_id },
            [ordered]@{ id = $serviceDiagnosis.id; rule_id = $serviceDiagnosis.rule_id }
            [ordered]@{ id = $pvcDiagnosis.id; rule_id = $pvcDiagnosis.rule_id }
            [ordered]@{ id = $ingressDiagnosis.id; rule_id = $ingressDiagnosis.rule_id }
            [ordered]@{ id = $hpaDiagnosis.id; rule_id = $hpaDiagnosis.rule_id }
            [ordered]@{ id = $nodeDiagnosis.id; rule_id = $nodeDiagnosis.rule_id }
        )
        remediation = [ordered]@{ id = $executed.id; status = $executed.status; replay_same_plan = ($replayed.id -eq $executed.id) }
        controlled_operations = [ordered]@{
            deployment_scale = [ordered]@{ id = $scaleExecuted.id; status = $scaleExecuted.status; replay_same_plan = ($scaleReplayed.id -eq $scaleExecuted.id); restored_replicas = $restoredDeployment.spec.replicas }
            cronjob_resume = [ordered]@{ id = $resumeExecuted.id; status = $resumeExecuted.status }
            cronjob_suspend = [ordered]@{ id = $suspendExecuted.id; status = $suspendExecuted.status; restored_suspend = [bool]$suspendedCronJob.spec.suspend }
        }
        rbac = [ordered]@{ list_pods = $canRead; patch_demo_deployments = $canPatchDemo; delete_demo_pods = $canDeletePod; patch_system_deployments = $canPatchSystem; patch_demo_cronjobs = $canPatchCronJob; patch_system_cronjobs = $canPatchSystemCronJob; list_secrets = $canListSecrets; create_secrets = $canCreateSecrets; list_hpa = $canListHPA }
    }
} catch {
    $failure = $_
} finally {
    if ($null -ne $deploymentOriginalReplicas) {
        try {
            Invoke-KubectlText @('-n', 'aiops-demo', 'scale', 'deployment', 'healthy-nginx', "--replicas=$deploymentOriginalReplicas") | Out-Null
        } catch {
            if ($null -eq $failure) { $failure = $_ } else { Write-Warning "Deployment fixture restore failed: $($_.Exception.Message)" }
        }
    }
    if ($null -ne $cronJobOriginalSuspend) {
        try {
            $restoreCronPatch = @{ spec = @{ suspend = [bool]$cronJobOriginalSuspend } } | ConvertTo-Json -Compress
            Invoke-KubectlPatch -Body $restoreCronPatch -Arguments @('-n', 'aiops-demo', 'patch', 'cronjob', 'demo-cleanup', '--type=merge') | Out-Null
        } catch {
            if ($null -eq $failure) { $failure = $_ } else { Write-Warning "CronJob fixture restore failed: $($_.Exception.Message)" }
        }
    }
    if ($syntheticNodeCreated) {
        try {
            Invoke-KubectlText @('delete', 'node', 'm18-pressure-node', '--ignore-not-found=true') | Out-Null
        } catch {
            if ($null -eq $failure) {
                $failure = $_
            } else {
                Write-Warning "synthetic Node cleanup failed: $($_.Exception.Message)"
            }
        }
    }
    $retainSuccessfulDemo = $KeepPlatformCluster -and $null -eq $failure
    if (-not $retainSuccessfulDemo -and $clusterID -gt 0 -and -not [string]::IsNullOrWhiteSpace($accessToken)) {
        try {
            Invoke-WebRequest -UseBasicParsing -Method Delete -Uri "$ApiBase/api/v1/clusters/$clusterID" -Headers @{ Authorization = "Bearer $accessToken" } | Out-Null
            $cleanupSucceeded = $true
        } catch {
            if ($null -eq $failure) {
                $failure = $_
            } else {
                Write-Warning "platform cluster cleanup failed: $($_.Exception.Message)"
            }
        }
    }
    if ($CleanupDemoResources) {
        try {
            Invoke-KubectlText @('delete', '-k', (Join-Path $Root 'deploy\managed-cluster')) | Write-Host
            Invoke-KubectlText @('delete', '-f', (Join-Path $Root 'deploy\demo-scenarios\namespace.yaml')) | Write-Host
        } catch {
            Write-Warning "demo resource cleanup failed: $($_.Exception.Message)"
        }
    }
}

if ($null -ne $failure) {
    throw $failure
}

$summary.platform_cluster_deleted = $cleanupSucceeded
$summary.platform_cluster_id = if ($KeepPlatformCluster) { $clusterID } else { $null }
$summary.platform_cluster_name = if ($KeepPlatformCluster) { $clusterName } else { $null }
$summary.mode = if ($KeepPlatformCluster) { 'demo-retained' } else { 'ephemeral-e2e' }
$path = Join-Path $ArtifactDirectory ("e2e-kind-{0}.json" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
[IO.File]::WriteAllText($path, ($summary | ConvertTo-Json -Depth 12), [Text.UTF8Encoding]::new($false))
if ($KeepPlatformCluster) {
    Write-Host "Real kind demo environment is ready and retained. Evidence: $path"
} else {
    Write-Host "Real kind end-to-end verification passed. Evidence: $path"
}
$summary | ConvertTo-Json -Depth 12
