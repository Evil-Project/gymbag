# 検証レベル (`0`–`3`) と Proof-of-Work

1 つの Gymbag プロセスで複数の Web サイト（ラウンドロビン負荷分散と RFC 6455 WebSocket アップグレード対応）と SSH・Minecraft 保護を同時に運用できます。サイトおよび各サービスごとに `verify <0-3>` でポリシーを指定するか、`protect off` で PoW チャレンジを無効化します。

<VerifyLevelMatrix />

## チャレンジ・ブロック時の表示状態

Gymbag は PoW チャレンジ用 HTML、ブラウザ側 SHA-256 ソルバー、3 種類のマスコット画像をバイナリ内に直接埋め込んでいます。

<div class="gb-mascot-strip">
  <div class="gb-mascot-tile">
    <img src="/mascot-pensive.png" alt="検証中のマスコット" />
    <strong>PoW 計算中</strong>
    <span class="gb-badge challenge">チャレンジ発行</span>
  </div>
  <div class="gb-mascot-tile">
    <img src="/mascot-happy.png" alt="通過時のマスコット" />
    <strong>クリアランス付与</strong>
    <span class="gb-badge pass">waf_clearance 発行</span>
  </div>
  <div class="gb-mascot-tile">
    <img src="/mascot-reject.png" alt="ブロック時のマスコット" />
    <strong>ルール / 脅威ブロック</strong>
    <span class="gb-badge block">403 Forbidden</span>
  </div>
</div>

## マルチサイト・マルチサービス構成例

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

ssh {
    upstream 127.0.0.1:41822
    verify 3
}
```

レベル `1` および `3` における不審（suspicious）判定には、脅威フィード、Shodan/StopForumSpam でのライブ照会結果、ブラウザヘッダーの欠落、自動化クライアント、攻撃プローブ、スクリプト化された SSH/Minecraft クライアントに加えて、**VPN・プロキシ・Tor 出口ノードの IP**（`vpn_detection on`）も含まれます。

## 認証済み有効期限（`verify_timeout` / `clearance_ttl`）

HTTP/HTTPS サイトでは、SHA-256 PoW チャレンジを通過するとホスト名とクライアント IP にバインドされた `HttpOnly`・`SameSite=Lax` の HMAC 署名付き Cookie（デフォルト名 `waf_clearance`）が発行されます。SSH と Minecraft では、ブラウザ上の検証リンクを通過した時点でそのサービス向けの IP クリアランスレコードが保存されます。

デフォルトの有効期間は **24 時間**（`86400` 秒）です。グローバルまたはサイト・サービス単位で、整数秒または期間文字列（`30m`、`12h`、`1h30m`、`7d`、`2w`）を指定して変更できます。

::: code-group
```caddyfile [Gymbagfile]
{
    verify_timeout 12h
}

staging.example.com {
    reverse_proxy http://localhost:3002
    verify 0
    verify_timeout 30m
}

docs.example.com {
    reverse_proxy http://localhost:4000
    verify 2
    clearance_ttl 7d
}
```

```bash [CLI フラグ]
gymbag init -d app.example.com -u :3000 --verify-timeout 6h
gymbag run --config Gymbagfile --verify-timeout 30m
gymbag reverse-proxy --from app.internal --to :3000 --verify 0 --verify-timeout 15m
```

```bash [環境変数]
GYMBAG_VERIFY_TIMEOUT=12h gymbag
GYMBAG_CLEARANCE_TTL=3600 gymbag
```

```bash [ライブ Admin API]
curl -X PATCH http://127.0.0.1:2019/api/v1/sites/staging.example.com \
  -H "Content-Type: application/json" \
  -d '{"clearance_ttl": 1800}'
```
:::

- `Gymbagfile` で使えるディレクティブ別名: `verify_timeout`、`verified_timeout`、`clearance_timeout`、`clearance_ttl`。
- 環境変数別名: `GYMBAG_CLEARANCE_TTL`、`GYMBAG_VERIFY_TIMEOUT`、`GYMBAG_VERIFIED_TIMEOUT`。
- `POST /.waf/v1/verifications` は `{"ok": true, "expires_in": <seconds>}` を返し、設定された TTL に合わせて Cookie の `Max-Age` を設定します。
- `GET /.waf/v1/session` は認証済みブラウザに対して `clearance_ttl`、有効期限 `expires_at`（RFC 3339 UTC）、残り秒数 `expires_in` を返します。

## 難易度・チャレンジ有効期間・レート制限

- `difficulty`（`1`–`5`、デフォルト `4`）: SHA-256 ダイジェスト先頭に要求する 16 進ゼロの桁数（難易度 `4` なら `0000...`）。
- `challenge_ttl`（デフォルト `300`s / `5m`）: 使い捨てチャレンジナンスの有効期限。
- `clearance_ttl` / `verify_timeout`（デフォルト `86400`s / `24h`）: クリアランス Cookie または TCP IP クリアランスの有効期間。
- `rate_limit` / `rate_window`: IP ごとの固定ウィンドウ型レート制限。Web サイトのデフォルトは `60`s あたり `240` リクエスト、`ssh` と `minecraft` ブロックのデフォルトは `60`s あたり `60` 接続です。
