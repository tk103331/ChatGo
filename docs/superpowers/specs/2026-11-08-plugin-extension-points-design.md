# ChatGo 插件扩展点详设（事件 / 命令 / 上下文 / 导入 / 触发器 / 渠道 / 渲染 / App）

- 状态：设计草案，待评审
- 日期：2026-11-08
- 上游文档：`2026-11-08-plugin-system-design.md`（运行时、清单、合并、安全、Provider/Middleware）

本文覆盖 8 个扩展点。每个扩展点都沿用上游文档的三条基本约定：**清单声明贡献、运行时注入现有结构、禁用即回收**。

## 0. 共同约定

| 项 | 约定 |
|---|---|
| 命名空间 | 工具 `<pluginId>__<name>`；命令 `/<pluginId>-<name>`；上下文项 `plugin:<id>:<provider>:<item>`；App `app:<pluginId>/<appId>` |
| 超时 | 事件 3s；斜杠命令 5s；上下文搜索 3s、取正文 5s；导入解析 5s；触发器求值 1s；渲染校验 50ms |
| 失败策略 | 默认 fail-open：超时/抛错跳过该次调用并记一条警告，不阻断对话、任务或启动。例外：任务触发器求值失败为 **fail-closed**（暂停任务，见 5.4） |
| 并发 | 每个插件一个串行执行队列（容量 1024）；同一插件的多个钩子依次执行，绝不在 JS 运行时内重入 |
| 权限 | 新增 `events`（只读旁路）、`context`（只读外部数据）、`render`（无额外权限）；其余沿用 `http:<host>` / `env:<KEY>` / `exec` / `store` |
| 来源 | 所有贡献在 UI 带插件徽标，各自页面只读，统一在插件页管理 |
| 版本 | 清单 `engines.chatgo` 声明最低版本，贡献类型不被旧版本识别时安装期直接报错 |

## 1. 通用事件钩子

### 1.1 目标

让插件旁路观察应用内发生的事：任务完成通知、审计日志、外部系统同步。事件**只读**，不改变主流程；要改行为请用工具或模型中间件。

### 1.2 清单与 JS

```json
{
  "permissions": ["events"],
  "events": ["turn.completed", "turn.failed", "tool.afterCall", "task.finished"]
}
```

```js
export const events = {
  async onEvent(name, payload, ctx) {
    if (name === "turn.failed") {
      await ctx.http.post("https://hooks.example.com/alert", { json: payload });
    }
  },
};
```

### 1.3 事件表

| 事件 | 触发点（现有代码） | 主要字段 |
|---|---|---|
| `plugin.enabled` / `plugin.disabled` | `internal/plugin/manager.go`（新增） | `pluginId` |
| `conversation.created` / `conversation.deleted` | `internal/ui/conversation.go`、`settings.go` `askDeleteConversation` | `conversationId`, `agent`, `workspace` |
| `message.received` | `internal/ui/conversation.go:126` `sendDraft()` | `conversationId`, `text`（截断） |
| `turn.started` | 同 `sendDraft()` 之后 | `turnId`, `provider`, `model` |
| `turn.completed` | `internal/ui/conversation.go:260` `runStream()` 收尾 | `reply`（截断）, `promptTokens`, `completionTokens`, `durationMs` |
| `turn.failed` | 同 `runStream()` 的错误分支 | `error` |
| `tool.afterCall` | `internal/llm/react_agent.go:154` `ObserveTool()` 回调 | `name`, `arguments`, `result`（截断）, `durationMs`, `error` |
| `plan.updated` | `internal/ui/builtin_tools.go:391` `updatePlanTool()` | `entries[]` |
| `task.started` / `task.finished` | `internal/ui/autotask.go:588` `startAutoTask()` | `taskId`, `name`, `ok`, `note` |
| `channel.message.received` | `internal/ui/channelmanager.go:150` `onChannelInbound()` | `channelId`, `chatId`, `fromUser`, `text`（截断） |
| `channel.reply.sent` | `internal/ui/channelmanager.go:193` `sendChannelReply()` | `channelId`, `chatId`, `reply`（截断） |
| `app.opened` | `internal/ui/apps.go:120` `openApp()` | `appId` |
| `knowledge.imported` | `internal/ui/knowledge_actions.go` 导入完成处 | `kbId`, `documents`, `chunks` |

