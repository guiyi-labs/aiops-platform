[CmdletBinding()]
param(
    [string]$PostgresImage = 'pgvector/pgvector:0.8.1-pg17',
    [int]$ReadyTimeoutSeconds = 120
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Root = Split-Path -Parent $PSScriptRoot
$ArtifactDirectory = Join-Path $Root '.artifacts\credential-reencryption'
$RunID = '{0}-{1}' -f (Get-Date -Format 'yyyyMMdd-HHmmss'), ([guid]::NewGuid().ToString('N').Substring(0, 6))
$Network = "aiops-credential-rotation-$RunID"
$DatabaseContainer = "aiops-credential-db-$RunID"
$BackendContainer = "aiops-credential-api-$RunID"
$BackendImage = "aiops-credential-rotation:$RunID"
$DatabaseName = 'aiops_credential_rotation'
$DatabaseUser = 'aiops_rotation'

[IO.Directory]::CreateDirectory($ArtifactDirectory) | Out-Null

function Invoke-NativeResult {
    param(
        [Parameter(Mandatory)] [string]$File,
        [Parameter(Mandatory)] [string[]]$Arguments
    )

    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $output = & $File @Arguments 2>&1
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorAction
    }
    return [pscustomobject]@{
        ExitCode = $exitCode
        Output = (($output | ForEach-Object { $_.ToString() }) -join [Environment]::NewLine).Trim()
    }
}

function Invoke-NativeText {
    param(
        [Parameter(Mandatory)] [string]$File,
        [Parameter(Mandatory)] [string[]]$Arguments,
        [switch]$AllowFailure
    )

    $result = Invoke-NativeResult -File $File -Arguments $Arguments
    if ($result.ExitCode -ne 0 -and -not $AllowFailure) {
        throw "$File command failed with exit code $($result.ExitCode): $($result.Output)"
    }
    return $result.Output
}

function New-RandomBase64 {
    param([int]$Bytes = 32)

    $buffer = [byte[]]::new($Bytes)
    $generator = [Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        $generator.GetBytes($buffer)
    } finally {
        $generator.Dispose()
    }
    return [Convert]::ToBase64String($buffer)
}

function New-RandomHex {
    param([int]$Bytes = 24)

    $buffer = [byte[]]::new($Bytes)
    $generator = [Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        $generator.GetBytes($buffer)
    } finally {
        $generator.Dispose()
    }
    return (($buffer | ForEach-Object { $_.ToString('x2') }) -join '')
}

function Assert-Equal {
    param($Actual, $Expected, [Parameter(Mandatory)] [string]$Message)

    if ($Actual -ne $Expected) {
        throw "$Message; expected $Expected, got $Actual"
    }
}

function Test-ContainerExists {
    param([Parameter(Mandatory)] [string]$Name)

    $id = Invoke-NativeText -File 'docker' -Arguments @('ps', '-aq', '--filter', "name=^/$Name`$") -AllowFailure
    return -not [string]::IsNullOrWhiteSpace($id)
}

function Remove-OwnedContainer {
    param([Parameter(Mandatory)] [string]$Name)

    Invoke-NativeText -File 'docker' -Arguments @('rm', '--force', '--volumes', $Name) -AllowFailure | Out-Null
}

function Wait-PostgresReady {
    $deadline = (Get-Date).AddSeconds($ReadyTimeoutSeconds)
    do {
        $output = Invoke-NativeText -File 'docker' -Arguments @(
            'exec', $DatabaseContainer, 'pg_isready', '--username', $DatabaseUser, '--dbname', $DatabaseName
        ) -AllowFailure
        if ($output -match 'accepting connections') {
            return
        }
        Start-Sleep -Seconds 2
    } while ((Get-Date) -lt $deadline)
    throw "isolated PostgreSQL did not become ready within $ReadyTimeoutSeconds seconds"
}

