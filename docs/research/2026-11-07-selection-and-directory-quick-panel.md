# 调研：系统级划词 / 目录 → Agent 浮窗

- 日期：2026-11-07
- 范围：ChatGo（Go + MyGo native UI，macOS 优先）
- 目标：评估「任意 App 内选中文本出现浮窗，选 Agent 执行任务」与「选择目录作为工作目录，弹出浮窗输入并快速执行 Agent 任务」两项能力的可行性、技术选型与实施方案

> 标注约定：**事实**＝已在代码/官方文档中确认；**推断**＝基于平台机制的工程判断；**猜测**＝待验证。
> 置信度：高 ≥80%，中 50-80%，低 <50%。

---

## 0. 结论（TL;DR）

1. **整体可行，且基座已完成大半。** MyGo 已提供做这件事最关键的窗口形态：`WindowOptions.Panel`——浮在所有 App（含全屏）之上、跨 Space、展示时**不激活本 App 却拿到键盘**，还能用 `SetPosition` 放到屏幕任意位置（**事实，高**，见 `mygo@v0.6.3/window.go` Panel 注释与 `docs/windows.md#panels`）。ChatGo 也已有全局快捷键、托盘、通知、会话/Agent/Workspace/Worktree、无头任务执行（auto task）与主题组件。
2. **真正的缺口只有两个系统能力**：系统级**取词**、系统级**目录捕获**。其中「**自动**读取任意 App 的选区」在 macOS 上绕不开**辅助功能（Accessibility / TCC）权限**（目录与文本都还有零权限的替代路径：剪贴板、拖拽、选择器、CLI）；且 ChatGo 目前没有任何 Objective-C 桥，需要用 `purego` 新写一层（MyGo 内部已有同款实现可参考）。
3. **推荐路线：快捷键优先，划词自动浮窗可选。**
   - 一期：把现有 `CmdOrCtrl+Shift+Space` 从「缩窄主窗」升级为**统一 Quick Panel**。能读到选中文本就预填引用，能读到目录就作为工作目录。
   - 二期：再上「划词自动弹浮窗」（默认关闭；授权后开启，带最小长度、白/黑名单、防打扰策略）。
   - 目录侧一期先用「快捷键 + 剪贴板 file-url + 拖拽 + 目录选择器」，Finder 右键 Services/Quick Action 作为增强。
   - 平台：一期 macOS；Windows 二期（UIA/剪贴板模拟 + 资源管理器右键菜单）；Linux X11 三期（PRIMARY selection），Wayland 降级。
4. **估算**（1 名熟悉 Go + macOS 的工程师）：
   - macOS 技术验证 spike：2-3 人日
   - macOS 可用 MVP：8-12 人日
   - 体验完善（权限引导、兼容性、防打扰、文档）：再 2-3 周
   - Windows：3-5 人日；Linux X11：3-4 人日；Wayland：2-3 人日或明确不支持
5. **最大风险**（详见第 5 节）：辅助功能权限的用户教育与签名稳定性；Electron/浏览器/终端的 AX 覆盖率不一致；自动浮窗误触发与打扰；剪贴板兜底会污染用户剪贴板；后台任务与现有「全局唯一 LLM client」状态耦合。
6. **需要拍板的 5 件事**见第 8 节（自动浮窗默认值、是否直接执行预设动作、前台/后台执行、是否默认 worktree、是否只做 macOS）。

---

## 1. 需求拆解

### 1.1 场景 A：划词 → 浮窗 → 选 Agent → 执行

| 环节 | 期望行为 |
|---|---|
| 触发 | 鼠标划选 / 双击选词 / 三击选段 / Shift+方向键（可选） |
| 取词 | 拿到纯文本、选区屏幕坐标、来源 App（名称/Bundle ID/PID） |
| 浮窗 | 贴近选区出现，不打断当前 App 的输入与激活；可用键盘选择 |
| 选 Agent | 收藏 Agent 横排（内置 Chat / 自定义 Agent / ACP Agent），支持数字键 1-9 |
| 任务 | 预设动作（解释/翻译/总结/改写）+「自定义…」进入输入框 |
| 执行 | 生成任务并运行，浮窗内可看流式输出 |
| 结果 | 完成通知；可「在主窗口打开」继续对话/看 diff |

### 1.2 场景 B：目录 → 工作目录 → 浮窗输入 → 执行

| 环节 | 期望行为 |
|---|---|
| 触发 | Finder 选中目录 + 快捷键 / 右键 / 拖拽 / CLI / URL scheme |
| 目录 | 作为本轮 Agent 的工作目录（workspace），可选「新建 worktree 隔离」 |
| 浮窗 | 与场景 A 复用同一个 Quick Panel，只是 workspace 已确定 |
| 执行 | 新建会话并发送首条消息；默认后台执行，不打断主窗口当前会话 |
| 结果 | 会话出现在该目录分组下，可随时接管 |

### 1.3 非目标（本期明确不做）

- Finder 工具栏按钮 / Finder Sync Extension / App Intents 扩展（需要 Swift App Extension，与纯 Go 构建链冲突，工作量极大）。
- macOS App Store 沙箱分发（AX 控制其他 App 在沙箱下基本不可行）。
- 读取密码框、安全输入（Secure Event Input）中的内容。
- 一期同时覆盖 Windows/Linux 完整体验。

---

## 2. 现状盘点：ChatGo / MyGo 已有资产

### 2.1 MyGo 侧（可直接用）

| 能力 | API / 位置 | 用途 | 证据 |
|---|---|---|---|
| 非激活浮窗 | `WindowOptions.Panel` | 划词条 + 输入面板 | **事实，高**（`window.go` Panel 注释；`docs/windows.md#panels`） |
| 不抢焦点展示 | `Window.ShowInactive()` | 划词条出现时不打断打字 | **事实，高**（`window.go:685`；darwin 实现 `orderFrontRegardless`） |
| 拿键盘不激活 App | `Window.Show()`（Panel） | 输入面板 | **事实，高**（darwin `makeKeyAndOrderFront` 且 `!Panel` 才 activate） |
| 任意定位 | `SetPosition(x,y)`、`Screen.DisplayNearestPoint` | 贴近选区 / 屏幕居中 | **事实，高** |
| 全局快捷键 | `mygo.GlobalShortcut.Register` | 统一入口 | **事实，高** |
| 原生目录选择 | `mygo.Dialog.Open{Directory:true}` | 目录兜底 | **事实，高**（`dialog.go`） |
| 文件拖拽 | `ui.Element.DroppedFiles()` | 目录拖入浮窗 | **事实，高**（`ui/drop.go`） |
| 托盘 / 通知 | `mygo.NewTray`、Notifications | 后台常驻与完成提醒 | **事实，高** |
| 失焦自动隐藏 | `Window.OnBlur` | 输入面板关闭 | **事实，高**（`window.go:1499`） |
| 单实例 / URL scheme / 文件关联 | `RequestSingleInstanceLock`、`RegisterURLScheme`、`OnOpenFile` | CLI / 外部工具唤起 | **事实，高**（`single_instance.go`、`deeplink.go`），但 ChatGo 尚未启用 |