### 1.4 投递语义

- 事件在宿主侧入队（每插件一条 FIFO 队列，容量 1024），单 worker 串行投递；队列满丢最旧一条并计数。
- **at-most-once**；进程退出不补投。
- 字段级截断 4 KiB、整包 64 KiB，超出部分替换为 `…(truncated)`。
- `ctx` 提供：`pluginId`、`event`、`settings`、`store`、`log`，以及按授权提供的 `http`。
- 事件派发在独立 goroutine，禁止阻塞 UI 线程；`onEvent` 返回值被忽略。

### 1.5 安全与验收

- 事件能读到对话内容与工具参数：安装确认页明确列为“可读取对话内容”。
- 禁用插件即停止投递并清空队列。
- 验收：示例插件订阅 `turn.completed` 后能在 1s 内收到事件；事件处理抛错/超时不影响回复落库；队列打满时只告警不阻塞。


## 2. 斜杠命令

### 2.1 现状

`internal/ui/composer_menu.go:19` `chatMenus()` 已经合并两类命令来源：ACP 会话命令（`:39`）与 Skill 命令（`:63`），菜单组件是 `internal/ui/kit/menu.go:33` `SlashCommandMenu`。插件命令是第三类来源。

### 2.2 清单与 JS

```json
"commands": [
  {
    "name": "summarize",
    "description": "总结当前会话",
    "kind": "prompt",
    "template": "请总结以下对话，保留结论与待办：\n\n{selection}"
  },
  {
    "name": "deploy",
    "description": "部署当前工作区",
    "kind": "action",
    "handler": "deploy",
    "autoSend": false
  }
]
```

```js
export const commands = {
  async deploy(args, ctx) {
    return { draft: `请帮我部署 ${ctx.workspace}，使用 deploy 工具。` };
  },
};
```

### 2.3 语义

| 类型 | 行为 |
|---|---|
| `prompt` | 把 `template` 写入输入框（`{selection}` 为当前选中/草稿内容），由用户编辑后自行发送 |
| `action` | 调用 JS handler，返回 `{draft?}` / `{message?}`；`autoSend` 默认 `false`，为 `true` 时才直接发送 |

- 命令 token：`/<pluginId>-<name>`。菜单展示时按来源分组，但用户输入短名也能匹配（只在该短名唯一时）。
- 合并顺序：ACP 命令 → 插件命令 → Skill 命令；token 冲突时插件加载失败并报错（不改名，避免记忆错位）。
- handler 的 `ctx`：`{conversation, agent, workspace, draft, selection, settings, store, log}`；**不注入模型调用**。
- 命令不得直接执行 shell：需要动作时返回 `draft` 让用户确认，或调用本插件已声明的工具（仍需工具处于开启状态）。

### 2.4 实现点

| 位置 | 改动 |
|---|---|
| `internal/ui/composer_menu.go:39/63` | 增加 `pluginCommands()` 并合并 |
| `internal/ui/kit/menu.go:33` | `SlashCommand` 增加可选 `Group` 字段，菜单按组显示 |
| `internal/plugin/manager.go` | 命令注册、token 冲突校验 |

### 2.5 验收

- 输入 `/weather-summarize` 出现命令项，选择后模板进入输入框且不自动发送。
- `action` 返回的草稿同样需要用户按 Enter。
- 命令超时 5s 后提示失败，输入框内容不变。

## 3. 上下文提供者（@）

### 3.1 现状

`internal/ui/composer_menu.go:252` `contextItems()` / `:298` `listWorkspaceFiles()` 只列工作区文件；`kit.ContextMentionMenu`（`kit/menu.go:143`）已支持异步：`ContextMentionOptions.Loading` + `QueryChanged()`。插入的 `@rel` 目前只是文本，由 Agent 通过文件工具读取。

### 3.2 清单与 JS

