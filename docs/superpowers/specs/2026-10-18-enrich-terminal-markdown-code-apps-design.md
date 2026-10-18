# 丰富 terminalapp / markdownapp / codeeditorapp 功能设计

日期：2026-10-18
状态：已确认，待实施

## 目标

为 Apps 菜单中的终端、Markdown 编辑器、代码编辑器三个应用同时增强 Agent 工具面与 UI 交互体验，并消除两个编辑器之间重复的文件工具实现。

## 范围

- 共享包 `internal/apps/filetools`：统一文件操作，带路径越权保护。
- `markdownapp`：新增 5 个文件工具（共 8 个）；新增 Markdown 渲染预览、脏标记保存、状态栏、新文档按钮。
- `codeeditorapp`：新增 6 个文件工具（共 9 个，含 open_file）；新增查找/替换条、行:列状态栏、脏标记保存。
- `terminalapp`：run_command 补 workdir/timeout/输出上限/白名单；新增 send_keys、read_screen 工具（共 4 个）；新增清屏、中断、新建标签、重命名、进程状态与当前目录显示。
- 适配层 `internal/ui/apps_adapt.go` 无需结构改动（wrap 通用）。
- 每个 app 新增对应单元测试。

## 约束

- 不改动 `internal/llm`、`internal/mcp`、`internal/ui/apps_adapt.go` 的 wrap 结构；仅随工具数量自然扩展。
- 破坏性工具（delete_file、rename_file、terminal 相关）执行前必须经 `host.Confirm` 确认。
- 保持项目可 `go build` / `go vet` / `go test` 通过。

## 一、共享包 internal/apps/filetools

函数均以 `(root, rel string)` 为路径，内部用 `within` 拒绝越权路径（与现有实现一致的 `filepath.Clean` 前缀检查）。

| 函数 | 签名 | 行为 |
|------|------|------|
| Read | `Read(root, rel) (string, error)` | 读文件，1MB 截断 |
| Write | `Write(root, rel, content) (string, error)` | 覆盖写，自动 MkdirAll |
| Append | `Append(root, rel, content) (string, error)` | 追加写，自动 MkdirAll |
| List | `List(root, rel) (string, error)` | 列目录，目录加 `/`，排序，空目录返回 "(空目录)" |
| Mkdir | `Mkdir(root, rel) (string, error)` | `os.MkdirAll` 0o755 |
| Rename | `Rename(root, old, new) (string, error)` | `os.Rename`，两路径均校验越权 |
| Delete | `Delete(root, rel) (string, error)` | 仅删除文件；目录返回错误，防止误删 |
| Search | `Search(root, rel, pattern string, opts SearchOpts) (string, error)` | 递归 grep；`SearchOpts{Regexp, CaseInsensitive, MaxLines}`；跳过 `.git` 等元目录 |

`Search` 默认 `MaxLines=100`；匹配行输出格式 `相对路径:行号: 内容`；无命中返回 "(无匹配)"。

## 二、markdownapp 增强

工具（3 → 8，全部走 filetools）：
`read_file` `write_file` `list_files`（保留）+ `append_file` + `rename_file` + `delete_file` + `create_directory` + `search_files`。

- `delete_file` / `rename_file` 在执行前经 `cl.host.Confirm` 确认（UI 线程）。
- `write_file` / `append_file` 仍同步已打开标签的内容。
- `rename_file` / `delete_file` 不自动改/关已打开标签（用户自行重新打开）。
- `search_files` 工具参数：`pattern`（必填）、`dir`（可选）、`regexp`（可选，"true"/"false" 字符串）、`case_insensitive`（可选）。布尔参数一律以字符串 "true"/"false" 传入（appTool 参数均为 String 类型）。
- System prompt 更新为列出全部工具。

UI：
- 工具栏在 打开/保存 之外新增「预览」按钮，切换 `mchat.MarkdownView` 渲染当前内容与源码编辑。
- 保存按钮在内容脏（加载后或被 Agent 写文件后未保存）时使用 Primary 高亮，否则 Secondary。
- 状态栏（编辑区下方）：显示 `路径 · 行数 · 字符数 · 未保存` 标记。
- 工具栏新增「新文档」按钮（等价于 NewTab 的空文档）。

## 三、codeeditorapp 增强

工具（3 → 9，全部走 filetools）：
`read_file` `write_file` `list_files`（保留）+ `append_file` + `rename_file` + `delete_file` + `create_directory` + `search_files` + `open_file`。

- `open_file`：用 `cl.Open(rel)` 打开一个标签并返回标签标题，供用户查看。
- 破坏性工具同上经 `host.Confirm` 确认。
- System prompt 更新。

UI：
- 查找/替换条：`查找` `替换` 两个输入框 + `上一个` `下一个` `全部替换` 按钮，作用于当前缓冲区（`strings.Index` 全匹配，忽略大小写可选）。
- 状态栏：`行:列 · 大小 · 未保存`；行:列由 `code.CodeViewerState` 提供，源码编辑模式至少显示行数。
- 保存按钮脏标记高亮（同 markdown）。
- 保留现有语法预览按钮。

## 四、terminalapp 增强

工具（2 → 4）：
- `run_command`：新增可选 `workdir`（相对根目录，经 `within` 校验）、`timeout`（秒，默认 60，上限 300）；输出截断到 64KB 并提示截断；命中白名单首词才执行。
- `send_keys`：把文本写入活动 shell 标签（`term.Send([]byte(text))`）；无标签时报错。
- `read_screen`：返回活动标签 `term.Text()`（屏幕+回滚）；无标签报错。
- `list_terminals` 保留。

白名单：
- System 声称"命令有白名单限制"，补默认安全白名单：`ls pwd echo cat head tail wc grep find git mkdir date uname du df ps curl node python3 go make`。
- 命中白名单的命令可带任意参数；未命中返回错误说明白名单。

UI（终端视图顶部工具栏）：
- 「清屏」：`Send("\x0c")`（Ctrl-L）。
- 「中断」：`Send("\x03")`（Ctrl-C）。
- 「新建标签」：`cl.NewTab()`。
- 活动标签状态文本：运行中显示当前目录（`term.Dir()`，可能为空），已退出显示 `已退出 (code N)`；标题由 shell 提供或回退 "Shell N"。
- 标签重命名沿用现有 appkit.Tabs 机制（标题来自 shell），不新增输入 UI；「运行中/已退出」状态由 OnExit 回调标记。

## 五、测试

- filetools：越权拒绝（`../` 与绝对路径）、读写往返、Append 追加、Rename/Delete、Mkdir、Search 正则/大小写/上限/无命中、目录 Delete 拒绝。
- markdownapp：新增工具可用性（append/rename/delete/mkdir/search 的正面与越权用例）、dirty 状态。
- codeeditorapp：同上 + open_file 打开标签、查找/替换逻辑函数。
- terminalapp：run_command 白名单命中/未命中、workdir、timeout、send_keys/read_screen 无标签报错；既有测试保持通过。

## 六、实施阶段

- 阶段 1：filetools 共享包 + 两个编辑器全部新工具 + 测试。
- 阶段 2：markdown/code UI 增强（预览、查找替换、状态栏、脏标记）。
- 阶段 3：terminal 工具（含白名单）+ UI。

每阶段独立可构建、可测试、可提交。
