# ChatGo 手机端中继连接设计（MyGo 原生 App + Relay）

> 日期：2026-11-18 · 状态：P1–P4 核心链路已实现（MVP），P5 已完成防重放与令牌失效处理
> 调研：[MyGo Mobile 调研](../../research/2026-11-18-mygo-mobile.md)
> 目标：手机端通过中继服务器连接桌面 ChatGo，完整使用 **Chat / Room / 自动任务**，桌面不需要公网 IP、不需要端口映射。

---

## 0. 结论先说

| 问题 | 结论 | 置信度 |
|------|------|--------|
| 中继是否必要？ | 需要。桌面在 NAT/防火墙后，手机在 4G/5G 时，现有 `127.0.0.1:8422` 的 Web 服务不可达。中继让桌面**主动外连**，手机从公网接入。 | 高 |
| 中继做什么？ | **只做传输**：配对、鉴权、路由、隧道、推送。不复制会话/群聊/任务业务逻辑。 | 高 |
| 手机端用什么？ | **MyGo v0.10 原生 App**（`ui/mobile` + 底部 Tab + 导航栈）；**不使用 WebView**，也不把 PWA 作为手机客户端。 | 高 |
| 现有后端要改多少？ | 核心 API 不改：`/api/sessions`、`/api/rooms`、`/api/tasks` 已经覆盖三个功能。新增的是中继连接器、配对、推送钩子和可选的事件流。 | 高 |
| 自动任务在哪跑？ | **桌面端**。手机负责查看、创建/编辑、立即运行、开关和接收通知；手机不可能常驻后台调度。 | 高 |
| 认证怎么做？ | 桌面生成一次性配对码 → 手机换取设备令牌；中继只存令牌哈希；桌面 Web Token **不离开桌面**。 | 高 |
| 是否需要 E2E？ | 已实现 X25519 + HKDF + ChaCha20-Poly1305：HTTP 请求/响应与事件帧端到端加密；中继只看到路由元数据。 | 高 |
| 推荐落地顺序 | P1 自建中继 → P2 原生 App 骨架（配对/服务器/推送）→ P3 原生 Chat → P4 原生 Room/任务 → P5 加固与上架。 | 高 |

**一句话**：中继是一个 **WSS 反向隧道 + 设备配对 + 推送网关**；手机说的是和浏览器完全相同的 `/api`，桌面仍然是唯一事实来源。

---

## 1. 目标与非目标

### 1.1 目标

| 目标 | 验收标准 |
|------|----------|
| 任意网络可达 | 手机在 Wi-Fi / 4G / 5G 下都能连上桌面；桌面无公网 IP、无入站端口 |
| Chat | 会话列表、新建、搜索、历史、流式回复、工具调用、计划、提问、ACP 权限审批、停止、附件 |
| Room | 群聊列表、创建/编辑/删除、成员与激活策略、群聊会话、发送、逐成员回复 |
| 自动任务 | 任务列表、创建/编辑/删除、启用/停用、立即运行、运行历史、完成通知 |
| 多端一致 | 手机与桌面读写同一份 `~/.chatgo`；手机上的操作桌面可见，反之亦然 |
| 断线可用 | 切网/锁屏/中继重启后自动重连；不重复发送、不丢当前回复 |
| 安全 | 配对码一次性、设备级令牌、可吊销、TLS、访问审计；默认不暴露桌面 |
| 可运维 | 中继单二进制 + SQLite 即可自托管；有健康检查、限流、日志 |

### 1.2 非目标（v1 明确不做）

- 手机端运行模型/Agent 本地推理。
- 手机端文件浏览器、Git diff、终端、全局设置编辑（这些是桌面能力，攻击面大）。
- 手机离线创建任务并延迟同步（桌面不在线时手机只能看缓存）。
- 多用户共享同一桌面（v1 是“我的手机连我的桌面”；团队共享放到后续）。
- 多租户/账号体系与计费（v1 是单用户自托管中继）。

---

## 2. 现状盘点

### 2.1 已经具备的能力

| 能力 | 位置 | 说明 |
|------|------|------|
| HTTP API + Token 认证 | `internal/webui/server.go:221`、`internal/webui/auth.go:21` | 所有 `/api/*`、静态页、`/mcp`、`/agui`、`/a2a` 都经过同一套 Bearer/Cookie 认证 |
| 桌面运行时 | `internal/ui/web_runtime.go:25` | Web 请求直接调用桌面 Agent 运行时，工具/MCP/ACP/Skills 与桌面一致 |
| 能力协商 | `internal/webui/runner.go:9` | `agents/tools/acp/ask/permission/checkpoints/attachments/skills/rooms/tasks` |
| 会话 | `internal/webui/server.go:248` | 列表、新建、详情、改名、删除、发消息、编辑、重生成、回滚、取消、附件、交互 |
| 群聊 | `internal/webui/rooms_server.go` | 列表、创建、更新、删除、会话、发送 |
| 自动任务 | `internal/webui/tasks_server.go` | 列表、创建、更新、删除、立即运行、运行历史 |
| 流式 | `cmd/web/main.go:171` | 浏览器每 700ms 轮询 `GET /api/sessions/{id}`，读取运行中的快照 |
| 桌面通知 | `internal/ui/notify.go:63`、`internal/ui/autotask.go:618` | 任务完成/需要审批时发系统通知；手机端可复用同一触发点 |
| 二维码 | `internal/ui/feishu.go:581` | 已依赖 `skip2/go-qrcode`，桌面设置页可直接画配对二维码 |
| Web 客户端 | `cmd/web/` | 完整 Chat/Room/Task 页面，可作为协议参考与组件来源 |

### 2.2 缺口

1. **没有公网入口**：`internal/ui/web_server.go:68` 只监听 `127.0.0.1:8422`，外部设备不可达。
2. **没有配对/设备概念**：只有一把全局 `web_token`；没有“这台手机”的独立令牌、吊销、最后活跃。
3. **没有中继**：桌面不能主动外连，手机不能穿透 NAT。
4. **没有手机 App**：`cmd/web` 是 wasm 浏览器客户端；桌面 `internal/ui` 不能直接跑在手机上。
5. **轮询不适合移动网络**：运行中 700ms 一次请求，锁屏/切网后恢复成本高；需要事件流或至少退避。
6. **没有推送**：自动任务完成、Agent 提问、权限审批在 App 关闭时无法通知。
7. **写并发没有跨端仲裁**：`webui` 的 `runs` 只覆盖 Web 发起的运行；桌面 UI 自己的 `cw.streams` 是另一套。手机加入后，两端同时发消息的冲突需要明确以 409 + 刷新处理。

### 2.3 关键判断

- **不要让手机直连 `webd`**：`cmd/webd` 使用 `webui.NewServer`（无 Runner），没有 Room/Task 能力（`internal/webui/rooms_server.go`、`tasks_server.go` 会返回“需要桌面版运行时”）。
- **中继必须连桌面 App 进程**：只有桌面 App 的 `WebRuntime()` 才有 Agent、ACP、Room、Task 调度器。
- **手机加入不改变存储模型**：仍然由 `webui` 串行化同一会话的运行；中继只是把 HTTP 请求送进来。

### 2.4 与 2026-10-31 手机方案的关系

