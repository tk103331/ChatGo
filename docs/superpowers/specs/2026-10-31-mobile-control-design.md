# ChatGo 手机端控制方案（Mobile Control）

> 日期：2026-10-31 · 状态：方案讨论稿，**未写代码**
> 调研范围：ChatGo 现有会话、Agent、渠道、工具、ACP 实现；同类产品 PocketCtl / AgentDock / Remi / MultiCC / OpenRemote 的公开架构。
> 关联代码：`internal/ui/conversation.go`、`internal/ui/feishu.go`、`internal/ui/autotask.go`、`internal/channel/`、`pkg/models/conversation.go`、`internal/config/config.go`

---

## 0. 结论先说

| 问题 | 结论 | 置信度 |
|------|------|--------|
| 手机端控制最合适的形态？ | 桌面内置控制服务（HTTP + WebSocket）+ 可安装 PWA；远程通过 Tailscale / Cloudflare Tunnel 复用同一 API | 高（代码与同类产品双向印证） |
| 能否直接复用现有 IM 渠道？ | **只能做兜底**。渠道把消息绑定到渠道自己的会话（`internal/ui/feishu.go:212`），无流式、无完整会话管理，不是"控制桌面会话" | 高 |
| 最大工程量在哪？ | 不是 HTTP 和页面，而是**把 Agent 运行时从 ChatWindow 的 UI 线程里抽出来**：`runStream`、`askQuestionTool`、ACP `RequestPermission` 全部依赖 `postMain` 和窗口状态 | 高 |
| 是否需要自建 Relay？ | 第一期不需要。局域网直连 + 现成隧道即可；自建 Relay 只在"独立 App 公网直连且不想依赖 VPN"时才值得 | 中高 |
| 建议客户端 | **PWA 先行**（一套代码覆盖手机 / 平板 / 桌面浏览器），原生 App 第二阶段基于同一协议 | 中高 |
| 预估工期 | P0-P2 可用版 12-18 人日；含 ACP、权限、群聊 20-28 人日（单人，熟悉代码后） | 中 |

**一句话**：先把"会话与 Agent 运行时"从 UI 里解耦成可订阅的事件内核，再加 HTTP + WebSocket 控制服务和 PWA。这样手机不是"另一个聊天机器人"，而是真正操作桌面上的同一批会话。

---

## 1. 需求拆解

| 需求 | 具体含义 | 验收标准 |
|------|----------|----------|
| 手机端控制 | 手机浏览器 / PWA 连接桌面 ChatGo，读取并操作桌面上的会话 | 同一 Wi-Fi 扫码即用；断网重连不丢当前回复 |
| 完整会话管理 | 列表、新建、切换、重命名、删除、查看历史、工作区、模型 / Agent 选择、搜索 | 手机上做的增删改，桌面立即可见；反之亦然 |
| Agent 能力 | 内置 Chat、自定义 Agent、ACP Agent、模型组、工具调用可视化、流式输出、执行计划、提问、权限审批、停止 | 手机发起的任务能跑完整 ReAct / ACP，不只是纯文本问答 |
| 多端一致 | 桌面与手机共享同一份 `~/.chatgo/conversations` | 自带单写入口，不出现两端互相覆盖 |
| 安全 | 配对、设备令牌、撤销、默认不暴露到局域网 | 默认仅 127.0.0.1；开启局域网必须配对；令牌可单独吊销 |

---

## 2. 现状盘点（能复用什么）

### 2.1 可直接复用的能力

| 能力 | 位置 | 复用方式 |
|------|------|----------|
| 会话持久化 | `pkg/models/conversation.go:234`（ConversationManager） | 直接读写同一目录；需补并发保护 |
| 会话数据模型 | `pkg/models/conversation.go:48`（Message）/ `:146`（Conversation） | 已含 `Segments`、`ToolCalls`、`Turn`、`Plan`，足够表达手机端历史 |
| Agent 客户端构建 | `internal/ui/autotask.go:715`（taskClientWithChannel） | 抽出为 `agent.Factory`，输入 agent / workspace / conv，输出四种客户端之一 |
| 流式回调 | `internal/ui/conversation.go:260`（runStream） | 回调签名可映射为事件；但写状态方式必须改 |
| 内置工具 + MCP | `internal/ui/provider_client.go:165`（buildEinoToolsEnv） | 抽到 `agent` 包；工具环境需抽象出 Interaction |
| 交互式提问 | `internal/ui/askquestion.go:75`（askQuestionTool） | 已有渠道版本；泛化出 `Ask` 接口即可给手机用 |
| ACP Agent | `internal/ui/acp_session.go` / `acp_client.go:342` | 会话与权限状态目前挂在 `cw.acp`，要抽成独立运行时 |
| IM 渠道子系统 | `internal/channel/driver.go:73`、`internal/ui/channelmanager.go:150` | 保留为低带宽兜底；命令集可平移 |
| WebSocket 依赖 | `github.com/gorilla/websocket v1.5.3`（`go.sum`；`internal/channel/wecom.go` 已用） | 不需要引入新依赖 |
| 二维码 | `github.com/skip2/go-qrcode`（`internal/ui/feishu.go` 已用） | 配对二维码直接复用 |

