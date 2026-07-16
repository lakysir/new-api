/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

// AitokenApiDocsPage renders the "purchase & run" flow as a self-contained
// integration guide so a buyer can drive the same marketplace endpoints the
// dashboard page uses (quote → order → E2EE relay → receipt) from their own
// site with a new-api token. It lives on a PUBLIC route (no login) so the whole
// page can be shared with an AI/developer for integration. The content mirrors
// the live implementation in aitoken-purchase-page.tsx / client-relay-session.ts
// / dataplane-browser.ts; keep it in sync when those change.

import { PublicLayout } from '@/components/layout'
import { Markdown } from '@/components/ui/markdown'

// Written as one Markdown string (rendered by the shared Markdown component)
// rather than i18n keys: it is a reference document that stays coherent as a
// whole, and the code samples must be copyable verbatim.
const API_DOCS_MARKDOWN = `本页面「购买并执行」的全部能力都由一组 HTTP 接口 + 一条端到端加密（E2EE）中继 WebSocket 组成。你可以用 new-api 的令牌（API Key）直接调用它们，把该功能接入到自己的网站。

## 概览

一次完整的「购买并执行」按下面顺序进行：

1. 选择脚本与版本，拉取服务商报价（offers）
2. 询价（quote）得到明细价格
3. 创建订单（order）并预留资金 —— 只有参数的哈希会上链，明文不经过控制面
4. 通过 E2EE 中继把明文参数发给执行方，等待加密结果返回
5. 提交客户端回执（receipt），控制面比对双方回执后结算

> 关键设计：任务参数（config）的**明文永远不经过服务器**。控制面只看到 \`input_hash\`（SHA-256）；明文经 X25519 + AES-256-GCM 加密后，只在你与执行方之间的中继链路上传输。

## 金额单位

- 报价、扣费等所有金额均以**人民币**计价。
- 接口里带 \`_micros\` 后缀的字段（如 \`price_micros\`、\`max_amount_micros\`、以及报价明细里的各项）单位为 **micros（微元）**：**1 元人民币 = 1,000,000 micros**。
- 例如 \`price_micros: 200000\` 表示 **0.2 元**，\`235000\` 表示 **0.235 元**。
- 注意：系统底层以「美元」作为计价币种，部分响应里会看到 \`currency\`/\`Currency\` 字段值为 \`"USD"\`。**本系统美元与人民币按 1:1 结算**，因此这些金额可直接按人民币理解，无需换算。

## 认证

所有 HTTP 接口都接受 new-api 的 **API Key**，放在请求头：

\`\`\`
Authorization: Bearer sk-xxxxxxxxxxxxxxxx
Content-Type: application/json
\`\`\`

服务端中间件（\`DeviceOrUserAuth\`）依次尝试：仪表盘会话 → 设备令牌 → API Key。用 API Key 时无需再带 \`New-Api-User\` 头，令牌自身即可确定用户身份。

Base URL 为你的 new-api 部署地址（与控制台同源），例如 \`https://your-newapi.com\`。下文路径均相对该地址。

### 统一响应格式

所有 HTTP 接口返回统一信封：

\`\`\`json
{ "success": true, "message": "", "data": { } }
\`\`\`

\`success=false\` 时读取 \`message\` 作为错误原因；业务数据在 \`data\` 中。

## HTTP 接口

### 1. 拉取某脚本版本的服务商报价

\`GET /api/scripts/{scriptId}/offers?version={version}&provider_group_id={可选}&consume_multiplier={可选}\`

返回 \`data\` 为报价数组，每项：

\`\`\`json
{
  "node_id": "node-abc",
  "provider_group_id": "grp-1",
  "provider_group_name": "provider-name",
  "price_micros": 200000,
  "online": true,
  "busy": false,
  "remaining_quota": 42,
  "available": true,
  "unavailable_reason": "",
  "executions": 120,
  "successes": 118
}
\`\`\`

你可以让平台自动挑选（不传 \`node_id\`，见下），也可以从这里挑一个具体 \`node_id\` 下单。\`consume_multiplier\` 是「工作量系数」（最小 1），费用 = 基础价 × 该系数。

### 2. 询价

\`POST /api/orders/quote\`

\`\`\`json
{
  "script_id": 12,
  "version": 3,
  "node_id": "",
  "provider_group_id": "",
  "consume_multiplier": 1
}
\`\`\`

- 传 \`node_id\`：对指定服务商询价。
- 不传 \`node_id\`（自动模式）：平台按整组定价，可用 \`provider_group_id\` 限定分组。

返回 \`data\`：

\`\`\`json
{
  "breakdown": {
    "Currency": "USD",
    "ProviderMicros": 200000,
    "AuthorMicros": 20000,
    "PlatformFeeMicros": 10000,
    "RiskReserveMicros": 5000,
    "MaxCustomerMicros": 235000
  },
  "chosen_node_id": "node-abc"
}
\`\`\`

\`MaxCustomerMicros\` 是本次最多扣费额（单位 micros），需 ≤ 你的市场可用余额。上例 \`235000\` 即 **0.235 元**。

### 3. 创建订单（预留资金）

\`POST /api/orders\`

**必须**携带 \`Idempotency-Key\` 请求头（任意唯一字符串），用于去重，重试不会重复预留资金。

\`\`\`
Idempotency-Key: order-1699999999999-abc123
\`\`\`

Body：

\`\`\`json
{
  "script_id": 12,
  "version": 3,
  "node_id": "",
  "provider_group_id": "",
  "input_hash": "sha256:9f86d0818...",
  "consume_multiplier": 1
}
\`\`\`

\`input_hash\` 是任务参数的哈希：对参数对象做稳定 JSON 序列化后取 SHA-256，格式 \`sha256:<hex>\`。**明文参数不放在这里**，只在下方「E2EE 中继」环节经加密发送。

返回 \`data\`：

\`\`\`json
{
  "order": {
    "id": "ord_xxx",
    "client_id": 1001,
    "script_id": 12,
    "version": 3,
    "state": "RESERVED",
    "input_hash": "sha256:...",
    "max_amount_micros": 235000,
    "chosen_node_id": "node-abc"
  },
  "created": true
}
\`\`\`

若 \`state\` 为 \`REFUNDED\`，表示服务商拒单、预留资金已退回。\`RESERVED\`/\`DATA_READY\`/\`RUNNING\`/\`OFFERED\` 表示可以继续执行（下方「E2EE 中继」环节）。

### 4. 查询 / 取消订单

- \`GET /api/orders/{id}\` —— 返回订单当前状态。失败态下 \`last_error\` 给出真实原因码（如 \`ORIGIN_NOT_ALLOWED\`、\`SCRIPT_EXECUTION_FAILED\`）。
- \`POST /api/orders/{id}/cancel\` —— 仅 \`FUNDS_RESERVED\`/\`MATCHING\`/\`OFFERED\` 等执行前状态可取消并退款。

轮询 \`GET /api/orders/{id}\` 可用于在等待中继结果时快速发现终态失败（\`FAILED\`/\`REFUNDED\`/\`TIMED_OUT\`/\`CANCELLED\`）。

## E2EE 中继（发送参数并取回结果）

订单创建后，明文参数经端到端加密中继发给执行方。中继服务器只转发不透明二进制帧，不持有密钥、看不到明文。

### 连接

\`GET wss://your-newapi.com/api/relay?task_id={订单id}&attempt=1&role=client\`

- \`task_id\` = 订单 id（MVP 阶段一致），\`attempt\` = 1。
- **认证**：浏览器无法给 WebSocket 设置请求头，因此令牌放在子协议里：
  \`Sec-WebSocket-Protocol: aitoken, <你的 API Key>\`
  服务端会校验该令牌对应用户是否等于订单的 \`client_id\`。

### 握手与加密（协议细节）

双方（client / provider）在带内交换临时 X25519 公钥，各自派生 AES-256-GCM 会话密钥：

- 帧格式：第 1 字节为 tag（\`0x01\`=握手，\`0x02\`=数据），其后为负载。
- 握手负载是 JSON：\`{ "role": "client", "pub": "<base64 X25519 公钥>", "device_id": "client-<订单client_id>" }\`。
- 密钥派生：\`sharedSecret = X25519(本方私钥, 对方公钥)\`，再用 HKDF-SHA256 派生，\`info\` 字符串为
  \`ai-token-p2p:dataplane:v1.0|task=<taskId>|attempt=<n>|client=<clientDeviceId>|provider=<providerDeviceId>|dir=<c2p|p2c>\`，salt 为空。
- 方向：client 用 \`c2p\` 密钥加密发送、用 \`p2c\` 密钥解密。
- 数据帧：8 字节大端序列号 header（同时作为 AES-GCM 的 AAD）+ 密文；nonce 为 12 字节，后 8 字节写入该序列号。序列号从 0 开始逐帧递增，接收端严格按序校验。

client 发送一帧加密的 config JSON，等待 provider 回一帧加密结果 JSON。默认结果超时 120s、握手超时 30s。

> 前端已有可复用实现：\`src/features/node-platform/lib/dataplane-browser.ts\`（Web Crypto 加解密原语）与 \`lib/client-relay-session.ts\`（\`ClientRelaySession\` 会话封装）。接入自己的站点时可直接复用这两处，或按上述协议在服务端/其它语言重写（与插件端 \`dataplane_vector.json\` 向量兼容）。

### 提交回执并结算

拿到结果后，对结果做 SHA-256，提交客户端回执：

\`POST /api/orders/{id}/receipts\`

\`\`\`json
{
  "task_id": "ord_xxx",
  "attempt": 1,
  "party": "client",
  "order_id": "ord_xxx",
  "result_hash": "sha256:<结果哈希>"
}
\`\`\`

控制面比对买卖双方回执，一致即结算。之后再 \`GET /api/orders/{id}\` 可看到终态。

## 端到端示例（TypeScript / 浏览器）

\`\`\`ts
const BASE = 'https://your-newapi.com'
const KEY = 'sk-xxxx'
const auth = { Authorization: 'Bearer ' + KEY, 'Content-Type': 'application/json' }

async function unwrap(res: Response) {
  const j = await res.json()
  if (!j.success) throw new Error(j.message || 'request failed')
  return j.data
}

async function sha256Hex(text: string) {
  const d = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(text))
  return 'sha256:' + [...new Uint8Array(d)].map((b) => b.toString(16).padStart(2, '0')).join('')
}

const scriptId = 12
const version = 3
const config = { prompt: 'a dog' }

// 1) 询价（自动挑选服务商）
const quote = await unwrap(
  await fetch(BASE + '/api/orders/quote', {
    method: 'POST',
    headers: auth,
    body: JSON.stringify({ script_id: scriptId, version, consume_multiplier: 1 }),
  })
)

// 2) 创建订单（幂等）
const inputHash = await sha256Hex(JSON.stringify(config))
const idem = 'order-' + Date.now() + '-' + Math.random().toString(36).slice(2)
const { order } = await unwrap(
  await fetch(BASE + '/api/orders', {
    method: 'POST',
    headers: { ...auth, 'Idempotency-Key': idem },
    body: JSON.stringify({ script_id: scriptId, version, input_hash: inputHash, consume_multiplier: 1 }),
  })
)

// 3) 通过 E2EE 中继发送参数、取回结果
//    复用 ClientRelaySession，令牌通过 deviceToken 走 Sec-WebSocket-Protocol 子协议
const relayUrl = BASE.replace(/^http/, 'ws') + '/api/relay'
const session = new ClientRelaySession({
  relayUrl,
  deviceToken: KEY,
  taskId: order.id,
  attempt: 1,
  clientDeviceId: 'client-' + order.client_id,
})
await session.connect()
await session.waitEstablished()
await session.sendConfig(config)
const result = await session.waitForResult()
session.close()

// 4) 提交客户端回执
await fetch(BASE + '/api/orders/' + order.id + '/receipts', {
  method: 'POST',
  headers: auth,
  body: JSON.stringify({
    task_id: order.id,
    attempt: 1,
    party: 'client',
    order_id: order.id,
    result_hash: await sha256Hex(JSON.stringify(result ?? null)),
  }),
})

console.log('执行结果：', result)
\`\`\`

> 说明：网页版仪表盘用登录会话认证中继，示例里改为把 API Key 作为 \`deviceToken\` 传入 —— 它会作为 \`Sec-WebSocket-Protocol\` 的第二个子协议发送，服务端据此校验并匹配订单归属。
`

export function AitokenApiDocsPage() {
  return (
    <PublicLayout>
      <div className='mx-auto max-w-4xl py-6'>
        <h1 className='mb-6 text-3xl font-semibold tracking-tight'>
          AiToken P2P 市场 · API 接口调用说明
        </h1>
        <Markdown>{API_DOCS_MARKDOWN}</Markdown>
      </div>
    </PublicLayout>
  )
}
