package mailapp

import (
	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
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

// accountForm draws the add-account form with service presets.
func (cl *Client) accountForm(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	mgo.Scroll(c).Fill().Children(func() {
		mgo.Column(c).FillWidth().Padding(u*3).Gap(u*2).MaxWidth(720).Children(func() {
			mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, "添加邮箱账户").FontSize(mcore.FontSize(c, mtheme.BodySize)).FontWeight(600).Grow(1)
				if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("x"), Label: "取消"}).Clicked() {
					cl.closeAccountForm()
				}
				if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Primary, Icon: icons.Must("check"), Label: "保存"}).Clicked() {
					cl.saveAccountForm()
				}
			})
			mgo.Text(c, "选择服务商，自动填入服务器地址").FontSize(caption).TextColor(k.TextMuted)
			mgo.Row(c).FillWidth().Gap(u).Wrap().Children(func() {
				for i, p := range providers {
					i, p := i, p
					sel := i == st.formProvider
					b := mgo.ButtonBase(c).Column().Key("provider:"+p.Name).
						Width(96).Padding(u).Gap(u/2).AlignItems(mgo.Center).
						Radius(mtheme.ControlRadius)
					if sel {
						b.Background(k.Selection)
					} else if b.Hovered() {
						b.Background(k.SurfaceHover)
					}
					b.Children(func() {
						providerLogo(c, p, 30)
						mgo.Text(c, p.Name).FontSize(caption).TextColor(k.Text).
							SingleLine().Ellipsis("…")
					})
					if b.Clicked() {
						cl.applyProvider(i)
					}
				}
				b := mgo.ButtonBase(c).Column().Key("provider:custom").
					Width(96).Padding(u).Gap(u/2).AlignItems(mgo.Center).
					Radius(mtheme.ControlRadius)
				if st.formProvider < 0 {
					b.Background(k.Selection)
				} else if b.Hovered() {
					b.Background(k.SurfaceHover)
				}
				b.Children(func() {
					mgo.Box(c).Width(30).Height(30).Shrink(0).Radius(6).
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
					mgo.Text(c, label).FontSize(caption).TextColor(k.TextMuted).Width(96).Shrink(0)
					minput.TextInput(c, value, minput.TextInputOptions{Placeholder: placeholder}).Element.Grow(1).MinWidth(0)
				})
			}
			field("邮箱地址", &cl.st.formEmail, "you@example.com")
			hint := "密码"
			if st.formProvider >= 0 && st.formProvider < len(providers) {
				hint = "密码（" + providers[st.formProvider].PasswordHint + "）"
			}
			field(hint, &cl.st.formPass, "密码或授权码")
			field("用户名", &cl.st.formUser, "留空则使用邮箱地址")
			field("IMAP 服务器", &cl.st.formIMAP, "imap.example.com:993")
			field("SMTP 服务器", &cl.st.formSMTP, "smtp.example.com:465")
			if st.formProvider < 0 {
				mgo.Text(c, "自定义服务商需要手动填写 IMAP 与 SMTP 地址").
					FontSize(caption).TextColor(k.TextMuted)
			}
		})
	})
}