`docs/superpowers/specs/2026-10-31-mobile-control-design.md` 当时的结论是“PWA + 现成隧道”，并把
“把 Agent 运行时从 UI 线程里抽出来”列为最大工程量。一个多月后，仓库已经落地了 `internal/webui`
和 `internal/ui/web_runtime.go`：Web 请求可以直接调用桌面 Agent 运行时，Chat、Room、自动任务都有
了完整 API。因此本方案不再等待 `agent.Runtime` 大重构，而是：

- 复用已经存在的 `/api`，中继只做传输；
- 把“手机端”从 PWA 升级为 MyGo 原生 App；
- 直接采用可配对、可推送、可自托管的中继，而不是现成隧道。

这缩短了第一版路径，但也保留了旧方案的风险提示：桌面必须运行、令牌等价远程控制、手机后台不可靠。

---

## 3. 方案选型

### 3.1 传输方式

| 方案 | 做法 | 优点 | 缺点 | 结论 |
|------|------|------|------|------|
| A. 自建中继（本方案） | 桌面 WSS 外连中继；手机 HTTPS/WSS 连中继 | 任意网络；扫码即用；可推送；可多设备；可自托管 | 要维护服务端；中继是信任点 | **唯一产品方案** |
| B. 现成隧道 | Tailscale / Cloudflare Tunnel / ngrok | 几乎零开发；TLS 现成 | 需要 VPN/云账号；无设备令牌/配对/推送 | 不采用 |
| C. 局域网直连 | 手机访问 `http://192.168.x.x:8422` | 零成本、低延迟 | 只能同一 Wi-Fi；需要开 `0.0.0.0` | 不采用；本地开发也用本地 `relayd` |
| D. 纯 P2P（WebRTC） | STUN/TURN 打洞 | 延迟最低、隐私最好 | NAT 成功率不稳定；TURN 仍要服务器 | 暂不做 |
| E. 消息队列中继 | 中继持久化事件，手机离线收 | 离线能力强 | 要复制会话状态、冲突合并；隐私/成本高 | 不采用 |

**只做自建中继，不保留 Cloudflare/Tailscale/直连兼容模式。** 手机 App 的 `BaseURL` 始终是
`https://relay.example.com/d/<desktop_id>`（本地开发为 `http://127.0.0.1:8787/d/<desktop_id>`，
Android 模拟器为 `http://10.0.2.2:8787/d/<desktop_id>`）。这样只需要维护一套认证、配对、吊销和推送逻辑。

### 3.2 手机客户端形态

| 方案 | 开发成本 | 体验 | 复用现有代码 | 结论 |
|------|----------|------|--------------|------|
| MyGo 原生 UI | 中高 | 高（原生列表、手势、键盘、推送、安全区） | 复用 API/类型/消息渲染，不复用桌面布局 | **唯一采用的手机客户端** |
| MyGo WebView 壳 | 低 | 中低（23 MB wasm，交互像网页） | 直接复用 `cmd/web` 全部页面 | **不采用**（用户决策） |
| PWA / 浏览器 | 最低 | 中 | 直接复用 `cmd/web` | **不作为手机客户端**；仅作为桌面调试入口保留 |
| Flutter / RN | 高 | 高 | 不能复用 Go 代码；两套语言 | 不采用 |

**选择 MyGo 原生 UI。** 理由：仓库已经是 Go + MyGo；v0.10 提供 Tab、导航栈、Sheet、SwipeRow、下拉刷新、键盘、安全区、Keychain/Keystore、推送、深链，足够做原生体验；同时避免再引入一套前端技术栈。

**节奏**：P2 直接搭建原生 App 骨架（四个 Tab、导航栈、配对、服务器列表、设置、推送注册、`/api/meta` 与列表读取），P3 完成原生 Chat，P4 完成原生 Room / 自动任务。中继在 P1 先用 `curl`/测试客户端验证，不等手机端。

> 明确约束：手机 App 不嵌入 WebView，不加载 `web/app.wasm`，不依赖 `/d/<id>/` 下的静态资源；中继只代理 `/api` 与 WebSocket。

### 3.3 中继拓扑

```text
┌──────────────────────┐        HTTPS / WSS         ┌────────────────────────┐
│ ChatGo Mobile        │  ───────────────────────►  │ ChatGo Relay           │
│ MyGo 0.10 原生 App   │   Bearer <device_token>    │  ├ 配对 / 设备注册      │
│ Chat / Room / Task   │  ◄───────────────────────  │  ├ WSS 隧道终止         │
└──────────────────────┘                            │  ├ HTTP 反向代理        │
                                                    │  ├ APNs / FCM 推送      │
┌──────────────────────┐        WSS（桌面主动外连）  │  └ SQLite（设备/桌面）  │
│ ChatGo Desktop       │  ◄───────────────────────  └────────────────────────┘
│ webui + WebRuntime   │   Bearer <desktop_secret>
│ Room/Task 调度器      │   yamux 多路复用
└──────────────────────┘
```

关键点：

- 桌面**只发起 outbound**，不监听公网端口。
- 中继永远不主动连桌面；桌面断线后自己重连。
- 一个桌面一条隧道，多个手机共享隧道（每条请求一个 yamux stream）。
- 手机看到的是 `https://relay.example.com/d/<desktop_id>/api/...`，和本地 `/api/...` 完全同构。

---

## 4. 总体架构

### 4.1 组件职责

| 组件 | 位置（建议） | 职责 | 不做什么 |
|------|--------------|------|----------|
| `cmd/relayd` | 新二进制 | 监听公网、TLS、隧道、配对、设备、推送、限流、审计 | 不解析会话业务，不存消息内容 |
| `internal/relay` | 新包 | 中继服务端实现：tunnel、proxy、pair、store、push | 不 import `internal/ui` |
| `internal/relayclient` | 新包 | 桌面连接器：外连、yamux listener、转发到本地 webui、控制请求 | 不直接改会话文件 |
| `internal/webui` | 现有 | 业务 API、运行时、Room/Task | 不知道中继存在 |
| `internal/ui` | 现有 | 桌面设置页：启用中继、二维码、设备列表；通知钩子 | 不参与隧道协议细节 |
| `internal/mobileapi` | 新包（从 `cmd/web` 抽） | 手机/桌面共享的 API 类型与客户端 | 不含 UI |
| `cmd/mobile` | 新二进制 | MyGo 手机 App：Tab、页面、状态、推送、深链 | 不跑 Agent |

### 4.2 一条请求的时序

```text
Phone                Relay                  Desktop connector           webui / Runtime
  │                    │                          │                          │
  │ GET /d/D/api/sessions                          │                          │
  │ Authorization: Bearer <device>                │                          │
  ├───────────────────►│                          │                          │
  │                    │ 校验 device→desktop       │                          │
  │                    │ 限流/大小/超时            │                          │
  │                    │ yamux.OpenStream()        │                          │
  │                    ├─────────────────────────►│                          │
  │                    │  HTTP/1.1 请求             │ 去掉客户端内部头          │
  │                    │                          │ 加本地 web token          │
  │                    │                          ├─────────────────────────►│
  │                    │                          │                          │ 运行/读取
  │                    │                          │◄─────────────────────────┤
  │                    │◄─────────────────────────┤ 响应                      │
  │◄───────────────────┤ 复制状态/头/体             │                          │
```

### 4.3 为什么用“反向代理”而不是“自定义 RPC”

