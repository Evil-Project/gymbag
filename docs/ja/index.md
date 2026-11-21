---
layout: home

hero:
  name: Gymbag
  text: HTTPS リバースプロキシ & Proof-of-Work WAF
  tagline: HTTPS を終端し、複数の CA で ACME を回し、SHA-256 の proof-of-work でスクレイパーを止める 1 本の Go バイナリ。
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
    title: 速いプロキシ経路
    details: Aho-Corasick のマッチング、シャード化したレート制限、再利用バッファで WAF を邪魔にしない構成。TLS 経由で約 72.5k RPS、エッジでの探査ブロックは 140k+ RPS。
  - icon: 🛡️
    title: 最初から WAF が効く
    details: /.env、/wp-admin、スキャナー、SQLi/XSS 探査のルールを同梱。4 段階の検証レベルで SHA-256 チャレンジを出す相手を決め、許可は 30m・12h・7d から選べます。
  - icon: 🌐
    title: リモートルールと脅威フィード
    details: HTTP 越しにファイアウォールルールを取り込み ETag でキャッシュ。IPsum、Spamhaus DROP、Feodo、公開されている AI クローラーの CIDR をタイマーで更新します。
  - icon: 🔒
    title: 証明書の手間を減らす
    details: Let's Encrypt、ZeroSSL、Google、SSL.com、LiteSSL、Buypass に対する ACME HTTP-01。*.internal なら自前の CA を作り /etc/hosts も書き込みます。
  - icon: 🐳
    title: いつもの方法でデプロイ
    details: GHCR のマルチアーキテクチャイメージ（amd64・arm64）、共有状態用の Redis 入り compose、Arch 向けパッケージ、systemd・launchd・OpenRC・Supervisor のユニット。
  - icon: 🔌
    title: 動かしたまま設定を変える
    details: --watch で Gymbagfile を編集するか、Admin API から検証レベル・タイムアウト・ローカルルール・リモート URL を変更。既存の接続は切れません。
---

<QuickStartBuilder />

## インタラクティブ検証レベル

下のボタンを切り替えると、普通のブラウザ、AI クローラー、自動化ツール、脅威 IP、攻撃プローブに対して Gymbag がどう動くか確認できます：

<VerifyLevelMatrix />