> 注意：`OnOpenFile` 只接收**常规文件**（`deeplink.go:fileArgs` 里 `info.Mode().IsRegular()`），**目录不会被投递**。所以「拖目录到 Dock 图标」这条不成立，除非改 MyGo 或自己解析 `OnSecondInstance` 的 args。

### 2.2 ChatGo 侧（可复用）

| 能力 | 位置 | 复用方式 |
|---|---|---|
| 全局快捷键注册/改键 | `internal/ui/app.go:386` `applyQuickShortcut`，默认值 `app.go:271` | 换入口为 Quick Panel，或新增第二个快捷键 |
| 现有「紧凑窗口」模式 | `app.go:485` `toggleQuickMode`、`app.go:500` `openQuick`、`app.go:522` `shrinkForQuick` | 仅作交互参考；它不是浮窗，是缩窄主窗 |
| 新建会话 | `conversation.go:61` `newConversationIn(ws, agent)` | 直接复用 |
| 发送首条消息 | `conversation.go:126` `sendDraft()` | 直接复用（前台模式） |
| 后台任务执行 | `autotask.go:588` `startAutoTask`、`autotask.go:707` `taskClient` | 后台模式的模板（**不含 ACP**） |
| Agent 规格解析 | `autotask.go:794` `taskAgentSpec` | 取 provider/model/system prompt/workspace/tools |
| Agent 选择器 | `create.go:130` `createAgentSelect` | 浮窗 chips 的数据源与头像 |
| 工作区选择/切换 | `create.go:182`、`worktree_ops.go:201` `pointAtWorkspace` | 目录模式 |
| 目录选择器 | `sidebar.go:73` `pickDirectory` | 兜底选目录 |
| 完成通知 | `notify.go:63` `notifySessionComplete` | 后台任务完成提醒 |
| 设置页 | `settings.go:129` Tabs（含「实验」） | 新增「划词浮窗」设置区 |
| 配置结构 | `config.go:14` `Config` | 新增 `quick_panel` 段 |
| 单实例 | 未启用（`main.go` 仅 `WhenReady`） | 目录/CLI 流的前置改造 |

### 2.3 结论

- UI、Agent、Workspace、通知、设置：**70% 可复用**。
- 缺口集中在三处：
  1. **系统级取词/取目录**（Darwin 原生能力，需自研或用第三方绑定）；
  2. **独立的 Quick Panel 状态机**（现有 quickMode 会切主窗，不能复用）；
  3. **可脱离主窗口运行的任务 runner**（现有 `runStream` 依赖全局 `cw.llmClient` 单槽位，见 3.6）。

---

## 3. 关键技术选型

### 3.1 系统级取词（macOS）

| 方案 | 原理 | 权限 | 延迟 | 覆盖率 | 对用户打扰 | 结论 |
|---|---|---|---|---|---|---|
| **AX 直读** | `AXUIElementCopyAttributeValue(AXSelectedText)` | 辅助功能 | 1-10ms（个别 App 会卡） | 原生 App 好；Chromium/Electron 需额外开关；终端/PDF 部分 | 无 | **主方案** |
| 剪贴板模拟 | 合成 `Cmd+C` → 读 `NSPasteboard` → 还原 | 辅助功能（CGEvent 投递） | 50-150ms | 几乎所有能复制的 App | 覆盖用户剪贴板、有竞态 | **仅显式操作的兜底** |
| Services 菜单 | Info.plist `NSServices` + 运行时注册 provider | 无 | 低 | 所有支持 Services 的 App | 需手动右键 | **增强项**（无自动浮窗） |
| 浏览器扩展 / IDE 插件 | 页面内 JS 读选区 → 本地 HTTP → ChatGo | 无 | 低 | 仅对应宿主 | 无 | **可选补充**（ChatGo 已有本地 Web 服务） |
| 拖拽文本 | 用户把选中文本拖到浮窗 | 无 | 低 | 视 App 支持 | 需手动 | 兜底 |
| 快捷键 + 手动 `Cmd+V` | 面板聚焦后用户自己粘贴 | 无 | 低 | 100% | 多一步 | **无权限模式**的兜底 |

#### AX 直读的调用链（事实，高）

1. `NSWorkspace.sharedWorkspace.frontmostApplication` → `processIdentifier` / `bundleIdentifier` / `localizedName`。
2. `AXUIElementCreateApplication(pid)`。
3. 读 `AXFocusedUIElement`（kAXFocusedUIElementAttribute）拿到焦点元素。
4. 读 `AXSelectedText`（kAXSelectedTextAttribute）。官方文档：**可编辑文本元素必须实现该属性**。
5. 读 `AXSelectedTextRange`（AXValue / CFRange），再用
   `AXUIElementCopyParameterizedAttributeValue(el, "AXBoundsForRange", range)` 拿选区矩形（官方文档：屏幕坐标系下的包围盒）。
6. 过滤：`AXSubrole == "AXSecureTextField"`、空串/纯空白、长度超限、来源是本 App、`IsSecureEventInputEnabled()` 为真时全部丢弃。
7. 超时保护：`AXUIElementSetMessagingTimeout(app, 0.25)`，避免目标 App 卡死拖住浮窗。

#### AX 的现实坑（推断，中-高）

- **Electron/Chromium**（VS Code、Slack、Notion、飞书部分版本、Chrome）在无读屏器时常不构建完整 AX 树。常见做法是给应用元素设置 `AXManualAccessibility = true` 触发其开启无障碍（Chromium 生态的已知行为，属半文档能力，**需实测**）。
- **终端类**（Terminal.app / iTerm2）选区可能通过 `AXSelectedText` 或 `AXSelectedTextRange` 暴露，但不同版本差异大，需要逐项验证。
- **PDF / 预览 / 截图工具**：选区可能拿不到文本；此时只能靠剪贴板兜底。
- **Office / 微信 / 飞书**：需实测，可能有 AX 树但不提供 `AXSelectedText`。
- AX 调用是跨进程 IPC，目标 App 卡住时调用会阻塞；必须放到主线程之外并设超时，绝不能在渲染帧里同步读。