- 现有 `/api` 已经完整覆盖 Chat/Room/Task；自定义 RPC 会把协议复制一遍，后续每加一个字段都要改两处。
- 反向代理天然支持附件上传、流式响应、WebSocket 升级；原生 App 只请求 `/api`，不需要静态资源代理。
- 手机端、浏览器、未来 CLI 都能复用同一套接口。
- 中继只按 `desktop_id` 路由，不解析 JSON，边界清晰。

---

## 5. 中继协议

### 5.1 桌面注册与隧道

桌面首次启用中继时生成：

```yaml
relay:
  enabled: true
  url: "wss://relay.example.com"
  desktop_id: "d_7f3a9c2e1b4d8a60"   # 16 字节随机，十六进制
  # desktop_secret 不放配置文件，放系统钥匙串（mygo.Secrets）
  allow_pairing: true
  push_enabled: true
```

`relay.enabled` 与 `web_enabled` 独立：中继连接器按需在回环地址启动一个内部 `webui.Server`（带
`WebRuntime`），不对外开放局域网端口；用户不需要为了手机去开启“Web 服务”。

连接：

```http
GET /v1/tunnel HTTP/1.1
Host: relay.example.com
Upgrade: websocket
Authorization: Bearer <desktop_secret>
X-ChatGo-Desktop-ID: d_7f3a9c2e1b4d8a60
X-ChatGo-Desktop-Name: TK-MacBook
X-ChatGo-Protocol: 1
```

- 中继首次见到 `desktop_id` 时记录 `sha256(secret)`（TOFU）；之后必须匹配。未来多租户版可再加一次性注册令牌。
- 升级 101 后，在 WebSocket 上跑 **yamux**：中继是 server，桌面是 client。WebSocket 是消息帧，yamux 需要 `net.Conn`，用一个 binary message 的读写适配器（单帧上限 64 KB，避免大帧阻塞）。
- 心跳 20s；桌面断线按 1s → 2s → 4s → … → 30s 重连；中继保留桌面记录 24h。
- 同一 `desktop_id` 新连接会替换旧连接（旧连接关闭），避免双写。

### 5.2 配对

```text
Desktop Settings             Relay                         Phone
   │ 生成 code (8 位, 120s)     │                             │
   │ 显示 QR / 链接             │                             │
   │                           │◄──── POST /v1/pair ─────────┤ 扫码/手输
   │                           │ {desktop_id, code,          │
   │                           │  device_name, platform}     │
   │                           │                             │
   │◄── POST /__relay/pair ────┤ 经隧道转发                   │
   │ 校验 code / 次数 / TTL     │                             │
   │ 生成 device_id + token     │                             │
   ├── {device_id, token, ...} ►│                             │
   │                           │ 存 sha256(token)            │
   │                           ├──── {device_id, token, ...} ►│
   │ 存设备元数据               │                             │ 存 Keychain/Keystore
```

细节：

- **配对码**：8 位 Crockford Base32（约 40 bit），TTL 120s，一次性；同一桌面同时只允许一个有效码；最多 5 次失败，超过作废；中继按 IP/桌面限流。
- **二维码内容**：优先使用 HTTPS App Link，避免额外扫码依赖：
  `https://relay.example.com/pair#v=1&d=<desktop_id>&c=<code>&n=<desktop_name>`
  放在 fragment 里，中继访问日志不会记录 code；手机 App 通过 `App.OnOpenURL` 接管。开发期可用 `chatgo://pair?...`。
- **设备令牌**：32 字节随机（base64url），只在配对响应中出现一次；手机存 Keychain/Keystore；桌面与中继只存 `sha256`。
- **桌面批准**：默认“配对码正确即批准”；可在设置里改为“需要桌面点确认”。v1 用前者，减少手机端等待。
- **能力协商**：配对响应带 `capabilities`（来自 `/api/meta`），手机据此隐藏 Room/Task 入口；桌面未启用 Web 时中继直接 503。

### 5.3 认证与路由

手机请求：

```http
GET /d/d_7f3a9c2e1b4d8a60/api/sessions HTTP/1.1
Host: relay.example.com
Authorization: Bearer <device_token>
```

中继：

1. 解析 `desktop_id`，查设备：`device.desktop_id == desktop_id` 且未吊销。
2. 常量时间比较 `sha256(token)`。
3. 限流：每设备 20 req/s，突发 40；每 IP 60 req/s；附件单独限流。
4. 限制 body：普通 1 MB，附件 12 MB；超时：普通 60s，附件 120s，WS 不设总超时。
5. 去掉客户端伪造的 `X-ChatGo-*`、`X-Forwarded-*`、`Authorization`；设置：
   - `X-ChatGo-Relay-Device: <device_id>`
   - `X-ChatGo-Relay-Desktop: <desktop_id>`
   - `X-Forwarded-Proto: https`
   - `X-Forwarded-For: <client ip>`
6. 打开 yamux stream，转发 `/{path}`（去掉 `/d/<id>` 前缀）。

实现上，中继用 `httputil.ReverseProxy` + 自定义 `http.Transport.DialContext`（每次请求 `yamux.OpenStream()`），
WebSocket 升级由 `ReverseProxy` 直接透传；桌面连接器则把 yamux 的 `Accept()` 包装成 `net.Listener`，
用 `http.Serve` 接收请求，再转发到本地 webui。

**只代理 `/api`**：原生 App 的 `BaseURL` 是 `https://relay.example.com/d/<desktop_id>`，所有请求由
`internal/mobileapi` 拼成 `/api/...`；中继只接受 `/d/<desktop_id>/api/...`（以及 P3 的 `/api/events`），
不代理静态页、`/login`、`/mcp`、`/agui`、`/a2a`。这样不需要 Cookie、一次性 code 或路径前缀适配，
也显著缩小攻击面。若未来要支持浏览器/PWA，再单独开放静态路由，不与手机 App 混在一起。

`internal/mobileapi` 的 `http.Client` 要设置 `CheckRedirect`：中继/桌面若返回 302 到 `/login`，手机端不跟随，
直接把 401/302 当作“设备令牌失效/需要重新配对”，避免把 HTML 登录页当成 JSON 解析。

桌面连接器：

1. 只接受来自隧道的请求；再次校验 `X-ChatGo-Relay-Desktop` 与自身 `desktop_id` 一致。
2. 删除所有 `X-ChatGo-Relay-*`，添加 `Authorization: Bearer <本地 web_token>`。
3. 转发到 `http://127.0.0.1:<webui port>`；如果 Web 服务未开，连接器自己启动一个仅回环的 `webui.Server`（带 `WebRuntime`），保证 Room/Task 可用。
4. **剥离响应里的 `Set-Cookie: chatgo_web_token`**，防止桌面 Web Token 意外出现在任何响应中；手机只持有设备令牌。
5. 控制路径 `/__relay/*` 不转发到 webui，由连接器自己处理（配对、吊销、推送、健康）。

### 5.4 设备管理与吊销

桌面设置页显示：

| 字段 | 来源 |
|------|------|
| 设备名 / 平台 | 配对时手机上报 |
| 首次配对 / 最后活跃 | 桌面本地 + 中继控制消息 |
| 推送状态 | 中继回执 |
| 操作 | 吊销、重命名 |

吊销流程：

1. 桌面 `relayclient.Revoke(deviceID)`。
2. 连接器在 yamux 上向中继发控制请求 `POST /__relay/revoke {device_id}`。
3. 中继删除设备记录、关闭该设备所有在途 stream、推送记录。
4. 手机下一次请求得到 `401 device_revoked`；App 清除本地令牌并提示重新配对。
5. 中继不可达时，桌面把吊销写入待办，重连后重试；期间该设备仍可用——设置页要明确提示“吊销将在中继恢复后生效”。