### 2.2 关键缺口

1. **没有对外服务端**：全仓库没有 `http.Server` / `net.Listen`，只有各家 IM SDK 的 outbound 连接。
2. **Agent 运行与 UI 强耦合**：`runStream` 在回调里通过 `postMain` 改 `conv.Messages[idx]`（`internal/ui/conversation.go:260`），而 `postMain` 依赖窗口（`internal/ui/app.go:650`）。
3. **流状态是窗口私有的**：`cw.streams` 按 message id 记录运行中的回复（`internal/ui/stream.go:37`），注释明确它只在主线程读写。
4. **ConversationManager 无并发保护**：`SaveConversation` 直接 `os.WriteFile`（`pkg/models/conversation.go:307`），桌面与手机同时写会丢更新或写坏 JSON。
5. **没有变更通知**：桌面靠主动 `reloadConvs()` 刷新，手机改了会话，桌面不会自动感知。
6. **无认证 / 配对基础设施**：现有 IM 渠道用的是平台侧凭证，不适用于自建控制服务。

---

## 3. 候选方案对比

| 方案 | 做法 | 优点 | 缺点 | 适配度 |
|------|------|------|------|--------|
| **A. 增强 IM 渠道** | 在 Telegram / 飞书 / 微信上补 `/sessions` `/switch` `/cancel` 等命令，流式用消息编辑模拟 | 零新客户端；天然穿透 NAT；认证成熟 | 会话被绑在渠道上，不是桌面主会话；富 UI / 工具可视化很差；平台限流；无法做完整会话管理 | 兜底 |
| **B. 内置 HTTP+WS 控制服务 + PWA** | ChatGo 内嵌 HTTP/WebSocket 服务，手机浏览器访问；扫码配对 | 同源数据、零云成本、延迟低、可复用 Go 后端；PWA 无需上架 | 默认只能局域网；桌面必须运行；HTTP 下 PWA 不可安装 | **首选** |
| **C. 云 Relay** | 桌面主动外连 Relay，手机连 Relay；事件持久化与重放 | 任意网络可达；可做推送；多设备 | 要维护服务端、数据库、TLS、账号体系；隐私与成本 | 二期可选 |
| **D. 原生 App** | iOS / Android 原生或 Flutter / RN，走同一套 API | 体验、后台、推送最好 | 开发与分发成本最高；需要先有稳定协议 | 三期 |

**组合建议**：**B 为主，C 用现成隧道（Tailscale / Cloudflare Tunnel）替代自研，A 保留兜底，D 在协议稳定后基于同一 API 实现。**

同类产品已验证这条路径：PocketCtl 用 daemon + Relay + Web/iOS，事件可重放；AgentDock 让桌面主动外连、不开端口；Remi 支持直连 / Tailscale / Relay 三种传输；OpenRemote 在单个 Go 二进制里嵌入 Flutter Web 并带 `seq` 断线补发。这些产品共同说明：**daemon 侧统一协议、客户端只做展示与受控操作**是可行解。

---

## 4. 推荐架构

```text
┌──────────────────┐        HTTPS / WSS（局域网 / Tailscale / 隧道）        ┌──────────────────────────────┐
│ Phone / Tablet   │ ◄──────────────────────────────────────────────────► │ ChatGo Desktop               │
│ PWA: 会话 / 聊天  │                                                    │  ├ internal/remote (HTTP+WS) │
└──────────────────┘                                                    │  ├ internal/agent (Runtime)  │
                                                                        │  ├ ConversationManager       │
┌──────────────────┐        in-process event bus（同一进程）              │  ├ tools / MCP / Skills / ACP│
│ Desktop UI       │ ◄──────────────────────────────────────────────────► │  └ config                    │
└──────────────────┘                                                    └──────────────────────────────┘
```