> 结论：AX 能做到「大多数场景好用」，但**不能承诺 100%**；产品上必须同时保留「快捷键 + 剪贴板」兜底路径，并在 UI 上说清楚。

### 3.2 触发方式

| 方案 | 权限 | 能否覆盖键盘选区 | 误触发 | 复杂度 | 结论 |
|---|---|---|---|---|---|
| **全局快捷键**（现有 `GlobalShortcut`） | 无 | 能 | 无 | 低 | **保底入口，必做** |
| `NSEvent` 全局鼠标监听（mouseUp） | 鼠标事件**不需要**辅助功能；仅按键事件需要 | 否 | 中（需去抖/过滤） | 中 | **自动浮窗的触发器** |
| `CGEventTap` | 通常需要辅助功能/输入监控 | 否 | 中 | 中高 | 不优先：权限更重，还要处理超时禁用 |
| `AXObserver` 通知（选中文本变化/焦点变化） | 辅助功能 | 能 | 低 | 高（逐 App 兼容） | 二期可选增强 |
| Services / 右键菜单 | 无 | 能 | 无 | 中 | 增强入口 |

官方文档明确：全局鼠标监听只对**按键相关事件**要求辅助功能授权（`addGlobalMonitorForEventsMatchingMask:handler:`）。因此「监听 mouseUp」本身不会额外增加权限门槛，**真正的权限门槛来自读取选区（AX）**。

自动浮窗的去抖与过滤规则（建议默认值）：

- mouseUp 后延迟 80-120ms 再读 AX（等 App 处理完选区）；
- 选区文本 trim 后长度 ≥ 2，且与上一次触发内容/位置相同则跳过；
- 鼠标按下到抬起位移很小、且位于本 App 窗口内则跳过；
- 安全输入、密码框、ChatGo 自身、黑名单 Bundle ID 跳过；
- 同一次选区只弹一次；用户按 Esc/点击别处后短时间内不重复弹。

### 3.3 浮窗载体

| 方案 | 定位能力 | 不激活 App | 键盘 | 结论 |
|---|---|---|---|---|
| **MyGo `WindowOptions.Panel`** | 任意 `SetPosition` | 是（`Show` 拿键盘但不 activate） | 是 | **推荐** |
| `NewFlyout` | 只能相对父窗口或托盘图标 | 是 | 是 | 不适用：选区位置任意 |
| 现有 quickMode（缩窄主窗） | 跟随主窗 | 否 | 是 | 不适用：会抢焦点、打断工作 |
| WebView 页面（`URL:`） | 任意 | 是 | 是 | 可行但多一层 IPC/资源，且样式与原生 UI 不一致 |

推荐用**一个 Panel、两种形态**：

- **划词条**（约 320×44）：`ShowInactive()` 出现，不抢键盘；只做「选 Agent / 更多」。
- **输入面板**（约 460×220）：用户点选后 `Show()`，拿键盘但不激活 ChatGo；输入、执行、看输出。

一个窗口两种尺寸/位置，避免频繁创建销毁窗口（macOS 建窗有成本，且首次显示可能闪一下）。也可以做两个 Panel，按需切换，实现更简单。**建议先做两个 Panel**（bar/compose），逻辑更清晰，后续再合并。

### 3.4 坐标与定位

- AX 返回的是**全局屏幕坐标**（原点在主屏左上角），与 MyGo `Rectangle`（DIP、原点主屏左上）方向一致（**事实，高**）。
- Retina 下 AX 的数值单位需实测确认是点还是像素（官方措辞是 "pixels"）；若发现是物理像素，统一除以 `backingScaleFactor`（**待验证，中**）。
- 多显示器/负坐标：用 `mygo.Screen.DisplayMatching(rect)` 找到所属屏，再做 workArea 内 clamp，避免浮窗跑到屏幕外。
- 默认位置：选区矩形下方 8pt、左对齐；下方空间不足则翻到上方；再不行贴屏幕边缘。
- 取不到选区矩形时：退化为鼠标当前位置；再退化为当前显示器居中偏上 1/3（与 Spotlight 一致）。

### 3.5 目录获取（macOS）

| 方案 | 权限 | 用户步骤 | 可靠性 | 工作量 | 结论 |
|---|---|---|---|---|---|
| **剪贴板 `public.file-url`** | 无 | Finder 里 `Cmd+C` → 快捷键 | 高（拿到完整路径） | 0.5d | **一期必做** |
| **拖拽目录到浮窗/主窗** | 无 | 拖 | 高 | 0.5d（复用 `DroppedFiles`） | **一期必做** |
| **`Dialog.Open{Directory:true}`** | 无 | 点选 | 高 | 0.2d（已有） | **一期必做** |
| **CLI / URL scheme** | 无 | 终端 `chatgo .` / `open chatgo://…` | 高 | 1-2d（需单实例） | **一期建议** |
| Finder AX 选区（`AXSelectedChildren` → `AXURL`） | 辅助功能 | 选中目录 → 快捷键 | 中（Finder 版本相关） | 1-2d | 二期增强 |
| Services / Quick Action | 无 / 无 | 右键 → 服务 | 高 | 0.5-1d（Quick Action）/ 2-3d（Services provider） | 二期 |
| Finder Sync Extension | — | 工具栏/右键 | 高 | 5d+（Swift 扩展） | **不做** |

Finder 选区读取的可行路径（推断，中）：Finder 的焦点元素层级大致是
`AXApplication → AXWindow → AXScrollArea → AXOutline/AXList → AXRow → AXCell`，
选中项通过容器上的 `AXSelectedChildren`/`AXSelectedRows` 暴露，行元素通常有 `AXURL`（`file://`）。
实现上建议：从 Finder 的 `AXFocusedWindow` 做有界 BFS（深度 ≤ 8），找到第一个有非空 `AXSelectedChildren` 的容器；取不到就退化为剪贴板路径或让用户用目录选择器。

> 更省事的替代：`osascript -e 'tell application "Finder" to get POSIX path of (target of front window as alias)'`，但会额外触发「自动化」权限，且慢 50-150ms，只建议作为调试兜底。

### 3.6 执行模型

| 方案 | 是否切主窗 | 支持 ACP | 并发安全 | 复用度 | 结论 |
|---|---|---|---|---|---|
| A. 前台会话：`newConversationIn` + `sendDraft` | 是（改 `cw.cur`/workspace/tab） | 是 | 是 | 最高 | 兜底/「在主窗口继续」 |
| B. 后台 LLM 任务：`taskClient` + 自建 runner | 否 | 否 | 是（独立 client） | 高（auto task 已证明） | **默认** |
| C. 后台 ACP 任务：`acpEnsureSession` + `acpChatStart`（不切 `cw.cur`） | 否 | 是 | 需审计 | 中 | 二期补齐 |
| D. 现有 `quickMode` | 是 | 是 | 是 | — | 不适用 |

