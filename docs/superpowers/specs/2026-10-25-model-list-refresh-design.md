# 模型列表刷新功能设计

日期：2026-10-25
状态：**已实现**。两项待定按建议落地：弹窗默认只勾选已配置的模型、按钮置于「模型」小标题右侧。magpie 已端到端实测（拉取 171 个模型）。

## 目标

在设置 → 模型的 Provider 表单中增加「刷新列表」功能，调用服务商的列模型接口自动拉取可用模型，免去手工逐个输入模型名。

## 调研结论

8 个 Provider 类型全部提供列模型接口。按「响应结构 × 鉴权方式 × 分页」归并为 4 个解析器——注意 anthropic 与 openai 的响应结构相同（都是 `data[].id`），但鉴权头与分页行为不同，故仍拆为两个：

| 解析器 | 端点 | 鉴权 | 分页 | 适用类型 | 验证状态 |
|--------|------|------|------|----------|----------|
| openaiStyle | `{base}/models` | `Authorization: Bearer` | 实际一次返回全部 | openai、magpie（无需 key）、deepseek、qwen、custom(openai/openai_responses) | magpie 实测 HTTP 200 / 171 个模型；路径经 openai-go `model.go:57` 证实 |
| anthropicStyle | `{base}/v1/models` | `X-Api-Key` + `anthropic-version: 2023-06-01` | **有，`has_more` + `after_id`，默认每页 20、上限 1000** | anthropic、claude、custom(claude) | 经 anthropic-sdk-go 证实（`model.go:71`、`requestconfig.go:160`、`option/requestoption.go:274`、`pagination.go:21`） |
| ollamaStyle | `{base}/api/tags` | 无 | 无 | ollama | 经 ollama 官方 client 证实（`api/client.go:295`、`api/types.go:398`，`do()` 不设任何鉴权头） |
| geminiStyle | `{base}/v1beta/models` | **`x-goog-api-key` 请求头**（非 `?key=` query） | **有，`nextPageToken`** | gemini、custom(gemini) | 经 genai 证实（`api_client.go:266`、`client.go:303`、`types.go:3391`） |

验证说明：4 个解析器的端点路径、鉴权方式与分页行为**全部经仓库 module cache 中的官方 SDK 源码交叉验证**（无需任何凭据，见上表证据列）。magpie 另有真实联调（HTTP 200 / 171 个模型）。

剩余风险：源码验证确认的是「SDK 怎么请求」，与「服务商线上契约」在绝大多数情况一致，但不等价——若某服务商变更 API 而 SDK 未跟进，仍会失败。该风险由错误处理兜底（toast 显示服务商原文），不阻塞实施。

ollama 的模型名取 `models[].name`（如 `llama3.2:latest`），`ListModelResponse` 同时有 `name` 与 `model` 两个字段，取 `name`。

模型数量：magpie 实测 171 个；OpenAI 预计约 80 个（含 whisper/tts/embedding 等非对话模型）——因此必须有筛选 UI，不能全量灌入配置。

## 范围

- 新增 `internal/llm/modellist.go`：`ListModels(ctx, p) ([]string, error)` 与 `SupportsListModels(p) bool`，按 `EffectiveAPIType()` 分派到 4 个解析器。anthropic 与 gemini 的解析器须**自行翻页取全**（见下「分页」一节），对调用方仍只暴露一个扁平的 `[]string`。
- 新增 `internal/llm/modellist_test.go`：`httptest` 伪造 4 种响应，含分页场景。
- 修改 `internal/ui/settings_provider.go`：「模型」小标题行右侧加「刷新列表」按钮。
- 新增刷新结果弹窗，复用 `kit.Dialog` + `selectListGrouped`（定义于 `internal/ui/selectlist.go:157`，调用示例见 `import_dialog.go:131`）。该函数已自带搜索框、逐项勾选与「全选/全不选」表头，无需新写 UI。
- 新增 `internal/ui/app.go` 中的刷新状态字段。

## 不在范围

- 不自动定时刷新，仅用户主动触发。
- 不缓存结果到磁盘。
- 不过滤非对话模型（embedding/tts 等）——服务商未提供可靠的能力标记，由用户在弹窗里自行勾选。

## 架构决策