### 4.1 组件职责

| 组件 | 职责 | 不做什么 |
|------|------|----------|
| `internal/agent` | 统一会话运行内核：加载会话、构建客户端、执行、落库、发事件 | 不 import UI，不碰窗口 |
| `internal/remote` | HTTP 路由、鉴权、配对、WS Hub、静态资源 | 不直接改会话文件，只调 Runtime |
| `internal/ui` | 桌面界面；作为 Runtime 的一个订阅者 | 不再自己跑一套独立的 Agent 循环（Phase 3 后） |
| `pkg/models` | 会话模型与持久化；补 per-ID 锁与原子写 | 不关心传输 |
| `internal/channel` | IM 桥接，保留为兜底通道 | 不复用为控制协议 |

### 4.2 一条消息的时序

1. 手机 `POST /api/sessions/{id}/messages {text}`。
2. `remote` 校验令牌、会话归属、是否有正在运行的 turn。
3. 调 `Runtime.Send(ctx, sessionID, text, opts)`。
4. Runtime 追加 human message 与占位 agent message，落库，广播 `turn.started`。
5. Runtime 用 `agent.Factory` 构建客户端（普通 / React / Group / GroupReact 或 ACP），带着 `ctx` 执行。
6. `onChunk` / `onReasoning` / `onTool` 回调转成事件，经事件总线扇出给所有订阅者（手机 WS、桌面 UI）。
7. 完成时写回 assistant message、Turn、token 用量，广播 `turn.finished`；标题为空则异步生成。
8. 手机断线重连：带 `since=seq` 请求补发；seq 太旧则下发会话快照。

### 4.3 关键设计决策

1. **API 与 UI 解耦**：`internal/remote` 只依赖 `internal/agent` 和 `pkg/models`，禁止 import `internal/ui`。这样未来原生 App、CLI、测试客户端都能复用。
2. **单写入口**：所有"发消息"都走 Runtime；桌面 UI 在 Phase 3 也改成调用 Runtime，彻底消除两套执行路径。
3. **事件是优化，会话文件是真相**：每个会话维护单调递增 `seq` 与环形缓冲；断线优先补发，补不了就发快照。绝不以内存事件为唯一状态。
4. **能力协商**：`GET /api/capabilities` 告知客户端哪些能力可用（流式 / 工具 / ACP / 提问 / 权限 / 群聊）；客户端据此隐藏按钮，而不是点了才报错。
5. **默认最小暴露**：默认 `127.0.0.1`；开启局域网是显式开关；远程推荐隧道而非端口映射。

---

## 5. 最大难点：把 Agent 运行时从 UI 中解耦

### 5.1 现状为什么不能直接给远程用

| 现状 | 代码事实 | 远程直接调用的后果 |
|------|----------|--------------------|
| 流式回调写 UI 状态 | `runStream` 的 `onChunk` 里 `cw.postMain(func(){ m := &conv.Messages[idx]; ... })`（`internal/ui/conversation.go:260`） | 无窗口时 `postMain` 静默丢弃（`internal/ui/app.go:650`），手机看不到流 |
| 客户端构建读窗口状态 | `taskClientWithChannel` 读 `cw.cfg`、`cw.fileRoot()`、`cw.mcpManager`、工具缓存（`internal/ui/autotask.go:715`、`provider_client.go:165`） | 与 UI 线程共享可变状态，存在数据竞争 |
| 提问阻塞在 UI | `askQuestionTool` 要求 `cw.askQ` 并等待 UI 面板（`internal/ui/askquestion.go:75`） | 手机发起的 Agent 一提问就报"当前没有可交互界面" |
| ACP 权限挂在窗口 | `RequestPermission` 把请求塞进 `cw.acp.permissionReq`（`internal/ui/acp_client.go:342`） | 手机无法审批，ACP Agent 卡死直到超时 |
| 流注册表是窗口私有 | `cw.streams` 按 message id 记录，只在主线程读写（`internal/ui/stream.go:37`） | 两端无法共享"正在运行"状态，也无法互相停止 |

### 5.2 目标内核接口

