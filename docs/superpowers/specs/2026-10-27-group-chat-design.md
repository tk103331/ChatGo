# ChatGo 多 Agent 群聊方案（借鉴 cumora）

> 日期：2026-10-27 · 状态：方案讨论稿，**未写代码**
> 参考源码：[yetone/cumora](https://github.com/yetone/cumora) —
> `docs/COORDINATION.md`、`server/src/agents/glance-protocol.ts`、`server/src/agents/triage-core.ts`

---

## 0. 结论先说

| 问题 | 结论 | 置信度 |
|------|------|--------|
| cumora 的群聊能否直接搬到 ChatGo？ | **不能直接搬**。cumora 的协调机制 80% 依赖 Postgres 事务 + Redis + 多进程架构，ChatGo 是单进程桌面应用、JSON 文件存储 | 高（已读两边源码） |
| 它的**设计思想**能否用？ | **能，而且价值很高**。真正值钱的是"乐观发言 + 服务端 HOLD 重算"这个模型和五条协调原则，这套思想与存储无关 | 高 |
| ChatGo 现有的 "Provider Group" 是不是群聊？ | **不是**。它是模型路由组（fallback/round-robin/random），一次请求只会命中一个 endpoint，见 `internal/llm/group.go:84` | 高（代码确认） |
| 改造成本 | 中等。核心是数据模型加 `AuthorID` + 一个串行化的群聊调度器，不需要动 UI 框架和 LLM 层 | 中（未实际动手） |

**一句话**：cumora 解决的是"N 个独立进程并发写同一个会话"的分布式竞态问题；ChatGo 是单进程，可以用 **一个 goroutine 串行调度** 把竞态问题直接消灭掉，省掉 cumora 一半的防御层。但 cumora 的 **提示词协调原则** 和 **triage 省钱门控** 要原样学过来。

---

## 1. cumora 群聊实现原理拆解

### 1.1 核心模型

cumora 的群聊不是"编排器轮流调用 Agent"，而是**每个 Agent 都是独立自治进程**：

```
一条消息落库
    ↓ SSE 广播
所有在该会话的 Agent 同时被唤醒（N 个独立进程/pod）
    ↓ 各自独立决策
每个 Agent 都可能决定发言 → 并发写 messages 表
    ↓
服务端闸门串行化，输掉的那个被 HOLD 并看到新消息，重新决策
```

这个设计的关键取舍：**不做"谁该发言"的中央编排**，而是让所有 Agent 乐观抢跑，用服务端闸门兜底。cumora 在 `glance-protocol.ts` 注释里明确写了理由：

> 一个 Agent 只能看到 **已发布的消息流** 和一个私有的 per-(agent, convo) seen-cursor —— 没有"谁在打字""谁排第几"的名册。
> 这让"按位置占座"（我是第 3 个认领的 → 我发 3）在**结构上不可表达**，正是这一点让旧的一大堆场景化提示词规则塌缩成了五条。

**这是整个设计最聪明的地方**：通过限制 Agent 能看到的信息，让错误的推理方式无法被表达，而不是靠提示词去禁止它。

### 1.2 防御层（从"常开、不占脑力"到"软性、靠脑子"）

cumora 列了 11 层，按性质归为四类：

**A. 并发/配额控制（纯工程，不涉及语义）**
- 大脑并发上限 `MAX_CONCURRENT_BIG_BRAIN`（默认 6）—— 7 个 Agent 同时醒会把 provider 打到限流，实测 17 分钟 130 次 429
- triage 小模型并发上限（默认 8）—— **教训：只给一层加帽子等于没加**。只限大脑不限 triage，会导致 triage 超时 → 被当成限流 → 全机器静默
- 确定性发车间隔 `MIN_SPAWN_INTERVAL_MS`（500ms）—— 原来用随机 jitter，但随机是概率性的，4 个请求可能都摇到小值仍然同时打过去；固定间隔把突发速率变成构造性的 1/interval
- `AdaptivePacer`：遇限流间隔翻倍（上限 8s），5 次干净调用后减半
- 唤醒去抖 `WAKE_DEBOUNCE_MS`（2500ms）—— 一串群消息合并成**一次**推理，而不是 N 次；运行中到来的唤醒合并成一次待重跑

**B. 服务端闸门（事务级，Agent 无法绕过）**
- **新鲜度预检**：发言前查"有没有比我上次看到的更新的别人消息"，有则返回 HELD（退出码 2）+ 把新消息内联给它重新决策
- **逐字重复 HOLD**：在 `BEGIN/COMMIT` 事务内、拿到行锁之后，再比一次和上一条他人消息是否逐字相同，相同就 ROLLBACK。**必须在事务内**，因为事务外检查有 TOCTOU 窗口
- **HOLD token 门控的覆盖标志**：`--send-anyway` 只在服务端真的给它展示过 HOLD 时才有效，且 token 绑定它确认过的那个 seq、turn 结束即失效、2 分钟 TTL

**C. 成本门控（triage，小模型当看门狗）**
- 小模型只判断 `actionable: true/false`，**绝不决定谁回、怎么回、说什么**
- 唯一原则：**有人类参与或等待 → 永远 actionable**；唯一压制的是"纯 Agent 之间、且背后没有权威未完成工作"的闲聊
- 人类消息**直接跳过 triage**（反正答案一定是 yes，付这个钱就是白付延迟）
- 底下压确定性地板：已认领线程硬上限 20 条、未认领线程"自缩放地板"（消息数 > 参与过的不同 Agent 数 = 开始套圈 = 死循环）
- **注释里写了回归守卫**：这些地板被以"AI-native 优雅"为名删过两次，两次都回归了循环，不要删

**D. 提示词原则（软性，有天花板）**
五条，全是形状级（shape-level），不枚举场景：
1. 人类可以不 @ 就指名某个成员 —— 看清他叫的是谁，不是你就别插嘴
2. 从**真实已发布的状态**回复，不要从"我排第几"或"猜别人会干啥"回复
3. **乐观发言，服务端是你的安全网** —— 不要 glance→think→glance 循环，HELD 了再重算
4. 不要重复别人，完成以**任务的项数**而非**人头数**衡量；有人缺席，在场的人接下一项，哪怕是第二次发言
5. **永远不要认领聊天轮次或游戏位次** —— 认领只用于真正会被重复生产的共享交付物

### 1.3 cumora 自己总结的反模式（这部分最值钱）

- **不要在提示词里堆场景示例** —— 看着有用，实际上 (a) 撑大提示词 (b) 让模型在非该场景下更差 (c) 开启"每个 bug 加一条"的滑坡，这是最贵的一类提示词 bug
- **不要用提示词修基础设施问题** —— 实例：分类器 100% 返回 503 导致整条唤醒链死掉，当时的诱惑是加更多提示词规则催 Agent，正确做法是读日志查上游
- **不要把"缺席成员"当成要修的故障** —— 用户的原话纠正了整个问题框架："团队里有人没响应，游戏也应该能打完。就像真实团队，有人休假你不会说这活干不了了。" 修复在提示词原则层（团队适配缺席），不在运维层（救活那个 Agent）
- **不要发没有代价的覆盖标志** —— 软机制守软机制必然腐化：Agent 为了省一轮往返，学会了**预防性**传 `--send-anyway`，闸门就静默地不存在了
- **出问题时先和上一个已知良好基线做 diff**，不要往坏状态上继续堆机制

---

## 2. ChatGo 现状盘点

### 2.1 技术栈

- Go 1.27.1 + [MyGo](https://github.com/egoist/mygo)（GPU 绘制原生 UI，无 webview，无 cgo）
- Cloudwego Eino 做 LLM 抽象和 ReAct Agent
- **单进程桌面应用**，状态全在 `ChatWindow` 结构体里，UI 更新靠 `cw.postMain()` 投递到主线程（`internal/ui/app.go:608`）
- 会话持久化：**每个会话一个 JSON 文件**，`~/.chatgo/conversations/<id>.json`（`pkg/models/conversation.go`）
- 配置：`~/.chatgo/config.yaml`
- 知识库用了 SQLite（`modernc.org/sqlite`，纯 Go），但会话没用

### 2.2 现有的"群"概念——不是群聊

`config.ProviderGroup` + `llm.GroupClient` 是**模型路由组**：

```go
// internal/llm/group.go:84
func (g *GroupClient) Chat(...) (*ChatResponse, error) {
    for _, endpoint := range g.routes("") {
        resp, err := endpoint.client.Chat(ctx, messages, chunk, onReasoning...)
        if err == nil {
            return resp, nil   // ← 第一个成功的就返回，不会多个一起说话
        }
        // 已经流出内容的不重试
    }
}
```

语义是**故障转移**，不是多方对话。README 里的"Provider Groups"说的也是这个。**命名上要小心**：如果新功能也叫 "Group"，会和这个撞车，建议叫 **Room（房间）** 或 **群聊**。

### 2.3 数据模型的硬伤

```go
// pkg/models/conversation.go
type Message struct {
    ID        string
    Role      string      // ← 只有 user / assistant / system
    Content   string
    Reasoning string
    Segments  []Segment
    Timestamp time.Time
    ToolCalls []ToolCall
}

type Conversation struct {
    ID       string
    Messages []Message
    Turns    []Turn
    Provider string      // ← 会话级单一 provider
    Model    string      // ← 会话级单一 model
    Agent    string      // ← 会话级单一 agent："agent:X" / "acp:X" / "app:X"
    ...
}
```

四个问题：
1. **`Message` 没有作者身份** —— 所有 AI 消息都是 `role=assistant`，无法区分是哪个 Agent 说的
2. **`Conversation.Agent` 是单值** —— 一个会话绑定一个 Agent
3. **`Turn` 假设严格的 user/assistant 配对**（`UserMsgID` + `ReplyMsgID`），群聊里一个用户消息可能对应 3 个 Agent 的回复，这个结构表达不了
4. **发给模型的上下文丢掉了身份**：
```go
// internal/ui/conversation.go:585
func buildMessages(conv *models.Conversation, upto int) []llm.ChatMessage {
    for i := 0; i < upto && i < len(conv.Messages); i++ {
        m := conv.Messages[i]
        out = append(out, llm.ChatMessage{Role: m.Role, Content: m.Content})
        //                                  ↑ 身份信息在这里彻底丢失
    }
}
```
群聊里 Agent 必须知道"这句是 Nova 说的、那句是人说的"，否则五条协调原则一条都没法执行。

### 2.4 流式状态是单槽的

```go
// internal/ui/app.go:160-169
llmClient   *llm.Client
reactClient *llm.ReactClient
groupClient *llm.GroupClient
groupReact  *llm.GroupReactClient

streaming       bool              // ← 全局单个布尔
streamingConvID string            // ← 单个会话 ID
streamCancel    context.CancelFunc // ← 单个 cancel
```

`cw.streaming` 在 8 个文件里被读写。群聊要让 2-3 个 Agent 同时流式输出，这套单槽状态必须改成 per-message 的。

### 2.5 可复用的现成资产（这部分比预想的多）

| 资产 | 位置 | 群聊里的用处 |
|------|------|-------------|
| `AgentDef`（名字/头像/system prompt/模型/工具/MCP/Skills） | `internal/config/config.go:112` | **直接就是 cumora 的 persona** |
| `taskClientWithChannel()` —— 按 agent 名构建独立 client，不碰全局状态 | `internal/ui/autotask.go:708` | **群聊调度器的现成构件**，已经支持 react/group/plain 四种形态 |
| `taskAgentSpec()` —— 解析 agent 的 provider/model/prompt/workspace/tools | `internal/ui/autotask.go:777` | 同上 |
| `startAutoTask()` —— 在 goroutine 里跑 Agent、`postMain` 回写 | `internal/ui/autotask.go:587` | **单 Agent 异步执行的参考实现** |
| Channel 驱动架构（飞书/微信/Telegram/钉钉/Slack/Discord/企微） | `internal/channel/driver.go` | 群聊接入 IM 的现成通道 |
| `MessageBubbleOptions{Name, Avatar}` | `internal/ui/kit/message.go` | **气泡已经支持显示名字和头像**，群聊 UI 改动比预想小 |
| `agentAvatar()` / `agentMarkByName()` | `internal/ui/agent_avatar.go` | 按 agent 名渲染头像，现成 |

**`autotask.go` 这套东西几乎就是为群聊准备的**：它已经做到了"不碰 `cw.llmClient` 全局状态，按 agent 名独立构建 client，异步跑，postMain 回写"。群聊调度器可以直接复用这个模式。

---

## 3. 落差分析：哪些能搬，哪些不能

| cumora 机制 | ChatGo 可行性 | 理由 |
|------------|--------------|------|
| 每 Agent 独立进程 + SSE 唤醒 | ❌ **不要** | 单进程桌面应用，起 N 个进程纯属自找麻烦 |
| Postgres 事务 + 行锁做串行化 | ❌ **不需要** | 单进程里一个 goroutine 串行调度即可，这是架构红利 |
| Redis seen-cursor | ❌ **不需要** | 内存 map 即可，退出不需要保留 |
| 新鲜度预检 / HELD | ⚠️ **简化保留** | 串行调度后竞态消失，但"Agent 决策时看到的状态"和"它发言时的状态"之间仍有时间差（它推理了 10 秒，期间上一个 Agent 发完了）→ 发言前再比一次仍有价值 |
| 逐字重复 HOLD | ✅ **保留** | 便宜、有效、防"两个 Agent 说一样的话" |
| 大脑并发上限 | ✅ **保留** | 本地跑 3 个 Agent 打同一个 provider 一样会限流 |
| 发车间隔 pacer | ✅ **保留** | 固定间隔而非随机 jitter，照抄 |
| 唤醒去抖/合并 | ✅ **保留** | 用户连发 3 条消息应该合并成一次推理 |
| triage 小模型门控 | ⚠️ **可选** | 桌面场景大多是人主动发起，Agent 之间自由对话时才需要；**确定性循环地板必须要** |
| HOLD token 门控 | ❌ **先不要** | ChatGo 的 Agent 不通过 CLI 发言，没有"传标志"这个动作面 |
| 五条协调提示词原则 | ✅ **原样照搬** | **最高价值项**，与存储/架构完全无关 |
| 认领机制（card claim） | ❌ **先不要** | ChatGo 没有看板/文档这类共享交付物 |

**核心洞察**：ChatGo 单进程这件事，让 cumora 的 A 类（并发控制）部分保留、B 类（事务闸门）大幅简化、C 类（成本门控）部分保留、**D 类（提示词原则）全量继承**。工作量主要在数据模型和调度器，不在协调算法。

---

## 4. ChatGo 群聊方案

### 4.1 概念模型

```
Room（群聊会话）
├── Members: []RoomMember          # 人类 1 个 + N 个 Agent
├── Messages: []Message            # 每条带 AuthorID
└── Cursor: map[agentID]int        # 每个 Agent 看到第几条（内存态）

一条新消息落盘
    ↓
Scheduler（单 goroutine，串行）
    ↓ 对每个成员跑 triage 门控（便宜）
    ↓ actionable 的进入发言队列
    ↓ 按并发上限 + 发车间隔逐个唤醒
每个 Agent 独立推理（并发，受 semaphore 限制）
    ↓ 发言前过闸门（新鲜度 + 逐字重复）
    ↓ 通过则 append 到 Messages，推进 cursor
    ↓ 新消息触发下一轮调度（去抖 2.5s 合并）
```

**与 cumora 的关键区别**：Scheduler 是**唯一的写入点**，所有 append 都经过它，所以"两个 Agent 同时 INSERT"这个竞态在 ChatGo 里不存在。但"Agent 推理期间世界变了"这个问题仍然存在，所以闸门要保留。

### 4.2 数据模型改动

**Message 加作者身份（向后兼容）**：

```go
type Message struct {
    ID   string `json:"id"`
    Role string `json:"role"`  // 保留：user / assistant / system

    // 新增：作者身份。空值 = 老数据，按 Role 推断（user→人类，assistant→当前 Agent）
    AuthorID   string `json:"author_id,omitempty"`   // "human" | "agent:Nova" | "acp:claude-code"
    AuthorName string `json:"author_name,omitempty"` // 显示名，渲染和入 prompt 都用
    AuthorKind string `json:"author_kind,omitempty"` // "human" | "agent" | "system"

    // 新增：群聊序号，单调递增，闸门比较用（JSON 文件没有数据库 sequence）
    Seq int `json:"seq,omitempty"`

    Content   string     `json:"content"`
    Reasoning string     `json:"reasoning,omitempty"`
    Segments  []Segment  `json:"segments,omitempty"`
    Timestamp time.Time  `json:"timestamp"`
    ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}
```

**Conversation 加成员列表**：

```go
type Conversation struct {
    // ... 现有字段全部保留 ...

    // 新增：群聊成员。为空 = 单聊（现有行为完全不变）
    Members []RoomMember `json:"members,omitempty"`
    // NextSeq 是下一条消息的序号
    NextSeq int `json:"next_seq,omitempty"`
}

// RoomMember 是群里的一个参与者。
type RoomMember struct {
    // ID 是 "agent:<Name>" 或 "acp:<Name>"，与 Conversation.Agent 同格式
    ID string `json:"id"`
    // Name 是显示名，默认取 AgentDef.Name，允许群内改名
    Name string `json:"name,omitempty"`
    // Muted 的成员留在群里但不被唤醒
    Muted bool `json:"muted,omitempty"`
}
```

**为什么用"空值 = 旧行为"而不是版本号**：ChatGo 的会话是一堆独立 JSON 文件，没有迁移脚本的地方。`len(Members) == 0` 判定为单聊，老文件读进来零改动就能跑，新字段首次保存时自然写入。

**Turn 怎么办**：群聊里不写 `Turn`。`Turn` 的语义是"一问一答 + 该轮的模型和 token"，群聊里一个用户消息对应 N 个回复，强行塞会把 `turnOf()`（`conversation.go:371`，按 `ReplyMsgID` 反查）的语义搞坏。群聊的 token 统计换成挂在 `Message` 上（新增 `Tokens` 字段），这是更正确的归属——**token 本来就是按消息产生的，`Turn` 是单聊场景下的巧合聚合**。

### 4.3 上下文构造：把身份喂给模型

这是**最关键的一处改动**，群聊成立与否全看这里。

```go
// buildRoomMessages 把群聊历史转成某个 Agent 视角的 LLM 消息。
//
// 群聊的根本约束：OpenAI/Claude 的 messages 结构只有 user/assistant/system
// 三个角色，无法表达"5 个不同的说话人"。所以把"谁说的"编码进 content，
// 并把视角 Agent 自己的发言标成 assistant，其余全部标成 user。
//
// 为什么不用 OpenAI 的 name 字段：Claude / Gemini / Ollama 协议里没有对等
// 字段，ChatGo 支持 9 种 provider，统一走 content 前缀是唯一跨协议的做法。
func buildRoomMessages(conv *models.Conversation, selfID string, upto int) []llm.ChatMessage {
    out := make([]llm.ChatMessage, 0, upto)
    for i := 0; i < upto && i < len(conv.Messages); i++ {
        m := conv.Messages[i]
        if strings.TrimSpace(m.Content) == "" {
            continue
        }
        if m.AuthorID == selfID {
            // 自己的发言：用 assistant，不加前缀——模型应该认得自己的话
            out = append(out, llm.ChatMessage{Role: "assistant", Content: m.Content})
            continue
        }
        // 别人的发言：标注说话人和序号。序号让"从真实已发布状态回复"
        // 这条原则可执行——Agent 能看出哪条是最新的。
        name := m.AuthorName
        if name == "" {
            name = legacyName(m)  // 老数据回退
        }
        out = append(out, llm.ChatMessage{
            Role:    "user",
            Content: fmt.Sprintf("[#%d %s]: %s", m.Seq, name, m.Content),
        })
    }
    return out
}
```

**权衡说明**：把别人（包括其他 Agent）的话都标成 `user`，会让模型觉得"所有人都在跟我说话"。替代方案是把其他 Agent 的话也标成 `assistant`，但那样模型会把别人的话当成自己说过的，更糟。cumora 的 `compactMessages` 也是把整个消息流压成带标记的纯文本喂进去，同一个思路。

### 4.4 System Prompt：协调原则

每个 Agent 的 system prompt = `AgentDef.SystemPrompt` + 群聊协调块：

```go
// roomProtocol 是群聊协调原则，拼在每个成员的 system prompt 后面。
//
// 形状级（shape-level）规则，不枚举场景。这一点是 cumora 用血换来的教训：
// 场景化示例会撑大提示词、让模型在别的场景下更差，并开启"每个 bug 加一条"
// 的滑坡。改这里时保持条数不变，不要加例子。
const roomProtocol = `你在一个群聊里，成员包括人类和其他 AI 助手。消息带 [#序号 说话人] 前缀。

1. 人类可以不用 @ 就指名某个成员。看清他叫的是谁——不是你就别插嘴，可以保持沉默。
   "人类问了群里，总得有人答"只适用于明确问全体的消息。

2. 基于**真实已发布的消息**回复，不要基于"我排第几"或"猜别人会说什么"。
   对于逐项推进的任务（计数、接龙、每人选一个），接**已发布的最大项**之后那一项。
   新任务定义自己的起点——不要延续上一个活动的旧状态。

3. 读完就说，不要反复确认。如果有人在你组织语言时说了同样的内容，系统会拦住你
   并把新消息给你，你重新判断再说就行。

4. 不要重复别人说过的。完成以**任务的项数**衡量，不是人头数——还有剩余项而
   有成员缺席时，在场的人接下一项，哪怕这是你第二次发言。所有项完成后就停。

5. 不需要你发言时，保持沉默就是正确的行为，不要为了"参与感"说话。`
```

**第 5 条是 ChatGo 特有的**。cumora 是团队协作场景，Agent 不说话是异常；ChatGo 是桌面工具，用户同时唤醒 3 个 Agent 很可能只想听其中一个的，必须明确给沉默授权。

### 4.5 调度器

```go
// roomScheduler 是群聊会话的唯一写入点，单 goroutine 串行执行。
//
// 单进程架构的红利：cumora 需要 Postgres 行锁 + Redis 来串行化 N 个独立
// 进程的并发写入，ChatGo 只需要让所有写入走同一个 goroutine。竞态消失，
// cumora 的"事务内逐字查重"这类机制可以简化。
//
// 但"Agent 推理期间世界变了"这个问题仍然存在（它想了 10 秒，期间别人发完
// 了），所以发言前的闸门要保留。
type roomScheduler struct {
    convID string

    // wake 是唤醒信号，去抖后触发一轮调度
    wake chan struct{}
    // cursor 记录每个成员看到第几号消息。内存态，退出不保留——
    // 重启后从头看一遍是安全的（最多多推理一次），
    // 而把它持久化会引入"cursor 和消息不一致"的新故障类
    cursor map[string]int

    // sem 限制同时推理的 Agent 数。本地 3 个 Agent 打同一个 provider
    // 一样会撞限流，cumora 实测 7 个 Agent 17 分钟 130 次 429
    sem chan struct{}
    // lastSpawn 实现确定性发车间隔。
    // 不用随机 jitter：随机是概率性的，几个请求可能都摇到小值仍然同时打出去；
    // 固定间隔让突发速率在构造上就是 1/interval
    lastSpawn time.Time
}

const (
    // roomWakeDebounce 合并连续唤醒。用户连发 3 条消息应该是一次推理，
    // 不是三次。内容无关，不做分类。
    roomWakeDebounce = 2500 * time.Millisecond
    // roomMaxConcurrent 是同时推理的 Agent 上限
    roomMaxConcurrent = 3
    // roomMinSpawnInterval 是两次 provider 调用之间的最小间隔
    roomMinSpawnInterval = 500 * time.Millisecond
)
```

一轮调度的流程：

```
1. 去抖等待（2.5s 内的新唤醒合并进来）
2. 快照当前消息列表，算出每个成员的未读
3. 对每个成员跑门控 gate()：
   - 未读为空 → 跳过（计数判断，不是分类）
   - 未读里有人类消息 → 直接 actionable（答案一定是 yes，不必付 triage 的钱）
   - 纯 Agent 之间的消息 → 查确定性循环地板，过了就压制
   - 其余 → 可选的小模型 triage
4. actionable 的成员进队列，按 sem + 发车间隔逐个起 goroutine
5. 每个 goroutine：构造该成员视角的 messages → 推理 → 产出草稿
6. 草稿回到调度器，过闸门：
   - 新鲜度：推理期间有没有新的他人消息？有 → 把新消息追加进上下文，重跑一次（最多 1 次）
   - 逐字重复：和最近的他人消息逐字相同？→ 丢弃
7. 通过的 append 到会话，Seq++，推进该成员 cursor，保存 JSON
8. 新消息触发下一轮唤醒（回到 1，被去抖吸收）
```

**关于"重跑一次"的上限**：cumora 的 HELD 是把控制权交回 Agent，它可以反复重试。ChatGo 里重跑意味着再花一次推理钱，所以限制为 1 次——第二次仍然冲突就丢弃该发言。这是桌面场景的成本取舍。

### 4.6 循环地板（必须要，不能省）

```go
// loopFloor 判断纯 Agent 之间的对话是否已经成为死循环。
//
// 自缩放设计（照搬 cumora）：以"自上次人类发言以来的消息数"对比"参与过的
// 不同 Agent 数"。消息数 > 不同 Agent 数 说明有人说了第二遍，一轮已经走完
// 并开始套圈 → 判定为死循环。
//
// 为什么不用固定阈值：固定值小于团队规模会掐死正常的多轮协作，大于闲聊规模
// 又放过死循环。自缩放地板随实际在场人数变化，规避了这个两难。
//
// 回归守卫：cumora 的注释明确写了这类确定性地板被以"AI-native 优雅"为名
// 删除过两次，两次都回归了死循环。不要删。
func loopFloor(msgs []models.Message) bool {
    // 从最后一条人类消息往后数
    agents := map[string]struct{}{}
    count := 0
    for i := len(msgs) - 1; i >= 0; i-- {
        if msgs[i].AuthorKind == "human" {
            break
        }
        if msgs[i].AuthorKind == "agent" {
            agents[msgs[i].AuthorID] = struct{}{}
            count++
        }
    }
    return count > len(agents)
}
```

**桌面场景额外需要一个硬刹车**：cumora 的循环烧的是公司的 token，ChatGo 烧的是用户自己的钱。建议再加一个绝对上限（比如人类不说话时，Agent 之间最多来回 10 条），并且在 UI 上给一个显眼的"停止"按钮。

### 4.7 并发流式输出

现有的单槽状态要改：

```go
// 现状（internal/ui/app.go:165-169）
streaming       bool
streamingConvID string
streamCancel    context.CancelFunc

// 改为：按消息 ID 索引的运行中流
type streamState struct {
    convID string
    cancel context.CancelFunc
}
streams map[string]*streamState   // key: 正在写入的 message ID
```

然后把 8 个读写点改成查这个 map：
- `internal/ui/chat.go:122` —— 输入框是否显示"停止"
- `internal/ui/message.go:25` —— 气泡是否显示光标
- `internal/ui/sidebar.go:519` —— 侧边栏是否显示运行指示
- `internal/ui/conversation.go:481` —— 停止逻辑改成停止该会话的全部流
- `internal/ui/acp_session.go` 的 3 处

**这是改动面最大的一块**，但都是机械替换，没有逻辑难点。建议**单独一个 commit 先做这个重构**，让单聊在新结构下跑通，再接群聊。

### 4.8 UI

改动比预想的小，因为 `MessageBubbleOptions` 已经有 `Name` 和 `Avatar`：

- **成员管理**：会话顶部一排头像 + 一个"+"按钮，从现有 Agent 列表里勾选；复用 `agents.go` 的选择器
- **消息气泡**：`roleOf()` 之外再按 `AuthorID` 取名字和头像，`agentMarkByName()` 现成
- **@ 提及**：输入框打 `@` 弹成员列表。作为**可选增强**——按原则 1，人类可以不 @ 就指名，但 @ 能让门控更确定
- **沉默可见性**：Agent 决定不发言时，UI 要有极轻量的痕迹（头像上一个灰点），否则用户会以为卡住了。cumora 靠 👀 reaction 解决，ChatGo 没有 reaction 机制，用状态点代替

### 4.9 配置

```yaml
rooms:
  - id: "room-1"
    name: "产品评审"
    members:
      - id: "agent:架构师"
      - id: "agent:产品经理"
      - id: "agent:测试"
    max_concurrent: 3        # 同时推理的 Agent 数
    triage: false            # 是否用小模型门控（默认关，桌面场景多为人主动发起）
    triage_model: ""         # 开启时用哪个便宜模型
    loop_cap: 10             # 人类沉默时 Agent 之间的硬上限
```

---

## 5. 实施路径

分四期，每期可独立验证、独立交付。

### 一期：地基（不含群聊功能）
1. `Message` 加 `AuthorID` / `AuthorName` / `AuthorKind` / `Seq`，空值回退到现有 `Role` 语义
2. 单聊的写入点补上这些字段（`sendDraft` 写 `human`，`runStream` 写当前 Agent）
3. `streaming` 单槽状态重构为 `streams map[string]*streamState`
4. **验证标准**：所有现有测试通过；老 JSON 会话能正常打开；单聊行为零变化

> 这期不碰群聊，纯粹是让数据模型和流式状态有能力承载群聊。单独交付可以降低风险。

### 二期：最小可用群聊（无门控、无闸门）
1. `Conversation.Members` + `RoomMember`
2. `buildRoomMessages()` 身份注入
3. `roomProtocol` system prompt
4. 调度器骨架：串行、去抖、semaphore、发车间隔；成员**全部**唤醒（不做门控）
5. UI：成员管理 + 带名字头像的气泡
6. **验证标准**：3 个人格差异明显的 Agent（比如架构师/产品/测试）进一个群，问一个开放问题，观察是否各自从自己角度发言、是否互相引用、是否出现明显重复

> 这期先不加门控和闸门，目的是**先看裸协调能力有多强**。cumora 的经验是提示词原则本身就能解决大部分问题，先验证这一点，再决定要加几层机制——避免一上来就过度工程。

### 三期：闸门与门控
1. 新鲜度检查 + 1 次重跑
2. 逐字重复丢弃
3. 循环地板 + 硬上限
4. 沉默的 UI 可见性
5. **验证标准**：复刻 cumora 的基准测试——计数游戏（3 个 Agent 从 1 数到 10，检查顺序和重复）、接龙（检查缺席成员时能否完成）

> cumora 的 `benchmarks/` 目录有现成的多 Agent 协调基准（chain / counting / werewolf / kanban），测试思路可以直接借用。

### 四期：增强（按需）
1. 小模型 triage（跑完三期基准后再判断值不值）
2. @ 提及
3. 群聊接入 IM 通道（Channel 层已经是驱动架构，群聊接上去是自然延伸）
4. ACP Agent 入群（需要先确认 ACP 会话的并发模型，`acp_session.go` 的状态机可能也是单槽的）

---

## 6. 风险与开放问题

### 必须正视的风险

| 风险 | 说明 | 缓解 |
|------|------|------|
| **成本失控** | 3 个 Agent × 每轮都发言 = 单聊的 3 倍 token，Agent 互相引用还会放大上下文。这是 ChatGo 和 cumora 最大的处境差异——烧的是用户自己的钱 | 循环地板 + 硬上限 + 显眼的停止按钮 + UI 显示本轮累计 token |
| **上下文膨胀** | 群聊历史带 `[#N 说话人]` 前缀，比单聊更占 token；N 个 Agent 各自一份完整历史 | 可能需要滑动窗口或摘要；**二期要实测增长曲线再决定** |
| **身份编码可能不够** | 把别人的话都标成 `user` 是妥协，小模型可能分不清说话人 | 二期用不同能力的模型实测；必要时在 system prompt 里重申"你是 X，前缀标明了说话人" |
| **ACP Agent 的并发未知** | ACP 是本地 CLI 进程，一个进程能不能承载群聊里的并发唤醒没验证 | 四期再碰；一到三期只支持普通 Agent |
| **提示词腐化** | cumora 反复强调的滑坡：每遇到一个 bug 就加一条规则 | 把"五条、形状级、不加例子"写进代码注释作为约束（上面 `roomProtocol` 的注释已经这样写了） |

### 需要你决策的问题

1. **这个功能的目标场景是什么？** 直接影响设计重心：
   - *多视角评审*（架构师+产品+测试看同一个方案）→ 重点是人格差异和并发发言，几乎不需要协调机制
   - *协作完成任务*（多 Agent 分工做一件事）→ 需要完整的协调层，甚至需要认领机制
   - *纯娱乐*（Agent 之间对话给人看）→ 重点是循环控制和成本上限

2. **命名**：`ProviderGroup` 已经占了 "Group"。建议 **Room / 群聊**，但你定。

3. **Turn 结构怎么处理**：方案里建议群聊不写 `Turn`、token 挂到 `Message` 上。但 `Turn` 可能在统计页面有用（`02dab98` 那个 commit 提到统计页），需要确认统计逻辑依赖 `Turn` 到什么程度。

4. **一期的重构要不要单独发版**：`streaming` 重构碰 8 个文件，单独发一版可以把风险隔离开，但会多一个发版周期。

### 我判断不确定的地方（低置信度，需要实测）

- **裸提示词协调能力**：cumora 的五条原则是在 Claude Opus 级别模型上调出来的，ChatGo 用户可能跑 Ollama 本地小模型。小模型能不能执行"看清他叫的是谁""从已发布状态回复"这类需要一定推理的规则，**我不知道**，必须二期实测。如果不行，可能需要对小模型退化成"轮流发言"的简单编排——这是一个重要的后备方案。
- **去抖窗口 2.5s 是否合适**：这是 cumora 在服务端场景调出来的值。桌面交互对延迟更敏感，用户发完消息等 2.5 秒才看到 Agent 开始动，可能被感知为卡顿。可能要降到 800ms-1s。

---

## 7. 参考

- cumora 协调设计：`docs/COORDINATION.md`（11 层防御 + 完整反模式清单，**强烈建议完整读一遍**，反模式部分比机制部分更有价值）
- 协调提示词：`server/src/agents/glance-protocol.ts`（`GLANCE_YIELD_RULES` 五条规则原文 + 头部注释里的设计哲学）
- triage 门控：`server/src/agents/triage-core.ts`（小模型门控 + 循环地板的完整实现和大量设计注释）
- cumora 基准测试：`benchmarks/`（chain / counting / werewolf / kanban 四个多 Agent 协调基准）

---
---

# 附录 A：同类项目横向分析

> 调研日期 2026-10-27。**全部结论来自直读源码**，非文档或博客转述。

## A.0 四家实现的源头

| 项目 | 定位 | 读的文件 |
|------|------|---------|
| **SillyTavern** | 本地角色扮演客户端，多角色群聊 | `public/scripts/group-chats.js`（2491 行）、`public/scripts/openai.js` |
| **AutoGen**（微软） | 多 Agent 编排框架 | `autogen_agentchat/teams/_group_chat/_selector_group_chat.py` |
| **LangGraph Supervisor** | 图式多 Agent 编排 | `langgraph_supervisor/supervisor.py`、`agent_name.py` |
| **cumora** | 团队协作 IM，Agent 是一等成员 | 见正文 |

---

## A.1 SillyTavern：和 ChatGo 处境最接近的那一个

**为什么它最值得看**：本地桌面应用、单进程、JSON 文件存历史、用户自己掏 API 钱、多角色一个群——**这就是 ChatGo 的处境**。而且它是这四家里**唯一不靠 LLM 做调度决策**的。

### 四种激活策略（`group_activation_strategy`）

```js
export const group_activation_strategy = {
    NATURAL: 0,   // 提及检测 + 概率掷骰
    LIST: 1,      // 按成员列表顺序全员发言
    MANUAL: 2,    // 用户手动点，非用户输入时随机抽 1 个
    POOLED: 3,    // 池化轮转：优先没说过话的
};
```

**NATURAL（默认，最精巧）** —— `activateNaturalOrder()`：

```js
// 1. 禁止同一角色连续发言（除非 allowSelfResponses）
let bannedUser = !isUserInput && lastMessage && !lastMessage.is_user && lastMessage.name;
if (allowSelfResponses) bannedUser = undefined;

// 2. 提及检测：把输入分词，和角色名分词求交集 —— 纯字符串匹配，不调 LLM
for (let inputWord of extractAllWords(input)) {
    for (let member of members) {
        if (extractAllWords(character.name).includes(inputWord)) {
            activatedMembers.push(member); break;
        }
    }
}

// 3. talkativeness 概率掷骰（成员顺序先 shuffle）
const shuffledMembers = shuffle([...members]);
for (let member of shuffledMembers) {
    const rollValue = Math.random();
    const talkativeness = isNaN(character.talkativeness)
        ? talkativeness_default : Number(character.talkativeness);
    if (talkativeness >= rollValue) activatedMembers.push(member);   // ← 每个角色一个"话痨度"
    if (talkativeness > 0) chattyMembers.push(member);
}

// 4. 兜底：没人被激活就随机抽一个，优先从 talkativeness > 0 的池子里抽
const randomPool = chattyMembers.length > 0 ? chattyMembers : members;
while (activatedMembers.length === 0 && ++retries <= randomPool.length) { ... }

// 5. 去重 → 映射成角色 ID
activatedMembers = activatedMembers.filter(onlyUnique);
```

设计要点：
- **`talkativeness` 是 per-character 的 0..1 浮点数**，用户可调。概率激活让群聊有自然的参差感——话痨角色常出现，寡言角色偶尔冒头
- **先 shuffle 再掷骰**，避免列表靠前的角色系统性优先
- **分词匹配而非子串匹配**（`extractAllWords`），避免"Anna"误匹配"Annabelle"
- **五层兜底保证至少有一个人说话**，群聊永不卡死

**POOLED（公平性最好）** —— `activatePooledOrder()`：

```js
// 从最后往前扫，收集"自上次用户发言以来已经说过话的人"
for (const message of chat.slice().reverse()) {
    if (message.is_user || isUserInput) break;      // 遇到用户消息就停
    if (message.is_system) continue;
    if (message.original_avatar) spokenSinceUser.push(message.original_avatar);
}
const haveNotSpoken = members.filter(x => !spokenSinceUser.includes(x));
if (haveNotSpoken.length) {
    // 优先从没说过的人里随机抽 —— 保证一轮内每人至多一次
    activatedMember = haveNotSpoken[Math.floor(Math.random() * haveNotSpoken.length)];
}
if (activatedMember === null) {
    // 一轮走完，重开一轮，但排除刚说话的那个（避免连续）
    const randomPool = lastMessageAvatar ? members.filter(x => x !== lastMessage.original_avatar) : members;
    activatedMember = randomPool[Math.floor(Math.random() * randomPool.length)];
}
```

> **这段逻辑和 cumora 的"自缩放循环地板"是同一个洞察的两种用法**：都用"自上次人类发言以来谁说过话"做状态。cumora 用它**检测死循环**（有人说第二遍 = 套圈 = 停），SillyTavern 用它**保证公平轮转**（优先没说过的）。同一个信号，一个用于刹车，一个用于派发。ChatGo 可以两个都要。

### 串行执行，不并发

```js
// now the real generation begins: cycle through every activated character
for (const chId of activatedMembers) {
    throwIfAborted();
    setCharacterId(chId);
    setCharacterName(characters[chId].name);
    textResult = await Generate(generateType, { automatic_trigger: byAutoMode, ...params });
    //            ↑ await，一个说完再下一个
}
```

**激活的成员是严格串行逐个生成的**。后发言的角色能看到前面刚生成的内容——这产生了"接话"效果，而不是几个人同时抢话。代价是延迟累加。

它还有 `groupChatQueueOrder`（Map）在 UI 上显示排队序号，让用户知道还有谁要说话。**这个小细节对桌面应用的感知体验很重要**，ChatGo 应该抄。

### 身份注入：和我方案选的做法一致

```js
// openai.js setOpenAIMessages()
const character_names_behavior = {
    NONE: -1,        // 不加名字
    DEFAULT: 0,      // 群聊里给非用户消息加名字前缀
    COMPLETION: 1,   // 用 API 的 name 字段
    CONTENT: 2,      // 总是加 content 前缀
};

case character_names_behavior.DEFAULT:
    if ((selected_group && chat[j].name !== name1) || ...) {
        content = `${chat[j].name}: ${content}`;    // ← content 前缀
    }
```

SillyTavern 支持四种模式，但**群聊默认走 content 前缀**。这印证了我在正文 §4.3 的判断：`name` 字段跨 provider 兼容性不行，content 前缀是唯一通吃的办法。

另外两个细节值得注意：

```js
// 群聊里，只保留当前生成角色的 reasoning，其他成员的 reasoning 一律剥掉
const isOtherGroupMember = selected_group && chat[j].name !== name2;
const signature = isSameModel && !isOtherGroupMember ? chat[j]?.extra?.reasoning_signature : null;
const reasoning = isSameModel && !isOtherGroupMember ? String(chat[j]?.extra?.reasoning ?? '') : '';
```

**群聊里不要把别人的思维链喂给当前角色** —— 这是我方案里漏掉的点。ChatGo 的 `Message.Reasoning` 和 `Segments` 里的 `reasoning` 块，在构造群聊上下文时必须跳过非自己的。而且 `buildMessages()` 现在压根没传 reasoning，所以这条**现在是对的，但要写成注释防止以后有人"顺手补全"**。

```js
const default_group_nudge_prompt = '[Write the next reply only as {{char}}.]';
```

每次生成时追加一条**系统提示**明确当前该谁说话。比在 system prompt 里说"你是 X"更强，因为它在上下文末尾、紧贴生成点。ChatGo 值得抄。

### 两种人格组合模式

```js
export const group_generation_mode = {
    SWAP: 0,              // 整卡切换：只用当前角色的人设
    APPEND: 1,            // 拼接：把所有启用成员的人设拼在一起
    APPEND_DISABLED: 2,   // 拼接，连禁用成员也算
};
```

SWAP 下 `getGroupDepthPrompts()` 直接返回空数组。APPEND 模式下用 `createLazyFields()` **懒求值**——字段只在首次访问时才做模板替换和拼接，省掉不必要的字符串处理。

> **取舍**：SWAP 省 token、人格更纯；APPEND 让角色知道同伴是谁、能产生互动。ChatGo 的 `AgentDef.SystemPrompt` 对应 SWAP，想要 Agent 之间有互相认知就得往 APPEND 走（在 system prompt 里列出群成员名字和描述）。

### 自动模式

```js
export const DEFAULT_AUTO_MODE_DELAY = 5;   // 秒

function setAutoModeWorker() {
    clearInterval(autoModeWorker);
    const autoModeDelay = groups.find(x => x.id === selected_group)?.auto_mode_delay ?? DEFAULT_AUTO_MODE_DELAY;
    autoModeWorker = setInterval(groupChatAutoModeWorker, autoModeDelay * 1000);
}

async function groupChatAutoModeWorker() {
    if (!is_group_automode_enabled || online_status === 'no_connection') return;
    if (!selected_group || is_send_press || is_group_generating) return;   // ← 三重忙碌检查
    await generateGroupWrapper(true, 'auto', { signal: groupAutoModeAbortController.signal });
}
```

**无限循环，没有 cumora 那样的循环地板**。因为它的定位是娱乐——用户就是想看角色们一直聊。但 `is_group_generating` 这个布尔锁是必须的，否则定时器会叠加生成。

> 这是 SillyTavern 和 cumora 定位差异的直接体现：**娱乐场景不需要循环刹车，生产力场景必须要**。ChatGo 偏生产力，所以我方案里的循环地板不能省。

---

## A.2 AutoGen：LLM 选调度员

### 核心：`SelectorGroupChatManager.select_speaker()`

三级优先链：

```python
# 1. selector_func（用户自定义函数）—— 返回非 None 就直接用，跳过模型
if self._selector_func is not None:
    speaker = sync_selector_func(thread)
    if speaker is not None:
        if speaker not in self._participant_names:
            raise ValueError(...)
        return [speaker]          # ← 完全跳过模型选择

# 2. candidate_func（候选过滤）—— 缩小模型的选择范围
if self._candidate_func is not None:
    participants = sync_candidate_func(thread)
else:
    # 3. 默认：排除上一个发言人
    if self._previous_speaker is not None and not self._allow_repeated_speaker:
        participants = [p for p in self._participant_names if p != self._previous_speaker]
```

注意 `allow_repeated_speaker` 默认 `False`，**和 SillyTavern 的 `bannedUser` 是同一个机制**——两家独立做出了"默认禁止连续发言"的相同选择。

### 选调度员的提示词

```python
selector_prompt: str = """You are in a role play game. The following roles are available:
{roles}.
Read the following conversation. Then select the next role from {participants} to play. Only return the role.

{history}

Read the above conversation. Then select the next role from {participants} to play. Only return the role.
"""
```

**指令在历史前后各出现一次**。这是对抗长上下文中间遗忘（lost-in-the-middle）的常见手法。

`{roles}` 的构造特意压成单行：
```python
for topic_type, description in zip(self._participant_names, self._participant_descriptions, strict=True):
    roles += re.sub(r"\s+", " ", f"{topic_type}: {description}").strip() + "\n"
    #        ↑ 把所有空白压成单个空格，保证"每个 agent 一行"
```

`{history}` 的身份注入——**又是 `name: content` 前缀**：
```python
def construct_message_history(self, message_history: List[LLMMessage]) -> str:
    for msg in message_history:
        if isinstance(msg, UserMessage) or isinstance(msg, AssistantMessage):
            message = f"{msg.source}: {msg.content}"
            history_messages.append(message.rstrip() + "\n\n")
    return "\n".join(history_messages)
```

> **三家（SillyTavern / AutoGen / cumora）都用 `说话人: 内容` 的文本前缀**。这是跨项目的一致收敛，不是巧合。我方案 §4.3 选的方向被独立验证了三次。

### 重试与纠偏循环（最有工程价值的部分）

```python
num_attempts = 0
while num_attempts < max_attempts:           # max_selector_attempts 默认 3
    num_attempts += 1
    response = await self._model_client.create(messages=select_speaker_messages)
    select_speaker_messages.append(AssistantMessage(content=response.content, source="selector"))

    # 注意：用全部参与者名单查提及，而不是过滤后的候选 —— 为的是能抓到"模型选了上一个发言人"
    mentions = self._mentioned_agents(response.content, self._participant_names)

    if len(mentions) == 0:
        feedback = f"No valid name was mentioned. Please select from: {str(participants)}."
        select_speaker_messages.append(UserMessage(content=feedback, source="user"))
    elif len(mentions) > 1:
        feedback = f"Expected exactly one name to be mentioned. Please select only one from: {...}."
        select_speaker_messages.append(UserMessage(content=feedback, source="user"))
    else:
        agent_name = list(mentions.keys())[0]
        if not self._allow_repeated_speaker and agent_name == self._previous_speaker:
            feedback = f"Repeated speaker is not allowed, please select a different name from: {...}."
            select_speaker_messages.append(UserMessage(content=feedback, source="user"))
        else:
            return agent_name         # 有效选择

# 三次都失败 → 退回上一个发言人，再退回第一个参与者
if self._previous_speaker is not None:
    trace_logger.warning(f"Model failed to select a speaker after {max_attempts}, using the previous speaker.")
    return self._previous_speaker
return participants[0]
```

**这段代码的存在本身就是最重要的证据**：微软需要三轮重试 + 对话式纠偏 + 两级兜底，才能让 LLM 稳定地从几个名字里选出一个。`_select_speaker` 里有三条不同的 `trace_logger.debug` 分别记录"没提及有效名字""提到多个名字""选了上一个发言人"——**说明这三种失败模式都是实际高频发生的**。

提及检测的正则也透露了踩过的坑：
```python
regex = (r"(?<=\W)(" + re.escape(name) + r"|"
         + re.escape(name.replace("_", " ")) + r"|"      # Story_writer == Story writer
         + re.escape(name.replace("_", r"\_")) + r")(?=\W)")   # Story_writer == Story\_writer
count = len(re.findall(regex, f" {message_content} "))   # 前后补空格辅助边界匹配
```
要处理模型把下划线写成空格、或者 markdown 转义成 `\_` 的情况。

> **对 ChatGo 的直接启示**："用 LLM 选下一个发言人"这条路，代价是**每轮多一次 LLM 调用 + 最坏三次重试 + 还可能失败回退**。在用户自己付费的桌面应用里，这个性价比很差。SillyTavern 用纯概率/规则做同一件事，零额外调用。**我方案里不设中央调度员、让每个 Agent 自己决定是否发言（cumora 路线），避开了这整类问题**——代价是每个 Agent 各自判断也要花钱，但那笔钱同时产出了内容，不是纯调度开销。

---

## A.3 LangGraph Supervisor：工具调用式交接

### 不是群聊，是任务路由

`create_supervisor()` 建的是**星型图**：

```python
builder.add_node(supervisor_agent, destinations=tuple(agent_names) + (END,))
builder.add_edge(START, supervisor_agent.name)
for agent in agents:
    builder.add_node(agent.name, _make_call_agent(agent, output_mode, ...))
    builder.add_edge(agent.name, supervisor_agent.name)   # ← 每个 agent 做完必须回到 supervisor
```

每个子 Agent 执行完**强制回到 supervisor**。Agent 之间没有直接边，不存在 Agent 互相对话——**这是任务分派，不是群聊**。

交接通过**工具调用**实现：

```python
handoff_tools = [
    create_handoff_tool(
        agent_name=agent_name,
        name=(None if handoff_tool_prefix is None
              else f"{handoff_tool_prefix}{_normalize_agent_name(agent_name)}"),
        add_handoff_messages=add_handoff_messages,
    )
    for agent_name in agent_names
]
```

即"选择下一个 Agent"被建模成"调用 `transfer_to_<agent>` 工具"。比 AutoGen 让模型吐一个名字字符串更可靠——**工具调用有 schema 约束，模型不能吐出不存在的名字**。代价是要求模型支持 function calling。

```python
parallel_tool_calls: bool = False     # 默认禁止并行交接
MODELS_NO_PARALLEL_TOOL_CALLS = {"o3-mini", "o3", "o4-mini"}   # 这些模型不支持该参数
```

默认 `False` = 一次只能交接给一个 Agent。想要"同时派给多个 Agent"得显式开。

### 第三种身份注入方案：XML 标签

```python
def add_inline_agent_name(message: BaseMessage) -> BaseMessage:
    """
    >>> add_inline_agent_name(AIMessage(content="Hello", name="assistant"))
    AIMessage(content="<name>assistant</name><content>Hello</content>", name="assistant")
    """
    formatted_message.content = f"<name>{message.name}</name><content>{formatted_message.content}</content>"
```

而且**进出都转换**——送进模型前加标签，模型输出后用正则剥掉：

```python
NAME_PATTERN = re.compile(r"<name>(.*?)</name>", re.DOTALL)
CONTENT_PATTERN = re.compile(r"<content>(.*?)</content>", re.DOTALL)

chain = (process_input_messages | model | RunnableLambda(process_output_message, ...))
```

文档里明确说了为什么需要它：

> `None`: Relies on the LLM provider using the `name` attribute on the AI message. **Currently, only OpenAI supports this.**

**这是对"不能用 name 字段"的第三方权威确认**。LangChain 官方承认只有 OpenAI 支持 `name`，所以提供了 inline 方案。

> **XML 标签 vs `名字: 内容` 前缀**：XML 更结构化、剥离干净（能原样还原内容）；前缀更省 token、更自然。ChatGo 支持 9 种 provider 且不需要还原（历史直接存原文，前缀是构造上下文时临时加的），**`[#N 名字]: 内容` 的前缀方案更合适**。保留当前选择。

---

## A.4 四家对照

### 调度决策权归属——这是根本分歧

| 项目 | 谁决定下一个谁说话 | 每轮额外 LLM 成本 | 失败模式 |
|------|------------------|----------------|---------|
| **SillyTavern** | **代码**（提及匹配 + 概率掷骰） | **0** | 概率可能不符合语义预期 |
| **AutoGen** | **中央 LLM 调度员** | 1 次（最坏 3 次） | 选不出有效名字 → 重试 → 兜底回退 |
| **LangGraph** | **Supervisor 的工具调用** | 1 次（含在 supervisor 推理里） | 受限于 function calling 支持 |
| **cumora** | **每个 Agent 自己**（无中央调度） | N 次 triage（可跳过） | 并发竞态 → 服务端闸门兜底 |

**四家分成两派**：
- **中央派**（AutoGen / LangGraph）：有一个调度员决定谁说话。好处是可控、可追溯；坏处是调度员本身是成本和故障点，且天然串行
- **自治派**（SillyTavern / cumora）：没有调度员，成员自己被激活。好处是无调度开销、可并发；坏处是需要别的机制防冲突（ST 用串行执行 + 禁连续发言，cumora 用服务端闸门）

> **ChatGo 该选自治派**。理由：(1) 桌面应用用户自己付费，中央调度员那笔纯开销的钱不产出内容；(2) ChatGo 是单进程，自治派的并发冲突问题可以用"单 goroutine 串行写入"几乎免费解决；(3) 正文方案已经是这个路线，此处为独立验证。

### 身份注入——跨项目强收敛

| 项目 | 做法 |
|------|------|
| SillyTavern | `名字: 内容` 前缀（群聊默认）；另有 `name` 字段模式可选 |
| AutoGen | `名字: 内容` 前缀（`construct_message_history`） |
| LangGraph | `<name>X</name><content>Y</content>` XML 标签 |
| cumora | `convo #seq ▸YOU 作者(kind): 正文` 带标记的扁平文本 |

**四家全都把身份编码进 content，没有一家只靠 API 的 `name` 字段。** LangGraph 文档直接写明"只有 OpenAI 支持"。这件事可以当定论了。

### 防连续发言

| 项目 | 机制 | 默认 |
|------|------|------|
| SillyTavern | `bannedUser`（上一条非用户消息的作者） | 禁止，`allowSelfResponses` 可开 |
| AutoGen | `allow_repeated_speaker` + 重试纠偏 | `False`（禁止） |
| LangGraph | 无（supervisor 可以重复派给同一个） | 允许 |
| cumora | 不禁止，但"不要重复别人"写进提示词原则 | 允许 |

ST 和 AutoGen 独立做出了"默认禁止连续发言"的相同选择。**ChatGo 的方案里没有这条，应该补上**——作为可配置项，默认禁止。

### 循环终止

| 项目 | 机制 |
|------|------|
| SillyTavern | **无**（auto mode 无限循环，仅 `is_group_generating` 防叠加） |
| AutoGen | `max_turns` + `TerminationCondition`（如 `TextMentionTermination("TERMINATE")`） |
| LangGraph | 图的 `END` 节点 + supervisor 自行决定 |
| cumora | 小模型 triage + 自缩放循环地板 + 硬上限 20 + 速率地板 |

**cumora 是唯一把循环终止当一等问题的**，因为只有它是"Agent 之间可以长期自主对话"的场景。AutoGen/LangGraph 是任务型，任务做完自然结束。SillyTavern 是娱乐型，用户想让它一直聊。

> ChatGo 偏生产力又允许 Agent 互相对话，**处境最接近 cumora，循环地板必须要**。同时因为用户自己付费，还要加 cumora 没有的 token 预算可见性。

### 并发 vs 串行

| 项目 | 执行方式 |
|------|---------|
| SillyTavern | **严格串行** `for...await`，后者能看到前者刚说的 |
| AutoGen | **串行**（一次选一个说话人） |
| LangGraph | 默认串行，`parallel_tool_calls=True` 可并行 |
| cumora | **并发**，受 semaphore 限流，冲突靠闸门 |

> **这里我要修正正文方案的一个判断。** 正文 §4.5 设计的是"并发推理 + 闸门兜底"（cumora 路线），但四家里三家是串行，且 SillyTavern 的串行带来一个我之前没重视的好处：**后发言的 Agent 能看到前面刚说的内容，产生真正的"接话"而非"抢话"**。
>
> 对桌面场景，串行的实际优势：
> - 不需要新鲜度闸门（世界不会在推理期间变）
> - 不需要并发信号量和发车间隔
> - 不会出现"3 个 Agent 同时流式输出"的混乱 UI
> - 输出顺序确定，可复现
>
> 串行的代价是延迟累加（3 个 Agent 各 5 秒 = 15 秒）。但 SillyTavern 用 `groupChatQueueOrder` 在 UI 上显示排队序号，把等待变成可见的进度——**用户知道还有谁要说话，等待就不焦虑**。
>
> **建议把正文方案改成：默认串行，并发作为可选项。** 这能砍掉正文 §4.5 的 semaphore、发车间隔、新鲜度闸门三块复杂度（约占调度器一半的代码量），而且是四家里多数派的选择。详见下面的"方案修正"。

---

## A.5 对正文方案的修正

调研后要改三处、补三处。

### 修正 1：默认串行执行（重要）

正文 §4.5 的并发 + 闸门方案降级为可选。默认改为 SillyTavern 式串行：

```go
// 群聊默认串行生成：激活的成员逐个发言，后者能看到前者刚说的内容。
//
// 为什么默认串行而非并发（调研四家后的修正）：
//  - SillyTavern / AutoGen / LangGraph 三家都是串行，只有 cumora 并发
//  - cumora 必须并发是因为它是 N 个独立进程被同一个 SSE 事件唤醒，
//    并发是架构的既成事实，闸门是不得不加的补救
//  - ChatGo 是单进程，可以自由选择。串行后"推理期间世界变了"这个问题
//    消失，于是新鲜度闸门、并发信号量、发车间隔三块复杂度一起消失
//  - 串行还产生"接话"而非"抢话"的效果，对话质量更好
//
// 代价是延迟累加。用排队序号 UI 缓解（抄 SillyTavern 的 groupChatQueueOrder）。
for _, member := range activated {
    if ctx.Err() != nil { return }
    cw.generateFor(ctx, conv, member)   // 阻塞直到该成员说完
}
```

连带可以删掉的：`roomMaxConcurrent`、`roomMinSpawnInterval`、`lastSpawn`、新鲜度检查 + 重跑。**逐字重复检查保留**（串行下它变得更简单，就是比一下上一条）。

### 修正 2：激活策略改成可选的多种，默认走规则而非 LLM

正文只有"全部唤醒"和"triage 门控"两档，中间缺一大块。补成 SillyTavern 式的策略枚举：

```go
// RoomActivation 决定一轮里哪些成员发言。
type RoomActivation string

const (
    // ActivationAll 全员按列表顺序发言。适合"多视角评审"——
    // 就是要听到每个角色的意见。对应 SillyTavern 的 LIST。
    ActivationAll RoomActivation = "all"

    // ActivationNatural 提及检测 + 话痨度概率掷骰。零额外 LLM 调用。
    // 对应 SillyTavern 的 NATURAL，是它的默认策略。
    ActivationNatural RoomActivation = "natural"

    // ActivationPooled 优先让自上次人类发言以来没说过话的成员发言，
    // 保证一轮内每人至多一次。对应 SillyTavern 的 POOLED。
    ActivationPooled RoomActivation = "pooled"

    // ActivationManual 只有被 @ 的成员发言。
    ActivationManual RoomActivation = "manual"

    // ActivationTriage 每个成员各自用小模型判断是否该发言（cumora 路线）。
    // 最贵但最智能。默认不开。
    ActivationTriage RoomActivation = "triage"
)
```

**默认用 `ActivationAll`**（多视角评审是最可能的首要场景，也最符合用户直觉），`ActivationNatural` 作为"想要自然群聊感"的选项。

`RoomMember` 加一个字段：
```go
type RoomMember struct {
    ID    string `json:"id"`
    Name  string `json:"name,omitempty"`
    Muted bool   `json:"muted,omitempty"`

    // Talkativeness 是 0..1 的发言倾向，ActivationNatural 下按它掷骰。
    // 0 = 只在被 @ 时发言，1 = 每轮必发言。零值按 0.5 处理。
    // 抄 SillyTavern 的 talkativeness：让群聊有自然的参差感，
    // 而不是每个成员一样活跃。
    Talkativeness float32 `json:"talkativeness,omitempty"`
}
```

### 修正 3：补防连续发言

```go
// bannedAuthor 是本轮不允许发言的成员：上一条消息的作者（当它不是人类时）。
//
// 抄 SillyTavern 的 bannedUser + AutoGen 的 allow_repeated_speaker。
// 两家独立做出了"默认禁止连续发言"的相同选择，默认跟上。
// AllowSelfResponses 可以放开（续写、自言自语场景需要）。
func bannedAuthor(msgs []models.Message, allowSelf bool) string {
    if allowSelf || len(msgs) == 0 { return "" }
    last := msgs[len(msgs)-1]
    if last.AuthorKind == "human" { return "" }
    return last.AuthorID
}
```

### 补充 1：排队序号 UI

抄 `groupChatQueueOrder`。串行下用户必须知道"还有谁要说话"，否则 15 秒的等待会被当成卡死。在成员头像上显示 `1` `2` `3`。

**这是串行方案能不能被用户接受的关键**，不是可选装饰。

### 补充 2：构造上下文时跳过别人的 reasoning

正文漏了。SillyTavern 明确做了这件事：

```go
// buildRoomMessages 里：只保留视角 Agent 自己的 reasoning，
// 其他成员的思维链一律不带。
//
// 抄 SillyTavern 的 isOtherGroupMember 判断。把别人的思维链喂进来，
// 一是浪费 token，二是会让当前 Agent 把别人的推理当成自己的。
//
// 现状：buildMessages() 压根没传 reasoning，所以当前行为是对的。
// 这条注释是防止以后有人"顺手补全 reasoning"时引入 bug。
```

### 补充 3：group nudge

抄 `default_group_nudge_prompt = '[Write the next reply only as {{char}}.]'`：

```go
// roomNudge 追加在上下文末尾，明确当前轮该谁说话。
//
// 比只在 system prompt 里说"你是 X"更强：它紧贴生成点，
// 不会被长历史冲淡。抄 SillyTavern 的 group_nudge_prompt。
func roomNudge(name string) llm.ChatMessage {
    return llm.ChatMessage{
        Role:    "system",
        Content: fmt.Sprintf("[现在只以 %s 的身份发言。如果这轮不该你说话，回复空内容。]", name),
    }
}
```

后半句给了"选择沉默"一个明确的表达方式——正文 §4.4 的第 5 条原则需要一个落地的出口。

---

## A.6 实施路径调整

修正后二期大幅简化，原三期的大半消失：

**二期（最小可用群聊）** —— 加 `ActivationAll` + 串行执行 + 排队序号 UI
- 删掉原计划的 semaphore、发车间隔
- 加 `bannedAuthor`、`roomNudge`、跳过他人 reasoning

**三期（激活策略 + 刹车）** —— 原三期的闸门部分大幅缩减
- `ActivationNatural`（提及匹配 + talkativeness）、`ActivationPooled`
- 逐字重复检查（串行下很简单）
- 循环地板 + 硬上限
- **删掉**新鲜度闸门 + 重跑（串行下不需要）

**四期** —— 并发模式（选配，此时才需要闸门那一套）、`ActivationTriage`、@ 提及、IM 通道、ACP

> 净效果：**二三期的工作量都减少**，而且默认路径（串行 + 全员发言）是四家里被验证最多的组合。cumora 那套并发闸门降级为四期的可选项——它是为分布式架构设计的，ChatGo 用不着先付那个复杂度。

---

## A.7 一句话总结各家

- **SillyTavern** —— 和 ChatGo 处境完全一致（本地、单进程、JSON、用户付费），**唯一零 LLM 调度开销**的实现。概率激活 + 串行执行 + content 前缀注入。**最该抄的一家。**
- **AutoGen** —— 中央 LLM 调度员。它的三轮重试 + 对话式纠偏 + 两级兜底，恰恰证明了这条路不稳定。桌面场景性价比差。
- **LangGraph Supervisor** —— 工具调用式交接，比吐名字字符串可靠（有 schema 约束）。但它是任务路由不是群聊，Agent 之间不能直接对话。其 `agent_name.py` 给了"不能依赖 API `name` 字段"的第三方权威确认。
- **cumora** —— 唯一真正的"Agent 自治群聊"，协调原则和循环终止机制最成熟，但一半机制是为分布式架构付的税，ChatGo 单进程不该抄那部分。

**最终取法**：**骨架抄 SillyTavern**（串行、规则激活、talkativeness、排队 UI、group nudge），**提示词原则和循环刹车抄 cumora**（五条协调原则、自缩放地板），**身份注入用四家收敛的 content 前缀**，**AutoGen/LangGraph 的中央调度路线整体不采纳**。

---
---

# 附录 B：Raft（raft.build）— cumora 的直接同形竞品

> 调研日期 2026-10-27。来源：官网、`docs.raft.build`、博客 *"Is Having Agents in the Room Meant to Be Chaotic?"*（2026-05-21，作者 Tenny，职位写的是 **AX Designer**），以及 **source-available 源码** `github.com/botiverse/raft-source`（clone 成功，5830 文件）。
> 结论基于直读源码，关键文件：`packages/server/src/routes/internalAgentApi.ts`（9641 行）、`packages/server/src/services/attestedSendService.ts`、`packages/cli/src/commands/message/send.ts`（1344 行）。

## B.0 为什么它值得单独一节

Raft 和 cumora 是**同一产品形态**：人类与 Agent 同处一个 IM 式工作区，Agent 是一等成员，有持久身份和记忆，自己决定是否发言，跑在本地的 Claude Code / Codex 上（Raft 叫 "Computer"，cumora 叫 "BYOA"）。

但最惊人的是**两家独立收敛到了同一个核心机制**，连验证基准都一样。

Raft 博客开头：

> Ask a room full of agents to count upward, one number per agent, no duplicates. In most workspaces, it breaks almost immediately. Three agents post "1" at the same second. Two more post "2". By the time the room reaches "4", three agents have already said it. **The agents are not broken. The room is.**

cumora 的 `COORDINATION.md` 里，计数游戏是贯穿全文的主基准（"Iris 和 Marcus 同时发 `3`"）。**两个团队各自独立地用"多 Agent 数数"作为协调能力的 hello world，并且都做出了"发言前检查新鲜度、不通过就 HOLD"的服务端闸门。** 这件事比任何单个实现都更有说服力——它说明这个问题和这个解法都是结构性的。

## B.1 "AX"：Raft 提出的概念框架

这是 Raft 相对 cumora 的**独特贡献**。cumora 的文档是工程日志（11 层防御 + 反模式），Raft 把同类问题抽象成了一门学科：

> **Agent Experience design (AX)**: the same kind of discipline UX has been for humans, but designed for **how agents actually see and act**.

AX 对每个 Agent 接触的界面问四个问题：
1. **什么是 Agent 在行动那一刻看到的**（what does the agent see at the moment of action）
2. **它在两次调用之间携带什么状态**（what state does it carry between invocations）
3. **它能从什么里恢复**（what can it recover from）
4. **它被允许决定什么**（what is it allowed to decide）

### 根因诊断：连续感知 vs 回合制

Raft 对"为什么群聊会乱"给了比 cumora 更清晰的理论表述：

> Humans coordinate gracefully in shared spaces because we have **continuous perception**. We sense the rhythm of a conversation without consciously reading every message; we feel the pause before stepping in... **None of that has to be designed. It is what being continuously present means.**
>
> Agents don't inhabit the room the way humans do. Their interaction is **turn-based**: each invocation, the agent reads a snapshot of the room, reasons, commits an action, and then waits for the next invocation. **Nothing runs in between.**

> **所有共享房间里的失败模式都来自同一个缺口**：为"连续在场者"设计的工作区，没有给"回合制在场者"提供任何表面（surfaces）。

这个表述比 cumora 的"race collision vs brain misjudgment"二分更本质——**它解释了为什么竞态和误判都会发生：都是因为 Agent 在"读快照"和"提交动作"之间有一段盲区，而房间不为这段盲区负责。**

### 对"@mention 门控"的批评——这条最该引起注意

> Many teams add rules to make this manageable, like only letting an agent respond when **@mentioned**. The room feels organized, but the agents stop catching what they would have caught: a misrouted instruction, a wrong assumption, the conversation drifting somewhere they could have helped.
>
> **Rules-based filtering doesn't reduce noise; it turns the agent back into a tool waiting to be invoked.**

> **这直接否定了我正文方案里的 `ActivationManual`（只有被 @ 的成员发言）作为主力策略的价值。** Raft 的论点是：@门控确实让房间安静了，但它把 Agent 从"同事"降级回"工具"——一个只能被 @ 唤醒的 Agent，再也不能主动发现跑偏的指令或错误假设。
>
> 这个批评我认为**成立但有适用边界**。Raft 和 cumora 都是团队协作生产力场景，Agent 的"主动发现价值"很高。ChatGo 是单人桌面工具，用户往往明确知道自己要问谁。所以：
> - `ActivationManual` 在 ChatGo 里**仍应保留**，它对应"我只想问架构师"这个合法且高频的意图
> - 但**不应该是默认值**，否则就是 Raft 批评的"把 Agent 变回工具"
> - 正文 §A.5 修正 2 定的默认是 `ActivationAll`，**这个选择经此验证是对的**

## B.2 机制一：Agent Inbox（拉取式注意力）

这是 Raft 和 cumora 差异最大的一处，也是**ChatGo 最该学的设计思想**。

### 问题

> In traditional messaging platforms, an agent that joins a channel typically gets **every** message in that channel pushed at it. The options that follow are not great: process everything (and watch the working context fill with chatter that has nothing to do with the task at hand) or filter aggressively (and miss the message that actually mattered). **Either way, the room is in charge of the agent's attention, not the agent.**

### Raft 的解法：反转推拉方向

> Raft inverts this with the **inbox**. Mentions, thread updates, and other notifications surface as **queryable items the agent can pull when it has bandwidth**, rather than being pushed straight into the working context. The agent checks what is new, decides what bears on the current task, ingests what is worth ingesting. **Signals it doesn't pull don't enter the working context; they stay queryable for when they're needed later.**
>
> **The agent decides what is worth its context**, instead of the room deciding for it. Every signal pulled into the working prompt **displaces something else** (task state, instructions, intermediate reasoning).

**和 cumora 的对比**：cumora 是服务端用小模型 triage **替 Agent 决定**该不该醒（`actionable: true/false`）；Raft 是把 inbox 做成**可查询资源**，让 Agent 自己决定拉什么进上下文。

| | cumora | Raft |
|---|---|---|
| 谁决定什么进上下文 | 服务端小模型（triage gate） | **Agent 自己**（拉取 inbox） |
| 未被选中的消息 | 丢弃（不唤醒大脑） | **仍可查询**，留着以后用 |
| 成本 | 每次唤醒一次小模型调用 | 0 额外模型调用（但 Agent 要花 token 读 inbox 列表） |
| 理由 | 省大模型的钱 | 上下文是零和的，谁的上下文谁做主 |

### 源码证据

`attestedSendService.ts` 里有一张 `inboxNotificationFacts` 表，带 `receiverType` / `receiverId` / `sourceChannelId` / `messageSeq` / `personalMention` 字段：

```ts
function agentAttentionConditions(agentId, channelId, seq, options) {
  const conditions = [
    eq(inboxNotificationFacts.receiverType, "agent" as const),
    eq(inboxNotificationFacts.receiverId, agentId),
    eq(inboxNotificationFacts.sourceChannelId, channelId),
    gt(inboxNotificationFacts.messageSeq, seq),
  ];
  // ...
}
```

**注意力是一张持久化的事实表，不是运行时的推送流。** 这是"可查询"的实现基础。

## B.3 机制二：Held Draft（比 cumora 更完整的 HOLD）

### 四条路径——这是 Raft 的核心创新

cumora 的 HELD 只给 Agent 两个现实选择：重算后再发，或者放弃。Raft 把它做成了**显式的四路径状态机**：

> From there, the agent picks one of four paths:
> - **Revise**: write a new reply against the current room, abandoning the original draft.
> - **Send as-is**: commit the original draft unchanged. The send still goes through the freshness check; if the room moved further during the hold, the draft can be held again.
> - **Stay silent**: let the draft expire. **Silence is a valid outcome.**
> - **Send anyway**: after the hold has triggered **repeatedly** and silence isn't the right outcome, explicitly bypass the check.

关键区别：**草稿是一等状态**（"The draft is preserved as a first-class state, not a failed send"）。cumora 的 HELD 是把内容退回给 Agent，让它自己重新组织；Raft 把草稿存下来，Agent 可以原样重发而不用重新生成——**省掉一次完整推理**。

源码里草稿有 TTL 和一致性键：

```ts
// send.ts validateDraftSendFlags()
if (opts.discardDraft && opts.sendDraft) throw ...  // 互斥
if (opts.anyway && !opts.sendDraft) throw new SendContentError(
  "SEND_DRAFT_ANYWAY_REQUIRES_SEND_DRAFT",
  "--anyway can only be used together with --send-draft.",
);
```

**`--anyway` 必须和 `--send-draft` 同时用** —— 这和 cumora 的 hold-token 门控是同一个意图的不同实现：cumora 用 Redis token 证明"服务端真的给你看过 HOLD"，Raft 用"必须配合 `--send-draft`"证明"你在覆盖一个真实存在的被 HOLD 草稿"，**预防性传 `--anyway` 在语法层就被拒绝**。比 cumora 的 token 方案更简单且无状态。

### `continueAnywaySuggested`：系统主动建议放行

```ts
// internalAgentApi.ts:1753
...(!withhold && action === "send"
  ? { continueAnywaySuggested: reholdCount >= 3 }
  : {}),
```

**被 HOLD 三次后，服务端主动在响应里建议"可以用 --anyway 了"。** 这解决了一个 cumora 文档里没解决的问题：房间持续高频活动时，Agent 可能永远发不出去。Raft 用 `reholdCount` 计数，到 3 次就告诉 Agent"这个闸门你已经尽力了"。

CLI 的提示文案也相应分层：

```
To update the draft, send revised content normally: ...
To send the current draft unchanged (within N minutes): raft message send --send-draft ...
You can also choose not to send anything.                    ← 沉默是明确选项
---- 仅当 reholdCount >= 3 才追加 ----
If repeated updates keep blocking the same draft and this is still the right reply, you may use:
  raft message send --send-draft --anyway ...
```

### "Action Explicitness"：这是全文最深的一句

> Here is where AX diverges most sharply from UX. A human composing a reply does not need a UI labeled "decide whether to send" or "abandon this draft and start over." Those decisions happen internally, fluidly, and the software only has to support the actions a human **externalizes**.
>
> **Agents need those internal options made external.** The four paths after a held draft are not options the agent generates from nowhere; they are options **AX explicitly puts in front of the agent**. Action explicitness means **surfacing the option-space, not assuming the agent will derive it.**

> **这解释了为什么 cumora 的"五条提示词原则"有天花板，而 Raft 选择了另一条路。** cumora 试图用提示词让 Agent *推导出*正确行为（"不要重复别人""有人缺席就接下一项"）；Raft 认为 Agent 不会自己推导出选项空间，**必须把选项摆在它面前**。
>
> 这两者不矛盾，是互补的：cumora 的原则告诉 Agent *该怎么判断*，Raft 的显式选项告诉 Agent *有哪些动作可选*。**ChatGo 应该两个都要**——而我正文方案只有前者，缺后者。

## B.4 机制三：attention facts —— 比 cumora 更精细的闸门

这是源码里发现的、博客没提的设计，**也是 Raft 技术上比 cumora 细的地方**。

cumora 的新鲜度预检查的是"会话里有没有比我 cursor 更新的**他人消息**"：

```sql
-- cumora
SELECT * FROM messages WHERE conversation_id=$1 AND author_id<>$2 AND sequence>$3
```

Raft 的闸门检查的是"有没有比我 boundary 更新的**我收到过通知的消息**"：

```ts
// internalAgentApi.ts evaluateAgentFreshness()
const useAttentionFacts = channelType !== "thread";
const latestAnchor = useAttentionFacts
  ? await attestedSendService.getLatestAgentAttentionAnchor(agentId, channelId, { excludeSender: selfSender })
  : await attestedSendService.getLatestMessageAnchor(channelId, { excludeSender: selfSender });
```

差别在于：**一条消息如果根本不该引起这个 Agent 的注意（它没在 inbox 里），就不会 HOLD 它的发言。** 这避免了 cumora 里那个被明确记录的痛点——"繁忙房间里每次首发都必然被 HOLD 一次"（cumora 的 compose-anchor 就是因为这个被废弃的）。

Raft 还支持 `seenExactSeqs`——Agent 可以精确声明"这几条我看过了"，而不只是一个 boundary：

```ts
const seenExactSeqs = [...new Set((requestedSeenExactSeqs ?? [])
  .map((seq) => Math.floor(Number(seq)))
  .filter((seq) => Number.isInteger(seq) && seq > attestedBoundarySeq && seq <= latestFreshnessSeq))]
  .slice(0, 2_500);
```

这让"我看过 #5 和 #7，但没看过 #6"这种非连续状态可以表达。cumora 只有单调 cursor，表达不了。

另外一个注释值得注意（对"什么能证明模型真的看过"的严格态度）：

```ts
// Freshness proof must come from the current model-seen boundary.
// Durable legacy read-ish cursors cannot prove the model actually saw
// the messages; delivery ack state is volatile inbox state. The only
// fallback is a cold thread's parent message.
```

**"投递确认"不等于"模型看过"** —— 这和 cumora 踩过的 `conversation_reads.last_read_at` 坑是同一类认知。

### 闸门覆盖的不只是发言

```ts
// Glimpse G2: the freshness gate on a task write (claim, status update, amend),
// by the same check and hold as a send.
```

任务认领/状态更新/修改也走同一个闸门，且有 `TASK_WRITE_REHOLD_WINDOW_MS` 窗口内"同一个写操作只 HOLD 一次"的规则（否则繁忙看板会 HOLD 掉每次重试）。

## B.5 Raft 的其他设计

### 角色层级（cumora 没有）

官网的团队图：

```
Alex (Founder, human)
  └── Tenny (Team leader, Fable · Claude Code)
        ├── XX (Fullstack Engineer, Fable · Claude Code)
        ├── Martin (Quality Assurance, GPT-6 · Codex)
        ├── Bugen (Design Engineer, Fable · Claude Code)
        └── Box (Agent Resources, Fable · Claude Code)
```

**每个 Agent 标注了自己的 model + runtime，而且是混搭的**（有的跑 Claude Code，QA 跑 Codex）。还有一个专职的 "Agent Resources (AR)" 角色管理团队本身——官网引用 TiDB CTO Ed Huang 的话："I'm not coding anymore — I'm the AR, Agent Resource Manager."

> ChatGo 的 `AgentDef` 已经支持 per-agent 的 `Models`，**天然支持这种混搭**。但没有层级概念。层级对 ChatGo 的价值存疑——单人桌面工具里"Agent 领导 Agent"的场景不清晰，**不建议抄**。

### 官网演示里的"会审"流程

官网 release-review 那段演示值得注意：

```
Tenny(leader): Can we ship the new settings page?
XX(fullstack): Ready.
Martin(QA): 🔴 Hold. The required E2E gate was skipped on this commit.
Bugen(design): The primary action is visually buried.
Tenny: Successor blockers: executed same-commit CI evidence and design sign-off.
XX: Fixed. [CI evidence · exact commit 8f3a2c]
Martin: Exact commit passed all required checks.
Bugen: Looks right.
Tenny: Team practice updated: every release needs same-commit CI evidence and design sign-off.
```

**这正是我在正文里问你的"多视角评审"场景**，而且是 Raft 官网自己选来当主打演示的。注意 QA 角色说的是 `🔴 Hold`——**不同角色有不同的"否决权"语义**。

### Constructed Initial Context（另一篇博客）

> How do you keep a long-running AI agent continuous after its prompt cache expires? Raft **rebuilds the agent's starting context** and cuts cold-wake cost by **60%**.

从 memory / messages / files / actions 重建一个紧凑的首轮输入，而不是重放完整历史。

> **这直接关联我正文 §6 列的"上下文膨胀"风险。** ChatGo 群聊里 N 个 Agent 各自一份带前缀的完整历史，增长曲线会很陡。Raft 的做法（重建而非重放）是一个明确的解法方向，二期实测增长曲线后如果确认是问题，照这个方向做。

## B.6 五家对照更新

| 维度 | SillyTavern | AutoGen | LangGraph | cumora | **Raft** |
|---|---|---|---|---|---|
| 调度决策权 | 代码（概率） | 中央 LLM | Supervisor 工具 | Agent 自治 | **Agent 自治** |
| 注意力方向 | 推（全量） | 推 | 推 | 推 + 小模型 triage 过滤 | **拉（inbox 可查询）** |
| 冲突处理 | 串行避免 | 串行避免 | 串行避免 | 服务端 HOLD | **服务端 HOLD + 一等草稿** |
| HOLD 后的选项 | — | — | — | 隐式（重算/放弃） | **显式四路径** |
| 闸门粒度 | — | — | — | 全部他人消息 | **仅该 Agent 的 attention facts** |
| 覆盖标志的代价 | — | — | — | Redis hold-token | **语法强制配 `--send-draft`** |
| 持续被挡的出路 | — | — | — | 无（文档未解决） | **`continueAnywaySuggested` @ rehold≥3** |
| 沉默是否一等选项 | 否 | 否 | 否 | 提示词层面 | **是（草稿过期即沉默）** |

**Raft 在"Agent 自治群聊"这条路上做得比 cumora 更完整。** cumora 的优势是把踩坑过程和反模式写得极透（`COORDINATION.md` 的反模式部分无可替代），Raft 的优势是机制设计更精细、概念框架更清晰。

## B.7 对 ChatGo 方案的新增修正

调研 Raft 后，正文 + 附录 A 还要补三处。

### 修正 4：给"沉默"一个一等的表达方式

正文 §4.4 第 5 条原则说"不需要发言时保持沉默就是正确的"，附录 A 的 `roomNudge` 里加了"如果这轮不该你说话，回复空内容"。但这不够——**"回复空内容"是让 Agent 从无到有地推导出一个动作，正是 Raft 批评的"假设 Agent 会自己推导选项空间"。**

应该把选项显式摆出来：

```go
// roomNudge 追加在上下文末尾，明确当前轮该谁说话，并显式列出可选动作。
//
// 为什么要列出动作（Raft 的 "action explicitness"）：
// 人类写回复时不需要一个叫"决定是否发送"的 UI，这些决定在内心流畅地发生。
// Agent 不会自己推导出选项空间 —— 必须把它摆在面前。只说"可以保持沉默"
// 不如给出一个明确的、可执行的表达形式。
func roomNudge(name string) llm.ChatMessage {
    return llm.ChatMessage{
        Role: "system",
        Content: fmt.Sprintf(
            "[现在轮到 %s 发言。你有三个选择：\n"+
            "1. 正常回复 —— 直接输出你要说的话\n"+
            "2. 保持沉默 —— 只输出 PASS 三个字母，不要有其他内容\n"+
            "3. 简短示意 —— 如果只是想表示看到了/同意，输出一个 emoji 即可\n"+
            "不需要你发言时，选 2 是完全正确的，不要为了参与感而说话。]",
            name,
        ),
    }
}
```

然后调度器识别 `PASS` 并丢弃该消息，UI 上把该成员头像标成灰点（正文 §4.8 的"沉默可见性"）。

> **为什么用 `PASS` 而不是空字符串**：空输出在流式场景下和"模型还没开始输出"无法区分，而且很多 provider 对空 completion 的行为不一致。一个明确的哨兵 token 更可靠。

### 修正 5：串行 + 乐观发言并存时，闸门要按"attention"而非"全部消息"

正文 §A.5 修正 1 把默认改成串行，并说"串行后新鲜度闸门不需要了"。**这个判断对默认路径成立，但四期的并发模式仍需要闸门，届时要按 Raft 的粒度做**：

```go
// 并发模式（四期选配）下的新鲜度检查：只检查"该成员本应注意到的消息"，
// 不是全部他人消息。
//
// 为什么（Raft 的 attention facts 粒度）：按"全部他人消息"检查会导致
// 繁忙房间里每次首发都必然被挡一次 —— cumora 的 compose-anchor 正是
// 因为这个问题被废弃（它保证了"任何繁忙房间里的第一次尝试必然 HOLD"，
// 每次回复要多付 1-2 次大模型往返）。
//
// ChatGo 的判定（无 inbox 表，从消息本身推导）：
//   - 人类发的消息 → 所有成员都应注意
//   - @ 了该成员的消息 → 该成员应注意
//   - 其他 Agent 的普通发言 → 不触发该成员的 HOLD
func shouldHold(member RoomMember, newMsgs []models.Message) bool {
    for _, m := range newMsgs {
        if m.AuthorKind == "human" { return true }
        if mentions(m.Content, member.Name) { return true }
    }
    return false
}
```

### 修正 6：被挡多次后要有出路

```go
// roomHoldLimit 是同一条草稿被挡的次数上限。超过后放行。
//
// 抄 Raft 的 continueAnywaySuggested（reholdCount >= 3）。
// 解决一个 cumora 文档里没解决的问题：房间持续高频活动时，
// Agent 可能永远发不出去。挡三次说明它已经尽力了。
//
// ChatGo 的差异：Raft 是建议 Agent 自己传 --anyway（它有 CLI 动作面），
// ChatGo 的 Agent 不通过 CLI 发言，所以由调度器直接放行，不走 Agent 决策。
const roomHoldLimit = 3
```

### 不采纳 Raft 的部分及理由

| Raft 机制 | 不采纳理由 |
|---|---|
| 拉取式 inbox | ChatGo 的 Agent 没有工具调用面去"查询 inbox"。要做得先给每个成员一套房间查询工具，是四期之后的事。**但"上下文是零和的，谁的上下文谁做主"这个思想要记住** |
| 一等草稿 + TTL | 串行模式下房间不会在推理期间变，草稿没有存在价值。四期并发模式再议 |
| 角色层级 / AR 角色 | 单人桌面工具里"Agent 领导 Agent"场景不清晰 |
| `seenExactSeqs` 非连续已读 | 过度精细，ChatGo 串行下连 cursor 都几乎不需要 |
| 任务看板 + 认领 | 正文已判断 ChatGo 没有共享交付物，不需要认领 |

## B.8 一句话总结

**Raft = cumora 的同形竞品，但机制更精细、理论框架更清晰。**

它最大的贡献是提出 **AX（Agent Experience design）** 这个框架，和其中两条可直接落地的原则：

1. **Perception empathy（感知共情）** —— 坐到 Agent 的位置上看房间：它在行动那一刻**实际**看到什么？人类在同一个房间里不费力就能注意到、而 Agent 没有自动访问权的是什么？那个缺口就是要设计的地方。
2. **Action explicitness（动作显式化）** —— Agent 不会自己推导出选项空间，必须把选项摆在它面前。人类不需要一个"决定是否发送"的 UI，Agent 需要。

**第 2 条是我正文方案最明显的缺口**：我写了五条判断原则（告诉 Agent 怎么判断），但没有给出动作空间（告诉 Agent 有哪些选择）。修正 4 补上了这一点。

另外两个直接可抄的具体机制：
- **`continueAnywaySuggested` @ rehold≥3** —— 被挡多次后的出路，cumora 没有
- **覆盖标志用语法强制而非 token** —— `--anyway` 必须配 `--send-draft`，比 cumora 的 Redis token 方案更简单且无状态

最后，Raft 对 **@mention 门控的批评**（"Rules-based filtering doesn't reduce noise; it turns the agent back into a tool waiting to be invoked"）验证了附录 A 修正 2 的选择：`ActivationManual` 保留但不作默认。
