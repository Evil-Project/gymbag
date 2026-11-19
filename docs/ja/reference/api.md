# REST API & Redis

## 公開 WAF エンドポイント

保護対象サイト上で自動的に提供されます：

| メソッド | パス | 説明 |
| :--- | :--- | :--- |
| `GET` | `/.waf/v1/session` | クリアランス状態 (`cleared`)、`protected`、`verify_level` (`0`–`3`)、`verify_mode`、`clearance_ttl`、認証済み時は `expires_at` / `expires_in` を返却 |
| `DELETE` | `/.waf/v1/session` | クリアランス Cookie を削除（`204 No Content`） |
| `POST` | `/.waf/v1/challenges` | 使い捨て SHA-256 PoW チャレンジを発行（`201 Created`） |
| `POST` | `/.waf/v1/verifications` | PoW 解を検証して署名 Cookie を発行し `{"ok": true, "expires_in": <ttl>}` を返却（`/.waf/verify` 別名対応） |
| `GET` | `/.waf/assets/{name}` | 埋め込みマスコット PNG を配信（`/.waf/v1/assets/{name}` 別名、`ETag` キャッシュ対応） |

---

## ローカル Admin REST API

`admin 127.0.0.1:2019`、`--admin 127.0.0.1:2019`、または `GYMBAG_ADMIN=127.0.0.1:2019` で有効化します。

::: warning 必ずループバックまたはプライベート管理ネットワークにバインドしてください
Admin API は認証トークンなしで実行中のルーティングやファイアウォールルールを変更できます。必ず `127.0.0.1:2019` やプライベート WireGuard / Unix インターフェースに限定してください。
:::

| メソッド | パス | 説明 |
| :--- | :--- | :--- |
| `GET` | `/api/v1/health` | ステータス、`sites_count`、`tls_source`、`threat_entries`、`ai_entries`、`remote_rules` 数 |
| `GET` | `/api/v1/tls` | アクティブ TLS ソース・証明書パス・SSL プロバイダー・HSTS 状態・対応 CA 一覧 |
| `POST` | `/api/v1/tls/renew` | 証明書の即時更新とアトミックリロードを実行 |
| `GET` | `/api/v1/intel` | 脅威/AI フィード別件数、更新日時、レピュテーションキャッシュ統計 |
| `POST` | `/api/v1/intel/refresh` | 脅威＆AI フィードの即時更新を実行 |
| `GET` | `/api/v1/rules/remote` | リモートルール URL ごとの読み込み件数、キャッシュ状態、エラーを確認 |
| `POST` | `/api/v1/rules/remote/refresh` | すべてのリモートルール URL を即時取得し WAF エンジンをアトミック更新 |
| `GET` | `/api/v1/config` | 現在のランタイム設定を取得（`secret`、`key_password`、`eab_hmac_key` はマスク） |
| `PUT` | `/api/v1/config` | ランタイム設定全体を検証しアトミックに置換 |
| `GET` | `/api/v1/sites` | 設定済み全サイトと WAF ポリシー一覧 |
| `POST` | `/api/v1/sites` | 実行時に新しいサイトを追加（`201 Created`） |
| `GET` | `/api/v1/sites/{domain}` | 単一サイトの設定を取得 |
| `PUT` | `/api/v1/sites/{domain}` | 単一サイトの設定を置換 |
| `PATCH` | `/api/v1/sites/{domain}` | `verify_level`、`protect`、`difficulty`、`clearance_ttl`、`upstreams`、`rate_limit`、`rules`、`remote_rules` を部分更新 |
| `DELETE` | `/api/v1/sites/{domain}` | サイトを削除（`204 No Content`；最後の1サイトは削除不可） |
| `GET` | `/api/v1/sites/{domain}/rules` | サイトのローカルカスタムルール一覧 |
| `POST` | `/api/v1/sites/{domain}/rules` | カスタムルールを末尾に追加（`201 Created`） |
| `PUT` | `/api/v1/sites/{domain}/rules` | サイトの全カスタムルールを置換 |
| `DELETE` | `/api/v1/sites/{domain}/rules/{index_or_name}` | インデックスまたは `name` でルールを削除（`204 No Content`） |

### 緊急緩和コマンド例

```bash
# app.example.com をレベル 0（全員検証）＆有効期限 30分に引き締め
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"verify_level": 0, "difficulty": 5, "clearance_ttl": 1800}'

# Gymbagfile を編集せず即座に攻撃元 CIDR をブロック
curl -X POST http://127.0.0.1:2019/api/v1/sites/app.example.com/rules \
  -H "Content-Type: application/json" \
  -d '{"name": "drop-abusive-subnet", "action": "block", "ips": ["198.51.100.0/24"]}'

# リモートルールと脅威インテルを即時更新
curl -X POST http://127.0.0.1:2019/api/v1/rules/remote/refresh
curl -X POST http://127.0.0.1:2019/api/v1/intel/refresh
```

---

## Redis 共有ステート

Gymbag は外部依存なしの純 Go 製 RESP2 Redis クライアントを内蔵しています。`redis redis://[:password@]host:6379/0`（または `GYMBAG_REDIS_URL`）を指定すると、使い捨て PoW チャレンジと IP 単位のレート制限をコンテナ再起動や複数レプリカ間で共有できます。

Docker / Compose での詳しい構成は [Docker & Docker Compose ガイド](/ja/guide/docker) を参照してください。