```json
"contextProviders": [
  {
    "id": "jira",
    "label": "Jira",
    "icon": "globe",
    "kind": "doc",
    "search": "jiraSearch",
    "resolve": "jiraResolve",
    "maxChars": 8000,
    "cacheSeconds": 300
  }
]
```

```js
export const contextProviders = {
  async jiraSearch(query, ctx) {
    const r = await ctx.http.get(`https://jira.example/rest/search?q=${encodeURIComponent(query)}`);
    return r.json.issues.map(i => ({ id: i.key, label: i.key, detail: i.fields.summary }));
  },
  async jiraResolve(id, ctx) {
    const r = await ctx.http.get(`https://jira.example/rest/issue/${id}`);
    return { text: r.json.fields.description };
  },
};
```

### 3.3 数据流

```
输入 @ → 合并本地文件项 + 各 provider 的 search 结果（防抖 250ms，3s 超时）
  → 选择后插入 @plugin:<id>:<provider>:<itemId>
  → 发送时解析 token：resolve(itemId) → 文本
  → 以 "## 引用上下文" 段落追加到用户消息，受 maxChars 限制
  → 失败：保留 token，追加一行 "（该引用未能加载）"
```

- `resolve` 结果缓存：按 `conversationId + itemId` 缓存 `cacheSeconds`，避免重复请求。
- 截断：单项 `maxChars` 默认 8000、硬上限 32000；超出按行截断并注明。
- 多个引用按插入顺序拼接，总预算 64000 字符，超出后只保留引用链接。
- 与 Skill 注入互不影响：Skill 在 system 消息，引用上下文在 user 消息。

### 3.4 实现点

| 位置 | 改动 |
|---|---|
| `internal/ui/composer_menu.go:256` | 合并 provider 结果，带 `Loading` 状态 |
| `internal/ui/conversation.go:126` `sendDraft()` | 发送前解析 `@plugin:` token 并内联 |
| `internal/plugin/manager.go` | provider 注册、搜索/解析调度与缓存 |

### 3.5 安全与验收

- 需要 `context` 权限；联网时需要对应 `http:<host>`。
- 只读：provider 不能写文件、不能改会话。
- 验收：输入 `@` 能看到 Jira 分组；选中并发送后，模型上下文里出现引用正文；接口超时只降级为引用链接。

## 4. 导入器

### 4.1 现状

`internal/agentimport/source.go:45` `Sources()` 是硬编码列表；`agentconf.go:96/172` 分别发现 MCP 与 Provider；UI 侧 `internal/ui/import_dialog.go:17` 定义 `importSkills / importMCP / importProviders` 三种导入。

### 4.2 清单（声明式映射）

```json
"importers": [
  {
    "id": "cursor",
    "label": "Cursor",
    "kinds": ["mcp", "providers"],
    "files": [
      { "os": "any", "path": "~/.cursor/mcp.json" },
      { "os": "any", "path": "~/.cursor/settings.json" }
    ],
    "format": "json",
    "mapping": {
      "mcp": { "root": "/mcpServers", "name": "@key",
               "command": "/command", "args": "/args", "env": "/env", "url": "/url" },
      "providers": { "root": "/providers", "name": "@key",
                     "baseURL": "/baseUrl", "apiKey": "/apiKey", "models": "/models" }
    }
  }
]
```

- `mapping` 用 JSON Pointer 风格：`/a/b` 取值，`@key` 取对象键名，`*` 遍历数组。支持 `json` / `yaml` / `toml` 三种格式。
- 非标准格式可声明 `parse` handler（P2），入参与返回都受 5s / 2MB 限制。
- 导入落盘仍走现有路径：Skill 走 `agentimport.Import`，MCP/Provider 走 `import_actions.go` 的既有写入，因此路径安全、去重、覆盖确认全部继承。

### 4.3 安全与验收

- 只读扫描：路径必须在 home 或当前工作区内；不跟随符号链接；单文件 2MB、总数 200。
- 不执行被导入 Agent 的任何代码或命令。
- 验收：插件声明的来源出现在导入对话框并标注来源；解析失败只让该来源显示为空，不影响其他来源。


## 5. 任务触发器

### 5.1 现状

`config.Schedule` 只有 `daily / weekly / monthly`；`internal/ui/autotask.go:534` `nextRun()` 计算下次时间，`:563` `autoTaskLoop()` 固定每 30s 轮询，`:570` `autoTaskTick()` 判断到期，`:588` `startAutoTask()` 执行。

### 5.2 目标与边界

插件贡献的是**触发器类型**，不是定时任务本身。任务仍由用户在“自动任务”页创建，触发器只决定“下次什么时候跑”。

### 5.3 清单与 JS

```json
"triggerTypes": [
  {
    "type": "interval",
    "label": "固定间隔",
    "fields": [
      { "key": "seconds", "label": "间隔（秒）", "type": "number", "default": 3600, "min": 60 }
    ],
    "evaluate": "evaluateInterval"
  },
  {
    "type": "webhook",
    "label": "Webhook 触发",
    "external": true,
    "fields": [{ "key": "secret", "label": "签名密钥", "type": "secret" }]
  }
]
```

```js
export const triggers = {
  async evaluateInterval(trigger, lastRunISO, nowISO) {
    const step = Number(trigger.seconds) * 1000;
    const base = lastRunISO ? Date.parse(lastRunISO) : Date.parse(nowISO);
    return { next: new Date(base + step).toISOString() };
  },
};
```

### 5.4 语义

- `evaluate` 必须是纯函数：只读 `trigger` 字段与时间，不联网、不写 store、不读会话；超时 1s。
- 返回 `{next}`（ISO 时间）或 `{next: null}`。返回 null、超时、抛错 → **暂停该任务**并在卡片上给出原因，**不**退化成“立刻执行”（fail-closed）。
- 任务配置新增 `trigger: {type, values}`；旧的 `schedule` 继续可读，加载时转换成内置 `daily/weekly/monthly` 触发器，保存后只写新字段。
- 内置触发器（每天/每周/每月）留在 Go 侧，插件只处理自己声明的 `type`。
- 外部触发（`external: true`）依赖宿主 HTTP 端点（见 6.4）：事件到达后置 `pending` 标记，下一次 tick 执行；同一任务 1 分钟内最多触发一次（去重）。

### 5.5 实现点

| 位置 | 改动 |
|---|---|
| `internal/config/config.go` `Schedule` | 新增 `Trigger` 与 `EffectiveTrigger()`，保留旧字段反序列化 |
| `internal/ui/autotask.go:534` `nextRun()` | 改为 `nextRun(task, from)`，先查插件求值器 |
| `internal/ui/autotask.go:570` `autoTaskTick()` | 到期判断改读缓存的 `next`，避免每轮重复调用 JS |
| `internal/ui/autotask.go:344` `autoScheduleForm()` | 按 `fields` 声明渲染触发器表单 |
| `internal/plugin/manager.go` | 触发器注册、求值调度、错误停用 |

### 5.6 验收

- 新建间隔 60s 的任务，1 分钟内只执行一次。
- 求值器抛错后任务暂停并提示原因，恢复后可继续。
- 旧配置（`schedule.kind=daily`）升级后行为不变。

## 6. 渠道驱动

### 6.1 现状

`internal/channel/driver.go:62` 定义 `Driver` 接口，`:121` `RegisterDriver()` 是注册表；`internal/ui/channelmanager.go:66` `startChannel()` 按 `config.Channel.Type` 创建驱动，`:150` `onChannelInbound()` 把消息交给 Agent，`:193` `sendChannelReply()` 回发。UI 字段在 `channelFields()`，发送工具在 `internal/ui/channel_tools.go`。

### 6.2 目标与边界

插件提供**通用 Webhook / 私有 IM** 渠道：收消息用宿主本地 HTTP 端点，发消息用 `ctx.http`。不开放平台 SDK 级别的长连接协议（那需要进程内 Go 驱动）。

### 6.3 清单与 JS

```json
"channels": [
  {
    "type": "myim",
    "name": "私有 IM",
    "mode": "webhook",
    "fields": [
      { "key": "token", "label": "Bot Token", "type": "secret" },
      { "key": "signing_secret", "label": "回调签名密钥", "type": "secret" }
    ],
    "verify": "verifyWebhook",
    "send": "sendMessage"
  }
]
```

```js
export const channels = {
  myim: {
    // 校验回调：返回 {ok, chatId, fromUser, text} 或 {ok:false}
    async verifyWebhook(req, ctx) {
      const body = req.json;
      return { ok: true, chatId: body.chat_id, fromUser: body.user, text: body.text };
    },
    // 发送：target 来自 Inbound.ChatID
    async sendMessage(target, markdown, ctx) {
      await ctx.http.post("https://myim.example/send", {
        json: { chat: target, text: markdown, token: ctx.settings.token },
      });
      return { ok: true };
    },
    // 可选：连接状态
    async status(ctx) { return { state: "connected" }; },
  },
};
```

### 6.4 宿主适配器

新增 `pluginChannelDriver`（Go，实现 `channel.Driver`）：

| 方法 | 行为 |
|---|---|
| `Start(ctx, cfg, onMessage)` | `webhook` 模式向宿主路由注册 `POST /plugins/<id>/channel/<type>`；`poll` 模式启动定时器调用 `poll` |
| `Send(ctx, target, markdown)` | 调用 JS `send`，10s 超时 |
| `Status()` | 由宿主维护（注册成功/最近一次收发/错误），JS `status` 只作补充 |
| `React()` | 首期不支持；私聊场景无需 |

收消息流程：

```
外部 POST → 路由校验（loopback、content-type、1MB 上限、60 req/min 限流）
  → 调用 verify(req, ctx)（5s 超时，签名不合法直接 401）
  → 归一化为 channel.Inbound{Text, ChatID, FromUser, ReplyToken}
  → cw.onChannelInbound(channelID, in)   // 现有 Agent 管线，零改动
