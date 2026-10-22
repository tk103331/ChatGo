package mailapp

import (
	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/overlay"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// providerLogo draws a service's brand tile.
func providerLogo(c *mgo.Context, p Provider, size float32) {
	mgo.Box(c).Width(size).Height(size).Shrink(0).Radius(size / 5).
		Background(mgo.Hex(p.Color)).Center().Children(func() {
		mgo.Text(c, p.Short).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).
			FontWeight(700).TextColor(mgo.Hex("#FFFFFF")).SingleLine()
	})
}

// accountForm shows the add-account dialog.
func (cl *Client) accountForm(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	overlay.Dialog(c, &cl.st.adding, overlay.DialogOptions{
		Title: "添加邮箱账户",
		Description: "选择服务商自动填入服务器地址",
		Width: 560,
		Actions: func() {
			if minput.Button(c, "取消", minput.ButtonOptions{Variant: minput.Ghost}).Clicked() {
				cl.closeAccountForm()
			}
			if minput.Button(c, "保存", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
				cl.saveAccountForm()
			}
		},
	}, func() {
		mgo.Column(c).FillWidth().Gap(u).Children(func() {
			mgo.Row(c).FillWidth().Gap(u/2).Wrap().Children(func() {
				for i, p := range providers {
					i, p := i, p
					sel := i == st.formProvider
					b := mgo.ButtonBase(c).Column().Key("provider:"+p.Name).
						Width(72).Padding(u*3/4).Gap(2).AlignItems(mgo.Center).
						Radius(mtheme.ControlRadius)
					if sel {
						b.Background(k.Selection)
					} else if b.Hovered() {
						b.Background(k.SurfaceHover)
					}
					b.Children(func() {
						providerLogo(c, p, 20)
						mgo.Text(c, p.Name).FontSize(caption).TextColor(k.Text).
							SingleLine().Ellipsis("…")
					})
					if b.Clicked() {
						cl.applyProvider(i)
					}
				}
				b := mgo.ButtonBase(c).Column().Key("provider:custom").
					Width(72).Padding(u*3/4).Gap(2).AlignItems(mgo.Center).
					Radius(mtheme.ControlRadius)
				if st.formProvider < 0 {
					b.Background(k.Selection)
				} else if b.Hovered() {
					b.Background(k.SurfaceHover)
				}
				b.Children(func() {
					mgo.Box(c).Width(20).Height(20).Shrink(0).Radius(4).
						Border(mtheme.BorderWidth, k.ControlBorder).Center().Children(func() {
						mgo.Text(c, "…").FontSize(caption).TextColor(k.TextMuted)
					})
					mgo.Text(c, "自定义").FontSize(caption).TextColor(k.TextMuted).SingleLine()
				})
				if b.Clicked() {
					cl.applyProvider(-1)
				}
			})
			mgo.Box(c).FillWidth().Height(mtheme.BorderWidth).Background(k.Border)
			field := func(label string, value *string, placeholder string) {
				mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
					mgo.Text(c, label).FontSize(caption).TextColor(k.TextMuted).Width(84).Shrink(0)
					minput.TextInput(c, value, minput.TextInputOptions{Placeholder: placeholder}).Element.Grow(1).MinWidth(0)
				})
			}
			field("邮箱地址", &cl.st.formEmail, "you@example.com")
			hint := "密码"
			if st.formProvider >= 0 && st.formProvider < len(providers) {
				hint = "密码/" + providers[st.formProvider].PasswordHint
			}
			field(hint, &cl.st.formPass, "密码或授权码")
			field("用户名", &cl.st.formUser, "留空则使用邮箱地址")
			field("IMAP", &cl.st.formIMAP, "imap.example.com:993")
			field("SMTP", &cl.st.formSMTP, "smtp.example.com:465")
		})
	})
}
