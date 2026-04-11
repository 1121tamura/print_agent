# print-agent/CLAUDE.md

## 概要

print-agent は、Windows端末に常駐し、Redis Streams 経由で印刷ジョブを受信し、Backend API から PDF を取得して、ローカルプリンターへ印刷する製品である。

県下関連会社の購買システム（数百台）での運用を前提とし、シンプルさと保守性を優先して実装する。

---

# 目的

* Windowsサービスとして常駐する
* Redis Streams から自端末向け印刷ジョブを受信する
* Backend API からジョブ詳細と PDF を取得する
* ローカルプリンターへ印刷する
* 印刷結果を Backend に返却する
* localhost API を提供する
* setup.exe でインストールできるようにする
* インストール時に Windowsサービスとして登録し、自動起動できるようにする

---

# スコープ

## 含むもの

* Agent本体
* Windowsサービス化
* Redis Consumer
* 印刷ワーカー
* Printer制御
* Local API
* Config管理
* Logging
* Installer関連ファイル
* サービス登録と自動起動設定
* インストール後の起動確認

## 含まないもの

* 業務ロジック
* 帳票生成
* Web画面
* 本番業務API

---

# 技術方針

* 言語: Go
* 実行形態: Windowsサービス
* 配布: setup.exe / Inno Setup
* 通信: HTTP / Redis Streams
* 対応OS: Windows
* PDF取得: Backend API経由
* 印刷対象: PDF
* PDF保存: 一時保存のみ（印刷後に削除）
* 設定ファイル: YAML（`C:\ProgramData\PrintAgent\config.yaml`）

---

# 設計原則

## 1. Windows依存を閉じ込める

Windowsサービス制御・プリンター操作などのOS依存コードは `infrastructure/` に閉じ込める。

## 2. シンプルに保つ

「受信→印刷→結果返却」は1本のフローであり、過剰な抽象化はしない。
レイヤーは config / worker / infrastructure の3つで足りる。

## 3. 業務知識を持たない

Agent は「印刷ジョブを受けて印刷する」ことだけを責務とする。

## 4. 一時ファイルは必ず消す

印刷用に取得したPDFは印刷後に削除する。起動時にも残骸掃除を行う。

## 5. 障害時に自己復旧しやすくする

Redis・Backend の切断時は再接続・再試行する。1件失敗しても次のジョブ処理を継続する。

## 6. 設定値をコードに埋め込まない

agent_id・Backend URL・Redis接続先・temp_dir などはすべて YAML で外出しする。

---

# ディレクトリ構成

```text
print-agent/
├─ main.go
├─ internal/
│  ├─ config/
│  │  ├─ config.go
│  │  └─ loader.go
│  ├─ worker/
│  │  └─ processor.go
│  └─ infrastructure/
│     ├─ backend/
│     │  └─ client.go
│     ├─ redis/
│     │  └─ consumer.go
│     ├─ printer/
│     │  ├─ windows_printer.go
│     │  └─ temp_file.go
│     ├─ localapi/
│     │  └─ server.go
│     ├─ logging/
│     │  └─ logger.go
│     └─ servicehost/
│        ├─ windows_service.go
│        └─ console_runner.go
├─ installer/
│  ├─ inno/
│  │  └─ setup.iss
│  └─ scripts/
│     ├─ install_service.ps1
│     ├─ uninstall_service.ps1
│     └─ post_install.ps1
├─ configs/
│  └─ config.example.yaml
├─ scripts/
│  ├─ build.ps1
│  └─ run-local.ps1
├─ docs/
│  ├─ CLAUDE.md
│  ├─ api-contract.md
│  └─ installer.md
├─ main.go
├─ go.mod
└─ README.md
```

---

# 各ディレクトリの役割

## main.go

エントリポイント。依存を組み立てて起動する。DI はここに直書きする。

## internal/config

YAML設定ファイルの読み込みと必須項目バリデーション。起動時に検証し、不備があれば即終了する。

## internal/worker

印刷処理の中核。Redis からジョブを受信し、PDF取得・印刷・結果返却・temp削除・ACK を1本のフローで行う。ハートビートもここで動かす。

