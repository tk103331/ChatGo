# ChatGo 插件系统设计（moejs / JavaScript）

- 状态：设计已确认，待实现
- 日期：2026-11-08
- 相关模块：`internal/plugin`（新增）、`internal/config`、`internal/ui`、`internal/llm`

## 0. 已确认的决策

| 决策点 | 结论 |
|---|---|
| 运行时 | 进程内 moejs（`github.com/Calcium-Ion/moejs`），纯 Go，可交叉编译 |
| 模型扩展 | 分成 **Provider** 与 **Middleware** 两类。Provider：新增自定义 Provider 预设（OpenAI/Claude/Gemini 兼容协议）。Middleware：消息级请求改写、流式增量改写、回复后处理、错误处理。不做自定义 ChatModel |
| 安装来源 | 本地目录、zip、git 仓库、npm 包（含更新与来源记录） |
| 安全默认 | 零权限启动，清单声明 + 首次启用逐项授权；扩权必须重新确认 |

## 1. 目标与非目标

### 1.1 目标

第三方 JavaScript 插件在不重新编译 ChatGo 的前提下扩展七类能力：

| 能力 | 插件提供 | 用户可见位置 |
|---|---|---|
| 工具 | 新工具（JSON Schema 参数 + JS handler） | 输入框工具菜单、Agent 工具白名单 |
| 模型 | 新 Provider 预设 + 模型中间件（改写请求消息、流式增量、回复后处理、错误处理） | 设置 → 模型（Provider）、插件页（中间件） |
| MCP | 新的 MCP Server（stdio / sse / streamable_http） | MCP 页、输入框工具菜单 |
| Skill | Skill 正文（清单内联或随包 SKILL.md） | Skill 页、Agent 技能白名单 |
| ACP Agent | 本地 coding CLI 连接定义 | 设置 → ACP、Agent 选择器 |
| 普通 Agent | 预置 Agent（提示词/模型/工具/技能/模式） | Agent 页、新建会话 |
| 外观 | 新配色主题（Accent 三态 + 明暗背景/表面色） | 设置 → 外观 |

### 1.2 非目标（明确排除）

- **不支持插件实现完整 ChatModel**（即插件不能自己发请求、自己解析流）。插件可以新增 Provider 预设，协议限于 OpenAI Chat、OpenAI Responses、Claude Messages、Gemini generateContent。
- **中间件不拦截 HTTP 传输层**：eino 的模型客户端不暴露 transport，中间件工作在消息与增量层。
- **中间件不能切换 Provider / Model**，不能修改路由策略（Provider Group 的选路发生在中间件之前）。这是为了保证一次请求的目标可预测。
- 插件不能自定义原生 UI 面板或布局。MyGo 是 GPU 自绘界面，没有 DOM/CSS；可用的 UI 能力只有颜色主题与声明式组件（见扩展点文档）。斜杠命令、上下文提供者、声明式 App 属于 P1/P2 扩展点，不在本文首批范围。
- 插件之间不互相调用，不共享运行时对象。
- 首期不提供在线插件市场（安装源已含 git/npm，市场留到后续）。
- `cmd/web`（wasm 客户端）不加载插件；`cmd/webd` 与桌面端共享 `~/.chatgo` 配置，插件在 `webd` 进程内加载后对 Web UI 的模型列表/技能生效（工具链在 Web UI 尚未实现，随其能力补齐）。

## 2. 现有可挂载点

| 能力 | 现有实现（文件:定位） | 插件接入方式 |
|---|---|---|
| 工具清单 | `internal/ui/tools.go` `toolMetas()`；`internal/ui/selection.go:55` `visibleBuiltinToolNames()` | 新增 `plugin:` 前缀的 toolMeta |
| 工具构建 | `internal/ui/provider_client.go:165` `buildEinoToolsEnv()`；`:308` `builtinToolWrapper` | 新增 `plugin:` 分支，复用 wrapper 模式 |
| 工具观测 | `internal/llm/react_agent.go:154` `ObserveTool()` | 直接复用，无需改动 |
| Provider | `internal/config/config.go` `Provider`；`internal/llm/client.go:28` `NewClient`；`internal/ui/models.go:39` `allModelRefs()` | 注入 `Source="plugin:<id>"` 的虚拟 Provider |
| 模型列表 | `internal/llm/modellist.go` `ListModels()` | 直接复用，无需改动 |
| MCP | `config.MCPServers`；`internal/mcp/manager.go`；`internal/ui/app.go:329` `startMCP()` | 注入虚拟 MCP Server，插件须先于 `startMCP` 加载 |
| Skill | `internal/ui/skills.go:52` `loadSkills()`；`:437` `skillPrompt()` | 插件 Skill 合并进 `cw.skills.list` |
| ACP Agent | `config.ACPAgents`；`internal/ui/acp.go` | 注入虚拟 ACP 连接 |
| Agent | `config.Agents`；`internal/ui/agents.go` | 注入虚拟 AgentDef |
| 外观 | `internal/ui/theme/palettes.go:34` `Themes`；`internal/ui/settings_appearance.go:70` | 追加主题项，外观页自动列出 |
| JS 运行时 | `internal/jssandbox/jssandbox.go` | 复用同一套 Compile / Runtime / Interrupt 用法 |

## 3. 架构选型

