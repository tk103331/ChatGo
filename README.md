# ChatGo

**A simple AI chat client built for learning purposes.**

A simple cross-platform AI chat client built with Go and Fyne, supporting multiple LLM providers.

![ChatGo](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/license-MIT-blue)
![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-lightgrey)

## ✨ Features

- 🎨 **Modern UI** - Clean and elegant cross-platform GUI based on Fyne
- 🏠 **Quick Start Home** - Centered input box for instant conversations
- 💬 **Streaming Output** - Real-time AI responses, word by word
- 🤖 **Multi-Model Support** - Supports OpenAI, Claude, Ollama, Qwen, DeepSeek, Gemini, and more
- 💾 **Chat Persistence** - Auto-save chat history with multi-session management
- ⚙️ **Flexible Configuration** - Visual configuration interface for easy API key and model management
- 🔧 **Custom Provider** - Support for OpenAI-compatible API endpoints
- 🚀 **Async Processing** - Non-blocking UI for smooth user experience

## 📸 Screenshots

### Home Page
Clean home page design with centered input box for quick conversation start

### Chat Interface
- Session list on the left, supporting create, edit, and delete operations
- Message area on the right with Markdown rendering and streaming output
- Model selector at the top for quick LLM switching

## 🚀 Quick Start

### Installation

#### Build from Source

```bash
# Clone the repository
git clone https://github.com/tk103331/ChatGo.git
cd ChatGo

# Build
go build -o chatgo ./cmd/chatgo

# Run
./chatgo
```

#### Install with Go

```bash
go install github.com/tk103331/ChatGo/cmd/chatgo@latest
```

### First Use

1. After launching the app, you'll see a centered input box on the home page
2. Type your message and click "Send" or press Enter
3. The system will automatically create a new session and enter the chat interface
4. Click the "Settings" button to configure your API keys and models

## ⚙️ Configuration

### Supported Providers

ChatGo supports the following LLM providers:

| Provider | Type | Description |
|----------|------|-------------|
| **OpenAI** | `openai` | OpenAI Official API (GPT-4, GPT-3.5, etc.) |
| **Claude** | `claude` | Anthropic Claude (Claude 3.5 Sonnet, etc.) |
| **Ollama** | `ollama` | Locally deployed open-source models |
| **Qwen** | `qwen` | Alibaba Tongyi Qianwen |
| **DeepSeek** | `deepseek` | DeepSeek AI |
| **Gemini** | `gemini` | Google Gemini |
| **Custom** | `custom` | Any OpenAI-compatible API |

### Configuration File

The configuration file is automatically saved at:
- **Windows**: `C:\Users\<User>\AppData\Roaming\chatgo\config.yaml`
- **macOS**: `~/Library/Application Support/chatgo/config.yaml`
- **Linux**: `~/.config/chatgo/config.yaml`

### Configuration Example

```yaml
providers:
  - name: "OpenAI"
    type: "openai"
    api_key: "sk-..."
    base_url: "https://api.openai.com/v1"
    model: "gpt-4"

  - name: "Claude"
    type: "claude"
    api_key: "sk-ant-..."
    model: "claude-3-5-sonnet-20241022"

  - name: "Ollama"
    type: "ollama"
    base_url: "http://localhost:11434"
    model: "llama3.2"

  - name: "Qwen"
    type: "qwen"
    api_key: "sk-..."
    model: "qwen-max"

mcp_servers: []
current_provider: "OpenAI"
```

### Configure in UI

1. Click the "Settings" button in the bottom right corner
2. Select the "Providers" tab
3. Click "Add New" to add a new Provider
4. Fill in the configuration:
   - **Name**: Provider name (arbitrary)
   - **Type**: Select provider type
   - **API Key**: API key (not required for some providers)
   - **Base URL**: API endpoint (optional)
   - **Model**: Model name
5. Click "Save" to save

## 💡 Usage Tips

### Shortcuts

- **Enter**: Send message
- **Shift + Enter**: New line in input box

### Session Management

- **New Session**: Click the "New Chat" button in the top left
- **Switch Session**: Click on a session in the left list
- **Edit Title**: Click the edit icon next to the session
- **Delete Session**: Click the delete icon next to the session

### Switching Models

- Select a different Provider from the dropdown menu above the message input box
- New messages will use the selected model after switching

## 🛠️ Tech Stack

- **Go 1.21+** - Main programming language
- **Fyne** - Cross-platform GUI framework
- **Cloudwego Eino** - LLM abstraction layer and component library
- **SQLite** - Chat history storage

## 📦 Project Structure

```
ChatGo/
├── cmd/
│   └── chatgo/
│       └── main.go          # Application entry point
├── internal/
│   ├── config/              # Configuration management
│   ├── llm/                 # LLM client
│   ├── mcp/                 # MCP server support
│   └── ui/                  # User interface
├── pkg/
│   └── models/              # Data models
└── README.md
```

## 🤝 Contributing

Contributions are welcome! Please feel free to submit issues or pull requests.

## 📄 License

MIT License - See [LICENSE](LICENSE) file for details

## 🙏 Acknowledgments

- [Fyne](https://fyne.io/) - Excellent cross-platform Go GUI framework
- [Cloudwego Eino](https://github.com/cloudwego/eino) - Powerful LLM application development framework

## 📮 Contact

- GitHub: [@tk103331](https://github.com/tk103331)
- Issues: [GitHub Issues](https://github.com/tk103331/ChatGo/issues)

---

⭐ If this project helps you, please give it a star!
