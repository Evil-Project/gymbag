---
layout: home

hero:
  name: Gymbag
  text: HTTPS リバースプロキシ & Proof-of-Work WAF
  tagline: 外部依存ゼロの純粋な Go 単一バイナリゲートウェイ — マルチ CA ACME、2層ローカル PKI、リモート＆組み込み WAF ルール、自動更新脅威インテリジェンス、Anubis スタイルの SHA-256 検証を統合。
  image:
    src: /mascot-happy.png
    alt: Gymbag マスコット
  actions:
    - theme: brand
      text: 2コマンド クイックスタート
      link: /ja/guide/getting-started
    - theme: alt
      text: Docker ガイド
      link: /ja/guide/docker
    - theme: alt
      text: Caddy / Nginx とのベンチマーク
      link: /ja/guide/benchmarks

features:
  - icon: ⚡
    title: 高スループット純 Go エンジン
    details: Aho-Corasick DFA パターンマッチング、シャーディングされたレート制限、プール化されたプロキシバッファを備えたゼロアロケーション WAF パイプラインにより、フル WAF 有効時で約 72.5k RPS（Caddy 比 +32〜69% 高速）と 140k+ RPS のエッジブロックを実現。
  - icon: 🛡️
    title: すぐ使える WAF・PoW (0–3)・カスタムタイムアウト
    details: 組み込みルールが /.env、/wp-admin、スキャナー、SQLi/XSS プローブを遮断し、SHA-256 PoW が AI クローラーを検証。認証済み有効期限（30m、12h、7d など）も柔軟に設定可能。
  - icon: 🌐
    title: リモートルール & 自動脅威インテリジェンス
    details: HTTP/HTTPS 経由でリモートのファイアウォールルール（Gymbagfile 形式または JSON）を ETag キャッシュ付きで同期し、IPsum、Spamhaus DROP、Feodo、公式 AI クローラー CIDR を自動更新。
  - icon: 🔒
    title: マルチ CA ACME + ローカル PKI
    details: Let's Encrypt、ZeroSSL（自動 EAB）、Google、SSL.com、LiteSSL、Buypass に対応した RFC 8555 HTTP-01 自動証明書発行、および *.internal ローカル CA と /etc/hosts 自動管理を提供。
  - icon: 🐳
    title: Docker & ネイティブシステムサービス
    details: マルチアーキテクチャ GHCR コンテナイメージ（linux/amd64・arm64）と Docker Compose + Redis に加え、systemd、launchd、OpenRC、Supervisor、Arch、NixOS をネイティブサポート。
  - icon: 🔌
    title: ライブ REST 管理 API & ホットリロード
    details: --watch による Gymbagfile の自動ホットリロードや、HTTP API 経由での検証レベル・タイムアウト・ローカル/リモートルールの無停止動的変更に対応。
---

<QuickStartBuilder />

## インタラクティブ検証レベル

下のボタンを切り替えて、通常のブラウザ、AI クローラー、自動化ツール、脅威 IP、攻撃プローブに対する Gymbag の挙動を確認できます：

<VerifyLevelMatrix />