function Start-Backend {
    param([Parameter(Mandatory)] [string]$KeyVersion, [Parameter(Mandatory)] [string]$DecryptionKeys)

    Remove-OwnedContainer -Name $BackendContainer
    $env:CREDENTIAL_KEY_VERSION = $KeyVersion
    $env:CREDENTIAL_DECRYPTION_KEYS = $DecryptionKeys
    Invoke-NativeText -File 'docker' -Arguments @(
        'run', '--detach', '--name', $BackendContainer,
        '--label', 'io.guiyi.aiops.purpose=credential-reencryption-drill',
        '--network', $Network, '--publish', '127.0.0.1::8080',
        '--env', 'APP_ENV', '--env', 'DATABASE_URL', '--env', 'JWT_SIGNING_KEY',
        '--env', 'BOOTSTRAP_ADMIN_USERNAME', '--env', 'BOOTSTRAP_ADMIN_PASSWORD',
        '--env', 'CREDENTIAL_ENCRYPTION_KEY', '--env', 'CREDENTIAL_KEY_VERSION',
        '--env', 'CREDENTIAL_DECRYPTION_KEYS', '--env', 'CLUSTER_PROBE_TIMEOUT',
        '--env', 'AI_ENABLED', '--env', 'NOTIFICATION_ENABLED',
        $BackendImage
    ) | Out-Null

    $port = (Invoke-NativeText -File 'docker' -Arguments @('port', $BackendContainer, '8080/tcp')).Split(':')[-1]
    $apiBase = "http://127.0.0.1:$port"
    $deadline = (Get-Date).AddSeconds($ReadyTimeoutSeconds)
    do {
        try {
            $health = Invoke-RestMethod "$apiBase/api/v1/health/ready" -TimeoutSec 5
            if ($health.status -eq 'ready') {
                return $apiBase
            }
        } catch {
        }
        Start-Sleep -Seconds 2
    } while ((Get-Date) -lt $deadline)
    throw "isolated backend did not become ready within $ReadyTimeoutSeconds seconds"
}

function Stop-Backend {
    Remove-OwnedContainer -Name $BackendContainer
}

function Invoke-PsqlScalar {
    param([Parameter(Mandatory)] [string]$Query)

    return (Invoke-NativeText -File 'docker' -Arguments @(
        'exec', $DatabaseContainer, 'psql', '--no-psqlrc', '--set', 'ON_ERROR_STOP=1',
        '--tuples-only', '--no-align', '--username', $DatabaseUser, '--dbname', $DatabaseName,
        '--command', $Query
    )).Trim()
}

function Invoke-Reencryption {
    param([switch]$Apply, [int]$ExpectedExitCode = 0)

    $arguments = @(
        'run', '--rm', '--network', $Network,
        '--env', 'APP_ENV', '--env', 'DATABASE_URL', '--env', 'JWT_SIGNING_KEY',
        '--env', 'BOOTSTRAP_ADMIN_PASSWORD', '--env', 'CREDENTIAL_ENCRYPTION_KEY',
        '--env', 'CREDENTIAL_KEY_VERSION', '--env', 'CREDENTIAL_DECRYPTION_KEYS',
        '--env', 'AI_ENABLED', '--env', 'NOTIFICATION_ENABLED',
        '--entrypoint', '/app/credential-reencrypt', $BackendImage,
        '--batch-size=2', '--max-records=2', '--timeout=2m'
    )
    if ($Apply) {
        $arguments += '--apply'
    }
    $command = Invoke-NativeResult -File 'docker' -Arguments $arguments
    Assert-Equal -Actual $command.ExitCode -Expected $ExpectedExitCode -Message 'credential command exit code mismatch'
    $jsonOutput = (($command.Output -split '\r?\n' | Where-Object {
        $_ -notmatch '^credential re-encryption failed \([A-Z_]+\)$'
    }) -join [Environment]::NewLine)
    $start = $jsonOutput.IndexOf('{')
    $end = $jsonOutput.LastIndexOf('}')
    if ($start -lt 0 -or $end -lt $start) {
        throw 'credential command did not return a JSON result'
    }
    return ($jsonOutput.Substring($start, $end - $start + 1) | ConvertFrom-Json)
}

function New-SessionHeaders {
    param([Parameter(Mandatory)] [string]$ApiBase)

    $login = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/auth/login" -ContentType 'application/json' -Body (@{
        username = 'admin'
        password = $env:BOOTSTRAP_ADMIN_PASSWORD
    } | ConvertTo-Json)
    return @{ Authorization = "Bearer $($login.access_token)" }
}

