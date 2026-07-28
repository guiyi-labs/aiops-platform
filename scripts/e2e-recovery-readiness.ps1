[CmdletBinding()]
param(
    [string]$EvidencePath
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Root = Split-Path -Parent $PSScriptRoot
$ArtifactDirectory = Join-Path $Root '.artifacts\recovery-readiness'
$RunID = '{0}-{1}' -f (Get-Date -Format 'yyyyMMdd-HHmmss'), ([guid]::NewGuid().ToString('N').Substring(0, 6))
$Image = "aiops-recovery-readiness:$RunID"
$TemporaryRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$TemporaryDirectory = [IO.Path]::GetFullPath((Join-Path $TemporaryRoot "aiops-recovery-readiness-$RunID"))
$FixturePolicy = Join-Path $Root 'backend\internal\recoveryreadiness\testdata\policy.json'

if (-not $TemporaryDirectory.StartsWith($TemporaryRoot, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'recovery readiness temporary directory escaped the system temporary root'
}
if ([string]::IsNullOrWhiteSpace($EvidencePath)) {
    $latest = Get-ChildItem -LiteralPath (Join-Path $Root '.artifacts\postgres-recovery') -Filter 'postgres-recovery-*.json' -File -ErrorAction SilentlyContinue |
        Sort-Object LastWriteTimeUtc -Descending | Select-Object -First 1
    if ($null -eq $latest) { throw 'no logical restore evidence is available; run e2e-postgres-backup-restore.ps1 first' }
    $EvidencePath = $latest.FullName
}
$EvidencePath = [IO.Path]::GetFullPath($EvidencePath)
if (-not (Test-Path -LiteralPath $EvidencePath -PathType Leaf)) { throw 'logical restore evidence file does not exist' }

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

function Invoke-Readiness {
    param([Parameter(Mandatory)] [string]$Policy, [Parameter(Mandatory)] [string]$Evidence, [int]$ExpectedExitCode)
    $result = Invoke-NativeResult -File 'docker' -Arguments @(
        'run', '--rm', '--network', 'none',
        '--mount', "type=bind,source=$TemporaryDirectory,target=/work,readonly",
        '--entrypoint', '/app/recovery-readiness', $Image,
        "--policy-file=/work/$Policy", "--logical-restore-evidence=/work/$Evidence"
    )
    if ($result.ExitCode -ne $ExpectedExitCode) {
        throw "recovery readiness exit code mismatch; expected $ExpectedExitCode, got $($result.ExitCode): $($result.Output)"
    }
    if ([string]::IsNullOrWhiteSpace($result.Output)) { throw 'recovery readiness returned an empty report' }
    return ($result.Output | ConvertFrom-Json)
}

$failure = $null
$summary = $null
$cleanup = [ordered]@{ temporary_image_deleted = $false; temporary_files_deleted = $false }
try {
    Get-Command docker -ErrorAction Stop | Out-Null
    [IO.File]::Copy($FixturePolicy, (Join-Path $TemporaryDirectory 'policy.json'), $false)
    [IO.File]::Copy($EvidencePath, (Join-Path $TemporaryDirectory 'evidence.json'), $false)

    Write-Host '[1/5] Building the production image with the offline recovery readiness command'
    $build = Invoke-NativeResult -File 'docker' -Arguments @('build', '--tag', $Image, '--build-arg', "VERSION=recovery-readiness-$RunID", (Join-Path $Root 'backend'))
    if ($build.ExitCode -ne 0) { throw "recovery readiness image build failed: $($build.Output)" }

    Write-Host '[2/5] Accepting the synthetic recovery policy against the latest real logical restore evidence'
    $accepted = Invoke-Readiness -Policy 'policy.json' -Evidence 'evidence.json' -ExpectedExitCode 0
    if (-not $accepted.ready_for_pitr_ha_implementation -or $accepted.production_recovery_validated -or [int]$accepted.failed -ne 0 -or [int]$accepted.passed -ne 15) {
        throw 'complete recovery implementation contract was not accepted with the required production boundary'
    }

    Write-Host '[3/5] Rejecting inadequate copy count and incomplete cleanup evidence'
    $badPolicy = Get-Content -LiteralPath (Join-Path $TemporaryDirectory 'policy.json') -Raw | ConvertFrom-Json
    $badPolicy.storage.independent_copies = 1
    [IO.File]::WriteAllText((Join-Path $TemporaryDirectory 'policy-bad.json'), ($badPolicy | ConvertTo-Json -Depth 10), [Text.UTF8Encoding]::new($false))
    $rejectedPolicy = Invoke-Readiness -Policy 'policy-bad.json' -Evidence 'evidence.json' -ExpectedExitCode 1
    if (@($rejectedPolicy.checks | Where-Object { -not $_.passed } | ForEach-Object { $_.code }) -notcontains 'recovery.storage') {
        throw 'inadequate recovery copy count did not fail storage readiness'
    }

    Write-Host '[4/5] Rejecting stale evidence and a retained temporary backup'
    $badEvidence = Get-Content -LiteralPath (Join-Path $TemporaryDirectory 'evidence.json') -Raw | ConvertFrom-Json
    $badEvidence.verified_at = '2000-01-01T00:00:00Z'
    $badEvidence.backup.retained = $true
    $badEvidence.cleanup.temporary_files_deleted = $false
    [IO.File]::WriteAllText((Join-Path $TemporaryDirectory 'evidence-bad.json'), ($badEvidence | ConvertTo-Json -Depth 10), [Text.UTF8Encoding]::new($false))
    $rejectedEvidence = Invoke-Readiness -Policy 'policy.json' -Evidence 'evidence-bad.json' -ExpectedExitCode 1
    $failedEvidenceCodes = @($rejectedEvidence.checks | Where-Object { -not $_.passed } | ForEach-Object { $_.code })
    foreach ($code in @('evidence.freshness', 'evidence.logical_restore', 'evidence.cleanup')) {
        if ($failedEvidenceCodes -notcontains $code) { throw "recovery evidence downgrade did not fail $code" }
    }

    $sourceEvidence = Get-Content -LiteralPath (Join-Path $TemporaryDirectory 'evidence.json') -Raw | ConvertFrom-Json
    $summary = [ordered]@{
        verified_at = (Get-Date).ToString('o')
        network_disabled = $true
        accepted_checks = [int]$accepted.passed
        logical_restore_migrations = [int]$sourceEvidence.migration_count
        inadequate_copies_rejected = $true
        stale_evidence_rejected = $true
        retained_backup_rejected = $true
        incomplete_cleanup_rejected = $true
        production_recovery_validated = $false
    }
} catch {
    $failure = $_
} finally {
    Write-Host '[5/5] Removing the temporary image and all copied policy/evidence files'
    Invoke-NativeResult -File 'docker' -Arguments @('image', 'rm', '--force', $Image) | Out-Null
    $remaining = Invoke-NativeResult -File 'docker' -Arguments @('image', 'ls', '--quiet', $Image)
    $cleanup.temporary_image_deleted = [string]::IsNullOrWhiteSpace($remaining.Output)
    Remove-Item -LiteralPath $TemporaryDirectory -Recurse -Force -ErrorAction SilentlyContinue
    $cleanup.temporary_files_deleted = -not (Test-Path -LiteralPath $TemporaryDirectory)
}

if (-not $cleanup.temporary_image_deleted -or -not $cleanup.temporary_files_deleted) {
    $cleanupFailure = 'recovery readiness cleanup was incomplete'
    if ($null -ne $failure) { throw "$($failure.Exception.Message); $cleanupFailure" }
    throw $cleanupFailure
}
if ($null -ne $failure) { throw $failure }
$summary.cleanup = $cleanup
$evidenceOutput = Join-Path $ArtifactDirectory ("recovery-readiness-{0}.json" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
[IO.File]::WriteAllText($evidenceOutput, ($summary | ConvertTo-Json -Depth 10), [Text.UTF8Encoding]::new($false))
Write-Host "Recovery readiness evaluation passed. Evidence: $evidenceOutput"
$summary | ConvertTo-Json -Depth 10
