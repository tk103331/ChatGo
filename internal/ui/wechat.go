package ui

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"chatgo/internal/config"
	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mfeedback "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/feedback"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
	mgo "github.com/egoist/mygo/ui"
)

const (
	wechatBaseURL      = "https://ilinkai.weixin.qq.com"
	wechatQRCodeURL    = "https://ilinkai.weixin.qq.com/ilink/bot/get_bot_qrcode?bot_type=3"
	wechatQRStatusURL  = "https://ilinkai.weixin.qq.com/ilink/bot/get_qrcode_status?qrcode="
	wechatLongPollWait = 35 * time.Second
	wechatSendTimeout  = 15 * time.Second
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
	client := newWechatHTTPClient(baseURL, botToken, botID)
	buf := ""
	cw.setWechatState(feishuConnected, "已连接")

	for {
		select {
		case <-ctx.Done():
			cw.setWechatState(feishuStopped, "未连接")
			return
		default:
		}

		resp, err := client.getUpdates(ctx, buf)
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
			text := extractWechatText(msg)
			if text == "" {
				continue
			}
			cw.notifyChannelMessage(channelID, text)
			fromUser := msg.FromUserID
			contextToken := msg.ContextToken
			cw.runChannelAgent(channelID, text, func(reply string) {
				_ = client.sendText(ctx, fromUser, reply, contextToken)
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
					f.qrTip = fmt.Sprintf("登录失败：%v", r)
					cw.invalidate()
				})
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		qrResp, err := wechatFetchQRCode(ctx)
		if err != nil {
			cw.postMain(func() {
				f.scanning = false
				f.qrTip = "获取二维码失败：" + err.Error()
				cw.invalidate()
			})
			return
		}

		cw.postMain(func() {
			if bmp := feishuQRBitmap(qrResp.QRCode); bmp != nil {
				f.qr = bmp
			}
			f.qrTip = "请用微信扫码登录（10 分钟内有效）"
			cw.invalidate()
		})

		creds, err := wechatPollQRStatus(ctx, qrResp.QRCode, func(status string) {
			cw.postMain(func() {
				switch status {
				case "scaned":
					f.qrTip = "已扫码，等待确认…"
				case "wait":
					f.qrTip = "等待扫码…"
				}
				cw.invalidate()
			})
		})
		cw.postMain(func() {
			f.scanning = false
			f.qr = nil
			if err != nil {
				if ctx.Err() != nil {
					f.qrTip = "登录超时"
				} else {
					f.qrTip = "登录失败：" + err.Error()
				}
			} else {
				f.appID = creds.ILinkBotID
				f.appSecret = creds.BotToken
				f.baseURL = creds.BaseURL
				if strings.TrimSpace(f.name) == "" {
					f.name = "WeChat Bot"
				}
				f.qrTip = "登录成功，已自动填入凭证，请点击保存"
			}
			cw.invalidate()
		})
	}()
}

// --- iLink HTTP client ---

type wechatClient struct {
	baseURL   string
	botToken  string
	botID     string
	wechatUIN string
	http      *http.Client
}

func newWechatHTTPClient(baseURL, botToken, botID string) *wechatClient {
	return &wechatClient{
		baseURL:   baseURL,
		botToken:  botToken,
		botID:     botID,
		wechatUIN: generateWechatUIN(),
		http:      &http.Client{},
	}
}

func (c *wechatClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("AuthorizationType", "ilink_bot_token")
	req.Header.Set("Authorization", "Bearer "+c.botToken)
	req.Header.Set("X-WECHAT-UIN", c.wechatUIN)
}

