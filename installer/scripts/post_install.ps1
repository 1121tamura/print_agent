# post_install.ps1 — インストール後の確認スクリプト
# サービスが正常に起動しているかを確認する。
#
# 引数:
#   -ServiceName  サービス名
#   -DataDir      データディレクトリ（config.yaml / tmp / logs の親）
#   -LocalAPIPort Local API のポート番号
#   -MaxRetry     /health リトライ回数
#   -RetryDelay   リトライ間隔（秒）

param(
    [Parameter(Mandatory)][string]$ServiceName,
    [Parameter(Mandatory)][string]$DataDir,
    [Parameter(Mandatory)][int]$LocalAPIPort,
    [int]$MaxRetry    = 10,
    [int]$RetryDelay  = 2
)

$HealthURL = "http://127.0.0.1:$LocalAPIPort/health"

# サービス状態確認
Write-Host "Checking service status..."
$service = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if (-not $service) {
    Write-Error "Service '$ServiceName' not found."
    exit 1
}
if ($service.Status -ne "Running") {
    Write-Error "Service is not running. Status: $($service.Status)"
    exit 1
}
Write-Host "Service status: Running"

# config.yaml の存在確認
$ConfigPath = "$DataDir\config.yaml"
if (-not (Test-Path $ConfigPath)) {
    Write-Error "config.yaml not found: $ConfigPath"
    exit 1
}
Write-Host "config.yaml: OK"

# tmp / logs ディレクトリの確認
foreach ($dir in @("$DataDir\tmp", "$DataDir\logs")) {
    if (-not (Test-Path $dir)) {
        Write-Error "Directory not found: $dir"
        exit 1
    }
    Write-Host "Directory $dir : OK"
}

# /health エンドポイントの確認（起動完了まで最大リトライ）
Write-Host "Checking Local API /health..."
$ok = $false
for ($i = 1; $i -le $MaxRetry; $i++) {
    try {
        $resp = Invoke-WebRequest -Uri $HealthURL -UseBasicParsing -TimeoutSec 3
        if ($resp.StatusCode -eq 200) {
            Write-Host "/health: OK"
            $ok = $true
            break
        }
    } catch {
        Write-Host "Retry $i/$MaxRetry ..."
        Start-Sleep -Seconds $RetryDelay
    }
}

if (-not $ok) {
    Write-Error "/health did not respond after $MaxRetry retries."
    exit 1
}

Write-Host ""
Write-Host "PrintAgent installation verified successfully."