| 方案 | 优点 | 缺点 |
|---|---|---|
| **A. 进程内 moejs（采用）** | 已是依赖；纯 Go 零 cgo，可交叉编译；单次调用微秒级；支持 async/await、Promise、eventloop；可限制内存、可任意 goroutine 中断 | 无法使用 Node 生态；隔离依赖中断兜底 |
| B. 子进程 JS 运行时（Node/Deno） | 隔离最好、生态最全 | 要求用户安装 Node；包体大；IPC 与生命周期复杂 |
| C. 纯声明式（无代码） | 最安全 | 无法承载工具逻辑，不满足需求 |

选 A 的决定性理由：moejs 的 `Options.MemoryLimit`、`Runtime.Interrupt`、`eventloop`（`NewPromise` / `RunOnLoop`）正好覆盖“可异步、可限时、可限内存”三项硬要求，且与现有 `jssandbox` 完全同构，维护成本最低。

```
┌──────────────── ChatGo (Go) ────────────────┐
│ config.Config ──合并贡献──┐                  │
│ UI / ReactAgent           │                  │
│   └─ plugin: 工具 ────────┤                  │
│                           ▼                  │
│ internal/plugin.Manager                      │
│  ├─ manifest 解析与校验                      │
│  ├─ 贡献合并 / 回收                          │
│  ├─ Instance(pluginID)                       │
│  │   ├─ *moejs.Module（编译一次，可复用）     │
│  │   ├─ Runtime 池（默认 2）                 │
│  │   └─ eventloop（异步宿主调用）             │
│  └─ HostAPI（按授权裁剪后注入）               │
└──────────────────┬───────────────────────────┘
                   │ JSON in / JSON out
        ~/.chatgo/plugins/<id>/
          plugin.json   main.js   skills/   icon.svg
          ~/.chatgo/plugin-data/<id>/   （运行期数据）

## 4. 插件包格式

```
~/.chatgo/plugins/weather/
├── plugin.json                  # 清单：元数据 + 贡献声明 + 权限 + 设置项
├── main.js                      # 入口（ESM；也可 main.ts，用 CompileTS）
├── skills/
│   └── weather-guide/SKILL.md   # 随包 Skill（可直接被自动发现）
├── icon.svg                     # 插件卡片图标
└── README.md
```

规则：

- `plugin.json` 必须在包根目录；`main` 缺省为 `main.js`。
- `id` 决定安装目录名与命名空间，必须匹配 `^[a-z0-9][a-z0-9-]{1,63}$`。
- 包内所有路径必须相对、不得含 `..`，解析后不得越出插件目录（防 zip 路径穿越）。
- Skill 可由清单声明，也可放在 `skills/*/SKILL.md` 由 `loadSkills()` 自动扫描；两种方式都归到 `plugin:<id>` 来源。

### 4.1 清单示例（覆盖全部扩展点）

```json
{
  "id": "weather",
  "name": "Weather Kit",
  "version": "1.0.0",
  "author": "tk",
  "description": "天气工具、Provider、模型中间件、MCP、Skill、Agent 与主题",
  "main": "main.js",
  "engines": { "chatgo": ">=0.12" },
  "limits": { "memoryMB": 64 },
  "permissions": ["http:api.weather-ai.example"],

  "settings": [
    { "key": "api_key", "label": "API Key", "type": "secret", "required": true },
    { "key": "units", "label": "单位", "type": "select",
      "options": ["metric", "imperial"], "default": "metric" }
  ],

  "tools": [
    {
      "name": "get_weather",
      "label": "查询天气",
      "description": "查询城市实时天气",
      "handler": "get_weather",
      "parameters": {
        "type": "object",
        "properties": { "city": { "type": "string", "description": "城市名" } },
        "required": ["city"]
      }
    }
  ],

  "providers": [
    {
      "name": "WeatherAI",
      "type": "custom",
      "apiType": "openai",
      "baseURL": "https://api.weather-ai.example/v1",
      "models": ["w-1"],
      "apiKeySetting": "api_key",
      "headers": { "X-Api-Version": "2" }
    }
  ],

  "middleware": [
    {
      "name": "units",
      "match": { "providers": ["WeatherAI"] },
      "priority": 100,
      "hooks": ["beforeRequest", "afterResponse"]
    }
  ],

  "mcpServers": [
    { "name": "weather-mcp", "type": "stdio", "command": "npx",
      "args": ["-y", "weather-mcp"], "env": {} }
  ],

  "acpAgents": [
    { "name": "WeatherCLI", "command": "weather-cli", "args": ["acp"] }
  ],

  "agents": [
    {
      "name": "Weather Helper",
      "description": "专注天气问答",
      "systemPrompt": "你是天气助手，优先调用 get_weather。",
      "models": ["WeatherAI::w-1"],
      "tools": ["plugin:weather:get_weather"],
      "skills": ["plugin:weather:weather-guide"],
      "mode": "agent"
    }
  ],

  "skills": [
    { "name": "weather-guide", "description": "天气工具使用规范",
      "path": "skills/weather-guide/SKILL.md" }
  ],

  "themes": [
    { "name": "weather-sky", "label": "晴空",
      "accent": { "light": "#0284c7", "lightHover": "#0369a1", "lightPressed": "#075985",
                  "dark": "#38bdf8", "darkHover": "#7dd3fc", "darkPressed": "#0ea5e9" },
      "lightBg": "#f0f9ff", "darkBg": "#0b1620",
      "lightSurface": "#f2f8fd", "darkSurface": "#0b1620" }
  ]
}
```

设计原则：**清单声明贡献，JS 只实现行为**。首期不做运行时动态注册，加载顺序可预测、UI 列表稳定、错误能精确归因到某个贡献项。

### 4.2 入口模块

```js
// main.js —— 名字与清单里的 handler 对应
export const tools = {
  async get_weather(args, ctx) {
    const res = await ctx.http.get(
      `https://api.weather-ai.example/v1/now?city=${encodeURIComponent(args.city)}`,
      { headers: { authorization: `Bearer ${ctx.settings.api_key}` } }
    );
    return { city: args.city, temp: res.json.temp, unit: ctx.settings.units };
  },
};

// 模型中间件：键与清单 middleware[].name 对应
export const middleware = {
  units: {
    async beforeRequest(req, ctx) {
      return { systemAppend: "回答时统一使用公制单位。" };
    },
    async afterResponse(resp, ctx) {
      return { text: resp.text.replace(/°F/g, "°C") };
    },
  },
};
```

- 导出对象 `tools`，键为清单中的 `handler` 名；宿主用 `mod.Hook("tools", name)` 取函数。
- handler 签名 `(args, ctx) => string | object | Promise<...>`；返回值会被序列化为工具结果字符串。
- 允许 ESM 多文件：`moejs.Link` + 受控的 `Importer`，只允许导入插件包内相对路径与 `chatgo:*` 内置模块（首期不提供内置模块）。

## 5. 运行时模型

| 项 | 设计 |
|---|---|
| 编译 | 加载期 `moejs.Compile` / `CompileTS` 一次；`*moejs.Module` 不可变，可被多个 Runtime 并发 `Load` |
| 并发 | 每插件一个 Runtime 池，默认 2，上限 `min(4, GOMAXPROCS)`，懒创建；池满排队。moejs Runtime 非并发安全，一 goroutine 一 Runtime |
| 异步 | 宿主 API 的阻塞调用（HTTP）经 `eventloop` + `NewPromise` 实现，JS 侧就是普通 `await` |
| 内存 | `Options.MemoryLimit = 64 MiB`（可在清单覆盖，上限 256 MiB） |
| 超时 | 工具 60s；`setup`/首次加载 10s；超时调用 `rt.Interrupt(ctx.Err())` |
| 错误 | JS 抛错 / 语法错误 / 超时 → 转成字符串工具结果返回模型，不中断整轮对话；`*moejs.InternalError`（宿主 panic）→ 销毁该 Runtime 并重建 |
| 卸载 | 清空池、取消在途调用、回收 contributions、关闭打开的 HTTP 句柄 |


## 6. 扩展点详细设计

### 6.1 工具

- 暴露给模型的工具名：`<pluginId>__<toolName>`，sanitize 为 `[a-zA-Z0-9_]`，避免与内置工具重名。
- UI toolMeta ID：`plugin:<pluginId>:<toolName>`，Label 用清单的 `label`（缺省 `name`）+ 插件徽标。
- `buildEinoToolsEnv()` 增加 `plugin:` 前缀分支，调用 `plugin.Manager.ToolInfo()` / `Invoke()`，结果包进 `llm.ObserveTool`，耗时/入参/出参与内置工具一致展示。
- 会话级开关（`toolOn`）与 Agent 白名单（`AgentDef.BuiltinTools`）直接复用：选择列表里存的就是 `plugin:` ID。
- 同一插件内工具名不合法或重名 → 拒绝该插件加载（不做静默改名，否则模型历史里的工具名会错位）。
- 工具返回：`string` 直接使用；对象/数组序列化为 JSON；抛错转成 `"错误: <message>"` 作为工具结果。

### 6.2 模型（两类扩展）

#### 6.2.1 Provider 预设

- 清单 `providers[]` 注入为 `Source="plugin:<id>"` 的虚拟 `config.Provider`，`Type="custom"`，`APIType` 取 `openai` / `openai_responses` / `claude` / `gemini`。
- `llm.NewClient` 不用改：现有 `EffectiveAPIType()` 分支已覆盖这四种协议。
- 需要在 `Provider` 上新增 `Headers map[string]string`，让插件声明的静态请求头（版本号、租户标识）进入客户端；现有 Provider 用不到时为空。
- 密钥：清单用 `apiKeySetting` 指向插件设置项，加载时从 `plugins[].settings` 解析并填入 `APIKey`；插件 Provider 不写回 `config.yaml`，避免“保存即丢”。
- 插件 Provider 在设置 → 模型中显示为只读卡片（名称、地址、模型，附“由插件 X 提供”），“测试连接 / 拉取模型列表”照常可用；若要改密钥，跳转插件设置页。
- 重名规则：与内置或其他插件的 Provider 同名 → 拒绝该贡献并报错。

#### 6.2.2 Middleware

中间件工作在**消息与增量层**，不触碰 HTTP 传输层，也不能切换 Provider/Model。

清单：

```json
"middleware": [
  {
    "name": "units",
    "match": { "providers": ["WeatherAI"], "models": ["w-*"] },
    "priority": 100,
    "timeoutMs": 3000,
    "hooks": ["beforeRequest", "onChunk", "afterResponse", "onError"]
  }
]
```

四种钩子：

| 钩子 | 签名 | 能力 |
|---|---|---|
| `beforeRequest` | `(req, ctx) => patch \| null` | 读 `req.messages` / `req.system` / `req.iteration`；返回 `{systemAppend?, system?, messages?}` 改写本次模型请求 |
| `onChunk` | `(delta, ctx) => string \| null` | 流式增量改写；`ctx.channel` 区分 `content` / `reasoning`，`ctx.index` 为增量序号 |
| `afterResponse` | `(resp, ctx) => patch \| null` | 对一次完整模型响应做后处理：`{text?, reasoning?}` |
| `onError` | `(err, ctx) => patch \| null` | 返回 `{message?, retry?}`，可把上游错误翻译成人话；`retry` 最多触发一次重试 |

执行顺序：

```
Agent 系统提示 + Skill 正文
  → beforeRequest 链（priority 升序；多个插件依次执行）
  → 模型调用（流式）
  → onChunk（每个增量，content/reasoning 分别经过）
  → afterResponse（每次模型响应一次）
  → 落库与渲染
         ↘ 出错 → onError 链 → 可选择重试一次或替换错误文案
```

约束与失败策略：

- `match` 为空表示匹配所有 Provider/Model；`models` 支持 `*` 通配。不匹配的中间件整条跳过。
- `priority` 数字小的先执行，默认 100；同一插件内按清单顺序。
- 每个钩子单次超时 `timeoutMs`（默认 3000，上限 10000）。超时或抛错 → **跳过该钩子并记录警告**（fail-open），不阻断对话。
- `onChunk` 在热路径上：单个中间件连续 20 个增量平均耗时超过 2ms，则本次响应内自动停用该中间件并提示，避免拖慢输出。
- React Agent 一轮对话会多次调用模型：`beforeRequest` / `afterResponse` 按**每次模型请求**触发，`ctx.iteration` 从 0 递增；中间件必须幂等，重复追加系统提示由宿主按内容去重兜底。
- 请求体上限：序列化后的 `messages` 超过 1 MiB 时跳过 `beforeRequest`，防止插件阻塞大上下文请求。
- 隐私：中间件能读到完整对话内容。安装确认页会明确提示“该插件可读取对话内容”，无需额外权限位。
- 中间件不能调用模型（宿主不注入 `ctx.llm`），避免递归与失控成本。

Go 侧实现：

```go
// internal/llm/middleware.go
type Middleware interface {
    Name() string
    Matches(provider, model string) bool
    BeforeRequest(ctx context.Context, req *MiddlewareRequest) error
    OnChunk(ctx context.Context, chunk MiddlewareChunk) (MiddlewareChunk, error)
    AfterResponse(ctx context.Context, resp *ChatResponse) error
    OnError(ctx context.Context, err error) (MiddlewareErrorResult, bool)
}

// 中间件可见的请求视图：改写这里不会改动已保存的会话消息。
type MiddlewareRequest struct {
    Provider  string
    Model     string
    System    string
    Messages  []ChatMessage
    Iteration int // React Agent 内的模型调用序号，从 0 开始
}

type MiddlewareChunk struct {
    Channel string // content | reasoning
    Index   int
    Text    string
}

type MiddlewareErrorResult struct {
    Message string
    Retry   bool
}
```

`Client`、`ReactClient`、`GroupClient` 在构造时接收 `[]Middleware`（空链即当前行为，零回归），在 `Chat` / `chatWithStream` 的对应位置调用；插件管理器把每条中间件适配成该接口。

### 6.3 MCP

- 清单 `mcpServers[]` 注入 `cfg.MCPServers`（`Source` 标记），插件加载必须先于 `go cw.startMCP()`（放在 `NewChatWindow` 内、`startMCP` 之前）。
- 校验：`name` 全局唯一；`stdio` 必须有 `command`；`sse` / `streamable_http` 必须有 http(s) `url`。
- 权限：`stdio` 需要 `exec`；远程类型需要对目标主机的 `http:<host>`。
- 连接失败只影响该 server，UI 显示错误原因与“重试”。

### 6.4 Skill

- 两种来源：清单 `skills[]`（`path` 指向包内 SKILL.md），或自动扫描 `skills/*/SKILL.md`。
- 合并进 `cw.skills.list`：`ID="plugin:<id>:<name>"`，`Source="plugin:<id>"`；`SkillsEnabled` 与 Agent 的 `Skills` 白名单直接可用。
- 正文注入仍走 `skillPrompt()`，无需改动。
- 与本地同名 Skill 并存，不做覆盖；UI 以来源徽标区分。


### 6.5 ACP Agent

- 清单 `acpAgents[]` 注入 `cfg.ACPAgents`（`Source` 标记），`Disabled` 默认 `true`，由用户在 ACP 页启用（与现有 4 个预置一致）。
- 需要 `exec` 权限；命令不存在时只报“未安装”，不阻塞插件加载。
- 名称全局唯一；`claude-code`、`opencode` 这类通用名建议插件自带前缀，避免与内置预置冲突。

### 6.6 普通 Agent

- 清单 `agents[]` 注入 `cfg.Agents`（`Source` 标记）。
- `Name` 必须全局唯一——冲突直接拒绝该贡献并报错，不做静默改名（会话里存的是 `agent:<Name>`，改名会让历史会话错位）。
- 引用的模型/工具/Skill 在加载期校验：缺失项只告警（UI 在 Agent 卡上标注“引用了不可用的模型”），不阻断加载。
- `mode` 支持 `chat` / `agent`，与现有 Agent 编辑器语义一致。

### 6.7 外观

- 清单 `themes[]` 追加进 `mtheme.Themes`，外观页自动列出，`ByName()` / 持久化字段 `color_theme` 自动可解析。
- 只开放**颜色 token**：Accent 明暗三态 + 页面/表面色。MyGo 是自绘 UI，没有 CSS，字体/圆角/间距属于全局布局语义，不在首期范围。
- `name` 冲突拒绝；`label` 用于显示，建议带插件来源（卡片上另有徽标）。
- 插件被禁用后，若用户当前正使用该主题，回退到默认主题并提示。

### 6.8 加载顺序与生命周期

```
NewChatWindow
  → 读 config.yaml（plugins 仅含开关/授权/设置）
  → plugin.Manager.Discover()（扫描 ~/.chatgo/plugins/*）
  → 逐个 Load：解析清单 → 校验 → 编译 main.js → 按授权构建 HostAPI
  → 合并贡献（注入 cfg.* / mtheme.Themes）
  → 校验冲突，记录每个贡献项的来源与错误
  → go cw.startMCP()（此时 cfg.MCPServers 已含插件贡献）
  → 首帧渲染（工具/Skill/Agent/ACP/主题列表已完整）
```

启用 / 禁用 / 重载都走同一条路径：

- **启用**：授权确认 → 编译 → 合并 → 重建模型客户端（`clientDirty`）→ 刷新列表。
- **禁用**：取消在途调用 → 回收贡献 → 若当前会话选中了插件工具/Agent/主题，回退到安全值 → 重建客户端。
- **重载**：先禁用旧实例再按启用流程加载，失败则保持禁用并显示原因。
- **运行期错误**：单个贡献项出错只标记该插件 `status=error`（附 message），其他插件与主程序不受影响。

## 7. 贡献合并与持久化

核心手法：**运行时把插件贡献注入 `config.Config` 现有字段并打 `Source` 标记，保存时过滤掉。**

| 贡献 | 注入目标 | Source |
|---|---|---|
| 工具 | `cfg.BuiltinTools`（`Type:"plugin"`, `Enabled:true`） | `plugin:weather` |
| Provider | `cfg.Providers` | `plugin:weather` |
| Middleware | `plugin.Manager` 内的中间件链（不进 cfg） | `plugin:weather` |
| MCP | `cfg.MCPServers` | `plugin:weather` |
| Skill | `cw.skills.list` | `plugin:weather` |
| ACP | `cfg.ACPAgents` | `plugin:weather` |
| Agent | `cfg.Agents` | `plugin:weather` |
| 主题 | `mtheme.Themes`（内存追加） | `plugin:weather` |

`config.yaml` 只落“插件开关 / 授权 / 设置 / 安装来源”，贡献定义每次启动重建：

```yaml
plugins:
  - id: weather
    enabled: true
    permissions: ["http:api.weather-ai.example", "env:WEATHER_KEY"]
    settings:
      api_key: sk-xxx
      units: metric
    source:
      kind: npm            # dir | zip | git | npm
      url: weather-kit@1.0.0
      integrity: sha256:...
      installed_at: 2026-11-08T10:00:00Z
```

需要的字段改动：

- `Provider` / `MCPServer` / `AgentDef` / `ACPConnection` / `BuiltinTool` 新增 `Source string` 字段，标签为 `yaml:"-"`。
- `Config` 新增 `Plugins []PluginRef`；`writeConfig` 过滤 `Source != ""` 的条目。
- `ensureAllBuiltinTools` 只在 `loadFile` 阶段运行，注入发生在其后，因此不会误删插件工具。
- 这样 `firstEnabled()`、`allModelRefs()`、`visible*Names()`、设置页、Agent 白名单**全部零改动**即可看到插件贡献。

插件运行期数据目录：`~/.chatgo/plugin-data/<id>/`，`ctx.store` 读写其中的 `state.json`；插件申请写文件时也只能落在这里。


## 8. 安装与更新

### 8.1 安装流水线

四种来源统一走同一条流水线，避免四套校验逻辑：

```
解析来源 → 落到临时目录 .staging/<rand>
  → 校验 plugin.json（id / 版本 / engines / 权限语法 / 路径安全）
  → 计算包摘要（sha256）
  → 与已安装版本比对：权限是否扩大
  → 用户确认权限（扩权时强制重新确认）
  → 原目录备份 → 原子 rename 替换 → 写 config.plugins[]
  → 加载插件并刷新 UI
```

失败时清理 staging，已安装版本不受影响。

### 8.2 各来源

| 来源 | 实现 | 说明 |
|---|---|---|
| 目录 `dir` | 复制到 `~/.chatgo/plugins/<id>` | 另提供“链接目录（开发模式）”：不复制、原地加载，清单变化需手动重载 |
| zip | `archive/zip` 解压 | 拒绝绝对路径、`..`、符号链接逃逸；解压上限 20 MiB / 2000 项 |
| git | `git` CLI，`clone --depth 1` | 支持 `https://host/repo.git#subdir=plugins/x`；记录 `commit` 与仓库 URL；无 git 时提示改用 zip |
| npm | 直接访问 registry HTTP API | `GET https://registry.npmjs.org/<name>` 取 `dist.tarball` 与 `dist.integrity`，下载后用 sha512 校验，解压 `package/`；**不依赖本机 npm**。包内必须有 `plugin.json` |

npm 包命名约定 `chatgo-plugin-*`；registry 可在设置中改为私有镜像。

### 8.3 更新与卸载

- **检查更新**：`git ls-remote` / npm registry 版本比对；只提示，不自动升级。
- **升级**：复用安装流水线；`version` 必须大于已安装版本（或用户显式允许降级）。
- **扩权**：新版本权限集合是旧集合的超集时，暂停安装并要求重新确认；用户拒绝则保留旧版本。
- **卸载**：先禁用（回收贡献）→ 删除插件目录 → 移除 `config.plugins[]` → 询问是否同时删除 `plugin-data/<id>`。
- **完整性**：记录并展示包摘要；来源与版本在插件卡上可见，便于排查供应链问题。

## 9. 安全模型

| 风险 | 措施 |
|---|---|
| 任意代码执行 | moejs 沙箱：无 Node API、无 `require`、无 `fetch`/`fs` 全局；只有宿主注入的命名空间 |
| 死循环 / 内存爆炸 | 超时 + `Runtime.Interrupt` + `MemoryLimit`；每个 macrotask 前重置分配计数 |
| 越权访问 | 默认零权限；清单声明 + 首次启用逐项授权；未授权时对应命名空间**根本不存在**（调用即 TypeError） |
| 文件读写 | `ctx.store` 仅限 `plugin-data/<id>/`；工作区读写需 `fs:workspace` 单独授权（首期不提供） |
| 网络 | `http:<host>` 逐域名授权，无通配 `*`；重定向目标也必须命中白名单 |
| 读取环境变量 | `env:KEY` 逐 key 授权，禁止通配 |
| 启动进程 | `exec` 仅用于 MCP stdio 与 ACP，首次确认时明确提示将执行什么命令 |
| 隐私 | 中间件可读完整对话、工具可把内容外发；安装确认页明确提示，并展示该插件声明的中间件与网络域名 |
| 命名冲突 | 工具名加 `pluginId__` 前缀；Provider/Agent/MCP/Theme 重名拒绝加载 |
| 包路径穿越 | 解压与导入统一做相对路径校验，拒绝绝对路径、`..`、越界符号链接 |
| 崩溃隔离 | 单 Runtime 出错只销毁该 Runtime；插件级 error 状态，不影响其他插件与主程序 |
| 供应链 | 记录来源 + 摘要；扩权重新确认；不自动更新；P2 再加签名校验 |

授权粒度（`plugins[].permissions`）：`http:<host>`、`env:<KEY>`、`exec`、`store`（默认授予，仅插件自身数据目录）、`events`（只读事件旁路）、`context`（只读外部上下文）。

## 10. UI 设计

### 10.1 设置 → 插件

- 卡片列表：图标、名称、版本、作者、状态（已加载 / 错误 / 已禁用）、来源（npm/git/zip/dir）、贡献计数（工具/模型/中间件/MCP/Skill/Agent/主题）、启用开关。
- 详情页：
  - 权限清单（已授权高亮，未授权说明用途）
  - 根据清单 `settings` 渲染表单（`secret` 打码，`select` 用下拉，`bool` 用开关），保存后热重载
  - 贡献列表（点击跳转到对应页面）
  - 操作：重新加载、检查更新、升级、卸载、打开目录、查看日志
- 错误展示：状态红点 + 错误摘要 + “查看详情”，不阻塞启动。

### 10.2 安装入口

- “安装插件”按钮 → 选择来源（文件夹 / zip / git 地址 / npm 包名）→ 校验进度 → 权限确认弹窗（逐项勾选，默认全不选）→ 完成。
- 权限确认弹窗必须列出：将访问的域名、将读取的环境变量、将执行的命令、是否可读取对话内容。

### 10.3 来源徽标

工具选择器、Skill 页、Agent 卡、ACP 列表、主题卡、模型卡上显示插件名徽标；插件贡献项在各自页面**只读**（统一在插件页管理），点击徽标跳转插件详情。


## 11. 分期计划

### P0：首个可发布版本（本次范围）

1. **内核**：`internal/plugin` 包 —— 清单解析/校验、Module 编译、Runtime 池、HostAPI 权限裁剪、贡献合并/回收、错误隔离。
2. **声明式贡献**：工具、Provider、Middleware、MCP、Skill、ACP Agent、Agent、主题。
3. **持久化**：`config.Plugins`、`Source` 字段、`writeConfig` 过滤、禁用时回退当前选择。
4. **UI**：设置 → 插件（列表/详情/设置表单/权限）；安装弹窗；各页面来源徽标。
5. **安装**：目录、zip、git、npm 四种来源 + 更新 + 卸载 + 摘要记录。
6. **交付物**：`docs/PLUGINS.md` 开发者文档、`examples/plugins/weather` 示例插件、权限清单说明。

验收标准：

- 安装示例插件后：工具出现在工具菜单并可被模型调用；外观页出现新主题；Agent 页出现预置 Agent；Skill 页出现随包 Skill；设置 → 模型出现插件 Provider。
- 禁用插件后：上述条目全部消失；当前会话选中的插件工具自动取消；正在使用的插件主题回退默认；`config.yaml` 中不残留任何贡献定义。
- 未授权 `http` 的插件调用 `ctx.http` 抛 TypeError，对话不中断。
- 含中间件的插件启用后：`beforeRequest` 追加的系统提示生效、`onChunk` 能改写流式增量、`afterResponse` 能改写最终文本、`onError` 能替换错误文案；禁用后恢复原生行为。
- 中间件超时/抛错时自动跳过，对话继续。
- 死循环工具在 60s 内被中断，返回超时错误，主程序无卡顿。
- 扩权升级被拦截，用户拒绝后旧版本仍可用。

### P1

- **扩展点（详见 `2026-11-08-plugin-extension-points-design.md`）**：通用事件钩子、斜杠命令、上下文提供者、导入器、任务触发器、渠道驱动。
- 动态注册：`export async function activate(host)`，允许按远端配置生成贡献（需定义加载顺序与失败回滚）。
- 开发体验：链接目录 + 文件监听热重载；`chatgo plugin install/list/remove` CLI。
- 工具级钩子：`onToolResult`（脱敏、审计、结果裁剪）。
- 中间件按会话/Agent 覆盖；中间件调试面板（命中次数、耗时）。

### P2

- **扩展点（详见扩展点文档）**：声明式渲染协议、声明式 App。
- 插件市场（浏览、评分、一键安装）。
- 包签名与发布者校验。
- `webui` 工具链对齐后，插件工具在 Web UI 可用。
- 如确有需求，再评估完整自定义 ChatModel。

## 12. 代码改动清单

### 新增 `internal/plugin/`

| 文件 | 职责 |
|---|---|
| `manifest.go` | `plugin.json` 结构体、语义校验（id/版本/engines/权限/路径/重名） |
| `manager.go` | 发现、加载、启用/禁用、重载、贡献合并与回收、错误状态 |
| `runtime.go` | Module 编译、Runtime 池、超时/中断/内存限制、InternalError 恢复 |
| `hostapi.go` | `ctx` 命名空间（http/fs/store/env/settings/log）与权限裁剪 |
| `contrib.go` | 清单 → `config.*` / `theme.Theme` / `skill` 的转换 |
| `middleware.go` | 列表中间件适配为 `llm.Middleware`，含匹配、优先级、超时与失败降级 |
| `install.go` | 四种安装来源、staging、摘要、扩权检测、更新/卸载 |
| `*_test.go` | 清单、合并、保存过滤、权限、超时、安装路径安全等单测 |

### 改动现有文件

| 文件 | 改动 |
|---|---|
| `internal/config/config.go` | 新增 `PluginRef`、`Config.Plugins`、`Source` 字段、`writeConfig` 过滤、`FindPlugin` |
| `internal/llm/middleware.go`（新增） | `Middleware` 接口与空链实现 |
| `internal/llm/client.go` | 构造时接收中间件链，在请求前/增量/响应后/错误处调用 |
| `internal/llm/react_agent.go` | 同上；`ctx.iteration` 传递；保持流式与工具循环行为不变 |
| `internal/llm/group.go` | 路由确定后应用中间件，中间件不得改目标 |
| `internal/ui/app.go` | `NewChatWindow` 中加载插件（先于 `startMCP`）；重载入口；错误状态 |
| `internal/ui/tools.go`、`selection.go` | `plugin:` toolMeta |
| `internal/ui/provider_client.go` | `buildEinoToolsEnv` 增加 `plugin:` 分支；把中间件链注入三个客户端 |
| `internal/ui/skills.go` | 合并插件 Skill 并标注来源 |
| `internal/ui/theme/palettes.go` | `RegisterThemes()`，支持运行时追加与移除 |
| `internal/ui/settings.go` + 新增 `settings_plugin.go` | 插件 tab、安装/权限/设置表单/更新 UI |
| `internal/ui/mcp.go`、`acp.go`、`agents.go` | 来源徽标；插件项只读 |
| `docs/PLUGINS.md` + `examples/plugins/` | 开发者文档与可运行示例 |

## 13. 测试策略

- **单元测试**
  - 清单校验：缺字段、非法 id、非法权限、engines 不匹配、路径穿越。
  - 合并/回收：启用后各列表出现，禁用后全部消失且当前选择回退。
  - 保存过滤：`writeConfig` 不写 `Source != ""` 的条目，`loadFile` 不误删。
  - 权限：未授权命名空间不存在；`http` 域名白名单与重定向校验；`env` 逐 key。
  - 运行时：超时中断、内存上限、`InternalError` 后 Runtime 重建、并发池排队。
  - 中间件：匹配/优先级/超时跳过/流式增量改写/错误改写与单次重试；空链行为与改造前一致。
  - 安装：zip 路径穿越、摘要校验、扩权拦截、版本降级拦截。
- **集成测试**：`testdata/plugin` 提供 fake host，跑通“加载 → 工具调用 → 中间件 → 禁用”全流程。
- **手工 QA**：安装四种来源各一次；权限拒绝路径；插件死循环；长时间流式输出下 `onChunk` 性能；主题回退；重启后状态保持。

## 14. 风险与缓解

| 风险 | 缓解 |
|---|---|
| moejs 仍是 alpha（`v0.1.0-alpha.6`），API 可能变动 | `internal/plugin` 对 moejs 做薄封装，升级只改一层；升级时回归 `jssandbox` |
| 中间件在流式热路径上 | 单次超时 + 连续慢调用自动停用 + 序列化上限；中间件链路可整体关闭 |
| eino 不暴露底层模型参数 | 中间件只做消息/增量/回复/错误四层，不承诺改 temperature 等调用参数；需要时单独评估 |
| `Source` 注入的隐形耦合 | 保存路径统一过滤 + 断言测试；注入集中在 `plugin.Manager` 一处 |
| native UI 主题能力有限 | 只开放颜色 token，文档与 UI 明确说明 |
| 跨平台路径与 zip 行为差异 | 统一 `filepath` + 显式路径校验单测（Windows 反斜杠、盘符） |
| 插件质量不可控 | 错误隔离 + fail-open 降级 + 插件页可见的耗时与错误统计 |


## 附录 A：其它可扩展点分析

> 本文只做盘点。其中 8 个扩展点（事件钩子、斜杠命令、上下文提供者、导入器、任务触发器、渠道驱动、声明式渲染、声明式 App）的详细设计见 `2026-11-08-plugin-extension-points-design.md`。

除首批七类能力外，代码里还有一批天然接缝。按“与现有架构的契合度 / 成本 / 风险”分三档。

### A.1 三个基础缺口（多数扩展点的前置条件）

| 缺口 | 现状 | 补上后解锁 |
|---|---|---|
| 通用事件钩子 | 只有模型中间件（`beforeRequest` 等），没有会话/工具/任务级事件 | 通知渠道、统计上报、自动任务触发器、外部审计 |
| 宿主 HTTP 回调端点 | `webui/server.go` 路由固定，插件无法接收外部请求 | Webhook 渠道、Webhook 触发器、OAuth 回调 |
| 声明式渲染协议 | `models.Segment.Kind` 只有 `reasoning/text/tool`，`appDef` 是 Go 闭包 | 自定义消息段、数据型 App |

事件钩子建议形态：

```json
"events": ["conversation.completed", "tool.afterCall", "task.finished"]
```

```js
export const events = {
  onEvent(name, payload, ctx) { /* 只读旁路；超时/抛错直接跳过 */ },
};
```

### A.2 第一梯队：与现有七类同构，建议 P1

| 扩展点 | 现有位置 | 插件提供 | 成本 | 关键约束 |
|---|---|---|---|---|
| 斜杠命令 | `internal/ui/composer_menu.go:39` `acpCommands()`、`:63` `slashSkills()` | `commands: [{name, description, handler}]` | 低 | 已有两条 slash 来源，合并即可；命令 handler 结果作为用户消息或直接执行工具 |
| 上下文提供者（@） | `composer_menu.go:252` `contextItems()`、`:298` `listWorkspaceFiles()` | 外部数据源（Jira / Notion / 数据库 / 知识库） | 中 | 异步搜索需超时；条目需截断预览；只传引用，按需 `resolve(id)` 取正文 |
| 导入器 | `internal/agentimport/source.go:38` `Sources()` 硬编码 | 从 Cursor / Windsurf / Cline 等导入 | 低 | 纯声明式目录+解析规则，不执行外部代码 |
| 通知渠道 | `internal/ui/notify.go:63` `notifySessionComplete()` | Webhook / IM 完成通知 | 低 | 依赖 A.1 事件钩子；失败静默，不打断任务 |
| 自动任务触发器 | `config.Schedule` + `internal/ui/autotask.go:534` `nextRun()` | cron、固定间隔、文件变化、Webhook、Git hook | 中 | 需要把 `Schedule` 抽象为 `Trigger` 接口；Webhook 依赖 A.1 端点 |
| 渠道驱动 | `internal/channel/driver.go:122` `RegisterDriver()` | 通用 Webhook / 私有 IM | 中高 | `Driver` 是长连接接口，JS 侧需宿主桥接收发与重连；需要 A.1 端点 |

### A.3 第二梯队：需要新宿主能力，P2

| 扩展点 | 现有位置 | 插件提供 | 成本 | 说明 |
|---|---|---|---|---|
| 知识库解析/分块/检索 | `internal/kb/ingest.go:114`、`search.go:35` | PDF/Office 解析、分块策略、embedding、rerank | 高 | 目前是纯文本 + FTS5；检索在同步路径上，调用 JS 必须超时 + fail-open，或走离线索引任务 |
| 消息段渲染器 | `pkg/models` `Segment.Kind` | 表格、图片、图表、diff、链接卡 | 中高 | 只能映射到白名单组件，插件不能画原生 UI；依赖 A.1 渲染协议 |
| App 贡献 | `internal/ui/apps.go:18` `appDef` | 工具集 + 数据视图 | 高 | `Body` 是 Go 闭包；需先做“声明式 App”（tabs + 视图类型 + 数据源） |
| 模型路由策略 | `config.GroupStrategies()`、`internal/llm/group.go` | 成本感知 / 延迟感知 / 加权 | 中 | 与“中间件不得改目标”冲突，需独立 `routing` 扩展点与明确预算；选路失败必须有兜底 |
| Git 托管集成 | `internal/ui/git_app.go`、`convgit.go` | PR 创建、Issue 关联、commit message | 中低 | 用工具已能实现，缺的是 UI 入口；可先做工具 + 命令 |

### A.4 不建议开放

| 方向 | 原因 |
|---|---|
| 终端 / Shell 集成 | 与 ACP、commandline 工具能力重叠，开放后安全面过大 |
| 检查点 VCS 后端 | 价值低，`checkpoint.go` 与 git 语义强绑定 |
| i18n 语言包 | 界面文案大量硬编码，改造面远超插件收益 |
| 图标包 / 字体 | 原生 UI，收益低 |
| Web API 路由 | 鉴权与跨域边界复杂，且与原生 UI 能力重复 |
| 任意配置 / 布局修改 | 破坏一致性、难以回滚，且无法保证升级兼容 |
| OAuth / 凭据管家 | 安全边界过大，应作为独立子系统而非插件能力 |
| HTTP transport 拦截 | eino 不暴露传输层，插件无法可靠实现 |
| 执行任意 Go 代码 | 违背 moejs 沙箱模型 |

### A.5 建议落地顺序

1. **P1-a**：通用事件钩子（A.1）—— 解锁通知、统计、触发器，改动集中在宿主事件点，风险低。
2. **P1-b**：斜杠命令 + 上下文提供者 + 导入器 —— 三处都是“列表合并”型改动，复用现有 UI。
3. **P1-c**：自动任务触发器（cron/间隔/文件变化）—— 把 `Schedule` 抽象为 `Trigger`，Webhook 触发随后补。
4. **P1-d**：宿主 HTTP 回调端点 + Webhook 渠道 —— 安全评审后再开。
5. **P2**：知识库管线 → 声明式渲染协议 → 声明式 App → 路由策略。