```

### 6.5 发送工具

`channel_tools.go` 的 `channelToolParams()` / `channelToolHandler()` 增加 `default` 分支：插件渠道类型自动获得 `<type>_send_message`，参数只有 `text`，实现调用对应驱动 `Send`。`config.GetChannelToolTypes()` 对插件类型返回该工具名，配置页与开关逻辑不变。

### 6.6 安全

- 回调端点只监听 `127.0.0.1`（与 `webui` 一致）；路径含安装时生成的随机 token。
- 必须声明并使用签名校验；无 `verify` 的插件不允许 `webhook` 模式（`poll` 模式无此要求）。
- 请求体上限 1MB；每渠道 60 次/分钟；连续失败 10 次自动停用并提示。
- 发送需要 `http:<host>` 权限；不需要 `exec`。

### 6.7 验收

- 配置插件渠道后，外部 POST 能进入对话并收到回复。
- 签名错误返回 401 且不触发 Agent。
- 插件停用后端点立即失效；重启后路径 token 不变（保存在 config）。
- `type_send_message` 工具在渠道会话中可用，且只能发往当前会话。


## 7. 声明式渲染协议

### 7.1 目标

让插件工具的输出以表格、图表、代码、diff 等结构化形式展示，而不是一大段 JSON 文本。**插件不画原生 UI**，只能从白名单组件里选，颜色一律用主题 token。

### 7.2 协议

工具结果字符串如果是一个渲染信封，宿主就按声明渲染；否则按普通文本处理（完全向后兼容）：

```json
{
  "$chatgo": {
    "v": 1,
    "view": {
      "type": "table",
      "title": "构建结果",
      "columns": [
        { "key": "target", "label": "目标" },
        { "key": "status", "label": "状态" }
      ],
      "rows": [["web", "ok"], ["desktop", "failed"]]
    }
  }
}
```

JS 工具直接返回对象即可，宿主序列化后原样落库：

```js
export const tools = {
  async build_report(args, ctx) {
    return { $chatgo: { v: 1, view: { type: "table", title: "构建结果", columns, rows } } };
  },
};
```

### 7.3 组件白名单

| type | 字段 | 上限 | 渲染组件 |
|---|---|---|---|
| `markdown` | `text` | 20 KB | `kit.MarkdownView` |
| `table` | `columns[{key,label,align?}]`, `rows[][]` | 12 列 / 500 行 / 单元格 500 字符 | 新 `kit.DataTable` |
| `keyvalue` | `items[{label,value}]` | 100 项 | 现有行布局 |
| `list` | `items[{title,detail?,badge?}]` | 200 项 | 现有列表 |
| `chart` | `labels[]`, `series[{name,values[]}]`, `stacked?` | 100 标签 / 4 系列 | `kit.BarChart` |
| `code` | `language`, `code` | 20 KB | `kit.CodeViewer` |
| `diff` | `path`, `before`, `after` | 各 20 KB | `kit.DiffViewer` |
| `links` | `items[{title,url}]` | 50 项 | 链接列表，仅 https |
| `image` | `src`, `alt?` | 8 MB | 仅 https 或工作区内本地路径 |

- 信封总大小 ≤ 64 KB；超限降级为文本。
- 未知 `type`、字段校验失败、版本不是 `1` → 降级为文本并显示一行提示，不报错。
- 不提供 HTML / CSS / JS / 自定义颜色 / 自定义字体。
- `markdown` 复用现有 Markdown 渲染（链接预览与安全策略不变）。

### 7.4 实现点

| 位置 | 改动 |
|---|---|
| `internal/ui/message.go:282` 工具段渲染 | 先尝试 `renderView(seg.ToolCall.Result)`，失败回退文本 |
| 新增 `internal/ui/view.go` | 信封解析、校验、按 type 分派到 kit 组件 |
| `internal/ui/kit/data.go` | 新增 `DataTable`（现有列布局基础上补表头/对齐） |
| `internal/plugin/manager.go` | 无：协议由工具返回值承载，不需要注册 |

### 7.5 安全与验收

- 链路与 URL 白名单：`links` / `image` 只允许 `https`；本地图片必须解析后仍在当前工作区内。
- 渲染校验耗时 ≤ 50ms；超过则直接按文本渲染。
- 验收：表格、图表、diff、代码四类组件正确显示；非法信封显示原文；历史消息在旧版本中仍显示为 JSON 文本（不丢数据）。

## 8. 声明式 App

### 8.1 现状与边界

`internal/ui/apps.go:18` 的 `appDef` 用 Go 闭包绘制左侧工作区（`Body`）、创建标签（`NewTab`）、暴露工具（`Tools`）。JS 无法提供闭包，因此插件 App 只能使用**声明式视图 + 数据处理器**。这是本组扩展点里成本最高的一个，放在最后。

### 8.2 清单

```json
"apps": [
  {
    "id": "notes",
    "title": "笔记",
    "desc": "浏览与编辑工作区笔记",
    "icon": "file-text",
    "systemPrompt": "你是笔记助手，使用 notes__* 工具。",
    "tools": ["notes__list", "notes__read", "notes__save"],
    "tabs": { "newTab": "newTab", "label": "tabLabel" },
    "views": [
      {
        "id": "main",
        "type": "list",
        "data": "listNotes",
        "actions": [
          { "id": "open", "label": "打开", "handler": "openNote" },
          { "id": "new", "label": "新建", "handler": "newNote", "primary": true }
        ]
      }
    ]
  }
]
```

```js
export const apps = {
  notes: {
    async newTab(ctx) { return "untitled-" + Date.now(); },
    async tabLabel(tab, ctx) { return tab; },
    async listNotes(tab, ctx) {
      return { items: (await ctx.store.get("notes") || []).map(n => ({ title: n.title, badge: n.tag })) };
    },
    async openNote(tab, args, ctx) { return { refresh: true, tabLabel: args.title }; },
  },
};
```

### 8.3 运行模型

| 能力 | 设计 |
|---|---|
| 视图 | 复用第 7 节白名单：`list` / `table` / `keyvalue` / `markdown` / `chart` / `code` / `diff` / `form` |
| 标签页 | 宿主管理；`newTab` 返回标签 ID，`label` 返回显示名 |
| 动作 | 按钮触发 handler，返回 `{refresh?, view?, tabLabel?}`；刷新后重跑当前视图的 `data` |
| 工具 | App 会话只挂载声明的工具；会话引用 `app:<pluginId>/<appId>` |
| 数据 | 默认内存态；声明 `stateKey` 时按标签持久化到 `plugin-data/<id>/app-state.json` |
| 表单 | `form` 视图字段与插件设置的字段类型一致；提交调用 `submit` handler |

### 8.4 实现点

| 位置 | 改动 |
|---|---|
| `internal/ui/apps.go:27` `apps()` | 改为 `cw.apps()`，合并内置与插件 App |
| `internal/ui/apps.go:18` `appDef` | 新增 `pluginID` 与声明式字段；`Body` 由通用渲染器实现 |
| `internal/ui/conversation.go:246` `applyApp` | 支持 `app:<pluginId>/<appId>` 引用 |
| 新增 `internal/ui/plugin_app.go` | 视图渲染、动作分发、标签状态 |
| `internal/plugin/manager.go` | App 注册、视图数据调度、状态持久化 |

### 8.5 风险与验收

- 视图协议一旦发布就要保持稳定，因此必须在第 7 节协议稳定后再做（P2）。
- 验收：插件 App 出现在 Apps 页；左侧列表与按钮可用；App 会话只看到声明的工具；禁用插件后 App 消失且会话回退为普通聊天。


## 9. 实施顺序与依赖

```
P1-a 事件钩子 ─────────────┬─→ 通知渠道
                           └─→ 外部触发器（Webhook）
