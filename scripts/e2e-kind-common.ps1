Set-StrictMode -Version Latest

function Get-AiopsRuntimeValue {
    param([Parameter(Mandatory)] [string]$Root, [Parameter(Mandatory)] [string]$Name, [string]$Fallback = '')
    $value = [Environment]::GetEnvironmentVariable($Name, 'Process')
    if (-not [string]::IsNullOrWhiteSpace($value)) { return $value }
    $environmentPath = Join-Path $Root '.env'
    if (Test-Path -LiteralPath $environmentPath) {
        $prefix = "$Name="
        $line = Get-Content -LiteralPath $environmentPath -Encoding UTF8 |
            Where-Object { $_.StartsWith($prefix, [StringComparison]::Ordinal) } | Select-Object -Last 1
        if ($null -ne $line) { return $line.Substring($prefix.Length) }
    }
    return $Fallback
}

function Resolve-KindExecutable {
    param([Parameter(Mandatory)] [string]$Root)
    $command = Get-Command kind -ErrorAction SilentlyContinue
    if ($null -ne $command) { return $command.Source }
    $candidate = Join-Path $Root '.tools\kind-v0.30.0.exe'
    if (Test-Path -LiteralPath $candidate) { return $candidate }
    throw 'kind executable is required'
}

function Invoke-NativeText {
    param([Parameter(Mandatory)] [string]$FilePath, [Parameter(Mandatory)] [string[]]$Arguments, [string]$InputText, [switch]$AllowFailure)
    $previous = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        if ($PSBoundParameters.ContainsKey('InputText')) { $output = $InputText | & $FilePath @Arguments 2>&1 }
        else { $output = & $FilePath @Arguments 2>&1 }
        $exitCode = $LASTEXITCODE
    } finally { $ErrorActionPreference = $previous }
    $text = (($output | ForEach-Object { if ($_ -is [Management.Automation.ErrorRecord]) { $_.Exception.Message } else { $_.ToString() } }) -join [Environment]::NewLine).Trim()
    if ($exitCode -ne 0 -and -not $AllowFailure) { throw "$FilePath $($Arguments -join ' ') failed with exit code $exitCode`: $text" }
    return $text
}

function Invoke-KubectlText {
    param(
        [Parameter(Mandatory, Position = 0)] [string]$Context,
        [Parameter(Mandatory, Position = 1, ValueFromRemainingArguments)] [string[]]$Arguments,
        [string]$InputText,
        [switch]$AllowFailure
    )
    $params = @{FilePath = 'kubectl'; Arguments = @('--context', $Context) + $Arguments; AllowFailure = $AllowFailure}
    if ($PSBoundParameters.ContainsKey('InputText')) { $params.InputText = $InputText }
    return Invoke-NativeText @params
}

function Wait-AiopsBackend {
    param([Parameter(Mandatory)] [string]$ApiBase, [int]$TimeoutSeconds = 180)
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    do {
        try {
            $ready = Invoke-RestMethod -Uri "$ApiBase/api/v1/health/ready" -TimeoutSec 5
            if ($ready.status -eq 'ready') { return }
        } catch {}
        Start-Sleep -Seconds 2
    } while ((Get-Date) -lt $deadline)
    throw 'platform backend did not become ready before the deadline'
}

function Get-AiopsHeaders {
    param([Parameter(Mandatory)] [string]$ApiBase, [Parameter(Mandatory)] [string]$Username, [Parameter(Mandatory)] [string]$Password)
    $login = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/auth/login" -ContentType 'application/json' -Body (@{username=$Username; password=$Password} | ConvertTo-Json -Compress)
    if ([string]::IsNullOrWhiteSpace([string]$login.access_token)) { throw 'platform login did not return an access token' }
    return @{Authorization = "Bearer $($login.access_token)"}
}

