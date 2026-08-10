[CmdletBinding()]
param(
    [string]$PostgresImage = 'pgvector/pgvector:0.8.1-pg17',
    [int]$ReadyTimeoutSeconds = 120,
    [string]$BackendImage
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Root = Split-Path -Parent $PSScriptRoot
$ArtifactDirectory = Join-Path $Root '.artifacts\audit-archive'
$RunID = '{0}-{1}' -f (Get-Date -Format 'yyyyMMdd-HHmmss'), ([guid]::NewGuid().ToString('N').Substring(0, 6))
$Network = "aiops-audit-archive-$RunID"
$DatabaseContainer = "aiops-audit-db-$RunID"
$BuildBackendImage = [string]::IsNullOrWhiteSpace($BackendImage)
if ($BuildBackendImage) {
    $BackendImage = "aiops-audit-archive:$RunID"
}
$DatabaseName = 'aiops_audit_archive'
$DatabaseUser = 'aiops_audit'
$TemporaryRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$TemporaryDirectory = [IO.Path]::GetFullPath((Join-Path $TemporaryRoot "aiops-audit-archive-$RunID"))
$ContainerUser = $null

if (-not $TemporaryDirectory.StartsWith($TemporaryRoot, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'audit archive temporary directory escaped the system temporary root'
}
[IO.Directory]::CreateDirectory($ArtifactDirectory) | Out-Null
[IO.Directory]::CreateDirectory($TemporaryDirectory) | Out-Null

function Invoke-NativeResult {
    param([Parameter(Mandatory)] [string]$File, [Parameter(Mandatory)] [string[]]$Arguments)

    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $output = & $File @Arguments 2>&1
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorAction
    }
    return [pscustomobject]@{ ExitCode = $exitCode; Output = (($output | ForEach-Object { $_.ToString() }) -join [Environment]::NewLine).Trim() }
}

function Invoke-NativeText {
    param([Parameter(Mandatory)] [string]$File, [Parameter(Mandatory)] [string[]]$Arguments, [switch]$AllowFailure)

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
    try { $generator.GetBytes($buffer) } finally { $generator.Dispose() }
    return [Convert]::ToBase64String($buffer)
}

function New-RandomHex {
    param([int]$Bytes = 24)

    $buffer = [byte[]]::new($Bytes)
    $generator = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $generator.GetBytes($buffer) } finally { $generator.Dispose() }
    return (($buffer | ForEach-Object { $_.ToString('x2') }) -join '')
}

function Assert-Equal {
    param($Actual, $Expected, [Parameter(Mandatory)] [string]$Message)
    if ($Actual -ne $Expected) { throw "$Message; expected $Expected, got $Actual" }
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
        $output = Invoke-NativeText -File 'docker' -Arguments @('exec', $DatabaseContainer, 'pg_isready', '--username', $DatabaseUser, '--dbname', $DatabaseName) -AllowFailure
        if ($output -match 'accepting connections') { return }
        Start-Sleep -Seconds 2
    } while ((Get-Date) -lt $deadline)
    throw 'isolated PostgreSQL did not become ready'
}

function Invoke-PsqlScalar {
    param([Parameter(Mandatory)] [string]$Query)
    return (Invoke-NativeText -File 'docker' -Arguments @(
        'exec', $DatabaseContainer, 'psql', '--no-psqlrc', '--set', 'ON_ERROR_STOP=1', '--tuples-only', '--no-align',
        '--username', $DatabaseUser, '--dbname', $DatabaseName, '--command', $Query
    )).Trim()
}