**关键约束（事实，高）**：`conversation.go:260 runStream` 直接使用 `cw.llmClient / cw.reactClient / cw.groupClient / cw.groupReact` 这些「当前会话」的单槽位状态。也就是说，**不能一边在主窗聊天、一边用 `runStream` 给另一个会话流式输出**，会串台。

因此后台快速任务必须：

- LLM/Provider Group：走 `autotask.go:707 taskClient(agent, workspace, conv)` 构造**任务专属 client**，在 goroutine 里跑，完成/流式回调 `postMain` 写回任务会话（与 `startAutoTask` 同构）。
- ACP：`acp` 状态本身以会话 ID 为键（`acp.convObj[conv.ID]`、`acpEnsureSession(conv, conn)`），理论上可在不切换 `cw.cur` 的情况下跑；但需要审计 `acp_session.go` 里所有对 `cw.cur`/当前 tab 的假设（**推断，中**，预计 1-2 人日）。
- 并发策略：同一时间允许 N 个后台任务（建议 3），超出排队；每个任务在浮窗内有独立 cancel。

推荐：**v1 默认后台（LLM 走 B，ACP 走 A 并提示「已切到主窗口」），v2 统一到 C**。

---

## 4. 推荐方案

### 4.1 总体架构

```
                 ┌─────────────────────────────┐
   mouseUp ─────▶│  capture (darwin)           │
   快捷键  ─────▶│  · AX 取词 + 选区矩形        │──▶ Selection{Text, Rect, App, PID}
   拖拽    ─────▶│  · 剪贴板 file-url / 目录    │──▶ Directory{Path}
   CLI/URL ─────▶│  · 权限探测/引导             │
                 └──────────────┬──────────────┘
                                │ 主线程投递 (mygo.RunOnMain)
                 ┌──────────────▼──────────────┐
                 │  QuickPanel (MyGo Panel)     │
                 │  bar 模式 ──▶ compose 模式   │
                 │  状态机：idle/bar/compose/   │
                 │          running/done        │
                 └──────────────┬──────────────┘
                                │ 提交 QuickTask
                 ┌──────────────▼──────────────┐
                 │  QuickRunner                 │
                 │  · LLM: taskClient（后台）   │
                 │  · ACP: detached session     │
                 │  · 写回 Conversation + 通知  │
                 └──────────────┬──────────────┘
                                │
                    主窗口 / 会话列表 / 通知
```

### 4.2 新增模块与文件（建议）

| 文件 | 职责 | 平台 |
|---|---|---|
| `internal/capture/capture.go` | 对外接口、数据结构、权限状态、过滤规则（纯逻辑，可单测） | all |
| `internal/capture/capture_darwin.go` | AX 取词、鼠标监听、剪贴板 file-url、Finder 选区 | darwin |
| `internal/capture/objc_darwin.go` | purego 的 objc/CF 最小桥（`send`/`nsString`/`CFRelease`/block） | darwin |
| `internal/capture/capture_windows.go` | UIA / 剪贴板模拟占位 | windows |
| `internal/capture/capture_linux.go` | X11 PRIMARY（`xclip`/Xlib）占位 | linux |
| `internal/capture/capture_other.go` | 空实现（不支持） | 其他 |
| `internal/ui/quickpanel.go` | Panel 创建、两种形态、定位、显示/隐藏 | all |
| `internal/ui/quickpanel_view.go` | 浮窗 UI（chips、输入框、workspace、输出） | all |
| `internal/ui/quicktask.go` | QuickRunner、任务模型、后台执行与取消 | all |
| `internal/ui/settings_quick.go` | 设置页：开关、快捷键、自动浮窗、黑名单、权限状态 | all |
| `internal/config/config.go` | 新增 `QuickPanel` 配置段 | all |

### 4.3 对外接口（草案）

```go
// internal/capture
type Selection struct {
    Text     string
    Rect     Rect   // 全局屏幕坐标（DIP）
    AppName  string
    BundleID string
    PID      int32
    Source   Source // ax | clipboard | manual
}

type Rect struct{ X, Y, W, H int }

type Capture interface {
    // Available 报告取词是否可用（权限 + 平台）。
    Available() bool
    // Selection 读取当前前台 App 的选区；无选区返回 ErrNoSelection。
    Selection(ctx context.Context) (Selection, error)
    // Watch 在用户完成划选时回调；返回停止函数。仅平台支持时有效。
    Watch(fn func(Selection)) (stop func())
    // PickDirectory 从剪贴板/前台 Finder 推断目录；失败返回 ErrNoDirectory。
    PickDirectory(ctx context.Context) (string, error)
    // Permission 返回权限状态与引导动作。
    Permission() Permission
}

type Permission struct {
    State    PermissionState // granted | denied | unsupported
    CanPrompt bool
    SettingsURL string // x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility
}
```

```go
// internal/ui
type QuickTask struct {
    Agent     string // "" | "agent:x" | "acp:x"
    Workspace string
    Prompt    string
    Quote     string // 选中的原文，作为引用块附在 Prompt 前
    Worktree  bool
    Mode      TaskMode // background | foreground
}

type taskState struct {
    ID       string
    Task     QuickTask
    ConvID   string
    Status   taskStatus // queued | running | done | failed | canceled
    Output   strings.Builder
    Cancel   context.CancelFunc
}
```

### 4.4 两条主流程

**A. 划词**

1. `capture.Watch` 收到 mouseUp → 去抖 → AX 读取（失败则静默，不弹）。
2. `RunOnMain` 定位并 `ShowInactive` 划词条（不抢键盘）。
3. 用户点 Agent chip → 保存 `Selection`，切换为 compose：`Show()` + `Focus()` 输入框，预填 `> 选区原文` + 光标。
4. Enter → 生成 `QuickTask` → `QuickRunner.Submit` → 浮窗进入 running，显示流式输出。
5. 完成：通知 + 「在主窗口打开」；`OnBlur`/Esc 隐藏并清空。

**B. 目录**

1. 快捷键（或 CLI/URL/拖拽）→ 先尝试 `PickDirectory`（剪贴板 file-url → Finder AX → 空）。
2. 有目录：compose 直接以该目录为 workspace，输入框聚焦，顶部显示目录 chip（可更换、可「新建 worktree」）。
3. 无目录：顶部 workspace 行显示「选择目录」按钮，走 `Dialog.Open{Directory:true}`。
4. Enter → `QuickTask{Workspace: dir}` → 后台执行；会话写入该目录分组。

