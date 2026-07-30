<# Runs M31 isolated Restore rehearsal against pinned Velero and disposable MinIO. #>
[CmdletBinding()]
param([string]$ApiBase='http://127.0.0.1:8080',[string]$Username='',[string]$AdminPassword=$env:AIOPS_ADMIN_PASSWORD,[string]$KindNodeImage='kindest/node:v1.34.0',[int]$ReadyTimeoutSeconds=300)
Set-StrictMode -Version Latest
$ErrorActionPreference='Stop'; $ProgressPreference='SilentlyContinue'
$Root=Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot 'e2e-kind-common.ps1')
$Kind=Resolve-KindExecutable $Root
$RunID='{0}-{1}' -f (Get-Date -Format 'yyyyMMddHHmmss'),([guid]::NewGuid().ToString('N').Substring(0,8))
$ClusterName="m31-restore-$RunID"; $Context="kind-$ClusterName"; $PlatformName="m31-kind-$RunID"; $SourceNamespace='m31-source'; $BackupName="m31-source-$($RunID.Substring($RunID.Length-8))"
$ArtifactDirectory=Join-Path $Root '.artifacts\m31-isolated-restore-kind'
$ClusterID=0L; $Headers=$null; $Created=$false

function Invoke-RestorePreview {
    Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/clusters/$ClusterID/restore-plans/preview" -Headers $Headers -ContentType 'application/json' -Body (@{source_backup_name=$BackupName;source_backup_namespace='velero'}|ConvertTo-Json -Compress)
}
function Invoke-RestoreExecute {
    param($Plan,[string]$Key)
    Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/restore-plans/$($Plan.id)/execute" -Headers @{Authorization=$Headers.Authorization;'Idempotency-Key'=$Key} -ContentType 'application/json' -Body (@{confirmation_token=$Plan.confirmation_token}|ConvertTo-Json -Compress) -TimeoutSec 360
}

if([string]::IsNullOrWhiteSpace($Username)){$Username=Get-AiopsRuntimeValue $Root 'BOOTSTRAP_ADMIN_USERNAME' 'admin'}
if([string]::IsNullOrWhiteSpace($AdminPassword)){$AdminPassword=Get-AiopsRuntimeValue $Root 'BOOTSTRAP_ADMIN_PASSWORD'}
if([string]::IsNullOrWhiteSpace($AdminPassword)){throw 'admin password is required'}

try {
    Wait-AiopsBackend $ApiBase $ReadyTimeoutSeconds; $Headers=Get-AiopsHeaders $ApiBase $Username $AdminPassword
    Invoke-NativeText $Kind @('create','cluster','--name',$ClusterName,'--image',$KindNodeImage,'--wait',"$ReadyTimeoutSeconds`s")|Out-Null; $Created=$true
    $veleroInfo=Install-AiopsTestVelero -Root $Root -Context $Context -RunID $RunID -TimeoutSeconds $ReadyTimeoutSeconds
    $fixture=@"
apiVersion: v1
kind: Namespace
metadata: {name: $SourceNamespace}
---
apiVersion: v1
kind: ConfigMap
metadata: {name: restore-marker, namespace: $SourceNamespace}
data: {marker: isolated-restore-acceptance}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: restore-workload, namespace: $SourceNamespace}
spec:
  replicas: 1
  selector: {matchLabels: {app: restore-workload}}
  template:
    metadata: {labels: {app: restore-workload}}
    spec: {containers: [{name: app, image: registry.k8s.io/pause:3.10}]}
---
apiVersion: velero.io/v1
kind: Backup
metadata: {name: $BackupName, namespace: velero}
spec:
  includedNamespaces: [$SourceNamespace]
  includeClusterResources: false
  snapshotVolumes: false
  storageLocation: default
  ttl: 24h
