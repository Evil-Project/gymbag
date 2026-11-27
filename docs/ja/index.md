---
layout: home

hero:
  name: Gymbag
  text: HTTP/HTTPS・SSH・Minecraft 対応 PoW ゲートウェイ
  tagline: HTTPS を終端し、ブラウザの Proof-of-Work で SSH と Minecraft を守り、スクレイパーや脅威 IP を止める 1 本の Go バイナリ。
  image:
    src: /mascot-happy.png
    alt: Gymbag マスコット
  actions:
    - theme: brand
      text: 2コマンド クイックスタート
      link: /ja/guide/getting-started
    - theme: alt
      text: SSH & Minecraft ガイド
      link: /ja/guide/ssh-minecraft
    - theme: alt
      text: Caddy / Nginx とのベンチマーク
      link: /ja/guide/benchmarks

features:
  - icon: ⚡
    title: 速いプロキシ経路
    details: "Aho-Corasick ルール照合、シャード化したレート制限、再利用バッファで WAF の負荷を最小限に抑えます。TLS 経由で約 72.5k RPS、エッジでの探査ブロックは 140k+ RPS。"
  - icon: 🛡️
    title: HTTP・SSH・Minecraft を 1 プロセスで
    details: "Web サイト、WebSocket、OpenSSH、Minecraft Java サーバーをまとめて保護。SSH は切断画面に HTTPS リンクを表示し、Minecraft 1.20.5+ は未認証プレイヤーをメモリ上の空ワールドへ入れてチャットにクリック可能なリンクを送り、完了後に Transfer で自動リダイレクトします。"
  - icon: 🌐
    title: 脅威インテル・VPN・AI リスト
    details: "IPsum、Spamhaus DROP、Feodo、X4BNet VPN/Tor リスト、公式 AI クローラー CIDR を自動更新。Shodan InternetDB、StopForumSpam、ipquery.io、ipapi.is のフェイルオープン照会にも対応。"
  - icon: 🔒
    title: ACME・IP 証明書・ローカル PKI
    details: "Let's Encrypt、ZeroSSL、Google、SSL.com、LiteSSL、Buypass への HTTP-01 に加え、検証ポータル用の RFC 8738 ACME IP 証明書に対応。*.internal は自前の CA で署名します。"
  - icon: 🐳
    title: 配布パッケージとコンテナを用意
    details: "静的バイナリ、Debian/Ubuntu 用 .deb、Arch 用 .pkg.tar.zst、NixOS flake、マルチアーキテクチャ GHCR イメージ、Redis 連携 Compose、systemd/launchd/OpenRC/Supervisor ユニットを提供。"
  - icon: 🔌
    title: 稼働中のまま設定を変更
    details: "gymbag.json と Gymbagfile の両方を生成でき、nginx/Apache/Caddyfile v2 もそのまま読込可能。ルールフォルダの監視や Admin REST API でのライブ更新時も接続を切りません。"
---

<QuickStartBuilder />

## インタラクティブ検証レベル

下のレベルを切り替えて、Gymbag が通常のブラウザ、AI クローラー、VPN/Tor IP、自動化ツール、脅威フィード、攻撃プローブをどう処理するか確認できます。

<VerifyLevelMatrix />
