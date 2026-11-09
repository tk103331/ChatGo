# ChatGo 插件开发指南（P0 + P1）

插件是放在 `~/.chatgo/plugins/<id>/` 下的 JavaScript 包，由 moejs 在进程内沙箱执行。清单声明贡献，`main.js` 只实现行为。

## 快速开始

1. 启动 ChatGo → 设置 → 插件 → 安装插件
2. 来源选“目录”，填 `examples/plugins/weather`（相对仓库根目录的绝对路径也可以）
3. 安装后插件默认禁用；点“启用”，确认权限
4. 返回对话，在工具菜单里能看到 `example-weather__forecast`；外观页会出现“晴空（插件）”；Agent 页会出现 `Weather Helper`；Skill 页会出现 `weather-guide`

## 包结构

```
~/.chatgo/plugins/weather/
├── plugin.json
├── main.js
├── skills/weather-guide/SKILL.md
└── icon.svg            # 可选
```

`plugin.json` 必需的字段：`id`（小写字母/数字/连字符）、`name`、`version`、`main`（可省略，默认 `main.js`）。

## 已实现的贡献类型

| 类型 | 清单字段 | 合并到 |
|---|---|---|
| 工具 | `tools[]` | 输入框工具菜单、Agent 工具白名单 |
| Provider | `providers[]` | 设置 → 模型 |
| 模型中间件 | `middleware[]` | 每次模型请求/流式增量/响应/错误 |
| MCP Server | `mcpServers[]` | MCP 页、工具菜单 |
| ACP Agent | `acpAgents[]` | 设置 → ACP |
| 普通 Agent | `agents[]` | Agent 页、新建会话 |
| Skill | `skills[]` 或 `skills/*/SKILL.md` | Skill 页 |
| 主题 | `themes[]` | 设置 → 外观 |
| 事件钩子 | `events[]` + `export const events` | 只读旁路 |
| 斜杠命令 | `commands[]` | 输入框 `/` 菜单 |
| 上下文提供者 | `contextProviders[]` | 输入框 `@` 菜单 |
| 工具结果钩子 | `toolResult: true` + `export function onToolResult` | 工具结果进入模型前 |

`triggerTypes`、`channels`（webhook / poll）、`importers`（MCP / Provider / Skill）与声明式渲染均已接线。

## 入口模块

```js
export const tools = {
  forecast(args, ctx) {
    ctx.log.info("city", args.city);
    return { city: args.city, tempC: 24 };
  },
  async slow(args, ctx) {          // async 也可以：宿主 API 是同步的，
    const r = ctx.http.get("https://api.example.com/x"); // 返回时 Promise 已结算
    return r.json;
  },
};

export const events = {
  onEvent(name, payload, ctx) {
    ctx.log.info("event", name);
  },
};

export function onToolResult(payload, ctx) {
  // 返回字符串则替换结果；返回其它值表示不修改
  return payload.result.replace(/secret/g, "***");
}

export const middleware = {
  units: {
    beforeRequest(req, ctx) { return { systemAppend: "使用公制单位。" }; },
    onChunk(text, ctx) { return text.replace(/°F/g, "°C"); },
    afterResponse(resp, ctx) { return { text: resp.text.trim() }; },
    onError(err, ctx) { return { message: "模型暂时不可用", retry: false }; },
  },
};
```

## 宿主 API（`ctx`）

只有已授权的命名空间存在；未授权时调用会抛 `TypeError`。

| 命名空间 | 权限 | 说明 |
|---|---|---|
| `ctx.log.debug/info/warn/error` | 无 | 输出到应用日志（`[plugin:<id>]`） |
| `ctx.store.get/set/delete/keys` | `store`（默认可用） | 保存在 `~/.chatgo/plugin-data/<id>/state.json` |
| `ctx.settings.<key>` | 无 | 插件设置值（明文；`secret` 类型在界面上打码） |
| `ctx.http.get/post/put/delete/request` | `http:<host>` | 20s 超时、1 MiB 响应上限、重定向同样校验白名单 |
| `ctx.env.get(key)` | `env:<KEY>` | 逐 key 授权 |
| `ctx.conversation` | 无 | 当前会话 ID（命令/事件/上下文） |

返回的 HTTP 结果：`{ status, body, headers, json? }`；状态码 ≥400 会抛错。

## 模型中间件