"@
    Invoke-KubectlText -Context $Context -Arguments @('apply','-f','-') -InputText $fixture|Out-Null
    Invoke-KubectlText -Context $Context -Arguments @('wait','-n',$SourceNamespace,'--for=condition=Available','deployment/restore-workload','--timeout=120s')|Out-Null
    Invoke-KubectlText -Context $Context -Arguments @('wait','-n','velero',"--for=jsonpath={.status.phase}=Completed","backup/$BackupName",'--timeout=300s')|Out-Null
    $ClusterID=Register-AiopsCluster $Root $ApiBase $Headers $Context $PlatformName

    $stale=Invoke-RestorePreview
    Invoke-KubectlText -Context $Context -Arguments @('annotate','-n','velero','backup',$BackupName,"aiops-stale=$RunID",'--overwrite')|Out-Null
    Assert-ApiStatus -ExpectedStatus 409 -Call { Invoke-RestoreExecute $stale "m31-stale-$RunID" }
    $plan=Invoke-RestorePreview
    $executed=Invoke-RestoreExecute $plan "m31-restore-$RunID"
    $replay=Invoke-RestoreExecute $plan "m31-restore-$RunID"
    Assert-Condition ($executed.status -eq 'succeeded' -and $replay.id -eq $executed.id) 'restore execute or same-key replay failed'
    Assert-Condition ($executed.execution_result.restore_phase -eq 'Completed' -and $executed.execution_result.quarantine_established) 'restore did not complete inside established quarantine'
    $destination=[string]$executed.destination_namespace
    $restoreObject=Invoke-KubectlText -Context $Context -Arguments @('get','-n','velero','restore',$executed.velero_restore_name,'-o','json')|ConvertFrom-Json
    $mapping=[string]$restoreObject.spec.namespaceMapping.PSObject.Properties[$SourceNamespace].Value
    Assert-Condition ($mapping -eq $destination) 'Restore namespaceMapping must use the source Namespace as its key'
    Invoke-KubectlText -Context $Context -Arguments @('get','namespace',$destination)|Out-Null
    Invoke-KubectlText -Context $Context -Arguments @('get','-n',$destination,'networkpolicy','quarantine-default-deny')|Out-Null
    $quota=Invoke-KubectlText -Context $Context -Arguments @('get','-n',$destination,'resourcequota','quarantine-zero-pods','-o','json')|ConvertFrom-Json
    Assert-Condition ([string]$quota.spec.hard.pods -eq '0') 'quarantine quota must enforce pods=0'
    Invoke-KubectlText -Context $Context -Arguments @('get','-n',$destination,'configmap','restore-marker')|Out-Null
    Assert-Condition ($null-ne (Invoke-KubectlText -Context $Context -Arguments @('get','namespace',$SourceNamespace,'-o','name'))) 'source Namespace must remain intact'

    foreach($permission in @(@('create','namespaces'),@('create','networkpolicies.networking.k8s.io'),@('create','resourcequotas'),@('create','restores.velero.io'))){
        $answer=Invoke-KubectlText -Context $Context -Arguments @('auth','can-i',$permission[0],$permission[1],'--as=system:serviceaccount:kube-system:aiops-platform')
        Assert-Condition ([bool]($answer -match '(?m)^yes\s*$')) "observer RBAC must allow $($permission -join ' '); got: $answer"
    }
    foreach($permission in @(@('delete','namespaces'),@('update','restores.velero.io'),@('delete','restores.velero.io'))){
        $answer=Invoke-KubectlText -Context $Context -Arguments @('auth','can-i',$permission[0],$permission[1],'--as=system:serviceaccount:kube-system:aiops-platform') -AllowFailure
        Assert-Condition ([bool]($answer -match '(?m)^no\s*$')) "observer RBAC must deny $($permission -join ' '); got: $answer"
    }
    Write-RedactedEvidence $ArtifactDirectory @{status='passed';milestone='M31';velero_version=$veleroInfo.VeleroVersion;aws_plugin=$veleroInfo.AwsPluginImage;storage='disposable-minio';stale_backup_rejected=$true;idempotent_replay=$true;restore_phase='Completed';quarantine_established=$true;source_preserved=$true;namespace_mapping_verified=$true}
} finally {
    if($null-ne $Headers){Remove-AiopsCluster $ApiBase $Headers $ClusterID}
    if($Created -and $ClusterName.StartsWith('m31-restore-',[StringComparison]::Ordinal)){Invoke-NativeText $Kind @('delete','cluster','--name',$ClusterName) -AllowFailure|Out-Null}
}
