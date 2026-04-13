# print-agent

Windows端末に常駐し、Redis Streams 経由で印刷ジョブを受信して、ローカルプリンターへ印刷するエージェント。

## 全体構成

```
print-agent          — このリポジトリ（Go / Windows サービス）
print-agent-backend  — Backend API（Python FastAPI + MySQL）
print-agent-frontend — 管理画面（React）
```

---

## 動作概要

```
フロントエンド
  ↓ 印刷実行
Backend
  ↓ XADD print_jobs:{agent_id}
Redis Streams
  ↓
print-agent（Windows端末）
  ↓ job詳細・PDF取得
Backend
  ↓ ローカルプリンターで印刷
  ↓ 結果返却
Backend
```

- 端末ごとに専用の Stream（`print_jobs:{agent_id}`）を使うため、他の端末のジョブが混入しない
- 初回起動時に Backend へ自動登録し、`agent_id`（UUID）を取得して `config.yaml` に保存

---

## 開発環境

Dev Container（VSCode）を使用。

**前提:**
- Docker Desktop
- VSCode + Dev Containers 拡張

**起動手順:**

1. このリポジトリを clone
2. VSCode で開く
3. 「Reopen in Container」を選択

コンテナ内には Go 1.24 がセットアップ済み。

---

## Dev Container での起動

印刷以外のフロー全体（ジョブ受信・PDF取得・状態通知・ACK）を確認できる。
印刷は stub（何もしない）になるため、実際の印刷確認は Windows 実機が必要。

```bash
# 起動
bash scripts/dev.sh start

# 停止
bash scripts/dev.sh stop

# 再起動
bash scripts/dev.sh restart

# 状態確認（/health・/info を表示）
bash scripts/dev.sh status
```

起動後は `http://127.0.0.1:18181` で Local API が使用可能。

**初回起動時:** `config.dev.yaml` の `agent.id` が空の場合、Backend に自動登録して UUID を書き込む。2回目以降はスキップ。

---

## ビルド

Windows 向け実行ファイル（`print-agent.exe`）を Dev Container 内でクロスコンパイルする。

```bash
bash scripts/build.sh
```

`dist/print-agent.exe` が生成される。バージョンは git tag から自動取得。

---

## 設定ファイル

`C:\ProgramData\PrintAgent\config.yaml` に配置する。

```yaml
agent:
  id: ""  # 初回起動時に自動登録されて書き込まれる

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


storage:
  temp_dir: "C:\\ProgramData\\PrintAgent\\tmp"
  log_dir:  "C:\\ProgramData\\PrintAgent\\logs"

print:
  delete_temp_after_print: true
  cleanup_on_startup: true
```

`configs/config.example.yaml` をコピーして使用する。

**必須項目（不足時は起動失敗）:**
- `backend.base_url`
- `redis.addr` / `stream_name` / `consumer_group`
- `storage.temp_dir` / `log_dir`

---

## Local API

起動後、`http://127.0.0.1:18181` で以下のエンドポイントが使用可能。

| メソッド | パス | 説明 |
|---|---|---|
| GET | /health | 疎通確認 |
| GET | /info | agent_id・バージョン・設定概要 |
| POST | /test-print | テスト印刷（未実装） |

---

## ディレクトリ構成

```
print-agent/
├── main.go
├── internal/
│   ├── config/          — YAML設定読込・バリデーション
│   ├── worker/          — 印刷処理の中核（Job受信→印刷→結果返却）
│   └── infrastructure/
│       ├── backend/     — Backend API との HTTP通信
│       ├── redis/       — Redis Streams Consumer
│       ├── printer/     — プリンター操作
│       ├── localapi/    — localhost HTTP API
│       └── servicehost/ — Windows サービス化（未実装）
├── installer/           — Inno Setup（未実装）
├── configs/
│   └── config.example.yaml
├── scripts/
│   └── build.sh
└── docs/
    ├── CLAUDE.md
    ├── backend-considerations.md
    └── api-contract.md
```

---

## 実装状況

### 完了

| 機能 | ファイル |
|---|---|
| 設定読込・バリデーション | `internal/config/` |
| Agent 自動登録（初回起動時） | `internal/infrastructure/backend/client.go` |
| Local API `/health` `/info` | `internal/infrastructure/localapi/server.go` |
| Redis Consumer（XREADGROUP） | `internal/infrastructure/redis/consumer.go` |
| Job取得・PDF取得 | `internal/infrastructure/backend/client.go` |
| 状態通知（online/printing/success/error） | `internal/infrastructure/backend/client.go` |
| X-API-Key 認証（全リクエスト共通） | `internal/infrastructure/backend/client.go` |
| 印刷ワーカー（Job受信→印刷→結果返却） | `internal/worker/processor.go` |

### 残タスク（Windows環境が必要）

#### 1. 印刷実行 `internal/infrastructure/printer/windows_printer.go`

`GetDefaultPrinter()` と `Print()` の Windows API 実装。

```go
// GetDefaultPrinter: Windows API で通常使うプリンターを取得
// Print: SumatraPDF などで PDF を印刷
```

参考: `golang.org/x/sys/windows` または `syscall` パッケージで `GetDefaultPrinter` を呼ぶ。

#### 2. Windows サービス化 `internal/infrastructure/servicehost/`

以下2ファイルを作成する。

- `windows_service.go` — `golang.org/x/sys/windows/svc` を使用
- `console_runner.go` — ローカルデバッグ用（シグナル待ち）

`servicehost.Run(fn)` を呼ぶだけでサービス／コンソールを自動判別して切り替える。

`main.go` の現在のシグナル待ち処理を `servicehost.Run()` に置き換える。

#### 3. インストーラ `installer/`

- `installer/inno/setup.iss` — Inno Setup スクリプト
- `installer/scripts/install_service.ps1` — Windows サービス登録
- `installer/scripts/uninstall_service.ps1` — サービス削除
- `installer/scripts/post_install.ps1` — インストール後確認

setup.exe が完了した時点で print-agent が Windows サービスとして起動済みであること。

---

## ドキュメント

- [docs/CLAUDE.md](docs/CLAUDE.md) — 設計方針・実装ガイド
- [docs/sequence.md](docs/sequence.md) — 処理フロー シーケンス図
- [docs/backend-considerations.md](docs/backend-considerations.md) — Backend 実装時の考慮事項
