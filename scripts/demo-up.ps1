[CmdletBinding()]
param(
    [string]$ApiBase = 'http://127.0.0.1:8080',
    [string]$Username = 'admin',
    [string]$AdminPassword = $env:AIOPS_ADMIN_PASSWORD,
    [string]$Context = 'kind-aiops-test'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Root = Split-Path -Parent $PSScriptRoot
$ArtifactDirectory = Join-Path $Root '.artifacts\demo'
[IO.Directory]::CreateDirectory($ArtifactDirectory) | Out-Null

if ([string]::IsNullOrWhiteSpace($AdminPassword)) {
    $secure = Read-Host 'AIOps administrator password' -AsSecureString
    $pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
    try {
        $AdminPassword = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer)
    } finally {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer)
    }
}

& (Join-Path $PSScriptRoot 'demo-down.ps1') `
    -ApiBase $ApiBase `
    -Username $Username `
    -AdminPassword $AdminPassword `
    -Context $Context | Write-Host

& (Join-Path $PSScriptRoot 'e2e-kind.ps1') `
    -ApiBase $ApiBase `
    -Username $Username `
    -AdminPassword $AdminPassword `
    -Context $Context `
    -KeepPlatformCluster | Write-Host

$login = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/auth/login" -ContentType 'application/json' -Body (@{
    username = $Username
    password = $AdminPassword
} | ConvertTo-Json)
$headers = @{ Authorization = "Bearer $($login.access_token)" }
$clusters = Invoke-RestMethod -Uri "$ApiBase/api/v1/clusters" -Headers $headers
$cluster = $clusters.items |
    Where-Object { $_.name -like 'demo-kind-*' } |
    Sort-Object id -Descending |
    Select-Object -First 1
if ($null -eq $cluster -or $cluster.status -ne 'ready') {
    throw 'retained demo cluster is missing or not ready'
}

$summary = [ordered]@{
    prepared_at = (Get-Date).ToString('o')
    status = 'demo-ready'
    web_url = 'http://localhost:18080'
    cluster = [ordered]@{
        id = $cluster.id
        name = $cluster.name
        status = $cluster.status
        kubernetes_version = $cluster.kubernetes_version
    }
    expected_diagnoses = 3
    credential_lifetime = '1h'
}
$path = Join-Path $ArtifactDirectory ("demo-ready-{0}.json" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
[IO.File]::WriteAllText($path, ($summary | ConvertTo-Json -Depth 8), [Text.UTF8Encoding]::new($false))
Write-Host "Demo environment ready at http://localhost:18080. Evidence: $path"
$summary | ConvertTo-Json -Depth 8