$environmentNames = @(
    'APP_ENV', 'POSTGRES_DB', 'POSTGRES_USER', 'POSTGRES_PASSWORD', 'DATABASE_URL',
    'JWT_SIGNING_KEY', 'BOOTSTRAP_ADMIN_USERNAME', 'BOOTSTRAP_ADMIN_PASSWORD',
    'CREDENTIAL_ENCRYPTION_KEY', 'CREDENTIAL_KEY_VERSION', 'CREDENTIAL_DECRYPTION_KEYS',
    'CLUSTER_PROBE_TIMEOUT', 'AI_ENABLED', 'NOTIFICATION_ENABLED'
)
$previousEnvironment = [ordered]@{}
foreach ($name in $environmentNames) {
    $previousEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}

$legacyKey = New-RandomBase64
$activeKey = New-RandomBase64
$failure = $null
$summary = $null
$cleanupFailures = [Collections.Generic.List[string]]::new()
$cleanup = [ordered]@{
    database_container_deleted = $false
    backend_container_deleted = $false
    network_deleted = $false
    temporary_image_deleted = $false
    process_environment_restored = $false
}

try {
    Get-Command docker -ErrorAction Stop | Out-Null
    $env:APP_ENV = 'production'
    $env:POSTGRES_DB = $DatabaseName
    $env:POSTGRES_USER = $DatabaseUser
    $env:POSTGRES_PASSWORD = New-RandomHex
    $env:DATABASE_URL = "postgres://${DatabaseUser}:$($env:POSTGRES_PASSWORD)@${DatabaseContainer}:5432/${DatabaseName}?sslmode=disable"
    $env:JWT_SIGNING_KEY = New-RandomHex -Bytes 32
    $env:BOOTSTRAP_ADMIN_USERNAME = 'admin'
    $env:BOOTSTRAP_ADMIN_PASSWORD = New-RandomHex
    $env:CREDENTIAL_ENCRYPTION_KEY = $legacyKey
    $env:CREDENTIAL_KEY_VERSION = 'v1'
    $env:CREDENTIAL_DECRYPTION_KEYS = '{}'
    $env:CLUSTER_PROBE_TIMEOUT = '2s'
    $env:AI_ENABLED = 'false'
    $env:NOTIFICATION_ENABLED = 'false'

    Write-Host '[1/8] Building the isolated backend and credential command image'
    Invoke-NativeText -File 'docker' -Arguments @(
        'build', '--tag', $BackendImage, '--build-arg', "VERSION=credential-rotation-$RunID", (Join-Path $Root 'backend')
    ) | Out-Null

    Write-Host '[2/8] Starting isolated PostgreSQL and v1 backend'
    Invoke-NativeText -File 'docker' -Arguments @('network', 'create', '--label', 'io.guiyi.aiops.purpose=credential-reencryption-drill', $Network) | Out-Null
    Invoke-NativeText -File 'docker' -Arguments @(
        'run', '--detach', '--name', $DatabaseContainer,
        '--label', 'io.guiyi.aiops.purpose=credential-reencryption-drill',
        '--network', $Network, '--env', 'POSTGRES_DB', '--env', 'POSTGRES_USER', '--env', 'POSTGRES_PASSWORD',
        $PostgresImage
    ) | Out-Null
    Wait-PostgresReady
    $apiBase = Start-Backend -KeyVersion 'v1' -DecryptionKeys '{}'

    Write-Host '[3/8] Creating two real v1 encrypted cluster credentials through the API'
    $headers = New-SessionHeaders -ApiBase $apiBase
    $kubeconfig = @'
apiVersion: v1
kind: Config
clusters:
  - name: rotation
    cluster:
      server: https://127.0.0.1:9
      insecure-skip-tls-verify: true
contexts:
  - name: rotation
    context:
      cluster: rotation
      user: rotation
current-context: rotation
users:
  - name: rotation
    user:
      token: synthetic-rotation-fixture
'@
    $clusterIDs = @()
    foreach ($suffix in @('a', 'b')) {
        $cluster = Invoke-RestMethod -Method Post -Uri "$apiBase/api/v1/clusters" -Headers $headers -ContentType 'application/json' -Body (@{
            name = "credential-rotation-$RunID-$suffix"
            kubeconfig = $kubeconfig
        } | ConvertTo-Json)
        $clusterIDs += [int64]$cluster.id
    }
    Stop-Backend
    Assert-Equal -Actual (Invoke-PsqlScalar -Query "SELECT COUNT(*) FROM cluster_credentials WHERE encryption_key_version = 'v1'") -Expected '2' -Message 'v1 seed count mismatch'

    $env:CREDENTIAL_ENCRYPTION_KEY = $activeKey
    $env:CREDENTIAL_KEY_VERSION = 'v2'
    $env:CREDENTIAL_DECRYPTION_KEYS = (@{ v1 = $legacyKey } | ConvertTo-Json -Compress)

    Write-Host '[4/8] Proving default dry-run validates but does not modify credentials'
    $dryRun = Invoke-Reencryption
    Assert-Equal -Actual $dryRun.status -Expected 'succeeded' -Message 'dry-run status mismatch'
    Assert-Equal -Actual ([int]$dryRun.examined_count) -Expected 2 -Message 'dry-run examined count mismatch'
    Assert-Equal -Actual ([int]$dryRun.reencrypted_count) -Expected 0 -Message 'dry-run write count mismatch'
    Assert-Equal -Actual (Invoke-PsqlScalar -Query "SELECT COUNT(*) FROM cluster_credentials WHERE encryption_key_version = 'v1'") -Expected '2' -Message 'dry-run changed credential versions'

    Write-Host '[5/8] Proving a corrupt second row rolls back the complete batch'
    $firstHashBefore = Invoke-PsqlScalar -Query "SELECT md5(encrypted_kubeconfig) FROM cluster_credentials WHERE cluster_id = $($clusterIDs[0])"
    Invoke-PsqlScalar -Query "UPDATE cluster_credentials SET encrypted_kubeconfig = decode('00', 'hex') WHERE cluster_id = $($clusterIDs[1]) RETURNING cluster_id" | Out-Null
    $failedRun = Invoke-Reencryption -Apply -ExpectedExitCode 1
    Assert-Equal -Actual $failedRun.status -Expected 'failed' -Message 'rollback run status mismatch'
    Assert-Equal -Actual $failedRun.error_code -Expected 'REENCRYPTION_FAILED' -Message 'rollback error code mismatch'
    Assert-Equal -Actual ([int]$failedRun.remaining_count) -Expected 2 -Message 'rollback remaining count mismatch'
    Assert-Equal -Actual (Invoke-PsqlScalar -Query "SELECT md5(encrypted_kubeconfig) FROM cluster_credentials WHERE cluster_id = $($clusterIDs[0])") -Expected $firstHashBefore -Message 'first row changed during failed batch'
    Assert-Equal -Actual (Invoke-PsqlScalar -Query "SELECT COUNT(*) FROM cluster_credentials WHERE encryption_key_version = 'v1'") -Expected '2' -Message 'failed batch changed credential versions'

    Write-Host '[6/8] Restoring the synthetic row and applying v1 to v2 re-encryption'
    Invoke-PsqlScalar -Query "UPDATE cluster_credentials SET encrypted_kubeconfig = (SELECT encrypted_kubeconfig FROM cluster_credentials WHERE cluster_id = $($clusterIDs[0])) WHERE cluster_id = $($clusterIDs[1]) RETURNING cluster_id" | Out-Null
    $applied = Invoke-Reencryption -Apply
    Assert-Equal -Actual $applied.status -Expected 'succeeded' -Message 'apply status mismatch'
    Assert-Equal -Actual ([int]$applied.reencrypted_count) -Expected 2 -Message 'apply count mismatch'
    Assert-Equal -Actual ([int]$applied.remaining_count) -Expected 0 -Message 'apply remaining count mismatch'
    Assert-Equal -Actual (Invoke-PsqlScalar -Query "SELECT COUNT(*) FROM cluster_credentials WHERE encryption_key_version = 'v2'") -Expected '2' -Message 'v2 credential count mismatch'

    Write-Host '[7/8] Starting a v2-only backend and proving the rotated credential decrypts'
    $env:CREDENTIAL_DECRYPTION_KEYS = '{}'
    $apiBase = Start-Backend -KeyVersion 'v2' -DecryptionKeys '{}'
    $headers = New-SessionHeaders -ApiBase $apiBase
    $probe = Invoke-RestMethod -Method Post -Uri "$apiBase/api/v1/clusters/$($clusterIDs[0])/probe" -Headers $headers
    $credentialCondition = @($probe.conditions | Where-Object { $_.type -eq 'CredentialValid' }) | Select-Object -First 1
    if ($null -eq $credentialCondition -or $credentialCondition.status -ne 'True') {
        throw 'v2-only backend could not decrypt the rotated credential'
    }
    Stop-Backend

    Write-Host '[8/8] Verifying sanitized audit metadata'
    $auditText = Invoke-PsqlScalar -Query @'
SELECT status || ':' || error_code || ':' || examined_count || ':' || reencrypted_count || ':' || remaining_count
FROM credential_reencryption_runs
ORDER BY started_at, id
'@
    $auditRows = @($auditText -split '\r?\n' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    Assert-Equal -Actual $auditRows.Count -Expected 3 -Message 'credential re-encryption audit count mismatch'
    if (@($auditRows | Where-Object { $_ -eq 'succeeded::2:0:2' }).Count -ne 1 -or
        @($auditRows | Where-Object { $_ -eq 'failed:REENCRYPTION_FAILED:0:0:2' }).Count -ne 1 -or
        @($auditRows | Where-Object { $_ -eq 'succeeded::2:2:0' }).Count -ne 1) {
        throw "credential re-encryption audit metadata mismatch: $($auditRows -join ', ')"
    }

    $summary = [ordered]@{
        verified_at = (Get-Date).ToString('o')
        postgres_image = $PostgresImage
        source_key_version = 'v1'
        target_key_version = 'v2'
        credentials = 2
        dry_run = [ordered]@{ status = $dryRun.status; examined = [int]$dryRun.examined_count; modified = [int]$dryRun.reencrypted_count; remaining = [int]$dryRun.remaining_count }
        rollback = [ordered]@{ status = $failedRun.status; error_code = $failedRun.error_code; first_row_unchanged = $true; remaining = [int]$failedRun.remaining_count }
        apply = [ordered]@{ status = $applied.status; modified = [int]$applied.reencrypted_count; remaining = [int]$applied.remaining_count }
        v2_only_backend_decryption = $true
        audit_rows = $auditRows
    }
} catch {
    $failure = $_
} finally {
    Stop-Backend
    Remove-OwnedContainer -Name $DatabaseContainer
    $cleanup.backend_container_deleted = -not (Test-ContainerExists -Name $BackendContainer)
    $cleanup.database_container_deleted = -not (Test-ContainerExists -Name $DatabaseContainer)
    Invoke-NativeText -File 'docker' -Arguments @('network', 'rm', $Network) -AllowFailure | Out-Null
    $cleanup.network_deleted = [string]::IsNullOrWhiteSpace((Invoke-NativeText -File 'docker' -Arguments @('network', 'ls', '--quiet', '--filter', "name=^$Network`$") -AllowFailure))
    Invoke-NativeText -File 'docker' -Arguments @('image', 'rm', '--force', $BackendImage) -AllowFailure | Out-Null
    $cleanup.temporary_image_deleted = [string]::IsNullOrWhiteSpace((Invoke-NativeText -File 'docker' -Arguments @('image', 'ls', '--quiet', $BackendImage) -AllowFailure))

    foreach ($name in $previousEnvironment.Keys) {
        [Environment]::SetEnvironmentVariable($name, $previousEnvironment[$name], 'Process')
    }
    $cleanup.process_environment_restored = @($previousEnvironment.Keys | Where-Object {
        $actual = [Environment]::GetEnvironmentVariable($_, 'Process')
        $expected = $previousEnvironment[$_]
        if ([string]::IsNullOrEmpty($expected)) {
            -not [string]::IsNullOrEmpty($actual)
        } else {
            $actual -cne $expected
        }
    }).Count -eq 0

    foreach ($entry in $cleanup.GetEnumerator()) {
        if (-not $entry.Value) {
            $cleanupFailures.Add("$($entry.Key) was not completed")
        }
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
$evidencePath = Join-Path $ArtifactDirectory ("credential-reencryption-{0}.json" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
[IO.File]::WriteAllText($evidencePath, ($summary | ConvertTo-Json -Depth 10), [Text.UTF8Encoding]::new($false))
Write-Host "Credential re-encryption verification passed. Evidence: $evidencePath"
$summary | ConvertTo-Json -Depth 10