### 4.5 浮窗 UI 草案

```
┌──────────────────────────────────────────────┐
│  ChatGo · 来自 Safari            [×]         │   ← 划词来源 / 目录路径
│  ┌────┐ ┌────┐ ┌────┐ ┌────┐                │
│  │Chat│ │Rev │ │Code│ │ +  │                │   ← Agent chips（1-9）
│  └────┘ └────┘ └────┘ └────┘                │
│  ┌────────────────────────────────────────┐  │
│  │ > 选中的原文（引用块，最多 6 行）        │  │
│  │ 解释这段内容…                            │  │
│  └────────────────────────────────────────┘  │
│  📁 ~/code/ChatGo   ▾     [ ] 新建 worktree  │
│  ⏎ 运行   ⌘⏎ 在主窗口运行        DeepSeek ▾  │
└──────────────────────────────────────────────┘
```

- 默认动作只做「把选区作为引用 + 用户输入作为指令」，不内置太多预设，避免猜错。
- 预设动作（解释/翻译/总结/改写）做成输入框上方的可选 chips，点一下填入 prompt 模板。
- running 态把输入区替换为可滚动的输出区，底部保留「停止 / 在主窗口打开」。

### 4.6 配置项（新增）

```yaml
quick_panel:
  enabled: true
  shortcut: "CmdOrCtrl+Shift+Space"
  auto_popup: false          # 划词自动弹窗，需辅助功能权限
  min_chars: 2
  exclude_apps: []           # Bundle ID 黑名单
  favorites: []              # Agent 收藏，空则全部展示（最多 6 个）
  default_agent: ""
  background: true           # 默认后台执行
  default_worktree: false
  directory_shortcut: "CmdOrCtrl+Shift+D"
  clipboard_fallback: true   # 快捷键模式下允许读剪贴板
```

### 4.7 权限与隐私

- **权限引导**：设置页显示「辅助功能：已授权/未授权」，按钮直接打开
  `x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility`；授权后提示重启生效（TCC 对运行中进程的授权通常需要重启 App）。
- **降级路径**：无权限时自动关闭 auto_popup，只保留快捷键 + 剪贴板 + 粘贴，UI 明确说明差异。
- **隐私**：
  - 只有用户点选 Agent 并提交后才把文本写入会话与发送给模型；auto_popup 阶段只在内存中持有，不上传、不落盘。
  - 密码框、`IsSecureEventInputEnabled()`、黑名单 App 一律不读。
  - 提供「快速任务不写入历史」开关（复用会话存储时需额外标记）。
  - 日志中不打印选区原文（只打印长度/来源）。
- **签名/公证**：AX 权限绑定 bundle ID + 签名；开发版（ad-hoc 签名）容易在重新构建后出现「已授权但无效」或重复条目，需在文档中说明，并确保正式包使用固定 Developer ID。

### 4.8 平台矩阵

| 能力 | macOS | Windows | Linux X11 | Linux Wayland |
|---|---|---|---|---|
| 全局快捷键 | ✅ MyGo | ✅ | ✅ | ✅（portal，需安装） |
| 浮窗 Panel | ✅ 非激活 | ✅ topmost tool window | ✅ utility/layer | ✅ layer-shell |
| 取词 | AX（需授权） | UIA / 剪贴板模拟 | PRIMARY selection | 受限（多为剪贴板或手动粘贴） |
| 取目录 | 剪贴板/AX/CLI | 资源管理器右键 + CLI | CLI / 文件管理器 | 受限 |
| 一期 | **是** | 二期 | 三期 | 降级 |

---

## 5. 风险与对策

| # | 风险 | 概率 | 影响 | 对策 |
|---|---|---|---|---|
| 1 | 用户不理解/不愿给辅助功能权限 | 高 | 高 | 默认走快捷键+剪贴板；权限只用于增强；设置页给出「为什么需要」与一键跳转；权限状态可见 |
| 2 | AX 覆盖率不一致（Electron/终端/PDF） | 高 | 中 | 兼容性矩阵 + 剪贴板兜底 + 逐 App 白名单开关；对已知不支持的 App 不弹窗 |
| 3 | 误触发/打扰（自动浮窗） | 高 | 高 | 默认关闭；最小长度；去抖；黑名单；同一选区只弹一次；提供「最近 7 天不再自动弹」 |
| 4 | 剪贴板兜底覆盖用户内容 | 中 | 中 | 仅在显式快捷键模式使用；先保存再还原；`changeCount` 校验，发现用户已改动则不还原 |
| 5 | 目标 App AX 卡死拖慢浮窗 | 中 | 中 | `AXUIElementSetMessagingTimeout(0.25)`；读取放后台 goroutine；超时静默失败 |
| 6 | 浮窗抢焦点/打断输入 | 中 | 高 | 划词条用 `ShowInactive`；只在用户点选后才 `Show()`；提交后归还焦点给来源 App |
| 7 | 后台任务与前台会话串台 | 高（若不改造） | 高 | 用独立 `taskClient`；禁止复用 `runStream` 的全局 client；同会话并发加锁 |
| 8 | 多屏/负坐标定位错位 | 中 | 中 | `DisplayMatching` + workArea clamp；Retina 单位实测；无 bounds 时退化鼠标点 |
| 9 | TCC 授权在重构建后失效/重复条目 | 中 | 中 | 固定 bundle ID 与 Developer ID 签名；文档说明 `tccutil reset Accessibility <bundleid>` |
| 10 | 动态加载框架/第三方 dylib 被公证拦截 | 中 | 高 | 不引入运行时释放的 dylib；只 dlopen 系统框架；或把能力上游到 MyGo |
| 11 | 安全输入/密码泄露 | 低 | 极高 | `IsSecureEventInputEnabled()` + `AXSecureTextField` 双重过滤；黑名单 |
| 12 | Wayland 无法读取全局选区 | 高 | 中 | 明确降级为「快捷键 + 粘贴」；文档写明 |
| 13 | Windows UIA 的 COM 成本 | 中 | 中 | 二期先用「Ctrl+C + 剪贴板」；UIA 作为后续优化 |
| 14 | MyGo 能力缺口（托盘拖拽、目录文件关联） | 中 | 低 | 不做或上游 PR；目录走 args 自己解析 |
| 15 | 系统升级导致 AX 行为变化 | 中 | 中 | 把捕获层隔离在 `internal/capture`，便于替换；兼容性回归清单 |

