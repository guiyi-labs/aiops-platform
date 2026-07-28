[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Root = Split-Path -Parent $PSScriptRoot
$ArtifactDirectory = Join-Path $Root '.artifacts\identity-readiness'
$RunID = '{0}-{1}' -f (Get-Date -Format 'yyyyMMdd-HHmmss'), ([guid]::NewGuid().ToString('N').Substring(0, 6))
$Image = "aiops-identity-readiness:$RunID"
$TemporaryRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$TemporaryDirectory = [IO.Path]::GetFullPath((Join-Path $TemporaryRoot "aiops-identity-readiness-$RunID"))
$FixtureDirectory = Join-Path $Root 'backend\internal\identityreadiness\testdata'

if (-not $TemporaryDirectory.StartsWith($TemporaryRoot, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'identity readiness temporary directory escaped the system temporary root'
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

function Invoke-Readiness {
    param([Parameter(Mandatory)] [string]$Policy, [Parameter(Mandatory)] [string]$Discovery, [int]$ExpectedExitCode)
    $arguments = @(
        'run', '--rm', '--network', 'none',
        '--mount', "type=bind,source=$TemporaryDirectory,target=/work,readonly",
        '--entrypoint', '/app/identity-readiness', $Image,
        "--policy-file=/work/$Policy", "--discovery-file=/work/$Discovery", '--jwks-file=/work/jwks.json'
    )
    $result = Invoke-NativeResult -File 'docker' -Arguments $arguments
    if ($result.ExitCode -ne $ExpectedExitCode) {
        throw "identity readiness exit code mismatch; expected $ExpectedExitCode, got $($result.ExitCode): $($result.Output)"
    }
    if ([string]::IsNullOrWhiteSpace($result.Output)) { throw 'identity readiness returned an empty report' }
    return ($result.Output | ConvertFrom-Json)
}

$failure = $null
$summary = $null
$cleanup = [ordered]@{ temporary_image_deleted = $false; temporary_files_deleted = $false }
try {
    Get-Command docker -ErrorAction Stop | Out-Null
    foreach ($name in @('policy.json', 'discovery.json', 'jwks.json')) {
        [IO.File]::Copy((Join-Path $FixtureDirectory $name), (Join-Path $TemporaryDirectory $name), $false)
    }

    Write-Host '[1/5] Building the production image with the offline readiness command'
    $build = Invoke-NativeResult -File 'docker' -Arguments @('build', '--tag', $Image, '--build-arg', "VERSION=identity-readiness-$RunID", (Join-Path $Root 'backend'))
    if ($build.ExitCode -ne 0) { throw "identity readiness image build failed: $($build.Output)" }

    Write-Host '[2/5] Accepting the complete synthetic OIDC and MFA contract without network access'
    $accepted = Invoke-Readiness -Policy 'policy.json' -Discovery 'discovery.json' -ExpectedExitCode 0
    if (-not $accepted.ready -or [int]$accepted.failed -ne 0 -or [int]$accepted.passed -ne 14) {
        throw 'complete synthetic provider contract was not accepted'
    }

    Write-Host '[3/5] Rejecting an issuer mismatch and missing PKCE S256 support'
    $badDiscovery = Get-Content -LiteralPath (Join-Path $TemporaryDirectory 'discovery.json') -Raw | ConvertFrom-Json
    $badDiscovery.issuer = 'https://unapproved.example.test'
    $badDiscovery.code_challenge_methods_supported = @('plain')
    [IO.File]::WriteAllText((Join-Path $TemporaryDirectory 'discovery-bad.json'), ($badDiscovery | ConvertTo-Json -Depth 10), [Text.UTF8Encoding]::new($false))
    $rejectedDiscovery = Invoke-Readiness -Policy 'policy.json' -Discovery 'discovery-bad.json' -ExpectedExitCode 1
    $failedDiscoveryCodes = @($rejectedDiscovery.checks | Where-Object { -not $_.passed } | ForEach-Object { $_.code })
    if ($failedDiscoveryCodes -notcontains 'oidc.issuer' -or $failedDiscoveryCodes -notcontains 'oidc.authorization_flow') {
        throw 'issuer/PKCE downgrade did not fail the expected checks'
    }

    Write-Host '[4/5] Rejecting disabled MFA and automatic email account linking'
    $badPolicy = Get-Content -LiteralPath (Join-Path $TemporaryDirectory 'policy.json') -Raw | ConvertFrom-Json
    $badPolicy.mfa.required = $false
    $badPolicy.account_linking.auto_link_by_email = $true
    [IO.File]::WriteAllText((Join-Path $TemporaryDirectory 'policy-bad.json'), ($badPolicy | ConvertTo-Json -Depth 10), [Text.UTF8Encoding]::new($false))
    $rejectedPolicy = Invoke-Readiness -Policy 'policy-bad.json' -Discovery 'discovery.json' -ExpectedExitCode 1
    $failedPolicyCodes = @($rejectedPolicy.checks | Where-Object { -not $_.passed } | ForEach-Object { $_.code })
    if ($failedPolicyCodes -notcontains 'identity.mfa' -or $failedPolicyCodes -notcontains 'identity.account_linking') {
        throw 'MFA/account-linking downgrade did not fail the expected checks'
    }

    $summary = [ordered]@{
        verified_at = (Get-Date).ToString('o')
        network_disabled = $true
        accepted_checks = [int]$accepted.passed
        issuer_mismatch_rejected = $true
        pkce_downgrade_rejected = $true
        missing_mfa_rejected = $true
        automatic_email_linking_rejected = $true
    }
} catch {
    $failure = $_
} finally {
    Write-Host '[5/5] Removing the temporary image and all provider snapshots'
    $remove = Invoke-NativeResult -File 'docker' -Arguments @('image', 'rm', '--force', $Image)
    $remaining = Invoke-NativeResult -File 'docker' -Arguments @('image', 'ls', '--quiet', $Image)
    $cleanup.temporary_image_deleted = [string]::IsNullOrWhiteSpace($remaining.Output)
    Remove-Item -LiteralPath $TemporaryDirectory -Recurse -Force -ErrorAction SilentlyContinue
    $cleanup.temporary_files_deleted = -not (Test-Path -LiteralPath $TemporaryDirectory)
}

if (-not $cleanup.temporary_image_deleted -or -not $cleanup.temporary_files_deleted) {
    $cleanupFailure = 'identity readiness cleanup was incomplete'
    if ($null -ne $failure) { throw "$($failure.Exception.Message); $cleanupFailure" }
    throw $cleanupFailure
}
if ($null -ne $failure) { throw $failure }
$summary.cleanup = $cleanup
$evidencePath = Join-Path $ArtifactDirectory ("identity-readiness-{0}.json" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
[IO.File]::WriteAllText($evidencePath, ($summary | ConvertTo-Json -Depth 10), [Text.UTF8Encoding]::new($false))
Write-Host "Identity readiness evaluation passed. Evidence: $evidencePath"
$summary | ConvertTo-Json -Depth 10
