<# Runs M30 cordon/drain/uncordon acceptance against a disposable two-worker kind cluster. #>
[CmdletBinding()]
param([string]$ApiBase='http://127.0.0.1:8080',[string]$Username='',[string]$AdminPassword=$env:AIOPS_ADMIN_PASSWORD,[string]$KindNodeImage='kindest/node:v1.34.0',[int]$ReadyTimeoutSeconds=240)
Set-StrictMode -Version Latest
$ErrorActionPreference='Stop'; $ProgressPreference='SilentlyContinue'
$Root=Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot 'e2e-kind-common.ps1')
$Kind=Resolve-KindExecutable $Root
$RunID='{0}-{1}' -f (Get-Date -Format 'yyyyMMddHHmmss'),([guid]::NewGuid().ToString('N').Substring(0,8))
$ClusterName="m30-maint-$RunID"; $Context="kind-$ClusterName"; $PlatformName="m30-kind-$RunID"; $Namespace='m30-drain'; $Worker="$ClusterName-worker"
$ArtifactDirectory=Join-Path $Root '.artifacts\m30-node-maintenance-kind'; $ConfigPath=Join-Path $env:TEMP "m30-$RunID.yaml"
$ClusterID=0L; $Headers=$null; $Created=$false

function Invoke-Preview([string]$Action,[string]$Node) {
    return Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/clusters/$ClusterID/maintenance-plans/preview" -Headers $Headers -ContentType 'application/json' -Body (@{action=$Action;node_name=$Node}|ConvertTo-Json -Compress)
}
function Invoke-Execute($Plan,[string]$Key) {
    return Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/maintenance-plans/$($Plan.id)/execute" -Headers (@{Authorization=$Headers.Authorization;'Idempotency-Key'=$Key}) -ContentType 'application/json' -Body (@{confirmation_token=$Plan.confirmation_token}|ConvertTo-Json -Compress)
}
function Assert-ApiRejected([scriptblock]$Call) {
    try { & $Call | Out-Null; throw 'request unexpectedly succeeded' } catch { if ($_.Exception.Message -eq 'request unexpectedly succeeded') { throw } }
}

if ([string]::IsNullOrWhiteSpace($Username)){$Username=Get-AiopsRuntimeValue $Root 'BOOTSTRAP_ADMIN_USERNAME' 'admin'}
if ([string]::IsNullOrWhiteSpace($AdminPassword)){$AdminPassword=Get-AiopsRuntimeValue $Root 'BOOTSTRAP_ADMIN_PASSWORD'}
if ([string]::IsNullOrWhiteSpace($AdminPassword)){throw 'admin password is required'}

try {
    Wait-AiopsBackend $ApiBase $ReadyTimeoutSeconds; $Headers=Get-AiopsHeaders $ApiBase $Username $AdminPassword
    @"
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: worker
- role: worker
"@ | Set-Content -LiteralPath $ConfigPath -Encoding UTF8
    Invoke-NativeText $Kind @('create','cluster','--name',$ClusterName,'--image',$KindNodeImage,'--config',$ConfigPath,'--wait',"$ReadyTimeoutSeconds`s")|Out-Null; $Created=$true
    $ClusterID=Register-AiopsCluster $Root $ApiBase $Headers $Context $PlatformName
    Assert-ApiRejected { Invoke-Preview 'cordon' "$ClusterName-control-plane" }

    $emptyDirFixture=@"
apiVersion: v1
kind: Namespace
metadata: {name: $Namespace}
---
apiVersion: v1
kind: Pod
metadata: {name: emptydir-blocker, namespace: $Namespace}
spec:
  nodeName: $Worker
  containers: [{name: app, image: registry.k8s.io/pause:3.10, resources: {requests: {cpu: 10m, memory: 8Mi}, limits: {cpu: 20m, memory: 16Mi}}}]
  volumes: [{name: scratch, emptyDir: {}}]
"@
    Invoke-KubectlText -Context $Context -Arguments @('apply','-f','-') -InputText $emptyDirFixture|Out-Null
    Assert-ApiRejected { Invoke-Preview 'drain' $Worker }
    Invoke-KubectlText -Context $Context -Arguments @('-n',$Namespace,'delete','pod','emptydir-blocker','--wait=true')|Out-Null

    $managedFixture=@"
apiVersion: apps/v1
kind: Deployment
metadata: {name: app, namespace: $Namespace}
spec:
  replicas: 1
  selector: {matchLabels: {app: drain-target}}
  template:
    metadata: {labels: {app: drain-target}}
    spec:
      nodeName: $Worker
      containers: [{name: app, image: registry.k8s.io/pause:3.10, resources: {requests: {cpu: 10m, memory: 8Mi}, limits: {cpu: 20m, memory: 16Mi}}}]
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata: {name: app, namespace: $Namespace}
spec: {maxUnavailable: 0, selector: {matchLabels: {app: drain-target}}}
"@
    Invoke-KubectlText -Context $Context -Arguments @('apply','-f','-') -InputText $managedFixture|Out-Null
    Invoke-KubectlText -Context $Context -Arguments @('wait','-n',$Namespace,'--for=condition=Available','deployment/app','--timeout=120s')|Out-Null

    $cordon=Invoke-Preview 'cordon' $Worker; $cordoned=Invoke-Execute $cordon "cordon-$RunID"; $replay=Invoke-Execute $cordon "cordon-$RunID"
    Assert-Condition ($cordoned.status -eq 'succeeded' -and $replay.id -eq $cordoned.id) 'cordon or same-key replay failed'
    Assert-ApiRejected { Invoke-Preview 'drain' $Worker }
    $unblockedPDB=@"
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata: {name: app, namespace: $Namespace}
spec: {maxUnavailable: 1, selector: {matchLabels: {app: drain-target}}}
"@
    Invoke-KubectlText -Context $Context -Arguments @('apply','-f','-') -InputText $unblockedPDB|Out-Null
    Start-Sleep -Seconds 3
    $drain=Invoke-Preview 'drain' $Worker; $drained=Invoke-Execute $drain "drain-$RunID"
    Assert-Condition ($drained.status -eq 'succeeded' -and $drained.execution_result.evicted_count -eq 1) 'bounded drain did not evict exactly one managed Pod'
    $uncordon=Invoke-Preview 'uncordon' $Worker; $uncordoned=Invoke-Execute $uncordon "uncordon-$RunID"
    Assert-Condition ($uncordoned.status -eq 'succeeded') 'separate uncordon failed'
    Write-RedactedEvidence $ArtifactDirectory @{status='passed';milestone='M30';workers=2;cordon_replay=$true;evicted_count=1;emptydir_blocked=$true;pdb_blocked=$true;uncordoned=$true}
} finally {
    if($null-ne $Headers){Remove-AiopsCluster $ApiBase $Headers $ClusterID}
    if($Created){Invoke-NativeText $Kind @('delete','cluster','--name',$ClusterName) -AllowFailure|Out-Null}
    if(Test-Path -LiteralPath $ConfigPath){Remove-Item -LiteralPath $ConfigPath -Force}
}