function Invoke-AuditArchive {
    param([Parameter(Mandatory)] [string[]]$CommandArguments, [int]$ExpectedExitCode = 0, [switch]$ReadOnlyMount)

    $mountMode = if ($ReadOnlyMount) { ',readonly' } else { '' }
    $arguments = @('run', '--rm')
    if ($null -ne $ContainerUser) { $arguments += @('--user', $ContainerUser) }
    $arguments += @('--network', $Network, '--mount', "type=bind,source=$TemporaryDirectory,target=/work$mountMode")
    if (-not $ReadOnlyMount) {
        $arguments += @('--env', 'APP_ENV', '--env', 'DATABASE_URL', '--env', 'JWT_SIGNING_KEY', '--env', 'BOOTSTRAP_ADMIN_PASSWORD', '--env', 'CREDENTIAL_ENCRYPTION_KEY', '--env', 'AI_ENABLED', '--env', 'NOTIFICATION_ENABLED')
    }
    $arguments += @('--entrypoint', '/app/audit-archive', $BackendImage) + $CommandArguments
    $result = Invoke-NativeResult -File 'docker' -Arguments $arguments
    if ($result.ExitCode -ne $ExpectedExitCode) {
        throw "audit archive command exit code mismatch; expected $ExpectedExitCode, got $($result.ExitCode): $($result.Output)"
    }
    return $result.Output
}

$environmentNames = @('APP_ENV', 'POSTGRES_DB', 'POSTGRES_USER', 'POSTGRES_PASSWORD', 'DATABASE_URL', 'JWT_SIGNING_KEY', 'BOOTSTRAP_ADMIN_PASSWORD', 'CREDENTIAL_ENCRYPTION_KEY', 'AI_ENABLED', 'NOTIFICATION_ENABLED')
$previousEnvironment = [ordered]@{}
foreach ($name in $environmentNames) { $previousEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }

$failure = $null
$summary = $null
$cleanupFailures = [Collections.Generic.List[string]]::new()
$cleanup = [ordered]@{ database_container_deleted = $false; network_deleted = $false; temporary_image_deleted = $false; temporary_files_deleted = $false; process_environment_restored = $false }

