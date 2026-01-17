# ChatGo

一个优雅的跨平台AI聊天客户端，使用Go和Fyne构建，支持多种大语言模型提供商。

![ChatGo](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/license-MIT-blue)
![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-lightgrey)

## ✨ 特性

- 🎨 **现代化界面** - 简洁优雅的跨平台GUI，基于Fyne框架
- 🏠 **首页快速开始** - 居中的输入框，一键开始对话
- 💬 **流式输出** - 实时显示AI响应，逐字逐句呈现
- 🤖 **多模型支持** - 支持OpenAI、Claude、Ollama、Qwen、DeepSeek、Gemini等
- 💾 **对话持久化** - 自动保存对话历史，支持多会话管理
- ⚙️ **灵活配置** - 可视化配置界面，轻松管理API密钥和模型参数
- 🔧 **自定义Provider** - 支持OpenAI兼容的API端点
- 🚀 **异步处理** - 非阻塞UI，流畅的用户体验

## 📸 截图

### 首页
简洁的首页设计，居中输入框，快速开始对话

### 聊天界面
- 左侧会话列表，支持创建、编辑、删除会话
- 右侧消息区域，支持Markdown渲染和流式输出
- 顶部模型选择器，快速切换不同的LLM

## 🚀 快速开始

### 安装

#### 从源码编译

```bash
# 克隆仓库
git clone https://github.com/tk103331/ChatGo.git
cd ChatGo

# 编译
go build -o chatgo ./cmd/chatgo

# 运行
./chatgo
```

#### 使用Go安装

```bash
go install github.com/tk103331/ChatGo/cmd/chatgo@latest
```

### 初次使用

1. 启动应用后，首页显示一个居中的输入框
2. 输入你的消息并点击"发送"或按回车
3. 系统会自动创建新会话并进入聊天界面
4. 点击"Settings"按钮配置你的API密钥和模型

## ⚙️ 配置

### 支持的Provider

ChatGo支持以下LLM提供商：

| Provider | 类型 | 说明 |
|----------|------|------|
| **OpenAI** | `openai` | OpenAI官方API (GPT-4, GPT-3.5等) |
| **Claude** | `claude` | Anthropic Claude (Claude 3.5 Sonnet等) |
| **Ollama** | `ollama` | 本地部署的开源模型 |
| **Qwen** | `qwen` | 阿里通义千问 |
| **DeepSeek** | `deepseek` | DeepSeek AI |
| **Gemini** | `gemini` | Google Gemini |
| **Custom** | `custom` | 任何OpenAI兼容的API |

### 配置文件

配置文件自动保存在：
- **Windows**: `C:\Users\<用户>\AppData\Roaming\chatgo\config.yaml`
- **macOS**: `~/Library/Application Support/chatgo/config.yaml`
- **Linux**: `~/.config/chatgo/config.yaml`

### 配置示例

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

### 在界面中配置

1. 点击右下角的"Settings"按钮
2. 选择"Providers"标签
3. 点击"Add New"添加新的Provider
4. 填写配置信息：
   - **Name**: Provider名称（任意）
   - **Type**: 选择Provider类型
   - **API Key**: API密钥（某些Provider不需要）
   - **Base URL**: API端点（可选）
   - **Model**: 模型名称
5. 点击"Save"保存

## 💡 使用技巧

### 快捷键

- **Enter**: 发送消息
- **Shift + Enter**: 在输入框中换行

### 会话管理

- **新建会话**: 点击左上角"New Chat"按钮
- **切换会话**: 在左侧列表点击会话
- **编辑标题**: 点击会话旁的编辑图标
- **删除会话**: 点击会话旁的删除图标

### 切换模型

- 在消息输入框上方的下拉菜单中选择不同的Provider
- 切换后新消息将使用选定的模型

## 🛠️ 技术栈

- **Go 1.21+** - 主要编程语言
- **Fyne** - 跨平台GUI框架
- **Cloudwego Eino** - LLM抽象层和组件库
- **SQLite** - 对话历史存储

## 📦 项目结构

```
ChatGo/
├── cmd/
│   └── chatgo/
│       └── main.go          # 应用入口
├── internal/
│   ├── config/              # 配置管理
│   ├── llm/                 # LLM客户端
│   ├── mcp/                 # MCP服务器支持
│   └── ui/                  # 用户界面
├── pkg/
│   └── models/              # 数据模型
└── README.md
```

## 🤝 贡献

欢迎贡献！请随时提交Issue或Pull Request。

## 📄 许可证

MIT License - 详见 [LICENSE](LICENSE) 文件

## 🙏 致谢

- [Fyne](https://fyne.io/) - 优秀的跨平台Go GUI框架
- [Cloudwego Eino](https://github.com/cloudwego/eino) - 强大的LLM应用开发框架

## 📮 联系方式

- GitHub: [@tk103331](https://github.com/tk103331)
- Issues: [GitHub Issues](https://github.com/tk103331/ChatGo/issues)

---

⭐ 如果这个项目对你有帮助，请给个Star！
