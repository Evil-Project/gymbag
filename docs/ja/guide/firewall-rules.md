# ファイアウォールルール・リモートルール・脅威インテル

## リクエスト評価パイプライン

保護対象サイトへのすべてのリクエストに対し、Gymbag は以下の厳密な順序でセキュリティ層を評価します：

1. **リクエスト形状チェック** — HTTP メソッド検証、最大 URI 長、NULL バイトおよび `..` / `%2e%2e` パストラバーサルの拒否。
2. **ACME HTTP-01 バイパス** — `/.well-known/acme-challenge/*` は常に通過し、証明書発行を妨げません。
3. **ローカルカスタムルール** — グローバルルール、続いてサイト固有ルールを上から評価（**最初の一致が優先**）。
4. **リモートルール (`remote_rules`)** — 指定した HTTP/HTTPS URL から取得・検証されたルールを定義順に評価。
5. **組み込みベースラインルール** — 機密ファイル、CMS 攻撃プローブ、スキャナー、SQLi/XSS パターン、自動化ツール検証（`default_rules off` 時は無効）。
6. **サイト別・IP 別レートリミッター** — `rate_limit` と `rate_window` を適用。
7. **IP 脅威インテリジェンス** — `ip_security_action`（既定 `verify` または `block`）を実行。
8. **Proof-of-Work 検証** — 設定された `verify` レベル（`0`–`3`）を実行。

ローカルカスタムルールはリモートルール・既定ルール・レート制限・脅威フィードよりも**先に**評価されるため、先頭に `allow` ルールを置くことで社内 CIDR やヘルスチェック、プレビュー Cookie を確実にホワイトリスト化できます。

---

## 1行ショートハンドルール

構文：`<allow | verify | block> <field> <pattern...>`（別名 `bypass`/`pass`、`challenge`/`pow`、`deny`/`drop`/`reject` も使用可）。

| フィールド | マッチング仕様 | 例 |
| :--- | :--- | :--- |
| `ip` | IPv4、IPv6、または CIDR 範囲 | `allow ip 127.0.0.1 10.0.0.0/8 2001:db8::/32` |
| `user_agent` | 大文字小文字を区別しない部分一致、`*` ワイルドカード、`~正規表現` | `block user_agent BadBot ~^(?i)python-urllib/` |
| `path` | 完全一致/プレフィックス、`*` ワイルドカード、`~正規表現` | `verify path /api/export* ~^/v[0-9]+/search` |
| `query` | クエリ文字列一致、`*` ワイルドカード、`~正規表現` | `block query "union select" ~(?i)<script` |
| `method` | HTTP メソッド（大文字小文字不問） | `verify method POST DELETE` |
| `header` | `<Header-Name> [pattern]`（`""` = 欠落/空、`*` = 存在） | `verify header Accept-Language ""` |
| `cookie` | `<cookie_name> [pattern]`（`""` = 欠落/空、`*` = 存在） | `allow cookie vip_session *` |

---

## 複数条件 `rule` ブロック

単一フィールドでは表現できない条件には `rule "name" { ... }` を使用します。**同一フィールド内**の複数値は **OR**、同じ `rule` ブロック内の**異なるフィールド**は **AND** で結合されます：

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

---

## リモートルール (`remote_rules`)

Git リポジトリの生 URL、S3/R2、CDN、社内ポリシーサーバーなどの HTTP/HTTPS URL から共有ファイアウォールルールを取得できます。Gymbag は起動時および `remote_rules_refresh` 間隔（既定 `5m` / `300` 秒）ごとに自動更新します。

### リモートルールの設定

グローバルブロック（全サイト継承）および個別サイトブロックの両方で指定できます：

::: code-group
```caddyfile [Gymbagfile]
{
    # 全サイトに適用される共通リモートルール
    remote_rules https://rules.example.com/org-baseline.rules
    remote_rules_refresh 5m
}

app.example.com {
    reverse_proxy http://localhost:3000
    verify 3

    # ローカルルールは常にリモートルールより優先されます
    allow ip 10.0.0.0/8

    # サイト専用リモートルール（テキスト形式または JSON）
    remote_rules https://rules.example.com/app-overrides.json
}
```