function Register-AiopsCluster {
    param([Parameter(Mandatory)] [string]$Root, [Parameter(Mandatory)] [string]$ApiBase, [Parameter(Mandatory)] [Collections.IDictionary]$Headers, [Parameter(Mandatory)] [string]$Context, [Parameter(Mandatory)] [string]$Name)
    Invoke-KubectlText -Context $Context -Arguments @('apply', '-f', (Join-Path $Root 'deploy\managed-cluster\observer.yaml')) | Out-Null
    $token = Invoke-KubectlText -Context $Context -Arguments @('-n', 'kube-system', 'create', 'token', 'aiops-platform', '--duration=1h')
    $raw = Invoke-KubectlText -Context $Context -Arguments @('config', 'view', '--raw', '--minify', '-o', 'json') | ConvertFrom-Json
    $serverUri = [Uri][string]$raw.clusters[0].cluster.server
    $ca = [string]$raw.clusters[0].cluster.'certificate-authority-data'
    $server = $serverUri.AbsoluteUri.TrimEnd('/')
    $targetCluster = [ordered]@{server=$server;'certificate-authority-data'=$ca}
    if ($serverUri.IsLoopback) {
        $builder = [UriBuilder]$serverUri; $builder.Host = 'host.docker.internal'; $server = $builder.Uri.AbsoluteUri.TrimEnd('/')
        $targetCluster.server = $server
        $targetCluster['tls-server-name'] = $serverUri.Host
    }
    $kubeconfig = [ordered]@{
        apiVersion='v1'
        kind='Config'
        clusters=@([ordered]@{name='target';cluster=$targetCluster})
        contexts=@([ordered]@{name='target';context=[ordered]@{cluster='target';user='aiops-platform'}})
        'current-context'='target'
        users=@([ordered]@{name='aiops-platform';user=[ordered]@{token=$token}})
    } | ConvertTo-Json -Depth 8 -Compress
    $cluster = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/clusters" -Headers $Headers -ContentType 'application/json' -Body (@{name=$Name; kubeconfig=$kubeconfig} | ConvertTo-Json -Compress)
    Invoke-RestMethod -Method Patch -Uri "$ApiBase/api/v1/clusters/$($cluster.id)" -Headers $Headers -ContentType 'application/json' -Body '{"enabled":true}' | Out-Null
    $probe = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/clusters/$($cluster.id)/probe" -Headers $Headers
    if ($probe.status -ne 'ready') { throw 'registered cluster probe did not become ready' }
    return [int64]$cluster.id
}

function Remove-AiopsCluster {
    param([string]$ApiBase, [Collections.IDictionary]$Headers, [int64]$ClusterID)
    if ($ClusterID -gt 0) {
        try { Invoke-RestMethod -Method Delete -Uri "$ApiBase/api/v1/clusters/$ClusterID" -Headers $Headers | Out-Null } catch { Write-Warning $_ }
    }
}

function Assert-Condition {
    param([Parameter(Mandatory)] [bool]$Condition, [Parameter(Mandatory)] [string]$Message)
    if (-not $Condition) { throw $Message }
}

function Write-RedactedEvidence {
    param([Parameter(Mandatory)] [string]$Directory, [Parameter(Mandatory)] [hashtable]$Summary)
    [IO.Directory]::CreateDirectory($Directory) | Out-Null
    $Summary['verified_at'] = (Get-Date).ToUniversalTime().ToString('o')
    $Summary | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath (Join-Path $Directory 'summary.json') -Encoding UTF8
}

function Get-PinnedVeleroExecutable {
    param(
        [Parameter(Mandatory)] [string]$Root,
        [string]$Version = 'v1.15.2',
        [string]$Sha256 = '1FA7C2448A5751DD3FDFD86AD9C49472D677B97237A25390E7727088ED82D668'
    )
    $cacheDirectory = Join-Path $Root ".tools\velero-$Version-windows-amd64"
    $executable = Join-Path $cacheDirectory 'velero.exe'
    if (Test-Path -LiteralPath $executable) { return $executable }

    [IO.Directory]::CreateDirectory($cacheDirectory) | Out-Null
    $archive = Join-Path $cacheDirectory "velero-$Version-windows-amd64.tar.gz"
    Invoke-WebRequest -UseBasicParsing -Uri "https://github.com/vmware-tanzu/velero/releases/download/$Version/velero-$Version-windows-amd64.tar.gz" -OutFile $archive
    $actual = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash
    if (-not $actual.Equals($Sha256, [StringComparison]::OrdinalIgnoreCase)) {
        Remove-Item -LiteralPath $archive -Force
        throw "Velero archive checksum mismatch: expected $Sha256, got $actual"
    }
    Invoke-NativeText -FilePath 'tar' -Arguments @('-xzf', $archive, '-C', $cacheDirectory, '--strip-components=1') | Out-Null
    Remove-Item -LiteralPath $archive -Force
    if (-not (Test-Path -LiteralPath $executable)) { throw 'Velero archive did not contain velero.exe' }
    return $executable
}

