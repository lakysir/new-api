<div align="center">

![AI Token P2P Platform](./web/default/public/logo.png)

# AI 闲置算力 Token-P2P 交易平台

🚀 **浏览器插件即节点 · AI Token 变现 · New-API 企业级中转网关**

<p align="center">
  <strong>简体中文</strong> |
  <a href="./README.en.md">English</a> |
  <a href="./README.ja.md">日本語</a> |
  <a href="./README.ko.md">한국어</a>
</p>

<p align="center">
  <a href="#%EF%B8%8F-编译与部署">编译部署</a> •
  <a href="#-p2p-ai-token-闲置算力交易">P2P算力交易</a> •
  <a href="#-new-api-中转站">New-API中转站</a>
</p>

</div>

---

## 📌 项目简介

本项目在 [New API](https://github.com/QuantumNous/new-api)（新一代大模型网关）的基础上，构建了一套完整的 **P2P AI 算力交易网络**。

每一位持有 ChatGPT Plus、Claude Pro、Gemini Advanced 等 AI 服务订阅的用户，都可以将每月的**闲置 Token 配额**接入市场，接受来自需求方的任务并获得收益；需求方则通过统一 SDK 低成本调用多种 AI 能力，无需分别订阅各平台。

### 核心亮点

| 特性 | 说明 |
|------|------|
| 🧩 P2P 算力市场 | 浏览器插件即节点，闲置 AI 配额直接变现 |
| 🔒 端对端加密 (E2EE) | 任务参数与结果全程加密，凭据永不离开本机 |
| 📜 市场脚本治理 | 脚本审核、哈希签名、版本不可变，杜绝恶意代码 |
| 🌐 New-API 中转网关 | 兼容 OpenAI / Claude / Gemini 等主流格式，多渠道智能路由 |
| 💰 透明结算账本 | 复式记账，每笔收入可追溯到订单、回执和费率 |
| ⚡ 脚本无需更新 | 新增 AI 网站只需作者上传脚本，插件无需发新版本 |

---

## 🛠️ 编译与部署

### 环境要求

| 组件 | 版本要求 |
|------|---------|
| Go | ≥ 1.21 |
| Node.js | ≥ 18（或使用 Bun ≥ 1.0，构建更快） |
| Docker | ≥ 20.10 |
| Docker Compose | ≥ 2.0 |
| 数据库 | SQLite（默认）/ MySQL ≥ 5.7.8 / PostgreSQL ≥ 9.6 |

---

### 方式一：Docker Compose（推荐）

```bash
# 克隆项目
git clone https://github.com/lakysir/new-api.git
cd new-api

# 按需编辑配置
nano docker-compose.yml   # 配置数据库、Redis、密钥等

# 一键启动
docker-compose up -d

# 查看运行状态
docker-compose ps
docker-compose logs -f new-api
```

🎉 启动完成后访问 `http://localhost:3000` 进入管理后台，默认管理员账号 `root` / `123456`，**首次登录后请立即修改密码**。

---

### 方式二：源码编译

#### 1. 后端（Go）

```bash
cd new-api

# 下载依赖
go mod download

# 编译
go build -ldflags "-s -w" -o new-api-server .

# 运行（默认 SQLite，数据保存在 ./data/）
./new-api-server
```

#### 2. 前端

```bash
cd new-api/web

# 推荐使用 Bun（速度更快）
bun install && bun run build

# 或使用 npm
npm install && npm run build
# 构建产物输出到 web/dist/
```

---

### 方式三：Docker 单命令

```bash
# 使用 SQLite（最简）
docker run --name ai-token-p2p -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  lakysir/new-api:latest

# 使用 MySQL
docker run --name ai-token-p2p -d --restart always \
  -p 3000:3000 \
  -e SQL_DSN="root:password@tcp(db:3306)/aitoken" \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  lakysir/new-api:latest
```

---

### 关键环境变量

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `SESSION_SECRET` | 会话加密密钥（多机部署**必填**） | 随机生成 |
| `CRYPTO_SECRET` | 数据加密密钥（Redis 场景**必填**） | - |
| `SQL_DSN` | 数据库连接字符串 | SQLite |
| `REDIS_CONN_STRING` | Redis 连接字符串 | - |
| `STREAMING_TIMEOUT` | 流式响应超时（秒） | `300` |
| `P2P_TURN_SECRET` | TURN 服务器共享密钥（WebRTC 中继用） | - |
| `P2P_RELAY_SECRET` | E2EE Relay 服务端签名密钥 | - |

> [!WARNING]
> 多机/生产部署时必须设置 `SESSION_SECRET` 和 `CRYPTO_SECRET`；建议使用 MySQL/PostgreSQL + Redis 替代默认 SQLite；面向公众提供服务前请完成相关合规义务。

---

## 🔗 P2P AI Token 闲置算力交易

### 什么是 P2P 算力交易？

很多人手里有 ChatGPT Plus、Claude Pro、Gemini Advanced 等 AI 服务订阅，但每个月的 Token 配额不一定用得完。本平台让你将这些**闲置 AI 配额**变成收益：

- **作为 Provider**：安装浏览器插件，在后台自动接单，用本地已登录的 AI 账号执行任务，获得报酬。
- **作为 Client**：通过统一 API / SDK 购买 AI 执行能力，按次付费，无需订阅多个平台。
- **作为脚本作者**：用插件内置的抓包分析器为任意 AI 网站编写脚本，发布后按采用量获得分成。

```
需求方（Client）发布任务 + 支付费用
  → 平台匹配在线节点
    → Provider 插件在本地浏览器中执行已登录的 AI 服务
      → 结果端到端加密返回需求方
        → 平台自动结算，Provider 获得收益
```

---

### 三种参与角色

#### 🖥️ Provider — AI 配额提供者

**你有**：ChatGPT Plus / Claude Pro / Gemini Advanced / Midjourney 等订阅账号。

**你做**：安装插件 → 选择允许执行的脚本 → 设置价格和每日限额 → 保持浏览器开启等待自动接单。

**你得到**：每次任务成功完成后的 Token 收益，可随时提现。

| 安全保证 | 说明 |
|---------|------|
| 凭据不离本机 | 账号密码、Cookie 仅存在于你的本地浏览器，不上传平台 |
| 可随时暂停 | 任何时候都可以关闭接单，不影响已结算收益 |
| 授权透明 | 每个脚本在上架前需你手动测试并确认允许执行 |
| 收入可追溯 | 每笔收益都可追溯到具体订单、回执和计费明细 |

#### 👤 Client — 任务需求方

通过 Client SDK 或 HTTP API 创建任务，任务参数端到端加密，平台服务端无法读取内容。

```typescript
import { AiTokenClient } from '@ai-token-p2p/sdk'

const client = new AiTokenClient({ apiKey: 'your-api-key' })

// 查询可用能力（支持的脚本、价格、在线节点数）
const caps = await client.listCapabilities()

// 创建任务（config 会被 E2EE 加密后发送）
const order = await client.createOrder({
  scriptId: 'chatgpt-text-v1',
  config: { prompt: '帮我写一份项目计划书', model: 'gpt-4o' },
  maxCost: 0.05,        // 最多支付 0.05 USD
  timeoutSeconds: 120,
})

// 等待结果（结果同样 E2EE 加密传输）
const result = await client.waitForResult(order.id)
console.log(result.output)
```

#### ✍️ Script Author — 脚本作者

使用插件内置的抓包分析工作台（`analysis.html`）为任意 AI 网站生成 `runGeneratedTest(config)` 脚本：

1. 打开目标 AI 网站，在插件分析器中启动抓包
2. 执行一次目标操作，AI 自动分析 Network 请求并生成脚本
3. 在本地测试通过后上传审核
4. 审核通过后发布为哈希锁定的不可变版本
5. 所有 Provider 节点可选用该脚本，按使用量获得分成

---

### 安全架构设计

| 安全特性 | 实现方式 |
|---------|---------|
| 🔒 端到端加密 | 任务参数与结果通过 E2EE 加密，平台控制面无法解密 |
| 🚫 凭据隔离 | 第三方 Cookie、密码、API Key 只在 Provider 本地使用，永不上传 |
| 📜 脚本不可变 | 发布版本哈希锁定，无法覆盖修改，防止供应链攻击 |
| 🎯 最小权限 | 每个脚本只能访问声明的目标网站 Origin，不能跨站操作 |
| ✅ 双边回执 | 每笔交易生成 Client + Provider 双边签名回执，任意一方可独立验证 |
| 🛡️ 沙箱审核 | 脚本上传后经自动扫描 + 沙箱测试 + 人工审核，才能发布上线 |

---

### Provider 快速上手

**第一步：安装插件**

从 [Releases](https://github.com/lakysir/new-api/releases) 下载最新插件包（`.zip` 文件），解压后：

在 Chrome 中打开 `chrome://extensions/` → 开启**开发者模式** → 点击**加载已解压的扩展程序** → 选择解压后的插件目录，即可完成安装。

**第二步：绑定账号**

点击插件图标 → 登录平台账号 → 完成设备绑定。

**第三步：上架能力**

1. 打开目标 AI 网站（如 `chat.openai.com`）并确保已登录
2. 插件弹窗 → **脚本市场** → 选择一个脚本
3. 阅读脚本说明（目标网站、所需权限、消耗与账号风险）
4. 点击**本地测试**，确认执行成功
5. 设置**单价**和**每日限额**，点击**上架**

**第四步：持续接单**

保持浏览器开启，插件在后台自动接收并执行任务。收益实时记录，可随时在控制台查看。

---

## 🌐 New-API 中转站

本项目集成了完整的 **New-API 大模型中转网关**，可作为企业或个人的 AI API 统一接入层，支持多种主流 AI 服务的统一管理、格式转换、智能路由和成本核算。

### 中转站能做什么？

只需将现有代码的 `base_url` 替换为本平台地址，即可无缝接入，无需改动其他任何代码：

```python
from openai import OpenAI

client = OpenAI(
    api_key="your-platform-token",   # 平台签发的 Token，非原始 API Key
    base_url="http://your-server:3000/v1"
)

response = client.chat.completions.create(
    model="gpt-4o",     # 也可以填 claude-3-5-sonnet-20241022、gemini-2.5-pro 等
    messages=[{"role": "user", "content": "你好"}]
)
print(response.choices[0].message.content)
```

---

### 核心功能

#### 🔄 多格式 API 转换

用一套 OpenAI 兼容接口调用所有主流大模型，无需学习各家 SDK：

| 源格式 | 目标格式 | 状态 |
|--------|---------|------|
| OpenAI Chat ↔ Claude Messages | 双向互转 | ✅ |
| OpenAI Chat → Google Gemini | 单向 | ✅ |
| Google Gemini → OpenAI Chat | 文本（函数调用开发中） | ✅ |
| OpenAI Realtime API（含 Azure）| 实时语音对话 | ✅ |
| OpenAI Responses API | 新版响应格式 | ✅ |

#### ⚖️ 智能路由与高可用

- **渠道加权随机**：为多个 API Key 或服务商分配权重，自动均衡负载
- **失败自动重试**：某个渠道请求失败时，自动切换备用渠道
- **用户级限流**：精细控制每个 Token 的每分钟/每天请求上限，防止滥用

#### 💰 精细化计费与成本管理

- 按 Token 计费，支持缓存命中折扣统计
- 兼容 OpenAI、Azure、DeepSeek、Claude、Qwen 等主流模型的缓存计费
- 组织内部额度分配（易支付 / Stripe 充值）
- 可视化数据看板，实时掌握各模型、各用户的用量与成本

#### 🔑 权限管理

- Token 分组，可限制：可用模型范围、最大用量、有效期、请求来源 IP
- 多种登录方式：Discord / Telegram / LinuxDO / OIDC
- 完整的请求日志与用量审计

---

### 支持的模型与接口

| 模型类型 | 说明 |
|---------|------|
| OpenAI（Chat / Responses / Realtime） | ✅ 完整支持 |
| Claude Messages | ✅ 完整支持 |
| Google Gemini | ✅ 完整支持 |
| Azure OpenAI | ✅ 完整支持 |
| Midjourney（via Proxy） | ✅ 图像生成 |
| Suno | ✅ 音乐生成 |
| Rerank（Cohere / Jina） | ✅ 向量重排 |
| 图像 / 音频 / 视频 / 嵌入 | ✅ 全接口覆盖 |

#### Reasoning Effort 支持

可在模型名称上直接声明推理强度，无需修改参数：

```
gpt-5-high          # GPT-5 高推理
gpt-5-low           # GPT-5 低推理
o3-mini-medium      # o3-mini 中等推理
gemini-2.5-pro-high # Gemini 高推理
claude-3-7-sonnet-20250219-thinking  # Claude 启用思考模式
```

---

### 管理后台快速上手

1. 访问 `http://your-server:3000`，使用管理员账号登录
2. **渠道管理** → 添加你的 API Key（OpenAI / Anthropic / Google 等）
3. **令牌管理** → 创建平台 Token 分发给团队成员或应用
4. **数据看板** → 实时查看各渠道用量、成本和成功率
5. **设置 → 运营设置** → 配置重试次数、限流规则、计费策略

---

## 🔗 相关项目

| 项目 | 说明 |
|------|------|
| [New API](https://github.com/QuantumNous/new-api) | 上游网关基础项目 |
| [One API](https://github.com/songquanpeng/one-api) | 原版项目基础 |
| [Midjourney-Proxy](https://github.com/novicezk/midjourney-proxy) | Midjourney 接口支持 |

---

## 📜 许可证

本项目基于 [GNU Affero 通用公共许可证 v3.0 (AGPLv3)](./LICENSE) 发布，在 [New API](https://github.com/QuantumNous/new-api)（MIT 许可证）基础上二次开发。

如果您所在的组织政策不允许使用 AGPLv3 许可的软件，请联系我们获取商业许可。

---

<div align="center">

### 💖 感谢使用 AI Token P2P 平台

如果这个项目对你有帮助，欢迎给我们一个 ⭐️ Star！

</div>

