[CmdletBinding()]
param(
    [string]$ApiBase = 'http://127.0.0.1:8080',
    [string]$Username = 'admin',
    [string]$AdminPassword = $env:AIOPS_ADMIN_PASSWORD,
    [string]$Context = 'kind-aiops-test',
    [switch]$CleanupDemoResources
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Root = Split-Path -Parent $PSScriptRoot
$ArtifactDirectory = Join-Path $Root '.artifacts\demo'
[IO.Directory]::CreateDirectory($ArtifactDirectory) | Out-Null

function Invoke-KubectlText {
    param([Parameter(Mandatory)] [string[]]$Arguments)
    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        $output = & kubectl --context $Context @Arguments 2>&1
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorAction
    }
    if ($exitCode -ne 0) {
        throw "kubectl $($Arguments -join ' ') failed: $($output -join "`n")"
    }
    return (($output | ForEach-Object { $_.ToString() }) -join "`n").Trim()
}

if ([string]::IsNullOrWhiteSpace($AdminPassword)) {
    $secure = Read-Host 'AIOps administrator password' -AsSecureString
    $pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
    try {
        $AdminPassword = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer)
    } finally {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer)
    }
}

Invoke-RestMethod "$ApiBase/api/v1/health/ready" -TimeoutSec 10 | Out-Null
$login = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/auth/login" -ContentType 'application/json' -Body (@{
    username = $Username
    password = $AdminPassword
} | ConvertTo-Json)
$headers = @{ Authorization = "Bearer $($login.access_token)" }
$clusters = Invoke-RestMethod -Uri "$ApiBase/api/v1/clusters" -Headers $headers
$targets = @($clusters.items | Where-Object { $_.name -like 'demo-kind-*' })
$deleted = [Collections.Generic.List[object]]::new()

foreach ($cluster in $targets) {
    Invoke-WebRequest -UseBasicParsing -Method Delete -Uri "$ApiBase/api/v1/clusters/$($cluster.id)" -Headers $headers | Out-Null
    $deleted.Add([ordered]@{ id = $cluster.id; name = $cluster.name })
}

if ($CleanupDemoResources) {
    Invoke-KubectlText @('delete', '-k', (Join-Path $Root 'deploy\managed-cluster'), '--ignore-not-found=true') | Write-Host
    Invoke-KubectlText @('delete', 'namespace', 'aiops-demo', '--ignore-not-found=true') | Write-Host
}

$summary = [ordered]@{
    cleaned_at = (Get-Date).ToString('o')
    deleted_platform_clusters = $deleted
    deleted_cluster_count = $deleted.Count
    kubernetes_resources_deleted = [bool]$CleanupDemoResources
}
$path = Join-Path $ArtifactDirectory ("demo-down-{0}.json" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
[IO.File]::WriteAllText($path, ($summary | ConvertTo-Json -Depth 8), [Text.UTF8Encoding]::new($false))
Write-Host "Demo cleanup completed. Evidence: $path"
$summary | ConvertTo-Json -Depth 8