---

## 6. 实施计划

> 假设：1 名熟悉 Go + macOS 的工程师；可在真机调试；不包含 Windows/Linux。

### M0 技术验证 spike（2-3 人日）— 决定「做不做/怎么做」

- [ ] 新建 `internal/capture` 最小实现：`purego` dlopen `ApplicationServices`，读前台 App 的 `AXSelectedText` + `AXBoundsForRange`。
- [ ] `NSEvent` mouseUp 全局监听 + 80ms 去抖（先用一个临时 Panel 显示坐标与文本长度）。
- [ ] 在 6 个 App 上记录兼容性：Safari、Chrome、VS Code、Slack、Terminal、飞书。
- [ ] 验证 Retina 坐标单位、多屏负坐标、全屏 App 下的浮窗层级。
- **产出**：兼容性矩阵 + 结论（AX 覆盖率是否 ≥80%）。
- **Go/No-Go**：若核心 3 个场景（浏览器/编辑器/IM）都拿不到文本，则改为「快捷键 + 剪贴板」方案，砍掉 auto_popup。

### M1 Quick Panel 基座（3-5 人日）

- [ ] `quickpanel.go`：Panel 创建（隐藏）、bar/compose 两态、`ShowInactive`/`Show`/`OnBlur`/Esc。
- [ ] `quickpanel_view.go`：Agent chips（复用 `agentLeading`/`logoLeading`）、输入框、workspace 行、运行按钮。
- [ ] 设置页「划词浮窗」区：开关、快捷键录制（复用现有快捷键控件）、默认 Agent、后台开关。
- [ ] 单实例 + `OnSecondInstance(args, wd)`：解析目录参数，打开 compose。
- [ ] 快捷键改为打开 Quick Panel；保留「紧凑主窗」为浮窗内动作。
- **验收**：`Cmd+Shift+Space` 在任意 App 下弹出面板，可输入并创建后台任务。

### M2 划词浮窗（3-5 人日）

- [ ] AX 取词 + 过滤 + 超时；Electron `AXManualAccessibility` 尝试。
- [ ] 划词条定位与显示；点击 Agent → compose 预填引用。
- [ ] 预设动作 chips（解释/翻译/总结/改写）。
- [ ] 权限检测、引导、重启提示；无权限自动降级。
- [ ] 黑名单与去抖参数落配置。
- **验收**：Safari/Chrome/VS Code 中划词 → 浮窗 → 选 Agent → 得到结果；无权限时明确提示且不崩。

### M3 目录流（2-3 人日）

- [ ] `PickDirectory`：剪贴板 `public.file-url` → Finder AX（可选）→ 目录选择器。
- [ ] 拖拽目录到浮窗（`DroppedFiles`）。
- [ ] CLI/URL scheme：`chatgo --dir <path>`、`chatgo://quick?dir=…&q=…`。
- [ ] 「新建 worktree」选项，复用 `addWorktreeIn` + `finishCreate` 的思路。
- [ ] 后台任务写入目录分组；不调用 `pointAtWorkspace` 以免扰乱主窗。
- **验收**：Finder 选中目录 → 快捷键 → 输入 → 在后台跑起来，会话出现在对应目录下。

### M4 打磨与发布（3-4 人日）

- [ ] 后台 ACP 任务（若 M2 用的是「切主窗」兜底，则在此统一）。
- [ ] 完成通知、取消、失败重试、并发上限。
- [ ] 兼容性回归 + 性能测量 + 文档/隐私说明。
- [ ] 打包验证（公证、TCC 授权、重启后状态）。
- **验收**：连续 50 次划词无残留窗口/焦点错乱；idle CPU < 0.5%；浮窗出现 P95 < 120ms。

### M5（可选）跨平台

- Windows：资源管理器右键菜单（注册表 HKCU）+ CLI + 剪贴板取词（3-5 人日）。
- Linux X11：PRIMARY selection + CLI（3-4 人日）；Wayland 明确降级。

---

## 7. 测试与验收

### 7.1 兼容性矩阵（macOS，人工）

| App | 取词 | 选区矩形 | 备注 |
|---|---|---|---|
| Safari | 预期 ✅ | ✅ | WebArea |
| Chrome | 预期 ✅（需 AX 开关） | ✅ | Chromium |
| VS Code | 预期 ✅（需 AX 开关） | ✅ | Electron |
| Slack / Notion | 待测 | 待测 | Electron |
| 飞书 / 微信 | 待测 | 待测 | 可能有自绘控件 |
| Terminal / iTerm2 | 待测 | 待测 | 版本差异大 |
| Xcode | 待测 | 待测 | 原生 |
| Preview（PDF） | 待测（可能 ❌） | 待测 | 兜底剪贴板 |
| 密码框 / 1Password | 必须 ❌ | — | 安全输入过滤 |

### 7.2 纯逻辑单测（可 CI）

- 选区过滤：空白、超长、黑名单、同位置重复。
- 坐标换算：主屏/副屏/负坐标/clamp/翻转到上方。
- 目录解析：`file://` URL、含空格/中文、相对路径、非目录。
- 配置迁移：旧配置无 `quick_panel` 时的默认值。
- 任务队列：并发上限、取消、失败状态。

### 7.3 性能预算

| 指标 | 目标 |
|---|---|
| 鼠标监听回调 | < 1ms（只投递，不读 AX） |
| AX 取词 P95 | < 20ms，超时 250ms |
| 浮窗出现 | P95 < 120ms |
| 空闲 CPU | < 0.5% |
| 常驻内存增量 | < 20MB |

---

## 8. 待决策问题（建议）

| # | 问题 | 建议 |
|---|---|---|
| 1 | 自动划词浮窗默认开还是关？ | **默认关**，设置里一键开启；避免首次使用就被打扰/要权限 |
| 2 | 选 Agent 后直接执行预设动作，还是统一进输入框？ | **统一进输入框**，预设动作只是填入模板；可预测、可撤销 |
| 3 | 快速任务默认前台还是后台？ | **后台**（LLM 用 taskClient）；ACP 二期补齐 |
| 4 | 目录默认直接执行还是新建 worktree？ | **直接执行**，worktree 作为显式勾选项 |
| 5 | 一期只做 macOS 是否可接受？ | **接受**；Windows/Linux 另立里程碑 |
| 6 | AX 能力自研还是上游给 MyGo？ | 先自研 spike；若稳定，**建议上游**成 `mygo.Selection`，长期维护成本更低 |
| 7 | 是否补浏览器扩展？ | 二期可选；对有权限顾虑的用户是很好的替代 |
| 8 | 快速任务是否写入会话历史？ | **默认写入**（可追溯），提供「不保存」开关 |

