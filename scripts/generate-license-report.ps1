[CmdletBinding()]
param(
    [string]$OutputPath
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Root = Split-Path -Parent $PSScriptRoot
if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path $Root 'docs\thesis\dependency-licenses.md'
}

function Invoke-NativeText {
    param(
        [Parameter(Mandatory)] [string]$File,
        [Parameter(Mandatory)] [string[]]$Arguments,
        [string]$WorkingDirectory = $Root
    )

    Push-Location $WorkingDirectory
    try {
        $previousErrorAction = $ErrorActionPreference
        try {
            $ErrorActionPreference = 'Continue'
            $output = & $File @Arguments 2>&1
            $exitCode = $LASTEXITCODE
        } finally {
            $ErrorActionPreference = $previousErrorAction
        }
        if ($exitCode -ne 0) {
            throw "$File $($Arguments -join ' ') failed: $($output -join "`n")"
        }
        return @($output | ForEach-Object { $_.ToString() })
    } finally {
        Pop-Location
    }
}

function Enable-NodePath {
    if (Get-Command node -ErrorAction SilentlyContinue) {
        return
    }
    $pnpm = Get-Command pnpm -ErrorAction Stop
    $dependencies = Split-Path -Parent (Split-Path -Parent (Split-Path -Parent $pnpm.Source))
    $candidate = Join-Path $dependencies 'node\bin'
    if (Test-Path -LiteralPath (Join-Path $candidate 'node.exe')) {
        $env:Path = "$candidate;$env:Path"
    }
    Get-Command node -ErrorAction Stop | Out-Null
}

function Get-LicenseName {
    param([string]$Text)

    if ([string]::IsNullOrWhiteSpace($Text)) { return 'UNKNOWN' }
    if ($Text -match 'Apache License\s+Version 2\.0') { return 'Apache-2.0' }
    if ($Text -match 'Mozilla Public License\s+Version 2\.0') { return 'MPL-2.0' }
    if ($Text -match 'GNU LESSER GENERAL PUBLIC LICENSE') { return 'LGPL' }
    if ($Text -match 'GNU GENERAL PUBLIC LICENSE') { return 'GPL' }
    if ($Text -match 'Permission to use, copy, modify, and/or distribute this software for any purpose with or without fee') { return 'ISC' }
    if ($Text -match 'MIT License' -or $Text -match 'Permission is hereby granted, free of charge') { return 'MIT' }
    if ($Text -match 'Redistributions of source code must retain') {
        if ($Text -match 'Neither the name') { return 'BSD-3-Clause' }
        return 'BSD-2-Clause'
    }
    return 'SEE-LICENSE'
}

function Escape-MarkdownCell {
    param([string]$Value)
    return ($Value -replace '\|', '\|') -replace "`r?`n", ' '
}

Enable-NodePath
$pnpm = (Get-Command pnpm -ErrorAction Stop).Source
$goImage = 'aiops-platform-go-source:local'

Write-Host 'Building the reproducible Go dependency inventory image...'
Invoke-NativeText 'docker' @('build', '--target', 'source', '-t', $goImage, '.') (Join-Path $Root 'backend') | Out-Null

$moduleCache = Join-Path $Root '.tools\gomodcache'
if (-not (Test-Path -LiteralPath $moduleCache)) {
    throw "Go module cache not found at $moduleCache; run a backend build before generating the report."
}
$goTemplate = '{{with .Module}}{{if not .Main}}{{.Path}}|{{.Version}}|{{.Dir}}{{end}}{{end}}'
$goInventory = Invoke-NativeText 'docker' @(
    'run', '--rm', '--network', 'none', $goImage,
    'go', 'list', '-deps', '-f', $goTemplate, './cmd/server'
)
$goRows = foreach ($line in $goInventory | Sort-Object -Unique) {
    $parts = $line -split '\|', 3
    if ($parts.Count -ne 3) { continue }
    $relativeModuleDirectory = $parts[2] -replace '^/go/pkg/mod/', '' -replace '/', '\'
    $moduleDirectory = Join-Path $moduleCache $relativeModuleDirectory
    $licenseFile = Get-ChildItem -LiteralPath $moduleDirectory -File -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -match '^(LICENSE|COPYING)' } |
        Select-Object -First 1
    if ($null -eq $licenseFile) {
        $licenseFile = Get-ChildItem -LiteralPath $moduleDirectory -Recurse -File -ErrorAction SilentlyContinue |
            Where-Object { $_.Name -match '^(LICENSE|COPYING)' } |
            Select-Object -First 1
    }
    $licenseText = if ($null -ne $licenseFile) { [IO.File]::ReadAllText($licenseFile.FullName) } else { '' }
    [pscustomobject]@{
        Name = $parts[0]
        Version = $parts[1]
        License = Get-LicenseName $licenseText
    }
}

Write-Host 'Reading the pnpm production dependency inventory...'
$frontendJson = (Invoke-NativeText $pnpm @('licenses', 'list', '--prod', '--json') (Join-Path $Root 'frontend')) -join "`n"
$frontendGroups = $frontendJson | ConvertFrom-Json
$frontendRows = foreach ($group in $frontendGroups.PSObject.Properties) {
    foreach ($package in $group.Value) {
        [pscustomobject]@{
            Name = $package.name
            Version = @($package.versions) -join ', '
            License = $group.Name
        }
    }
}

$lines = [Collections.Generic.List[string]]::new()
$lines.Add('# Dependency License Report')
$lines.Add('')
$lines.Add("Generated: $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss zzz')")
$lines.Add('')
$lines.Add('This inventory covers dependencies reachable from the backend server binary and the pnpm production dependency graph. It is an engineering inventory, not legal advice. Re-run `scripts/generate-license-report.ps1` after dependency changes.')
$lines.Add('')
$lines.Add('## Go production dependencies')
$lines.Add('')
$lines.Add("Count: $(@($goRows).Count)")
$lines.Add('')
$lines.Add('| Package | Version | License |')
$lines.Add('|---|---:|---|')
foreach ($row in $goRows | Sort-Object Name, Version) {
    $lines.Add("| $(Escape-MarkdownCell $row.Name) | $(Escape-MarkdownCell $row.Version) | $(Escape-MarkdownCell $row.License) |")
}
$lines.Add('')
$lines.Add('## Frontend production dependencies')
$lines.Add('')
$lines.Add("Count: $(@($frontendRows).Count)")
$lines.Add('')
$lines.Add('| Package | Version | License |')
$lines.Add('|---|---:|---|')
foreach ($row in $frontendRows | Sort-Object Name, Version) {
    $lines.Add("| $(Escape-MarkdownCell $row.Name) | $(Escape-MarkdownCell $row.Version) | $(Escape-MarkdownCell $row.License) |")
}
$lines.Add('')
$lines.Add('## Review policy')
$lines.Add('')
$lines.Add('- `UNKNOWN`, `SEE-LICENSE`, GPL, LGPL or other reciprocal licenses require manual review before redistribution.')
$lines.Add('- This report records third-party dependencies only; reference projects are documented separately and are not copied into the application source tree.')

$directory = Split-Path -Parent $OutputPath
[IO.Directory]::CreateDirectory($directory) | Out-Null
[IO.File]::WriteAllLines($OutputPath, $lines, [Text.UTF8Encoding]::new($false))
Write-Host "License report written to $OutputPath"
