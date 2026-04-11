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

heartbeat:
  interval_sec: 10

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

## ドキュメント

- [docs/CLAUDE.md](docs/CLAUDE.md) — 設計方針・実装ガイド
- [docs/backend-considerations.md](docs/backend-considerations.md) — Backend 実装時の考慮事項
