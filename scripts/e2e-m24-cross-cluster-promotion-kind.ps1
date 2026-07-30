<#
.SYNOPSIS
Runs the M24 cross-cluster promotion acceptance against two disposable kind clusters.

.DESCRIPTION
Requires Docker, kind, kubectl, the repository backend with M24 promotion routes, and
registry access for the pinned workload image. The run normally takes 5-10 minutes because
it creates two kind clusters, registers both with the platform, promotes a Deployment/Service
bundle from source to destination, and verifies the promoted resources exist on the destination.

The script creates uniquely named kind clusters and platform registrations, never reuses
aiops-test, and removes only resources bearing this run's identifier in finally. It writes a
redacted success summary under .artifacts/m24-cross-cluster-promotion-kind; credentials, tokens
and kubeconfig material remain in memory and are never archived.
#>
[CmdletBinding()]
param(
    [string]$ApiBase = 'http://127.0.0.1:8080',
    [string]$Username = '',
    [string]$AdminPassword = $env:AIOPS_ADMIN_PASSWORD,
    [string]$KindNodeImage = '',
    [string]$WorkloadImage = 'registry.k8s.io/pause:3.10',
    [int]$ReadyTimeoutSeconds = 180,
    [int]$RolloutTimeoutSeconds = 180
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Root = Split-Path -Parent $PSScriptRoot
$ArtifactDirectory = Join-Path $Root '.artifacts\m24-cross-cluster-promotion-kind'
$RunID = '{0}-{1}' -f (Get-Date -Format 'yyyyMMddHHmmss'), ([guid]::NewGuid().ToString('N').Substring(0, 8))
$SourceKindClusterName = "m24-source-$RunID"
$DestKindClusterName = "m24-dest-$RunID"
$SourceContext = "kind-$SourceKindClusterName"
$DestContext = "kind-$DestKindClusterName"
$SourceNamespace = 'aiops-m24-source'
$DestNamespace = 'aiops-m24-dest'
$TargetDeployment = 'promote-target'
$TargetService = 'promote-target'
$SourcePlatformClusterName = "m24-source-$RunID"
$DestPlatformClusterName = "m24-dest-$RunID"
$kindCommand = Get-Command kind -ErrorAction SilentlyContinue
$Kind = if ($null -ne $kindCommand) { $kindCommand.Source } else { Join-Path $Root '.tools\kind-v0.30.0.exe' }
if (-not (Test-Path -LiteralPath $Kind)) { throw 'kind executable is required' }

if ($ReadyTimeoutSeconds -lt 30 -or $RolloutTimeoutSeconds -lt 30) {
    throw 'ready and rollout timeouts must be at least 30 seconds'
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
    param(
        [Parameter(Mandatory, Position = 0)] [string[]]$Arguments,
        [Parameter(Mandatory)] [string]$Context,
        [switch]$AllowFailure
    )
    return Invoke-NativeText -FilePath 'kubectl' -Arguments (@('--context', $Context) + $Arguments) -AllowFailure:$AllowFailure
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
    param([Parameter(Mandatory)] [string]$Context, [Parameter(Mandatory)] [string]$Namespace, [Parameter(Mandatory)] [string]$Name)
    $deadline = (Get-Date).AddSeconds($RolloutTimeoutSeconds)
    do {
        $deployment = Invoke-KubectlText -Context $Context @('-n', $Namespace, 'get', 'deployment', $Name, '-o', 'json') | ConvertFrom-Json
        $desired = [int]$deployment.spec.replicas
        $available = 0
        $availableProperty = $deployment.status.PSObject.Properties['availableReplicas']
        if ($null -ne $availableProperty) { $available = [int]$availableProperty.Value }
        if ($available -ge $desired -and $desired -gt 0) { return $deployment }
        Start-Sleep -Seconds 3
    } while ((Get-Date) -lt $deadline)
    throw "Deployment $Namespace/$Name did not become available before the deadline"
}

function Assert-Equal {
    param($Actual, $Expected, [Parameter(Mandatory)] [string]$Message)
    if ($Actual -ne $Expected) {
        throw "$Message; expected $Expected, got $Actual"
    }
}

function New-KindCluster {
    param([Parameter(Mandatory)] [string]$Name)
    $kindArguments = @('create', 'cluster', '--name', $Name, '--wait', "$ReadyTimeoutSeconds`s")
    if (-not [string]::IsNullOrWhiteSpace($KindNodeImage)) { $kindArguments += @('--image', $KindNodeImage) }
    Invoke-NativeText -FilePath $Kind -Arguments $kindArguments | Write-Host
}

function Register-PlatformCluster {
    param(
        [Parameter(Mandatory)] [string]$Context,
        [Parameter(Mandatory)] [string]$PlatformName,
        [Parameter(Mandatory)] [Collections.IDictionary]$Headers
    )
    $token = Invoke-KubectlText -Context $Context @('-n', 'kube-system', 'create', 'token', 'aiops-platform', '--duration=1h')
    $rawContext = Invoke-KubectlText -Context $Context @('config', 'view', '--raw', '--minify', '-o', 'json') | ConvertFrom-Json
    $server = [string]$rawContext.clusters[0].cluster.server
    $ca = [string]$rawContext.clusters[0].cluster.'certificate-authority-data'
    if ([string]::IsNullOrWhiteSpace($server) -or [string]::IsNullOrWhiteSpace($ca) -or [string]::IsNullOrWhiteSpace($token)) {
        throw "kind context $Context did not provide server, CA and short-lived token material"
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
  - name: $PlatformName
    cluster:
      server: $server
$tlsServerNameLine      certificate-authority-data: $ca
contexts:
  - name: $PlatformName
    context: { cluster: $PlatformName, user: aiops-platform }
current-context: $PlatformName
users:
  - name: aiops-platform
    user: { token: $token }
"@
    $cluster = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/clusters" -Headers $Headers -ContentType 'application/json' `
        -Body (@{ name = $PlatformName; kubeconfig = $kubeconfig } | ConvertTo-Json -Compress) -TimeoutSec 20
    $clusterID = [int64]$cluster.id
    Invoke-WebRequest -UseBasicParsing -Method Patch -Uri "$ApiBase/api/v1/clusters/$clusterID" -Headers $Headers `
        -ContentType 'application/json' -Body '{"enabled":true}' -TimeoutSec 15 | Out-Null
    $probe = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/clusters/$clusterID/probe" -Headers $Headers -TimeoutSec 20
    if ($probe.status -ne 'ready') { throw "cluster $PlatformName probe did not become ready" }
    return $clusterID
}

foreach ($command in @('docker', 'kubectl')) {
    if ($null -eq (Get-Command $command -ErrorAction SilentlyContinue)) { throw "$command is required for the M24 kind acceptance" }
}

if ([string]::IsNullOrWhiteSpace($Username)) { $Username = Get-RuntimeValue -Name 'BOOTSTRAP_ADMIN_USERNAME' -Fallback 'admin' }
if ([string]::IsNullOrWhiteSpace($AdminPassword)) { $AdminPassword = Get-RuntimeValue -Name 'BOOTSTRAP_ADMIN_PASSWORD' }
if ([string]::IsNullOrWhiteSpace($AdminPassword)) {
    throw 'set AIOPS_ADMIN_PASSWORD, pass -AdminPassword, or configure BOOTSTRAP_ADMIN_PASSWORD in .env'
}

$sourceKindCreated = $false
$destKindCreated = $false
$sourceClusterID = 0L
$destClusterID = 0L
$accessToken = ''
$failure = $null
$cleanupErrors = [Collections.Generic.List[string]]::new()
$summary = $null

Write-Host '[0/7] Caching the workload image'
if (-not (Test-DockerImageExists -Image $WorkloadImage)) {
    Invoke-NativeText -FilePath 'docker' -Arguments @('pull', $WorkloadImage) | Write-Host
}

$existingKindClusters = @(Invoke-NativeText -FilePath $Kind -Arguments @('get', 'clusters') -AllowFailure) -split "`r?`n" |
    Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
if ($existingKindClusters -contains $SourceKindClusterName) { throw "refusing to reuse existing kind cluster $SourceKindClusterName" }
if ($existingKindClusters -contains $DestKindClusterName) { throw "refusing to reuse existing kind cluster $DestKindClusterName" }
$AiopsTestWasPresent = $existingKindClusters -contains 'aiops-test'
$PreviousContext = (Invoke-NativeText -FilePath 'kubectl' -Arguments @('config', 'current-context') -AllowFailure).Trim()
$knownContexts = @(Invoke-NativeText -FilePath 'kubectl' -Arguments @('config', 'get-contexts', '-o', 'name') -AllowFailure) -split "`r?`n"
if ($knownContexts -notcontains $PreviousContext) { $PreviousContext = '' }

try {
    Write-Host '[1/7] Checking the platform runtime and creating two disposable kind clusters'
    Wait-BackendReady
    New-KindCluster -Name $SourceKindClusterName
    $sourceKindCreated = $true
    New-KindCluster -Name $DestKindClusterName
    $destKindCreated = $true

    Write-Host '[2/7] Installing observer RBAC and fixtures on both clusters'
    Invoke-KubectlText -Context $SourceContext @('apply', '-f', (Join-Path $Root 'deploy\managed-cluster\observer.yaml')) | Write-Host
    Invoke-KubectlText -Context $DestContext @('apply', '-f', (Join-Path $Root 'deploy\managed-cluster\observer.yaml')) | Write-Host
    Invoke-KubectlText -Context $SourceContext @('apply', '-k', (Join-Path $Root 'deploy\m24-cross-cluster-promotion-e2e\source')) | Write-Host
    Invoke-KubectlText -Context $DestContext @('apply', '-k', (Join-Path $Root 'deploy\m24-cross-cluster-promotion-e2e\destination')) | Write-Host
    $baselineDeployment = Wait-DeploymentAvailable -Context $SourceContext -Namespace $SourceNamespace -Name $TargetDeployment
    Assert-Equal ([string]$baselineDeployment.spec.template.spec.containers[0].image) $WorkloadImage 'baseline deployment image mismatch'

    Write-Host '[3/7] Registering both clusters with the platform'
    $login = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/auth/login" -ContentType 'application/json' `
        -Body (@{ username = $Username; password = $AdminPassword } | ConvertTo-Json -Compress) -TimeoutSec 15
    $accessToken = [string]$login.access_token
    if ([string]::IsNullOrWhiteSpace($accessToken)) { throw 'platform login did not return an access token' }
    $headers = @{ Authorization = "Bearer $accessToken" }
    $sourceClusterID = Register-PlatformCluster -Context $SourceContext -PlatformName $SourcePlatformClusterName -Headers $headers
    $destClusterID = Register-PlatformCluster -Context $DestContext -PlatformName $DestPlatformClusterName -Headers $headers

    Write-Host '[4/7] Previewing the cross-cluster promotion bundle'
    $previewBody = @{
        source_cluster_id = $sourceClusterID
        destination_cluster_id = $destClusterID
        source_namespace = $SourceNamespace
        destination_namespace = $DestNamespace
        bundle = @(
            @{ kind = 'Deployment'; namespace = $SourceNamespace; name = $TargetDeployment },
            @{ kind = 'Service'; namespace = $SourceNamespace; name = $TargetService }
        )
        dependency_mappings = @(
            @{ kind = 'ConfigMap'; source_namespace = $SourceNamespace; source_name = 'app-config'; destination_namespace = $DestNamespace; destination_name = 'app-config-promoted' }
        )
    }
    $preview = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/promotions/preview" -Headers $headers -ContentType 'application/json' `
        -Body ($previewBody | ConvertTo-Json -Depth 10) -TimeoutSec 30
    Assert-Equal ([string]$preview.status) 'awaiting_confirmation' 'preview status must be awaiting_confirmation'
    if ([string]::IsNullOrWhiteSpace($preview.confirmation_token)) { throw 'preview must return a one-time confirmation token' }
    if (@($preview.items).Count -lt 2) { throw 'preview must include at least 2 bundle items (Deployment + Service)' }
    $depItem = @($preview.items) | Where-Object { [string]$_.kind -eq 'Deployment' } | Select-Object -First 1
    if ($null -eq $depItem) { throw 'preview must include the Deployment bundle item' }
    if ([string]$depItem.destination_namespace -ne $DestNamespace) { throw 'preview must rewrite destination namespace' }
    if ([string]$depItem.source_uid -eq '') { throw 'preview must capture the source UID' }
    if (@($preview.dependencies).Count -lt 1) { throw 'preview must include the resolved ConfigMap dependency' }
    $resolvedDep = @($preview.dependencies) | Where-Object { [string]$_.kind -eq 'ConfigMap' } | Select-Object -First 1
    if ($null -eq $resolvedDep -or -not [bool]$resolvedDep.resolved) { throw 'ConfigMap dependency must be marked resolved' }

    Write-Host '[5/7] Executing the promotion with idempotency key'
    $executeHeaders = @{ Authorization = "Bearer $accessToken"; 'Idempotency-Key' = "m24-promote-$RunID" }
    $executeBody = @{ confirmation_token = $preview.confirmation_token } | ConvertTo-Json -Compress
    $executed = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/promotions/$($preview.id)/execute" -Headers $executeHeaders -ContentType 'application/json' -Body $executeBody -TimeoutSec 30
    Assert-Equal ([string]$executed.status) 'succeeded' 'promotion execution must succeed'

    Write-Host '[6/7] Verifying promoted resources exist on the destination cluster'
    $destDeployment = Invoke-KubectlText -Context $DestContext @('-n', $DestNamespace, 'get', 'deployment', $TargetDeployment, '-o', 'json') | ConvertFrom-Json
    Assert-Equal ([string]$destDeployment.spec.template.spec.containers[0].image) $WorkloadImage 'promoted deployment image mismatch'
    Assert-Equal ([int]$destDeployment.spec.replicas) 2 'promoted deployment replicas mismatch'
    Assert-Equal ([string]$destDeployment.spec.template.spec.containers[0].envFrom[0].configMapRef.name) 'app-config-promoted' 'promoted deployment dependency name was not rewritten'
    $destService = Invoke-KubectlText -Context $DestContext @('-n', $DestNamespace, 'get', 'service', $TargetService, '-o', 'json') | ConvertFrom-Json
    Assert-Equal ([string]$destService.spec.type) 'ClusterIP' 'promoted service type mismatch'
    Assert-Equal ([int]$destService.spec.ports[0].port) 80 'promoted service port mismatch'

    Write-Host '[7/7] Verifying promotion plan retrieval and listing'
    $retrieved = Invoke-RestMethod -Uri "$ApiBase/api/v1/promotions/$($preview.id)" -Headers $headers -TimeoutSec 15
    Assert-Equal ([string]$retrieved.id) ([string]$preview.id) 'retrieved plan id mismatch'
    Assert-Equal ([string]$retrieved.status) 'succeeded' 'retrieved plan status mismatch'
    $listed = Invoke-RestMethod -Uri "$ApiBase/api/v1/promotions?source_cluster_id=$sourceClusterID" -Headers $headers -TimeoutSec 15
    if (@($listed.items).Count -lt 1) { throw 'promotion list must include the executed plan' }

    $summary = [ordered]@{
        verified_at = (Get-Date).ToUniversalTime().ToString('o')
        mode = 'disposable-kind-isolated-registration'
        source_kind_cluster = $SourceKindClusterName
        dest_kind_cluster = $DestKindClusterName
        preexisting_aiops_test = $AiopsTestWasPresent
        source_platform_cluster_id = $sourceClusterID
        dest_platform_cluster_id = $destClusterID
        source_namespace = $SourceNamespace
        dest_namespace = $DestNamespace
        deployment = $TargetDeployment
        service = $TargetService
        plan_id = [string]$preview.id
        plan_status = [string]$executed.status
        bundle_items = @($preview.items).Count
        dependencies = @($preview.dependencies).Count
        promoted_deployment_image = [string]$destDeployment.spec.template.spec.containers[0].image
        promoted_deployment_replicas = [int]$destDeployment.spec.replicas
        promoted_configmap_reference = [string]$destDeployment.spec.template.spec.containers[0].envFrom[0].configMapRef.name
        credential_lifetime = '1h'
        cleanup_complete = $false
    }
} catch {
    $failure = $_
} finally {
    Write-Host '[cleanup] Removing platform registrations and disposable kind clusters'
    if ($destClusterID -gt 0 -and -not [string]::IsNullOrWhiteSpace($accessToken)) {
        try { Invoke-WebRequest -UseBasicParsing -Method Delete -Uri "$ApiBase/api/v1/clusters/$destClusterID" -Headers @{ Authorization = "Bearer $accessToken" } -TimeoutSec 20 | Out-Null }
        catch { $cleanupErrors.Add('destination platform cluster cleanup failed') }
    }
    if ($sourceClusterID -gt 0 -and -not [string]::IsNullOrWhiteSpace($accessToken)) {
        try { Invoke-WebRequest -UseBasicParsing -Method Delete -Uri "$ApiBase/api/v1/clusters/$sourceClusterID" -Headers @{ Authorization = "Bearer $accessToken" } -TimeoutSec 20 | Out-Null }
        catch { $cleanupErrors.Add('source platform cluster cleanup failed') }
    }
    if ($destKindCreated) {
        if (-not $DestKindClusterName.StartsWith('m24-dest-', [StringComparison]::Ordinal)) {
            $cleanupErrors.Add('refused unsafe dest kind cleanup target')
        } else {
            try { Invoke-NativeText -FilePath $Kind -Arguments @('delete', 'cluster', '--name', $DestKindClusterName) | Write-Host }
            catch { $cleanupErrors.Add('destination kind cluster cleanup failed') }
        }
    }
    if ($sourceKindCreated) {
        if (-not $SourceKindClusterName.StartsWith('m24-source-', [StringComparison]::Ordinal)) {
            $cleanupErrors.Add('refused unsafe source kind cleanup target')
        } else {
            try { Invoke-NativeText -FilePath $Kind -Arguments @('delete', 'cluster', '--name', $SourceKindClusterName) | Write-Host }
            catch { $cleanupErrors.Add('source kind cluster cleanup failed') }
        }
    }
    if (-not [string]::IsNullOrWhiteSpace($PreviousContext)) {
        try { Invoke-NativeText -FilePath 'kubectl' -Arguments @('config', 'use-context', $PreviousContext) -AllowFailure | Out-Null }
        catch { $cleanupErrors.Add('previous kubectl context restore failed') }
    }
    $remainingKindClusters = @(Invoke-NativeText -FilePath $Kind -Arguments @('get', 'clusters') -AllowFailure) -split "`r?`n" |
        Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
    if ($remainingKindClusters -contains $SourceKindClusterName) { $cleanupErrors.Add('source kind cluster still exists') }
    if ($remainingKindClusters -contains $DestKindClusterName) { $cleanupErrors.Add('destination kind cluster still exists') }
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
$evidencePath = Join-Path $ArtifactDirectory ("m24-cross-cluster-promotion-kind-{0}.json" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
[IO.File]::WriteAllText($evidencePath, ($summary | ConvertTo-Json -Depth 12), [Text.UTF8Encoding]::new($false))
Write-Host "M24 real kind cross-cluster promotion acceptance passed. Redacted evidence: $evidencePath"
$summary | ConvertTo-Json -Depth 12
