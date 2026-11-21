# MyGo Mobile 调研

> 日期：2026-11-18 · 范围：MyGo v0.10.0 的手机端能力、构建链、与本仓库的兼容性
> 结论先行：**MyGo 已经具备生产可用的 iOS / Android 能力，但本仓库锁定的 v0.9.0 没有移动端；升级到 v0.10.0 的源码兼容性已实测通过。**
> 关联方案：[ChatGo 手机端中继连接方案](../superpowers/specs/2026-11-18-mygo-mobile-relay-design.md)

---

## 1. 版本与可用性

| 事实 | 证据 |
|------|------|
| 本仓库使用 MyGo **v0.9.0** | `go.mod:25` |
| v0.9.0 **没有**手机端构建链 | 模块内无 `docs/mobile/`、`cmd/mygo/ios.go`、`cmd/mygo/android.go`、`ui/mobile/`，也没有 `backend_mobile.go` |
| 最新版本为 **v0.10.0** | `go list -m -versions github.com/egoist/mygo` 的最后一个版本 |
| v0.10.0 提供完整手机端 | `docs/mobile/{README,building,interface,plugins}.md`、`cmd/mygo/{ios,android,mobile_dev,mobile_modules}.go`、`ui/mobile/`、`backend_mobile.go`、`touch_mobile.go` |
| 官方支持 iOS / Android / iPadOS | [Phones and tablets](https://github.com/egoist/mygo/blob/main/docs/mobile/README.md) |

这不是“另一个框架分支”，而是同一套 Go 代码、同一套 `ui` API 编译到手机：

```sh
mygo doctor
mygo devices
mygo dev -platform ios
mygo dev -platform android
mygo build -platform ios        # .ipa
mygo build -platform android    # .apk + .aab
mygo publish -platform ios
mygo publish -platform android
```

---

## 2. 手机端能力清单

### 2.1 界面与导航

| 能力 | 说明 | 对 ChatGo 的价值 |
|------|------|------------------|
| 原生 UI（GPU） | iOS 用 Metal，Android 用 OpenGL ES；不是 WebView | 消息列表、输入框、流式渲染的体验接近原生 |
| WebView 窗口 | iOS WKWebView / Android WebView，可与原生 UI 混用 | 本方案**不使用**；手机 App 直接做原生 UI |
| 系统底部 Tab | `mygo.NewTabs(TabItem{...})`，iOS 用 UIKit、Android 用 Material | Chat / 群聊 / 任务 / 设置四个主入口 |
| 导航栈 | `Window.Push` / `Window.Pop`，Android 返回键、iOS 左缘手势 | 会话详情、群聊详情、任务详情 |
| `ui.Router` 系统栈 | `SystemStack` 让页面历史变成系统导航栈 | 聊天页内部的设置/成员/多级页面 |
| 安全区 | 原生 UI 自动布局在安全区；`Context.SafeArea` 可读取 | 刘海、底部 Home 条、键盘 |
| 触摸手势 | 点击、长按、滑动锁定、下拉刷新、双指缩放、边缘返回 | `mobile.SwipeRow` 左滑重命名/删除；`PullToRefresh` 刷新列表 |
| 手机组件 | `ui/mobile`：Sheet、ActionSheet、Drawer、PageView、SwipeRow、Wheel、DatePicker、TopBar、BottomBar、FloatingButton、ListRow、Section、Ink、Switch、Zoomable | 新建会话、模型选择、权限审批、任务编辑 |
| 键盘 | 系统输入法、联想、组合输入；`TextInput` 可指定键盘类型 | 聊天输入、搜索 |
| 无障碍 | VoiceOver / TalkBack | 系统级要求 |
| 生命周期 | `OnSuspend` / `OnResume` / `OnMemoryWarning`；`KeepState` 恢复状态 | 前后台切换、断线重连、内存回收 |
| 深链 | `urlSchemes` + `App.OnOpenURL`；iOS Associated Domains / Android App Links | 扫码配对、推送点开、`chatgo://session/...` |

### 2.2 设备与系统能力

官方插件按需引入，不引入不增加包体与权限：

| 插件 | 能力 | 在本方案中的用途 |
|------|------|------------------|
| `push` | APNs（iOS）/ FCM（Android）令牌、消息、点击打开 | 自动任务完成、Agent 提问、权限审批的通知 |
| `background` | BGTaskScheduler / WorkManager，系统择机执行 | 前台列表的低频刷新；**不能替代推送** |
| `preferences` | 小值持久化 | 服务器列表、上次打开的会话 |
| `secrets`（核心 API） | iOS Keychain / Android Keystore 加密存储 | 设备令牌、E2E 私钥 |
| `camera` / `imagepicker` | 相机预览、拍照、系统相册 | 聊天图片附件；可选的二维码扫描 |
| `permissionflow` | 相机、相册、通知等权限申请与跳转设置 | 首次配对/发图/通知 |
| `network` | 在线状态、是否计费网络 | 离线提示、移动网络下暂停轮询 |
| `sqlite` | 内置 SQLite + FTS | 可选的会话缓存/离线搜索 |
| `biometrics` | Face ID / Touch ID / 指纹 | 打开 App 时解锁设备令牌（可选） |
| `share` | 系统分享面板 | 分享会话/消息 |
| `haptics` | 选择、影响、结果反馈 | 发送、停止、审批的触感 |

与手机无关的桌面能力（菜单栏、托盘、Dock、全局快捷键、自动更新）在手机上自动失效，不需要条件编译。

### 2.3 构建与分发要求

| 平台 | 要求 | 本机现状（`mygo doctor`） |
|------|------|--------------------------|
| iOS | macOS + Xcode；真机/商店需要 Apple 开发者团队与 Provisioning Profile；Go 通过 cgo 链接 | Xcode 27.2、iOS SDK 27.2、10 个模拟器（1 个已启动）✅；无签名身份、无 Profile ⚠️ |
| Android | Android Studio、SDK、NDK、JDK 17+ | SDK API 36、NDK 26.1、adb、2 个模拟器 ✅；JDK 11.0.19 过旧 ⚠️ |
| 桌面 | 不需要 cgo，也不需要 Xcode/NDK | 现有桌面构建不受影响 |

默认最低系统版本（可在 `mygo.json` 调整）：

- iOS `minimumSystemVersion` 默认 **17.0**
- Android `minSdk` 默认 **26**（Android 8.0）

分发：`mygo build -platform ios` 产出签名 `.ipa`，`mygo publish` 上传 App Store Connect/TestFlight；Android 产出 `.aab`，`mygo publish -platform android -track internal` 上传 Google Play 内部测试。真机调试需要 Developer Mode（iOS）/ USB 调试（Android）。

---

## 3. 限制与坑

1. **后台不是常驻**：手机系统会挂起 App；WebSocket 在后台不可靠。实时性必须靠推送，前台再拉取详情。
2. **`background` 是“尽力而为”**：iOS 约 30 秒（`Long` 才可能几分钟），Android 约 10 分钟，且由系统决定时机。不能把自动任务调度放在手机端。
3. **cgo 是硬门槛**：iOS/Android 构建必须启用 cgo；`CGO_ENABLED=0` 的交叉编译会在 `purego` / `internal/mobile` 处失败，这是预期行为，不是代码问题。
4. **WebView 不是原生的替代品**：现有 wasm 客户端 `web/app.wasm` 为 **23 MB**（gzip 后约 **5.8 MB**，brotli 约 **4.2 MB**）。蜂窝网络首次加载成本高，且交互手感与原生 UI 有差距；因此本方案直接使用原生 UI。
5. **App Store / Google Play 合规**：远程控制桌面、执行 Agent 工具属于敏感能力；需要隐私清单、加密出口合规声明、清晰的权限说明，审核周期要预留。
6. **MyGo v0.10.0 较新**：手机端 API 仍在快速演进；升级前要跑完整桌面回归（见第 4 节）。

---

## 4. 与本仓库的兼容性实测

在临时 git worktree 中只把 MyGo 从 v0.9.0 升到 v0.10.0：

```sh
go mod edit -require=github.com/egoist/mygo@v0.10.0
go mod tidy
go build ./...
```

结果：

| 检查 | 结果 |
|------|------|
| `go build ./...`（桌面） | ✅ 通过 |
| `GOOS=js GOARCH=wasm go build ./cmd/web` | ✅ 通过 |
| `GOOS=darwin GOARCH=arm64 go build ./cmd/web` | ✅ 通过 |
| `go.mod` / `go.sum` 变更 | 仅 MyGo 版本两处，无其他依赖漂移 |
| `GOOS=android/ios` + `CGO_ENABLED=0` | ❌ 预期失败（需要 cgo 与手机工具链） |

结论：**升级本身风险低**；真正的工作量在手机 UI、中继服务和配对/推送，不在依赖升级。

---

## 5. 对 ChatGo 的启示

### 5.1 可直接复用

- `pkg/models` 的会话、消息、Turn、ToolCall、Plan 结构是纯 Go，手机端可直接反序列化 `/api` 返回。
- `internal/webui` 已经提供 Chat、Room、自动任务的完整 JSON API，以及 Token 认证、能力协商、附件上传。
- `cmd/web` 是用 MyGo `ui` 写的完整客户端，已经实现列表、聊天、群聊、任务、工具、提问、权限、模型选择等页面；它的 API/类型层可以抽出来给手机端复用。
- `cmd/web` 的 700ms 轮询虽然简单，但已经能表达流式回复、工具调用和交互请求。

### 5.2 不能直接复用

- `internal/ui` 是桌面专用：侧栏、文件面板、Git diff、终端、窗口标题栏、托盘、命令面板。手机端不应引入。
- `cmd/web` 的布局是桌面三栏；手机上需要底部 Tab + 导航栈 + Sheet。
- `cmd/web` 的 `apiGet` 使用根绝对 `/api` 路径和浏览器 Cookie；手机端使用自己的 `BaseURL` 与设备令牌，固定走自建中继。

### 5.3 建议

1. **先升级 MyGo 到 v0.10.0**，保留桌面构建与 wasm 构建的回归测试。
2. 手机端直接做 **原生 MyGo UI**，不使用 WebView；现有 wasm/PWA 客户端保留给浏览器和桌面调试。
3. 把 `cmd/web` 的 API/类型/消息渲染抽成可复用包，避免手机端复制第三套协议代码。
4. 中继只做传输，不复制 Chat/Room/Task 业务逻辑；桌面仍是唯一事实来源。

---

## 6. 调研方法（可复现）

```sh
# 1. 版本
go list -m -versions github.com/egoist/mygo

# 2. 对比 v0.9.0 与 v0.10.0 的模块内容
ls "$(go env GOMODCACHE)/github.com/egoist/mygo@v0.9.0"
ls "$(go env GOMODCACHE)/github.com/egoist/mygo@v0.10.0/docs/mobile"

# 3. 在临时 worktree 中验证升级
git worktree add --detach /tmp/chatgo-probe HEAD
cd /tmp/chatgo-probe
go mod edit -require=github.com/egoist/mygo@v0.10.0
go mod tidy && go build ./...
GOOS=js GOARCH=wasm go build -o /dev/null ./cmd/web

# 4. 手机工具链
go build -o /tmp/mygo010 github.com/egoist/mygo/cmd/mygo
/tmp/mygo010 doctor
```

---

## 7. 参考

- MyGo 手机与平板：https://github.com/egoist/mygo/blob/main/docs/mobile/README.md
- 手机端构建与发布：https://github.com/egoist/mygo/blob/main/docs/mobile/building.md
- 手机界面、触摸与文本：https://github.com/egoist/mygo/blob/main/docs/mobile/interface.md
- 设备 API 与插件：https://github.com/egoist/mygo/blob/main/docs/mobile/plugins.md
- MyGo 配置（iOS/Android 字段）：https://github.com/egoist/mygo/blob/main/docs/configuration.md
- 本仓库现有 Web API：`internal/webui/server.go:221`
- 本仓库现有 wasm 客户端：`cmd/web/main.go:583`
