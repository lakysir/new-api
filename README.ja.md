<div align="center">

![AI Token P2P Platform](./web/default/public/logo.png)

# AI 遊休算力 Token-P2P 取引プラットフォーム

🚀 **ブラウザ拡張機能がノードに · AI Token 収益化 · New-API エンタープライズ中継ゲートウェイ**

<p align="center">
  <a href="./README.md">简体中文</a> |
  <a href="./README.en.md">English</a> |
  <strong>日本語</strong> |
  <a href="./README.ko.md">한국어</a>
</p>

<p align="center">
  <a href="#%EF%B8%8F-ビルドとデプロイ">ビルド・デプロイ</a> •
  <a href="#-p2p-ai-token-遊休算力取引">P2P算力取引</a> •
  <a href="#-new-api-中継ゲートウェイ">New-API中継</a>
</p>

</div>

---

## 📌 プロジェクト概要

本プロジェクトは [New API](https://github.com/QuantumNous/new-api)（次世代大規模モデルゲートウェイ）をベースに、完全な **P2P AI 算力取引ネットワーク**を構築したものです。

ChatGPT Plus・Claude Pro・Gemini Advanced などの AI サービスを購読しているユーザーは、毎月の**遊休 Token 枠**をマーケットに接続し、クライアントのタスクを受注して収益を得ることができます。一方、クライアントは統一 SDK を通じて低コストで複数の AI 能力を呼び出せ、各プラットフォームに個別に登録する必要がありません。

### 主な特徴

| 特徴 | 説明 |
|------|------|
| 🧩 P2P 算力マーケット | ブラウザ拡張機能がノードに — 遊休 AI 枠を直接収益化 |
| 🔒 エンドツーエンド暗号化 (E2EE) | タスクのパラメータと結果は全プロセスで暗号化され、認証情報はローカルを離れない |
| 📜 マーケットスクリプト管理 | スクリプト審査・ハッシュ署名・バージョン不変 — 悪意あるコードを防止 |
| 🌐 New-API 中継ゲートウェイ | OpenAI / Claude / Gemini 等の主要フォーマットに対応、マルチチャネル・インテリジェントルーティング |
| 💰 透明な決済台帳 | 複式簿記で、すべての収益を注文・領収書・レートまで追跡可能 |
| ⚡ スクリプト更新不要 | 新しい AI サイトを追加するにはスクリプトをアップロードするだけ — 拡張機能の更新は不要 |

---

## 🛠️ ビルドとデプロイ

### 動作要件

| コンポーネント | バージョン要件 |
|------|---------|
| Go | ≥ 1.21 |
| Node.js | ≥ 18（または Bun ≥ 1.0、高速ビルド推奨） |
| Docker | ≥ 20.10 |
| Docker Compose | ≥ 2.0 |
| データベース | SQLite（デフォルト）/ MySQL ≥ 5.7.8 / PostgreSQL ≥ 9.6 |

---

### 方法1：Docker Compose（推奨）

```bash
git clone https://github.com/lakysir/new-api.git
cd new-api
nano docker-compose.yml
docker-compose up -d
docker-compose ps
```

🎉 起動後、`http://localhost:3000` にアクセス。デフォルト管理者: `root` / `123456`（**初回ログイン後は直ちにパスワードを変更してください**）。

---

### 方法2：ソースからビルド

#### 1. バックエンド（Go）

```bash
cd new-api
go mod download
go build -ldflags "-s -w" -o new-api-server .
./new-api-server
```

#### 2. フロントエンド

```bash
cd new-api/web
bun install && bun run build
# または npm install && npm run build
# 成果物: web/dist/
```

---

### 方法3：Docker 単一コマンド

```bash
# SQLite 使用
docker run --name ai-token-p2p -d --restart always \
  -p 3000:3000 -e TZ=Asia/Tokyo -v ./data:/data \
  lakysir/new-api:latest

# MySQL 使用
docker run --name ai-token-p2p -d --restart always \
  -p 3000:3000 \
  -e SQL_DSN="root:password@tcp(db:3306)/aitoken" \
  -e TZ=Asia/Tokyo -v ./data:/data \
  lakysir/new-api:latest
```

---

### 主要な環境変数

| 変数名 | 説明 | デフォルト |
|--------|------|--------|
| `SESSION_SECRET` | セッション暗号化キー（マルチノードでは**必須**） | 自動生成 |
| `CRYPTO_SECRET` | データ暗号化キー（Redis 使用時は**必須**） | - |
| `SQL_DSN` | データベース接続文字列 | SQLite |
| `REDIS_CONN_STRING` | Redis 接続文字列 | - |
| `STREAMING_TIMEOUT` | ストリーミングタイムアウト（秒） | `300` |
| `P2P_TURN_SECRET` | TURN サーバー共有シークレット | - |
| `P2P_RELAY_SECRET` | E2EE Relay 署名キー | - |

> [!WARNING]
> マルチノード/本番環境では `SESSION_SECRET` と `CRYPTO_SECRET` の設定が必須です。

---

## 🔗 P2P AI Token 遊休算力取引

### P2P 算力取引とは？

ChatGPT Plus・Claude Pro・Gemini Advanced などを利用しているが毎月の Token 枠を使い切れていない方向けに、その**遊休 AI 枠**を収益化するプラットフォームです。

- **Provider として**: ブラウザ拡張機能をインストールし、バックグラウンドで自動受注してローカルの AI アカウントでタスクを実行、報酬を得ます。
- **Client として**: 統一 API / SDK 経由でタスクごとに AI 実行能力を購入。各プラットフォームへの個別登録不要。
- **スクリプト作者として**: 内蔵アナライザーで任意の AI サイト向けスクリプトを作成し、採用量に応じた報酬を得ます。

```
Client がタスクを投稿 + 料金を支払う
  → プラットフォームがオンラインノードをマッチング
    → Provider 拡張機能がローカルの AI サービスでタスクを実行
      → 結果を E2EE 暗号化して Client に返送
        → プラットフォームが自動決済、Provider が収益を受け取る
```

---

### 3つの参加ロール

#### 🖥️ Provider — AI 枠提供者

**あなたが持っているもの**: ChatGPT Plus / Claude Pro / Gemini Advanced / Midjourney など。

**あなたがすること**: 拡張機能をインストール → スクリプトを選択 → 価格と上限を設定 → ブラウザを開いたまま待つ。

**あなたが得るもの**: タスク完了ごとの Token 収益（いつでも引き出し可能）。

| セキュリティ保証 | 内容 |
|---------|------|
| 認証情報はローカルに留まる | パスワードと Cookie はローカルブラウザにのみ存在 |
| いつでも停止可能 | 既決済収益に影響なく受注を停止可能 |
| 透明な認可 | 各スクリプトは手動テスト・確認後に出品 |
| 収益の追跡可能性 | すべての収益を注文・領収書・課金明細まで追跡 |

#### 👤 Client — タスク依頼者

```typescript
import { AiTokenClient } from '@ai-token-p2p/sdk'

const client = new AiTokenClient({ apiKey: 'your-api-key' })

const order = await client.createOrder({
  scriptId: 'chatgpt-text-v1',
  config: { prompt: 'プロジェクト計画書を書いてください', model: 'gpt-4o' },
  maxCost: 0.05,
  timeoutSeconds: 120,
})

const result = await client.waitForResult(order.id)
console.log(result.output)
```

#### ✍️ スクリプト作者

1. 対象 AI サイトでパケットキャプチャを開始
2. 対象操作を実行 → AI が自動でスクリプトを生成
3. ローカルテスト → 審査に提出
4. 承認後、ハッシュロックされた不変バージョンとして公開
5. 採用量に応じた報酬を受け取る

---

### セキュリティアーキテクチャ

| セキュリティ機能 | 実装内容 |
|---------|---------|
| 🔒 E2EE | タスク内容の暗号化、コントロールプレーンは復号不可 |
| 🚫 認証情報の隔離 | Cookie・パスワード・API Key は Provider ローカルのみ |
| 📜 スクリプトの不変性 | ハッシュロックで上書き不可、サプライチェーン攻撃を防止 |
| 🎯 最小権限 | 宣言した Origin にのみアクセス可能 |
| ✅ 双方向領収書 | 双方署名領収書で独立検証可能 |
| 🛡️ サンドボックス審査 | 自動スキャン + サンドボックス + 人工審査 |

---

### Provider クイックスタート

**ステップ1**: [Releases](https://github.com/lakysir/new-api/releases) から `.zip` をダウンロードして解凍。Chrome で `chrome://extensions/` → **デベロッパー モード** → **パッケージ化されていない拡張機能を読み込む** → 解凍フォルダを選択。

**ステップ2**: 拡張機能アイコンをクリック → プラットフォームアカウントでログイン → デバイスバインド。

**ステップ3**: 対象 AI サイトを開いてログイン → **スクリプトマーケット** → スクリプトを選択 → ローカルテスト → 価格・上限を設定 → **出品**。

**ステップ4**: ブラウザを開いたままにする。拡張機能がバックグラウンドで自動受注・実行します。

---

## 🌐 New-API 中継ゲートウェイ

### 基本的な使い方

`base_url` を置き換えるだけで既存コードから接続できます：

```python
from openai import OpenAI

client = OpenAI(
    api_key="your-platform-token",
    base_url="http://your-server:3000/v1"
)

response = client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "こんにちは"}]
)
print(response.choices[0].message.content)
```

---

### コア機能

#### 🔄 マルチフォーマット API 変換

| ソース | ターゲット | ステータス |
|--------|---------|------|
| OpenAI Chat ↔ Claude Messages | 双方向 | ✅ |
| OpenAI Chat → Google Gemini | 一方向 | ✅ |
| Google Gemini → OpenAI Chat | テキスト | ✅ |
| OpenAI Realtime API（Azure 含む）| リアルタイム音声 | ✅ |
| OpenAI Responses API | 新形式 | ✅ |

#### ⚖️ インテリジェントルーティング

- 加重ランダム選択で負荷分散
- 失敗時バックアップチャネルへ自動切り替え
- ユーザーレベルのレート制限

#### 💰 詳細な課金管理

- Token 単位課金・キャッシュヒット割引
- OpenAI / Azure / DeepSeek / Claude / Qwen 対応
- EPay / Stripe チャージ対応
- 可視化ダッシュボード

#### 🔑 アクセス制御

- Token グループ化（モデル制限・有効期限・IP 制限）
- Discord / Telegram / LinuxDO / OIDC ログイン
- 完全なリクエストログ・監査

---

### 対応モデル

| タイプ | 対応 |
|---------|------|
| OpenAI（Chat / Responses / Realtime） | ✅ |
| Claude Messages | ✅ |
| Google Gemini | ✅ |
| Azure OpenAI | ✅ |
| Midjourney（via Proxy） | ✅ |
| Suno | ✅ |
| Rerank（Cohere / Jina） | ✅ |
| 画像 / 音声 / 動画 / 埋め込み | ✅ |

#### Reasoning Effort

```
gpt-5-high / gpt-5-low / o3-mini-medium
gemini-2.5-pro-high
claude-3-7-sonnet-20250219-thinking
```

---

### 管理画面クイックスタート

1. `http://your-server:3000` にアクセス、管理者でログイン
2. **チャンネル管理** → API Key を追加
3. **トークン管理** → チームやアプリ向けトークンを作成
4. **ダッシュボード** → 使用量・コスト・成功率をリアルタイム確認
5. **設定 → 運用設定** → リトライ・レート制限・課金ポリシーを設定

---

## 🔗 関連プロジェクト

| プロジェクト | 説明 |
|------|------|
| [New API](https://github.com/QuantumNous/new-api) | 上流ゲートウェイ |
| [One API](https://github.com/songquanpeng/one-api) | オリジナルプロジェクト |
| [Midjourney-Proxy](https://github.com/novicezk/midjourney-proxy) | Midjourney サポート |

---

## 📜 ライセンス

本プロジェクトは [AGPLv3](./LICENSE) でライセンスされており、[New API](https://github.com/QuantumNous/new-api)（MIT）をベースに二次開発されています。

---

<div align="center">

### 💖 AI Token P2P プラットフォームをご利用いただきありがとうございます

このプロジェクトが役に立った場合は、⭐️ Star をいただけると嬉しいです！

</div>
