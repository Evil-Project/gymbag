# REST API & Redis

## 公開 WAF エンドポイント

保護対象の Web サイト上で自動的に提供されます。

| メソッド | パス | 説明 |
| :--- | :--- | :--- |
| `GET` | `/.waf/v1/session` | `site`、`protected`、`verify_level`（`0`–`3`）、`verify_mode`、`clearance_ttl`、`cleared`、および（クリア済みの場合）`expires_at` / `expires_in` を返却 |
| `DELETE` | `/.waf/v1/session` | 訪問者のクリアランス Cookie を削除（`204 No Content`） |
| `POST` | `/.waf/v1/challenges` | 使い捨て SHA-256 PoW チャレンジを発行（`201 Created`） |
| `POST` | `/.waf/v1/verifications` | PoW 解を検証し、署名済みクリアランス Cookie を設定して `{"ok": true, "expires_in": <ttl>}` を返却（`/.waf/verify` 別名対応） |
| `GET` | `/.waf/assets/{name}` | 埋め込みチャレンジ PNG アセットを配信（`/.waf/v1/assets/{name}` 別名、`ETag` キャッシュ付き） |

## 検証ポータルエンドポイント（SSH & Minecraft）

`verify_port`（デフォルト `8443`）上で配信されます。`verify_port` が `https_port` と一致し HTTP サイトが有効な場合は `https_port`（`443`）上で配信されます。

| メソッド | パス | 説明 |
| :--- | :--- | :--- |
| `GET` | `/v/{token}` | SSH または Minecraft のゲートチケット向けブラウザ PoW ページを配信（`/verify/{token}` 別名対応） |
| `POST` | `/.waf/gate/verify` | `{token, challenge_id, nonce, verified}` を検証し、IP クリアランスを保存（`{"ok": true, "service": ..., "client_ip": ..., "expires_in": <ttl>}`） |
| `GET` | `/.waf/assets/{name}` | 埋め込みマスコット PNG を配信（`/.waf/v1/assets/{name}` 別名対応） |
| `GET` | `/.well-known/acme-challenge/{token}` | ポータルドメインまたは RFC 8738 IP 証明書向けの ACME HTTP-01 チャレンジ応答 |

## ローカル Admin REST API

`admin 127.0.0.1:2019`、`--admin 127.0.0.1:2019`、または `GYMBAG_ADMIN=127.0.0.1:2019` で有効になります。

::: warning ループバックまたはプライベートインターフェースにのみバインドしてください
Admin API には認証トークンがなく、Gymbag を再起動せずに稼働中のルーティングやファイアウォール状態を変更できます。必ず `127.0.0.1:2019` かプライベートな管理インターフェースにバインドしてください。
:::

| メソッド | パス | 説明 |
| :--- | :--- | :--- |
| `GET` | `/api/v1/health` | ステータス、`sites_count`、`ssh_enabled`、`minecraft_enabled`、`tls_source`、`threat_entries`、`ai_entries`、`vpn_entries`、読み込み済み `remote_rules` 件数 |
| `GET` | `/api/v1/tls` | アクティブ TLS ソース、証明書パス、SSL プロバイダ、HSTS 状態、対応 CA カタログ |
| `POST` | `/api/v1/tls/renew` | 即時証明書更新とアトミックリロードを実行 |
| `GET` | `/api/v1/intel` | 脅威・AI クローラー・VPN/Tor フィード件数（`vpn_detection_enabled`, `vpn_entries`, `vpn_cache_size`, `reputation_cache_size`）、更新タイムスタンプ、フィード別ステータス（`threat`, `ai`, `vpn`） |
| `POST` | `/api/v1/intel/refresh` | すべての脅威・VPN・AI フィードを即時更新 |
| `GET` | `/api/v1/rules/remote` | リモートルール URL、読み込み済みルール件数、キャッシュ状態、直近エラー |
| `POST` | `/api/v1/rules/remote/refresh` | リモートルール URL を取得し WAF エンジンをアトミックにリロード |
| `GET` | `/api/v1/config` | 現在のランタイム設定を出力（`secret`、`key_password`、`eab_hmac_key` はマスク） |
| `PUT` | `/api/v1/config` | 稼働中のランタイム設定を検証して置換 |
| `GET` | `/api/v1/sites` | 設定済み Web サイトと WAF ポリシーを一覧取得 |
| `POST` | `/api/v1/sites` | 実行時に Web サイトを追加（`201 Created`） |
| `GET` | `/api/v1/sites/{domain}` | 単一 Web サイトの設定を取得 |
| `PUT` | `/api/v1/sites/{domain}` | 単一 Web サイトの設定を置換 |
| `PATCH` | `/api/v1/sites/{domain}` | `verify_level`、`protect`、`difficulty`、`clearance_ttl`、`upstreams`、`rate_limit`、`rules`、`remote_rules`、`routes`、`headers`、`rule_sources` をライブパッチ |
| `DELETE` | `/api/v1/sites/{domain}` | Web サイトを削除（`204 No Content`；SSH も Minecraft も有効でない状態で最後の 1 サイトを消そうとすると `409 Conflict`） |
| `GET` | `/api/v1/sites/{domain}/rules` | Web サイト上のローカルカスタムルールを一覧取得 |
| `POST` | `/api/v1/sites/{domain}/rules` | カスタムルールを末尾に追加（`201 Created`） |
| `PUT` | `/api/v1/sites/{domain}/rules` | Web サイト上の全カスタムルールを置換 |
| `DELETE` | `/api/v1/sites/{domain}/rules/{index_or_name}` | 数値インデックスまたは `name` でカスタムルールを削除（`204 No Content`） |

### ライブ緩和コマンド例

```bash
# app.example.com を検証レベル 0、クリアランス有効期間 30 分に引き締め
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"verify_level": 0, "difficulty": 5, "clearance_ttl": 1800}'

# パスルート、レスポンスヘッダー、ローカルルールソースをライブで更新
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"routes": [{"path": "/static/*", "strip_prefix": "/static", "root": "./dist", "browse": true}], "headers": {"X-Served-By": "gymbag"}, "rule_sources": [{"source": "./rules.d", "dynamic": true}]}'

# Gymbagfile を編集せずに攻撃元 CIDR をブロック
curl -X POST http://127.0.0.1:2019/api/v1/sites/app.example.com/rules \
  -H "Content-Type: application/json" \
  -d '{"name": "drop-abusive-subnet", "action": "block", "ips": ["198.51.100.0/24"]}'

# リモートルールと脅威・VPN・AI フィードを更新
curl -X POST http://127.0.0.1:2019/api/v1/rules/remote/refresh
curl -X POST http://127.0.0.1:2019/api/v1/intel/refresh
```

## Redis 共有ステート

Gymbag は `redis://` および `rediss://` URL に対応した純 Go 製 RESP2 Redis クライアントを内蔵しています。`redis`（または `GYMBAG_REDIS_URL`）を指定すると、以下を複数インスタンスや再起動間で共有できます。

- HTTP および検証ゲート用の使い捨て PoW チャレンジ（`gymbag:challenge:<id>`）
- 固定ウィンドウの IP ごとのレート制限（`gymbag:rate:<key>`）
- SSH および Minecraft 用のブラウザ検証チケット（`gymbag:ticket:<token>`）と IP クリアランス（`gymbag:clearance:<service>:<ip>`）

起動時に Redis へ到達できない場合は警告ログを出し、64 シャードのインメモリストアへフォールバックします。そのまま動かせる Compose スタックについては [Docker & Docker Compose ガイド](/ja/guide/docker) を参照してください。
