<div align="center">

![AI Token P2P Platform](./web/default/public/logo.png)

# AI Idle Compute Token-P2P Trading Platform

🚀 **Browser Extension as Node · AI Token Monetization · New-API Enterprise-Grade Relay Gateway**

<p align="center">
  <a href="./README.md">简体中文</a> | <strong>English</strong> | <a href="./README.ja.md">日本語</a> | <a href="./README.ko.md">한국어</a>
</p>

<p align="center">
  <a href="#️-build--deploy">Build & Deploy</a> •
  <a href="#-p2p-ai-token-idle-compute-trading">P2P Compute Trading</a> •
  <a href="#-new-api-relay-gateway">New-API Relay</a>
</p>

</div>

---

## 📌 Project Overview

This project builds a complete **P2P AI compute trading network** on top of [New API](https://github.com/QuantumNous/new-api) (a next-generation large model gateway).

Every user who holds a subscription to AI services such as ChatGPT Plus, Claude Pro, or Gemini Advanced can connect their monthly **idle Token quota** to the marketplace, accept tasks from clients, and earn revenue. Clients, in turn, can call multiple AI capabilities at low cost through a unified SDK without subscribing to each platform individually.

### Core Highlights

| Feature | Description |
|------|------|
| 🧩 P2P Compute Market | Browser extension as node — monetize idle AI quotas directly |
| 🔒 End-to-End Encryption (E2EE) | Task parameters and results are encrypted throughout; credentials never leave the local machine |
| 📜 Marketplace Script Governance | Script review, hash signing, immutable versioning — malicious code is prevented |
| 🌐 New-API Relay Gateway | Compatible with OpenAI / Claude / Gemini and other mainstream formats, with multi-channel intelligent routing |
| 💰 Transparent Settlement Ledger | Double-entry bookkeeping; every payment is traceable to its order, receipt, and rate |
| ⚡ No Script Updates Required | Adding a new AI site only requires the author to upload a script — the extension needs no new release |

---

## 🛠️ Build & Deploy

### Requirements

| Component | Version Requirement |
|------|---------|
| Go | ≥ 1.21 |
| Node.js | ≥ 18 (or use Bun ≥ 1.0 for faster builds) |
| Docker | ≥ 20.10 |
| Docker Compose | ≥ 2.0 |
| Database | SQLite (default) / MySQL ≥ 5.7.8 / PostgreSQL ≥ 9.6 |

---

### Option 1: Docker Compose (Recommended)

```bash
# Clone the project
git clone https://github.com/lakysir/new-api.git
cd new-api

# Edit configuration as needed
nano docker-compose.yml   # Configure database, Redis, secrets, etc.

# Start with a single command
docker-compose up -d

# Check running status
docker-compose ps
docker-compose logs -f new-api
```

🎉 After startup, visit `http://localhost:3000` to access the admin dashboard. Default credentials: `root` / `123456` — **change the password immediately after your first login**.

---

### Option 2: Build from Source

#### 1. Backend (Go)

```bash
cd new-api

# Download dependencies
go mod download

# Build
go build -ldflags "-s -w" -o new-api-server .

# Run (SQLite by default, data stored in ./data/)
./new-api-server
```

#### 2. Frontend

```bash
cd new-api/web

# Recommended: use Bun (faster)
bun install && bun run build

# Or use npm
npm install && npm run build
# Build output goes to web/dist/
```

---

### Option 3: Docker Single Command

```bash
# Using SQLite (simplest)
docker run --name ai-token-p2p -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  lakysir/new-api:latest

# Using MySQL
docker run --name ai-token-p2p -d --restart always \
  -p 3000:3000 \
  -e SQL_DSN="root:password@tcp(db:3306)/aitoken" \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  lakysir/new-api:latest
```

---

### Key Environment Variables

| Variable | Description | Default |
|--------|------|--------|
| `SESSION_SECRET` | Session encryption key (**required** for multi-node deployments) | Randomly generated |
| `CRYPTO_SECRET` | Data encryption key (**required** for Redis scenarios) | - |
| `SQL_DSN` | Database connection string | SQLite |
| `REDIS_CONN_STRING` | Redis connection string | - |
| `STREAMING_TIMEOUT` | Streaming response timeout (seconds) | `300` |
| `P2P_TURN_SECRET` | TURN server shared secret (for WebRTC relay) | - |
| `P2P_RELAY_SECRET` | E2EE Relay server signing key | - |

> [!WARNING]
> You MUST set `SESSION_SECRET` and `CRYPTO_SECRET` for multi-node/production deployments. It is recommended to use MySQL/PostgreSQL + Redis instead of the default SQLite. Please fulfill all applicable compliance obligations before offering this service to the public.

---

## 🔗 P2P AI Token Idle Compute Trading

### What is P2P Compute Trading?

Many people hold subscriptions to ChatGPT Plus, Claude Pro, or Gemini Advanced but don't use up their monthly Token quota. This platform lets you turn those **idle AI quotas** into income:

- **As a Provider**: Install the browser extension, auto-accept tasks in the background, execute them with your locally logged-in AI account, and get paid.
- **As a Client**: Purchase AI execution capability via a unified API / SDK, paying per task — no need to subscribe to each platform separately.
- **As a Script Author**: Use the extension's built-in packet-capture analyzer to write scripts for any AI website, then earn a share of revenue based on adoption.

```
Client posts task + pays fee
  → Platform matches an online node
    → Provider extension executes the task in the local browser using a logged-in AI service
      → Result returned to Client via end-to-end encryption
        → Platform auto-settles; Provider receives earnings
```

---

### Three Roles

#### 🖥️ Provider — AI Quota Supplier

**You have**: ChatGPT Plus / Claude Pro / Gemini Advanced / Midjourney subscriptions.

**You do**: Install extension → choose allowed scripts → set price and daily limit → keep browser open to auto-accept tasks.

**You earn**: Token revenue after each successful task, withdrawable at any time.

| Security Guarantee | Details |
|---------|------|
| Credentials stay local | Passwords and cookies exist only in your local browser — never uploaded |
| Pause anytime | Stop accepting tasks at any time without affecting settled earnings |
| Transparent authorization | Each script must be manually tested and confirmed before you list it |
| Traceable earnings | Every payment traces back to a specific order, receipt, and billing detail |

#### 👤 Client — Task Requester

Create tasks via the Client SDK or HTTP API. Task parameters are end-to-end encrypted — the platform server cannot read the content.

```typescript
import { AiTokenClient } from '@ai-token-p2p/sdk'

const client = new AiTokenClient({ apiKey: 'your-api-key' })

// Query available capabilities (scripts, prices, online node count)
const caps = await client.listCapabilities()

// Create a task (config is E2EE-encrypted before sending)
const order = await client.createOrder({
  scriptId: 'chatgpt-text-v1',
  config: { prompt: 'Write a project plan for me', model: 'gpt-4o' },
  maxCost: 0.05,
  timeoutSeconds: 120,
})

const result = await client.waitForResult(order.id)
console.log(result.output)
```

#### ✍️ Script Author

Use the extension's built-in workbench (`analysis.html`) to generate `runGeneratedTest(config)` scripts for any AI website:

1. Open the target AI site; start packet capture in the analyzer
2. Perform the target operation; AI auto-analyzes requests and generates a script
3. Test locally, then submit for review
4. Upon approval, publish as a hash-locked immutable version
5. All Provider nodes can adopt the script and you earn a usage share

---

### Security Architecture

| Feature | Implementation |
|---------|---------|
| 🔒 End-to-End Encryption | Task params and results are E2EE-encrypted; control plane cannot decrypt |
| 🚫 Credential Isolation | Third-party cookies, passwords, API keys stay local — never uploaded |
| 📜 Immutable Scripts | Hash-locked on publish, cannot be overwritten — prevents supply-chain attacks |
| 🎯 Least Privilege | Scripts can only access their declared target Origin — no cross-site operations |
| ✅ Dual-sided Receipts | Client + Provider dual-signed receipt per transaction, independently verifiable |
| 🛡️ Sandbox Review | Auto-scan + sandbox test + manual review before any script goes live |

---

### Provider Quick Start

**Step 1: Install the extension**

Download the latest plugin `.zip` from [Releases](https://github.com/lakysir/new-api/releases) and unzip it.

In Chrome: `chrome://extensions/` → enable **Developer mode** → **Load unpacked** → select the unzipped folder.

**Step 2: Bind your account**

Click the extension icon → log in to your platform account → complete device binding.

**Step 3: List a capability**

1. Open the target AI website (e.g. `chat.openai.com`) and ensure you are logged in
2. Extension popup → **Script Marketplace** → select a script
3. Read the description (target site, permissions, cost, account risks)
4. Click **Local Test** and confirm success
5. Set your **unit price** and **daily limit**, then click **List**

**Step 4: Accept tasks**

Keep your browser open. The extension auto-accepts and executes tasks in the background. Earnings are logged in real time.

---

## 🌐 New-API Relay Gateway

This project includes a full **New-API large-model relay gateway** — a unified AI API ingress layer supporting multi-model management, format conversion, intelligent routing, and cost accounting.

### What can the relay do?

Just replace `base_url` in your existing code — no other changes needed:

```python
from openai import OpenAI

client = OpenAI(
    api_key="your-platform-token",
    base_url="http://your-server:3000/v1"
)

response = client.chat.completions.create(
    model="gpt-4o",   # also accepts claude-3-5-sonnet-20241022, gemini-2.5-pro, etc.
    messages=[{"role": "user", "content": "Hello"}]
)
print(response.choices[0].message.content)
```

---

### Core Features

#### 🔄 Multi-format API Conversion

| Source | Target | Status |
|--------|---------|------|
| OpenAI Chat ↔ Claude Messages | Bidirectional | ✅ |
| OpenAI Chat → Google Gemini | One-way | ✅ |
| Google Gemini → OpenAI Chat | Text (function calling in progress) | ✅ |
| OpenAI Realtime API (incl. Azure) | Real-time voice | ✅ |
| OpenAI Responses API | New response format | ✅ |

#### ⚖️ Intelligent Routing & High Availability

- **Weighted random**: Assign weights across API keys / providers, auto-balance load
- **Auto-retry**: Switch to backup channel on failure
- **Per-user rate limiting**: Fine-grained per-Token request limits to prevent abuse

#### 💰 Granular Billing

- Per-Token billing with cache-hit discount tracking
- Cache billing for OpenAI, Azure, DeepSeek, Claude, Qwen, and more
- Internal quota allocation (EPay / Stripe)
- Visual dashboard for real-time cost tracking per model and user

#### 🔑 Access Control

- Token groups: restrict model scope, usage limits, expiry, source IP
- Login methods: Discord / Telegram / LinuxDO / OIDC
- Full request logs and usage audit

---

### Supported Models

| Type | Support |
|---------|------|
| OpenAI (Chat / Responses / Realtime) | ✅ Full |
| Claude Messages | ✅ Full |
| Google Gemini | ✅ Full |
| Azure OpenAI | ✅ Full |
| Midjourney (via Proxy) | ✅ Image generation |
| Suno | ✅ Music generation |
| Rerank (Cohere / Jina) | ✅ |
| Image / Audio / Video / Embeddings | ✅ Full |

#### Reasoning Effort

Append a suffix to any model name — no parameter changes needed:

```
gpt-5-high / gpt-5-low
o3-mini-medium
gemini-2.5-pro-high
claude-3-7-sonnet-20250219-thinking
```

---

### Admin Dashboard Quick Start

1. Visit `http://your-server:3000`, log in with the admin account
2. **Channel Management** → add your API keys
3. **Token Management** → create tokens for team members or apps
4. **Dashboard** → view real-time usage, cost, and success rates
5. **Settings → Operations** → configure retries, rate limits, billing policies

---

## 🔗 Related Projects

| Project | Description |
|------|------|
| [New API](https://github.com/QuantumNous/new-api) | Upstream gateway |
| [One API](https://github.com/songquanpeng/one-api) | Original project |
| [Midjourney-Proxy](https://github.com/novicezk/midjourney-proxy) | Midjourney support |

---

## 📜 License

Licensed under [AGPLv3](./LICENSE), built on [New API](https://github.com/QuantumNous/new-api) (MIT).
For commercial licensing inquiries, please open an issue or contact us.

---

<div align="center">

### 💖 Thank you for using the AI Token P2P Platform

If this project helps you, please give us a ⭐️ Star!

</div>
