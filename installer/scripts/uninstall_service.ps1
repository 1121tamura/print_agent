# uninstall_service.ps1 — PrintAgent Windows サービス削除スクリプト
# setup.exe のアンインストール時に呼ばれる。管理者権限が必要。
#
# 引数:
#   -ServiceName  サービス名（例: PrintAgent）

param(
    [Parameter(Mandatory)][string]$ServiceName
)

$existing = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if (-not $existing) {
    Write-Host "Service not found. Skipping."
    exit 0
}

# サービス停止
Write-Host "Stopping service..."
sc.exe stop $ServiceName 2>$null
Start-Sleep -Seconds 3

# サービス削除
Write-Host "Deleting service..."
sc.exe delete $ServiceName

if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to delete service"
    exit 1
}

Write-Host "PrintAgent service removed successfully."
