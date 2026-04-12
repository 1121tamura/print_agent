# Backend 実装考慮事項

print-agent と連携する Backend を実装する際に考慮が必要な項目をまとめる。

---

## Agent 管理

### DB テーブル設計

```
agents
├── id            UUID, PK        Backend が発行
├── mac_address   VARCHAR, UNIQUE  Agent から送信
├── hostname      VARCHAR          Agent から送信
├── name          VARCHAR          管理者が付ける表示名（後から編集可）
├── created_at    TIMESTAMP
└── last_seen_at  TIMESTAMP        ハートビートで更新
```

### 登録API

```
POST /agents/register
```

- Request: `{ mac: string, hostname: string }`
- Response: `{ agent_id: string }`
- 同一 `mac_address` で再登録が来た場合は既存の UUID をそのまま返す（べき等）
- 認証: インストール時に共有シークレット（APIキーなど）を使用することを検討する

### NIC交換・MAC変更時の挙動

- 旧MACのレコードは残る（孤児レコードになる）
- 新MACで新規登録され、新しいUUIDが発行される
- 管理画面で旧レコードを無効化・削除できると運用しやすい

---

## 印刷ジョブ管理

### Redis Streams との連携

- Stream 名: `print_jobs:{agent_id}`（端末ごとに専用の Stream）
  - 例: `print_jobs:uuid-aaa`（端末Aのみが受信）
  - `print_jobs` の部分は Agent 側の config.yaml `redis.stream_name` に合わせる
- Consumer Group: `print_agent_group`
- メッセージに含めるのは `job_id` のみ（PDF本体は流さない）
- Backend はジョブ発生時に、発生元端末の agent_id を使って該当 Stream に XADD する

### ジョブ詳細 API

```
GET /jobs/{job_id}
```

- Response: ジョブ詳細（PDF取得URL など）
- プリンター指定は不要。Agent は OS の「通常使うプリンター」に出力する。

### PDF 取得 API

```
GET /jobs/{job_id}/pdf
```

- Response: PDF バイナリ
- Agent は印刷後に一時ファイルを削除する（Backend 側での保存期限も検討）

### 結果受け取り API

```
POST /jobs/{job_id}/result
```

- Request: `{ agent_id: string, status: "success" | "error", error_message?: string }`

---

## 状態通知API

```
POST /agents/{agent_id}/status
```

- Request: `{ status: string, job_id?: string, error_message?: string }`
- Backend は `last_seen_at` を更新する

### 送信タイミング（Agent側）

| status | タイミング | job_id |
|---|---|---|
| `online` | Agent 起動時 | なし |
| `printing` | 印刷開始直前 | あり |
| `success` | 印刷完了時 | あり |
| `error` | 印刷失敗時 / PDF取得失敗時 | あり |

- 定期送信（ハートビート）は行わない
- `last_seen_at` は「最後に通知が来た時刻」として扱う
- 一定時間 `online` / `printing` 以外の通知がなければオフライン扱いにする仕組みを検討

---

## 認証・セキュリティ

- Agent ↔ Backend 間の通信は社内ネットワーク前提だが、APIキーによる認証は最低限入れることを推奨
- `POST /agents/register` は誰でも叩けると野良登録されるため、共有シークレットで保護する
- HTTPSの使用を検討（証明書管理コストとのトレードオフ）

---

## 運用・管理画面

- Agent 一覧（id / hostname / last_seen_at / オンライン状態）
- Agent の name 編集
- 孤児レコード（旧MAC）の無効化・削除
- ジョブ履歴・印刷結果の確認

※ プリンター管理は Agent 側では行わない。各端末の「通常使うプリンター」設定に委ねる。