---

## 9. 代码定位与参考资料

### 9.1 本仓库

- `internal/ui/app.go:271` 默认快捷键；`:386` 注册；`:485` quickMode；`:500` openQuick
- `internal/ui/conversation.go:61` 新建会话；`:126` 发送；`:260` runStream（全局 client 约束）
- `internal/ui/autotask.go:588` 后台运行模板；`:707` taskClient；`:794` taskAgentSpec
- `internal/ui/create.go:130` Agent 选择器；`:182` 工作区选择；`:316` 共享 composer
- `internal/ui/worktree_ops.go:201` pointAtWorkspace；`internal/ui/sidebar.go:73` 目录选择
- `internal/ui/notify.go:63` 完成通知；`internal/ui/settings.go:129` 设置页
- `internal/config/config.go:14` 配置结构；`main.go` 启动入口（未启用单实例）
- `internal/webui/server.go` 本地 Web 服务（浏览器扩展回调可复用）

### 9.2 MyGo（`github.com/egoist/mygo@v0.6.3`）

- `window.go`：`WindowOptions.Panel`、`ShowInactive`、`SetPosition`、`OnBlur`
- `internal/darwin/window.go`：Panel 的 NSPanel 实现、`orderFrontRegardless`
- `flyout.go`：Flyout 的定位与焦点规则（作为对照）
- `docs/launchers.md`、`docs/windows.md#panels`、`docs/native.md#global-shortcuts`
- `examples/launcher/main.go`：Panel + 全局快捷键 + 托盘 的完整范式
- `deeplink.go`：URL scheme；`single_instance.go`：`OnSecondInstance`

### 9.3 平台文档

- Apple：`AXUIElementCopyAttributeValue`、`kAXSelectedTextAttribute`、`kAXBoundsForRangeParameterizedAttribute`
- Apple：`NSEvent.addGlobalMonitorForEvents(matching:handler:)`——按键事件需辅助功能，鼠标事件不需要
- Apple：Information Property List → `NSServices`（Services 菜单声明）
- 参考产品：PopClip（划词工具条）、Raycast Quick AI、ChatGPT macOS 客户端（Option+Space）

### 9.4 可评估的第三方

- `github.com/LocalKinAI/kinax-go`：纯 Go 的 AX 绑定（`purego` + 内嵌 dylib）。
  - 优点：省掉 AX 桥的开发；有 `FocusedElement`、`Attribute`、`Observer`。
  - 风险：2026-04 新建、0 star、未经验证；**运行时释放内嵌 dylib** 可能影响签名/公证；是否支持 `AXBoundsForRange` 参数化属性需确认。
  - 结论：可用于 spike，不建议直接上生产；生产建议自研最小桥或上游 MyGo。

---

## 附录 A：Darwin 原生桥的最小实现草案

> 目标：不引入 cgo、不依赖第三方 dylib；只 dlopen 系统框架 + `purego/objc`，与 MyGo 内部做法一致。

### A.1 需要的符号

| 框架 | 符号 | 用途 |
|---|---|---|
| ApplicationServices | `AXUIElementCreateApplication` | 由 pid 建 app 元素 |
| ApplicationServices | `AXUIElementCopyAttributeValue` | 读 `AXFocusedUIElement` / `AXSelectedText` / `AXSelectedTextRange` |
| ApplicationServices | `AXUIElementCopyParameterizedAttributeValue` | 读 `AXBoundsForRange` |
| ApplicationServices | `AXValueGetValue` | 把 AXValue 解成 CFRange / CGRect |
| ApplicationServices | `AXUIElementSetMessagingTimeout` | 防卡死 |
| ApplicationServices | `AXIsProcessTrusted` / `AXIsProcessTrustedWithOptions` | 权限状态/引导 |
| AppKit | `[NSWorkspace sharedWorkspace] frontmostApplication` | 前台 App 信息 |
| AppKit | `[NSEvent addGlobalMonitorForEventsMatchingMask:handler:]` | mouseUp 监听（mask = leftMouseUp(1<<2) \| rightMouseUp(1<<4)） |
| AppKit | `[NSPasteboard generalPasteboard]` | 剪贴板文本 / `public.file-url` |
| AppKit | `[NSRunningApplication activateWithOptions:]` | 提交后把焦点还给来源 App |
| Carbon/HIToolbox | `IsSecureEventInputEnabled` | 安全输入过滤 |

### A.2 读取流程（骨架）

```go
//go:build darwin

func readSelection(pid int32) (text string, rect Rect, ok bool) {
    app := axUIElementCreateApplication(pid)
    axSetMessagingTimeout(app, 0.25)
    defer cfRelease(app)

    focused := axCopyElement(app, "AXFocusedUIElement")
    if focused == 0 {
        return "", Rect{}, false
    }
    defer cfRelease(focused)

    if sub := axCopyString(focused, "AXSubrole"); sub == "AXSecureTextField" {
        return "", Rect{}, false
    }
    text = axCopyString(focused, "AXSelectedText")
    if strings.TrimSpace(text) == "" {
        return "", Rect{}, false
    }
    if rng := axCopyValue(focused, "AXSelectedTextRange"); rng != 0 {
        defer cfRelease(rng)
        if r := axCopyRectParam(focused, "AXBoundsForRange", rng); r != nil {
            rect = *r
        }
    }
    return text, rect, true
}
```

要点：

- `AXSelectedTextRange` 是 `AXValue`（CFRange），要原样作为参数化属性的参数传入，**不要自己解包再传**。
- CFString/CFArray 与 `NSString`/`NSArray` toll-free bridged，可直接用 `objc` 的 `nsString(...)` 创建属性名，省掉一套 CF 字符串桥。
- 所有 `Copy*` 返回的对象都要 `CFRelease`，否则长期运行会泄漏（每次划词 3-5 个对象）。
- 读取放到后台 goroutine，结果用 `mygo.RunOnMain` 投递，切勿在主线程同步等待 AX。

### A.3 与现有代码的接缝

1. `main.go`：`mygo.App.RequestSingleInstanceLock()`；`OnSecondInstance` 解析 `--dir` / 目录参数；`OnOpenURL` 解析 `chatgo://quick`。
2. `internal/ui/app.go`：
   - `Show()` 里创建（或延迟创建）Quick Panel；
   - `applyQuickShortcut` 的默认行为改成 `cw.openQuickPanel()`；
   - `invalidate()` 同时刷新面板；
   - `RunInBackground` 之外，新增「浮窗常驻」所需的 `OnWindowAllClosed` 处理（参考 MyGo launcher 示例）。
