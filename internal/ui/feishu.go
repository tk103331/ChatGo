package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"chatgo/internal/config"
	"chatgo/internal/llm"
	"chatgo/pkg/models"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	acp "github.com/coder/acp-go-sdk"
	mgo "github.com/egoist/mygo/ui"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/larksuite/oapi-sdk-go/v3/channel"
	"github.com/larksuite/oapi-sdk-go/v3/channel/types"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	"github.com/larksuite/oapi-sdk-go/v3/scene/registration"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	qrcode "github.com/skip2/go-qrcode"
)

type feishuConnState int

const (
	feishuStopped feishuConnState = iota
	feishuConnecting
	feishuConnected
	feishuReconnecting
	feishuError
)

func (cw *ChatWindow) setChannelState(state feishuConnState, note string) {
	cw.channelMu.Lock()
	cw.channelState = state
	cw.channelNote = note
	cw.channelMu.Unlock()
	cw.postMain(func() { cw.invalidate() })
}

func (cw *ChatWindow) feishuStatus() (feishuConnState, string) {
	cw.channelMu.Lock()
	defer cw.channelMu.Unlock()
	return cw.channelState, cw.channelNote
}

func (cw *ChatWindow) startFeishuChannel() {
	cw.channelMu.Lock()
	if cw.channelRunning {
		cw.channelMu.Unlock()
		return
	}
	cw.channelMu.Unlock()

	var conf *config.Channel
	for i := range cw.cfg.Channels {
		if cw.cfg.Channels[i].Type == "feishu" && cw.cfg.Channels[i].Enabled {
			conf = &cw.cfg.Channels[i]
			break
		}
	}
	if conf == nil {
		return
	}

	appID, appSecret, channelID := conf.AppID, conf.AppSecret, conf.ID
	client := lark.NewClient(appID, appSecret)
	wsClient := larkws.NewClient(appID, appSecret, larkws.WithEventHandler(dispatcher.NewEventDispatcher("", "")), larkws.WithAutoReconnect(true))
	ch := channel.NewChannel(client, wsClient)

	ch.OnMessage(func(_ context.Context, msg *types.NormalizedMessage) error {
		cw.feishuLogf("收到消息: chatType=%q mentioned=%v text=%q chatID=%q", msg.ChatType, msg.MentionedBot, msg.Content, msg.ChatID)
		if !msg.MentionedBot && msg.ChatType != "p2p" {
			cw.feishuLogf("消息被过滤（非私聊且未@机器人）")
			return nil
		}
		text := strings.TrimSpace(msg.Content)
		if text == "" {
			cw.feishuLogf("消息内容为空，跳过")
			return nil
		}
		cw.notifyChannelMessage(channelID, text)
		feishuReaction(client, msg.MessageID, "Typing")
		cw.runChannelAgent(channelID, text, func(reply string) {
			_, _ = ch.Send(context.Background(), &types.SendInput{
				ChatID:         msg.ChatID,
				Markdown:       reply,
				ReplyMessageID: msg.MessageID,
			})
		})
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	cw.channelMu.Lock()
	cw.channelRunning = true
	cw.channelCancel = cancel
	cw.channelMu.Unlock()
	cw.setChannelState(feishuConnecting, "正在连接…")

	ch.OnReady(func() { cw.setChannelState(feishuConnected, "长连接已建立") })
	ch.OnReconnecting(func() { cw.setChannelState(feishuReconnecting, "网络中断，重连中…") })
	ch.OnReconnected(func() { cw.setChannelState(feishuConnected, "重连成功") })
	ch.OnDisconnected(func() { cw.setChannelState(feishuStopped, "连接已断开") })
	ch.OnError(func(err error) { cw.setChannelState(feishuError, err.Error()) })

	go func() {
		err := ch.Start(ctx)
		cw.channelMu.Lock()
		cw.channelRunning = false
		cw.channelCancel = nil
		cw.channelMu.Unlock()
		if err != nil {
			cw.setChannelState(feishuError, err.Error())
			cw.toast("飞书连接结束: "+err.Error(), mcore.SeverityError)
		} else {
			cw.setChannelState(feishuStopped, "未连接")
		}
	}()
}

func (cw *ChatWindow) stopFeishuChannel() {
	cw.channelMu.Lock()
	cancel := cw.channelCancel
	cw.channelRunning = false
	cw.channelCancel = nil
	cw.channelMu.Unlock()
	if cancel != nil {
		cancel()
	}
	cw.setChannelState(feishuStopped, "未连接")
}

func (cw *ChatWindow) runChannelAgent(channelID, text string, done func(string)) {
	if reply, handled := cw.channelCommand(channelID, text); handled {
		done(reply)
		return
	}
	spec := cw.channelSpec(channelID)
	if spec == nil {
		done("⚠️ 渠道不存在")
		return
	}
	conv := cw.ensureChannelConv(channelID)
	if conv == nil {
		done("⚠️ 无法创建渠道会话")
		return
	}
	if connName, ok := strings.CutPrefix(spec.agent, "acp:"); ok {
		cw.runChannelACPAgent(conv, connName, spec.workspace, text, done)
		return
	}
	cw.runChannelLLMAgent(conv, spec.agent, spec.workspace, text, done)
}

type channelSpec struct {
	agent     string
	workspace string
}

func (cw *ChatWindow) channelSpec(channelID string) *channelSpec {
	return onMain(cw, func() *channelSpec {
		c := cw.channelByID(channelID)
		if c == nil {
			return nil
		}
		return &channelSpec{agent: c.Agent, workspace: c.Workspace}
	})
}

func (cw *ChatWindow) runChannelLLMAgent(conv *models.Conversation, agent, workspace, text string, done func(string)) {
	conv.Messages = append(conv.Messages, models.Message{ID: cw.newID(), Role: "user", Content: text, Timestamp: time.Now()})
	msgs := buildMessages(conv, len(conv.Messages))
	if sys := cw.taskSystemPrompt(agent); sys != "" {
		msgs = append([]llm.ChatMessage{{Role: "system", Content: sys}}, msgs...)
	}
	client, react, err := cw.taskClient(agent, workspace)
	if err != nil {
		done("⚠️ " + err.Error())
		return
	}
	go func() {
		ctx := context.Background()
		reply := ""
		var runErr error
		if react != nil {
			resp, e := react.Chat(ctx, msgs, nil, nil)
			runErr = e
			if resp != nil {
				reply = resp.Content
			}
		} else {
			resp, e := client.Chat(ctx, msgs, nil)
			runErr = e
			if resp != nil {
				reply = resp.Content
			}
		}
		if runErr != nil {
			reply = "⚠️ " + runErr.Error()
		}
		cw.postMain(func() {
			conv.Messages = append(conv.Messages, models.Message{ID: cw.newID(), Role: "assistant", Content: reply, Timestamp: time.Now()})
			_ = cw.convManager.SaveConversation(conv)
			cw.reloadConvs()
			cw.invalidate()
			cw.notifySessionComplete(conv, reply)
		})
		done(reply)
	}()
}

func (cw *ChatWindow) runChannelACPAgent(conv *models.Conversation, connName, workspace, text string, done func(string)) {
	conn, ok := cw.findACPConn(connName)
	if !ok {
		done("⚠️ 未找到 ACP 连接：" + connName)
		return
	}
	go func() {
		conv.Agent = "acp:" + connName
		if conn.Cwd != "" {
			conv.Workspace = conn.Cwd
		} else if workspace != "" {
			conv.Workspace = workspace
		}
		conv.Messages = append(conv.Messages, models.Message{ID: cw.newID(), Role: "user", Content: text, Timestamp: time.Now()})
		cw.acpMu.Lock()
		cw.acpConvObj[conv.ID] = conv
		cw.acpMu.Unlock()
		rt, err := cw.acpEnsureSession(conv, conn)
		if err != nil {
			done("⚠️ " + err.Error())
			return
		}
		cw.acpMu.Lock()
		cw.channelSess[rt.sessionID] = true
		cw.channelReply[rt.sessionID] = &strings.Builder{}
		cw.acpMu.Unlock()
		_, perr := rt.conn.Prompt(context.Background(), acp.PromptRequest{
			SessionId: rt.sessionID,
			Prompt:    []acp.ContentBlock{acp.TextBlock(text)},
		})
		cw.acpMu.Lock()
		var reply string
		if b := cw.channelReply[rt.sessionID]; b != nil {
			reply = strings.TrimSpace(b.String())
		}
		delete(cw.channelReply, rt.sessionID)
		cw.acpMu.Unlock()
		cw.postMain(func() {
			_ = cw.convManager.SaveConversation(conv)
			cw.reloadConvs()
			cw.invalidate()
			cw.notifySessionComplete(conv, reply)
		})
		if perr != nil && reply == "" {
			reply = "⚠️ " + perr.Error()
		}
		if reply == "" {
			reply = "（Agent 没有文本输出）"
		}
		done(reply)
	}()
}

func (cw *ChatWindow) channelByID(id string) *config.Channel {
	for i := range cw.cfg.Channels {
		if cw.cfg.Channels[i].ID == id {
			return &cw.cfg.Channels[i]
		}
	}
	return nil
}

// notifyChannelMessage notifies the user when a channel receives a message.
func (cw *ChatWindow) notifyChannelMessage(channelID, text string) {
	onMain(cw, func() struct{} {
		ch := cw.channelByID(channelID)
		name := "渠道"
		agent := chatAgentName
		if ch != nil {
			if n := strings.TrimSpace(ch.Name); n != "" {
				name = n
			}
			agent = agentLabel(ch.Agent)
		}
		cw.notifyUser(agent+" · "+name, notifyPreview(text))
		return struct{}{}
	})
}

// feishuReaction adds an emoji reaction to a Feishu message as an
// acknowledgement that it was received.
func feishuReaction(client *lark.Client, messageID, emoji string) {
	go func() {
		req := larkim.NewCreateMessageReactionReqBuilder().
			MessageId(messageID).
			Body(larkim.NewCreateMessageReactionReqBodyBuilder().
				ReactionType(larkim.NewEmojiBuilder().EmojiType(emoji).Build()).
				Build()).
			Build()
		_, err := client.Im.V1.MessageReaction.Create(context.Background(), req)
		if err != nil {
			return
		}
	}()
}

func (cw *ChatWindow) channelConvTitle(ch config.Channel, n int) string {
	name := strings.TrimSpace(ch.Name)
	if name == "" {
		name = "渠道"
	}
	return fmt.Sprintf("%s · 会话%d", name, n)
}

func (cw *ChatWindow) ensureChannelConv(channelID string) *models.Conversation {
	return onMain(cw, func() *models.Conversation {
		ch := cw.channelByID(channelID)
		if ch == nil {
			return nil
		}
		if len(ch.ConvIDs) == 0 && ch.ConvID != "" {
			ch.ConvIDs = []string{ch.ConvID}
		}
		if len(ch.ConvIDs) > 0 {
			if ch.ActiveConv < 0 || ch.ActiveConv >= len(ch.ConvIDs) {
				ch.ActiveConv = len(ch.ConvIDs) - 1
			}
			if conv, err := cw.convManager.LoadConversation(ch.ConvIDs[ch.ActiveConv]); err == nil {
				return conv
			}
		}
		conv, err := cw.convManager.CreateConversation(cw.channelConvTitle(*ch, len(ch.ConvIDs)+1), "", "")
		if err != nil {
			return nil
		}
		ch.ConvIDs = append(ch.ConvIDs, conv.ID)
		ch.ActiveConv = len(ch.ConvIDs) - 1
		ch.ConvID = conv.ID
		_ = cw.convManager.SaveConversation(conv)
		_ = config.SaveConfig(cw.cfg)
		cw.reloadConvs()
		return conv
	})
}

func (cw *ChatWindow) channelCommand(channelID, text string) (string, bool) {
	t := strings.TrimSpace(text)
	if !strings.HasPrefix(t, "/") {
		return "", false
	}
	return onMain(cw, func() string { return cw.execChannelCommand(channelID, t) }), true
}

func (cw *ChatWindow) execChannelCommand(channelID, text string) string {
	fields := strings.Fields(text)
	cmd := strings.ToLower(fields[0])
	ch := cw.channelByID(channelID)
	if ch == nil {
		return "⚠️ 渠道不存在"
	}
	if len(ch.ConvIDs) == 0 && ch.ConvID != "" {
		ch.ConvIDs = []string{ch.ConvID}
	}
	convTitle := func(id string) string {
		if conv, err := cw.convManager.LoadConversation(id); err == nil {
			return conv.Title
		}
		return id
	}
	rest := strings.TrimSpace(strings.TrimPrefix(text, fields[0]))
	switch cmd {
	case "/new", "/新建":
		if len(ch.ConvIDs) >= 20 {
			return "⚠️ 会话数已达上限（20）"
		}
		conv, err := cw.convManager.CreateConversation(cw.channelConvTitle(*ch, len(ch.ConvIDs)+1), "", "")
		if err != nil {
			return "⚠️ " + err.Error()
		}
		ch.ConvIDs = append(ch.ConvIDs, conv.ID)
		ch.ActiveConv = len(ch.ConvIDs) - 1
		ch.ConvID = conv.ID
		_ = config.SaveConfig(cw.cfg)
		cw.reloadConvs()
		return fmt.Sprintf("已新建会话 #%d「%s」并切换", len(ch.ConvIDs), conv.Title)
	case "/list", "/列表":
		if len(ch.ConvIDs) == 0 {
			return "还没有会话，发送 /new 新建"
		}
		var b strings.Builder
		b.WriteString("会话列表：\n")
		for i, id := range ch.ConvIDs {
			mark := "  "
			if i == ch.ActiveConv {
				mark = "→ "
			}
			fmt.Fprintf(&b, "%s#%d %s\n", mark, i+1, convTitle(id))
		}
		return strings.TrimRight(b.String(), "\n")
	case "/switch", "/切换":
		if len(fields) < 2 {
			return "用法：/switch <序号>（序号见 /list）"
		}
		n, err := strconv.Atoi(fields[1])
		if err != nil || n < 1 || n > len(ch.ConvIDs) {
			return "⚠️ 无效序号"
		}
		ch.ActiveConv = n - 1
		ch.ConvID = ch.ConvIDs[n-1]
		_ = config.SaveConfig(cw.cfg)
		cw.reloadConvs()
		return fmt.Sprintf("已切换到会话 #%d「%s」", n, convTitle(ch.ConvIDs[n-1]))
	case "/current", "/当前":
		if ch.ActiveConv >= 0 && ch.ActiveConv < len(ch.ConvIDs) {
			return fmt.Sprintf("当前会话 #%d「%s」", ch.ActiveConv+1, convTitle(ch.ConvIDs[ch.ActiveConv]))
		}
		return "还没有会话，发送 /new 新建"
	case "/agent", "/智能体":
		if rest == "" {
			return cw.channelAgentOptions(ch)
		}
		resolved, errMsg := cw.resolveChannelAgent(rest)
		if errMsg != "" {
			return errMsg
		}
		ch.Agent = resolved
		_ = config.SaveConfig(cw.cfg)
		return "已设置触发 Agent：" + channelAgentLabel(resolved) + "（下一条消息生效）"
	case "/agents", "/agent列表":
		return cw.channelAgentOptions(ch)
	case "/cwd", "/workspace", "/工作目录":
		if rest == "" {
			return cw.channelWorkspaceOptions(ch)
		}
		ch.Workspace = rest
		_ = config.SaveConfig(cw.cfg)
		return "已设置工作目录：" + rest + "（对新建会话生效；ACP 需 /new 后生效）"
	case "/workspaces", "/dirs", "/目录":
		return cw.channelWorkspaceOptions(ch)
	case "/help", "/帮助":
		return "可用命令：\n/new 新建会话\n/list 列出会话\n/switch N 切换会话\n/current 当前会话\n/agent [名称] 查看/设置 Agent\n/agents 列出可用 Agent\n/cwd [路径] 查看/设置工作目录\n/workspaces 列出工作区"
	default:
		return "未知命令。发送 /help 查看可用命令"
	}
}

func (cw *ChatWindow) resolveChannelAgent(arg string) (string, string) {
	a := strings.TrimSpace(arg)
	if a == "" {
		return "", "⚠️ 请提供 Agent 名称"
	}
	if strings.EqualFold(a, "chat") || a == chatAgentName {
		return "", ""
	}
	if name, ok := strings.CutPrefix(a, "agent:"); ok {
		if cw.agentExists(name) {
			return a, ""
		}
		return "", "⚠️ 未找到 Agent：" + name
	}
	if name, ok := strings.CutPrefix(a, "acp:"); ok {
		if _, ok := cw.findACPConn(name); ok {
			return a, ""
		}
		return "", "⚠️ 未找到 ACP 连接：" + name
	}
	if cw.agentExists(a) {
		return "agent:" + a, ""
	}
	if _, ok := cw.findACPConn(a); ok {
		return "acp:" + a, ""
	}
	return "", "⚠️ 未找到 Agent 或 ACP 连接：" + a
}

func (cw *ChatWindow) agentExists(name string) bool {
	for _, a := range cw.cfg.Agents {
		if a.Name == name {
			return true
		}
	}
	return false
}

func channelAgentLabel(agent string) string {
	if name, ok := strings.CutPrefix(agent, "agent:"); ok {
		return "Agent · " + name
	}
	if name, ok := strings.CutPrefix(agent, "acp:"); ok {
		return "ACP · " + name
	}
	return chatAgentName
}

func (cw *ChatWindow) channelAgentOptions(ch *config.Channel) string {
	var b strings.Builder
	b.WriteString("当前 Agent：" + channelAgentLabel(ch.Agent) + "\n")
	b.WriteString("可选 Agent（用法 /agent <名称>）：\n")
	b.WriteString("  chat — 内置 Chat\n")
	for _, a := range cw.cfg.Agents {
		fmt.Fprintf(&b, "  %s — 自定义 Agent\n", a.Name)
	}
	for _, a := range cw.cfg.ACPAgents {
		if a.Disabled {
			continue
		}
		fmt.Fprintf(&b, "  %s — ACP（也可写 acp:%s）\n", a.Name, a.Name)
	}
	return strings.TrimRight(b.String(), "\n")
}

func (cw *ChatWindow) channelWorkspaceOptions(ch *config.Channel) string {
	cur := strings.TrimSpace(ch.Workspace)
	if cur == "" {
		cur = "（未设置，使用默认）"
	}
	var b strings.Builder
	b.WriteString("当前工作目录：" + cur + "\n")
	b.WriteString("已配置工作区（用法 /cwd <路径>）：\n")
	if len(cw.cfg.Workspaces) == 0 {
		b.WriteString("  （无，可直接输入绝对路径）\n")
	}
	for _, w := range cw.cfg.Workspaces {
		fmt.Fprintf(&b, "  %s\n", w)
	}
	return strings.TrimRight(b.String(), "\n")
}

func onMain[T any](cw *ChatWindow, fn func() T) T {
	cw.winMu.Lock()
	w := cw.window
	cw.winMu.Unlock()
	if w == nil {
		return fn()
	}
	ch := make(chan T, 1)
	w.Update(func() { ch <- fn() })
	return <-ch
}

func (cw *ChatWindow) startFeishuRegister() {
	f := &cw.settings.channel
	if f.scanning {
		return
	}
	f.scanning = true
	f.qr = nil
	f.qrTip = "正在获取二维码…"
	cw.invalidate()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				cw.postMain(func() {
					f.scanning = false
					f.qr = nil
					f.qrTip = fmt.Sprintf("创建失败：%v", r)
					cw.invalidate()
				})
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		addons := &registration.AppAddons{
			Scopes: registration.AppAddonsScopes{Tenant: []string{"im:message", "im:message:send_as_bot"}},
			Events: registration.AppAddonsEvents{Items: registration.AppAddonsEventItems{Tenant: []string{"im.message.receive_v1"}}},
		}
		result, err := registration.RegisterApp(ctx, &registration.Options{
			Addons: addons,
			OnQRCode: func(info *registration.QRCodeInfo) {
				url := info.URL
				cw.postMain(func() {
					if bmp := feishuQRBitmap(url); bmp != nil {
						f.qr = bmp
					}
					f.qrTip = "请用飞书 App 扫码授权创建应用（10 分钟内有效）"
					cw.invalidate()
				})
			},
			OnStatusChange: func(info *registration.StatusChangeInfo) {
				cw.postMain(func() {
					if info.Status == registration.StatusSlowDown {
						f.qrTip = "等待授权中…"
						cw.invalidate()
					}
				})
			},
		})
		cw.postMain(func() {
			f.scanning = false
			f.qr = nil
			if err != nil {
				f.qrTip = "创建失败：" + err.Error()
			} else {
				f.appID = result.ClientID
				f.appSecret = result.ClientSecret
				if strings.TrimSpace(f.name) == "" {
					f.name = "Feishu Bot"
				}
				f.qrTip = "应用创建成功，已自动填入 AppID / AppSecret，请点击保存"
			}
			cw.invalidate()
		})
	}()
}

func feishuQRBitmap(url string) *mgo.Bitmap {
	png, err := qrcode.Encode(url, qrcode.Medium, 320)
	if err != nil {
		return nil
	}
	bmp, err := mgo.DecodeBitmap(png)
	if err != nil {
		return nil
	}
	return bmp
}

// feishuLogf appends a diagnostic line to ~/.chatgo/feishu.log.
func (cw *ChatWindow) feishuLogf(format string, args ...any) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(home, ".chatgo", "feishu.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}
