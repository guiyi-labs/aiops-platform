[CmdletBinding()]
param(
    [string]$ApiBase = 'http://127.0.0.1:8080',
    [string]$Username = 'admin',
    [string]$AdminPassword = $env:AIOPS_ADMIN_PASSWORD
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Root = Split-Path -Parent $PSScriptRoot
$Kind = Join-Path $Root '.tools\kind-v0.30.0.exe'
$FixtureDirectory = Join-Path $Root 'deploy\diagnosis-e2e'
$ObserverManifest = Join-Path $Root 'deploy\managed-cluster\observer.yaml'
$ArtifactDirectory = Join-Path $Root '.artifacts\diagnosis-e2e'
$Timestamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$KindClusterName = "aiops-diagnosis-$Timestamp"
$TemporaryKubeconfig = Join-Path ([IO.Path]::GetTempPath()) ("aiops-diagnosis-{0}.kubeconfig" -f [guid]::NewGuid().ToString('N'))
$TemporaryStatusPatch = Join-Path ([IO.Path]::GetTempPath()) ("aiops-diagnosis-{0}.node-status.json" -f [guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($ArtifactDirectory) | Out-Null

function Invoke-NativeText {
    param(
        [Parameter(Mandatory)] [string]$File,
        [Parameter(Mandatory)] [string[]]$Arguments,
        [switch]$AllowDenied
    )

    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $output = & $File @Arguments 2>&1
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorAction
    }
    if ($exitCode -ne 0 -and -not ($AllowDenied -and $exitCode -eq 1)) {
        throw "$File $($Arguments -join ' ') failed: $($output -join [Environment]::NewLine)"
    }
    return (($output | ForEach-Object { $_.ToString() }) -join [Environment]::NewLine).Trim()
}

function Invoke-KubectlText {
    param(
        [Parameter(Mandatory, Position = 0)] [string[]]$Arguments,
        [switch]$AllowDenied
    )

    return Invoke-NativeText -File 'kubectl' -Arguments (@('--kubeconfig', $TemporaryKubeconfig) + $Arguments) -AllowDenied:$AllowDenied
}

function Assert-Equal {
    param($Actual, $Expected, [Parameter(Mandatory)] [string]$Message)
    if ($Actual -ne $Expected) {
        throw "$Message; expected $Expected, got $Actual"
    }
}

function Get-AuthorizationDecision {
    param([Parameter(Mandatory)] [string[]]$Arguments)
    $output = Invoke-KubectlText -Arguments $Arguments -AllowDenied
    $decisions = @($output -split [Environment]::NewLine | ForEach-Object { $_.Trim() } | Where-Object { $_ -in @('yes', 'no') })
    if ($decisions.Count -ne 1) {
        throw "kubectl auth can-i did not return exactly one decision: $output"
    }
    return $decisions[0]
}

function Get-IntegerProperty {
    param($Object, [Parameter(Mandatory)] [string]$Name)
    if ($null -eq $Object) {
        return 0
    }
    $property = $Object.PSObject.Properties[$Name]
    if ($null -eq $property -or $null -eq $property.Value) {
        return 0
    }
    return [int]$property.Value
}

function Wait-DiagnosisFixtureState {
    $deadline = (Get-Date).AddMinutes(3)
    do {
        $deployment = Invoke-KubectlText @('-n', 'aiops-diagnosis-e2e', 'get', 'deployment', 'stalled-deployment', '-o', 'json') | ConvertFrom-Json
        $node = Invoke-KubectlText @('get', 'node', 'synthetic-not-ready', '-o', 'json') | ConvertFrom-Json
        $desired = Get-IntegerProperty $deployment.spec 'replicas'
        $replicas = Get-IntegerProperty $deployment.status 'replicas'
        $ready = Get-IntegerProperty $deployment.status 'readyReplicas'
        $available = Get-IntegerProperty $deployment.status 'availableReplicas'
        $unavailable = Get-IntegerProperty $deployment.status 'unavailableReplicas'
        $readyCondition = @($node.status.conditions | Where-Object { $_.type -eq 'Ready' } | Select-Object -First 1)
        if ($desired -eq 2 -and $replicas -eq 2 -and $ready -eq 0 -and $available -eq 0 -and $unavailable -eq 2 -and
            $readyCondition.Count -eq 1 -and $readyCondition[0].status -ne 'True') {
            return [ordered]@{
                deployment = [ordered]@{
                    desired_replicas = $desired
                    replicas = $replicas
                    ready_replicas = $ready
                    available_replicas = $available
                    unavailable_replicas = $unavailable
                }
                node = [ordered]@{
                    ready_status = $readyCondition[0].status
                    ready_reason = $readyCondition[0].reason
                }
            }
        }
        Start-Sleep -Seconds 2
    } while ((Get-Date) -lt $deadline)
    throw "diagnosis fixtures did not converge; desired=$desired replicas=$replicas ready=$ready available=$available unavailable=$unavailable"
}

function Get-EvidenceByType {
    param($Diagnosis, [Parameter(Mandatory)] [string]$Type)
    return @($Diagnosis.evidence | Where-Object { $_.type -eq $Type })
}

if (-not (Test-Path -LiteralPath $Kind)) {
    throw "bundled kind executable is missing: $Kind"
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

$initialKindClusters = @((Invoke-NativeText -File $Kind -Arguments @('get', 'clusters')) -split [Environment]::NewLine | Where-Object { $_ })
$clusterID = 0
$accessToken = ''
$kindCreated = $false
$failure = $null
$cleanupFailures = [Collections.Generic.List[string]]::new()
$cleanup = [ordered]@{
    platform_cluster_deleted = $false
    diagnosis_records_remaining = $null
    kind_cluster_deleted = $false
    temporary_kubeconfig_deleted = $false
    temporary_status_patch_deleted = $false
    preexisting_kind_clusters_preserved = $false
}
$summary = $null

try {
    $health = Invoke-RestMethod "$ApiBase/api/v1/health/ready" -TimeoutSec 10
    Assert-Equal $health.status 'ready' 'backend is not ready'

    Invoke-NativeText -File $Kind -Arguments @(
        'create', 'cluster', '--name', $KindClusterName,
        '--config', (Join-Path $FixtureDirectory 'kind.yaml'),
        '--kubeconfig', $TemporaryKubeconfig, '--wait', '120s'
    ) | Write-Host
    $kindCreated = $true

    Invoke-KubectlText @('apply', '--dry-run=server', '-f', (Join-Path $FixtureDirectory 'namespace.yaml')) | Write-Host
    Invoke-KubectlText @('apply', '-f', (Join-Path $FixtureDirectory 'namespace.yaml')) | Write-Host
    Invoke-KubectlText @('apply', '--dry-run=server', '-f', (Join-Path $FixtureDirectory 'workloads.yaml')) | Write-Host
    Invoke-KubectlText @('apply', '--dry-run=server', '-f', (Join-Path $FixtureDirectory 'synthetic-node.yaml')) | Write-Host
    Invoke-KubectlText @('apply', '-k', $FixtureDirectory) | Write-Host
    Invoke-KubectlText @('apply', '--dry-run=server', '-f', $ObserverManifest) | Write-Host
    Invoke-KubectlText @('apply', '-f', $ObserverManifest) | Write-Host

    $transitionTime = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
    $statusPatch = @{
        status = @{
            conditions = @(
                @{
                    type = 'Ready'
                    status = 'False'
                    reason = 'SyntheticKubeletUnavailable'
                    message = 'Synthetic node is intentionally unavailable for deterministic diagnosis verification.'
                    lastHeartbeatTime = $transitionTime
                    lastTransitionTime = $transitionTime
                },
                @{
                    type = 'MemoryPressure'
                    status = 'False'
                    reason = 'SyntheticNodeHasSufficientMemory'
                    message = 'Synthetic node reports no memory pressure.'
                    lastHeartbeatTime = $transitionTime
                    lastTransitionTime = $transitionTime
                }
            )
        }
    } | ConvertTo-Json -Depth 8 -Compress
    [IO.File]::WriteAllText($TemporaryStatusPatch, $statusPatch, [Text.UTF8Encoding]::new($false))
    Invoke-KubectlText @('patch', 'node', 'synthetic-not-ready', '--subresource=status', '--type=merge', '--patch-file', $TemporaryStatusPatch) | Write-Host
    $fixtureState = Wait-DiagnosisFixtureState

    $serviceAccountToken = Invoke-KubectlText @('-n', 'kube-system', 'create', 'token', 'aiops-platform', '--duration=30m')
    $rawContext = Invoke-KubectlText @('config', 'view', '--raw', '--minify', '-o', 'json') | ConvertFrom-Json
    $server = [string]$rawContext.clusters[0].cluster.server
    $ca = [string]$rawContext.clusters[0].cluster.'certificate-authority-data'
    if ([string]::IsNullOrWhiteSpace($server) -or [string]::IsNullOrWhiteSpace($ca)) {
        throw 'temporary kind context does not contain an embedded API server and CA'
    }
    $serverUri = [Uri]$server
    $tlsServerNameLine = ''
    if ($serverUri.IsLoopback) {
        $tlsServerNameLine = "      tls-server-name: $($serverUri.Host)$([Environment]::NewLine)"
        $builder = [UriBuilder]$serverUri
        $builder.Host = 'host.docker.internal'
        $server = $builder.Uri.AbsoluteUri.TrimEnd('/')
    }
    $platformKubeconfig = @"
apiVersion: v1
kind: Config
clusters:
  - name: diagnosis-e2e
    cluster:
      server: $server
$tlsServerNameLine      certificate-authority-data: $ca
contexts:
  - name: diagnosis-e2e
    context:
      cluster: diagnosis-e2e
      user: aiops-platform
current-context: diagnosis-e2e
users:
  - name: aiops-platform
    user:
      token: $serviceAccountToken
"@

    $login = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/auth/login" -ContentType 'application/json' -Body (@{
        username = $Username
        password = $AdminPassword
    } | ConvertTo-Json)
    $accessToken = [string]$login.access_token
    $headers = @{ Authorization = "Bearer $accessToken" }
    $platformCluster = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/clusters" -Headers $headers -ContentType 'application/json' -Body (@{
        name = $KindClusterName
        kubeconfig = $platformKubeconfig
    } | ConvertTo-Json)
    $clusterID = [int64]$platformCluster.id
    Invoke-WebRequest -UseBasicParsing -Method Patch -Uri "$ApiBase/api/v1/clusters/$clusterID" -Headers $headers -ContentType 'application/json' -Body '{"enabled":true}' | Out-Null
    $probe = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/clusters/$clusterID/probe" -Headers $headers
    Assert-Equal $probe.status 'ready' 'temporary cluster probe did not become ready'

    $resourceBase = "$ApiBase/api/v1/clusters/$clusterID"
    $nodes = Invoke-RestMethod -Uri "$resourceBase/nodes?name=synthetic-not-ready&limit=50" -Headers $headers
    $deployments = Invoke-RestMethod -Uri "$resourceBase/deployments?namespace=aiops-diagnosis-e2e&name=stalled-deployment&limit=50" -Headers $headers
    if (@($nodes.items | Where-Object { $_.metadata.name -eq 'synthetic-not-ready' }).Count -ne 1) {
        throw 'platform resource API did not return synthetic-not-ready'
    }
    if (@($deployments.items | Where-Object { $_.metadata.name -eq 'stalled-deployment' }).Count -ne 1) {
        throw 'platform resource API did not return stalled-deployment'
    }

    $nodeDiagnosis = Invoke-RestMethod -Method Post -Uri "$resourceBase/diagnoses" -Headers $headers -ContentType 'application/json' -Body (@{
        resource_kind = 'Node'
        namespace = ''
        name = 'synthetic-not-ready'
    } | ConvertTo-Json)
    $deploymentDiagnosis = Invoke-RestMethod -Method Post -Uri "$resourceBase/diagnoses" -Headers $headers -ContentType 'application/json' -Body (@{
        resource_kind = 'Deployment'
        namespace = 'aiops-diagnosis-e2e'
        name = 'stalled-deployment'
    } | ConvertTo-Json)
    Assert-Equal $nodeDiagnosis.rule_id 'node.not_ready.v1' 'Node diagnosis rule mismatch'
    Assert-Equal $deploymentDiagnosis.rule_id 'deployment.replicas_unavailable.v1' 'Deployment diagnosis rule mismatch'

    $storedNodeDiagnosis = Invoke-RestMethod -Uri "$ApiBase/api/v1/diagnoses/$($nodeDiagnosis.id)" -Headers $headers
    $storedDeploymentDiagnosis = Invoke-RestMethod -Uri "$ApiBase/api/v1/diagnoses/$($deploymentDiagnosis.id)" -Headers $headers
    $nodeEvidence = @(Get-EvidenceByType $storedNodeDiagnosis 'node_condition')
    $deploymentEvidence = @(Get-EvidenceByType $storedDeploymentDiagnosis 'deployment_status')
    if ($nodeEvidence.Count -lt 1) {
        throw 'stored Node diagnosis has no node_condition evidence'
    }
    Assert-Equal $deploymentEvidence.Count 1 'stored Deployment diagnosis evidence count mismatch'
    $deploymentContent = $deploymentEvidence[0].content
    Assert-Equal ([int]$deploymentContent.desired_replicas) 2 'stored desired replica count mismatch'
    Assert-Equal ([int]$deploymentContent.ready_replicas) 0 'stored ready replica count mismatch'
    Assert-Equal ([int]$deploymentContent.available_replicas) 0 'stored available replica count mismatch'

    $actor = 'system:serviceaccount:kube-system:aiops-platform'
    $canListNodes = Get-AuthorizationDecision @('auth', 'can-i', 'list', 'nodes', "--as=$actor")
    $canGetDeployments = Get-AuthorizationDecision @('auth', 'can-i', 'get', 'deployments', '-n', 'aiops-diagnosis-e2e', "--as=$actor")
    $canPatchDeployments = Get-AuthorizationDecision @('auth', 'can-i', 'patch', 'deployments', '-n', 'aiops-diagnosis-e2e', "--as=$actor")
    $canPatchNodes = Get-AuthorizationDecision @('auth', 'can-i', 'patch', 'nodes', "--as=$actor")
    Assert-Equal $canListNodes 'yes' 'observer cannot list Nodes'
    Assert-Equal $canGetDeployments 'yes' 'observer cannot get Deployments'
    Assert-Equal $canPatchDeployments 'no' 'observer unexpectedly can patch Deployments'
    Assert-Equal $canPatchNodes 'yes' 'observer should retain the reviewed Node patch mutation'

    $summary = [ordered]@{
        verified_at = (Get-Date).ToString('o')
        kind_version = (Invoke-NativeText -File $Kind -Arguments @('version'))
        kubernetes_version = $probe.kubernetes_version
        cluster_status = $probe.status
        fixture_state = $fixtureState
        resource_counts = [ordered]@{ nodes = $nodes.total; deployments = $deployments.total }
        diagnoses = @(
            [ordered]@{ id = $nodeDiagnosis.id; rule_id = $nodeDiagnosis.rule_id; evidence_type = 'node_condition'; evidence_count = $nodeEvidence.Count },
            [ordered]@{ id = $deploymentDiagnosis.id; rule_id = $deploymentDiagnosis.rule_id; evidence_type = 'deployment_status'; evidence_count = $deploymentEvidence.Count }
        )
        rbac = [ordered]@{
            list_nodes = $canListNodes
            get_deployments = $canGetDeployments
            patch_deployments = $canPatchDeployments
            patch_nodes = $canPatchNodes
        }
    }
} catch {
    $failure = $_
} finally {
    if ($clusterID -gt 0 -and -not [string]::IsNullOrWhiteSpace($accessToken)) {
        $cleanupHeaders = @{ Authorization = "Bearer $accessToken" }
        try {
            Invoke-WebRequest -UseBasicParsing -Method Delete -Uri "$ApiBase/api/v1/clusters/$clusterID" -Headers $cleanupHeaders | Out-Null
            $cleanup.platform_cluster_deleted = $true
            $remaining = Invoke-RestMethod -Uri "$ApiBase/api/v1/diagnoses?cluster_id=$clusterID&limit=100" -Headers $cleanupHeaders
            $cleanup.diagnosis_records_remaining = [int]$remaining.total
            if ($cleanup.diagnosis_records_remaining -ne 0) {
                throw "diagnosis cascade cleanup left $($cleanup.diagnosis_records_remaining) records"
            }
        } catch {
            $cleanupFailures.Add("platform cleanup failed: $($_.Exception.Message)")
        }
    }
    if ($kindCreated) {
        try {
            Invoke-NativeText -File $Kind -Arguments @('delete', 'cluster', '--name', $KindClusterName) | Write-Host
            $remainingKindClusters = @((Invoke-NativeText -File $Kind -Arguments @('get', 'clusters')) -split [Environment]::NewLine | Where-Object { $_ })
            $cleanup.kind_cluster_deleted = $KindClusterName -notin $remainingKindClusters
            $cleanup.preexisting_kind_clusters_preserved = (@(Compare-Object ($initialKindClusters | Sort-Object) ($remainingKindClusters | Sort-Object)).Count -eq 0)
            if (-not $cleanup.kind_cluster_deleted -or -not $cleanup.preexisting_kind_clusters_preserved) {
                throw 'kind cluster cleanup did not preserve the initial cluster set'
            }
        } catch {
            $cleanupFailures.Add("kind cleanup failed: $($_.Exception.Message)")
        }
    }
    try {
        Remove-Item -LiteralPath $TemporaryKubeconfig -Force -ErrorAction SilentlyContinue
        $cleanup.temporary_kubeconfig_deleted = -not (Test-Path -LiteralPath $TemporaryKubeconfig)
        if (-not $cleanup.temporary_kubeconfig_deleted) {
            throw 'temporary kubeconfig still exists'
        }
    } catch {
        $cleanupFailures.Add("temporary kubeconfig cleanup failed: $($_.Exception.Message)")
    }
    try {
        Remove-Item -LiteralPath $TemporaryStatusPatch -Force -ErrorAction SilentlyContinue
        $cleanup.temporary_status_patch_deleted = -not (Test-Path -LiteralPath $TemporaryStatusPatch)
        if (-not $cleanup.temporary_status_patch_deleted) {
            throw 'temporary Node status patch still exists'
        }
    } catch {
        $cleanupFailures.Add("temporary Node status patch cleanup failed: $($_.Exception.Message)")
    }
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
$path = Join-Path $ArtifactDirectory ("diagnosis-e2e-{0}.json" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
[IO.File]::WriteAllText($path, ($summary | ConvertTo-Json -Depth 12), [Text.UTF8Encoding]::new($false))
Write-Host "Node and Deployment kind verification passed. Evidence: $path"
$summary | ConvertTo-Json -Depth 12