### 5.5 推送

手机注册：

```http
POST /v1/push
Authorization: Bearer <device_token>
{ "provider": "apns" | "fcm", "token": "...", "env": "prod" | "dev" }
```

桌面触发（在现有通知点旁边加钩子）：

| 触发 | 现有位置 | 推送 kind |
|------|----------|-----------|
| 自动任务完成 | `internal/ui/autotask.go:730`、`:808` 的 `notifySessionComplete` | `task_done` |
| Agent 提问 | `internal/ui/notify.go:99` 的 `notifyAttention` | `ask` |
| ACP 权限审批 | 同上 | `permission` |
| 群聊一轮完成 | `internal/ui/web_rooms.go:224` 的 `RunRoom` 结束 | `room_done` |

连接器通过 yamux 向中继发 `POST /__relay/notify`：

```json
{
  "desktop_id": "d_...",
  "kind": "task_done",
  "session_id": "20261118-...",
  "device_ids": ["dev_..."]
}
```

中继只发**元数据**：

```json
{
  "aps": {
    "alert": { "title": "ChatGo", "body": "自动任务已完成" },
    "sound": "default",
    "thread-id": "task:20261118-...",
    "mutable-content": 1
  },
  "v": 1,
  "kind": "task_done",
  "desktop_id": "d_...",
  "session_id": "20261118-..."
}
```

- 不把提示词、回复正文、工作区路径放进推送；手机点开后用 API 拉详情。
- iOS 需要 APNs key（`.p8` + Key ID + Team ID）与 Push Notifications capability；Android 需要 FCM 服务账号。
- 前台时不用推送，直接 App 内横幅 + 触感；锁屏/后台才发。
- 没有配置推送时，App 仍可在前台轮询；设置页明确“仅前台通知”。

### 5.6 断线、重试、幂等

| 场景 | 行为 |
|------|------|
| 桌面断线 | 中继对手机返回 `503 desktop_offline` + `Retry-After: 3`；手机显示“桌面离线”，指数退避重连 |
| 手机切网 | HTTP 请求失败 → 重连；正在轮询的会话重新拉快照 |
| 中继重启 | 设备/桌面从 SQLite 恢复；桌面重连；手机最多一个退避周期后恢复 |
| 发送消息超时 | **不自动重发**（可能已写入）；提示“可能已发送，正在刷新”；P3 加 `Idempotency-Key` 后自动重试 |
| 同一会话并发 | 服务端返回 `409`；手机停止发送并刷新，显示“另一端正在回复” |
| 中继限流 | `429` + `Retry-After`；手机退避 |
| 慢客户端 | 中继限制每设备并发 stream；超时断开；不阻塞桌面 Agent |

### 5.7 事件流（P3，可选但强烈建议）

现有轮询可以工作，但移动网络下耗电、耗流量，且无法同时订阅多个会话。建议在 `internal/webui` 增加：

```http
GET /api/events?since=<seq>&session=<id>
Upgrade: websocket
Authorization: Bearer <token>
```

事件信封：

```json
{
  "v": 1,
  "seq": 42,
  "type": "message.delta",
  "session_id": "20261118-...",
  "turn_id": "t_...",
  "at": "2026-11-18T10:00:00Z",
  "data": { "text": "..." }
}
```

- 每桌面一个事件 Hub，环形缓冲 4096 条；`since` 在窗口内补发，太旧则发 `snapshot` 事件。
- 事件类型沿用现有能力：`turn.started`、`message.delta`、`reasoning.delta`、`tool.started/finished`、`plan.updated`、`question.asked`、`permission.asked`、`turn.finished/failed`、`session.updated/deleted`、`room.*`、`task.*`。
- 中继只透传 WS，不解析事件；手机断线重连带 `since`。
- 在事件流就绪前，手机用 1s（运行中）/ 5s（空闲）轮询，并遵守 `Network` 计费网络提示。

---

### 5.8 中继端点总表

| 方法 | 路径 | 认证 | 用途 |
|------|------|------|------|
| `GET` | `/v1/tunnel` | `Authorization: Bearer <desktop_secret>` + `X-ChatGo-Desktop-ID` | 桌面 WSS 隧道（升级后 yamux） |
| `POST` | `/v1/pair` | 配对码 | 手机换取设备令牌 |
| `POST` | `/v1/push` | 设备令牌 | 注册 APNs/FCM token |
| `GET/POST/...` | `/d/{desktop_id}/api/{path...}` | `Authorization: Bearer <device_token>` | 反向代理到桌面 `/api` |
| `GET` | `/healthz` | 无 | 中继存活/版本 |
| `POST` | `/__relay/{pair,revoke,notify,devices}` | 仅隧道内 | 桌面连接器发起的控制请求（中继在 yamux 上接受） |

所有响应都带 `Cache-Control: no-store` 与 `Referrer-Policy: no-referrer`；错误统一为
`{"error":{"code":"...","message":"..."}}`，便于手机端区分 `desktop_offline` / `device_revoked` /
`rate_limited` / `payload_too_large`。

---

## 6. 手机端设计

### 6.1 信息架构

```text
ChatGo
├── 会话 (Chat)
│   ├── 会话列表（按工作区分组、搜索、下拉刷新）
│   └── 会话详情（消息流、工具卡片、计划、提问/权限、输入框）
├── 群聊 (Rooms)
│   ├── 群列表（成员数、运行状态）
│   ├── 群会话列表
│   └── 群聊详情（成员、激活策略、逐成员回复）
├── 任务 (Tasks)
│   ├── 任务列表（计划、上次运行、开关）
│   ├── 任务详情（提示词、Agent、工作区、运行历史）
│   └── 新建/编辑任务
└── 设置
    ├── 服务器（配对、切换、重连、吊销）
    ├── 通知（推送权限、前台/后台）
    ├── 外观（跟随系统 / 浅色 / 深色）
    └── 关于 / 版本 / 退出登录
```

手机端不做“桌面镜像”：没有文件树、Git、终端、全局模型配置；模型/Agent 选择只作用于当前会话。

### 6.2 页面与组件

| 页面 | MyGo 组件 | 关键交互 |
|------|-----------|----------|
| 会话列表 | `ui.Scroll` + `mobile.ListRow` + `Section` + `PullToRefresh` | 点击进入；左滑重命名/删除（`mobile.SwipeRow`）；下拉刷新 |
| 会话详情 | `ui.Router` + `SystemStack`；消息列表 + composer | 发送/停止；滚动到底部；工具折叠；计划清单；提问/权限 Sheet |
| 新建会话 | `mobile.Sheet` / `ActionSheet` | Agent、项目/工作区、模型、首条消息 |
| 群列表 | `ListRow` + `Section` | 创建/编辑/删除；运行中标记 |
| 群聊详情 | 消息列表 + 成员栏 | 成员头像/名称、激活策略、发送、逐成员回复 |
| 任务列表 | `ListRow` + `Switch` | 启用/停用、立即运行、查看历史 |
| 任务详情 | `TopBar` + 表单 | 名称、Agent、提示词、工作区、计划（`TimePicker`/`Wheel`）、复用会话 |
| 设置/配对 | `Section` + 表单 + 二维码 | 扫码/粘贴链接/手输配对码；服务器切换；推送开关 |
| 审批/提问 | `mobile.Sheet` | 选项、文本输入、允许/拒绝；显示工具名与参数 |

