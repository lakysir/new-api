<div align="center">

![AI Token P2P Platform](./web/default/public/logo.png)

# AI 유휴 연산력 Token-P2P 거래 플랫폼

🚀 **브라우저 확장이 노드 · AI Token 수익화 · New-API 엔터프라이즈 릴레이 게이트웨이**

<p align="center">
  <a href="./README.md">简体中文</a> |
  <a href="./README.en.md">English</a> |
  <a href="./README.ja.md">日本語</a> |
  <strong>한국어</strong>
</p>

<p align="center">
  <a href="#%EF%B8%8F-빌드-및-배포">빌드·배포</a> •
  <a href="#-p2p-ai-token-유휴-연산력-거래">P2P 연산력 거래</a> •
  <a href="#-new-api-릴레이-게이트웨이">New-API 릴레이</a>
</p>

</div>

---

## 📌 프로젝트 소개

본 프로젝트는 [New API](https://github.com/QuantumNous/new-api)（차세대 대규모 모델 게이트웨이）를 기반으로 완전한 **P2P AI 연산력 거래 네트워크**를 구축한 것입니다.

ChatGPT Plus, Claude Pro, Gemini Advanced 등의 AI 서비스를 구독하는 모든 사용자는 매월 사용하지 못한 **유휴 Token 할당량**을 마켓에 연결하여 클라이언트의 작업을 수행하고 수익을 얻을 수 있습니다. 클라이언트는 통합 SDK를 통해 저비용으로 다양한 AI 능력을 호출할 수 있으며, 각 플랫폼에 개별 구독할 필요가 없습니다.

### 핵심 특징

| 특징 | 설명 |
|------|------|
| 🧩 P2P 연산력 마켓 | 브라우저 확장이 노드 — 유휴 AI 할당량을 직접 수익화 |
| 🔒 엔드투엔드 암호화 (E2EE) | 작업 파라미터와 결과 전과정 암호화, 자격 증명은 로컬을 벗어나지 않음 |
| 📜 마켓 스크립트 거버넌스 | 스크립트 심사·해시 서명·버전 불변 — 악성 코드 방지 |
| 🌐 New-API 릴레이 게이트웨이 | OpenAI / Claude / Gemini 등 주요 형식 호환, 멀티 채널 지능형 라우팅 |
| 💰 투명한 정산 원장 | 복식 부기로 모든 수익을 주문·영수증·요금까지 추적 가능 |
| ⚡ 스크립트 업데이트 불필요 | 새로운 AI 사이트 추가 시 스크립트 업로드만 하면 됨 — 확장 업데이트 불필요 |

---

## 🛠️ 빌드 및 배포

### 환경 요구 사항

| 컴포넌트 | 버전 요구 사항 |
|------|---------|
| Go | ≥ 1.21 |
| Node.js | ≥ 18 (또는 Bun ≥ 1.0, 빠른 빌드 권장) |
| Docker | ≥ 20.10 |
| Docker Compose | ≥ 2.0 |
| 데이터베이스 | SQLite (기본값) / MySQL ≥ 5.7.8 / PostgreSQL ≥ 9.6 |

---

### 방법 1: Docker Compose (권장)

```bash
git clone https://github.com/lakysir/new-api.git
cd new-api
nano docker-compose.yml   # 데이터베이스, Redis, 시크릿 키 등 설정
docker-compose up -d
docker-compose ps
```

🎉 시작 후 `http://localhost:3000`에 접속합니다. 기본 관리자 계정: `root` / `123456` — **최초 로그인 후 즉시 비밀번호를 변경하세요**.

---

### 방법 2: 소스코드 빌드

#### 1. 백엔드 (Go)

```bash
cd new-api
go mod download
go build -ldflags "-s -w" -o new-api-server .
./new-api-server
```

#### 2. 프론트엔드

```bash
cd new-api/web
bun install && bun run build
# 또는 npm install && npm run build
# 빌드 결과물: web/dist/
```

---

### 방법 3: Docker 단일 명령어

```bash
# SQLite 사용 (가장 간단)
docker run --name ai-token-p2p -d --restart always \
  -p 3000:3000 -e TZ=Asia/Seoul -v ./data:/data \
  lakysir/new-api:latest

# MySQL 사용
docker run --name ai-token-p2p -d --restart always \
  -p 3000:3000 \
  -e SQL_DSN="root:password@tcp(db:3306)/aitoken" \
  -e TZ=Asia/Seoul -v ./data:/data \
  lakysir/new-api:latest
```

---

### 주요 환경 변수

| 변수명 | 설명 | 기본값 |
|--------|------|--------|
| `SESSION_SECRET` | 세션 암호화 키 (멀티 노드 환경 **필수**) | 자동 생성 |
| `CRYPTO_SECRET` | 데이터 암호화 키 (Redis 사용 시 **필수**) | - |
| `SQL_DSN` | 데이터베이스 연결 문자열 | SQLite |
| `REDIS_CONN_STRING` | Redis 연결 문자열 | - |
| `STREAMING_TIMEOUT` | 스트리밍 응답 타임아웃 (초) | `300` |
| `P2P_TURN_SECRET` | TURN 서버 공유 시크릿 (WebRTC 릴레이용) | - |
| `P2P_RELAY_SECRET` | E2EE Relay 서버 서명 키 | - |

> [!WARNING]
> 멀티 노드/프로덕션 환경에서는 반드시 `SESSION_SECRET`과 `CRYPTO_SECRET`을 설정하세요. 기본 SQLite 대신 MySQL/PostgreSQL + Redis 사용을 강력히 권장합니다.

---

## 🔗 P2P AI Token 유휴 연산력 거래

### P2P 연산력 거래란?

ChatGPT Plus, Claude Pro, Gemini Advanced 등을 구독하고 있지만 매월 Token 할당량을 다 쓰지 못하는 분들을 위한 플랫폼입니다. 이 **유휴 AI 할당량**을 수익으로 바꿀 수 있습니다:

- **Provider로서**: 브라우저 확장을 설치하고 백그라운드에서 자동으로 작업을 수락하여 로컬에 로그인된 AI 계정으로 실행한 뒤 보상을 받습니다.
- **Client로서**: 통합 API / SDK를 통해 작업당 비용으로 AI 실행 능력을 구매합니다. 각 플랫폼 개별 구독 불필요.
- **스크립트 작성자로서**: 내장 패킷 캡처 분석기로 임의의 AI 사이트용 스크립트를 작성하고 채택량에 따라 수익을 받습니다.

```
Client가 작업 게시 + 비용 지불
  → 플랫폼이 온라인 노드 매칭
    → Provider 확장이 로컬 브라우저의 AI 서비스로 작업 실행
      → 결과를 E2EE 암호화하여 Client에 반환
        → 플랫폼 자동 정산, Provider가 수익 수령
```

---

### 세 가지 참여 역할

#### 🖥️ Provider — AI 할당량 제공자

**당신이 가진 것**: ChatGPT Plus / Claude Pro / Gemini Advanced / Midjourney 등의 구독.

**당신이 할 일**: 확장 설치 → 허용할 스크립트 선택 → 가격과 일일 한도 설정 → 브라우저를 열어 자동 수주 대기.

**당신이 얻는 것**: 작업 완료마다 Token 수익 (언제든지 인출 가능).

| 보안 보장 | 내용 |
|---------|------|
| 자격 증명은 로컬에 유지 | 비밀번호와 Cookie는 로컬 브라우저에만 존재, 플랫폼에 업로드되지 않음 |
| 언제든지 일시 정지 가능 | 이미 정산된 수익에 영향 없이 수주를 중단 가능 |
| 투명한 권한 부여 | 각 스크립트는 등록 전 수동 테스트 및 확인 필요 |
| 수익 추적 가능 | 모든 수익을 구체적인 주문·영수증·청구 내역까지 추적 가능 |

#### 👤 Client — 작업 의뢰자

Client SDK 또는 HTTP API로 작업을 생성합니다. 작업 파라미터는 E2EE 암호화되어 플랫폼 서버가 내용을 읽을 수 없습니다.

```typescript
import { AiTokenClient } from '@ai-token-p2p/sdk'

const client = new AiTokenClient({ apiKey: 'your-api-key' })

const order = await client.createOrder({
  scriptId: 'chatgpt-text-v1',
  config: { prompt: '프로젝트 계획서를 작성해주세요', model: 'gpt-4o' },
  maxCost: 0.05,
  timeoutSeconds: 120,
})

const result = await client.waitForResult(order.id)
console.log(result.output)
```

#### ✍️ 스크립트 작성자

1. 대상 AI 사이트를 열고 분석기에서 패킷 캡처 시작
2. 대상 동작을 실행하면 AI가 자동으로 스크립트 생성
3. 로컬에서 테스트 후 심사 제출
4. 승인 후 해시 고정된 불변 버전으로 게시
5. 모든 Provider 노드가 스크립트를 채택할 수 있으며 사용량에 따른 수익 수령

---

### 보안 아키텍처

| 보안 기능 | 구현 방식 |
|---------|---------|
| 🔒 E2EE | 작업 파라미터·결과 암호화, 컨트롤 플레인 복호화 불가 |
| 🚫 자격 증명 격리 | 서드파티 Cookie·비밀번호·API Key는 Provider 로컬에서만 사용 |
| 📜 스크립트 불변성 | 게시 버전 해시 고정, 덮어쓰기 불가, 공급망 공격 방지 |
| 🎯 최소 권한 | 각 스크립트는 선언된 대상 Origin에만 접근 가능 |
| ✅ 양방향 영수증 | 거래마다 Client + Provider 양방향 서명 영수증 생성, 독립 검증 가능 |
| 🛡️ 샌드박스 심사 | 자동 스캔 + 샌드박스 테스트 + 수동 심사 후 게시 |

---

### Provider 빠른 시작

**1단계: 확장 설치**

[Releases](https://github.com/lakysir/new-api/releases)에서 최신 플러그인 패키지(`.zip`)를 다운로드하여 압축을 해제합니다.

Chrome에서 `chrome://extensions/` → **개발자 모드** 활성화 → **압축 해제된 확장 프로그램 로드** → 압축 해제된 폴더 선택.

**2단계: 계정 연결**

확장 아이콘 클릭 → 플랫폼 계정으로 로그인 → 기기 바인딩 완료.

**3단계: 능력 등록**

1. 대상 AI 사이트(예: `chat.openai.com`)를 열고 로그인 상태 확인
2. 확장 팝업 → **스크립트 마켓** → 스크립트 선택
3. 설명 확인 (대상 사이트, 필요 권한, 소비량, 계정 위험)
4. **로컬 테스트** 클릭 후 성공 확인
5. **단가**와 **일일 한도** 설정 후 **등록** 클릭

**4단계: 작업 수락**

브라우저를 열어 두세요. 확장이 백그라운드에서 자동으로 작업을 수락·실행합니다. 수익은 실시간으로 기록되며 언제든지 콘솔에서 확인할 수 있습니다.

---

## 🌐 New-API 릴레이 게이트웨이

완전한 **New-API 대규모 모델 릴레이 게이트웨이**가 통합되어 있어, 기업 또는 개인의 AI API 통합 접속 레이어로 활용할 수 있습니다.

### 릴레이 게이트웨이로 무엇을 할 수 있나요?

기존 코드의 `base_url`만 교체하면 됩니다:

```python
from openai import OpenAI

client = OpenAI(
    api_key="your-platform-token",
    base_url="http://your-server:3000/v1"
)

response = client.chat.completions.create(
    model="gpt-4o",   # claude-3-5-sonnet-20241022, gemini-2.5-pro 등도 지정 가능
    messages=[{"role": "user", "content": "안녕하세요"}]
)
print(response.choices[0].message.content)
```

---

### 핵심 기능

#### 🔄 멀티 포맷 API 변환

| 소스 | 대상 | 상태 |
|--------|---------|------|
| OpenAI Chat ↔ Claude Messages | 양방향 | ✅ |
| OpenAI Chat → Google Gemini | 단방향 | ✅ |
| Google Gemini → OpenAI Chat | 텍스트 (함수 호출 개발 중) | ✅ |
| OpenAI Realtime API (Azure 포함) | 실시간 음성 대화 | ✅ |
| OpenAI Responses API | 새 응답 형식 | ✅ |

#### ⚖️ 지능형 라우팅 및 고가용성

- **가중 랜덤 선택**: 여러 API Key 또는 제공자에 가중치를 부여하여 자동 부하 분산
- **실패 시 자동 재시도**: 요청 실패 시 백업 채널로 자동 전환
- **사용자 레벨 속도 제한**: 각 Token의 분당/일당 요청 한도를 세밀하게 제어

#### 💰 세밀한 청구 및 비용 관리

- Token 단위 청구, 캐시 히트 할인 통계 지원
- OpenAI, Azure, DeepSeek, Claude, Qwen 등 주요 모델 캐시 청구 호환
- 조직 내부 할당량 배분 (EPay / Stripe 충전)
- 시각화 대시보드로 모델·사용자별 사용량과 비용을 실시간 파악

#### 🔑 접근 제어

- Token 그룹화: 사용 가능 모델 범위·최대 사용량·만료일·소스 IP 제한 가능
- 로그인 방식: Discord / Telegram / LinuxDO / OIDC
- 완전한 요청 로그 및 사용량 감사

---

### 지원 모델 및 인터페이스

| 모델 유형 | 지원 |
|---------|------|
| OpenAI (Chat / Responses / Realtime) | ✅ 완전 지원 |
| Claude Messages | ✅ 완전 지원 |
| Google Gemini | ✅ 완전 지원 |
| Azure OpenAI | ✅ 완전 지원 |
| Midjourney (via Proxy) | ✅ 이미지 생성 |
| Suno | ✅ 음악 생성 |
| Rerank (Cohere / Jina) | ✅ 벡터 재순위 |
| 이미지 / 오디오 / 비디오 / 임베딩 | ✅ 전체 인터페이스 |

#### Reasoning Effort 지원

모델 이름에 접미사를 추가하기만 하면 됩니다:

```
gpt-5-high / gpt-5-low
o3-mini-medium
gemini-2.5-pro-high
claude-3-7-sonnet-20250219-thinking
```

---

### 관리 대시보드 빠른 시작

1. `http://your-server:3000`에 접속, 관리자 계정으로 로그인
2. **채널 관리** → API Key 추가 (OpenAI / Anthropic / Google 등)
3. **토큰 관리** → 팀원이나 앱을 위한 플랫폼 토큰 생성
4. **대시보드** → 채널별 사용량·비용·성공률 실시간 확인
5. **설정 → 운영 설정** → 재시도 횟수·속도 제한·청구 정책 설정

---

## 🔗 관련 프로젝트

| 프로젝트 | 설명 |
|------|------|
| [New API](https://github.com/QuantumNous/new-api) | 업스트림 게이트웨이 기반 프로젝트 |
| [One API](https://github.com/songquanpeng/one-api) | 원본 프로젝트 |
| [Midjourney-Proxy](https://github.com/novicezk/midjourney-proxy) | Midjourney 인터페이스 지원 |

---

## 📜 라이선스

본 프로젝트는 [GNU Affero General Public License v3.0 (AGPLv3)](./LICENSE)에 따라 라이선스되며, [New API](https://github.com/QuantumNous/new-api) (MIT 라이선스)를 기반으로 2차 개발되었습니다.

AGPLv3 라이선스 소프트웨어 사용이 조직 정책상 허용되지 않는 경우, 상업용 라이선스에 대해 문의해 주세요.

---

<div align="center">

### 💖 AI Token P2P 플랫폼을 이용해 주셔서 감사합니다

이 프로젝트가 도움이 되었다면 ⭐️ Star를 눌러 주세요!

</div>
