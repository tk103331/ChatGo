# ChatGo

**一个简单的 AI 聊天客户端，仅用于学习目的。**

一个简单的跨平台 AI 聊天客户端，使用 Go、[MyGo](https://github.com/egoist/mygo) 与 [MujicaUI](https://git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI) 构建，支持多模型、Agent、工具、MCP、ACP 以及内置终端。

![ChatGo](https://img.shields.io/badge/Go-1.27.1+-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/license-MIT-blue)
![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-lightgrey)

## ✨ 特性

- 🎨 **原生界面** - MyGo GPU 自绘 + MujicaUI 组件库（象牙白/炭黑酒红主题），自绘标题栏
- 🏠 **首页快速开始** - 居中输入框 + 能力卡片，并可选项目（工作区）与 Agent
- 🤖 **多 Provider、多模型** - 支持 OpenAI、Anthropic、Claude、Ollama、Qwen、DeepSeek、Gemini 及任意 OpenAI 兼容接口；每个 Provider 可配置多个模型，每种类型自带一条默认不启用的示例配置与品牌 logo
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

> **依赖提示**：MujicaUI 尚未发布到公共代理，首次拉取依赖请让 Go 直连 git.zacharyzhang.com：
> ```bash
> go env -w GOPROXY=https://goproxy.cn,direct
> go env -w GOPRIVATE=git.zacharyzhang.com/*
> ```
> MyGo 为纯 Go 实现（无 cgo），可在任意平台交叉编译原生 UI。内置终端在运行时加载 Ghostty 的 `libghostty-vt`（首次下载到用户缓存，或用 `mygo build` 打包进应用）。

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
| **Custom** | `custom` | 任何 OpenAI 兼容的 API |

每个 Provider 可列出多个模型，模型选择器会全部列出。

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

mcp_servers: []
current_provider: "OpenAI"
```

### 在界面中配置

- **设置 → 模型**：增删 Provider 及其模型列表（每行显示 Provider logo）
- **设置 → 内置工具**：启用内置工具并填写配置
- **设置 → ACP**：管理本地 ACP 连接（Claude Code、OpenCode 等）
- **设置 → 外观**：主题模式与语言
- **Agent** 页：创建自定义 Agent（模型、内置工具、MCP 服务器、Skill）
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

### 工具、MCP 与 Skill

在输入框工具栏勾选内置工具、整台 MCP 服务器与 Skill；每个菜单都带搜索框。

### 面板与终端

标题栏可切换会话列表、文件树与终端。双击文件打开多 Tab 预览（代码/图片/SVG/差异）；终端支持多 Tab。

## 🛠️ 技术栈

- **Go 1.27.1+** - 主要编程语言
- **[MyGo](https://github.com/egoist/mygo)** - 原生 UI 框架（GPU 自绘，无 webview、无 cgo）
- **[MujicaUI](https://git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI)** - MyGo 桌面组件库
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
│   └── ui/                  # MyGo + MujicaUI 界面
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
│       ├── theme.go         # 主题令牌与配色主题
│       └── assets/          # 内嵌品牌图标、Provider 与 Agent 图标
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
- [MujicaUI](https://git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI) - MyGo 桌面组件库
- [Cloudwego Eino](https://github.com/cloudwego/eino) - 强大的 LLM 应用开发框架
- [simple-icons](https://simpleicons.org) - Provider 品牌图标

## 📮 联系方式

- GitHub: [@tk103331](https://github.com/tk103331)
- Issues: [GitHub Issues](https://github.com/tk103331/ChatGo/issues)

---

⭐ 如果这个项目对你有帮助，请给个 Star！
