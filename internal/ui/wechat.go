package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"chatgo/internal/channel"
)

const (
	wechatBaseURL = channel.BaseURL
)

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

		qrResp, err := channel.FetchQRCode(ctx)
		if err != nil {
			cw.postMain(func() {
				f.scanning = false
				f.qrTip = "获取二维码失败：" + err.Error()
				cw.invalidate()
			})
			return
		}

		cw.postMain(func() {
			if bmp := qrBitmap(qrResp.QRCodeImgContent); bmp != nil {
				f.qr = bmp
			}
			f.qrTip = "请用微信扫码登录（10 分钟内有效）"
			cw.invalidate()
		})

		creds, err := channel.PollQRStatus(ctx, qrResp.QRCode, func(status string) {
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
