[CmdletBinding()]
param(
    [string]$PostgresImage = 'pgvector/pgvector:0.8.1-pg17',
    [int]$ReadyTimeoutSeconds = 120
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Root = Split-Path -Parent $PSScriptRoot
$MigrationDirectory = Join-Path $Root 'backend\migrations'
$ArtifactDirectory = Join-Path $Root '.artifacts\postgres-recovery'
$RunID = '{0}-{1}' -f (Get-Date -Format 'yyyyMMdd-HHmmss'), ([guid]::NewGuid().ToString('N').Substring(0, 6))
$SourceContainer = "aiops-postgres-source-$RunID"
$TargetContainer = "aiops-postgres-target-$RunID"
$DatabaseName = 'aiops_restore_drill'
$DatabaseUser = 'aiops_drill'
$TemporaryRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$TemporaryDirectory = [IO.Path]::GetFullPath((Join-Path $TemporaryRoot "aiops-postgres-recovery-$RunID"))
$BackupPath = Join-Path $TemporaryDirectory 'aiops.dump'

if (-not $TemporaryDirectory.StartsWith($TemporaryRoot, [StringComparison]::OrdinalIgnoreCase)) {
    throw "temporary directory escaped the system temporary root: $TemporaryDirectory"
}

[IO.Directory]::CreateDirectory($ArtifactDirectory) | Out-Null
[IO.Directory]::CreateDirectory($TemporaryDirectory) | Out-Null

function Invoke-NativeText {
    param(
        [Parameter(Mandatory)] [string]$File,
        [Parameter(Mandatory)] [string[]]$Arguments,
        [switch]$AllowFailure
    )

    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $output = & $File @Arguments 2>&1
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorAction
    }
    if ($exitCode -ne 0 -and -not $AllowFailure) {
        throw "$File command failed with exit code $exitCode`: $($output -join [Environment]::NewLine)"
    }
    return (($output | ForEach-Object { $_.ToString() }) -join [Environment]::NewLine).Trim()
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

function Write-Utf8File {
    param([Parameter(Mandatory)] [string]$Path, [Parameter(Mandatory)] [string]$Contents)

    [IO.File]::WriteAllText($Path, $Contents, [Text.UTF8Encoding]::new($false))
}

function Test-ContainerExists {
    param([Parameter(Mandatory)] [string]$Name)

    $containerID = Invoke-NativeText -File 'docker' -Arguments @(
        'ps', '-aq', '--filter', "name=^/$Name`$"
    ) -AllowFailure
    return -not [string]::IsNullOrWhiteSpace($containerID)
}

function Remove-DrillContainer {
    param([Parameter(Mandatory)] [string]$Name)

    Invoke-NativeText -File 'docker' -Arguments @('rm', '--force', '--volumes', $Name) -AllowFailure | Out-Null
}

function Wait-PostgresReady {
    param([Parameter(Mandatory)] [string]$Container)

    $deadline = (Get-Date).AddSeconds($ReadyTimeoutSeconds)
    do {
        $output = Invoke-NativeText -File 'docker' -Arguments @(
            'exec', $Container, 'pg_isready', '--username', $DatabaseUser, '--dbname', $DatabaseName
        ) -AllowFailure
        if ($output -match 'accepting connections') {
            return
        }
        Start-Sleep -Seconds 2
    } while ((Get-Date) -lt $deadline)
    throw "$Container did not accept PostgreSQL connections within $ReadyTimeoutSeconds seconds"
}

function Start-PostgresContainer {
    param(
        [Parameter(Mandatory)] [string]$Name,
        [switch]$MountMigrations
    )

    $arguments = @(
        'run', '--detach', '--name', $Name,
        '--label', 'io.guiyi.aiops.purpose=postgres-recovery-drill',
        '--env', 'POSTGRES_DB', '--env', 'POSTGRES_USER', '--env', 'POSTGRES_PASSWORD'
    )
    if ($MountMigrations) {
        $arguments += @(
            '--mount', "type=bind,source=$MigrationDirectory,target=/migrations,readonly"
        )
    }
    $arguments += $PostgresImage
    Invoke-NativeText -File 'docker' -Arguments $arguments | Out-Null
    Wait-PostgresReady -Container $Name
}

function Invoke-Psql {
    param(
        [Parameter(Mandatory)] [string]$Container,
        [Parameter(Mandatory)] [string[]]$Arguments
    )

    return Invoke-NativeText -File 'docker' -Arguments (@(
        'exec', $Container, 'psql', '--no-psqlrc', '--set', 'ON_ERROR_STOP=1',
        '--username', $DatabaseUser, '--dbname', $DatabaseName
    ) + $Arguments)
}

function Get-PsqlScalar {
    param(
        [Parameter(Mandatory)] [string]$Container,
        [Parameter(Mandatory)] [string]$Query
    )

    return (Invoke-Psql -Container $Container -Arguments @(
        '--tuples-only', '--no-align', '--command', $Query
    )).Trim()
}

function Assert-Equal {
    param($Actual, $Expected, [Parameter(Mandatory)] [string]$Message)

    if ($Actual -ne $Expected) {
        throw "$Message; expected $Expected, got $Actual"
    }
}

function Get-DatabaseSnapshot {
    param([Parameter(Mandatory)] [string]$Container)

    return [ordered]@{
        migrations = [int](Get-PsqlScalar -Container $Container -Query 'SELECT COUNT(*) FROM schema_migrations')
        roles = [int](Get-PsqlScalar -Container $Container -Query 'SELECT COUNT(*) FROM roles')
        users = [int](Get-PsqlScalar -Container $Container -Query "SELECT COUNT(*) FROM users WHERE username LIKE 'recovery-%'")
        user_roles = [int](Get-PsqlScalar -Container $Container -Query "SELECT COUNT(*) FROM user_roles ur JOIN users u ON u.id = ur.user_id WHERE u.username LIKE 'recovery-%'")
        clusters = [int](Get-PsqlScalar -Container $Container -Query "SELECT COUNT(*) FROM clusters WHERE name LIKE 'recovery-%'")
        credentials = [int](Get-PsqlScalar -Container $Container -Query "SELECT COUNT(*) FROM cluster_credentials cc JOIN clusters c ON c.id = cc.cluster_id WHERE c.name LIKE 'recovery-%' AND encode(cc.encrypted_kubeconfig, 'hex') IN ('01020304', 'a1b2c3d4')")
        diagnoses = [int](Get-PsqlScalar -Container $Container -Query "SELECT COUNT(*) FROM diagnosis_records WHERE rule_id = 'recovery.drill.v1'")
        audit_logs = [int](Get-PsqlScalar -Container $Container -Query "SELECT COUNT(*) FROM audit_logs WHERE request_id = 'recovery-drill-request'")
        saved_filters = [int](Get-PsqlScalar -Container $Container -Query "SELECT COUNT(*) FROM saved_global_search_filters WHERE name = 'Recovery workloads'")
        invalid_foreign_keys = [int](Get-PsqlScalar -Container $Container -Query @'
SELECT
    (SELECT COUNT(*) FROM user_roles ur LEFT JOIN users u ON u.id = ur.user_id WHERE u.id IS NULL) +
    (SELECT COUNT(*) FROM cluster_credentials cc LEFT JOIN clusters c ON c.id = cc.cluster_id WHERE c.id IS NULL) +
    (SELECT COUNT(*) FROM diagnosis_records d LEFT JOIN clusters c ON c.id = d.cluster_id WHERE c.id IS NULL) +
    (SELECT COUNT(*) FROM saved_global_search_filters f LEFT JOIN users u ON u.id = f.user_id WHERE u.id IS NULL)
'@)
    }
}

function Assert-Snapshot {
    param(
        [Parameter(Mandatory)] [Collections.IDictionary]$Actual,
        [Parameter(Mandatory)] [Collections.IDictionary]$Expected,
        [Parameter(Mandatory)] [string]$Stage
    )

    foreach ($key in $Expected.Keys) {
        Assert-Equal -Actual $Actual[$key] -Expected $Expected[$key] -Message "$Stage snapshot mismatch for $key"
    }
}

$previousEnvironment = [ordered]@{
    POSTGRES_DB = [Environment]::GetEnvironmentVariable('POSTGRES_DB', 'Process')
    POSTGRES_USER = [Environment]::GetEnvironmentVariable('POSTGRES_USER', 'Process')
    POSTGRES_PASSWORD = [Environment]::GetEnvironmentVariable('POSTGRES_PASSWORD', 'Process')
}
$databasePassword = New-RandomHex
$failure = $null
$summary = $null
$cleanupFailures = [Collections.Generic.List[string]]::new()
$cleanup = [ordered]@{
    source_container_deleted = $false
    target_container_deleted = $false
    temporary_files_deleted = $false
    process_environment_restored = $false
}

try {
    Get-Command docker -ErrorAction Stop | Out-Null
    if (-not (Test-Path -LiteralPath $MigrationDirectory)) {
        throw "migration directory does not exist: $MigrationDirectory"
    }

    $env:POSTGRES_DB = $DatabaseName
    $env:POSTGRES_USER = $DatabaseUser
    $env:POSTGRES_PASSWORD = $databasePassword

    Write-Host '[1/7] Starting isolated source PostgreSQL'
    Start-PostgresContainer -Name $SourceContainer -MountMigrations

    Write-Host '[2/7] Applying embedded migration set and synthetic business fixtures'
    Invoke-Psql -Container $SourceContainer -Arguments @(
        '--command', 'CREATE TABLE IF NOT EXISTS schema_migrations (version VARCHAR(255) PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())'
    ) | Out-Null
    $migrationFiles = @(Get-ChildItem -LiteralPath $MigrationDirectory -Filter '*.up.sql' -File | Sort-Object Name)
    if ($migrationFiles.Count -eq 0) {
        throw 'no PostgreSQL up migrations were found'
    }
    foreach ($migration in $migrationFiles) {
        Invoke-Psql -Container $SourceContainer -Arguments @(
            '--single-transaction', '--file', "/migrations/$($migration.Name)"
        ) | Out-Null
        $escapedName = $migration.Name.Replace("'", "''")
        Invoke-Psql -Container $SourceContainer -Arguments @(
            '--command', "INSERT INTO schema_migrations (version) VALUES ('$escapedName')"
        ) | Out-Null
    }

    $seedPath = Join-Path $TemporaryDirectory 'seed.sql'
    Write-Utf8File -Path $seedPath -Contents @'
BEGIN;
INSERT INTO users (username, password_hash, display_name, status) VALUES
    ('recovery-admin', 'synthetic-not-a-login-hash', 'Recovery Admin', 'active'),
    ('recovery-viewer', 'synthetic-not-a-login-hash', 'Recovery Viewer', 'active');

INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id
FROM users u
JOIN roles r ON r.code = CASE u.username
    WHEN 'recovery-admin' THEN 'system_admin'
    ELSE 'viewer'
END
WHERE u.username IN ('recovery-admin', 'recovery-viewer');

INSERT INTO clusters (name, api_server, status, enabled) VALUES
    ('recovery-cluster-a', 'https://recovery-a.invalid:6443', 'ready', TRUE),
    ('recovery-cluster-b', 'https://recovery-b.invalid:6443', 'unknown', TRUE);

INSERT INTO cluster_credentials (cluster_id, encrypted_kubeconfig, encryption_key_version)
SELECT id,
    CASE name WHEN 'recovery-cluster-a' THEN decode('01020304', 'hex') ELSE decode('a1b2c3d4', 'hex') END,
    'recovery-drill-v1'
FROM clusters
WHERE name IN ('recovery-cluster-a', 'recovery-cluster-b');

INSERT INTO diagnosis_records (
    cluster_id, rule_id, severity, resource_kind, resource_namespace,
    resource_name, resource_uid, status, summary, root_causes,
    recommendations, observed_at, sla_due_at
)
SELECT id, 'recovery.drill.v1', 'warning', 'Deployment', 'recovery',
    'fixture', 'recovery-fixture-uid', 'open', 'Synthetic recovery fixture',
    '["synthetic"]'::JSONB, '["restore and verify"]'::JSONB,
    TIMESTAMPTZ '2026-07-28 00:00:00+00', TIMESTAMPTZ '2026-07-29 00:00:00+00'
FROM clusters WHERE name = 'recovery-cluster-a';

INSERT INTO audit_logs (
    actor_user_id, cluster_id, action, resource_type, resource_namespace,
    resource_name, result, request_id, details, actor_name, status_code
)
SELECT u.id, c.id, 'recovery.drill.seed', 'PostgreSQL', '', 'isolated',
    'success', 'recovery-drill-request', '{"synthetic":true}'::JSONB,
    u.display_name, 200
FROM users u CROSS JOIN clusters c
WHERE u.username = 'recovery-admin' AND c.name = 'recovery-cluster-a';

INSERT INTO saved_global_search_filters (user_id, name, query_text, namespace, kinds)
SELECT id, 'Recovery workloads', 'recovery', 'recovery', 'Pod,Deployment'
FROM users WHERE username = 'recovery-viewer';
COMMIT;
'@
    Invoke-NativeText -File 'docker' -Arguments @('cp', $seedPath, "${SourceContainer}:/tmp/seed.sql") | Out-Null
    Invoke-Psql -Container $SourceContainer -Arguments @('--file', '/tmp/seed.sql') | Out-Null

    $expectedSnapshot = [ordered]@{
        migrations = $migrationFiles.Count
        roles = 4
        users = 2
        user_roles = 2
        clusters = 2
        credentials = 2
        diagnoses = 1
        audit_logs = 1
        saved_filters = 1
        invalid_foreign_keys = 0
    }
    $sourceSnapshot = Get-DatabaseSnapshot -Container $SourceContainer
    Assert-Snapshot -Actual $sourceSnapshot -Expected $expectedSnapshot -Stage 'source'
    $latestMigration = Get-PsqlScalar -Container $SourceContainer -Query 'SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1'
    Assert-Equal -Actual $latestMigration -Expected $migrationFiles[-1].Name -Message 'source latest migration mismatch'

    Write-Host '[3/7] Creating custom-format logical backup'
    Invoke-NativeText -File 'docker' -Arguments @(
        'exec', $SourceContainer, 'pg_dump', '--format=custom', '--compress=6',
        '--no-owner', '--no-acl', '--username', $DatabaseUser, '--dbname', $DatabaseName,
        '--file', '/tmp/aiops.dump'
    ) | Out-Null
    Invoke-NativeText -File 'docker' -Arguments @('cp', "${SourceContainer}:/tmp/aiops.dump", $BackupPath) | Out-Null
    $backup = Get-Item -LiteralPath $BackupPath
    if ($backup.Length -le 0) {
        throw 'PostgreSQL backup is empty'
    }
    $backupHash = (Get-FileHash -LiteralPath $BackupPath -Algorithm SHA256).Hash.ToLowerInvariant()

    Write-Host '[4/7] Destroying source instance before restore'
    Remove-DrillContainer -Name $SourceContainer
    if (Test-ContainerExists -Name $SourceContainer) {
        throw 'source PostgreSQL container still exists before restore'
    }

    Write-Host '[5/7] Starting a fresh target PostgreSQL and restoring backup'
    Start-PostgresContainer -Name $TargetContainer
    Invoke-NativeText -File 'docker' -Arguments @('cp', $BackupPath, "${TargetContainer}:/tmp/aiops.dump") | Out-Null
    $archiveItems = Invoke-NativeText -File 'docker' -Arguments @(
        'exec', $TargetContainer, 'pg_restore', '--list', '/tmp/aiops.dump'
    )
    if ([string]::IsNullOrWhiteSpace($archiveItems)) {
        throw 'pg_restore did not recognize any backup archive entries'
    }
    Invoke-NativeText -File 'docker' -Arguments @(
        'exec', $TargetContainer, 'pg_restore', '--exit-on-error', '--no-owner', '--no-acl',
        '--username', $DatabaseUser, '--dbname', $DatabaseName, '/tmp/aiops.dump'
    ) | Out-Null

    Write-Host '[6/7] Verifying migrations, business rows and foreign-key consistency'
    $targetSnapshot = Get-DatabaseSnapshot -Container $TargetContainer
    Assert-Snapshot -Actual $targetSnapshot -Expected $expectedSnapshot -Stage 'target'
    $targetLatestMigration = Get-PsqlScalar -Container $TargetContainer -Query 'SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1'
    Assert-Equal -Actual $targetLatestMigration -Expected $latestMigration -Message 'restored latest migration mismatch'
    Assert-Snapshot -Actual $targetSnapshot -Expected $sourceSnapshot -Stage 'source-to-target'

    $summary = [ordered]@{
        format = 'aiops.logical-restore-evidence/v1'
        verified_at = (Get-Date).ToString('o')
        postgres_image = $PostgresImage
        database_name = $DatabaseName
        migration_count = $migrationFiles.Count
        latest_migration = $latestMigration
        backup = [ordered]@{
            format = 'custom'
            bytes = [int64]$backup.Length
            sha256 = $backupHash
            retained = $false
        }
        source_destroyed_before_restore = $true
        source_snapshot = $sourceSnapshot
        restored_snapshot = $targetSnapshot
    }
} catch {
    $failure = $_
} finally {
    Write-Host '[7/7] Removing containers, anonymous volumes and temporary backup material'
    Remove-DrillContainer -Name $SourceContainer
    Remove-DrillContainer -Name $TargetContainer
    $cleanup.source_container_deleted = -not (Test-ContainerExists -Name $SourceContainer)
    $cleanup.target_container_deleted = -not (Test-ContainerExists -Name $TargetContainer)
    if (-not $cleanup.source_container_deleted) { $cleanupFailures.Add('source container remains') }
    if (-not $cleanup.target_container_deleted) { $cleanupFailures.Add('target container remains') }

    Remove-Item -LiteralPath $TemporaryDirectory -Recurse -Force -ErrorAction SilentlyContinue
    $cleanup.temporary_files_deleted = -not (Test-Path -LiteralPath $TemporaryDirectory)
    if (-not $cleanup.temporary_files_deleted) { $cleanupFailures.Add('temporary backup material remains') }

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
    if (-not $cleanup.process_environment_restored) { $cleanupFailures.Add('PostgreSQL process environment was not restored') }
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
$evidencePath = Join-Path $ArtifactDirectory ("postgres-recovery-{0}.json" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
Write-Utf8File -Path $evidencePath -Contents ($summary | ConvertTo-Json -Depth 10)
Write-Host "Isolated PostgreSQL backup/restore verification passed. Evidence: $evidencePath"
$summary | ConvertTo-Json -Depth 10
