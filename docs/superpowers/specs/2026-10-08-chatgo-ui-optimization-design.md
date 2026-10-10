# ChatGo 界面优化设计（MyGo + MujicaUI）

日期：2026-10-08
状态：已确认，实施中

## 目标

在已完成的 Fyne→MyGo/MujicaUI 迁移基础上，用组件库能力全面优化首页、聊天页、设置页，并补充常用会话/消息功能。

## 范围

- 全局：外观（System/Light/Dark）、密度（Compact/Default/Comfortable）、语言（zh-CN/en-US），持久化；Toast 反馈。
- 首页：WelcomeScreen + 能力卡片 + 输入 + 最近会话。
- 聊天页：侧栏（搜索 + 分组会话列表 + 会话项操作）、消息（Header/Actions/可折叠工具调用/空状态）、输入区。
- 设置：Tabs（模型/MCP/内置工具/外观）；PasswordInput、SegmentedControl、StatusIndicator、Toast。
- 功能：复制、重新生成、编辑用户消息、会话搜索、会话导出（Markdown→剪贴板）。
- 结构调整：按职责拆分 ui 包文件；扩展无头渲染测试。

## 约束

- 不改动 `internal/llm`、`internal/mcp` 既有逻辑；`internal/config` 仅新增外观字段。
- 保持项目可 `go build` / `go vet` / `go test` 通过。

## 文件

app.go 状态/视图/流式；welcome.go 首页；sidebar.go 侧栏；message.go 消息；settings.go 设置；tools.go 工具；theme.go 外观配置。
