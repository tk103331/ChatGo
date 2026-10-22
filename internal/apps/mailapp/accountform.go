package mailapp

import (
	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/overlay"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// providerLogo draws a service's logo, falling back to a colored tile.
func providerLogo(c *mgo.Context, p Provider, size float32) {
	if bmp := logoBitmap(p.Logo); bmp != nil {
		mgo.Image(c, bmp).Size(size, size).Shrink(0)
		return
	}
	mgo.Box(c).Width(size).Height(size).Shrink(0).Radius(size / 5).
		Background(mgo.Hex(p.Color)).Center().Children(func() {
		mgo.Text(c, p.Short).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).
			FontWeight(700).TextColor(mgo.Hex("#FFFFFF")).SingleLine()
	})
}

// accountForm shows the add-account dialog: pick a service, then fill in
// the details.
func (cl *Client) accountForm(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	step := st.formStep
	title := "添加邮箱账户"
	desc := "选择服务商"
	width := float32(520)
	if step == 1 {
		title = "填写账户信息"
		desc = ""
		width = 560
	}
	overlay.Dialog(c, &cl.st.adding, overlay.DialogOptions{
		Title:       title,
		Description: desc,
		Width:       width,
		Actions: func() {
			if minput.Button(c, "取消", minput.ButtonOptions{Variant: minput.Ghost}).Clicked() {
				cl.closeAccountForm()
			}
			if step == 1 {
				if minput.Button(c, "上一步", minput.ButtonOptions{Variant: minput.Ghost}).Clicked() {
					cl.backToProviders()
				}
				if minput.Button(c, "保存", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
					cl.saveAccountForm()
				}
			}
		},
	}, func() {
		if step == 0 {
			cl.providerPicker(c, st, k, u, caption)
			return
		}
		cl.accountFields(c, st, k, u, caption)
	})
}

// providerPicker draws the service tiles.
func (cl *Client) providerPicker(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	mgo.Row(c).FillWidth().Gap(u).Wrap().Children(func() {
		for i, p := range providers {
			i, p := i, p
			b := mgo.ButtonBase(c).Column().Key("provider:"+p.Name).
				Width(96).Padding(u).Gap(u/2).AlignItems(mgo.Center).
				Radius(mtheme.ControlRadius)
			if b.Hovered() {
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
		if b.Hovered() {
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
}

// accountFields draws the account form fields.
func (cl *Client) accountFields(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	mgo.Column(c).FillWidth().Gap(u).Children(func() {
		if st.formProvider >= 0 && st.formProvider < len(providers) {
			p := providers[st.formProvider]
			mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
				providerLogo(c, p, 30)
				mgo.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					mgo.Text(c, p.Name).FontSize(caption).FontWeight(600)
					mgo.Text(c, "密码需使用："+p.PasswordHint).FontSize(caption).TextColor(k.TextMuted)
				})
			})
		}
		mgo.Box(c).FillWidth().Height(mtheme.BorderWidth).Background(k.Border)
		field := func(label string, value *string, placeholder string) {
			mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, label).FontSize(caption).TextColor(k.TextMuted).Width(84).Shrink(0)
				minput.TextInput(c, value, minput.TextInputOptions{Placeholder: placeholder}).Element.Grow(1).MinWidth(0)
			})
		}
		field("邮箱地址", &cl.st.formEmail, "you@example.com")
		field("密码", &cl.st.formPass, "密码或授权码")
		field("用户名", &cl.st.formUser, "留空则使用邮箱地址")
		field("IMAP", &cl.st.formIMAP, "imap.example.com:993")
		field("SMTP", &cl.st.formSMTP, "smtp.example.com:465")
	})
}
