<#
.SYNOPSIS
Runs the M23 safe deployment release lifecycle acceptance against a disposable real kind cluster.

.DESCRIPTION
Requires Docker, kind, kubectl, the repository Compose backend with M23 routes, and registry
access for the pinned workload image. The run normally takes 4-8 minutes because it waits for
Deployment rollout, ReplicaSet revision propagation, and the controlled-operation confirmation
lifecycle.

The script creates a uniquely named kind cluster and platform registration, never reuses
aiops-test, and removes only resources bearing this run's identifier in finally. It writes a
redacted success summary under .artifacts/m23-release-lifecycle-kind; credentials, tokens and
kubeconfig material remain in memory and are never archived.
#>
[CmdletBinding()]
param(
    [string]$ApiBase = 'http://127.0.0.1:8080',
    [string]$Username = '',
    [string]$AdminPassword = $env:AIOPS_ADMIN_PASSWORD,
    [string]$KindNodeImage = '',
    [string]$WorkloadImage = 'registry.k8s.io/pause:3.10',
    [string]$UpdatedImage = 'registry.k8s.io/pause:3.9',
    [int]$ReadyTimeoutSeconds = 180,
    [int]$RolloutTimeoutSeconds = 180
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Root = Split-Path -Parent $PSScriptRoot
$ArtifactDirectory = Join-Path $Root '.artifacts\m23-release-lifecycle-kind'
$RunID = '{0}-{1}' -f (Get-Date -Format 'yyyyMMddHHmmss'), ([guid]::NewGuid().ToString('N').Substring(0, 8))
$KindClusterName = "m23-release-$RunID"
$Context = "kind-$KindClusterName"
$Namespace = "aiops-m23-e2e"
$TargetDeployment = 'release-target'
$PlatformClusterName = "m23-kind-$RunID"
$kindCommand = Get-Command kind -ErrorAction SilentlyContinue
$Kind = if ($null -ne $kindCommand) { $kindCommand.Source } else { Join-Path $Root '.tools\kind-v0.30.0.exe' }
if (-not (Test-Path -LiteralPath $Kind)) { throw 'kind executable is required' }

if ($ReadyTimeoutSeconds -lt 30 -or $RolloutTimeoutSeconds -lt 30) {
    throw 'ready and rollout timeouts must be at least 30 seconds'
}
if ($WorkloadImage -eq $UpdatedImage) {
    throw 'WorkloadImage and UpdatedImage must differ to exercise the image-update diff'
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

function Wait-DeploymentAvailable {
    param([Parameter(Mandatory)] [string]$Namespace, [Parameter(Mandatory)] [string]$Name)
    $deadline = (Get-Date).AddSeconds($RolloutTimeoutSeconds)
    do {
        $deployment = Invoke-KubectlText @('-n', $Namespace, 'get', 'deployment', $Name, '-o', 'json') | ConvertFrom-Json
        $desired = [int]$deployment.spec.replicas
        $available = 0
        $availableProperty = $deployment.status.PSObject.Properties['availableReplicas']
        if ($null -ne $availableProperty) { $available = [int]$availableProperty.Value }
        if ($available -ge $desired -and $desired -gt 0) { return $deployment }
        Start-Sleep -Seconds 3
    } while ((Get-Date) -lt $deadline)
    throw "Deployment $Namespace/$Name did not become available before the deadline"
}

function Get-CurrentImage {
    param([Parameter(Mandatory)]$Deployment, [Parameter(Mandatory)] [string]$ContainerName)
    foreach ($containerSpec in $Deployment.spec.template.spec.containers) {
        if ([string]$containerSpec.name -eq $ContainerName) { return [string]$containerSpec.image }
    }
    throw "container $ContainerName not found on deployment"
}

function Get-ReplicaSetByRevision {
    param([Parameter(Mandatory)] [int]$Revision)
    $replicaSets = Invoke-KubectlText @('-n', $Namespace, 'get', 'rs', '-o', 'json') | ConvertFrom-Json
    foreach ($rs in $replicaSets.items) {
        $rsRevision = 0
        $revisionAnnotation = $rs.metadata.annotations.PSObject.Properties['deployment.kubernetes.io/revision']
        if ($null -ne $revisionAnnotation -and -not [string]::IsNullOrWhiteSpace([string]$revisionAnnotation.Value)) {
            $rsRevision = [int]$revisionAnnotation.Value
        }
        if ($rsRevision -eq $Revision) { return $rs }
    }
    throw "ReplicaSet for revision $Revision not found"
}

function Assert-Equal {
    param($Actual, $Expected, [Parameter(Mandatory)] [string]$Message)
    if ($Actual -ne $Expected) {
        throw "$Message; expected $Expected, got $Actual"
    }
}

foreach ($command in @('docker', 'kubectl')) {
    if ($null -eq (Get-Command $command -ErrorAction SilentlyContinue)) { throw "$command is required for the M23 kind acceptance" }
}

if ([string]::IsNullOrWhiteSpace($Username)) { $Username = Get-RuntimeValue -Name 'BOOTSTRAP_ADMIN_USERNAME' -Fallback 'admin' }
if ([string]::IsNullOrWhiteSpace($AdminPassword)) { $AdminPassword = Get-RuntimeValue -Name 'BOOTSTRAP_ADMIN_PASSWORD' }
if ([string]::IsNullOrWhiteSpace($AdminPassword)) {
    throw 'set AIOPS_ADMIN_PASSWORD, pass -AdminPassword, or configure BOOTSTRAP_ADMIN_PASSWORD in .env'
}

$kindCreated = $false
$platformClusterID = 0L
$accessToken = ''
$failure = $null
$cleanupErrors = [Collections.Generic.List[string]]::new()
$summary = $null

Write-Host '[0/8] Caching the workload image (docker image inspect / docker pull)'
if (-not (Test-DockerImageExists -Image $WorkloadImage)) {
    Invoke-NativeText -FilePath 'docker' -Arguments @('pull', $WorkloadImage) | Write-Host
}
if (-not (Test-DockerImageExists -Image $UpdatedImage)) {
    Invoke-NativeText -FilePath 'docker' -Arguments @('pull', $UpdatedImage) | Write-Host
}

$existingKindClusters = @(Invoke-NativeText -FilePath $Kind -Arguments @('get', 'clusters') -AllowFailure) -split "`r?`n" |
    Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
if ($existingKindClusters -contains $KindClusterName) { throw "refusing to reuse existing kind cluster $KindClusterName" }
$AiopsTestWasPresent = $existingKindClusters -contains 'aiops-test'
$PreviousContext = (Invoke-NativeText -FilePath 'kubectl' -Arguments @('config', 'current-context') -AllowFailure).Trim()
$knownContexts = @(Invoke-NativeText -FilePath 'kubectl' -Arguments @('config', 'get-contexts', '-o', 'name') -AllowFailure) -split "`r?`n"
if ($knownContexts -notcontains $PreviousContext) { $PreviousContext = '' }

try {
    Write-Host '[1/8] Checking the isolated platform runtime and creating a disposable kind cluster'
    Wait-BackendReady
    $kindArguments = @('create', 'cluster', '--name', $KindClusterName, '--wait', "$ReadyTimeoutSeconds`s")
    if (-not [string]::IsNullOrWhiteSpace($KindNodeImage)) { $kindArguments += @('--image', $KindNodeImage) }
    Invoke-NativeText -FilePath $Kind -Arguments $kindArguments | Write-Host
    $kindCreated = $true

    Write-Host '[2/8] Installing least-privilege observer RBAC and the M23 release-target fixture'
    Invoke-KubectlText @('apply', '-f', (Join-Path $Root 'deploy\managed-cluster\observer.yaml')) | Write-Host
    Invoke-KubectlText @('apply', '-k', (Join-Path $Root 'deploy\m23-release-lifecycle-e2e')) | Write-Host
    $baselineDeployment = Wait-DeploymentAvailable -Namespace $Namespace -Name $TargetDeployment
    $baselineImage = Get-CurrentImage -Deployment $baselineDeployment -ContainerName 'app'
    Assert-Equal $baselineImage $WorkloadImage 'baseline deployment image mismatch'

    Write-Host '[3/8] Registering only the disposable cluster'
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
  - name: m23
    cluster:
      server: $server
$tlsServerNameLine      certificate-authority-data: $ca
contexts:
  - name: m23
    context: { cluster: m23, user: aiops-platform }
current-context: m23
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

    Write-Host '[4/8] Reading rollout history and status from the platform contract'
    $history = Invoke-RestMethod -Uri "$ApiBase/api/v1/clusters/$platformClusterID/deployments/$Namespace/$TargetDeployment/rollout/history" -Headers $headers -TimeoutSec 15
    $status = Invoke-RestMethod -Uri "$ApiBase/api/v1/clusters/$platformClusterID/deployments/$Namespace/$TargetDeployment/rollout/status" -Headers $headers -TimeoutSec 15
    Assert-Equal ([string]$history.deployment) $TargetDeployment 'rollout history deployment name mismatch'
    Assert-Equal ([string]$history.namespace) $Namespace 'rollout history namespace mismatch'
    Assert-Equal ([int]$history.current_revision) 1 'baseline current revision must be 1'
    if (@($history.revisions).Count -lt 1) { throw 'rollout history must include at least one revision' }
    Assert-Equal ([string]$status.phase) 'complete' 'rollout status phase must be complete on a healthy deployment'
    Assert-Equal ([int]$status.current_revision) 1 'rollout status current revision mismatch'

    Write-Host '[5/8] Previewing and executing a controlled image update'
    $imagePreview = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/clusters/$platformClusterID/operations/preview" -Headers $headers -ContentType 'application/json' -Body (@{
        action = 'deployment.image_update'
        namespace = $Namespace
        target_name = $TargetDeployment
        container_name = 'app'
        desired_image = $UpdatedImage
    } | ConvertTo-Json)
    Assert-Equal ([string]$imagePreview.action) 'deployment.image_update' 'image update action mismatch'
    Assert-Equal ([string]$imagePreview.parameters.container_name) 'app' 'image update container mismatch'
    Assert-Equal ([string]$imagePreview.parameters.before_image) $baselineImage 'image update before image mismatch'
    Assert-Equal ([string]$imagePreview.parameters.desired_image) $UpdatedImage 'image update desired image mismatch'
    if ($null -eq $imagePreview.change) { throw 'image update preview must emit a typed change' }
    Assert-Equal ([string]$imagePreview.change.before) $baselineImage 'image update change before mismatch'
    Assert-Equal ([string]$imagePreview.change.after) $UpdatedImage 'image update change after mismatch'
    $imageHeaders = @{ Authorization = "Bearer $accessToken"; 'Idempotency-Key' = "m23-image-$RunID" }
    $imageBody = @{ confirmation_token = $imagePreview.confirmation_token } | ConvertTo-Json
    $imageExecuted = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/remediations/$($imagePreview.id)/execute" -Headers $imageHeaders -ContentType 'application/json' -Body $imageBody
    Assert-Equal ([string]$imageExecuted.status) 'succeeded' 'image update execution failed'
    $imageReplayed = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/remediations/$($imagePreview.id)/execute" -Headers $imageHeaders -ContentType 'application/json' -Body $imageBody
    Assert-Equal ([string]$imageReplayed.id) ([string]$imageExecuted.id) 'image update replay returned another plan'
    $updatedDeployment = Wait-DeploymentAvailable -Namespace $Namespace -Name $TargetDeployment
    Assert-Equal (Get-CurrentImage -Deployment $updatedDeployment -ContainerName 'app') $UpdatedImage 'deployment image was not updated'

    $historyAfterUpdate = Invoke-RestMethod -Uri "$ApiBase/api/v1/clusters/$platformClusterID/deployments/$Namespace/$TargetDeployment/rollout/history" -Headers $headers -TimeoutSec 15
    Assert-Equal ([int]$historyAfterUpdate.current_revision) 2 'current revision must advance to 2 after image update'
    if (@($historyAfterUpdate.revisions).Count -lt 2) { throw 'rollout history must include at least two revisions after image update' }
    $revision1 = @($historyAfterUpdate.revisions) | Where-Object { [int]$_.revision -eq 1 } | Select-Object -First 1
    $revision2 = @($historyAfterUpdate.revisions) | Where-Object { [int]$_.revision -eq 2 } | Select-Object -First 1
    if ($null -eq $revision1 -or $null -eq $revision2) { throw 'rollout history must enumerate revisions 1 and 2 after image update' }
    Assert-Equal ([bool]$revision2.current) $true 'revision 2 must be marked current after image update'
    Assert-Equal ([bool]$revision1.current) $false 'revision 1 must not be marked current after image update'

    Write-Host '[6/8] Previewing and executing a controlled rollback to revision 1'
    $rollbackPreview = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/clusters/$platformClusterID/operations/preview" -Headers $headers -ContentType 'application/json' -Body (@{
        action = 'deployment.rollback'
        namespace = $Namespace
        target_name = $TargetDeployment
        rollback_revision = 1
    } | ConvertTo-Json)
    Assert-Equal ([string]$rollbackPreview.action) 'deployment.rollback' 'rollback action mismatch'
    Assert-Equal ([int]$rollbackPreview.parameters.rollback_revision) 1 'rollback revision parameter mismatch'
    if ($null -eq $rollbackPreview.change) { throw 'rollback preview must emit a typed change' }
    Assert-Equal ([int]$rollbackPreview.change.after) 1 'rollback change after mismatch'
    $rollbackHeaders = @{ Authorization = "Bearer $accessToken"; 'Idempotency-Key' = "m23-rollback-$RunID" }
    $rollbackBody = @{ confirmation_token = $rollbackPreview.confirmation_token } | ConvertTo-Json
    $rollbackExecuted = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/remediations/$($rollbackPreview.id)/execute" -Headers $rollbackHeaders -ContentType 'application/json' -Body $rollbackBody
    Assert-Equal ([string]$rollbackExecuted.status) 'succeeded' 'rollback execution failed'
    $rollbackReplayed = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/remediations/$($rollbackPreview.id)/execute" -Headers $rollbackHeaders -ContentType 'application/json' -Body $rollbackBody
    Assert-Equal ([string]$rollbackReplayed.id) ([string]$rollbackExecuted.id) 'rollback replay returned another plan'
    $rolledBackDeployment = Wait-DeploymentAvailable -Namespace $Namespace -Name $TargetDeployment
    Assert-Equal (Get-CurrentImage -Deployment $rolledBackDeployment -ContainerName 'app') $baselineImage 'deployment image was not restored by rollback'

    $historyAfterRollback = Invoke-RestMethod -Uri "$ApiBase/api/v1/clusters/$platformClusterID/deployments/$Namespace/$TargetDeployment/rollout/history" -Headers $headers -TimeoutSec 15
    Assert-Equal ([int]$historyAfterRollback.current_revision) 3 'current revision must advance to 3 after rollback'
    $revision3 = @($historyAfterRollback.revisions) | Where-Object { [int]$_.revision -eq 3 } | Select-Object -First 1
    if ($null -eq $revision3) { throw 'rollout history must enumerate revision 3 after rollback' }
    Assert-Equal ([bool]$revision3.current) $true 'revision 3 must be marked current after rollback'
    $revision3Images = (@($revision3.images) -join ',')
    if (-not $revision3Images.Contains($baselineImage)) { throw "revision 3 must reference the restored baseline image; got $revision3Images" }

    Write-Host '[7/8] Verifying namespaced RBAC and operation audit history'
    $actor = 'system:serviceaccount:kube-system:aiops-platform'
    $canPatchDeployment = Invoke-KubectlText -Arguments @('auth', 'can-i', 'patch', 'deployments', '-n', $Namespace, "--as=$actor") -AllowFailure
    $canPatchSystem = Invoke-KubectlText -Arguments @('auth', 'can-i', 'patch', 'deployments', '-n', 'kube-system', "--as=$actor") -AllowFailure
    $canListReplicaSets = Invoke-KubectlText -Arguments @('auth', 'can-i', 'list', 'replicasets.apps', '-n', $Namespace, "--as=$actor") -AllowFailure
    $canDeletePod = Invoke-KubectlText -Arguments @('auth', 'can-i', 'delete', 'pods', '-n', $Namespace, "--as=$actor") -AllowFailure
    Assert-Equal ([string]$canPatchDeployment) 'yes' 'namespaced Deployment patch permission is missing'
    Assert-Equal ([string]$canPatchSystem) 'no' 'service account unexpectedly can patch kube-system Deployments'
    Assert-Equal ([string]$canListReplicaSets) 'yes' 'observer ReplicaSet read permission is missing'
    Assert-Equal ([string]$canDeletePod) 'no' 'service account unexpectedly can delete Pods'

    $operations = Invoke-RestMethod -Uri "$ApiBase/api/v1/clusters/$platformClusterID/operations?namespace=$Namespace&target_name=$TargetDeployment" -Headers $headers -TimeoutSec 15
    if (@($operations.items).Count -lt 2) { throw 'operation history must include the image update and rollback plans' }
    $operationActions = @($operations.items | ForEach-Object { [string]$_.action }) -join ','
    if (-not $operationActions.Contains('deployment.image_update')) { throw 'operation history missing image_update plan' }
    if (-not $operationActions.Contains('deployment.rollback')) { throw 'operation history missing rollback plan' }

    $summary = [ordered]@{
        verified_at = (Get-Date).ToUniversalTime().ToString('o')
        mode = 'disposable-kind-isolated-registration'
        kind_cluster = $KindClusterName
        preexisting_aiops_test = $AiopsTestWasPresent
        platform_cluster_id = $platformClusterID
        namespace = $Namespace
        deployment = $TargetDeployment
        baseline_image = $baselineImage
        updated_image = $UpdatedImage
        rollout_history = [ordered]@{
            baseline_revisions = @($history.revisions).Count
            after_update_revisions = @($historyAfterUpdate.revisions).Count
            after_rollback_revisions = @($historyAfterRollback.revisions).Count
            baseline_current_revision = [int]$history.current_revision
            after_update_current_revision = [int]$historyAfterUpdate.current_revision
            after_rollback_current_revision = [int]$historyAfterRollback.current_revision
        }
        image_update = [ordered]@{
            plan_id = [string]$imageExecuted.id
            status = [string]$imageExecuted.status
            replay_same_plan = ([string]$imageReplayed.id -eq [string]$imageExecuted.id)
            change_field = [string]$imagePreview.change.field
        }
        rollback = [ordered]@{
            plan_id = [string]$rollbackExecuted.id
            status = [string]$rollbackExecuted.status
            replay_same_plan = ([string]$rollbackReplayed.id -eq [string]$rollbackExecuted.id)
            restored_image = (Get-CurrentImage -Deployment $rolledBackDeployment -ContainerName 'app')
        }
        rbac = [ordered]@{
            can_patch_deployment = ([string]$canPatchDeployment)
            cannot_patch_kube_system = ([string]$canPatchSystem)
            can_list_replicasets = ([string]$canListReplicaSets)
            cannot_delete_pods = ([string]$canDeletePod)
        }
        operation_history_count = @($operations.items).Count
        credential_lifetime = '1h'
        cleanup_complete = $false
    }
} catch {
    $failure = $_
} finally {
    Write-Host '[8/8] Removing only this run registration and disposable kind cluster'
    if ($platformClusterID -gt 0 -and -not [string]::IsNullOrWhiteSpace($accessToken)) {
        try {
            Invoke-WebRequest -UseBasicParsing -Method Delete -Uri "$ApiBase/api/v1/clusters/$platformClusterID" `
                -Headers @{ Authorization = "Bearer $accessToken" } -TimeoutSec 20 | Out-Null
        } catch { $cleanupErrors.Add('platform cluster registration cleanup failed') }
    }
    if ($kindCreated) {
        if (-not $KindClusterName.StartsWith('m23-release-', [StringComparison]::Ordinal)) {
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
$evidencePath = Join-Path $ArtifactDirectory ("m23-release-lifecycle-kind-{0}.json" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
[IO.File]::WriteAllText($evidencePath, ($summary | ConvertTo-Json -Depth 12), [Text.UTF8Encoding]::new($false))
Write-Host "M23 real kind release lifecycle acceptance passed. Redacted evidence: $evidencePath"
$summary | ConvertTo-Json -Depth 12
