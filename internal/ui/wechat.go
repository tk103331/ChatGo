package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"chatgo/internal/channel"
	"chatgo/internal/config"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mfeedback "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/feedback"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
	mgo "github.com/egoist/mygo/ui"
)

const (
	wechatBaseURL = channel.BaseURL
)

func (cw *ChatWindow) setWechatState(state feishuConnState, note string) {
	cw.wechatMu.Lock()
	cw.wechatState = state
	cw.wechatNote = note
	cw.wechatMu.Unlock()
	cw.postMain(func() { cw.invalidate() })
}

func (cw *ChatWindow) wechatStatus() (feishuConnState, string) {
	cw.wechatMu.Lock()
	defer cw.wechatMu.Unlock()
	return cw.wechatState, cw.wechatNote
}

func (cw *ChatWindow) startWechatChannel() {
	cw.wechatMu.Lock()
	if cw.wechatRunning {
		cw.wechatMu.Unlock()
		return
	}
	cw.wechatMu.Unlock()

	var conf *config.Channel
	for i := range cw.cfg.Channels {
		if cw.cfg.Channels[i].Type == "wechat" && cw.cfg.Channels[i].Enabled {
			conf = &cw.cfg.Channels[i]
			break
		}
	}
	if conf == nil {
		return
	}

	botToken := conf.AppSecret
	botID := conf.AppID
	if botToken == "" || botID == "" {
		cw.setWechatState(feishuError, "未登录，请扫码绑定微信")
		return
	}

	channelID := conf.ID
	baseURL := conf.BaseURL
	if baseURL == "" {
		baseURL = wechatBaseURL
	}

	ctx, cancel := context.WithCancel(context.Background())
	cw.wechatMu.Lock()
	cw.wechatRunning = true
	cw.wechatCancel = cancel
	cw.wechatMu.Unlock()
	cw.setWechatState(feishuConnecting, "正在连接…")

	go func() {
		defer func() {
			cw.wechatMu.Lock()
			cw.wechatRunning = false
			cw.wechatCancel = nil
			cw.wechatMu.Unlock()
		}()
		cw.wechatPollLoop(ctx, channelID, baseURL, botToken, botID)
	}()
}

func (cw *ChatWindow) wechatPollLoop(ctx context.Context, channelID, baseURL, botToken, botID string) {
	client := channel.NewWeChatClient(baseURL, botToken, botID)
	buf := ""
	cw.setWechatState(feishuConnected, "已连接")

	for {
		select {
		case <-ctx.Done():
			cw.setWechatState(feishuStopped, "未连接")
			return
		default:
		}

		resp, err := client.GetUpdates(ctx, buf)
		if err != nil {
			if ctx.Err() != nil {
				cw.setWechatState(feishuStopped, "未连接")
				return
			}
			cw.setWechatState(feishuReconnecting, "连接中断，重连中…")
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
			continue
		}
		cw.setWechatState(feishuConnected, "已连接")

		if resp.GetUpdatesBuf != "" {
			buf = resp.GetUpdatesBuf
		}

		for _, msg := range resp.Msgs {
			if msg.MessageType != 1 || msg.MessageState != 2 {
				continue
			}
			text := channel.ExtractText(msg)
			if text == "" {
				continue
			}
			cw.notifyChannelMessage(channelID, text)
			fromUser := msg.FromUserID
			contextToken := msg.ContextToken
			cw.runChannelAgent(channelID, text, func(reply string) {
				_ = client.SendText(ctx, fromUser, reply, contextToken)
			})
		}
	}
}

func (cw *ChatWindow) stopWechatChannel() {
	cw.wechatMu.Lock()
	cancel := cw.wechatCancel
	cw.wechatRunning = false
	cw.wechatCancel = nil
	cw.wechatMu.Unlock()
	if cancel != nil {
		cancel()
	}
	cw.setWechatState(feishuStopped, "未连接")
}

func (cw *ChatWindow) startWechatLogin() {
	f := &cw.settings.Channel
	if f.Scanning {
		return
	}
	f.Scanning = true
	f.QR = nil
	f.QRTip = "正在获取二维码…"
	cw.invalidate()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				cw.postMain(func() {
					f.Scanning = false
					f.QR = nil
					f.QRTip = fmt.Sprintf("登录失败：%v", r)
					cw.invalidate()
				})
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		qrResp, err := channel.FetchQRCode(ctx)
		if err != nil {
			cw.postMain(func() {
				f.Scanning = false
				f.QRTip = "获取二维码失败：" + err.Error()
				cw.invalidate()
			})
			return
		}

		cw.postMain(func() {
			if bmp := feishuQRBitmap(qrResp.QRCodeImgContent); bmp != nil {
				f.QR = bmp
			}
			f.QRTip = "请用微信扫码登录（10 分钟内有效）"
			cw.invalidate()
		})

		creds, err := channel.PollQRStatus(ctx, qrResp.QRCode, func(status string) {
			cw.postMain(func() {
				switch status {
				case "scaned":
					f.QRTip = "已扫码，等待确认…"
				case "wait":
					f.QRTip = "等待扫码…"
				}
				cw.invalidate()
			})
		})
		cw.postMain(func() {
			f.Scanning = false
			f.QR = nil
			if err != nil {
				if ctx.Err() != nil {
					f.QRTip = "登录超时"
				} else {
					f.QRTip = "登录失败：" + err.Error()
				}
			} else {
				f.AppID = creds.ILinkBotID
				f.AppSecret = creds.BotToken
				f.BaseURL = creds.BaseURL
				if strings.TrimSpace(f.Name) == "" {
					f.Name = "WeChat Bot"
				}
				f.QRTip = "登录成功，已自动填入凭证，请点击保存"
			}
			cw.invalidate()
		})
	}()
}

// --- iLink HTTP client ---

func (cw *ChatWindow) wechatStatusLine(c *mgo.Context) {
	state, note := cw.wechatStatus()
	st, label := wechatStatusView(state, note)
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
		mgo.Text(c, "微信").Font(mtheme.SerifFont).FontWeight(600).Shrink(0)
		mfeedback.StatusIndicator(c, st, mfeedback.StatusIndicatorOptions{})
		mgo.Text(c, label).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
	})
}

func wechatStatusView(state feishuConnState, note string) (mfeedback.Status, string) {
	switch state {
	case feishuConnected:
		return mfeedback.StatusSuccess, "已连接 · " + note
	case feishuConnecting:
		return mfeedback.StatusBusy, "正在连接…"
	case feishuReconnecting:
		return mfeedback.StatusBusy, "重连中…"
	case feishuError:
		return mfeedback.StatusError, "连接失败：" + note
	default:
		return mfeedback.StatusOffline, "未连接"
	}
}
