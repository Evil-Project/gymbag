# 検証レベル (`0`–`3`)・Proof-of-Work・タイムアウト

1つの Gymbag プロセスで数十のドメインを同時にホスト・保護し、マルチアップストリームのラウンドロビン負荷分散や全二重 RFC 6455 WebSocket プロキシを実行できます。各サイトは `verify <0-3>` または `protect off` で個別にポリシーを設定できます。

<VerifyLevelMatrix />

## チャレンジ・ブロック画面の状態

Gymbag は Proof-of-Work ブラウザチャレンジ画面、SHA-256 ソルバー JS、3種類のマスコット画像をバイナリ内に直接埋め込んでいます：

<div class="gb-mascot-strip">
  <div class="gb-mascot-tile">
    <img src="/mascot-pensive.png" alt="PoW 検証中マスコット" />
    <strong>PoW 計算中</strong>
    <span class="gb-badge challenge">チャレンジ発行</span>
  </div>
  <div class="gb-mascot-tile">
    <img src="/mascot-happy.png" alt="検証通過マスコット" />
    <strong>クリアランス付与</strong>
    <span class="gb-badge pass">waf_clearance 発行</span>
  </div>
  <div class="gb-mascot-tile">
    <img src="/mascot-reject.png" alt="ブロックマスコット" />
    <strong>ルール / 脅威ブロック</strong>
    <span class="gb-badge block">403 Forbidden</span>
  </div>
</div>

## マルチサイト構成例

```caddyfile
app.example.com, www.example.com {
    reverse_proxy http://localhost:3000 http://localhost:3001
    verify 3
    difficulty 4
    verify_timeout 12h
}

docs.example.com {
    reverse_proxy http://localhost:4000
    verify 2
    verify_timeout 7d
}

api.example.com {
    reverse_proxy http://localhost:8080
    verify 1
}

staging.example.com {
    reverse_proxy http://localhost:3002
    verify 0
    verify_timeout 30m
}

static.example.com {
    reverse_proxy http://localhost:9000
    protect off
}
```

## 認証済みステータスのカスタムタイムアウト (`verify_timeout` / `clearance_ttl`)

訪問者が SHA-256 Proof-of-Work を完了すると、Gymbag はドメインとクライアント IP にバインドされた `HttpOnly`・`SameSite=Lax` 属性の HMAC 署名付きクリアランス Cookie（既定名 `waf_clearance`）を発行します。

既定の有効期間は **24時間**（`86400` 秒）です。グローバルまたはサイト単位で、整数秒**または**期間文字列（`30m`、`12h`、`1h30m`、`7d`、`2w`）で柔軟にカスタマイズできます：

::: code-group
```caddyfile [Gymbagfile]
{
    # すべてのサイトのデフォルト認証有効期間
    verify_timeout 12h
}

staging.example.com {
    reverse_proxy http://localhost:3002
    verify 0
    # ステージング環境向けに短い 30分タイムアウトを設定
    verify_timeout 30m
}

docs.example.com {
    reverse_proxy http://localhost:4000
    verify 2
    # ドキュメント向けに 7日間の長い有効期間を設定（clearance_ttl 別名も使用可）
    clearance_ttl 7d
}
```

```bash [CLI フラグ]
# 6時間の認証有効期間を指定して設定を生成
gymbag init -d app.example.com -u :3000 --verify-timeout 6h

# 実行時に上書き
gymbag run --config Gymbagfile --verify-timeout 30m
gymbag reverse-proxy --from app.internal --to :3000 --verify 0 --verify-timeout 15m
```

```bash [環境変数]
# 整数秒または期間表記（30m, 12h, 7d など）を指定可能
GYMBAG_VERIFY_TIMEOUT=12h gymbag
GYMBAG_CLEARANCE_TTL=3600 gymbag
```

```bash [ライブ管理 API]
# 再起動なしで clearance_ttl（秒単位）を動的変更
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/staging.example.com \
  -H "Content-Type: application/json" \
  -d '{"clearance_ttl": 1800}'
```
:::

- `Gymbagfile` で使えるディレクティブ別名：`verify_timeout`、`verified_timeout`、`clearance_timeout`、`clearance_ttl`。
- 環境変数別名：`GYMBAG_CLEARANCE_TTL`、`GYMBAG_VERIFY_TIMEOUT`、`GYMBAG_VERIFIED_TIMEOUT`。
- `POST /.waf/v1/verifications` は `{"ok": true, "expires_in": <秒数>}` を返し、Cookie の `Max-Age` を設定します。
- `GET /.waf/v1/session` は認証済みユーザーに対して `clearance_ttl`、有効期限 `expires_at`（RFC 3339 UTC）、残り秒数 `expires_in` を返します。

## 難易度・チャレンジ有効期間・レート制限

- **`difficulty`**（`1`–`5`、既定 `4`）：SHA-256 ハッシュに必要な先頭16進ゼロの個数（難易度 `4` は `0000...`）。
- **`challenge_ttl`**（既定 `300` 秒 / `5m`）：使い捨て PoW チャレンジの有効期間。秒数または期間表記に対応。
- **`clearance_ttl` / `verify_timeout`**（既定 `86400` 秒 / `24h`）：認証済み Cookie の有効期間（例：`30m`、`12h`、`7d`）。
- **`rate_limit` / `rate_window`**（既定 `60` 秒あたり `240` リクエスト）：64シャードインメモリストア（または Redis）によるサイト別・IP 別の固定ウィンドウレート制限。
