# ChatGo 自建中继部署与手机端构建

本文对应 [手机端中继连接设计](superpowers/specs/2026-11-18-mygo-mobile-relay-design.md) 的 P1–P4 实现。

## 组件

| 组件 | 位置 | 作用 |
|------|------|------|
| 中继服务 | `cmd/relayd`、`internal/relay` | 公网 WSS 入口、配对、设备令牌、`/api` 反向代理、APNs/FCM 推送 |
| 桌面连接器 | `internal/relayclient`、`internal/ui/relay_service.go` | 桌面主动外连、回环 webui 转发、设备/配对/通知控制 |
| 手机 App | `cmd/mobile`、`internal/mobileapi` | MyGo 原生 iOS/Android App，Chat / Room / 自动任务 |
| 桌面设置 | `internal/ui/settings_mobile.go` | 启用中继、地址、二维码、配对码、设备吊销 |

中继只代理 `/d/<desktop_id>/api/...` 和 `/api/events`；不代理静态页、`/mcp`、`/agui`、`/a2a`。桌面 Web Token 只由连接器在本机注入，不下发手机。

## 1. 构建并运行中继

### 一键安装（Linux，推荐）

在全新的 Linux 服务器上安装 `relayd`、systemd 服务和 nginx 反向代理：

```sh
curl -fsSL https://raw.githubusercontent.com/tk103331/ChatGo/main/scripts/install-relay.sh \
  | sudo bash -s -- --port 8080 --public-url http://relay.example.com:8080

# 或从仓库运行
sudo bash scripts/install-relay.sh --port 8080 --public-url http://relay.example.com:8080
```

脚本默认从 GitHub Release 下载 `relayd-linux-<arch>`，创建 `chatgo-relay`
用户与 `/var/lib/chatgo-relay/relay.db`，并由 nginx 在 8080 端口代理到
`127.0.0.1:8787`。重复执行会升级二进制，保留数据库。

HTTPS：

```sh
sudo bash scripts/install-relay.sh --port 443 --domain relay.example.com \
  --tls-cert /etc/letsencrypt/live/relay.example.com/fullchain.pem \
  --tls-key /etc/letsencrypt/live/relay.example.com/privkey.pem
```

推送、直接监听、未发布版本等选项：

```sh
sudo bash scripts/install-relay.sh --help
sudo bash scripts/install-relay.sh --apns-key AuthKey.p8 --apns-key-id ABC123 \
  --apns-team-id TEAM123456 --apns-topic com.example.app
sudo bash scripts/install-relay.sh --no-nginx --addr 0.0.0.0:8787 \
  --public-url https://relay.example.com
sudo bash scripts/install-relay.sh --binary ./relayd
```

脚本会写入 `/etc/systemd/system/chatgo-relay.service` 与
`/etc/nginx/conf.d/chatgo-relay.conf`；如果 nginx 配置检查失败，会恢复原配置。
Release 尚未附带 relayd 资产时，脚本会在检测到 Go 工具链后从源码构建；
也可以直接用 `--binary ./relayd` 指定本地二进制。

### 手动构建与运行

```sh
go build -o relayd ./cmd/relayd

# 开发（HTTP，仅本机）
./relayd -addr 127.0.0.1:8787 -data ./relay.db

# 生产：由 Caddy/Nginx 终止 TLS，或将证书交给 relayd
./relayd -addr :8787 -data /var/lib/chatgo/relay.db \
  -public-url https://relay.example.com \
  -tls-cert /etc/letsencrypt/live/relay.example.com/fullchain.pem \
  -tls-key  /etc/letsencrypt/live/relay.example.com/privkey.pem
```

健康检查：

```sh
curl -s https://relay.example.com/healthz
# {"ok":true,"protocol":1}
```

`relay.db` 保存桌面、设备和推送 token；请定期备份并限制文件权限。

## 2. 桌面端

1. 启动 ChatGo，打开 **设置 → 手机**。
2. 启用“启用中继”，填写 `wss://relay.example.com`，保存并重连。
3. 打开“允许新设备配对”，点“生成配对码”（2 分钟有效）。
4. 手机 App 扫码/输入配对码后，桌面“已配对设备”会出现该设备，可随时吊销。

桌面只需要出站网络；不需要公网 IP、端口映射，也不要求开启“Web 服务”。连接器会在 `127.0.0.1` 上按需启动一个仅回环的 webui，供中继转发。

配置位于 `~/.chatgo/config.yaml`：

```yaml
relay:
  enabled: true
  url: wss://relay.example.com
  desktop_id: d_7f3a...
  desktop_secret: ...   # 生产环境应迁移到系统钥匙串
  allow_pairing: true
  devices:
    - id: dev_...
      name: iPhone
      platform: ios
```

> `desktop_secret` 和每设备的 `e2e_key` 目前仍写在配置文件里；后续会迁移到 `mygo.Secrets`。请确保 `~/.chatgo/config.yaml` 仅当前用户可读。E2E 已覆盖 HTTP 请求/响应与事件帧，中继只看到密文。

