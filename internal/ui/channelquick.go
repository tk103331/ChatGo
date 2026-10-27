package ui

import (
	"context"

	"chatgo/internal/channel"

	"chatgo/internal/ui/icons"
	kit "chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
	mgo "github.com/egoist/mygo/ui"
)

// channelQuickLogin starts a platform's quick setup: it fetches the code to
// show, renders it, and waits for the platform to hand back credentials. The
// form is filled in place, so the user reviews and saves as usual rather than
// having a credential written behind their back.
func (cw *ChatWindow) channelQuickLogin(typ string) {
	f := &cw.settings.channel
	if f.scanning {
		return
	}
	login, err := cw.channelQuickBegin(typ)
	if err != nil {
		cw.fail(err.Error())
		return
	}
	f.scanning = true
	f.qr = nil
	f.qrTip = "正在获取二维码…"
	f.qrNotice = login.Notice
	cw.invalidate()

	if login.URL != "" {
		if bmp := qrBitmap(login.URL); bmp != nil {
			f.qr = bmp
		}
	}
	f.qrTip = login.Tip
	cw.invalidate()

	go func() {
		creds, err := login.Wait(context.Background())
		cw.postMain(func() {
			f.scanning = false
			f.qr = nil
			switch {
			case err == channel.ErrQRLoginExpired:
				f.qrTip = "二维码已过期，请重新获取"
			case err != nil:
				f.qrTip = "接入失败：" + err.Error()
			default:
				f.appID = creds.AppID
				f.appSecret = creds.AppSecret
				if len(creds.Options) > 0 {
					if f.options == nil {
						f.options = map[string]string{}
					}
					for k, v := range creds.Options {
						f.options[k] = v
					}
				}
				if f.name == "" {
					f.name = channelTypeName(typ)
				}
				f.qrTip = "接入成功，凭证已填入，请点击保存"
			}
			cw.invalidate()
		})
	}()
}

// channelQuickBegin asks a platform for its quick-setup code.
func (cw *ChatWindow) channelQuickBegin(typ string) (*channel.QRLogin, error) {
	driver := channel.NewDriver(typ)
	if driver == nil {
		return nil, errNoQuickSetup(typ)
	}
	loginer, ok := driver.(channel.QRLoginer)
	if !ok {
		return nil, errNoQuickSetup(typ)
	}
	return loginer.BeginQRLogin(context.Background())
}

// errNoQuickSetup explains that a platform has no code-based setup.
func errNoQuickSetup(typ string) error {
	return &quickSetupUnsupported{typ: typ}
}

type quickSetupUnsupported struct{ typ string }

func (e *quickSetupUnsupported) Error() string {
	return channelTypeName(e.typ) + " 不支持扫码接入，请手动填写凭证"
}

// channelQuickButtons draws a platform's quick-setup controls: a scan button
// where the platform hands out a code, or a manifest link where it hands out a
// pre-filled app form.
func (cw *ChatWindow) channelQuickButtons(c *mgo.Context, typ string) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	f := &cw.settings.channel
	caption := mcore.FontSize(c, mtheme.CaptionSize)

	switch typ {
	case "wecom", "dingtalk":
		mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
			label := "扫码创建机器人"
			if typ == "dingtalk" {
				label = "扫码创建应用"
			}
			if kit.Button(c, label, kit.ButtonOptions{
				Variant: kit.Secondary, Icon: icons.Must("globe"), Disabled: f.scanning,
			}).Clicked() {
				cw.channelQuickLogin(typ)
			}
			if f.scanning {
				mgo.Text(c, "等待扫码授权…").TextColor(k.TextMuted).FontSize(caption)
			}
		})
	case "slack":
		// Slack has no code to scan; a manifest link pre-fills the whole app
		// so the user only clicks Create and installs it.
		mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
			if kit.Button(c, "一键创建 Slack 应用", kit.ButtonOptions{
				Variant: kit.Secondary, Icon: icons.Must("globe"),
			}).Clicked() {
				if link, err := channel.SlackManifestURL(); err == nil {
					c.OpenURL(link)
				}
			}
			mgo.Text(c, "打开后点 Create 并安装到工作区，再回来填两个 Token").
				TextColor(k.TextMuted).FontSize(caption)
		})
	}
}

// channelQuickCode draws the code a scan flow returned, under the button that
// asked for it: the user presses the button, then the code appears beneath it.
func (cw *ChatWindow) channelQuickCode(c *mgo.Context) {
	f := &cw.settings.channel
	k := mcore.Tokens(c)
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	if f.qr != nil {
		mgo.Image(c, f.qr).Size(220, 220)
	}
	if f.qrTip != "" {
		mgo.Text(c, f.qrTip).TextColor(k.TextMuted).FontSize(caption)
	}
}

// channelQuickNotice draws the warning a platform's setup requires before the
// user scans, such as DingTalk's page carrying another product's name.
func (cw *ChatWindow) channelQuickNotice(c *mgo.Context) {
	f := &cw.settings.channel
	if f.qrNotice == "" {
		return
	}
	k := mcore.Tokens(c)
	mgo.Text(c, f.qrNotice).TextColor(k.TextMuted).
		FontSize(mcore.FontSize(c, mtheme.CaptionSize))
}