```go
// internal/agent
type Runtime struct { /* cfg, convManager, factory, acp, subscribers */ }

type SendOptions struct {
    Agent     string // "" | "agent:Name" | "acp:Name" | "app:id"
    Workspace string
    Provider  string
    Model     string
    Source    string // "desktop" | "mobile" | "channel" | "task"
}

type Event struct {
    Seq       int64           `json:"seq"`
    SessionID string          `json:"session_id"`
    TurnID    string          `json:"turn_id,omitempty"`
    Type      string          `json:"type"`
    At        time.Time       `json:"at"`
    Data      json.RawMessage `json:"data,omitempty"`
}

func (r *Runtime) Send(ctx context.Context, sessionID, text string, opt SendOptions) (turnID string, err error)
func (r *Runtime) Cancel(sessionID string) bool
func (r *Runtime) Session(sessionID string) (*models.Conversation, error)
func (r *Runtime) Subscribe(sessionID string) (<-chan Event, func())
```

交互式能力用接口注入，UI 与远程各自实现：

```go
type Interaction interface {
    Ask(ctx context.Context, sessionID string, items []Question) (string, error)
    RequestPermission(ctx context.Context, sessionID string, req PermissionRequest) (PermissionResponse, error)
    Emit(ev Event)
}
```

- `uiInteraction`：沿用 `cw.askQ` / `cw.acp`，通过 `postMain` 更新界面（行为不变）。
- `remoteInteraction`：广播 `question.asked` / `permission.asked`，等待任一客户端应答。
- `compositeInteraction`：**先答先赢**，同时通知另一端"已被处理"，避免两端重复操作。

### 5.3 事件类型

| 事件 | 触发点 | 手机展示 |
|------|--------|----------|
| `turn.started` | 用户消息落库后 | 插入用户气泡 + 助手占位 |
| `message.delta` | `onChunk` | 追加正文 |
| `reasoning.delta` | `onReasoning` | 折叠的"思考中" |
| `tool.started` / `tool.finished` | `onTool` / 工具结果 | 工具卡片（名称、参数、结果、耗时） |
| `plan.updated` | `update_plan` 工具 | 计划清单 |
| `question.asked` | `Ask` | 问题卡片 + 选项/输入框 |
| `permission.asked` | ACP `RequestPermission` | 允许/拒绝按钮 |
| `turn.finished` / `turn.failed` | 完成 / 错误 / 取消 | 结束态、token 用量、重试 |
| `session.updated` | 会话保存、标题生成、删除 | 刷新列表项 |

### 5.4 迁移策略（降低回归风险）

| 阶段 | 做法 | 风险 |
|------|------|------|
| Step 1（旁路） | 远程先用独立的 headless runner：把 `taskClientWithChannel` 的构建逻辑抽成 `agent.Factory`，自己跑流式并落库；桌面 UI 完全不动 | 低。手机发起的流，桌面不实时显示，完成后 `reloadConvs` 可见 |
| Step 2（统一） | 桌面 `sendDraft` / `runStream` 改为调用 Runtime，删除重复实现；桌面作为订阅者接收事件 | 中。需要严格保持现有渲染、tool call、通知行为 |
| Step 3（高级） | ACP、ask、permission、plan、群聊全部走 Interaction；IM 渠道也接同一套 | 中高。涉及 `cw.acp` 生命周期与阻塞模型 |

> **不要图省事**：直接在远程 goroutine 里调 `cw.postMain` 或改 `cw.streams`。mygo 的更新必须走主线程，且 `streams` 没有锁；这会把"偶发崩溃"引入到一个目前稳定的桌面应用里。

---

## 6. API 设计

Base：`http://<host>:8422/api`（端口可配）。鉴权：`Authorization: Bearer <device-token>`。