P1-b 斜杠命令 ─────────────── 独立
P1-b 上下文提供者 ─────────── 独立（联网需 http 授权）
P1-b 导入器 ───────────────── 独立（纯声明式）
P1-c 任务触发器 ───────────── 依赖 P1-a 仅限 external 类型
P1-d 宿主 HTTP 端点 ─────────┬─→ Webhook 触发器
                             └─→ 渠道驱动
P2   声明式渲染协议 ─────────→ 声明式 App
```

| 阶段 | 内容 | 交付物 | 主要风险 |
|---|---|---|---|
| P1-a | 事件钩子 | `internal/plugin/event.go`、事件埋点、示例订阅插件 | 事件洪泛、隐私提示 |
| P1-b | 斜杠命令 / 上下文 / 导入器 | 菜单合并、发送前内联、导入对话框合并 | 菜单冲突、上下文截断策略 |
| P1-c | 任务触发器 | `config.Trigger`、求值调度、表单渲染 | 旧配置兼容、fail-closed |
| P1-d | HTTP 端点 + 渠道驱动 | `pluginChannelDriver`、路由、限流、签名 | 端点暴露、重放攻击 |
| P2 | 渲染协议 + App | `view.go`、`kit.DataTable`、`plugin_app.go` | 协议稳定性、App 复杂度 |

建议把 P1-a 单独作为一次发布：它不碰 UI，风险最低，却能把通知、审计、外部触发器三件事一起解锁。

## 10. 逐项验收清单

| 扩展点 | 必验项 |
|---|---|
| 事件钩子 | 订阅 `turn.completed` 能收到；超时/抛错不影响回复；队列打满只告警；禁用即停投 |
| 斜杠命令 | 模板命令不自动发送；动作命令返回草稿；token 冲突时插件加载失败 |
| 上下文提供者 | `@` 菜单出现分组；发送后引用正文进入上下文；接口失败降级为链接 |
| 导入器 | 声明来源出现在导入对话框；解析失败只影响自身；不跟随符号链接 |
| 任务触发器 | 间隔任务按时执行且不重复；求值失败暂停任务；旧 `schedule` 配置行为不变 |
| 渠道驱动 | 签名校验、限流、1MB 上限生效；停用后端点失效；发送工具只发当前会话 |
| 渲染协议 | 9 类组件正确渲染；非法信封回退文本；超限降级；URL 白名单生效 |
| 声明式 App | Apps 页出现；视图/动作可用；会话工具隔离；禁用后回退普通聊天 |

## 11. 明确不在本期

- 插件贡献原生 UI 组件、布局、主题以外的样式。
- 插件实现平台长连接协议（WebSocket 客户端等）。
- 插件执行 shell、读写工作区文件（除非后续单开 `fs` 权限）。
- 插件提供 OAuth / 凭据托管。
- 插件间相互调用或共享状态。