### 6.3 数据层

参考 `cmd/web/api.go`、`cmd/web/features.go`、`cmd/web/rooms.go`、`cmd/web/tasks.go` 的请求/类型实现，新建：

```text
internal/mobileapi/
  client.go      // BaseURL + Token + HTTP Client + Do/Get/Post/Patch/Delete
  types.go       // meta/session/agent/tool/model/room/task/interaction
  sessions.go    // 会话 CRUD、消息、取消、交互、附件
  rooms.go       // 群聊 CRUD、会话、发送
  tasks.go       // 任务 CRUD、立即运行、历史
  stream.go      // P3：/api/events WS
```

传输接口：

```go
type Transport interface {
    Do(req *http.Request) (*http.Response, error)
}

// dev (iOS 模拟器):  BaseURL = http://127.0.0.1:8787/d/<desktop_id>
// dev (Android 模拟器): BaseURL = http://10.0.2.2:8787/d/<desktop_id>
// production:       BaseURL = https://relay.example.com/d/<desktop_id>
// 所有模式的 Token 都是设备令牌；不再有直连/Tailscale/Cloudflare 分支。
```

这样同一套页面只面向自建中继；本地开发时在开发机跑 `relayd`，模拟器连本地中继即可。

消息渲染可以借鉴 `cmd/web/message.go`（Markdown、工具卡片、计划），但它依赖桌面密度/主题假设；手机端应抽成 `internal/mobileui/components` 后重新排版，不能直接复用整页。

### 6.4 状态与生命周期

- 所有 UI 更新走 `win.Update(func(){...})`；网络 goroutine 不直接改 UI 状态。
- 每个页面持有 `context.CancelFunc`；离开页面/进入后台取消轮询。
- `Power.OnSuspend` 停止轮询与 WS；`OnResume` 重新拉取当前会话与列表。
- `mygo.Network`：离线时显示横幅，停止请求；恢复后自动重试。
- App 启动顺序：读服务器列表 → 选上次服务器 → 验证 `/api/meta` → 拉列表；失败进入“连接”页，不阻塞 UI。
- `mygo.KeepState`：开发热重载/系统回收后恢复当前 Tab、打开的会话、草稿。

### 6.5 附件、相机、扫码

- 图片/文件：`imagepicker`（相册）/ `camera`（拍照）→ 读取字节 → base64 → `POST /api/sessions/{id}/attachments` → 发送消息时带 `attachments`。
- 现有服务端限制 8 MB/文件（`internal/webui/attachments.go`），手机端先压缩图片（长边 2048、JPEG 85%）。
- 扫码：v1 优先用系统相机扫 HTTPS App Link（`ios.associatedDomains` / `android.appLinks`），App 通过 `OnOpenURL` 接收配对参数；备用“手动输入 8 位码 + 中继地址”。如果必须在 App 内扫码，再加 `camera` + 纯 Go QR 解码器（如 `gozxing`），不阻塞 v1。

### 6.6 推送与深链

```go
push.Register(ctx)            // 拿 APNs/FCM token
push.OnOpened(func(data []byte) { /* 打开会话/任务 */ })
push.OnMessage(func(data []byte) { /* 前台刷新 */ })
mygo.App.OnOpenURL(func(raw string) { /* chatgo://session/... */ })
```

- 推送 payload 只带 `kind`、`desktop_id`、`session_id`；打开后再拉详情。
- 深链格式：
  - `chatgo://session/<session_id>?server=<desktop_id>`
  - `chatgo://room/<room_id>`
  - `chatgo://task/<task_id>`
  - `chatgo://pair?...`（开发期）
- 配对深链直接开始配对，不再弹配对面板：扫码/点链接即连，进度与失败原因显示在空状态页；只有手动输入才打开配对面板。
- 通知权限在“设置 → 通知”开启时申请；被拒绝时给系统设置跳转（`permissionflow`）。

### 6.7 本地存储

| 内容 | 存储 |
|------|------|
| 服务器列表、最后打开项、草稿 | `preferences` 插件 |
| 设备令牌、E2E 私钥 | `mygo.Secrets.Service("chatgo.mobile")` |
| 推送 token | `preferences` + 上报中继 |
| 会话缓存（可选） | `sqlite` 插件；v1 不做，避免缓存一致性问题 |
| 日志 | 仅错误摘要，不含消息正文/令牌 |

---

## 7. Chat / Room / 自动任务

### 7.1 API 映射

| 手机功能 | 现有接口 | 备注 |
|----------|----------|------|
| 元信息/能力 | `GET /api/meta` | `ready`、`models`、`capabilities` |
| 会话列表 | `GET /api/sessions` | 返回普通会话；Room/Task 会话由各自接口返回 |
| 新建会话 | `POST /api/sessions` | `{title, provider, model, agent, workspace, tools, acp_config}` |
| 会话详情 | `GET /api/sessions/{id}` | 运行中返回内存快照 + `running` + `interaction` |
| 发消息 | `POST /api/sessions/{id}/messages` | 202；返回 `reply_message_id`，随后轮询 |
| 停止 | `POST /api/sessions/{id}/cancel` | 取消当前 run |
| 回答/权限 | `POST /api/sessions/{id}/interaction` | `{id, answer}` 或 `{id, option_id, canceled}` |
| 编辑/重生成/回滚 | `POST /api/sessions/{id}/messages/{mid}/{edit|regenerate|revert}` | 手机可先只做“重生成/停止” |
| 附件 | `POST /api/sessions/{id}/attachments` | base64，8 MB |
| Agent/模型/工具/Skills | `/api/agents`、`/api/models`、`/api/tools`、`/api/skills` | 新建会话/切换时用 |
| 工作区/项目 | `/api/workspaces`、`/api/projects` | 手机不浏览任意路径 |
| 群聊列表/CRUD | `/api/rooms` | `POST`/`PATCH`/`DELETE` |
| 群聊会话 | `/api/rooms/{id}/sessions` | `GET`/`POST` |
| 群聊发消息 | `/api/rooms/{id}/sessions/{conv}/messages` | 202；轮询会话 |
| 任务列表/CRUD | `/api/tasks` | `POST`/`PATCH`/`DELETE` |
| 任务立即运行 | `POST /api/tasks/{id}/run` | 202；返回 `session_id` |
| 任务历史 | `GET /api/tasks/{id}/sessions` | 复用 `RoomSessionInfo` |

### 7.2 Chat 时序

```text
Phone                          Relay/Desktop                    Runtime
  │ POST /messages                │                               │
  ├──────────────────────────────►│ 追加 user + 占位 assistant     │
  │◄──── 202 {reply_message_id} ──┤ 启动 run                       │
  │ GET /sessions/{id} (1s)       │                               │
  ├──────────────────────────────►│ 返回运行快照                   │
  │◄──── messages/tool_calls ─────┤                               │
  │ ...                           │                               │
  │ GET /sessions/{id}            │                               │
  │◄──── running:false ───────────┤ 保存完成                       │
```

- 发送后立刻把用户消息和占位回复放进本地状态，避免等待一个轮询周期。
- 运行中只渲染变化的最后一条回复；工具卡片按 `ToolCalls` 增量更新。
- 滚动位置：用户不在底部时显示“回到最新”，不强制跟随。
- 停止：调用 cancel 后本地标记“正在停止”，收到 `running:false` 再恢复输入。

