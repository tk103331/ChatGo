// Package channel implements ChatGo's IM channel clients (WeChat iLink bot).
package channel

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// BaseURL is the default WeChat iLink API host.
const BaseURL = "https://ilinkai.weixin.qq.com"

const (
	qrCodeURL    = "https://ilinkai.weixin.qq.com/ilink/bot/get_bot_qrcode?bot_type=3"
	qrStatusURL  = "https://ilinkai.weixin.qq.com/ilink/bot/get_qrcode_status?qrcode="
	longPollWait = 35 * time.Second
	sendTimeout  = 15 * time.Second
)

// WeChatClient talks to the WeChat iLink bot API.
type WeChatClient struct {
	baseURL   string
	botToken  string
	botID     string
	wechatUIN string
	http      *http.Client
}

// NewWeChatClient builds a client for a bot token and id.
func NewWeChatClient(baseURL, botToken, botID string) *WeChatClient {
	return &WeChatClient{
		baseURL:   baseURL,
		botToken:  botToken,
		botID:     botID,
		wechatUIN: generateWechatUIN(),
		http:      &http.Client{},
	}
}

func (c *WeChatClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("AuthorizationType", "ilink_bot_token")
	req.Header.Set("Authorization", "Bearer "+c.botToken)
	req.Header.Set("X-WECHAT-UIN", c.wechatUIN)
}

// GetUpdates long-polls for new messages since buf.
func (c *WeChatClient) GetUpdates(ctx context.Context, buf string) (*GetUpdatesResp, error) {
	body := map[string]interface{}{
		"get_updates_buf": buf,
		"base_info":       map[string]string{"channel_version": "1.0.0"},
	}
	pollCtx, cancel := context.WithTimeout(ctx, longPollWait+5*time.Second)
	defer cancel()

	data, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(pollCtx, "POST", c.baseURL+"/ilink/bot/getupdates", strings.NewReader(string(data)))
	c.setHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result GetUpdatesResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SendText sends a plain-text reply, converting Markdown to plain text first.
func (c *WeChatClient) SendText(ctx context.Context, toUserID, text, contextToken string) error {
	plainText := MarkdownToPlain(text)
	reqBody := sendMsgReq{
		Msg: sendMsg{
			FromUserID:   c.botID,
			ToUserID:     toUserID,
			ClientID:     newClientID(),
			MessageType:  2,
			MessageState: 2,
			ItemList: []msgItem{
				{Type: 1, TextItem: &textItem{Text: plainText}},
			},
			ContextToken: contextToken,
		},
		BaseInfo: baseInfo{ChannelVersion: "1.0.0"},
	}
	sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
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

// QRCodeResp is the WeChat login QR code payload.
type QRCodeResp struct {
	QRCode           string `json:"qrcode"`
	QRCodeImgContent string `json:"qrcode_img_content"`
}

type qrStatusResp struct {
	Status      string `json:"status"`
	BotToken    string `json:"bot_token"`
	ILinkBotID  string `json:"ilink_bot_id"`
	BaseURL     string `json:"baseurl"`
	ILinkUserID string `json:"ilink_user_id"`
}

// Credentials are the bot credentials returned by a confirmed QR login.
type Credentials struct {
	BotToken    string
	ILinkBotID  string
	BaseURL     string
	ILinkUserID string
}

// FetchQRCode requests a login QR code.
func FetchQRCode(ctx context.Context) (*QRCodeResp, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", qrCodeURL, nil)
	client := &http.Client{Timeout: 40 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result QRCodeResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// PollQRStatus polls a login QR code until it is confirmed or expires,
// reporting each status to onStatus.
func PollQRStatus(ctx context.Context, qrcode string, onStatus func(string)) (*Credentials, error) {
	url := qrStatusURL + qrcode
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
		var result qrStatusResp
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		if onStatus != nil {
			onStatus(result.Status)
		}

		switch result.Status {
		case "confirmed":
			return &Credentials{
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

// --- iLink wire types ---

// GetUpdatesResp is the long-poll response carrying new messages.
type GetUpdatesResp struct {
	Ret           int         `json:"ret"`
	Msgs          []WeixinMsg `json:"msgs"`
	GetUpdatesBuf string      `json:"get_updates_buf"`
}

// WeixinMsg is one inbound WeChat message.
type WeixinMsg struct {
	Seq          int       `json:"seq,omitempty"`
	MessageID    int64     `json:"message_id,omitempty"`
	FromUserID   string    `json:"from_user_id"`
	ToUserID     string    `json:"to_user_id"`
	MessageType  int       `json:"message_type"`
	MessageState int       `json:"message_state"`
	ItemList     []msgItem `json:"item_list"`
	ContextToken string    `json:"context_token"`
}

type msgItem struct {
	Type     int       `json:"type"`
	TextItem *textItem `json:"text_item,omitempty"`
}

type textItem struct {
	Text string `json:"text"`
}

type sendMsgReq struct {
	Msg      sendMsg  `json:"msg"`
	BaseInfo baseInfo `json:"base_info"`
}

type sendMsg struct {
	FromUserID   string    `json:"from_user_id"`
	ToUserID     string    `json:"to_user_id"`
	ClientID     string    `json:"client_id"`
	MessageType  int       `json:"message_type"`
	MessageState int       `json:"message_state"`
	ItemList     []msgItem `json:"item_list"`
	ContextToken string    `json:"context_token"`
}

type baseInfo struct {
	ChannelVersion string `json:"channel_version,omitempty"`
}

// ExtractText returns the message's text item, trimmed.
func ExtractText(msg WeixinMsg) string {
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

func newClientID() string {
	var b [16]byte
	rand.Read(b[:])
	return fmt.Sprintf("%x", b)
}

// MarkdownToPlain strips Markdown markers for plain-text delivery.
func MarkdownToPlain(s string) string {
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
