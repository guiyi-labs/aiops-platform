<# Runs M29 fixed-risk posture acceptance against a disposable real kind cluster. #>
[CmdletBinding()]
param(
    [string]$ApiBase = 'http://127.0.0.1:8080',
    [string]$Username = '',
    [string]$AdminPassword = $env:AIOPS_ADMIN_PASSWORD,
    [string]$KindNodeImage = 'kindest/node:v1.34.0',
    [int]$ReadyTimeoutSeconds = 180
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$Root = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot 'e2e-kind-common.ps1')
$Kind = Resolve-KindExecutable $Root
$RunID = '{0}-{1}' -f (Get-Date -Format 'yyyyMMddHHmmss'), ([guid]::NewGuid().ToString('N').Substring(0, 8))
$ClusterName = "m29-posture-$RunID"
$Context = "kind-$ClusterName"
$PlatformName = "m29-kind-$RunID"
$Namespace = 'm29-risk'
$ArtifactDirectory = Join-Path $Root '.artifacts\m29-governance-posture-kind'
$ClusterID = 0L
$Headers = $null
$Created = $false

if ([string]::IsNullOrWhiteSpace($Username)) { $Username = Get-AiopsRuntimeValue $Root 'BOOTSTRAP_ADMIN_USERNAME' 'admin' }
if ([string]::IsNullOrWhiteSpace($AdminPassword)) { $AdminPassword = Get-AiopsRuntimeValue $Root 'BOOTSTRAP_ADMIN_PASSWORD' }
if ([string]::IsNullOrWhiteSpace($AdminPassword)) { throw 'admin password is required' }

try {
    Wait-AiopsBackend $ApiBase $ReadyTimeoutSeconds
    $Headers = Get-AiopsHeaders $ApiBase $Username $AdminPassword
    Invoke-NativeText $Kind @('create', 'cluster', '--name', $ClusterName, '--image', $KindNodeImage, '--wait', "$ReadyTimeoutSeconds`s") | Out-Null
    $Created = $true

    $fixture = @"
apiVersion: v1
kind: Namespace
metadata: {name: $Namespace}
---
apiVersion: v1
kind: ResourceQuota
metadata: {name: exhausted, namespace: $Namespace}
spec: {hard: {pods: "1"}}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: best-effort, namespace: $Namespace}
spec:
  replicas: 1
  selector: {matchLabels: {app: best-effort}}
  template:
    metadata: {labels: {app: best-effort}}
    spec:
      containers:
      - {name: app, image: registry.k8s.io/pause:3.10}
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata: {name: blocked, namespace: $Namespace}
spec:
  minAvailable: 1
  selector: {matchLabels: {app: best-effort}}
"@
    Invoke-KubectlText -Context $Context -Arguments @('apply', '-f', '-') -InputText $fixture | Out-Null
    Invoke-KubectlText -Context $Context -Arguments @('wait', '-n', $Namespace, '--for=condition=Available', 'deployment/best-effort', '--timeout=120s') | Out-Null
    Invoke-KubectlText -Context $Context -Arguments @('cordon', "$ClusterName-control-plane") | Out-Null
    $ClusterID = Register-AiopsCluster $Root $ApiBase $Headers $Context $PlatformName

    $posture = Invoke-RestMethod -Uri "$ApiBase/api/v1/clusters/$ClusterID/namespace-postures/$Namespace" -Headers $Headers
    $codes = @($posture.findings | ForEach-Object { [string]$_.code })
    foreach ($required in @('EXHAUSTED_QUOTA', 'MISSING_LIMIT_RANGE_DEFAULTS', 'MISSING_CONTAINER_REQUESTS', 'MISSING_CONTAINER_LIMITS', 'BEST_EFFORT_WORKLOAD', 'BLOCKED_PDB_DISRUPTIONS', 'NODE_UNSCHEDULABLE')) {
        Assert-Condition ($codes -contains $required) "missing M29 finding $required"
    }
    Assert-Condition ($posture.overall_state -eq 'critical') 'risk fixture must produce critical posture'
    Write-RedactedEvidence $ArtifactDirectory @{status='passed'; milestone='M29'; cluster_count=1; namespace=$Namespace; finding_codes=@($codes | Sort-Object -Unique); overall_state=$posture.overall_state}
} finally {
    if ($null -ne $Headers) { Remove-AiopsCluster $ApiBase $Headers $ClusterID }
    if ($Created) { Invoke-NativeText $Kind @('delete', 'cluster', '--name', $ClusterName) -AllowFailure | Out-Null }
}