```bash [CLI & 環境変数]
# CLI フラグ（複数回またはカンマ区切り）
gymbag run --config Gymbagfile \
  --remote-rules https://rules.example.com/baseline.rules \
  --remote-rules-refresh 10m

# 環境変数
GYMBAG_REMOTE_RULES="https://rules.example.com/baseline.rules,https://rules.example.com/extra.json" \
GYMBAG_REMOTE_RULES_REFRESH=5m \
gymbag
```

```bash [Admin REST API]
# 同期ステータス・ルール数・エラー確認
curl http://127.0.0.1:2019/api/v1/rules/remote

# 即時取得と WAF エンジンのアトミックリロードを実行
curl -X POST http://127.0.0.1:2019/api/v1/rules/remote/refresh

# 単一サイトのリモートルール URL を動的に更新
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/app.example.com \
  -H "Content-Type: application/json" \
  -d '{"remote_rules": ["https://rules.example.com/emergency.rules"]}'
```
:::

### サポートされるリモートルール形式

**Gymbagfile ルール構文**と **JSON** の両方を自動判別します：

::: code-group
```caddyfile [Gymbagfile テキスト構文]
# 共有リモートファイアウォールルール
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

```json [JSON オブジェクト（または配列）]
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

### キャッシュとフェイルセーフ保証

- **条件付きリクエスト**：`If-None-Match` (`ETag`) と `If-Modified-Since` を送信し、`304 Not Modified` の場合は再コンパイルをスキップします。
- **ディスクキャッシュ**：検証済みルールは `<cert_dir>/rules-cache/` に保存され、起動直後のネットワーク取得完了前から即座に読み込まれます。
- **フェイルセーフ検証**：有効化前に構文・項目を厳密検証します（上限 4 MiB、15秒タイムアウト）。取得失敗や不正構文時は最後に正常だったルールセットを保持し、`GET /api/v1/rules/remote` にエラーを記録します。
- **無停止スワップ**：変更時は WAF エンジン（O(1) パスマップおよび Aho-Corasick DFA を含む）を再コンパイルし、既存接続を切断せずルーティングスナップショットをアトミックに入れ替えます。

---

## 組み込みルール (`default_rules on|off`)

`default_rules` は既定で `on` です：
- サイトにローカル・リモートルールが**ない**場合、組み込みブロックルール（`/.env`、`/.git/*`、`/wp-admin/*`、`sqlmap`/`nuclei`、SQLi/XSS クエリ）と自動化検証ルール（`curl`、`wget`、`python-requests`、`headlesschrome`、`playwright`、空 `User-Agent`）の両方が動作します。
- カスタムルールを**追加した**場合、カスタムルールが先に評価され、その後に組み込み**ブロック**ルールが適用されるため、`/.env` 等が誤って露出するのを防ぎます。
- 完全に手動制御したい場合は `default_rules off` を設定してください。

---

## 自動更新される脅威・AI クローラーフィード

すべてのフィードは起動時および `intel_refresh`（既定 `21600` 秒 / `6h`）ごとに自動更新され、`ETag` / `If-Modified-Since` 付きで `<cert_dir>/intel-cache` にキャッシュされます：

- **脅威フィード (`ip_security on`)**：IPsum level 3+、Spamhaus DROP IPv4/IPv6、abuse.ch Feodo Tracker。
- **ライブ IP レピュテーション (`ip_reputation on`)**：Shodan InternetDB と StopForumSpam をキー不要で照会（TTL キャッシュ・フェイルオープン対応）。
- **公式 AI クローラー IP 範囲 (`ai_ip_list on`)**：OpenAI、Anthropic、Perplexity、Google、Apple の公式 CIDR リスト。

```caddyfile
{
    ip_security on
    ip_security_action block   # "verify" (PoW チャレンジ) または "block" (403 拒否)
    ip_reputation on
    ai_ip_list on
    intel_refresh 6h
}
```