### 7.3 Room 时序

- `GET /api/rooms` 返回 `running`；进入群聊后 `GET /api/rooms/{id}/sessions`，再打开某个会话。
- 发送后房间进入 `running`；成员按 `RunRoom` 顺序逐个追加回复（`internal/ui/web_rooms.go:224`）。
- 手机端显示“谁在回复”（成员名 + 头像），不要把所有成员当成一个流。
- 群聊没有单独的“停止”接口；停止当前会话的 run 即可（`POST /api/sessions/{conv}/cancel`），并刷新房间状态。
- 群聊创建/编辑表单：名称、工作区、激活策略、Master、成员（Agent、名称、静音、话痨度）。手机 v1 可以只暴露常用字段，高级字段折叠。

### 7.4 自动任务时序

```text
Desktop scheduler (30s tick)        Phone
  │ 到点，运行任务                    │
  │ 追加消息 / 调 Agent               │
  │ 完成 → notifySessionComplete      │
  │ relay.Notify(task_done) ─────────►│ APNs/FCM
  │                                   │ 点开 → GET /api/tasks/{id}/sessions
  │                                   │ → GET /api/sessions/{conv}
```

- 手机端只展示“计划配置 / 上次运行 / 结果”，不在本地计算或触发调度；如需“下次运行时间”，由桌面端计算后随 `TaskInfo` 返回。
- “立即运行”调用 `/api/tasks/{id}/run`；桌面端用同一套 `TaskRunner`，不会与定时器重复（`StartTask` 有占用标记）。
- 手机可以启用/停用、修改计划；修改在桌面下一次 tick 生效。
- 任务完成通知在桌面 App 运行时才有；桌面退出时任务本来也不会跑，手机端要明确提示“桌面离线，任务未运行”。

### 7.5 并发与一致性

| 规则 | 行为 |
|------|------|
| 同一会话同时只有一个 run | 服务端 409；手机刷新并提示 |
| 手机与桌面同时发 | 先到先得；另一端收到 409 后刷新 |
| 删除运行中的会话 | 先 cancel，再删；手机确认对话框 |
| 任务运行中修改/删除 | 409；手机禁用按钮 |
| 群聊运行中再次发送 | 409；手机显示“群聊正在生成” |
| 设备吊销 | 立即 401；App 清理令牌，保留服务器条目为“需重新配对” |
| 中继离线 | 手机只读缓存列表；发送按钮禁用并解释原因 |

> 现有 `webui` 的 run registry 只覆盖 Web 发起的运行；桌面 UI 自己的运行是另一套。手机加入后，**所有手机请求都走 `webui`**，所以手机之间是一致的；手机 vs 桌面仍可能出现 409/覆盖窗口，P3 可把桌面 UI 也接入同一 run registry，彻底消除。

---

## 8. 安全与隐私

### 8.1 认证与密钥

| 密钥 | 生成 | 存储 | 用途 |
|------|------|------|------|
| `desktop_id` | 桌面首次启用 | 配置文件 | 路由标识，非秘密 |
| `desktop_secret` | 桌面首次启用 | 系统钥匙串（`mygo.Secrets`） | 桌面连中继 |
| 配对码 | 桌面按需生成 | 内存 | 一次性配对 |
| `device_token` | 桌面配对时生成 | 手机 Keychain/Keystore；桌面/中继存哈希 | 手机访问中继 |
| 桌面 `web_token` | 现有机制 | 桌面配置 | **只在本机连接器内使用，不下发手机/中继** |
| E2E 密钥 | 配对时 X25519 + HKDF | 手机 Keychain；桌面配置/钥匙串 | 消息体与事件加密 |

### 8.2 威胁模型

> 前提：拿到设备令牌 ≈ 拿到桌面 Agent 的聊天/任务能力（可能触发工具、读写工作区）。必须按“远程控制软件”对待，而不是普通聊天 App。

| 威胁 | 影响 | 缓解 |
|------|------|------|
| 中继被入侵 | 可读/改所有流量 | 自托管优先；TLS 1.3；E2E；中继不存消息；最小化日志 |
| 设备令牌泄露 | 攻击者可操作桌面 | 设备级令牌、可吊销、最后活跃审计；高风险操作二次确认；可选生物识别解锁 |
| 配对码被暴力破解 | 非法配对 | 40 bit + 120s + 一次性 + 5 次失败 + IP/桌面限流 |
| 中继被当作开放代理 | 扫描内网/SSRF | 只路由已注册 `desktop_id`；不提供任意 URL；禁止 CONNECT |
| 请求头注入 | 伪造设备/桌面 | 中继剥离客户端 `X-ChatGo-*`/`Authorization`；连接器只信任隧道元数据 |
| Web Token 泄漏 | 直接控制桌面 Web | 连接器本地注入；响应剥离 `Set-Cookie`；手机永远拿不到 |
| 中间人 | 窃听/篡改 | TLS；证书校验；E2E（AEAD）；时间戳绑定 AAD |
| 恶意网页 CSRF | 借用浏览器 | 手机 App 纯 Bearer、无 Cookie；中继校验 Origin；不开放浏览器静态入口 |
| 推送内容泄露 | 锁屏可见敏感信息 | 推送只带通用文案 + ID；详情点开再拉 |
| 手机丢失 | 令牌被用 | 远程吊销；生物识别；短会话令牌（可选） |
| 日志泄露 | 消息正文入库 | 访问日志只记设备、路径、状态、字节数；不记 body/token |
| 重放 | 重复执行操作 | 已实现：时间戳绑定 AAD + 桌面端 nonce 去重窗口（见 8.3） |

### 8.3 端到端加密（已实现）

- 配对时手机生成 X25519 密钥对，并把公钥放进 `device_pubkey`；桌面生成临时 X25519 密钥对，计算共享密钥后返回 `desktop_pubkey`。双方用 HKDF-SHA256 派生 32 字节会话密钥。
- HTTP 请求体用 ChaCha20-Poly1305 加密，AAD 绑定 `http + method + path + device_id + X-E2E-Time`；响应体用固定 response AAD 加密。中继只看到 `X-E2E-Version`、`X-E2E-Nonce`、`X-E2E-Time` 与密文。
- `/api/events` WebSocket 的每个事件帧用二进制 envelope（版本 + nonce + 密文）加密；手机解密后得到原有 JSON 事件。
- 每台设备一把密钥，保存在手机 Keychain/Keystore 与桌面配置；吊销设备后密钥不再使用。
- **防重放（已实现）**：`X-E2E-Time` 是手机时钟的毫秒时间戳，既参与 AAD，也在桌面端被校验。桌面拒绝 ±2 分钟之外的请求（`e2e_clock_skew`），并为每台设备缓存最近用过的 nonce（保留 2 倍窗口时间），同一 nonce 第二次到达直接被拒。复制一段密文后换个时间戳重发也无法解密，因为 AAD 不匹配。
- 限制：仍然没有 Ed25519 请求签名（身份依赖设备令牌 + AEAD）；E2E 只对原生 App 生效，不兼容旧版客户端；时间窗口依赖手机时钟，偏差过大时手机侧会把错误显示为“请校准手机时间”。

### 8.3.1 令牌失效与重新配对

设备被吊销、桌面重装或令牌被清空时，中继/桌面返回 `401`。手机端不再无限重试：

