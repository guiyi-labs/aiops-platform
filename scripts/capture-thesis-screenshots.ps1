[CmdletBinding()]
param(
    [string]$WebBase = 'http://127.0.0.1:18080',
    [string]$Username = 'admin',
    [string]$AdminPassword = $env:AIOPS_ADMIN_PASSWORD
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Root = Split-Path -Parent $PSScriptRoot
$OutputDirectory = Join-Path $Root 'docs\thesis\screenshots'

if ([string]::IsNullOrWhiteSpace($AdminPassword)) {
    $secure = Read-Host 'AIOps administrator password' -AsSecureString
    $pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
    try {
        $AdminPassword = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer)
    } finally {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer)
    }
}

$nodeCommand = Get-Command node -ErrorAction SilentlyContinue
$node = if ($null -ne $nodeCommand) { $nodeCommand.Source } else { $null }
if ($null -eq $node) {
    $candidates = @(
        (Join-Path $env:ProgramFiles 'nodejs\node.exe'),
        (Join-Path $env:USERPROFILE '.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe')
    )
    $node = $candidates | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
}
if ($null -eq $node) {
    throw 'Node.js is required to capture thesis screenshots'
}

$previousPassword = $env:AIOPS_CAPTURE_PASSWORD
$previousUsername = $env:AIOPS_CAPTURE_USERNAME
$previousWebBase = $env:AIOPS_CAPTURE_WEB_BASE
$previousOutput = $env:AIOPS_CAPTURE_OUTPUT
try {
    $env:AIOPS_CAPTURE_PASSWORD = $AdminPassword
    $env:AIOPS_CAPTURE_USERNAME = $Username
    $env:AIOPS_CAPTURE_WEB_BASE = $WebBase
    $env:AIOPS_CAPTURE_OUTPUT = $OutputDirectory
    & $node (Join-Path $PSScriptRoot 'capture-thesis-screenshots.mjs')
    if ($LASTEXITCODE -ne 0) {
        throw "screenshot capture failed with exit code $LASTEXITCODE"
    }
} finally {
    $env:AIOPS_CAPTURE_PASSWORD = $previousPassword
    $env:AIOPS_CAPTURE_USERNAME = $previousUsername
    $env:AIOPS_CAPTURE_WEB_BASE = $previousWebBase
    $env:AIOPS_CAPTURE_OUTPUT = $previousOutput
}
