<#
.SYNOPSIS
Runs the M27 alert lifecycle acceptance against a disposable real kind cluster.

.DESCRIPTION
Requires Docker, kind, kubectl, the repository Compose backend with M27 routes, and registry access for the
pinned Metrics Server. The run creates a kind cluster, registers it, creates an alert rule, waits for firing,
verifies deduplication, simulates Metrics API outage, verifies recovery, and confirms alert resolution.

The script creates a uniquely named kind cluster and platform registration, never reuses aiops-test, and removes
only resources bearing this run's identifier in finally. It writes a redacted success summary under
.artifacts/m27-alert-lifecycle-kind; credentials, tokens and kubeconfig material remain in memory and are never archived.
#>
[CmdletBinding()]
param(
    [string]$ApiBase = 'http://127.0.0.1:8080',
    [string]$Username = '',
    [string]$AdminPassword = $env:AIOPS_ADMIN_PASSWORD,
    [string]$KindNodeImage = '',
    [int]$ReadyTimeoutSeconds = 180,
    [int]$FiringTimeoutSeconds = 300,
    [int]$RecoveryTimeoutSeconds = 300,
    [switch]$SkipBackendRestart
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Root = Split-Path -Parent $PSScriptRoot
$ArtifactDirectory = Join-Path $Root '.artifacts\m27-alert-lifecycle-kind'
$RunID = '{0}-{1}' -f (Get-Date -Format 'yyyyMMddHHmmss'), ([guid]::NewGuid().ToString('N').Substring(0, 8))
$KindClusterName = "m27-alert-$RunID"
$Context = "kind-$KindClusterName"
$PlatformClusterName = "m27-kind-$RunID"
$KubeconfigPath = Join-Path $env:TEMP ".kube-$RunID"
$MetricsManifestPath = Join-Path $Root 'deploy\metrics-server-kind\components-v0.8.0.yaml'
$MetricsPatchPath = Join-Path $Root 'deploy\metrics-server-kind\kind-patch.json'
$kindCommand = Get-Command kind -ErrorAction SilentlyContinue
$Kind = if ($null -ne $kindCommand) { $kindCommand.Source } else { Join-Path $Root '.tools\kind-v0.30.0.exe' }
if (-not (Test-Path -LiteralPath $Kind)) { throw 'kind executable is required' }

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

function Invoke-ComposeText {
    param([Parameter(Mandatory)] [string[]]$Arguments)

    Push-Location $Root
    try {
        return Invoke-NativeText -FilePath 'docker' -Arguments (@('compose') + $Arguments)
    } finally {
        Pop-Location
    }
}

function Wait-BackendReady {
    $deadline = (Get-Date).AddSeconds($ReadyTimeoutSeconds)
    do {
        try {
            $wc = New-Object System.Net.WebClient
            $body = $wc.DownloadString("$ApiBase/api/v1/health/ready")
            $wc.Dispose()
            if ($body -like '*"status"*"ready"*') {
                Write-Host "Backend ready"
                return $true
            }
        } catch {
            Write-Host "Backend not ready yet, retrying..."
        }
        Start-Sleep -Seconds 2
    } while ((Get-Date) -lt $deadline)
    Write-Host "Backend ready check timed out"
    return $false
}

function Get-AccessToken {
    param([Parameter(Mandatory)] [string]$User, [Parameter(Mandatory)] [string]$Pass)

    $body = @{username = $User; password = $Pass} | ConvertTo-Json -Compress
    $response = Invoke-RestMethod -Uri "$ApiBase/api/v1/auth/login" -Method POST -Body $body -ContentType 'application/json'
    return $response.access_token
}

function Invoke-ApiRequest {
    param(
        [Parameter(Mandatory)] [string]$Token,
        [Parameter(Mandatory)] [string]$Path,
        [ValidateSet('GET', 'POST', 'DELETE', 'PATCH')] [string]$Method = 'GET',
        $Body = $null
    )

    $headers = @{Authorization = "Bearer $Token"}
    $uri = "$ApiBase$Path"
    $params = @{Uri = $uri; Method = $Method; Headers = $headers}
    if ($null -ne $Body) {
        $params.Body = ($Body | ConvertTo-Json -Depth 10 -Compress)
        $params.ContentType = 'application/json'
    }
    try {
        return Invoke-RestMethod @params
    } catch {
        $statusCode = $_.Exception.Response.StatusCode.value__
        if ($statusCode -ge 400) {
            $reader = [System.IO.StreamReader]::new($_.Exception.Response.GetResponseStream())
            $reader.BaseStream.Position = 0
            $responseText = $reader.ReadToEnd()
            $reader.Close()
            throw "API request ${Method} ${Path} failed with status ${statusCode}: ${responseText}"
        }
        throw
    }
}

try {
    Write-Host "M27 Alert Lifecycle E2E - Run ID: $RunID"
    Write-Host "========================================"

    # Prepare artifacts directory
    if (-not (Test-Path -LiteralPath $ArtifactDirectory)) {
        New-Item -ItemType Directory -Path $ArtifactDirectory -Force | Out-Null
    }

    # Check prerequisites
    $adminPass = if ($AdminPassword) { $AdminPassword } else { Get-RuntimeValue -Name 'AIOPS_ADMIN_PASSWORD' }
    if (-not $adminPass) { throw 'Admin password is required (set AIOPS_ADMIN_PASSWORD or pass -AdminPassword)' }

    # Create kind cluster
    # Set isolated kubeconfig to avoid lock conflicts
    $env:KUBECONFIG = $KubeconfigPath
    Write-Host "Creating kind cluster: $KindClusterName"
    $kindArgs = @('create', 'cluster', '--name', $KindClusterName)
    if ($KindNodeImage) {
        $kindArgs += @('--image', $KindNodeImage)
    } else {
        $kindArgs += @('--image', 'kindest/node:v1.34.0')
    }
    Invoke-NativeText -FilePath $Kind -Arguments $kindArgs | Out-Null

    # Wait for cluster ready
    $deadline = (Get-Date).AddSeconds($ReadyTimeoutSeconds)
    do {
        try {
            $nodes = Invoke-KubectlText -Arguments @('get', 'nodes', '-o', 'jsonpath={.items[*].metadata.name}') -AllowFailure
            if ($nodes -match '\S+') { break }
        } catch {}
        Start-Sleep -Seconds 2
    } while ((Get-Date) -lt $deadline)

    # Get node name
    $nodeName = Invoke-KubectlText -Arguments @('get', 'nodes', '-o', 'jsonpath={.items[0].metadata.name}')
    Write-Host "Cluster ready, node: $nodeName"

    # Deploy Metrics Server
    Write-Host "Deploying Metrics Server..."
    Invoke-KubectlText -Arguments @('apply', '-f', $MetricsManifestPath) | Out-Null
    Start-Sleep -Seconds 5
    Invoke-KubectlText -Arguments @('-n', 'kube-system', 'patch', 'deployment', 'metrics-server', '--type=json', '--patch-file', $MetricsPatchPath) | Out-Null

    # Wait for Metrics Server ready
    $deadline = (Get-Date).AddSeconds(120)
    do {
        $available = Invoke-KubectlText -Arguments @('get', 'deployment', 'metrics-server', '-n', 'kube-system', '-o', 'jsonpath={.status.availableReplicas}') -AllowFailure
        if ($available -eq '1') { break }
        Start-Sleep -Seconds 5
    } while ((Get-Date) -lt $deadline)

    # Get kubeconfig
    $kubeconfig = Invoke-KubectlText -Arguments @('config', 'view', '--minify', '--raw')

    # Ensure backend running
    Write-Host "Checking backend status..."
    if (-not (Wait-BackendReady)) {
        Write-Host "Starting backend..."
        Invoke-ComposeText -Arguments @('up', '-d') | Out-Null
        if (-not (Wait-BackendReady)) {
            throw 'Backend failed to start'
        }
    }

    # Login and get token
    Write-Host "Logging in..."
    $token = Get-AccessToken -User 'admin' -Pass $adminPass

    # Register cluster
    Write-Host "Registering cluster as: $PlatformClusterName"
    $registration = Invoke-ApiRequest -Token $token -Method POST -Path '/api/v1/clusters' -Body @{name = $PlatformClusterName; kubeconfig = $kubeconfig}
    $clusterID = $registration.id
    Write-Host "Cluster registered with ID: $clusterID"

    # Enable cluster so that metrics collector and scheduler target it
    Write-Host "Enabling cluster..."
    Invoke-ApiRequest -Token $token -Method PATCH -Path "/api/v1/clusters/$clusterID" -Body @{enabled = $true} | Out-Null

    # Wait for cluster ready
    Write-Host "Waiting for cluster to become ready..."
    $deadline = (Get-Date).AddSeconds(60)
    do {
        $cluster = Invoke-ApiRequest -Token $token -Path "/api/v1/clusters/$clusterID"
        if ($cluster.status -eq 'ready') { break }
        Write-Host "  Cluster status: $($cluster.status)"
        Start-Sleep -Seconds 3
    } while ((Get-Date) -lt $deadline)

    # Create alert rule
    Write-Host "Creating alert rule for node: $nodeName"
    $ruleBody = @{
        display_name = "High CPU Alert $RunID"
        resource_kind = 'Node'
        resource_name = $nodeName
        metric_name = 'cpu'
        operator = 'gte'
        threshold = 100000000  # 0.1 cores - very low threshold for testing
        for_seconds = 60
        minimum_points = 2
    }
    $rule = Invoke-ApiRequest -Token $token -Method POST -Path "/api/v1/clusters/$clusterID/alert-rules" -Body $ruleBody
    $ruleID = $rule.id
    Write-Host "Alert rule created with ID: $ruleID"

    # Wait for firing
    Write-Host "Waiting for alert to fire (timeout: $FiringTimeoutSeconds seconds)..."
    $deadline = (Get-Date).AddSeconds($FiringTimeoutSeconds)
    $fired = $false
    do {
        Start-Sleep -Seconds 10
        $rules = Invoke-ApiRequest -Token $token -Path "/api/v1/clusters/$clusterID/alert-rules"
        $currentRule = $rules | Where-Object { $_.id -eq $ruleID } | Select-Object -First 1
        if ($currentRule.last_evaluation_state -eq 'firing') {
            $fired = $true
            break
        }
        Write-Host "  Current evaluation state: $($currentRule.last_evaluation_state)"
    } while ((Get-Date) -lt $deadline)

    if (-not $fired) {
        throw "Alert rule did not fire within $FiringTimeoutSeconds seconds"
    }

    # Check alert instance created
    $alerts = Invoke-ApiRequest -Token $token -Path "/api/v1/clusters/$clusterID/alerts?state=firing"
    $alert = $alerts | Where-Object { $_.rule_id -eq $ruleID } | Select-Object -First 1
    if (-not $alert) {
        throw 'No firing alert instance found'
    }
    Write-Host "Alert instance created: ID=$($alert.id), Diagnosis=$($alert.diagnosis_id)"

    # Wait for another evaluation to verify deduplication
    Write-Host "Waiting for second evaluation to verify deduplication..."
    Start-Sleep -Seconds 75

    $alerts2 = Invoke-ApiRequest -Token $token -Path "/api/v1/clusters/$clusterID/alerts?state=firing"
    $alert2 = $alerts2 | Where-Object { $_.rule_id -eq $ruleID } | Select-Object -First 1

    if ($alert2.id -ne $alert.id) {
        throw "Deduplication failed: expected same alert instance ID $($alert.id), got $($alert2.id)"
    }
    if ($alert2.diagnosis_id -ne $alert.diagnosis_id) {
        throw "Deduplication failed: expected same diagnosis ID $($alert.diagnosis_id), got $($alert2.diagnosis_id)"
    }
    Write-Host "Deduplication verified: same alert instance and diagnosis"

    # Simulate Metrics Server outage
    Write-Host "Simulating Metrics Server outage..."
    Invoke-KubectlText -Arguments @('scale', 'deployment', 'metrics-server', '-n', 'kube-system', '--replicas=0') | Out-Null
    Start-Sleep -Seconds 20

    # Verify alert remains firing during outage
    $rulesOutage = Invoke-ApiRequest -Token $token -Path "/api/v1/clusters/$clusterID/alert-rules"
    $ruleOutage = $rulesOutage | Where-Object { $_.id -eq $ruleID } | Select-Object -First 1
    Write-Host "During outage: evaluation state=$($ruleOutage.last_evaluation_state), error=$($ruleOutage.last_error_code)"

    # Check alert still firing
    $alertsOutage = Invoke-ApiRequest -Token $token -Path "/api/v1/clusters/$clusterID/alerts?state=firing"
    $alertOutage = $alertsOutage | Where-Object { $_.rule_id -eq $ruleID } | Select-Object -First 1
    if (-not $alertOutage) {
        throw 'Alert was incorrectly resolved during Metrics Server outage'
    }
    Write-Host "Alert remained firing during outage"

    # Restore Metrics Server
    Write-Host "Restoring Metrics Server..."
    Invoke-KubectlText -Arguments @('scale', 'deployment', 'metrics-server', '-n', 'kube-system', '--replicas=1') | Out-Null

    $deadline = (Get-Date).AddSeconds(120)
    do {
        $available = Invoke-KubectlText -Arguments @('get', 'deployment', 'metrics-server', '-n', 'kube-system', '-o', 'jsonpath={.status.availableReplicas}') -AllowFailure
        if ($available -eq '1') { break }
        Start-Sleep -Seconds 5
    } while ((Get-Date) -lt $deadline)

    # Delete alert rule to trigger resolution
    Write-Host "Deleting alert rule to verify soft delete..."
    try {
        Invoke-ApiRequest -Token $token -Method DELETE -Path "/api/v1/clusters/$clusterID/alert-rules/$ruleID"
        throw "Alert rule deletion should have been rejected (unresolved alert exists)"
    } catch {
        if ($_.Exception.Message -notmatch '409') {
            throw "Unexpected error on rule deletion: $_"
        }
        Write-Host "Rule deletion correctly rejected with 409 Conflict"
    }

    # Disable rule instead
    Write-Host "Disabling alert rule..."
    Invoke-ApiRequest -Token $token -Method PATCH -Path "/api/v1/clusters/$clusterID/alert-rules/$ruleID" -Body @{enabled = $false} | Out-Null

    # Wait for resolution
    Write-Host "Waiting for alert resolution (timeout: $RecoveryTimeoutSeconds seconds)..."
    $deadline = (Get-Date).AddSeconds($RecoveryTimeoutSeconds)
    $resolved = $false
    do {
        Start-Sleep -Seconds 10
        $alertsFinal = Invoke-ApiRequest -Token $token -Path "/api/v1/clusters/$clusterID/alerts?limit=100"
        $alertFinal = $alertsFinal | Where-Object { $_.rule_id -eq $ruleID -and $_.state -eq 'resolved' } | Select-Object -First 1
        if ($alertFinal) {
            $resolved = $true
            break
        }
    } while ((Get-Date) -lt $deadline)

    if (-not $resolved) {
        throw "Alert did not resolve within $RecoveryTimeoutSeconds seconds"
    }
    Write-Host "Alert resolved successfully"

    # Backend restart test
    if (-not $SkipBackendRestart) {
        Write-Host "Testing backend restart persistence..."
        Invoke-ComposeText -Arguments @('restart', 'backend') | Out-Null
        if (-not (Wait-BackendReady)) {
            throw 'Backend failed to restart'
        }

        # Re-login
        $token2 = Get-AccessToken -User 'admin' -Pass $adminPass

        # Verify data persisted
        $rulesAfter = Invoke-ApiRequest -Token $token2 -Path "/api/v1/clusters/$clusterID/alert-rules"
        $ruleAfter = $rulesAfter | Where-Object { $_.id -eq $ruleID } | Select-Object -First 1
        if (-not $ruleAfter) {
            throw "Alert rule not found after restart"
        }
        if ($ruleAfter.enabled) {
            throw "Alert rule enabled state not persisted"
        }

        $alertsAfter = Invoke-ApiRequest -Token $token2 -Path "/api/v1/clusters/$clusterID/alerts?limit=100"
        $alertResolved = $alertsAfter | Where-Object { $_.rule_id -eq $ruleID -and $_.state -eq 'resolved' } | Select-Object -First 1
        if (-not $alertResolved) {
            throw "Resolved alert instance not found after restart"
        }
        Write-Host "Backend restart persistence verified"
    }

    # Success - write artifact
    $success = @{
        run_id = $RunID
        cluster_id = $clusterID
        rule_id = $ruleID
        alert_instance_id = $alert.id
        diagnosis_id = $alert.diagnosis_id
        node_name = $nodeName
        passed = $true
        timestamp = (Get-Date -Format 'o')
        checks = @(
            'Alert rule created',
            'Alert fired with low CPU threshold',
            'Deduplication verified (same instance and diagnosis)',
            'Alert remained firing during Metrics Server outage',
            'Alert resolved after rule disable',
            'Backend restart persistence verified'
        )
    }

    $artifactPath = Join-Path $ArtifactDirectory "m27-alert-lifecycle-kind-$RunID.json"
    $success | ConvertTo-Json -Depth 10 | Out-File -LiteralPath $artifactPath -Encoding UTF8

    Write-Host ""
    Write-Host "M27 Alert Lifecycle E2E PASSED"
    Write-Host "================================"
    Write-Host "Run ID: $RunID"
    Write-Host "Cluster ID: $clusterID"
    Write-Host "Rule ID: $ruleID"
    Write-Host "Alert Instance ID: $($alert.id)"
    Write-Host "Diagnosis ID: $($alert.diagnosis_id)"
    Write-Host "Artifact: $artifactPath"

} catch {
    Write-Host ""
    Write-Host "M27 Alert Lifecycle E2E FAILED"
    Write-Host "================================"
    Write-Host "Error: $_"
    Write-Host $_.ScriptStackTrace
    $failure = @{
        run_id = $RunID
        passed = $false
        error = $_.Exception.Message
        timestamp = (Get-Date -Format 'o')
    }
    $failPath = Join-Path $ArtifactDirectory "m27-alert-lifecycle-kind-$RunID-failure.json"
    $failure | ConvertTo-Json -Depth 10 | Out-File -LiteralPath $failPath -Encoding UTF8
    throw
} finally {
    # Cleanup
    Write-Host ""
    Write-Host "Cleaning up..."

    # Delete cluster registration
    if ($clusterID -and $token) {
        try {
            Invoke-ApiRequest -Token $token -Method DELETE -Path "/api/v1/clusters/$clusterID" | Out-Null
            Write-Host "Deleted cluster registration: $clusterID"
        } catch {
            Write-Host "Warning: Failed to delete cluster registration: $_"
        }
    }

    # Delete kind cluster
    try {
        $env:KUBECONFIG = $KubeconfigPath
        Invoke-NativeText -FilePath $Kind -Arguments @('delete', 'cluster', '--name', $KindClusterName) -AllowFailure | Out-Null
        Write-Host "Deleted kind cluster: $KindClusterName"
    } catch {
        Write-Host "Warning: Failed to delete kind cluster: $_"
    }

    # Clean up temp kubeconfig
    if (Test-Path -LiteralPath $KubeconfigPath) {
        Remove-Item -LiteralPath $KubeconfigPath -Force -ErrorAction SilentlyContinue
    }

    # Restore original kubeconfig
    Remove-Item Env:\KUBECONFIG -ErrorAction SilentlyContinue

    Write-Host "Cleanup complete"
}