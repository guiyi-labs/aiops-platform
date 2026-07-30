<# Runs M28 controlled Backup creation against pinned Velero and disposable MinIO. #>
[CmdletBinding()]
param([string]$ApiBase='http://127.0.0.1:8080',[string]$Username='',[string]$AdminPassword=$env:AIOPS_ADMIN_PASSWORD,[string]$KindNodeImage='kindest/node:v1.34.0',[int]$ReadyTimeoutSeconds=300)
Set-StrictMode -Version Latest
$ErrorActionPreference='Stop'; $ProgressPreference='SilentlyContinue'
$Root=Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot 'e2e-kind-common.ps1')
$Kind=Resolve-KindExecutable $Root
$RunID='{0}-{1}' -f (Get-Date -Format 'yyyyMMddHHmmss'),([guid]::NewGuid().ToString('N').Substring(0,8))
$ClusterName="m28-backup-$RunID"; $Context="kind-$ClusterName"; $PlatformName="m28-kind-$RunID"; $Namespace='m28-source'
$ArtifactDirectory=Join-Path $Root '.artifacts\m28-backup-creation-kind'
$ClusterID=0L; $Headers=$null; $Created=$false

function Invoke-BackupPreview {
    param([hashtable]$Body)
    Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/clusters/$ClusterID/backup-plans/preview" -Headers $Headers -ContentType 'application/json' -Body ($Body|ConvertTo-Json -Compress)
}
function Invoke-BackupExecute {
    param($Plan,[string]$Key)
    Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/backup-plans/$($Plan.id)/execute" -Headers @{Authorization=$Headers.Authorization;'Idempotency-Key'=$Key} -ContentType 'application/json' -Body (@{confirmation_token=$Plan.confirmation_token}|ConvertTo-Json -Compress)
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
metadata: {name: $Namespace}
---
apiVersion: v1
kind: ConfigMap
metadata: {name: application-config, namespace: $Namespace}
data: {mode: acceptance}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: application, namespace: $Namespace}
spec:
  replicas: 1
  selector: {matchLabels: {app: application}}
  template:
    metadata: {labels: {app: application}}
    spec: {containers: [{name: app, image: registry.k8s.io/pause:3.10}]}
"@
    Invoke-KubectlText -Context $Context -Arguments @('apply','-f','-') -InputText $fixture|Out-Null
    Invoke-KubectlText -Context $Context -Arguments @('wait','-n',$Namespace,'--for=condition=Available','deployment/application','--timeout=120s')|Out-Null
    $ClusterID=Register-AiopsCluster $Root $ApiBase $Headers $Context $PlatformName

    Assert-ApiStatus -ExpectedStatus 400 -Call { Invoke-BackupPreview @{source_namespace=$Namespace;storage_location='default';ttl='720h';backup_name='caller-controlled'} }
    $stale=Invoke-BackupPreview @{source_namespace=$Namespace;storage_location='default';ttl='24h'}
    Invoke-KubectlText -Context $Context -Arguments @('label','namespace',$Namespace,"aiops-stale=$RunID",'--overwrite')|Out-Null
    Assert-ApiStatus -ExpectedStatus 409 -Call { Invoke-BackupExecute $stale "m28-stale-$RunID" }

    $plan=Invoke-BackupPreview @{source_namespace=$Namespace;storage_location='default';ttl='24h'}
    $executed=Invoke-BackupExecute $plan "m28-create-$RunID"
    $replay=Invoke-BackupExecute $plan "m28-create-$RunID"
    Assert-Condition ($executed.status -eq 'succeeded' -and $replay.id -eq $executed.id) 'backup execute or same-key replay failed'
    Invoke-KubectlText -Context $Context -Arguments @('wait','-n','velero',"--for=jsonpath={.status.phase}=Completed","backup/$($executed.backup_name)",'--timeout=300s')|Out-Null
    $backup=Invoke-KubectlText -Context $Context -Arguments @('get','-n','velero','backup',$executed.backup_name,'-o','json')|ConvertFrom-Json
    Assert-Condition (@($backup.spec.includedNamespaces).Count -eq 1 -and $backup.spec.includedNamespaces[0] -eq $Namespace) 'created Backup must include exactly the source Namespace'
    Assert-Condition ($backup.spec.includeClusterResources -eq $false -and $backup.spec.snapshotVolumes -eq $false) 'created Backup must disable cluster resources and snapshots'
    Assert-Condition ($backup.spec.PSObject.Properties.Name -notcontains 'labelSelector') 'created Backup must not contain a label selector'
    Assert-Condition ([string]$backup.metadata.uid -eq [string]$executed.backup_uid) 'platform did not persist the created Backup UID'

    foreach($permission in @(@('create','backups.velero.io'),@('get','backupstoragelocations.velero.io'),@('list','backupstoragelocations.velero.io'))){
        $answer=Invoke-KubectlText -Context $Context -Arguments @('auth','can-i',$permission[0],$permission[1],'--as=system:serviceaccount:kube-system:aiops-platform')
        Assert-Condition ([bool]($answer -match '(?m)^yes\s*$')) "observer RBAC must allow $($permission -join ' '); got: $answer"
    }
    foreach($permission in @(@('delete','backups.velero.io'),@('update','backups.velero.io'))){
        $answer=Invoke-KubectlText -Context $Context -Arguments @('auth','can-i',$permission[0],$permission[1],'--as=system:serviceaccount:kube-system:aiops-platform') -AllowFailure
        Assert-Condition ([bool]($answer -match '(?m)^no\s*$')) "observer RBAC must deny $($permission -join ' '); got: $answer"
    }
    Write-RedactedEvidence $ArtifactDirectory @{status='passed';milestone='M28';velero_version=$veleroInfo.VeleroVersion;aws_plugin=$veleroInfo.AwsPluginImage;storage='disposable-minio';strict_contract=$true;stale_namespace_rejected=$true;idempotent_replay=$true;backup_phase='Completed';backup_scope='single-namespace-no-snapshot'}
} finally {
    if($null-ne $Headers){Remove-AiopsCluster $ApiBase $Headers $ClusterID}
    if($Created -and $ClusterName.StartsWith('m28-backup-',[StringComparison]::Ordinal)){Invoke-NativeText $Kind @('delete','cluster','--name',$ClusterName) -AllowFailure|Out-Null}
}
