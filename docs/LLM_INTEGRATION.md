# LLM Integration with CloudWeGo Eino

本项目使用 [CloudWeGo Eino](https://github.com/cloudwego/eino) 作为 LLM 接入层，支持多种主流 AI 模型。

## 概述

Eino 是字节跳动开源的 LLM 应用开发框架，提供了：
- 统一的组件抽象
- 强大的编排能力
- 完整的流式处理
- 高度可扩展的回调机制

## 支持的模型提供商

| 提供商 | 类型标识 | 说明 | 官方文档 |
|--------|----------|------|----------|
| **OpenAI** | `openai` | GPT-4, GPT-4o, GPT-4o-mini 等 | [文档](https://platform.openai.com/docs) |
| **Claude** | `claude` | Claude 3.5 Sonnet, Claude 3.5 Haiku 等 | [文档](https://docs.anthropic.com) |
| **Ollama** | `ollama` | 本地部署的开源模型 (Llama, Qwen, DeepSeek 等) | [文档](https://ollama.com) |
| **Qwen** | `qwen` | 通义千问系列 (qwen-max, qwen-plus 等) | [文档](https://help.aliyun.com/zh/model-studio/) |
| **DeepSeek** | `deepseek` | DeepSeek-V3, DeepSeek-Chat 等 | [文档](https://platform.deepseek.com) |
| **Gemini** | `gemini` | Google Gemini 2.0 Flash, Pro 等 | [文档](https://ai.google.dev) |
| **自定义** | `custom` | 兼容 OpenAI API 格式的服务 | - |

## 配置

配置文件位置: `~/.chatgo/config.yaml`

### 完整配置示例

```yaml
current_provider: "openai"
providers:
  # OpenAI
  - name: "OpenAI"
    type: "openai"
    api_key: "sk-xxx"
    base_url: "https://api.openai.com/v1"
    model: "gpt-4"

  # Claude (Anthropic)
  - name: "Claude"
    type: "claude"
    api_key: "sk-ant-xxx"
    model: "claude-3-5-sonnet-20241022"

  # Ollama (本地部署)
  - name: "Ollama"
    type: "ollama"
    base_url: "http://localhost:11434"
    model: "llama3.2"

  # 通义千问
  - name: "Qwen"
    type: "qwen"
    api_key: "sk-xxx"
    base_url: "https://dashscope.aliyuncs.com/compatible-mode/v1"
    model: "qwen-max"

  # DeepSeek
  - name: "DeepSeek"
    type: "deepseek"
    api_key: "sk-xxx"
    model: "deepseek-chat"

  # Google Gemini
  - name: "Gemini"
    type: "gemini"
    api_key: "AIzaSyxxx"
    model: "gemini-2.0-flash-exp"

  # 自定义 API (兼容 OpenAI 格式)
  - name: "Custom"
    type: "custom"
    api_key: "your-key"
    base_url: "https://your-api.com/v1"
    model: "your-model"
```

## 模型详细说明

### 1. OpenAI

**支持的模型：**
- `gpt-4` - 最强大的模型
- `gpt-4o` - 最新的多模态模型
- `gpt-4o-mini` - 轻量级模型
- `o1-preview`, `o1-mini` - 推理模型

**配置示例：**
```yaml
type: "openai"
api_key: "sk-proj-xxx"
base_url: "https://api.openai.com/v1"
model: "gpt-4o"
```

**获取 API Key：** https://platform.openai.com/api-keys

### 2. Claude (Anthropic)

**支持的模型：**
- `claude-3-5-sonnet-20241022` - 最新的 Sonnet 模型
- `claude-3-5-haiku-20241022` - 快速的 Haiku 模型
- `claude-3-opus-20240229` - Opus 模型

**配置示例：**
```yaml
type: "claude"
api_key: "sk-ant-xxx"
model: "claude-3-5-sonnet-20241022"
```

**获取 API Key：** https://console.anthropic.com/

### 3. Ollama

**特点：**
- 完全本地运行，无需 API Key
- 支持多种开源模型
- 适合隐私敏感场景

**支持的模型示例：**
- `llama3.2` - Meta Llama 3.2
- `qwen2.5` - 阿里通义千问 2.5
- `deepseek-coder` - DeepSeek 代码模型
- `codellama` - Code Llama

**配置示例：**
```yaml
type: "ollama"
base_url: "http://localhost:11434"
model: "llama3.2"
```

**安装 Ollama：**
```bash
# macOS
brew install ollama

# Linux
curl -fsSL https://ollama.com/install.sh | sh

# 拉取模型
ollama pull llama3.2
```

### 4. 通义千问 (Qwen)

**支持的模型：**
- `qwen-max` - 最强大的模型
- `qwen-plus` - 性价比模型
- `qwen-turbo` - 快速响应模型

**配置示例：**
```yaml
type: "qwen"
api_key: "sk-xxx"
base_url: "https://dashscope.aliyuncs.com/compatible-mode/v1"
model: "qwen-max"
```

**获取 API Key：** https://dashscope.console.aliyun.com/

### 5. DeepSeek

**支持的模型：**
- `deepseek-chat` - 对话模型
- `deepseek-coder` - 代码模型

**配置示例：**
```yaml
type: "deepseek"
api_key: "sk-xxx"
model: "deepseek-chat"
```

**获取 API Key：** https://platform.deepseek.com/

### 6. Google Gemini

**支持的模型：**
- `gemini-2.0-flash-exp` - 最新的 Flash 实验模型
- `gemini-1.5-pro` - Pro 模型
- `gemini-1.5-flash` - Flash 模型

**配置示例：**
```yaml
type: "gemini"
api_key: "AIzaSyxxx"
model: "gemini-2.0-flash-exp"
```

**获取 API Key：** https://aistudio.google.com/app/apikey

### 7. 自定义 API

适用于任何兼容 OpenAI API 格式的服务，如：
- Azure OpenAI
- LocalAI
- vLLM
- 其他兼容服务

**配置示例：**
```yaml
type: "custom"
api_key: "your-key"
base_url: "https://your-endpoint.com/v1"
model: "your-model"
```

## 实现细节

### 架构

```
chatgo/
├── internal/llm/
│   └── client.go          # 统一的 LLM 客户端接口
└── internal/config/
    └── config.go          # 配置管理
```

### 使用示例

```go
// 创建客户端
client, err := llm.NewClient(provider)
if err != nil {
    log.Fatal(err)
}

// 准备消息
messages := []llm.ChatMessage{
    {Role: "system", Content: "You are a helpful assistant"},
    {Role: "user", Content: "Hello!"},
}

// 发送消息（流式）
response, err := client.Chat(ctx, messages, func(chunk string) {
    fmt.Print(chunk) // 实时输出
})

// 发送消息（非流式）
response, err := client.ChatNonBlocking(ctx, messages)
```

### 消息格式

```go
type ChatMessage struct {
    Role    string // user, assistant, system
    Content string
}
```

### 响应格式

```go
type ChatResponse struct {
    Content string
    Done    bool
}
```

## 技术栈

### Eino 组件

本项目使用以下 Eino 组件：

```go
import (
    "github.com/cloudwego/eino/components/model"
    "github.com/cloudwego/eino/schema"
    "github.com/cloudwego/eino-ext/components/model/claude"
    "github.com/cloudwego/eino-ext/components/model/deepseek"
    "github.com/cloudwego/eino-ext/components/model/gemini"
    "github.com/cloudwego/eino-ext/components/model/ollama"
    "github.com/cloudwego/eino-ext/components/model/qwen"
    "github.com/cloudwego/eino-ext/libs/acl/openai"
)
```

### 核心接口

所有模型实现都统一使用 `model.ChatModel` 接口：

```go
type ChatModel interface {
    Generate(ctx context.Context, in []*schema.Message, opts ...Option) (*schema.Message, error)
    Stream(ctx context.Context, in []*schema.Message, opts ...Option) (*schema.StreamReader[*schema.Message], error)
}
```

## 优势

### 1. 统一接口
通过 Eino 的抽象层，可以轻松切换不同的 LLM Provider，无需修改业务代码。

### 2. 类型安全
Eino 提供编译时类型检查，减少运行时错误。

### 3. 流式处理
所有模型都支持流式响应，Eino 自动处理流数据的拼接、分发和管理。

### 4. 可扩展性
基于 Eino 框架，未来可以轻松添加：
- Tool Calling (工具调用)
- Multi-modal (多模态)
- RAG (检索增强生成)
- Agent 模式

## 性能对比

| 模型 | 响应速度 | 推理能力 | 成本 | 推荐场景 |
|------|----------|----------|------|----------|
| GPT-4o | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | 高 | 复杂任务 |
| Claude 3.5 | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | 高 | 长文本、代码 |
| Gemini 2.0 Flash | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | 低 | 快速响应 |
| Qwen Max | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | 低 | 中文场景 |
| DeepSeek | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | 极低 | 代码任务 |
| Ollama | ⭐⭐⭐ | ⭐⭐⭐ | 免费 | 本地、隐私 |

## 未来扩展

基于 Eino 框架，计划支持：

1. **多模态能力**
   - 图像理解
   - 语音输入输出
   - 视频分析

2. **工具调用**
   ```go
   tools := []*schema.ToolInfo{
       {Name: "web_search", Desc: "Search the web"},
       {Name: "code_exec", Desc: "Execute code"},
   }
   model, _ := client.WithTools(tools)
   ```

3. **Agent 模式**
   - ReAct Agent
   - Multi-Agent 协作

4. **RAG 集成**
   - 向量数据库集成
   - 文档检索
   - 知识库管理

## 故障排除

### 常见问题

**Q: Ollama 连接失败**
```
A: 确保 Ollama 服务正在运行：
   ollama serve
```

**Q: Gemini API 错误**
```
A: Gemini 需要特殊的客户端配置，确保 API Key 格式正确
```

**Q: Qwen 连接超时**
```
A: 国内用户可能需要设置代理或使用兼容的 base_url
```

## 参考资料

- [Eino GitHub](https://github.com/cloudwego/eino)
- [Eino 文档](https://www.cloudwego.io/docs/eino/)
- [Eino Examples](https://github.com/cloudwego/eino-examples)
- [各模型官方文档](#模型详细说明)

## 许可证

本项目使用 Apache 2.0 许可证。
