# ChatGo

**一个简单的 AI 聊天客户端，仅用于学习目的。**

一个简单的跨平台 AI 聊天客户端，使用 Go 与 [MyGo](https://github.com/egoist/mygo) 构建，支持多模型、Agent、工具、MCP、ACP 以及内置终端。

![ChatGo](https://img.shields.io/badge/Go-1.27.1+-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/license-MIT-blue)
![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-lightgrey)

## ✨ 特性

- 🎨 **原生界面** - MyGo GPU 自绘，沿用其原生浅色/深色主题，自绘标题栏
- 🏠 **首页快速开始** - 居中输入框 + 能力卡片，并可选项目（工作区）与 Agent
- 🤖 **多 Provider、多模型** - 支持 OpenAI、Anthropic、Claude、Ollama、Qwen、DeepSeek、Gemini、本机 magpie 网关及任意 OpenAI 兼容接口；每个 Provider 可配置多个模型，每种类型自带一条默认不启用的示例配置与品牌 logo
- 🧩 **Agent** - 内置 Chat、自定义 Agent（模型/内置工具/MCP/Skill 白名单）、ACP Agent（本地编码 CLI）
- 🔧 **工具与 MCP** - 按会话/Agent 勾选内置工具与整个 MCP 服务器，支持检索；内置 MCP 市场
- 📚 **Skill** - 扫描本地 SKILL 目录，按 Agent 选择并支持检索
- 🗂️ **工作区与面板** - 会话按工作区分组、可折叠；聊天/预览/文件面板可拖拽；多 Tab 文件预览与 Git 差异面板
- ⌨️ **终端** - 底部多 Tab 终端（Ghostty VT）
- 💬 **流式输出** - 实时流式应答，Markdown 渲染
- 💾 **持久化** - 配置与对话历史保存在 `~/.chatgo`

## 📸 截图

### 首页
居中输入框与能力卡片，底部可选项目与 Agent。

### 聊天
左侧按工作区分组的会话列表，右侧 Markdown 消息区、模型/Agent/工具/Skill 控件，可选文件预览与 Git 面板，底部终端。

## 🚀 快速开始

### 安装

#### 从源码编译

```bash
# 克隆仓库
git clone https://github.com/tk103331/ChatGo.git
cd ChatGo

# 编译（需要 Go 1.27.1+，会自动下载匹配的工具链）
go build -o chatgo .

# 运行
./chatgo
```

> **依赖提示**：MyGo 为纯 Go 实现（无 cgo），可在任意平台交叉编译原生 UI。内置终端在运行时加载 Ghostty 的 `libghostty-vt`（首次下载到用户缓存，或用 `mygo build` 打包进应用）。

#### 使用 Go 安装

```bash
go install github.com/tk103331/ChatGo@latest
```

### 初次使用

1. 启动应用，首页显示居中的输入框
2. 可选选择项目与 Agent，输入消息后回车
3. 在设置页配置 Provider、工具与 Agent

## ⚙️ 配置

### 支持的 Provider

| Provider | 类型 | 说明 |
|----------|------|------|
| **OpenAI** | `openai` | OpenAI（GPT-4o、o1 等）及 OpenAI 兼容接口 |
| **Anthropic** | `anthropic` | Anthropic Messages API |
| **Claude** | `claude` | Anthropic Claude |
| **Ollama** | `ollama` | 本地部署的开源模型 |
| **Qwen** | `qwen` | 阿里通义千问 |
| **DeepSeek** | `deepseek` | DeepSeek AI |
| **Gemini** | `gemini` | Google Gemini |
| **magpie** | `magpie` | 本机运行的 [magpie](https://usemagpie.ai) 网关，把 magpie 里配置的全部模型以 OpenAI 兼容接口暴露在 `http://127.0.0.1:3425/v1`，无需 API Key |
| **Custom** | `custom` | 任意 API；通过 `api_type` 选择上游协议（`openai`、`openai_responses`、`claude`、`gemini`），请求直接按该协议发送，不经中间转换 |

每个 Provider 可列出多个模型，模型选择器会全部列出。

### 模型组

模型组把多个模型聚合成一个逻辑模型名，在模型选择器里与普通模型一样可选。
每个成员先选 Provider，再勾选该 Provider 下要参与路由的模型。支持三种路由策略：

| 策略 | 取值 | 行为 |
|------|------|------|
| 故障迁移 | `fallback` | 固定先请求第一个成员，未产生任何输出就失败时换下一个 |
| 依次轮询 | `round_robin` | 每次请求轮换起始成员，失败后按顺序重试其余成员 |
| 随机访问 | `random` | 随机选择起始成员，失败后按顺序重试其余成员 |

当所选模型不在某个成员下时，该成员改用它自己勾选的其他模型，而不是被跳过。
已经流式输出或执行过工具的请求不会重试，避免重复输出。在**设置 → 模型**页点击**新增组**创建和编辑模型组。

### 配置文件

配置文件保存在 `~/.chatgo/config.yaml`（各平台一致）；对话历史保存在 `~/.chatgo/conversations`。

### 配置示例

```yaml
providers:
  - name: "OpenAI"
    type: "openai"
    api_key: "sk-..."
    base_url: "https://api.openai.com/v1"
    model: "gpt-4o"
    models: ["gpt-4o", "gpt-4o-mini"]

  - name: "Ollama"
    type: "ollama"
    base_url: "http://localhost:11434"
    model: "llama3.2"
    enabled: true

  # custom：用 api_type 指定上游协议
  - name: "MyClaudeProxy"
    type: "custom"
    api_type: "claude"          # openai | openai_responses | claude | gemini
    api_key: "sk-..."
    base_url: "https://proxy.example.com"
    model: "claude-sonnet-4-5"
    models: ["claude-sonnet-4-5"]
    enabled: true

provider_groups:
  - name: "Claude Pool"
    strategy: "fallback"       # fallback | round_robin | random
    enabled: true
    members:
      - provider: "MyClaudeProxy"
        models: ["claude-sonnet-4-5"]

mcp_servers: []
current_provider: "OpenAI"
```

### 在界面中配置

- **设置 → 模型**：增删 Provider 及其模型列表（每行显示 Provider logo）
- **设置 → 模型 → 新增组**：把多个 Provider 的模型聚合成一个带路由策略的模型名
- **设置 → 内置工具**：启用内置工具并填写配置
- **设置 → ACP**：管理本地 ACP 连接（Claude Code、OpenCode 等）
- **设置 → 外观**：主题模式与语言
- **Agent** 页：创建自定义 Agent（模型、内置工具、MCP 服务器、Skill）
- **知识库** 页：新建知识库、导入文档、重建索引与检索测试
- **MCP** 页：我的服务与内置市场

## 💡 使用

### 快捷键

- **Enter**：发送
- **Shift + Enter**：换行

### 会话

按工作区分组；支持新建、重命名、删除与搜索；可在标题栏隐藏/显示列表。

### Agent

- **内置 Chat**：记住全局的模型/工具/MCP/Skill 选择，下次启动自动恢复
- **自定义 Agent**：在设置里定义可用白名单（模型可多选），默认全启用，聊天中可临时关闭
- **ACP Agent**：按 ACP 协议提供的模式与选项控制

### 知识库

**知识库**页管理本地文本知识库。文档按 Markdown 标题切块，经分词（中文切成重叠二元组、
英文小写化）后写入 SQLite FTS5，检索完全离线、不需要任何嵌入服务。导入是增量的：
内容哈希未变的文件会被跳过。

- 支持导入单个文件、整个目录（自动跳过隐藏目录）或粘贴文本
- 支持的文本类型：Markdown、txt、rst/adoc/org、JSON/YAML/TOML/INI、CSV、日志与常见源码文件
- `重建索引` 会重新读取所有文件型文档，并移除磁盘上已删除的文档
- 内置检索测试框会显示命中片段与 BM25 分数，便于调参
- 内置工具 `knowledge_search` 把同一套检索暴露给 Agent：在**设置 → 工具**中启用，可指定默认知识库与 `top_k`

数据保存在 `~/.chatgo/knowledge/kb.db`。全部为纯 Go 实现（`modernc.org/sqlite`），
因此仍可用 `CGO_ENABLED=0` 构建。

### 工具、MCP 与 Skill

在输入框工具栏勾选内置工具、整台 MCP 服务器与 Skill；每个菜单都带搜索框。

### 面板与终端

标题栏可切换会话列表、文件树与终端。双击文件打开多 Tab 预览（代码/图片/SVG/差异）；终端支持多 Tab。

## 🛠️ 技术栈

- **Go 1.27.1+** - 主要编程语言
- **[MyGo](https://github.com/egoist/mygo)** - 原生 UI 框架（GPU 自绘，无 webview、无 cgo）
- **Ghostty VT** - 通过 MyGo 终端插件提供终端模拟
- **Cloudwego Eino** - LLM 抽象层与 React Agent 框架
- **JSON 文件** - 对话历史存储（`~/.chatgo/conversations`）

## 📦 项目结构

```
ChatGo/
├── main.go                  # 应用入口
├── internal/
│   ├── config/              # 配置（Provider/Agent/工具/预置示例）
│   ├── llm/                 # LLM 客户端与 React Agent
│   ├── mcp/                 # MCP 服务器支持
│   ├── jssandbox/           # 隔离的 JS/TS 沙箱运行时
│   └── ui/                  # MyGo 原生界面
│       ├── app.go           # 状态、视图入口、窗口与标题栏
│       ├── provider_client.go # Provider/模型客户端、工具与 MCP 启动
│       ├── conversation.go  # 会话、流式、标题与分段
│       ├── notify.go        # Toast 与会话通知
│       ├── welcome.go       # 首页
│       ├── chat.go          # 聊天输入与工具栏
│       ├── chatpanel.go     # 文件树、预览 Tab 与 Git 面板
│       ├── sidebar.go       # 按工作区分组的会话列表
│       ├── message.go       # 消息行与操作
│       ├── agents.go        # Agent 管理与编辑
│       ├── skills.go        # Skill 扫描与选择
│       ├── mcp.go           # MCP 页面与市场
│       ├── acp.go           # ACP 连接与目录
│       ├── acp_client.go    # ACP 客户端回调与会话更新
│       ├── acp_session.go   # ACP 会话生命周期与运行时
│       ├── terminal.go      # 多 Tab 终端面板
│       ├── settings*.go     # 设置标签页（模型/MCP/工具/外观/渠道）
│       ├── selection.go     # 可用性白名单
│       ├── models.go        # 模型引用
│       ├── theme.go         # 外观设置持久化
│       ├── kit/             # 基于 MyGo 图元自绘的 ChatGo 组件
│       ├── uikit/           # 窗口主题运行时与共享绘制
│       ├── theme/           # ChatGo 组件共享的尺寸度量
│       ├── icons/           # 内嵌 Lucide 图标
│       └── assets/          # 内嵌品牌标识、服务商与 Agent 图标
├── pkg/
│   └── models/              # 数据模型
└── README.md
```

## 🤝 贡献

欢迎贡献！请随时提交 Issue 或 Pull Request。

## 📄 许可证

MIT License - 详见 [LICENSE](LICENSE) 文件

## 🙏 致谢

- [MyGo](https://github.com/egoist/mygo) - 原生 UI 框架
- [Cloudwego Eino](https://github.com/cloudwego/eino) - 强大的 LLM 应用开发框架
- [simple-icons](https://simpleicons.org) - Provider 品牌图标

## 📮 联系方式

- GitHub: [@tk103331](https://github.com/tk103331)
- Issues: [GitHub Issues](https://github.com/tk103331/ChatGo/issues)

---

⭐ 如果这个项目对你有帮助，请给个 Star！