- `mobileapi.Client.OnAuthError` 在收到 401 时通知 App；
- App 删除本机保存的令牌与 E2E 密钥（`deleteToken` / `deleteE2EKey`），断开事件流并标记 `pairRequired`；
- 会话/群聊/任务页统一显示“需要重新配对”空状态，主按钮直接打开配对面板并预填中继地址与桌面 ID；设置页同步出现“重新配对”行；
- 重新配对成功后标记清除，连接恢复。

### 8.4 手机权限边界

| 能力 | v1 | 说明 |
|------|----|------|
| Chat 发送/停止/审批 | ✅ | 基本能力 |
| Room 创建/编辑/删除/发送 | ✅ | 删除需二次确认 |
| Task 创建/编辑/删除/运行/开关 | ✅ | 删除需二次确认 |
| 查看工具调用与参数 | ✅ | 只读展示 |
| 批准危险工具 | ✅（需确认） | 显示工具名、参数摘要、工作区；可配置“只读模式”直接拒绝 |
| 文件浏览/读写 | ❌ | 攻击面大，v1 不做 |
| Git/终端 | ❌ | 桌面专属 |
| 修改全局模型/MCP/渠道配置 | ❌ | 手机只选当前会话的模型/Agent |
| 查看其他设备令牌 | ❌ | 只显示设备名/时间 |

---

## 9. 代码改动清单

### 9.1 新增

```text
cmd/relayd/                 # 中继服务端入口
cmd/mobile/                 # MyGo 手机 App 入口
internal/relay/
  server.go                 # HTTP/WSS 服务、路由、生命周期
  tunnel.go                 # yamux、WebSocket net.Conn、反向代理 Transport
  pair.go                   # 配对码、设备令牌、审批
  store.go                  # SQLite：desktops/devices/push_tokens
  push.go                   # APNs/FCM 适配器（可插拔）
  limits.go                 # 限流、body/timeout、审计
  protocol.go               # 头、错误码、版本
internal/relayclient/
  connector.go              # 桌面外连、重连、yamux listener
  control.go                # /__relay/pair、revoke、notify
  settings.go               # 本地设置页用的状态/二维码/设备列表
internal/mobileapi/
  client.go types.go sessions.go rooms.go tasks.go stream.go
internal/mobileui/
  app.go tabs.go chat_list.go chat.go room_list.go room.go
  task_list.go task.go settings.go pair.go components/
```

### 9.2 修改

| 文件 | 改动 |
|------|------|
| `go.mod` | MyGo `v0.9.0` → `v0.10.0`；新增 `github.com/hashicorp/yamux`（或同类） |
| `mygo.json` | 新增 `ios` / `android` 段：identifier、team、最低版本、相机/相册/通知权限、Associated Domains / App Links、`urlSchemes` |
| `internal/config/config.go` | 新增 `RelayConfig`：enabled/url/desktop_id/allow_pairing/push_enabled/devices |
| `internal/ui/web_server.go` | 提供 `Server()` 访问器，供连接器取本地地址/令牌；未启用 Web 时按需启动回环服务 |
| `internal/ui/app.go` | 启动/停止 `relayclient`；生命周期与 Web 服务一致 |
| `internal/ui/settings_web.go` 或新 `settings_mobile.go` | 设置页：启用、地址、状态、二维码、配对码、设备列表、吊销、推送测试 |
| `internal/ui/notify.go` | 在 `notifySessionComplete`/`notifyAttention` 旁增加独立的 `relayNotifier` 调用点（不要复用 `notifyUser` 的 focused 短路；手机前台由 App 自己抑制） |
| `internal/ui/autotask.go` | 任务完成/失败时调用 `relay.NotifyTask`；不改变调度逻辑 |
| `internal/ui/web_rooms.go` | 群聊一轮完成时调用 `relay.NotifyRoom` |
| `internal/webui/server.go` | P3 新增 `GET /api/events`（WS）；其余 API 不变 |
| `internal/webui/attachments.go` | 可选：手机图片压缩后的 MIME/大小校验（现有 8 MB 限制） |
| `cmd/web/` | v1 不改行为；后续可选用 `internal/mobileapi` 的类型，减少重复定义 |
| `README.md` / `README.zh-CN.md` | 新功能与安全说明 |
| `docs/` | 本方案与调研文档 |

手机端 `mygo.json` 的关键片段（示意，实际值按团队/域名替换）：

```json
{
  "identifier": "com.tk103331.chatgo.mobile",
  "ios": {
    "team": "<APPLE_TEAM_ID>",
    "minimumSystemVersion": "17.0",
    "associatedDomains": ["relay.example.com"],
    "backgroundModes": ["remote-notification"],
    "infoPlist": {
      "NSCameraUsageDescription": "拍摄聊天图片或扫描配对二维码",
      "NSPhotoLibraryUsageDescription": "选择图片作为聊天附件"
    }
  },
  "android": {
    "minSdk": 26,
    "permissions": ["CAMERA", "POST_NOTIFICATIONS", "READ_MEDIA_IMAGES"],
    "appLinks": ["relay.example.com"]
  }
}
```

> 桌面 App 仍使用现有 `com.tk103331.chatgo`；手机是独立应用标识，避免与桌面安装包冲突。`urlSchemes` 继续使用 `chatgo`，手机端同时支持 `chatgo://` 与 HTTPS App Link。

### 9.3 关键接口草图

```go
// internal/relayclient
type Connector struct {
    DesktopID string
    RelayURL  string
    Secret    string
    Local     *webui.Server // 或 localURL + token
}

func (c *Connector) Run(ctx context.Context) error // 外连、重连、yamux listener
func (c *Connector) PairCode() (code string, expires time.Time, err error)
func (c *Connector) Devices() []DeviceInfo
func (c *Connector) Revoke(deviceID string) error
func (c *Connector) Notify(n Notification) error
```

```go
// internal/relay
func (s *Server) Handler() http.Handler
func (s *Server) handleTunnel(w http.ResponseWriter, r *http.Request)
func (s *Server) proxyFor(desktopID string) (http.Handler, bool)
```

```go
// internal/mobileapi
type Client struct {
    BaseURL string
    Token   string
    HTTP    *http.Client
}

func (c *Client) Sessions(ctx context.Context) ([]SessionSummary, error)
func (c *Client) Send(ctx context.Context, sessionID, text string, atts []Attachment) (*SendResult, error)
func (c *Client) Rooms(ctx context.Context) ([]Room, error)
func (c *Client) RunTask(ctx context.Context, taskID string) (*RunResult, error)
```

---

## 10. 分阶段计划

| 阶段 | 交付 | 验收 | 预估（1 人） |
|------|------|------|--------------|
| **P1 中继骨架** | `relayd` + 桌面连接器 + 配对 + 设备令牌 + `/api` 反向代理；桌面设置页 | 用 `curl`/测试客户端带设备令牌通过中继调用 `/api/meta`、`/api/sessions`、`/api/rooms`、`/api/tasks`；配对/吊销/401/离线 503 正常 | 6-9 天 |
| **P2 原生骨架** | MyGo 0.10 升级；`cmd/mobile` 四 Tab/导航栈；`internal/mobileapi`；配对、服务器列表、设置、推送注册、`/api/meta` 与列表读取 | 真机扫码配对；会话/群聊/任务列表能显示；推送 token 已上报；无 WebView | 5-8 天 |
| **P3 原生 Chat** | 原生会话列表/聊天/工具/提问/权限/停止/附件；`/api/events` | 原生 Chat 全流程；断线重连不重复、不丢；滚动与键盘体验达标 | 10-15 天 |
| **P4 原生 Room/Task + 推送** | 群聊与任务页面；完成/提问/权限推送；深链；任务编辑 | 手机创建群聊/任务并运行；桌面离线提示准确；推送可跳转 | 7-10 天 |
| **P5 加固与上架** | E2E 签名/防重放、多设备、审计、限流、生物识别、TestFlight/内部测试、商店素材 | 安全测试通过；iOS/Android 真机稳定；审核提交 | 8-12 天 |

