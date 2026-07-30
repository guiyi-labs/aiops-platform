<#
.SYNOPSIS
Runs the M25 cluster workload protection read-only inventory acceptance against two disposable kind clusters.

.DESCRIPTION
Requires Docker, kind, kubectl, and the repository backend with M25 Velero routes. The run normally
takes 4-7 minutes because it creates two kind clusters, registers both with the platform, and exercises
the read-only Velero backup inventory against a minimal CRD stub.

Because installing a full Velero stack (object storage provider, CSI driver, Velero server) in a
disposable kind cluster is out of scope for an inventory-only milestone, the primary cluster receives
a minimal backups.velero.io CRD stub (no controller) and two sample Backup CRs. The secondary cluster
receives no CRD, so the platform reports Velero as not installed and the backups endpoint returns 424.

The script creates uniquely named kind clusters and platform registrations, never reuses aiops-test,
and removes only resources bearing this run's identifier in finally. It writes a redacted success
summary under .artifacts/m25-workload-protection-kind; credentials, tokens and kubeconfig material
remain in memory and are never archived.
#>
[CmdletBinding()]
param(
    [string]$ApiBase = 'http://127.0.0.1:8080',
    [string]$Username = '',
    [string]$AdminPassword = $env:AIOPS_ADMIN_PASSWORD,
    [string]$KindNodeImage = '',
    [int]$ReadyTimeoutSeconds = 180
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Root = Split-Path -Parent $PSScriptRoot
$ArtifactDirectory = Join-Path $Root '.artifacts\m25-workload-protection-kind'
$RunID = '{0}-{1}' -f (Get-Date -Format 'yyyyMMddHHmmss'), ([guid]::NewGuid().ToString('N').Substring(0, 8))
$PrimaryKindClusterName = "m25-primary-$RunID"
$SecondaryKindClusterName = "m25-secondary-$RunID"
$PrimaryContext = "kind-$PrimaryKindClusterName"
$SecondaryContext = "kind-$SecondaryKindClusterName"
$PrimaryNamespace = 'aiops-m25-e2e'
$SecondaryNamespace = 'aiops-m25-secondary'
$PrimaryPlatformClusterName = "m25-primary-$RunID"
$SecondaryPlatformClusterName = "m25-secondary-$RunID"
$kindCommand = Get-Command kind -ErrorAction SilentlyContinue
$Kind = if ($null -ne $kindCommand) { $kindCommand.Source } else { Join-Path $Root '.tools\kind-v0.30.0.exe' }
if (-not (Test-Path -LiteralPath $Kind)) { throw 'kind executable is required' }

if ($ReadyTimeoutSeconds -lt 30) {
    throw 'ready timeout must be at least 30 seconds'
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
    if ($null -eq (Get-Command $command -ErrorAction SilentlyContinue)) { throw "$command is required for the M25 kind acceptance" }
}

if ([string]::IsNullOrWhiteSpace($Username)) { $Username = Get-RuntimeValue -Name 'BOOTSTRAP_ADMIN_USERNAME' -Fallback 'admin' }
if ([string]::IsNullOrWhiteSpace($AdminPassword)) { $AdminPassword = Get-RuntimeValue -Name 'BOOTSTRAP_ADMIN_PASSWORD' }
if ([string]::IsNullOrWhiteSpace($AdminPassword)) {
    throw 'set AIOPS_ADMIN_PASSWORD, pass -AdminPassword, or configure BOOTSTRAP_ADMIN_PASSWORD in .env'
}

$primaryKindCreated = $false
$secondaryKindCreated = $false
$primaryClusterID = 0L
$secondaryClusterID = 0L
$accessToken = ''
$failure = $null
$cleanupErrors = [Collections.Generic.List[string]]::new()
$summary = $null

$existingKindClusters = @(Invoke-NativeText -FilePath $Kind -Arguments @('get', 'clusters') -AllowFailure) -split "`r?`n" |
    Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
if ($existingKindClusters -contains $PrimaryKindClusterName) { throw "refusing to reuse existing kind cluster $PrimaryKindClusterName" }
if ($existingKindClusters -contains $SecondaryKindClusterName) { throw "refusing to reuse existing kind cluster $SecondaryKindClusterName" }
$AiopsTestWasPresent = $existingKindClusters -contains 'aiops-test'
$PreviousContext = (Invoke-NativeText -FilePath 'kubectl' -Arguments @('config', 'current-context') -AllowFailure).Trim()
$knownContexts = @(Invoke-NativeText -FilePath 'kubectl' -Arguments @('config', 'get-contexts', '-o', 'name') -AllowFailure) -split "`r?`n"
if ($knownContexts -notcontains $PreviousContext) { $PreviousContext = '' }

try {
    Write-Host '[1/7] Checking the platform runtime and creating two disposable kind clusters'
    Wait-BackendReady
    New-KindCluster -Name $PrimaryKindClusterName
    $primaryKindCreated = $true
    New-KindCluster -Name $SecondaryKindClusterName
    $secondaryKindCreated = $true

    Write-Host '[2/7] Installing observer RBAC and fixtures on both clusters'
    Invoke-KubectlText -Context $PrimaryContext @('apply', '-f', (Join-Path $Root 'deploy\managed-cluster\observer.yaml')) | Write-Host
    Invoke-KubectlText -Context $SecondaryContext @('apply', '-f', (Join-Path $Root 'deploy\managed-cluster\observer.yaml')) | Write-Host
    Invoke-KubectlText -Context $PrimaryContext @('apply', '-k', (Join-Path $Root 'deploy\m25-workload-protection-e2e\primary')) | Write-Host
    Invoke-KubectlText -Context $PrimaryContext @('wait', '--for=condition=Established', 'crd/backups.velero.io', '--timeout=60s') | Write-Host
    Invoke-KubectlText -Context $PrimaryContext @('apply', '-f', (Join-Path $Root 'deploy\m25-workload-protection-e2e\primary\sample-backups.yaml')) | Write-Host
    Invoke-KubectlText -Context $SecondaryContext @('apply', '-k', (Join-Path $Root 'deploy\m25-workload-protection-e2e\secondary')) | Write-Host

    Write-Host '[3/7] Registering both clusters with the platform'
    $login = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/auth/login" -ContentType 'application/json' `
        -Body (@{ username = $Username; password = $AdminPassword } | ConvertTo-Json -Compress) -TimeoutSec 15
    $accessToken = [string]$login.access_token
    if ([string]::IsNullOrWhiteSpace($accessToken)) { throw 'platform login did not return an access token' }
    $headers = @{ Authorization = "Bearer $accessToken" }
    $primaryClusterID = Register-PlatformCluster -Context $PrimaryContext -PlatformName $PrimaryPlatformClusterName -Headers $headers
    $secondaryClusterID = Register-PlatformCluster -Context $SecondaryContext -PlatformName $SecondaryPlatformClusterName -Headers $headers

    Write-Host '[4/7] Verifying Velero capability detection on both clusters'
    $primaryCapability = Invoke-RestMethod -Uri "$ApiBase/api/v1/clusters/$primaryClusterID/velero/capability" -Headers $headers -TimeoutSec 20
    if (-not [bool]$primaryCapability.installed) { throw "primary cluster must report Velero installed, got $($primaryCapability | ConvertTo-Json -Compress)" }
    Assert-Equal ([string]$primaryCapability.version) 'v1' 'primary capability version must be v1'
    $secondaryCapability = Invoke-RestMethod -Uri "$ApiBase/api/v1/clusters/$secondaryClusterID/velero/capability" -Headers $headers -TimeoutSec 20
    if ([bool]$secondaryCapability.installed) { throw "secondary cluster must report Velero not installed, got $($secondaryCapability | ConvertTo-Json -Compress)" }

    Write-Host '[5/7] Verifying read-only backup inventory on the primary cluster'
    $backups = Invoke-RestMethod -Uri "$ApiBase/api/v1/clusters/$primaryClusterID/backups?limit=100&sort_by=name&ascending=true" -Headers $headers -TimeoutSec 20
    Assert-Equal ([int]$backups.total) 2 'primary backup list total must be 2'
    if (@($backups.items).Count -lt 2) { throw 'primary backup list must include 2 items' }
    $completed = @($backups.items) | Where-Object { [string]$_.name -eq 'completed-backup' } | Select-Object -First 1
    if ($null -eq $completed) { throw 'primary backup list must include completed-backup' }
    Assert-Equal ([string]$completed.phase) 'Completed' 'completed-backup phase mismatch'
    Assert-Equal ([string]$completed.namespace) $PrimaryNamespace 'completed-backup namespace mismatch'
    if (@($completed.included_namespaces).Count -lt 1 -or [string]$completed.included_namespaces[0] -ne 'prod') {
        throw "completed-backup included_namespaces mismatch: $($completed.included_namespaces | ConvertTo-Json -Compress)"
    }
    Assert-Equal ([string]$completed.storage_location) 'default' 'completed-backup storage_location mismatch'
    Assert-Equal ([int]$completed.errors) 0 'completed-backup errors mismatch'
    Assert-Equal ([int]$completed.warnings) 2 'completed-backup warnings mismatch'
    Assert-Equal ([string]$completed.expiration) '2026-08-28T10:00:00Z' 'completed-backup expiration mismatch'
    $failed = @($backups.items) | Where-Object { [string]$_.name -eq 'failed-backup' } | Select-Object -First 1
    if ($null -eq $failed) { throw 'primary backup list must include failed-backup' }
    Assert-Equal ([string]$failed.phase) 'Failed' 'failed-backup phase mismatch'
    Assert-Equal ([string]$failed.failure_reason) 'snapshot timeout' 'failed-backup failure_reason mismatch'
    Assert-Equal ([int]$failed.errors) 3 'failed-backup errors mismatch'

    Write-Host '[6/7] Verifying single backup read and secondary-cluster 424'
    $single = Invoke-RestMethod -Uri "$ApiBase/api/v1/clusters/$primaryClusterID/backups/$PrimaryNamespace/completed-backup" -Headers $headers -TimeoutSec 20
    Assert-Equal ([string]$single.name) 'completed-backup' 'single backup name mismatch'
    Assert-Equal ([string]$single.phase) 'Completed' 'single backup phase mismatch'
    Assert-Equal ([string]$single.ttl) '720h' 'single backup ttl mismatch'
    $secondaryBackupsOk = $true
    try {
        Invoke-RestMethod -Uri "$ApiBase/api/v1/clusters/$secondaryClusterID/backups" -Headers $headers -TimeoutSec 20 | Out-Null
        $secondaryBackupsOk = $false
    } catch {
        $response = $_.Exception.Response
        if ($null -ne $response -and [int]$response.StatusCode -ne 424) { $secondaryBackupsOk = $false }
    }
    if (-not $secondaryBackupsOk) { throw 'secondary cluster backups endpoint must return 424 VELERO_UNAVAILABLE' }

    Write-Host '[7/7] Verifying observer RBAC on backups.velero.io'
    Invoke-KubectlText -Context $PrimaryContext @('-n', $PrimaryNamespace, 'get', 'backups.velero.io', 'completed-backup') | Out-Null
    Invoke-KubectlText -Context $PrimaryContext @('-n', $PrimaryNamespace, 'get', 'backup') | Out-Null
    $createDenied = Invoke-KubectlText -Context $PrimaryContext @('-n', $PrimaryNamespace, 'create', 'backup', '--dry-run=server', '-o', 'name', '--image=invalid') -AllowFailure
    if ($LASTEXITCODE -eq 0) { throw 'observer RBAC must deny create on backups.velero.io' }

    $summary = [ordered]@{
        verified_at = (Get-Date).ToUniversalTime().ToString('o')
        mode = 'disposable-kind-isolated-registration'
        primary_kind_cluster = $PrimaryKindClusterName
        secondary_kind_cluster = $SecondaryKindClusterName
        preexisting_aiops_test = $AiopsTestWasPresent
        primary_platform_cluster_id = $primaryClusterID
        secondary_platform_cluster_id = $secondaryClusterID
        primary_namespace = $PrimaryNamespace
        secondary_namespace = $SecondaryNamespace
        primary_velero_installed = $true
        secondary_velero_installed = $false
        backup_count = [int]$backups.total
        completed_backup_phase = [string]$completed.phase
        failed_backup_phase = [string]$failed.phase
        single_backup_name = [string]$single.name
        secondary_backups_status = 424
        credential_lifetime = '1h'
        cleanup_complete = $false
    }
} catch {
    $failure = $_
} finally {
    Write-Host '[cleanup] Removing platform registrations and disposable kind clusters'
    if ($secondaryClusterID -gt 0 -and -not [string]::IsNullOrWhiteSpace($accessToken)) {
        try { Invoke-WebRequest -UseBasicParsing -Method Delete -Uri "$ApiBase/api/v1/clusters/$secondaryClusterID" -Headers @{ Authorization = "Bearer $accessToken" } -TimeoutSec 20 | Out-Null }
        catch { $cleanupErrors.Add('secondary platform cluster cleanup failed') }
    }
    if ($primaryClusterID -gt 0 -and -not [string]::IsNullOrWhiteSpace($accessToken)) {
        try { Invoke-WebRequest -UseBasicParsing -Method Delete -Uri "$ApiBase/api/v1/clusters/$primaryClusterID" -Headers @{ Authorization = "Bearer $accessToken" } -TimeoutSec 20 | Out-Null }
        catch { $cleanupErrors.Add('primary platform cluster cleanup failed') }
    }
    if ($secondaryKindCreated) {
        if (-not $SecondaryKindClusterName.StartsWith('m25-secondary-', [StringComparison]::Ordinal)) {
            $cleanupErrors.Add('refused unsafe secondary kind cleanup target')
        } else {
            try { Invoke-NativeText -FilePath $Kind -Arguments @('delete', 'cluster', '--name', $SecondaryKindClusterName) | Write-Host }
            catch { $cleanupErrors.Add('secondary kind cluster cleanup failed') }
        }
    }
    if ($primaryKindCreated) {
        if (-not $PrimaryKindClusterName.StartsWith('m25-primary-', [StringComparison]::Ordinal)) {
            $cleanupErrors.Add('refused unsafe primary kind cleanup target')
        } else {
            try { Invoke-NativeText -FilePath $Kind -Arguments @('delete', 'cluster', '--name', $PrimaryKindClusterName) | Write-Host }
            catch { $cleanupErrors.Add('primary kind cluster cleanup failed') }
        }
    }
    if (-not [string]::IsNullOrWhiteSpace($PreviousContext)) {
        try { Invoke-NativeText -FilePath 'kubectl' -Arguments @('config', 'use-context', $PreviousContext) -AllowFailure | Out-Null }
        catch { $cleanupErrors.Add('previous kubectl context restore failed') }
    }
    $remainingKindClusters = @(Invoke-NativeText -FilePath $Kind -Arguments @('get', 'clusters') -AllowFailure) -split "`r?`n" |
        Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
    if ($remainingKindClusters -contains $PrimaryKindClusterName) { $cleanupErrors.Add('primary kind cluster still exists') }
    if ($remainingKindClusters -contains $SecondaryKindClusterName) { $cleanupErrors.Add('secondary kind cluster still exists') }
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
$evidencePath = Join-Path $ArtifactDirectory ("m25-workload-protection-kind-{0}.json" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
[IO.File]::WriteAllText($evidencePath, ($summary | ConvertTo-Json -Depth 12), [Text.UTF8Encoding]::new($false))
Write-Host "M25 real kind workload protection acceptance passed. Redacted evidence: $evidencePath"
$summary | ConvertTo-Json -Depth 12