### 6.1 REST

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/health` | 存活探测，返回版本与是否已配对 |
| GET | `/api/capabilities` | 能力协商 |
| POST | `/api/pair` | 一次性配对码换取设备令牌 |
| GET | `/api/devices` | 已配对设备列表（桌面端也可看） |
| DELETE | `/api/devices/{id}` | 吊销设备，立即断开其 WS |
| GET | `/api/sessions` | 列表；支持 `scope=normal|task|room|all`、`workspace=`、`q=` |
| POST | `/api/sessions` | 新建 `{title?, agent, workspace, provider?, model?}` |
| GET | `/api/sessions/{id}` | 完整快照：messages（含 segments/tool_calls）、turns、plan、agent、workspace、running |
| PATCH | `/api/sessions/{id}` | 改标题 / Agent / 模型 / 工作区 |
| DELETE | `/api/sessions/{id}` | 删除；先取消运行中的 turn |
| POST | `/api/sessions/{id}/messages` | 发送 `{text}`，返回 `{turn_id}` |
| POST | `/api/sessions/{id}/cancel` | 停止当前 turn |
| POST | `/api/sessions/{id}/answers/{req_id}` | 回答 Agent 提问 |
| POST | `/api/sessions/{id}/permissions/{req_id}` | 提交权限选择 |
| GET | `/api/agents` | 内置 Chat、自定义 Agent、ACP、App、Room 及能力标记 |
| GET | `/api/models` | Provider / ProviderGroup / 模型清单 |
| GET | `/api/workspaces` | 已配置工作区 |

### 6.2 WebSocket

`GET /api/events?since=<seq>`，升级时校验同一 Bearer 令牌与 `Origin`。

统一信封：

```json
{ "v": 1, "type": "message.delta", "seq": 42, "session_id": "20261031-...", "turn_id": "t_...", "ts": "2026-10-31T10:00:00Z", "data": { "text": "..." } }
```

客户端 → 服务端：

| type | data | 说明 |
|------|------|------|
| `subscribe` | `{session_id?, since?}` | 订阅单会话或全部会话 |
| `ping` | `{}` | 心跳 |
| `answer` | `{request_id, value}` | 回答提问 |
| `permission` | `{request_id, option_id}` | 权限选择 |

服务端 → 客户端：

| type | 说明 |
|------|------|
| `hello` | `{device, server_version, capabilities}` |
| `session.updated` / `session.deleted` | 列表失效通知 |
| `turn.*` / `message.delta` / `reasoning.delta` | 流式正文与思考 |
| `tool.*` / `plan.updated` / `question.asked` / `permission.asked` | Agent 过程 |
| `error` | `{code, message}` |

### 6.3 断线与背压

- 每会话环形缓冲建议 2000 条事件；`since` 在窗口内则补发，否则下发 `snapshot`。
- 慢客户端：正文 delta 可合并/丢弃，**控制类事件**（提问、权限、结束）不可丢；重连后以快照校正。
- 心跳 25s；连续两次无 pong 关闭连接。
- 发送失败不阻塞 Agent 运行，事件总线只做非阻塞投递。

### 6.4 错误码

| HTTP | code | 场景 |
|------|------|------|
| 400 | `bad_request` | 参数缺失 / JSON 错误 |
| 401 | `unauthorized` | 令牌缺失 / 失效 / 已吊销 |
| 404 | `not_found` | 会话不存在 |
| 409 | `run_in_progress` | 同一会话已有运行中的 turn |
| 413 | `payload_too_large` | 消息体超限 |
| 429 | `rate_limited` | 配对或鉴权失败过多 |
| 500 | `internal_error` | 构建客户端失败等 |

---

## 7. 安全设计

### 7.1 配对流程

1. 桌面：设置 → 手机控制 → 开启 → 显示二维码。
2. 二维码内容：`chatgo://pair?u=http://192.168.1.8:8422&c=8F3K2Q`；配对码 6 位、TTL 120 秒、一次性。
3. 手机：`POST /api/pair {code, device_name}` → `{device_id, token, server_name, capabilities}`。
4. 桌面只保存 `sha256(token)` 与设备元数据；明文令牌只在手机端保存一次。
5. 桌面设备列表可查看"名称 / 首次配对 / 最后活跃 / 最后 IP"，并可单独吊销。

### 7.2 配置结构

```yaml
remote:
  enabled: false
  bind: "127.0.0.1"        # 开启局域网时改为 0.0.0.0
  port: 8422
  tls:
    mode: "off"            # off | selfsigned | files
    cert: ""
    key: ""
  devices:
    - id: "dev_7f3a"
      name: "iPhone 16"
      token_sha256: "..."
      created_at: "2026-10-31T10:00:00Z"
      last_seen: "2026-10-31T10:05:00Z"
      last_ip: "192.168.1.23"
```

### 7.3 威胁模型

> **关键前提**：拿到令牌等于拿到桌面 Agent 的全部能力（读写文件、执行命令、访问 MCP）。这不是普通聊天应用的权限模型，必须按"远程控制软件"对待。