- 清单 `middleware[]` 的 `match.providers` / `match.models` 支持 `*` 前缀通配；`priority` 小的先执行。
- 钩子：`beforeRequest`（`{systemAppend, system, messages}`）、`onChunk`（返回字符串替换增量）、`afterResponse`（`{text, reasoning}`）、`onError`（`{message, retry}`）。
- 超时默认 3s、上限 10s；超时/抛错会跳过该钩子，不阻断对话。
- 当前实现中 `beforeRequest` / `afterResponse` 按每次 `Chat` 调用触发（React Agent 一轮内的多次模型调用共享一次），`ctx.iteration` 恒为 0。

## 命令行管理

```bash
chatgo plugin list
chatgo plugin install -kind dir -src ~/my-plugin -enable
chatgo plugin install -kind npm -src chatgo-plugin-weather
chatgo plugin install -kind git -src https://github.com/user/repo.git -ref main
chatgo plugin enable <id>
chatgo plugin disable <id>
chatgo plugin remove <id>
```

`-enable` 会授予清单声明的全部权限；不加时插件保持禁用，等你确认权限。

## 安全

- 默认零权限：`http`、`env`、`exec`、`events`、`context` 都需要在启用前确认。
- 没有 `require`、没有 Node API；只有上表注入的命名空间。
- 单次调用有超时和 64 MiB（可配到 256 MiB）内存上限；出错只影响该插件。
- 插件贡献的名称冲突会被跳过并在插件页显示警告。

## 插件渠道

- 清单 `channels[]` 支持 `mode: "webhook"` 与 `mode: "poll"`；poll 模式需要导出 `poll` 函数，返回 `[{chatId, fromUser, text, replyToken}]`，默认每 10 秒拉取一次（`pollSeconds` 可调，2–300）。
- 在插件详情页点“添加渠道”生成一个禁用的渠道；填好凭证并启用后，渠道卡片会显示 Webhook 地址。
- 外部平台 POST 到该地址时必须带 `?token=<webhook_token>` 或 `X-Chatgo-Token` 头（token 在“添加渠道”时自动生成，可在渠道 Options 中修改）。
- 插件的 `verify(req, ctx)` 里通过 `ctx.channel.app_id` / `app_secret` / `base_url` 与 Options 读取凭证；返回 `{ok, chatId, fromUser, text, replyToken}`。
- 端点只监听 `127.0.0.1`；每渠道 60 次/分钟、请求体 1 MiB 上限。
- 渠道会话自动获得 `plugin_send_message` 工具。

## 插件触发器

- 清单 `triggerTypes[]` 的 `evaluate(trigger, lastRunISO, nowISO)` 必须是纯函数，返回 `{next}` 或 `{next: null}`。
- 自动任务编辑器的“频率”下拉会列出插件触发器，字段按 `fields` 声明渲染。
- 求值超时/抛错会暂停该任务，不会退化成“立即执行”。

## 插件导入器

- 清单 `importers[]` 用 JSON Pointer 风格映射（`/mcpServers`、`@key`）读取其它 Agent 的 json / yaml / toml 配置。
- 支持 `kinds: ["mcp"]`、`["providers"]` 与 `["skills"]`；Skill 导入器用 `dirs` 指向 Skill 根目录，每个含 `SKILL.md` 的子目录是一项候选。
- 只读扫描：路径必须在 home 或当前工作区内，不跟随符号链接，单文件 2 MiB。

## 声明式渲染

工具返回以下信封时，宿主用对应组件渲染，而不是显示一段 JSON：

```js
return {
  $chatgo: {
    v: 1,
    view: {
      type: "table",
      title: "构建结果",
      columns: [{ key: "target", label: "目标" }, { key: "status", label: "状态" }],
      rows: [["web", "ok"], ["desktop", "failed"]],
    },
  },
};
```

| type | 字段 | 上限 |
|---|---|---|
| `markdown` | `text` | 20 KB |
| `table` | `columns[{key,label}]`, `rows[][]` | 12 列 / 500 行 / 单元格 500 字符 |
| `keyvalue` | `items[{label,value}]` | 100 项 |
| `list` | `items[{title,detail,badge}]` | 200 项 |
| `chart` | `labels[]`, `series[{name,values[]}]`, `stacked?` | 100 标签 / 4 系列 |
| `code` | `language`, `code` | 20 KB |
| `diff` | `path`, `before`, `after` | 各 20 KB |
| `links` | `items[{title,url}]` | 50 项，仅 https |
| `image` | `src`, `alt` | 仅 https |

信封总大小 ≤ 64 KB；未知 type、校验失败或超限时自动回退为原始文本。

## 当前未实现

- 声明式 App（左侧工作区视图协议）。
- 动态注册 `activate(host)`、CLI 安装命令、插件市场。