## internal/infrastructure

外部依存の実装。以下の責務を持つ：

* `backend/` — Backend API との HTTP通信
* `redis/` — Redis Streams の Consumer実装
* `printer/` — Windowsプリンター操作・temp管理
* `localapi/` — localhost HTTP APIサーバー
* `logging/` — ログ出力
* `servicehost/` — Windowsサービスとコンソール起動の切り替え

## installer

setup.exe 生成（Inno Setup）とWindowsサービス登録用スクリプト。

---

# 依存の方向

```text
main.go
  ↓ 組み立て・起動
worker.Processor  ←→  infrastructure（backend, redis, printer）
localapi.Server   ←→  infrastructure（printer）
```

`infrastructure` から `worker` は呼ばない。

---

# main.go の役割

依存を直接組み立てて起動する。bootstrap 層は設けない。

```go
package main

import (
    "print-agent/internal/config"
    "print-agent/internal/infrastructure/backend"
    "print-agent/internal/infrastructure/localapi"
    "print-agent/internal/infrastructure/logging"
    "print-agent/internal/infrastructure/printer"
    "print-agent/internal/infrastructure/redis"
    "print-agent/internal/infrastructure/servicehost"
    "print-agent/internal/worker"
)

func main() {
    cfg, err := config.Load()
    if err != nil {
        log.Fatalf("config load failed: %v", err)
    }

    logger   := logging.NewLogger(cfg)
    backend  := backend.NewClient(cfg, logger)
    consumer := redis.NewConsumer(cfg, logger)
    prt      := printer.NewWindowsPrinter(cfg, logger)

    proc   := worker.NewProcessor(cfg, logger, consumer, backend, prt)
    server := localapi.NewServer(cfg, logger, prt)

    servicehost.Run(func(ctx context.Context) error {
        var wg sync.WaitGroup
        wg.Add(2)
        go proc.Start(ctx, &wg)
        go server.Start(ctx, &wg)
        wg.Wait()
        return nil
    })
}
```

---

# worker/processor.go の役割

goroutine は2本のみ。

```text
goroutine 1: ハートビート（ticker で定期送信）
goroutine 2: Redis Consumer ループ
  → ジョブ受信
  → Backend からジョブ詳細取得
  → Backend から PDF 取得
  → temp 保存
  → 印刷
  → Backend へ結果返却
  → temp 削除
  → Redis ACK
```

印刷は1件ずつ同期処理。プリンターは同時印刷を前提としない。

---

# goroutine 構成

全体で4本。

```text
main goroutine        : servicehost.Run（シグナル待ち）
worker.Start          : Redis Consumer ループ
worker（内部）         : ハートビート
localapi.Server.Start : HTTP待受
```

---

# Job処理フロー

```text
1. Redis Streams から job_id を受信
2. Backend API から job詳細取得
3. Backend API から PDF 取得
4. temp保存（storage.temp_dir）
5. 印刷実行
6. Backend へ結果返却（成功 or 失敗）
7. temp削除
8. Redis ACK
```

ステップ 5 失敗時は 6 でエラーを返却し、7・8 は続行する。
ステップ 2〜4 失敗時は Backend にエラー返却し、次のジョブへ進む。

---

# Local API 設計

## エンドポイント

```text
GET  /health       — 疎通確認
GET  /info         — agent_id・バージョン・設定概要
POST /test-print   — テスト印刷
```

---

# Agent自動登録

Agent は自分のIDを持たずに起動し、初回起動時に Backend へ自動登録してUUIDを取得する。

## 起動フロー

```
setup.exe でインストール
  ↓
config.yaml を配置（agent.id は空）
  ↓
Windows サービスとして起動
  ↓
config の agent.id が空 → Backend POST /agents/register → UUID取得 → config.yaml に書き込み
  ↓
通常稼働開始（以降の再起動では agent.id が埋まっているのでスキップ）
```

## 登録リクエスト

```
POST /agents/register
Request:  { mac: "AA:BB:CC:DD:EE:FF", hostname: "PC-01" }
Response: { agent_id: "uuid-xxxx" }
```

同じMACアドレスで再登録が来た場合は既存のUUIDをそのまま返す（べき等）。