| 威胁 | 影响 | 缓解 |
|------|------|------|
| 局域网被嗅探 | 会话与令牌泄露 | 推荐 Tailscale / HTTPS；局域网开启时显著警告 |
| 令牌泄露 | 等同设备被控 | 短期一次性配对码；设备级令牌；随时吊销；审计最后 IP |
| 恶意网页 CSRF | 借用浏览器发起请求 | Bearer 而非 Cookie；校验 `Origin`；WS 握手校验 |
| 暴力破解配对码 | 非法配对 | 6 位 + TTL 120s + 失败限速 + 连续失败锁定 |
| 误操作危险工具 | 文件/数据损坏 | 手机端对删除会话/危险工具二次确认；可配置只读模式 |
| 桌面休眠/退出 | 连接中断 | 明确提示；后续用 launchd/systemd 常驻（可选） |
| 日志泄露 | 消息正文入库 | 访问日志不记正文与令牌；只记设备、路径、状态码 |

---

## 8. 会话管理映射

| 桌面行为 | 手机 API | 备注 |
|----------|----------|------|
| 会话列表（按工作区分组） | `GET /sessions` | 返回 title、workspace、agent、updated、running、最后一条摘要 |
| 新建会话 | `POST /sessions` | 与 `newConversationIn`（`internal/ui/conversation.go:61`）语义一致 |
| 切换会话 | `GET /sessions/{id}` | 服务端没有"当前会话"概念，由客户端本地选中 |
| 重命名 | `PATCH /sessions/{id}` | 只改 title，不动消息 |
| 删除 | `DELETE /sessions/{id}` | 先 `Cancel`，再删文件 |
| 历史 / 思考 / 工具调用 | `GET /sessions/{id}` | 直接映射 `Message.Segments` 与 `ToolCalls` |
| 模型 / Agent 选择 | `GET /models`、`GET /agents`；发送时带覆盖或 `PATCH` | Agent 引用沿用 `"" / agent: / acp: / app:` |
| 工作区 | `GET /workspaces` | 新建会话时可选；不提供任意路径浏览（第一阶段） |
| 自动生成标题 | 无独立接口 | Runtime 完成后异步生成并推 `session.updated` |
| 搜索 | 第一阶段客户端过滤 | 第二阶段 `?q=`；需要索引或全量扫描，注意大目录 |
| 群聊（Room） | 第二阶段 `/rooms` | 与普通会话模型不同，先只读/后写入 |
| 自动任务 | 第三阶段只读展示 | 不阻塞首版 |

### 8.1 并发与一致性

- **每个会话同一时间只允许一个运行中的 turn**；第二次发送返回 `409 run_in_progress`。群聊的一次"轮次"可能有多个成员回复，但仍视为一个运行单元。
- **删除运行中的会话**：先取消 `ctx`，等待收尾落库，再删文件；期间该会话标记为 `deleting`。
- **ConversationManager 补两件事**：
  1. per-ID `sync.Mutex`，所有读写走锁；
  2. 原子写：先写 `*.json.tmp`，`fsync` 后 `rename`，避免半写文件。
- **变更通知**：`SaveConversation` / `DeleteConversation` 后向订阅者发 `session.updated` / `session.deleted`，桌面 UI 收到后 `reloadConvs()`，实现双端一致。
- **外部编辑**：若用户用编辑器改了会话 JSON，以最后一次落库为准；不做文件监听（第一阶段）。

---

## 9. Agent 能力映射