**拉取逻辑放 `internal/llm` 而非 `internal/ui`**：`settings_provider.go` 已 302 行，塞入 HTTP + JSON 解析会破 400 行；且 UI 包测试需起窗口，拉取逻辑几乎无法单测。放在 `llm` 包可用 `httptest` 脱离 UI 测试（仓库已有先例 `group_test.go:25`）。

**不复用 eino SDK**：`eino-ext` 的 `openai.Client` 只暴露 chat 能力，无 list-models；genai 客户端有但仅覆盖 8 个类型中的 1 个。

**解析器按响应结构划分而非按类型划分**：8 个类型收敛到 4 个解析器，新增同协议类型无需加代码。

## 数据流

```
点击刷新 → cw.startModelRefresh()
         → go func(){ llm.ListModels(ctx, p) }        // 不阻塞 UI 线程
         → cw.postMain(){ 填充状态; cw.invalidate() }  // 回主线程
         → 弹窗勾选 → union 合并进 f.models → persistProvider()
```

沿用 `startImportScan`（`import_actions.go:14`）的既有异步模式，不引入新机制。

## 合并语义

union 去重，**不删除任何现有模型**。用户手工添加的、或服务商未列出但实际可用的模型不会丢失。

## 错误处理

| 情况 | 表现 |
|------|------|
| 类型不支持 | 按钮置灰 |
| 缺 API Key（需要的类型） | 点击前拦住并提示，不发请求 |
| 网络不通 / 本机网关未启动 | toast「无法连接 {base}」 |
| 401/403 | toast 显示服务商原文（截断 200 字节） |
| 返回 0 个模型 | 弹窗空状态，不报错 |
| 拉取中切换或关闭表单 | `postMain` 回调校验 `editing` 仍指向同一 provider，否则丢弃结果（防竞态写错条目） |

超时 15 秒，使用 `http.NewRequestWithContext`。

## 分页

anthropic 与 gemini 的列模型接口是分页的，一次 GET 不返回全部：

- **anthropic**：响应含 `has_more` 与 `last_id`，默认每页 20、上限 1000。按 `?limit=1000` 请求，若 `has_more` 为真则带 `after_id={last_id}` 继续取。
- **gemini**：响应含 `nextPageToken`，非空则带 `?pageToken=` 继续取。

两者都设**最多 10 页**的硬上限，防止服务端异常导致无限循环；达到上限即返回已取到的结果，不报错。openai 系与 ollama 实测/按文档一次返回全部，不翻页。

遗漏这一点会让 anthropic 静默只返回 20 个模型，且看不出是被截断的——这是本设计中最容易悄悄做错的地方。

## 测试

`internal/llm/modellist_test.go`：

- 4 种响应结构各能正确解析（含 gemini 的 `models/` 前缀剥离）
- 鉴权正确携带：openai 的 `Authorization: Bearer`、anthropic 的 `X-Api-Key` + `anthropic-version: 2023-06-01`、gemini 的 `x-goog-api-key` 请求头
- **分页取全**：anthropic 的 `has_more`/`after_id` 与 gemini 的 `nextPageToken` 各造 3 页响应，断言返回全部而非首页；并断言第 10 页后停止，不无限循环
- 401 的响应体出现在错误信息中
- 畸形 JSON、空 data 不 panic
- `SupportsListModels` 遍历 `config.ProviderTypes()` 覆盖全部 8 个类型（延续既有不变量测试风格）

## 已确定的两项

1. **弹窗默认勾选策略**：已配置的模型默认勾选并置顶（带「已配置」标签），新发现的默认不勾选。`selectListGrouped` 自带「全选」表头，想一次全要点一下即可。
2. **按钮位置**：「模型」小标题右侧，Ghost 变体 + `refresh-cw` 图标；类型不支持或正在拉取时置灰。

## 实现文件

- `internal/llm/modellist.go` — `ListModels` / `SupportsListModels` 与 4 个解析器
- `internal/llm/modellist_test.go` — 11 个测试（含 4 种结构、4 种鉴权、2 种分页、1 个翻页上限）
- `internal/ui/model_refresh.go` — 异步拉取、弹窗、合并
- `internal/ui/model_refresh_test.go` — 4 个测试（合并保留、防竞态、默认勾选、按钮门控）
- `internal/ui/app.go`、`settings.go`、`settings_provider.go` — 状态字段、弹窗挂载、按钮