function Install-AiopsTestVelero {
    param(
        [Parameter(Mandatory)] [string]$Root,
        [Parameter(Mandatory)] [string]$Context,
        [Parameter(Mandatory)] [string]$RunID,
        [string]$VeleroServerImage = 'velero/velero:v1.15.2',
        [string]$AwsPluginImage = 'velero/velero-plugin-for-aws:v1.11.1',
        [string]$MinioImage = 'quay.io/minio/minio:RELEASE.2025-04-22T22-12-26Z',
        [string]$MinioClientImage = 'quay.io/minio/mc:RELEASE.2025-04-16T18-13-26Z',
        [int]$TimeoutSeconds = 300
    )
    $velero = Get-PinnedVeleroExecutable -Root $Root
    $accessKey = 'aiops'
    $secretKey = "aiops-$($RunID.Replace('-', ''))"
    $credentialsPath = Join-Path $env:TEMP "aiops-velero-$RunID.credentials"
    $fixture = @"
apiVersion: v1
kind: Namespace
metadata: {name: velero}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: minio, namespace: velero}
spec:
  replicas: 1
  selector: {matchLabels: {app: minio}}
  template:
    metadata: {labels: {app: minio}}
    spec:
      containers:
      - name: minio
        image: $MinioImage
        args: [server, /data]
        env:
        - {name: MINIO_ROOT_USER, value: '$accessKey'}
        - {name: MINIO_ROOT_PASSWORD, value: '$secretKey'}
        ports: [{name: api, containerPort: 9000}]
        readinessProbe: {httpGet: {path: /minio/health/ready, port: api}, initialDelaySeconds: 2, periodSeconds: 2}
---
apiVersion: v1
kind: Service
metadata: {name: minio, namespace: velero}
spec:
  selector: {app: minio}
  ports: [{name: api, port: 9000, targetPort: api}]
---
apiVersion: batch/v1
kind: Job
metadata: {name: create-aiops-backup-bucket, namespace: velero}
spec:
  backoffLimit: 6
  template:
    spec:
      restartPolicy: OnFailure
      containers:
      - name: mc
        image: $MinioClientImage
        command: [/bin/sh, -c]
        args: ['until mc alias set local http://minio:9000 "`$MINIO_ROOT_USER" "`$MINIO_ROOT_PASSWORD"; do sleep 2; done; mc mb --ignore-existing local/aiops-backups']
        env:
        - {name: MINIO_ROOT_USER, value: '$accessKey'}
        - {name: MINIO_ROOT_PASSWORD, value: '$secretKey'}
"@
    Invoke-KubectlText -Context $Context -Arguments @('apply', '-f', '-') -InputText $fixture | Out-Null
    Invoke-KubectlText -Context $Context -Arguments @('wait', '-n', 'velero', '--for=condition=Available', 'deployment/minio', "--timeout=$($TimeoutSeconds)s") | Out-Null
    Invoke-KubectlText -Context $Context -Arguments @('wait', '-n', 'velero', '--for=condition=Complete', 'job/create-aiops-backup-bucket', "--timeout=$($TimeoutSeconds)s") | Out-Null

    try {
        "[default]`naws_access_key_id=$accessKey`naws_secret_access_key=$secretKey`n" | Set-Content -LiteralPath $credentialsPath -Encoding ASCII
        Invoke-NativeText -FilePath $velero -Arguments @(
            'install', '--kubecontext', $Context, '--image', $VeleroServerImage, '--provider', 'aws', '--plugins', $AwsPluginImage,
            '--bucket', 'aiops-backups', '--secret-file', $credentialsPath,
            '--backup-location-config', 'region=minio,s3ForcePathStyle=true,s3Url=http://minio.velero.svc.cluster.local:9000',
            '--use-volume-snapshots=false'
        ) | Out-Null
    } finally {
        if (Test-Path -LiteralPath $credentialsPath) { Remove-Item -LiteralPath $credentialsPath -Force }
    }
    Invoke-KubectlText -Context $Context -Arguments @('wait', '-n', 'velero', '--for=condition=Available', 'deployment/velero', "--timeout=$($TimeoutSeconds)s") | Out-Null
    Invoke-KubectlText -Context $Context -Arguments @('wait', '-n', 'velero', "--for=jsonpath={.status.phase}=Available", 'backupstoragelocation/default', "--timeout=$($TimeoutSeconds)s") | Out-Null
    return @{VeleroExecutable=$velero; VeleroVersion='v1.15.2'; VeleroServerImage=$VeleroServerImage; AwsPluginImage=$AwsPluginImage; MinioImage=$MinioImage; MinioClientImage=$MinioClientImage}
}

function Assert-ApiStatus {
    param([Parameter(Mandatory)] [scriptblock]$Call, [Parameter(Mandatory)] [int]$ExpectedStatus)
    try {
        & $Call | Out-Null
        throw "request unexpectedly succeeded; expected HTTP $ExpectedStatus"
    } catch {
        if ($_.Exception.Message.StartsWith('request unexpectedly succeeded', [StringComparison]::Ordinal)) { throw }
        $response = $_.Exception.Response
        if ($null -eq $response) { throw }
        $actual = [int]$response.StatusCode
        if ($actual -ne $ExpectedStatus) { throw "expected HTTP $ExpectedStatus, got $actual`: $($_.Exception.Message)" }
    }
}