| 能力 | 现有实现 | 手机端方案 |
|------|----------|-----------|
| 内置 Chat | `taskAgentSpec("")` 取全局模型与工具 | 直接复用 |
| 自定义 Agent | `cfg.Agents` 的模型白名单、内置工具、MCP、Skills | 直接复用；`GET /agents` 暴露可选清单 |
| Provider Group | `fallback / round_robin / random` 路由 | 直接复用；客户端只看到一个逻辑模型名 |
| ACP Agent | `cw.acp` + `acpEnsureSession` + `RequestPermission` | 抽出 ACP 运行时；手机只发提示与审批，CLI 仍在桌面跑 |
| 流式正文 / 思考 | `onChunk` / `onReasoning` | 映射 `message.delta` / `reasoning.delta` |
| 工具调用 | `llm.ToolCallInfo` | `tool.started` / `tool.finished`，含参数、结果、状态、耗时 |
| 执行计划 | `models.PlanEntry` + `update_plan` | `plan.updated`；手机显示清单与进度 |
| Agent 提问 | `askQuestionTool`（`internal/ui/askquestion.go:75`） | `question.asked`；手机作答后作为工具结果返回 |
| 权限审批 | ACP `RequestPermission`（`internal/ui/acp_client.go:342`） | `permission.asked`；选项来自 ACP，不在客户端硬编码 |
| 停止 | `context.CancelFunc` | `POST /cancel`；广播 `turn.failed`（reason=cancelled） |
| Skills / 知识库 | 服务端扫描 / SQLite FTS | 手机不直接操作，只在 Agent 运行时生效 |
| 群聊 | `cw.room` + `roomturn` | 第三阶段；先做只读历史与成员展示 |
| 文件预览 / Git / 终端 | 桌面 UI 能力 | **不纳入首版**；如纳入需单独评估攻击面 |

**能力协商示例**：

```json
{ "streaming": true, "tools": true, "acp": true, "ask": true, "permission": true, "rooms": false, "fileBrowser": false, "git": false }
```

---

## 10. 目录与改动清单

### 10.1 新增

```text
internal/agent/
  runtime.go          # 统一运行内核：Send / Cancel / Subscribe / 并发控制
  events.go           # Event 类型、seq、环形缓冲
  factory.go          # 从 ui/autotask.go 抽出的客户端与工具构建
  interaction.go      # Ask / Permission 抽象与 composite
  acp_runtime.go      # ACP 会话与权限运行时（从 ui/acp_* 抽离）
internal/remote/
  server.go           # HTTP mux、生命周期、优雅关闭
  auth.go             # Bearer、设备校验、限速、Origin
  pairing.go          # 一次性配对码、QR payload、令牌哈希
  sessions.go         # REST handlers
  ws.go               # Hub、订阅、心跳、断线补发
  events.go           # 对外事件 DTO 与编解码
  static/             # PWA：index.html / app.js / style.css / manifest.json / sw.js
internal/ui/
  remote_settings.go  # 设置页：开关、端口、二维码、设备列表、吊销
```

### 10.2 修改

| 文件 | 改动 |
|------|------|
| `main.go` | 配置启用时启动 / 停止控制服务 |
| `internal/config/config.go` | 新增 `Remote` 结构、默认值、保存 |
| `internal/ui/app.go` | 创建 Runtime；订阅事件刷新 UI；`NewChatWindow` 注入 |
| `internal/ui/conversation.go` | Phase 3：`sendDraft` / `runStream` 改为 Runtime |
| `internal/ui/feishu.go`、`channelmanager.go` | Phase 3：渠道 Agent 接 Interaction |
| `pkg/models/conversation.go` | per-ID 锁、原子写、保存/删除通知 |
| `go.mod` | `gorilla/websocket` 由间接依赖改为直接依赖（版本已有） |
| `README.md` / `README.zh-CN.md` | 功能与安全说明 |

### 10.3 启动接入点

```go
// main.go（示意）
cw, err := ui.NewChatWindow(cfg)
// ...
if cfg.Remote.Enabled {
    srv := remote.New(cw.Runtime(), cfg.Remote) // 只依赖 Runtime，不依赖窗口
    srv.Start()
    defer srv.Stop()
}
```

---

## 11. 分阶段实施

| 阶段 | 交付物 | 验收 | 预估 |
|------|--------|------|------|
| **P0 骨架与鉴权** | config `remote`、HTTP server、配对码/QR、设备令牌、`/health`、静态页壳 | 手机扫码后能打开页面并拿到令牌；错误令牌 401；吊销后立即失效 | 2-3 人日 |
| **P1 会话 CRUD + 历史** | ConversationManager 锁与通知、`/sessions` 全套、PWA 列表/详情 | 手机可增删改查，桌面列表同步刷新 | 3-5 人日 |
| **P2 发送 + 流式 + 工具 + 停止** | `agent.Factory` headless runner、WS 事件、PWA 聊天页、标题生成 | 手机发消息能看到流式正文、思考、工具卡片，可停止；断线重连不重复/不丢 | 4-6 人日 |
| **P3 运行时统一 + ACP + 提问/权限 + 计划** | `agent.Runtime` 抽取、桌面改订阅、ACP 运行时、Interaction、plan | 手机能审批 ACP 权限、回答提问；桌面与手机同时看到同一条流 | 5-8 人日 |
| **P4 远程与体验** | TLS 自签、Tailscale/Cloudflare 指南、Web Push（可选）、原生壳（Capacitor，可选） | 外网可安全接入；后台提醒可用 | 3-5 人日 |

