# ChatGo 工具列表说明

本文档说明 ChatGo 中可用的三类工具能力：**内置工具（Built-in Tools）**、**MCP 服务器** 与 **Skills**。三者都可以在对话输入框中按会话/Agent 分别开关，并支持搜索。

---

## 目录

- [一、内置工具（Built-in Tools）](#一内置工具built-in-tools)
- [二、MCP 服务器](#二mcp-服务器)
- [三、Skills](#三skills)
- [四、工具的组合与作用范围](#四工具的组合与作用范围)
- [五、配置文件片段参考](#五配置文件片段参考)

---

## 一、内置工具（Built-in Tools）

内置工具由 [Cloudwego Eino](https://github.com/cloudwego/eino) 框架提供，在 **设置 → 内置工具** 中启用和配置。部分工具需要填写必需的配置项（如 API Key）后才能使用。

下表按工具类型列出说明、启用状态、配置项。

| 类型标识 | 名称 | 说明 | 默认状态 | 配置项 | 必填项 |
|----------|------|------|----------|--------|--------|
| `bingsearch` | Bing Search | 使用 Bing 搜索引擎进行网页搜索 | 关闭 | `api_key` | `api_key` |
| `googlesearch` | Google Search | 使用 Google 搜索引擎进行网页搜索 | 关闭 | `api_key`、`search_engine_id` | `api_key`、`search_engine_id` |
| `wikipedia` | Wikipedia | 在 Wikipedia 中搜索和获取信息 | 关闭 | `language` | — |
| `duckduckgosearch` | DuckDuckGo Search | 使用 DuckDuckGo 进行隐私搜索 | 关闭 | — | — |
| `httprequest` | HTTP Request | 向 Web 服务发起 HTTP 请求 | **开启** | `timeout`、`max_redirects` | — |
| `browseruse` | Browser Use | 自动化浏览器交互 | 关闭 | `headless`、`timeout` | — |
| `commandline` | Command Line | 执行 Shell 命令（谨慎使用） | **开启** | `allowed_commands` | `allowed_commands` |
| `filesystem` | Filesystem | 读取、写入或列出工作目录下的文件 | **开启** | — | — |
| `sequentialthinking` | Sequential Thinking | 思维链推理工具 | 关闭 | `max_iterations` | — |

### 说明要点

- **默认开启的工具**：`httprequest`、`commandline`、`filesystem` 开箱即用，并带有一套合理的默认配置。
- **需要凭据的搜索工具**：`bingsearch`、`googlesearch` 需要 API Key；`googlesearch` 还需要自定义搜索引擎 ID（`search_engine_id`）。
- **命令执行安全**：`commandline` 通过 `allowed_commands` 白名单限制可执行的命令，默认白名单为
  `ls, pwd, cat, echo, grep, find, head, tail, wc`。**该字段为必填的安全要求**，请勿随意放宽。
- **默认配置值**：
  - `httprequest`：`timeout=30`、`max_redirects=5`
  - `browseruse`：`headless`、`timeout` 有默认值
  - `sequentialthinking`：`max_iterations` 有默认值

### 工具调用行为

- 工具执行时，UI 会收到 `ToolCallInfo`，包含工具名、参数、结果、耗时与错误信息，用于在消息流中展示。
- 工具失败时，**不会中断整轮对话**，而是把错误信息作为工具结果返回给模型，让模型自行重试或调整策略。
- React Agent 的 **最大步数（MaxStep）** 默认 `40`，用于限制工具调用轮次，可在配置中调整。

---

## 二、MCP 服务器

MCP（Model Context Protocol）服务器为模型提供扩展工具集。你可以在 **MCP 页面** 管理自建服务器，并使用内置的 **MCP 市场（Marketplace）** 一键添加。

### 支持的连接类型

| 类型 | 标识 | 说明 |
|------|------|------|
| 标准输入输出 | `stdio` | 以子进程方式通过标准输入输出通信，最常用 |
| Server-Sent Events | `sse` | 通过 SSE 长连接通信 |
| 流式 HTTP | `streamable_http` | 通过流式 HTTP 通信 |

### 配置字段

| 字段 | 适用类型 | 说明 |
|------|----------|------|
| `name` | 全部 | 服务器名称（唯一标识） |
| `type` | 全部 | 连接类型：`stdio` / `sse` / `streamable_http` |
| `enabled` | 全部 | 是否启用 |
| `command` | `stdio` | 启动命令（如 `npx`） |
| `args` | `stdio` | 命令参数 |
| `env` | `stdio` | 环境变量 |
| `url` | `sse` / `streamable_http` | 服务地址 |
| `headers` | `sse` / `streamable_http` | 请求头 |
| `timeout_seconds` | `sse` / `streamable_http` | 超时时间（秒） |

### 默认内置的 MCP 服务器

首次运行时，ChatGo 会预置一个 **filesystem** MCP 服务器示例：

```yaml
- name: "filesystem"
  type: "stdio"
  enabled: true
  command: "npx"
  args: ["-y", "@modelcontextprotocol/server-filesystem", "<HOME>"]
```

可在对话输入框按会话整体开关某个 MCP 服务器，并支持搜索。

---

## 三、Skills

ChatGo 会扫描本地 SKILL 目录。你可以在对话输入框按会话选择 Skill，或在 **Agent** 页面为自定义 Agent 配置可用 Skill 白名单。Skills 菜单支持搜索。

- **扫描来源**：本地 SKILL 目录。
- **选择方式**：按会话/Agent 多选，默认全部可用。
- **与 Agent 的关系**：自定义 Agent 可配置专属 Skill 白名单；内置 Chat 会记住你上次的全局选择并在启动时恢复。

---

## 四、工具的组合与作用范围

ChatGo 的工具能力围绕 **Agent** 组织，作用范围分三个层级：

| 层级 | 说明 |
|------|------|
| **内置 Chat** | 记住全局的 模型 / 工具 / MCP / Skill 选择，启动时自动恢复 |
| **自定义 Agent（Custom Agents）** | 在设置中定义可用白名单（模型可多选），默认全部启用，对话中可临时开关 |
| **ACP Agent** | 本地编码 CLI（Claude Code、OpenCode 等），暴露协议提供的模式与选项 |

### 默认预置的 ACP 连接

| 名称 | 命令 | 参数 |
|------|------|------|
| Claude Code | `npx` | `-y @zed-industries/claude-code-acp` |
| OpenCode | `opencode` | `acp` |

> 自定义 Agent 还可配置：系统提示词（System Prompt）、模型集合、内置工具、MCP 服务器、Skills 以及工作区（Workspace）。

---

## 五、配置文件片段参考

配置文件位于 `~/.chatgo/config.yaml`。以下为内置工具的配置片段示例：

```yaml
builtin_tools:
  - name: httprequest
    type: httprequest
    enabled: true
    config:
      timeout: "30"
      max_redirects: "5"
  - name: commandline
    type: commandline
    enabled: true
    config:
      allowed_commands: "ls, pwd, cat, echo, grep, find, head, tail, wc"
  - name: filesystem
    type: filesystem
    enabled: true
  - name: bingsearch
    type: bingsearch
    enabled: false
    config:
      api_key: ""

# React Agent 相关
use_react_agent: false
react_agent_max_step: 40

# MCP 服务器
mcp_servers:
  - name: filesystem
    type: stdio
    enabled: true
    command: npx
    args: ["-y", "@modelcontextprotocol/server-filesystem", "/Users/you"]
```

---

## 附：工具类型速查

| 分类 | 工具 |
|------|------|
| 搜索 | `bingsearch`、`googlesearch`、`duckduckgosearch`、`wikipedia` |
| 网络 | `httprequest` |
| 浏览器 | `browseruse` |
| 系统 | `commandline`、`filesystem` |
| 推理 | `sequentialthinking` |
| 扩展 | MCP 服务器（`stdio` / `sse` / `streamable_http`） |
| 本地 CLI | ACP Agent（Claude Code、OpenCode……） |
| 提示词资产 | Skills |