**可用版（P1–P4）约 28-42 人日**，能覆盖 Chat / Room / 自动任务三个核心场景；**完整版约 36-54 人日**。建议先做 P1，不要一开始就投入 P3 的大 UI。

---

## 11. 测试方案

| 层级 | 内容 |
|------|------|
| 单元 | 配对码 TTL/次数、令牌哈希/吊销、header 清洗、路由归属、限流、body 上限、错误码 |
| 隧道 | `httptest` + 内存 yamux：GET/POST/大 body/流式响应/并发 stream/桌面断线重连 |
| 集成 | 真 `webui.Server` + fake Runner：Chat 发送/轮询、Room 发送、Task run；附件上传；401/409/503 |
| 竞态 | `go test -race ./internal/relay/... ./internal/relayclient/... ./internal/mobileapi/...` |
| 安全 | 无令牌、过期/吊销令牌、错误 desktop、伪造 `X-ChatGo-*`、超大 body、暴力配对、重放 |
| 手机 | iOS 模拟器 + 真机（TestFlight）；Android 模拟器 + 真机（internal track）；Wi-Fi ↔ 4G 切换；锁屏 8 小时；中继重启；桌面退出 |
| 性能 | 首屏时间、消息延迟、内存、耗电、弱网（3G/高丢包）、长会话（1000+ 消息） |
| 回归 | 桌面构建、`cmd/web` wasm 构建、现有 Web UI、Room/Task 调度、IM 渠道行为不变 |

真机矩阵建议：iPhone（Face ID）、iPad、Pixel（Android 14+）、一台低端 Android；至少覆盖一个刘海屏和一个无刘海屏。

---

## 12. 风险与取舍

| 风险 | 影响 | 缓解 |
|------|------|------|
| 中继成为单点/被攻击 | 全部手机不可用或数据泄露 | 自托管优先；限流；TLS；审计；P5 E2E；多实例后续再做 |
| 桌面必须运行 | 用户以为“云同步”，实际桌面关机就不可用 | 设置页/手机端明确状态；支持托盘常驻/开机自启（现有 `RunInBackground`） |
| 手机 vs 桌面并发写 | 冲突/覆盖 | 统一走 webui run registry；409 + 刷新；P3 把桌面接入同一 registry |
| 轮询耗电 | 移动网络体验差 | P3 `/api/events`；前台 1s/空闲 5s；后台停 |
| MyGo 0.10 较新 | 手机端 API 变化/回归 | 升级前跑全量桌面回归；锁定版本；真机矩阵 |
| iOS 后台限制 | 推送不及时/静默推送被限流 | 重要事件用可见通知；App 前台实时；后台刷新只做补偿 |
| App Store 审核 | 远程控制类应用被质疑 | 清晰隐私说明、权限用途、加密合规；先 TestFlight |
| 配对码泄露 | 未授权设备接入 | 短 TTL、一次性、限次、桌面可见、可吊销 |
| 中继日志泄露 | 元数据/路径泄露 | 不记 body/token；code 放 fragment；日志脱敏 |
| 附件过大 | 上传失败/内存 | 8 MB 限制；手机端压缩；分片上传后续再说 |
| 长会话渲染 | 手机卡顿 | 限制首屏消息数/分页；P3 加 `before` 参数 |

---

## 13. 待确认问题

1. **中继部署在哪？** 推荐公网 VPS + 域名 + TLS，单二进制 + SQLite 起步；后续再考虑多实例。
2. **E2E 签名与防重放是否现在做？** 当前已做 AEAD 加密；Ed25519 签名/时间窗建议放 P5。
3. **推送通道？** 推荐 APNs + FCM；自托管可先支持 ntfy/Bark webhook 作为降级。
4. **iOS 开发者账号/Team ID 是否就绪？** 当前机器只有模拟器签名，没有真机 Profile。
5. **最低系统版本？** MyGo 默认 iOS 17 / Android 8（minSdk 26）；是否要降到 iOS 16？
6. **手机权限边界？** 是否允许删除群聊/任务、批准危险工具？推荐允许 + 二次确认 + 只读模式。
7. **多桌面？** 推荐手机支持多个服务器条目，但同一时间只连一个。
8. **桌面常驻？** 是否要求合盖/退出窗口后继续运行（托盘/后台）？这决定自动任务与推送的可用性。
9. **发布渠道？** TestFlight/Play 内部测试先跑，还是直接商店？

---

## 14. 实施记录（P1–P4）

已落地：

```text
cmd/relayd/                 # 中继服务
cmd/mobile/                 # MyGo 原生手机 App
internal/relay/             # tunnel / pair / store / proxy / APNs / FCM
internal/relayclient/       # 桌面外连连接器
internal/e2e/               # X25519 + HKDF + ChaCha20-Poly1305
internal/mobileapi/         # 手机 API 客户端
internal/ui/relay_service.go
internal/ui/settings_mobile.go
docs/RELAY.md               # 部署与构建说明
```

与方案的主要差异：

- `/api/events` WebSocket 已实现，手机端用它触发刷新；1s 轮询保留为断线/旧桌面回退。
- 中继已支持 APNs / FCM 推送，但需要部署方提供证书/服务账号；未配置时通知写入中继日志。
- 配对支持深链（`chatgo://pair?...`）与手动输入；深链直接配对并显示进度，手动输入走配对面板；App 内扫码尚未实现。
- E2E 已实现 HTTP 请求/响应与事件帧加密，并在 P5 补上时间戳绑定 AAD + nonce 去重窗口（防重放）；Ed25519 请求签名与多设备审计仍未做。
- 桌面 `desktop_secret` 与每设备 E2E key 暂存于 `~/.chatgo/config.yaml`，后续迁移到系统钥匙串。

验证：

```sh
go build ./...
go test ./internal/relay/... ./internal/relayclient/... ./internal/mobileapi/... ./cmd/mobile/...
mygo build -platform ios/simulator ./cmd/mobile
```

---

## 15. 参考

- 本仓库现有 Web API：`internal/webui/server.go:221`
- 现有认证：`internal/webui/auth.go:21`
- 桌面运行时：`internal/ui/web_runtime.go:25`
- 群聊运行时：`internal/ui/web_rooms.go:224`
- 自动任务调度：`internal/ui/autotask.go:617`
- 现有 wasm 客户端轮询：`cmd/web/main.go:171`
- MyGo 手机端文档：https://github.com/egoist/mygo/blob/main/docs/mobile/README.md
- MyGo 构建与发布：https://github.com/egoist/mygo/blob/main/docs/mobile/building.md
- MyGo 手机界面：https://github.com/egoist/mygo/blob/main/docs/mobile/interface.md
- MyGo 设备与插件：https://github.com/egoist/mygo/blob/main/docs/mobile/plugins.md
- yamux：https://github.com/hashicorp/yamux
