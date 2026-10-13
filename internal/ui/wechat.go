package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"chatgo/internal/config"
	"github.com/gorilla/websocket"
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

	serverURL := strings.TrimRight(conf.AppID, "/")
	token := conf.AppSecret
	channelID := conf.ID

	if serverURL == "" {
		cw.setWechatState(feishuError, "未配置 OpenClaw 服务器地址")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	cw.wechatMu.Lock()
	cw.wechatRunning = true
	cw.wechatCancel = cancel
	cw.wechatMu.Unlock()
	cw.setWechatState(feishuConnecting, "正在连接 OpenClaw…")

	go func() {
		defer func() {
			cw.wechatMu.Lock()
			cw.wechatRunning = false
			cw.wechatCancel = nil
			cw.wechatMu.Unlock()
		}()
		cw.wechatConnectLoop(ctx, channelID, serverURL, token)
	}()
}

func (cw *ChatWindow) wechatConnectLoop(ctx context.Context, channelID, serverURL, token string) {
	wsURL := wechatWSScheme(serverURL)
	header := http.Header{}
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}

	for {
		select {
		case <-ctx.Done():
			cw.setWechatState(feishuStopped, "未连接")
			return
		default:
		}

		cw.setWechatState(feishuConnecting, "正在连接 OpenClaw…")
		conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, header)
		if err != nil {
			cw.setWechatState(feishuError, "连接失败："+err.Error())
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}

		cw.setWechatState(feishuConnected, "已连接 OpenClaw")

		err = cw.wechatReadLoop(ctx, channelID, serverURL, token, conn)
		conn.Close()

		if ctx.Err() != nil {
			cw.setWechatState(feishuStopped, "未连接")
			return
		}
		cw.setWechatState(feishuReconnecting, "连接断开，重连中…")
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (cw *ChatWindow) wechatReadLoop(ctx context.Context, channelID, serverURL, token string, conn *websocket.Conn) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		var evt wechatEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			continue
		}

		switch evt.Type {
		case "message":
			text := strings.TrimSpace(evt.Content)
			if text == "" {
				continue
			}
			cw.notifyChannelMessage(channelID, text)
			cw.runChannelAgent(channelID, text, func(reply string) {
				_ = wechatSendReply(serverURL, token, evt.FromUser, evt.MsgID, reply)
			})
		case "status":
			note := evt.Note
			if note == "" {
				note = "已连接"
			}
			cw.setWechatState(feishuConnected, note)
		}
	}
}

func wechatSendReply(serverURL, token, toUser, msgID, content string) error {
	body, _ := json.Marshal(map[string]string{
		"to":      toUser,
		"msg_id":  msgID,
		"content": content,
	})
	req, _ := http.NewRequest("POST", serverURL+"/api/wechat/send", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func wechatWSScheme(url string) string {
	if strings.HasPrefix(url, "https://") {
		return "wss://" + strings.TrimPrefix(url, "https://") + "/ws"
	}
	if strings.HasPrefix(url, "http://") {
		return "ws://" + strings.TrimPrefix(url, "http://") + "/ws"
	}
	return "ws://" + url + "/ws"
}

type wechatEvent struct {
	Type     string `json:"type"`
	Content  string `json:"content"`
	FromUser string `json:"from"`
	MsgID    string `json:"msg_id"`
	Note     string `json:"note"`
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

func (cw *ChatWindow) startWechatBind() {
	f := &cw.settings.channel
	if f.scanning {
		return
	}
	serverURL := strings.TrimRight(f.appID, "/")
	if serverURL == "" {
		f.qrTip = "请先填写 OpenClaw 服务器地址"
		cw.invalidate()
		return
	}
	f.scanning = true
	f.qr = nil
	f.qrTip = "正在获取绑定二维码…"
	cw.invalidate()

	go func() {
		req, _ := http.NewRequest("POST", serverURL+"/api/wechat/bind", nil)
		if f.appSecret != "" {
			req.Header.Set("Authorization", "Bearer "+f.appSecret)
		}
		resp, err := http.DefaultClient.Do(req)
		cw.postMain(func() {
			f.scanning = false
			f.qr = nil
			if err != nil {
				f.qrTip = "绑定请求失败：" + err.Error()
			} else {
				defer resp.Body.Close()
				var result struct {
					QRCodeURL string `json:"qrcode_url"`
					Message   string `json:"message"`
				}
				if json.NewDecoder(resp.Body).Decode(&result) == nil && result.QRCodeURL != "" {
					if bmp := feishuQRBitmap(result.QRCodeURL); bmp != nil {
						f.qr = bmp
					}
					f.qrTip = "请用微信扫码授权绑定"
				} else if result.Message != "" {
					f.qrTip = result.Message
				} else {
					f.qrTip = "绑定响应异常"
				}
			}
			cw.invalidate()
		})
	}()
}