## 3. 构建手机 App

要求：MyGo v0.10.0、Go 1.27+；iOS 需要 Xcode，Android 需要 SDK/NDK/JDK 17+。

```sh
# iOS 模拟器
mygo build -platform ios/simulator ./cmd/mobile

# iOS 真机/TestFlight：先在 cmd/mobile/mygo.json 填 ios.team 和 profile
mygo build -platform ios ./cmd/mobile

# Android（需要 JDK 17+）
mygo build -platform android ./cmd/mobile
```

产物在 `dist/mobile/`。手机端配置在 `cmd/mobile/mygo.json`；上线前替换：

- `ios.team`
- `ios.associatedDomains` / `android.appLinks` 中的 `relay.example.com`
- 应用图标、版本号、签名 keystore

开发调试：

```sh
mygo dev -platform ios -device "iPhone 15 Pro" ./cmd/mobile
mygo dev -platform android ./cmd/mobile
```

模拟器连本地中继时，中继地址填：

- iOS 模拟器：`ws://127.0.0.1:8787`
- Android 模拟器：`ws://10.0.2.2:8787`

## 4. 推送（可选）

中继支持 APNs 和 FCM。没有配置时，通知只写入中继日志，不会到达手机。

```sh
./relayd \
  -apns-key /secrets/AuthKey_ABC123.p8 \
  -apns-key-id ABC123 \
  -apns-team-id TEAM123456 \
  -apns-topic com.tk103331.chatgo.mobile \
  -apns-env prod \
  -fcm-service-account /secrets/firebase.json
```

手机启动时会向中继注册推送 token：

```http
POST /v1/push
Authorization: Bearer <device_token>
X-ChatGo-Desktop-ID: d_...
{"provider":"apns","token":"...","env":"prod"}
```

桌面在自动任务完成、Agent 提问/权限审批和群聊轮次结束时，经隧道发送 `POST /__relay/notify`；中继只发通用文案 + `session_id`，不发送提示词或回复正文。

## 5. 验证

```sh
go test ./internal/relay/... ./internal/relayclient/... ./cmd/mobile/...
go test -race ./internal/relay/...
go build ./...
```

P1 集成测试覆盖：配对、设备令牌、`/api` 转发、吊销、桌面离线 503、未授权 401。

手机端 UI 渲染测试：

```sh
go test ./cmd/mobile/...
```

## 6. 已知限制

- 手机端使用 `/api/events` WebSocket 触发刷新，并保留 1s 轮询作为断线/旧桌面回退。
- E2E 已覆盖 HTTP 请求/响应与事件帧，并带防重放：`X-E2E-Time` 参与 AAD，桌面拒绝 ±2 分钟之外的请求并对每台设备做 nonce 去重。尚未实现 Ed25519 请求签名。
- 手机时钟偏差超过 2 分钟时，所有加密请求会被桌面拒绝；App 会提示“请校准手机时间”。
- 自动任务仍在桌面运行；桌面退出/休眠时手机显示离线。
- iOS 后台推送依赖 APNs 证书/Profile；Android 需要 FCM 服务账号。
- App 内扫码已实现（Android zxing-android-embedded，iOS AVFoundation）；模拟器没有摄像头时仍可手动输入配对码/桌面 ID。深链会直接开始配对，空状态页显示“正在配对”/失败原因。
- 手机端会话支持新建、切换、改名、删除；消息编辑、重生成和回滚仍在桌面完成。
- ACP Agent 在手机端提供模型选择，模式与开关仍由桌面控制；A2A Agent 自带模型，手机不显示模型选择。
- 手机端「项目」页提供只读文件浏览、Git 状态/Diff/日志/暂存/提交，以及交互式 PTY 终端。终端通过 WebSocket 传输，会话常驻桌面，手机断线或退出 App 后重新进入会接回同一个 shell；输出保留 512 KB，断线期间补齐，超出后手机重置屏幕。终端按纯文本屏幕渲染，暂不显示颜色。文件暂不支持编辑、删除或重命名。
- 新建项目会在用户目录或已有项目内创建目录，可选 `git init`，并自动加入桌面项目列表。
- 手机端不提供全局模型/MCP 配置编辑。

## 7. 排障

| 现象 | 检查 |
|------|------|
| 桌面一直“连接中” | 中继地址是否 `wss://`；TLS 证书；反向代理是否升级 WebSocket |
| 手机 503 `desktop_offline` | 桌面 ChatGo 是否运行、设置里中继是否“已连接” |
| 手机 401 | 设备被吊销或令牌丢失；App 会自动清除令牌并提示“需要重新配对” |
| 手机提示“请校准手机时间” | 手机与桌面时钟相差超过 2 分钟，打开系统自动时间同步后重试 |
| 配对 403 | 配对码过期/已被使用；重新生成 |
| 收不到推送 | relayd 是否带 APNs/FCM 参数；手机是否注册 token；iOS Profile 是否有 Push Notifications |
| 手机连不上本地中继 | iOS 模拟器用 `127.0.0.1`，Android 模拟器用 `10.0.2.2` |
