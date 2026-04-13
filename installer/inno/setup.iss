; setup.iss — Inno Setup スクリプト
; Inno Setup 6.x でコンパイルして setup.exe を生成する。
; ダウンロード: https://jrsoftware.org/isinfo.php

#define AppName      "PrintAgent"
#define AppVersion   "0.1.0"
#define AppExe       "print-agent.exe"
#define InstallDir   "{pf}\PrintAgent"
#define DataDir      "{commonappdata}\PrintAgent"
#define LocalAPIPort "18181"

[Setup]
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher=TODO
DefaultDirName={#InstallDir}
DefaultGroupName={#AppName}
OutputDir=..\..\dist
OutputBaseFilename=setup
Compression=lzma
SolidCompression=yes
; 管理者権限が必要（サービス登録のため）
PrivilegesRequired=admin
; インストール後に setup.exe が終了する前にサービスを起動する
CloseApplications=no

[Languages]
Name: "japanese"; MessagesFile: "compiler:Languages\Japanese.isl"

[Files]
; print-agent.exe を配置
Source: "..\..\dist\{#AppExe}"; DestDir: "{#InstallDir}"; Flags: ignoreversion

; config.yaml のひな形を配置（上書きしない: onlyifdoesntexist）
Source: "..\..\configs\config.example.yaml"; DestDir: "{#DataDir}"; \
    DestName: "config.yaml"; Flags: onlyifdoesntexist

; PowerShell スクリプトを配置（サービス登録・アンインストール用）
Source: "..\scripts\install_service.ps1";   DestDir: "{#InstallDir}"; Flags: ignoreversion
Source: "..\scripts\uninstall_service.ps1"; DestDir: "{#InstallDir}"; Flags: ignoreversion
Source: "..\scripts\post_install.ps1";      DestDir: "{#InstallDir}"; Flags: ignoreversion

[Dirs]
; tmp / logs ディレクトリを作成
Name: "{#DataDir}\tmp"
Name: "{#DataDir}\logs"

[Run]
; サービス登録・起動（setup.iss の #define から引数として渡す）
Filename: "powershell.exe"; \
    Parameters: "-ExecutionPolicy Bypass -File ""{#InstallDir}\install_service.ps1"" -ServiceName ""{#AppName}"" -DisplayName ""{#AppName}"" -BinPath ""{#InstallDir}\{#AppExe}"""; \
    Flags: runhidden waituntilterminated; \
    StatusMsg: "Registering Windows service..."

; インストール後の確認
Filename: "powershell.exe"; \
    Parameters: "-ExecutionPolicy Bypass -File ""{#InstallDir}\post_install.ps1"" -ServiceName ""{#AppName}"" -DataDir ""{#DataDir}"" -LocalAPIPort {#LocalAPIPort}"; \
    Flags: runhidden waituntilterminated; \
    StatusMsg: "Verifying installation..."

[UninstallRun]
; アンインストール時にサービスを停止・削除
Filename: "powershell.exe"; \
    Parameters: "-ExecutionPolicy Bypass -File ""{#InstallDir}\uninstall_service.ps1"" -ServiceName ""{#AppName}"""; \
    Flags: runhidden waituntilterminated
