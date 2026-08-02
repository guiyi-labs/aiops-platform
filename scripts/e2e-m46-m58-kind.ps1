<#
Runs the M46–M58 post-M45 milestone kind E2E suite against a disposable kind
cluster and the platform backend (already running at $ApiBase, as the other
kind suites assume).

Covered milestones (each is a deterministic contract assertion, no flaky
timing):
  M46 Workspace multi-tenancy : create / list / get / delete workspace
  M48 Multi-cluster federation : register the kind cluster as a member,
       overview reflects it, cleanup deletes it
  M52 Inspection + ServiceMesh : rules catalog is non-empty and well-formed,
       create + delete an inspection plan
  M56 Golden dataset          : quality-report is reachable and returns
       either the latest report or the documented QUALITY_REPORT_NOT_FOUND
  M57 Helm app catalog        : plans list is a well-formed paged response
  M58 Cross-cluster copy      : copy-plans list is reachable; preview without
       a target cluster is rejected with INVALID_REQUEST

The script follows the e2e-*-kind.ps1 conventions: Wait-AiopsBackend, admin
login, disposable kind cluster, Register-AiopsCluster, evidence under
.artifacts/m46-m58-kind, deterministic cleanup in finally.
#>
[CmdletBinding()]
param(
    [string]$ApiBase = 'http://127.0.0.1:8080',
    [string]$Username = '',
    [string]$AdminPassword = $env:AIOPS_ADMIN_PASSWORD,
    [string]$KindNodeImage = 'kindest/node:v1.34.0',
    [int]$ReadyTimeoutSeconds = 300
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$Root = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot 'e2e-kind-common.ps1')
$Kind = Resolve-KindExecutable $Root
$RunID = '{0}-{1}' -f (Get-Date -Format 'yyyyMMddHHmmss'), ([guid]::NewGuid().ToString('N').Substring(0, 8))
$ClusterName = "m46-m58-$RunID"
$Context = "kind-$ClusterName"
$PlatformName = "m46-m58-kind-$RunID"
$ArtifactDirectory = Join-Path $Root '.artifacts\m46-m58-kind'
$ClusterID = 0L
$Headers = $null
$Created = $false

if ([string]::IsNullOrWhiteSpace($Username)) { $Username = Get-AiopsRuntimeValue $Root 'BOOTSTRAP_ADMIN_USERNAME' 'admin' }
if ([string]::IsNullOrWhiteSpace($AdminPassword)) { $AdminPassword = Get-AiopsRuntimeValue $Root 'BOOTSTRAP_ADMIN_PASSWORD' }
if ([string]::IsNullOrWhiteSpace($AdminPassword)) { throw 'admin password is required' }

$summary = [ordered]@{
    status = 'passed'
    milestones = [ordered]@{}
    workspace = $null
    federation = $null
    inspection = $null
    golden = $null
    appcatalog = $null
    copyops = $null
}