try {
    Get-Command docker -ErrorAction Stop | Out-Null
    if ([Environment]::OSVersion.Platform -eq [PlatformID]::Unix) {
        $uid = Invoke-NativeText -File 'id' -Arguments @('-u')
        $gid = Invoke-NativeText -File 'id' -Arguments @('-g')
        $ContainerUser = "${uid}:${gid}"
    }
    $env:APP_ENV = 'production'
    $env:POSTGRES_DB = $DatabaseName
    $env:POSTGRES_USER = $DatabaseUser
    $env:POSTGRES_PASSWORD = New-RandomHex
    $env:DATABASE_URL = "postgres://${DatabaseUser}:$($env:POSTGRES_PASSWORD)@${DatabaseContainer}:5432/${DatabaseName}?sslmode=disable"
    $env:JWT_SIGNING_KEY = New-RandomHex -Bytes 32
    $env:BOOTSTRAP_ADMIN_PASSWORD = New-RandomHex
    $env:CREDENTIAL_ENCRYPTION_KEY = New-RandomBase64
    $env:AI_ENABLED = 'false'
    $env:NOTIFICATION_ENABLED = 'false'

    if ($BuildBackendImage) {
        Write-Host '[1/6] Building isolated audit archive image'
        Invoke-NativeText -File 'docker' -Arguments @('build', '--tag', $BackendImage, '--build-arg', "VERSION=audit-archive-$RunID", (Join-Path $Root 'backend')) | Out-Null
    } else {
        Write-Host "[1/6] Reusing prebuilt backend image: $BackendImage"
        Invoke-NativeText -File 'docker' -Arguments @('image', 'inspect', $BackendImage) | Out-Null
    }
    Write-Host '[2/6] Starting isolated PostgreSQL'
    Invoke-NativeText -File 'docker' -Arguments @('network', 'create', '--label', 'io.guiyi.aiops.purpose=audit-archive-drill', $Network) | Out-Null
    Invoke-NativeText -File 'docker' -Arguments @('run', '--detach', '--name', $DatabaseContainer, '--label', 'io.guiyi.aiops.purpose=audit-archive-drill', '--network', $Network, '--env', 'POSTGRES_DB', '--env', 'POSTGRES_USER', '--env', 'POSTGRES_PASSWORD', $PostgresImage) | Out-Null
    Wait-PostgresReady

    Write-Host '[3/6] Seeding two synthetic sanitized audit rows and an ephemeral signing seed'
    [IO.File]::WriteAllText((Join-Path $TemporaryDirectory 'signing.key'), (New-RandomBase64 -Bytes 32), [Text.UTF8Encoding]::new($false))
    # The empty bounded selection applies migrations without producing an archive.
    $migrationProbe = Invoke-AuditArchive -CommandArguments @('--archive=/work/unused.json', '--private-key-file=/work/signing.key', '--from-id=1', '--to-id=1') -ExpectedExitCode 1
    if ($migrationProbe -notmatch 'audit archive write failed') {
        throw "audit archive migration probe failed before empty-range validation: $migrationProbe"
    }
    Invoke-PsqlScalar -Query @'
INSERT INTO audit_logs (action, resource_type, resource_namespace, resource_name, result, request_id, details, actor_name, status_code, ip_address, user_agent) VALUES
('audit.archive.synthetic', 'AuditLog', 'security', 'first', 'success', 'audit-archive-fixture-1', jsonb_build_object('fixture', TRUE, 'kind', 'synthetic'), 'Archive Fixture', 200, '127.0.0.1', 'audit-e2e'),
('audit.archive.synthetic', 'AuditLog', 'security', 'second', 'success', 'audit-archive-fixture-2', jsonb_build_object('fixture', TRUE, 'kind', 'synthetic'), 'Archive Fixture', 200, '127.0.0.1', 'audit-e2e')
'@ | Out-Null
    $firstID = [int64](Invoke-PsqlScalar -Query "SELECT MIN(id) FROM audit_logs WHERE request_id LIKE 'audit-archive-fixture-%'")
    $lastID = [int64](Invoke-PsqlScalar -Query "SELECT MAX(id) FROM audit_logs WHERE request_id LIKE 'audit-archive-fixture-%'")
    $publicResult = Invoke-AuditArchive -ReadOnlyMount -CommandArguments @('--print-public-key', '--private-key-file=/work/signing.key')
    $trustedPublicKey = ($publicResult | ConvertFrom-Json).public_key
    [IO.File]::WriteAllText((Join-Path $TemporaryDirectory 'trusted-public.key'), $trustedPublicKey, [Text.UTF8Encoding]::new($false))

    Write-Host '[4/6] Creating and verifying a bounded signed archive'
    $archiveResult = Invoke-AuditArchive -CommandArguments @('--archive=/work/audit.json', '--private-key-file=/work/signing.key', "--from-id=$firstID", "--to-id=$lastID", '--max-records=2') | ConvertFrom-Json
    Assert-Equal -Actual ([int]$archiveResult.record_count) -Expected 2 -Message 'archive record count mismatch'
    $verification = Invoke-AuditArchive -ReadOnlyMount -CommandArguments @('--verify', '--archive=/work/audit.json', '--trusted-public-key-file=/work/trusted-public.key') | ConvertFrom-Json
    Assert-Equal -Actual ([int]$verification.record_count) -Expected 2 -Message 'verification record count mismatch'

    Write-Host '[5/6] Refusing an over-limit selection before output and rejecting tampering'
    Invoke-PsqlScalar -Query "INSERT INTO audit_logs (action, result, request_id, details, actor_name, status_code) VALUES ('audit.archive.synthetic', 'success', 'audit-archive-fixture-3', jsonb_build_object('fixture', TRUE), 'Archive Fixture', 200)" | Out-Null
    $overflowLastID = [int64](Invoke-PsqlScalar -Query "SELECT MAX(id) FROM audit_logs WHERE request_id LIKE 'audit-archive-fixture-%'")
    Invoke-AuditArchive -CommandArguments @('--archive=/work/overflow.json', '--private-key-file=/work/signing.key', "--from-id=$firstID", "--to-id=$overflowLastID", '--max-records=2') -ExpectedExitCode 1 | Out-Null
    if ((Test-Path -LiteralPath (Join-Path $TemporaryDirectory 'overflow.json')) -or (Test-Path -LiteralPath (Join-Path $TemporaryDirectory 'overflow.json.manifest.json'))) { throw 'record limit created output files' }
    $archivePath = Join-Path $TemporaryDirectory 'audit.json'
    $bytes = [IO.File]::ReadAllBytes($archivePath)
    $bytes[0] = $bytes[0] -bxor 1
    [IO.File]::WriteAllBytes($archivePath, $bytes)
    Invoke-AuditArchive -ReadOnlyMount -CommandArguments @('--verify', '--archive=/work/audit.json', '--trusted-public-key-file=/work/trusted-public.key') -ExpectedExitCode 1 | Out-Null

    $summary = [ordered]@{ verified_at = (Get-Date).ToString('o'); postgres_image = $PostgresImage; records = 2; trusted_verification = $true; over_limit_refused_before_output = $true; byte_tamper_rejected = $true }
} catch { $failure = $_ } finally {
    Write-Host '[6/6] Removing isolated database, network, image and ephemeral key/archive files'
    Remove-OwnedContainer -Name $DatabaseContainer
    $cleanup.database_container_deleted = -not (Test-ContainerExists -Name $DatabaseContainer)
    Invoke-NativeText -File 'docker' -Arguments @('network', 'rm', $Network) -AllowFailure | Out-Null
    $cleanup.network_deleted = [string]::IsNullOrWhiteSpace((Invoke-NativeText -File 'docker' -Arguments @('network', 'ls', '--quiet', '--filter', "name=^$Network`$") -AllowFailure))
    Invoke-NativeText -File 'docker' -Arguments @('image', 'rm', '--force', $BackendImage) -AllowFailure | Out-Null
    $cleanup.temporary_image_deleted = [string]::IsNullOrWhiteSpace((Invoke-NativeText -File 'docker' -Arguments @('image', 'ls', '--quiet', $BackendImage) -AllowFailure))
    Remove-Item -LiteralPath $TemporaryDirectory -Recurse -Force -ErrorAction SilentlyContinue
    $cleanup.temporary_files_deleted = -not (Test-Path -LiteralPath $TemporaryDirectory)
    foreach ($name in $previousEnvironment.Keys) { [Environment]::SetEnvironmentVariable($name, $previousEnvironment[$name], 'Process') }
    $cleanup.process_environment_restored = @($previousEnvironment.Keys | Where-Object {
        $actual = [Environment]::GetEnvironmentVariable($_, 'Process')
        $expected = $previousEnvironment[$_]
        if ([string]::IsNullOrEmpty($expected)) {
            -not [string]::IsNullOrEmpty($actual)
        } else {
            $actual -cne $expected
        }
    }).Count -eq 0
    foreach ($entry in $cleanup.GetEnumerator()) { if (-not $entry.Value) { $cleanupFailures.Add("$($entry.Key) was not completed") } }
}

if ($cleanupFailures.Count -gt 0) {
    $cleanupError = $cleanupFailures -join '; '
    if ($null -ne $failure) { throw "$($failure.Exception.Message); $cleanupError" }
    throw $cleanupError
}
if ($null -ne $failure) { throw $failure }
$summary.cleanup = $cleanup
$evidencePath = Join-Path $ArtifactDirectory ("audit-archive-{0}.json" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
[IO.File]::WriteAllText($evidencePath, ($summary | ConvertTo-Json -Depth 10), [Text.UTF8Encoding]::new($false))
Write-Host "Signed audit archive verification passed. Evidence: $evidencePath"
$summary | ConvertTo-Json -Depth 10
