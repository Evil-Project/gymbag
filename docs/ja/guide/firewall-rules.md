# ファイアウォールルール・リモートルール・脅威インテル

## リクエスト評価パイプライン

保護対象の HTTP サイトに届いたリクエストは、次の順で評価されます。

1. **リクエスト形式の検証**: HTTP メソッドの確認、URI 長上限、NULL バイトおよび `..` / `%2e%2e` パストラバーサルの拒否。
2. **ACME HTTP-01 バイパス**: `/.well-known/acme-challenge/*` は常に通過し、証明書の発行や更新を妨げません。
3. **ローカルカスタムルール**: グローバルルール、続いてサイト固有ルールを上から評価（最初の一致で決定）。
4. **リモートルール (`remote_rules`)**: HTTP/HTTPS URL から取得・検証されたルールを定義順に評価。
5. **組み込みベースラインルール**: 機密パス、CMS 探査、スキャナー、SQLi/XSS クエリ、自動化クライアントへの PoW ルール（`default_rules off` 時は無効）。
6. **サイト別・IP 別レートリミッター**: `rate_limit` と `rate_window`（デフォルト `240` / `60s`）。
7. **IP 脅威インテリジェンス**: `ip_security_action verify`（デフォルト）または `block`。
8. **Proof-of-Work 検証**: 設定された `verify` レベル（`0`–`3`）。VPN/プロキシ/Tor 検出（`1` & `3`）および AI クローラー照合（`2` & `3`）を含みます。

ローカルカスタムルールはリモートルール、ベースラインルール、レート制限、脅威フィードより先に評価されるため、サイトブロックの先頭に `allow` ルールを書けばオフィスの CIDR、ヘルスチェック、プレビュー用 Cookie をそのまま許可できます。`{ ... }` 内のグローバルルールは `ssh` と `minecraft` サービスにも適用されます（`ssh { ... }` や `minecraft { ... }` 内のサービス別ルールも併用できます）。

## 1 行短縮ルール

構文: `<allow | verify | block> <field> <pattern...>`（別名 `bypass`/`pass`、`challenge`/`pow`、`deny`/`drop`/`reject` も使用可）

| フィールド | マッチング仕様 | 例 |
| :--- | :--- | :--- |
| `ip` | IPv4、IPv6、CIDR レンジ | `allow ip 127.0.0.1 10.0.0.0/8 2001:db8::/32` |
| `user_agent` | 大文字小文字を区別しない部分一致、`*` グロブ、`~regex` | `block user_agent BadBot ~^(?i)python-urllib/` |
| `path` | 完全/前方一致、`*` グロブ、`~regex` | `verify path /api/export* ~^/v[0-9]+/search` |
| `query` | 生/デコード済みクエリ一致、`*` グロブ、`~regex` | `block query "union select" ~(?i)<script` |
| `method` | HTTP メソッド（大文字小文字不問） | `verify method POST DELETE` |
| `header` | `<Header-Name> [pattern]`（`""` = 欠落または空、`*` = 存在） | `verify header Accept-Language ""` |
| `cookie` | `<cookie_name> [pattern]`（`""` = 欠落または空、`*` = 存在） | `allow cookie vip_session *` |

## 複数条件の `rule` ブロック

複数条件を組み合わせたい場合は `rule "name" { ... }` を使います。同一フィールドに並べた複数値は OR、同じブロック内の異なるフィールド同士は AND で評価されます。

```caddyfile
app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    rule "challenge-auth-mutations" {
        action verify
        method POST
        path /login* /api/auth/* /reset-password*
    }

    rule "block-headless-api-posts" {
        action block
        method POST
        path /api/*
        header Origin ""
    }
}
```

## リモートルール (`remote_rules`)

Git リポジトリの生ファイル、オブジェクトストレージ、CDN、社内ポリシーサーバーなど 1 つ以上の HTTP/HTTPS URL から共有ルールセットを取り込めます。起動時と `remote_rules_refresh`（デフォルト `5m` / `300s`）ごとに自動取得します。

::: code-group
```caddyfile [Gymbagfile]
{
    remote_rules https://rules.example.com/org-baseline.rules
    remote_rules_refresh 5m
}

app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    allow ip 10.0.0.0/8
    remote_rules https://rules.example.com/app-overrides.json
}
```

```bash [CLI & 環境変数]
gymbag run --config Gymbagfile \
  --remote-rules https://rules.example.com/baseline.rules \
  --remote-rules-refresh 10m

GYMBAG_REMOTE_RULES="https://rules.example.com/baseline.rules,https://rules.example.com/extra.json" \
GYMBAG_REMOTE_RULES_REFRESH=5m \
gymbag
```

```bash [Admin REST API]
curl http://127.0.0.1:2019/api/v1/rules/remote
curl -X POST http://127.0.0.1:2019/api/v1/rules/remote/refresh

curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"remote_rules": ["https://rules.example.com/emergency.rules"]}'
```
:::

### サポートされるリモートルール形式

**Gymbagfile ルール構文** と **JSON**（`[ ... ]` または `{"rules": [ ... ]}`）の両方をそのまま読み込めます。

::: code-group
```caddyfile [Gymbagfile テキスト構文]
block ip 198.51.100.0/24 203.0.113.42
block user_agent BadScraper ~^(?i)EvilBot/
verify path /api/export*

rule "challenge-xml-posts" {
    action verify
    method POST
    path /xmlrpc* /api/import*
    header Origin ""
}
```