func (c *wechatClient) getUpdates(ctx context.Context, buf string) (*wechatGetUpdatesResp, error) {
	body := map[string]interface{}{
		"get_updates_buf": buf,
		"base_info":       map[string]string{"channel_version": "1.0.0"},
	}
	pollCtx, cancel := context.WithTimeout(ctx, wechatLongPollWait+5*time.Second)
	defer cancel()

	data, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(pollCtx, "POST", c.baseURL+"/ilink/bot/getupdates", strings.NewReader(string(data)))
	c.setHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result wechatGetUpdatesResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *wechatClient) sendText(ctx context.Context, toUserID, text, contextToken string) error {
	plainText := markdownToPlain(text)
	reqBody := wechatSendMsgReq{
		Msg: wechatSendMsg{
			FromUserID:   c.botID,
			ToUserID:     toUserID,
			ClientID:     newWechatClientID(),
			MessageType:  2,
			MessageState: 2,
			ItemList: []wechatMsgItem{
				{Type: 1, TextItem: &wechatTextItem{Text: plainText}},
			},
			ContextToken: contextToken,
		},
		BaseInfo: wechatBaseInfo{ChannelVersion: "1.0.0"},
	}
	sendCtx, cancel := context.WithTimeout(ctx, wechatSendTimeout)
	defer cancel()

	data, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(sendCtx, "POST", c.baseURL+"/ilink/bot/sendmessage", strings.NewReader(string(data)))
	c.setHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// --- QR login ---

type wechatQRCodeResp struct {
	QRCode           string `json:"qrcode"`
	QRCodeImgContent string `json:"qrcode_img_content"`
}

type wechatQRStatusResp struct {
	Status      string `json:"status"`
	BotToken    string `json:"bot_token"`
	ILinkBotID  string `json:"ilink_bot_id"`
	BaseURL     string `json:"baseurl"`
	ILinkUserID string `json:"ilink_user_id"`
}

type wechatCredentials struct {
	BotToken    string
	ILinkBotID  string
	BaseURL     string
	ILinkUserID string
}

func wechatFetchQRCode(ctx context.Context) (*wechatQRCodeResp, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", wechatQRCodeURL, nil)
	client := &http.Client{Timeout: 40 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result wechatQRCodeResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func wechatPollQRStatus(ctx context.Context, qrcode string, onStatus func(string)) (*wechatCredentials, error) {
	url := wechatQRStatusURL + qrcode
	client := &http.Client{Timeout: 40 * time.Second}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		var result wechatQRStatusResp
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		if onStatus != nil {
			onStatus(result.Status)
		}

		switch result.Status {
		case "confirmed":
			return &wechatCredentials{
				BotToken:    result.BotToken,
				ILinkBotID:  result.ILinkBotID,
				BaseURL:     result.BaseURL,
				ILinkUserID: result.ILinkUserID,
			}, nil
		case "expired":
			return nil, fmt.Errorf("二维码已过期")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// --- iLink types ---

type wechatGetUpdatesResp struct {
	Ret           int               `json:"ret"`
	Msgs          []wechatWeixinMsg `json:"msgs"`
	GetUpdatesBuf string            `json:"get_updates_buf"`
}

type wechatWeixinMsg struct {
	Seq          int             `json:"seq,omitempty"`
	MessageID    int64           `json:"message_id,omitempty"`
	FromUserID   string          `json:"from_user_id"`
	ToUserID     string          `json:"to_user_id"`
	MessageType  int             `json:"message_type"`
	MessageState int             `json:"message_state"`
	ItemList     []wechatMsgItem `json:"item_list"`
	ContextToken string          `json:"context_token"`
}

type wechatMsgItem struct {
	Type     int             `json:"type"`
	TextItem *wechatTextItem `json:"text_item,omitempty"`
}

type wechatTextItem struct {
	Text string `json:"text"`
}

type wechatSendMsgReq struct {
	Msg      wechatSendMsg  `json:"msg"`
	BaseInfo wechatBaseInfo `json:"base_info"`
}

type wechatSendMsg struct {
	FromUserID   string          `json:"from_user_id"`
	ToUserID     string          `json:"to_user_id"`
	ClientID     string          `json:"client_id"`
	MessageType  int             `json:"message_type"`
	MessageState int             `json:"message_state"`
	ItemList     []wechatMsgItem `json:"item_list"`
	ContextToken string          `json:"context_token"`
}

type wechatBaseInfo struct {
	ChannelVersion string `json:"channel_version,omitempty"`
}

func extractWechatText(msg wechatWeixinMsg) string {
	for _, item := range msg.ItemList {
		if item.Type == 1 && item.TextItem != nil {
			return strings.TrimSpace(item.TextItem.Text)
		}
	}
	return ""
}

func generateWechatUIN() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "0000000000"
	}
	n := binary.BigEndian.Uint32(b[:])
	return fmt.Sprintf("%010d", uint64(n)%10000000000)
}

func newWechatClientID() string {
	var b [16]byte
	rand.Read(b[:])
	return fmt.Sprintf("%x", b)
}

func markdownToPlain(s string) string {
	s = strings.ReplaceAll(s, "```", "")
	s = strings.ReplaceAll(s, "`", "")
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "*", "")
	s = strings.ReplaceAll(s, "__", "")
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, "~~", "")
	s = strings.ReplaceAll(s, "#", "")
	return s
}

func (cw *ChatWindow) wechatStatusLine(c *mgo.Context) {
	state, note := cw.wechatStatus()
	st, label := wechatStatusView(state, note)
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
		mgo.Text(c, "微信 ClawBot").Font(mtheme.SerifFont).FontWeight(600).Shrink(0)
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