---

# 設定ファイル（YAML）

## 配置先

```text
C:\ProgramData\PrintAgent\config.yaml
```

## YAML例

```yaml
agent:
  id: ""  # 初回起動時に自動登録して書き込まれる

backend:
  base_url: "http://backend.local:8000"
  timeout_sec: 30

redis:
  addr: "redis.local:6379"
  password: ""
  db: 0
  stream_name: "print_jobs"
  consumer_group: "print_agent_group"

local_api:
  host: "127.0.0.1"
  port: 18181

heartbeat:
  interval_sec: 10

storage:
  temp_dir: "C:\\ProgramData\\PrintAgent\\tmp"
  log_dir:  "C:\\ProgramData\\PrintAgent\\logs"

print:
  delete_temp_after_print: true
  cleanup_on_startup: true
```

## 必須項目（起動時バリデーション）

* backend.base_url
* redis.addr / stream_name / consumer_group
* storage.temp_dir / log_dir

`agent.id` は空でも起動可（自動登録フローで補完される）。  
上記が不足している場合は起動失敗とする。

## インストール時に手動設定が必要な項目

* backend.base_url — Backend サーバーのURL
* redis.addr — Redis サーバーのアドレス

これらはネットワーク環境依存のため自動化できない。インストーラのUIで入力させるか、社内共通値で固定する。

---

# ログ方針

以下を出力する。

* 起動・設定読込
* Redis 接続・切断・再接続
* ジョブ受信
* PDF取得
* 印刷開始・完了
* temp削除
* ハートビート送信（DEBUG レベル）
* エラー全般

---

# エラー方針

* Agent全体を落とさない
* 1件失敗しても次のジョブ処理を継続
* Redis 切断時は再接続（バックオフあり）
* Backend 切断時は再試行（バックオフあり）
* 印刷失敗はジョブ単位でエラー返却
* YAML不備など起動前に検知できるものは起動失敗

---

# Windowsサービス化

`internal/infrastructure/servicehost/` で切り替えを行う。

* `windows_service.go` — `golang.org/x/sys/windows/svc` を使用
* `console_runner.go` — ローカルデバッグ用（シグナル待ち）

`servicehost.Run(fn)` を呼ぶだけで、サービス／コンソールを自動判別して切り替える。

---

# インストーラ方針

Inno Setup を使用する。

## setup.exe でやること

1. `print-agent.exe` を `C:\Program Files\PrintAgent\` に配置
2. `config.yaml` を `C:\ProgramData\PrintAgent\` に配置
3. `tmp/` `logs/` ディレクトリを作成
4. `install_service.ps1` を実行してWindowsサービス登録・起動

## インストール完了の定義

**setup.exe 完了時点で、print-agent が Windowsサービスとして起動済みであること。**

## install_service.ps1 でやること

```powershell
sc.exe create PrintAgent binPath= "C:\Program Files\PrintAgent\print-agent.exe" start= auto
sc.exe config PrintAgent start= auto
sc.exe failure PrintAgent reset= 0 actions= restart/5000/restart/5000/restart/5000
sc.exe start PrintAgent
```

---

# インストール後の確認項目

* サービスが存在し、状態が Running
* `config.yaml` が存在する
* `tmp/` `logs/` が存在する
* `GET /health` が応答する

---

# 開発優先順位

1. YAML設定読込・バリデーション
2. Local API（/health, /printers）
3. Backend 疎通確認
4. Redis Consumer
5. Job取得・PDF取得
6. 印刷実行
7. ハートビート
8. Windowsサービス化
9. インストーラ

---

# NG事項

* 業務ロジックを入れない
* PDFを端末に恒久保存しない
* RedisにPDF本体を流さない
* 設定値をコードに直書きしない
* ユーザーに手動でサービス登録させない
* 過剰な抽象化をしない（usecase層・domain層・bootstrap層は不要）

---

# 最終目標

* setup.exe で導入できる
* `config.yaml` で設定変更できる
* Windowsサービスとして自動起動する
* Redis Streams から job を受信する
* Backend API から PDF を取得して印刷する
* 数百台の端末で安定運用できる