**建议先做 P0-P2 并加特性开关**（`remote.enabled` 默认 false），验证真实使用习惯后再投入 P3 的大重构。

---

## 12. 验证方案

| 层级 | 内容 |
|------|------|
| 单元测试 | 事件 seq / 环形缓冲、令牌哈希与校验、配对码过期、每会话单 run、原子写 |
| 集成测试 | `httptest` + 假 LLM 流式客户端：发送、增量、工具事件、取消；WS 断线补发与快照降级；两个设备同时连接 |
| 竞态 | `go test -race ./internal/agent ./internal/remote ./pkg/models` |
| 安全测试 | 无令牌 / 过期令牌 / 吊销令牌 401；错误 Origin 拒绝；配对码限速；超大 body 413 |
| 手工矩阵 | iOS Safari、Android Chrome、iPad；局域网 / Tailscale；锁屏 8 小时后重连；桌面关闭；超长输出；中文 / emoji / Markdown / 代码块 |
| 回归 | 现有桌面聊天、IM 渠道、自动任务行为不变；关闭 `remote.enabled` 时零行为差异 |

---

## 13. 风险与取舍

| 风险 | 影响 | 缓解 |
|------|------|------|
| 运行时重构引入回归 | 高 | Phase 1 走旁路，不删旧路径；特性开关；先补集成测试再迁移 |
| 多端写同一会话丢更新 | 高 | per-ID 锁 + 原子 rename + 统一写入口；删除与运行互斥 |
| 令牌等价远程控制 | 高 | 一次性配对、设备级令牌、TLS/Tailscale、可撤销、审计 |
| 手机切网断流 | 中 | seq 补发 + 快照降级 + 心跳；客户端前台重连 |
| 局域网 HTTP 下 PWA 不可安装 | 中 | 文档说明；推荐 Tailscale HTTPS；或提供自签证书信任指引 |
| 桌面休眠 / 退出导致不可用 | 中 | 明确提示；后续提供常驻服务安装 |
| ACP 权限无人应答 | 中 | 超时 + 桌面兜底 + 通知；手机端常驻权限卡片 |
| 工具循环导致成本失控 | 中 | 展示步骤与 token；沿用 `ReactAgentMaxStep`；可加手机端步数上限 |
| iOS 后台挂起 WS | 中 | 前台重连；Web Push 作为 P4 可选项 |
| 文件 / Git / 终端能力扩大攻击面 | 高 | 首版明确不做；如做需独立权限与路径白名单 |

---

## 14. 待确认问题

1. **首期客户端**：PWA 还是直接原生 iOS/Android？（推荐 PWA，先验证需求）
2. **远程范围**：只做局域网，还是必须"任何网络都能连"？后者需要 Tailscale/隧道，或自建 Relay。
3. **是否要求镜像桌面正在运行的流**：如果必须，P3 的运行时统一要提前到 P2。
4. **手机权限边界**：允许删除会话、改配置、执行危险工具吗？建议默认"聊天 + 停止 + 审批"，删除/配置需二次确认。
5. **认证强度**：一次性配对码 + 设备令牌是否足够？是否需要 passkey / SSO。
6. **群聊、自动任务、文件预览、Git diff、终端**是否纳入首版？
7. **是否接受"桌面必须运行"**：如果要离线/常驻，需要考虑后台服务化与开机自启。

---

## 15. 参考

- PocketCtl：daemon + Relay + Web/iOS，事件持久化与重放 — https://github.com/pocketctl/pocketctl
- AgentDock：桌面主动外连，不开端口，tmux 式会话 — https://github.com/Nigh/AgentDock
- Remi：直连 / Tailscale / Relay 三种传输，手机审批 — https://docs.yooz.live/tools/remi/
- MultiCC：REST + WS，PWA / Flutter / IM 多端 — https://github.com/lsjwzh/MultiCC
- OpenRemote：单 Go 二进制 + 嵌入式 Flutter Web + `seq` 断线补发 — https://github.com/morewebs/OpenRemote
