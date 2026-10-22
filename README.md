# ChatGo

**A simple AI chat client built for learning purposes.**

A simple cross-platform AI chat client built with Go and [MyGo](https://github.com/egoist/mygo), with multi-provider models, Agents, tools, MCP, ACP and a built-in terminal.

![ChatGo](https://img.shields.io/badge/Go-1.27.1+-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/license-MIT-blue)
![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-lightgrey)

## ✨ Features

- 🎨 **Native UI** - MyGo's GPU-drawn interface and its own light and dark themes, with a self-drawn title bar
- 🏠 **Quick Start Home** - Centered composer with capability cards and project (workspace) / Agent selectors
- 🤖 **Multi-Provider, Multi-Model** - OpenAI, Anthropic, Claude, Ollama, Qwen, DeepSeek, Gemini and any OpenAI-compatible API. Each provider holds a list of models; every type ships a disabled example config with its brand logo
- 🧩 **Agents** - Built-in Chat, custom Agents (whitelist of models, built-in tools, MCP servers and Skills) and ACP Agents (local coding CLIs)
- 🔧 **Tools & MCP** - Built-in tools and whole MCP servers toggle per chat/agent, with search; an MCP marketplace is bundled
- 📚 **Skills** - Scans local SKILL directories, with per-agent selection and search
- 🗂️ **Workspaces & Panels** - Sessions grouped by workspace and collapsible; resizable chat / preview / file panels with a multi-tab file preview and a Git diff panel
- ⌨️ **Terminal** - Multi-tab bottom terminal (Ghostty VT)
- 💬 **Streaming Output** - Real-time streaming responses with Markdown rendering
- 💾 **Persistence** - Config and chat history live under `~/.chatgo`

## 📸 Screenshots

### Home
Centered composer with capability cards, plus project and Agent selectors.

### Chat
Workspace-grouped session list, Markdown message area, model / Agent / tool / Skill controls, optional file-preview and Git panels, and a bottom terminal.

## 🚀 Quick Start

### Installation

#### Build from Source

```bash
# Clone the repository
git clone https://github.com/tk103331/ChatGo.git
cd ChatGo

# Build (requires Go 1.27.1+; the matching toolchain is downloaded automatically)
go build -o chatgo .

# Run
./chatgo
```

> **Dependency note:** MyGo is pure Go (no cgo), so the native UI cross-compiles from any platform. The built-in terminal loads Ghostty's `libghostty-vt` at run time (downloaded once into the user cache, or bundled by `mygo build`).

#### Install with Go

```bash
go install github.com/tk103331/ChatGo@latest
```

### First Use

1. Launch the app; the home page shows a centered composer
2. Optionally pick a project and an Agent, type a message, press Enter
3. Configure providers, tools and Agents from the Settings page

## ⚙️ Configuration

### Supported Providers

| Provider | Type | Description |
|----------|------|-------------|
| **OpenAI** | `openai` | OpenAI (GPT-4o, o1, …) and OpenAI-compatible endpoints |
| **Anthropic** | `anthropic` | Anthropic Messages API |
| **Claude** | `claude` | Anthropic Claude |
| **Ollama** | `ollama` | Locally deployed open-source models |
| **Qwen** | `qwen` | Alibaba Tongyi Qianwen |
| **DeepSeek** | `deepseek` | DeepSeek AI |
| **Gemini** | `gemini` | Google Gemini |
| **Custom** | `custom` | Any OpenAI-compatible API |

Each provider can list several models, and the model selector shows them all.

### Configuration File

The configuration is saved at `~/.chatgo/config.yaml` on every platform; chat history lives in `~/.chatgo/conversations`.

### Configuration Example

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

### Configure in UI

- **Settings → Models**: add/edit providers and their model lists (each row shows the provider logo)
- **Settings → Built-in tools**: enable built-in tools and fill their config
- **Settings → ACP**: manage local ACP connections (Claude Code, OpenCode, …)
- **Settings → Appearance**: theme mode and language
- **Agent** page: create custom Agents (models, built-in tools, MCP servers, Skills)
- **MCP** page: your servers plus an embedded marketplace

## 💡 Usage

### Shortcuts

- **Enter**: send
- **Shift + Enter**: new line

### Sessions

Grouped by workspace; create, rename, delete, and search. Hide or show the list from the title bar.

### Agents

- **Built-in Chat** remembers its global model / tool / MCP / Skill selection and reloads it on start
- **Custom Agents** define an availability whitelist in Settings (models are multi-select), all enabled by default and temporary toggles in chat
- **ACP Agents** expose the modes and options the protocol provides

### Tools, MCP & Skills

Toggle built-in tools and whole MCP servers, and pick Skills, from the composer; each menu has a search box.

### Panels & Terminal

The title bar toggles the session list, the file tree and the terminal. Double-click a file for a tabbed preview (code, image, SVG, diff); the terminal supports multiple tabs.

## 🛠️ Tech Stack

- **Go 1.27.1+** - Main programming language
- **[MyGo](https://github.com/egoist/mygo)** - Native UI framework (GPU-drawn, no webview, no cgo)
- **Ghostty VT** - Terminal emulator via MyGo's terminal plugin
- **Cloudwego Eino** - LLM abstraction layer and React Agent framework
- **JSON files** - Chat history storage (`~/.chatgo/conversations`)

## 📦 Project Structure

```
ChatGo/
├── main.go                  # Application entry point
├── internal/
│   ├── config/              # Configuration (providers, agents, tools, presets)
│   ├── llm/                 # LLM client and React Agent
│   ├── mcp/                 # MCP server support
│   ├── jssandbox/           # Isolated JS/TS sandbox runtime
│   └── ui/                  # MyGo native interface
│       ├── app.go           # State, view entry, window and title bar
│       ├── provider_client.go # Provider/model clients, tools and MCP boot
│       ├── conversation.go  # Conversations, streaming, titles, segments
│       ├── notify.go        # Toast and session notifications
│       ├── welcome.go       # Home page
│       ├── chat.go          # Chat composer and toolbar
│       ├── chatpanel.go     # File tree, preview tabs and Git panel
│       ├── sidebar.go       # Workspace-grouped session list
│       ├── message.go       # Message rows and actions
│       ├── agents.go        # Agent management and editor
│       ├── skills.go        # Skill scanning and selection
│       ├── mcp.go           # MCP page and marketplace
│       ├── acp.go           # ACP connections and catalog
│       ├── acp_client.go    # ACP client callbacks and session updates
│       ├── acp_session.go   # ACP session lifecycle and runtime
│       ├── terminal.go      # Multi-tab terminal panel
│       ├── settings*.go     # Settings tabs (providers, mcp, tools, appearance, channels)
│       ├── selection.go     # Availability whitelists
│       ├── models.go        # Model references
│       ├── theme.go         # Appearance persistence
│       ├── kit/             # ChatGo's widgets, drawn on MyGo primitives
│       ├── uikit/           # Window theme runtime and shared drawing
│       ├── theme/           # Metrics shared by ChatGo's widgets
│       ├── icons/           # Embedded Lucide icons
│       └── assets/          # Embedded brand marks, provider and agent icons
├── pkg/
│   └── models/              # Data models
└── README.md
```

## 🤝 Contributing

Contributions are welcome! Please feel free to submit issues or pull requests.

## 📄 License

MIT License - See [LICENSE](LICENSE) file for details

## 🙏 Acknowledgments

- [MyGo](https://github.com/egoist/mygo) - Native UI framework
- [Cloudwego Eino](https://github.com/cloudwego/eino) - Powerful LLM application development framework
- [simple-icons](https://simpleicons.org) - Provider brand marks

## 📮 Contact

- GitHub: [@tk103331](https://github.com/tk103331)
- Issues: [GitHub Issues](https://github.com/tk103331/ChatGo/issues)

---

⭐ If this project helps you, please give it a star!
