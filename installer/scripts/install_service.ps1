# install_service.ps1 — PrintAgent Windows サービス登録・起動スクリプト
# setup.exe の [Run] セクションから呼ばれる。管理者権限が必要。
#
# 引数:
#   -ServiceName  サービス名（例: PrintAgent）
#   -DisplayName  表示名（例: Print Agent）
#   -BinPath      実行ファイルのフルパス（例: C:\Program Files\PrintAgent\print-agent.exe）

param(
    [Parameter(Mandatory)][string]$ServiceName,
    [Parameter(Mandatory)][string]$DisplayName,
    [Parameter(Mandatory)][string]$BinPath
)

# 既存サービスが残っている場合は削除してから再登録
$existing = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($existing) {
    Write-Host "Removing existing service..."
    sc.exe stop $ServiceName 2>$null
    Start-Sleep -Seconds 2
    sc.exe delete $ServiceName
    Start-Sleep -Seconds 2
}

# サービス登録
# start= auto: Windows 起動時に自動起動
Write-Host "Creating service..."
sc.exe create $ServiceName `
    binPath= $BinPath `
    DisplayName= $DisplayName `
    start= auto

if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to create service"
    exit 1
}

# 障害時の自動再起動設定
# 1回目・2回目・3回目の失敗で 5秒後に再起動
sc.exe failure $ServiceName reset= 0 actions= restart/5000/restart/5000/restart/5000

# サービス起動
Write-Host "Starting service..."
sc.exe start $ServiceName

if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to start service"
    exit 1
}

Write-Host "PrintAgent service installed and started successfully."