3. `internal/ui/conversation.go`：抽出 `newConversationAt(dir, agent string, forceDir bool)`，避免 `agent.Workspace` 覆盖用户显式选择的目录（当前 `newConversationIn` 会覆盖，**事实，高**）。
4. `internal/ui/autotask.go`：把 `taskClient` 复用为 `QuickRunner` 的 LLM 后端。
5. `internal/ui/settings.go`：加「划词浮窗」页签或并入「实验」。

### A.4 Spike 验收清单（M0）

- [ ] 权限未授权时：`Available()==false`，UI 给出跳转按钮，不崩溃、不刷日志。
- [ ] Safari 选中一段中英混排文本：文本与矩形正确，浮窗出现在选区下方。
- [ ] Chrome（未开读屏器）：验证 `AXManualAccessibility` 是否让 `AXSelectedText` 生效。
- [ ] VS Code：同上。
- [ ] Terminal：记录是否可读。
- [ ] 双屏（副屏在主屏左侧，负坐标）：浮窗仍在正确的屏幕。
- [ ] 全屏 App（Safari 全屏）：浮窗可见且可点击。
- [ ] 密码框：不弹窗。
- [ ] 连续快速划选 20 次：无窗口残留、无焦点错乱。
- [ ] 空闲 5 分钟：CPU 无明显占用（监听应为事件驱动）。

### A.5 需要实测确认的点

1. Retina 下 AX 矩形单位是点还是像素。
2. macOS 15+ 对程序化读取剪贴板是否弹隐私提示（若弹，剪贴板兜底必须由用户显式触发并在文案中说明）。
3. 飞书/微信/钉钉等自绘控件是否暴露 `AXSelectedText`。
4. Finder 的 `AXSelectedChildren`/`AXURL` 在当前系统版本是否稳定。
5. `NSPasteboard` 的 `public.file-url` 在 Finder 复制目录时是否总是可用（不同系统版本行为可能不同）。

---

## 附录 B：方案对比小结（给决策用）

| 维度 | 全自动（AX + 鼠标监听） | 快捷键优先（推荐） | Services 右键 | 浏览器扩展 |
|---|---|---|---|---|
| 体验 | 最好 | 好（多按一次键） | 一般 | 浏览器内最好 |
| 权限 | 辅助功能 | 无（AX 增强时才需要） | 无 | 无 |
| 覆盖率 | 中-高（依赖 App） | 100%（剪贴板/粘贴兜底） | 高 | 仅浏览器 |
| 误触发 | 有 | 无 | 无 | 无 |
| 开发量 | 高 | 中 | 中 | 中 |
| 适合作为 | 二期增强 | **一期主线** | 增强入口 | 可选补充 |

**结论**：一期做「快捷键优先 + Quick Panel + 目录流」，把 AX 取词作为**增强能力**接入同一面板；二期再打开「划词自动浮窗」。这样既能让功能尽快可用，又能把权限与兼容性风险控制在可接受范围内。

---

## 附录 C：本分支的实现状态（feat/quick-panel）

按上文「一期」落地，代码已就位；**尚未在真机上跑过**（本环境不做编译/运行验证），
需要按 A.4 的清单做一次 M0 验收。

### 已实现

| 能力 | 位置 |
|---|---|
| 系统级取词 / 取目录 / 剪贴板 / 全局鼠标监听 | `internal/capture`（darwin，purego + objc，无 cgo） |
| 浮窗（bar / compose / running 三态、定位、失焦隐藏、Esc） | `internal/ui/quickpanel.go`、`quickpanel_view.go` |
| 后台任务 runner（LLM/Group 独立 client；ACP 以会话为键） | `internal/ui/quicktask.go` |
| 快捷键复用与设置项 | `internal/ui/app.go`、`settings_quick.go`（设置 → 系统） |
| 命令行 / URL scheme / 单实例 | `main.go`、`mygo.json`（`chatgo`） |
| 配置 | `internal/config/config.go` 的 `quick_panel` 段 |
| 纯逻辑单测（过滤、路径、定位） | `internal/capture/capture_test.go` |

### 与调研稿的差异

- 采用双快捷键：紧凑窗口仍是 `QuickChatShortcut`（默认 Cmd/Ctrl+Shift+Space），浮窗新增独立 `quick_panel.shortcut`（默认 Cmd/Ctrl+Shift+K），两者分别注册、分别配置。
- 自动弹窗默认关闭；后台执行默认开启；剪贴板兜底默认开启。
- `default_worktree` 已在面板里接通：勾选后先在所选目录执行 `git worktree add -b quick-<时间>`，再在隔离树里跑任务。
- 目录优先取剪贴板的 `public.file-url`，其次 Finder 的 `AXSelectedChildren → AXURL`，最后是纯文本路径。
- 划词条额外装了一个 mouseDown 全局监听：点击面板外任意位置立即收起，点击条本身不受影响。
- macOS 之外暂不读取系统选区，但剪贴板兜底已接通：先复制文本或目录，再按快捷键即可使用；Windows 的 `file:///C:/...` 路径也已解析。
- 设置 → 系统可配置默认 Agent、自动弹出的最短选区、默认新建 worktree。
- Windows 侧已把「用 ChatGo 快捷任务打开」写进 HKCU 的文件夹与文件夹背景右键菜单，命令带 `--dir "%1"/"%V"`。
- Windows 增加可选的「自动复制选区」（opt-in）：快捷键触发时合成 Ctrl+C，读剪贴板并尽量还原文本；不引入 COM/UIA。
- Linux 侧通过 `xclip` / `xsel` / `wl-paste` 读取 X11 PRIMARY 选区，统一走同一个 `Watch` 之外的单次读取路径。
- 权限引导：未授权时点「请求权限」会调用 `AXIsProcessTrustedWithOptions(kAXTrustedCheckOptionPrompt)`，由系统弹窗并自动把 ChatGo 加进辅助功能列表；也可直接跳到系统设置。

### 下一步（需要真机）

1. 按 A.4 逐项验证 AX 覆盖率、Retina 坐标、全屏/多屏、Electron 的 `AXManualAccessibility`。
2. 观察 `CHATGO_CAPTURE_DEBUG=1` 的日志，确认取词来自 AX 而不是剪贴板兜底。
3. 真机检查浮窗焦点：划词条首击是否命中按钮、输入面板是否拿到键盘、隐藏后焦点是否回到来源 App。
4. 打包验证 TCC 授权在签名/公证后的稳定性。
5. 视验证结果决定是否把 `capture` 上游为 MyGo 公共 API。