```json [JSON オブジェクト]
{
  "rules": [
    {
      "name": "block-abusive-cidr",
      "action": "block",
      "ips": ["198.51.100.0/24", "203.0.113.42"]
    },
    {
      "name": "verify-exports",
      "action": "verify",
      "methods": ["GET", "POST"],
      "paths": ["/api/export*"]
    }
  ]
}
```
:::

### キャッシュと検証

- `If-None-Match`（`ETag`）と `If-Modified-Since` を送信し、`304 Not Modified` 応答時はルールセットの再コンパイルをスキップします。
- 検証済みペイロードを `<cert_dir>/rules-cache/` にキャッシュし、起動直後の初回フェッチ完了前でもすぐに読み込めるようにします。
- 切り替える前に必ずペイロードを検証します（上限 4 MiB、タイムアウト 15 秒）。取得先が構文エラーや HTTP エラーを返した場合は直前の正常なルールセットを維持し、エラー内容を `GET /api/v1/rules/remote` で確認できます。
- ルールの更新はアクティブな接続を切断せずアトミックに差し替えます。

## ローカルルールファイル・フォルダ・ホットリロード

ルールを独立した `.rules` / `.json` ファイルやディレクトリに分割して配置できます。ソースに `dynamic` を指定するとディスク上の変更を監視して自動で再読み込みします。

::: code-group
```caddyfile [Gymbagfile]
{
    rules_watch_interval 2s
}

app.example.com {
    reverse_proxy http://localhost:3000
    rules_file ./rules/custom.rules dynamic
    rules_dir ./rules.d dynamic
    ruleset static-corp {
        source ./rules/corp.json
        dynamic off
    }
}
```

```json [gymbag.json]
{
  "sites": [
    {
      "domain": "app.example.com",
      "upstreams": ["http://localhost:3000"],
      "waf": {
        "rule_sources": [
          {"source": "./rules.d", "dynamic": true, "name": "shared"}
        ]
      }
    }
  ]
}
```

```bash [CLI]
gymbag init -d app.example.com -u :3000 \
  --rules-file ./rules/custom.rules --rules-dir ./rules.d \
  --rules-dynamic --rules-watch-interval 2s

gymbag ruleset init ./rules/custom.rules
gymbag ruleset add --source ./rules --dynamic --name local-dir
gymbag ruleset update local-dir --static
gymbag ruleset validate ./rules
gymbag ruleset rule add --file ./rules/custom.rules --action block --name bad-bot --ua "*BadBot*"
gymbag ruleset remove local-dir
```
:::

ポーリング間隔は `rules_watch_interval`（デフォルト `2s`）です。`rule_sources` は `PATCH /api/v1/sites/{domain}` 経由でライブ変更することもできます。`ruleset` コマンドの詳細は [CLI & 環境変数](/ja/reference/cli-env) を参照してください。

## 組み込みルール (`default_rules on|off`)

`default_rules` はデフォルトで `on` です。
- サイトにローカル・リモートのカスタムルールが **1 つもない**場合、組み込みブロックルール（`/.env`、`/.git/*`、`/wp-admin/*`、`sqlmap`/`nuclei`、SQLi/XSS クエリ）と自動化 PoW ルール（`curl`、`wget`、`python-requests`、`headlesschrome`、`playwright`、空 `User-Agent`）の両方が動作します。
- サイトにカスタムルールを追加すると、ユーザー定義ルールが先に評価され、組み込みの**ブロック**ルールはその後に残ります。`verify path` を 1 つ足しただけで `/.env` が無防備になる心配はありません。
- 組み込みルールを完全に無効にしたい場合は、サイト単位またはグローバルで `default_rules off` を設定してください。

## 脅威フィード・VPN/Tor 検出・AI クローラーリスト

各フィードは起動時と `intel_refresh`（デフォルト `21600`s / `6h`）ごとに更新され、`<cert_dir>/intel-cache` にキャッシュされます。

- **脅威フィード (`ip_security on`)**: IPsum level 3+、Spamhaus DROP IPv4/IPv6、abuse.ch Feodo Tracker。
- **ライブ IP レピュテーション (`ip_reputation on`)**: Shodan InternetDB と StopForumSpam へのキー不要なライブ照会（1 時間 TTL キャッシュ、フェイルオープンタイムアウト）。
- **VPN / プロキシ / Tor 検出 (`vpn_detection on`)**: 自動更新される X4BNet VPN IPv4/IPv6 リストと Tor 一括出口ノードリストに加え、`api.ipquery.io` と `api.ipapi.is` へのキー不要なライブ照会（タイムアウト 1.2 秒、singleflight 重複排除、1 時間キャッシュ、フェイルオープン）を実行します。検出された IP は HTTP・SSH・Minecraft の検証レベル `1` と `3` で **suspicious** 扱いになります。
- **公式 AI クローラー CIDR (`ai_ip_list on`)**: OpenAI（`gptbot`、`searchbot`、`chatgpt-user`）、Anthropic（`claude.com/crawling/bots.json`）、Perplexity、Google、Apple（`applebot`）。

```caddyfile
{
    ip_security on
    ip_security_action block   # "verify" (PoW チャレンジ) または "block" (403 Forbidden)
    ip_reputation on
    vpn_detection on
    ai_ip_list on
    intel_refresh 6h
    # vpn_feed https://rules.example.com/extra-vpn-cidrs.txt
}
```