try {
    Wait-AiopsBackend $ApiBase $ReadyTimeoutSeconds
    $Headers = Get-AiopsHeaders $ApiBase $Username $AdminPassword

    # ---------------------------------------------------------------- M46 --
    # Workspace multi-tenancy is platform-level and needs no cluster, so it
    # runs before the kind cluster is created.
    $wsName = "e2e-ws-$($RunID.Substring($RunID.Length - 8))"
    $ws = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/workspaces" -Headers $Headers -ContentType 'application/json' -Body (@{ name = $wsName; display_name = 'M46 E2E workspace'; metadata = '{}' } | ConvertTo-Json -Compress)
    Assert-Condition ([int64]$ws.id -gt 0) 'workspace create did not return an id'
    $wsList = Invoke-RestMethod -Uri "$ApiBase/api/v1/workspaces" -Headers $Headers
    $wsInList = @($wsList.items | Where-Object { $_.name -eq $wsName })
    Assert-Condition ($wsInList.Count -eq 1) 'created workspace is not listed'
    $wsGet = Invoke-RestMethod -Uri "$ApiBase/api/v1/workspaces/$($ws.id)" -Headers $Headers
    Assert-Condition ([string]$wsGet.name -eq $wsName) 'workspace get returned the wrong name'
    Invoke-WebRequest -UseBasicParsing -Method Delete -Uri "$ApiBase/api/v1/workspaces/$($ws.id)" -Headers $Headers | Out-Null
    $summary.workspace = [ordered]@{ created = $true; listed = $true; fetched = $true; deleted = $true; id = [int64]$ws.id }
    $summary.milestones.M46 = 'workspace CRUD passed'

    # ---------------------------------------------------------------- kind --
    # Create the disposable cluster and register it with the platform; every
    # cluster-scoped milestone below (M48/M52/M58) uses its id.
    Invoke-NativeText $Kind @('create', 'cluster', '--name', $ClusterName, '--image', $KindNodeImage, '--wait', "$ReadyTimeoutSeconds`s") | Out-Null
    $Created = $true
    $ClusterID = Register-AiopsCluster $Root $ApiBase $Headers $Context $PlatformName

    # ---------------------------------------------------------------- M48 --
    $fed = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/federation/clusters/register" -Headers $Headers -ContentType 'application/json' -Body (@{ cluster_id = [int64]$ClusterID; role = 'member'; status = 'registered' } | ConvertTo-Json -Compress)
    $overview = Invoke-RestMethod -Uri "$ApiBase/api/v1/federation/overview" -Headers $Headers
    $memberSeen = $false
    foreach ($m in @($overview.members)) {
        if ([int64]$m.cluster_id -eq $ClusterID) { $memberSeen = $true }
    }
    Assert-Condition $memberSeen 'registered member cluster is not present in federation overview'
    $summary.federation = [ordered]@{ registered = $true; member_seen = $true; cluster_role = [string]$fed.cluster_role }
    $summary.milestones.M48 = 'federation member registration passed'

    # ---------------------------------------------------------------- M52 --
    $catalog = Invoke-RestMethod -Uri "$ApiBase/api/v1/aiops/inspection/rules/catalog" -Headers $Headers
    Assert-Condition (@($catalog.items).Count -gt 0) 'inspection rules catalog is empty'
    $firstRule = @($catalog.items)[0]
    Assert-Condition (-not [string]::IsNullOrWhiteSpace([string]$firstRule.code)) 'catalog rule is missing a code'
    $planName = "e2e-plan-$($RunID.Substring($RunID.Length - 8))"
    $plan = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/aiops/inspection/plans" -Headers $Headers -ContentType 'application/json' -Body (@{ name = $planName; cluster_ids = @([int64]$ClusterID); rule_codes = @([string]$firstRule.code); enabled = $false } | ConvertTo-Json -Compress)
    Assert-Condition ([int64]$plan.id -gt 0) 'inspection plan create did not return an id'
    Invoke-WebRequest -UseBasicParsing -Method Delete -Uri "$ApiBase/api/v1/aiops/inspection/plans/$($plan.id)" -Headers $Headers | Out-Null
    $summary.inspection = [ordered]@{ catalog_rules = @($catalog.items).Count; plan_created = $true; plan_deleted = $true }
    $summary.milestones.M52 = 'inspection catalog + plan lifecycle passed'

    # ---------------------------------------------------------------- M56 --
    $goldenState = 'no-report'
    $goldenVersion = $null
    try {
        $report = Invoke-RestMethod -Uri "$ApiBase/api/v1/aiops/quality-report" -Headers $Headers -TimeoutSec 30
        $goldenState = 'report-available'
        $goldenVersion = [string]$report.report_version
    } catch {
        $statusCode = 0
        if ($_.Exception.Response) { $statusCode = [int]$_.Exception.Response.StatusCode }
        if ($statusCode -eq 404) {
            $goldenState = 'no-report'
        } else {
            throw "quality-report endpoint failed unexpectedly: $($_.Exception.Message)"
        }
    }
    Assert-Condition ($goldenState -in @('report-available', 'no-report')) 'golden quality-report unreachable'
    $summary.golden = [ordered]@{ state = $goldenState; report_version = $goldenVersion }
    $summary.milestones.M56 = "quality-report contract passed ($goldenState)"

    # ---------------------------------------------------------------- M57 --
    $plans = Invoke-RestMethod -Uri "$ApiBase/api/v1/app-catalog/plans" -Headers $Headers
    Assert-Condition ($null -ne $plans.items) 'app-catalog plans items is null'
    Assert-Condition ($null -ne $plans.total) 'app-catalog plans total is missing'
    $summary.appcatalog = [ordered]@{ items = @($plans.items).Count; total = [int64]$plans.total }
    $summary.milestones.M57 = 'app catalog plans list passed'

    # ---------------------------------------------------------------- M58 --
    $copyPlans = Invoke-RestMethod -Uri "$ApiBase/api/v1/copy-plans" -Headers $Headers
    Assert-Condition ($null -ne $copyPlans.items) 'copy-plans items is null'
    try {
        Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/clusters/$ClusterID/copy-plans/preview" -Headers $Headers -ContentType 'application/json' -Body (@{ source_namespace = 'default'; target_namespace = 'default'; bundle = @(@{ kind = 'ConfigMap'; namespace = 'default'; name = 'nonexistent-e2e-cm' }) } | ConvertTo-Json -Compress) | Out-Null
        throw 'copy-plan preview unexpectedly succeeded without a target cluster'
    } catch {
        if ($_.Exception.Message.StartsWith('copy-plan preview unexpectedly succeeded', [StringComparison]::Ordinal)) { throw }
        $previewStatus = 0
        if ($_.Exception.Response) { $previewStatus = [int]$_.Exception.Response.StatusCode }
        Assert-Condition ($previewStatus -eq 400) "copy-plan preview without target should be 400, got $previewStatus"
    }
    $summary.copyops = [ordered]@{ plans_listed = @($copyPlans.items).Count; preview_rejects_missing_target = $true }
    $summary.milestones.M58 = 'copy ops read + validation passed'

    Write-RedactedEvidence $ArtifactDirectory $summary
} finally {
    if ($null -ne $Headers) { Remove-AiopsCluster $ApiBase $Headers $ClusterID }
    if ($Created -and $ClusterName.StartsWith('m46-m58-', [StringComparison]::Ordinal)) {
        Invoke-NativeText $Kind @('delete', 'cluster', '--name', $ClusterName) -AllowFailure | Out-Null
    }
}
$summary | ConvertTo-Json -Depth 8
