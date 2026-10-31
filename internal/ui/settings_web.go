package ui

import (
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// webTab shows the local Web service, its address, and the restart action.
func (cw *ChatWindow) webTab(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	info := cw.webInfo()
	mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		mgo.Column(c).Gap(u * 2).MaxWidth(640).Children(func() {
			mgo.Text(c, "Web 服务").FontWeight(600)
			mgo.Text(c, "浏览器访问同一套本地会话；不会上传到远程服务器。可随时关闭，关闭后不再监听端口。").
				FontSize(small).TextColor(k.TextMuted)

			enabled := cw.cfg.WebEnabled
			if mgo.Switch(c, &enabled).Label("启用 Web 服务").Changed() {
				cw.setWebEnabled(enabled)
				info = cw.webInfo()
				if enabled {
					if info.Err != "" {
						cw.toastErrorDetail("Web 服务启用失败", info.Err)
					} else {
						cw.toast("Web 服务已启用", mcore.SeveritySuccess)
					}
				} else {
					cw.toast("Web 服务已禁用", mcore.SeveritySuccess)
				}
			}

			color := k.TextMuted
			switch {
			case info.Running:
				color = k.Success
			case info.Err != "":
				color = k.Danger
			}
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				mgo.Box(c).Size(u*2, u*2).Shrink(0).Radius(u).Background(color)
				mgo.Text(c, info.Label).FontWeight(600)
			})

			if info.Disabled {
				mgo.Text(c, "启用后可在此查看访问地址，并打开或复制给浏览器使用。").
					FontSize(small).TextColor(k.TextMuted)
				return
			}

			mgo.Text(c, "访问地址").FontSize(small).TextColor(k.TextMuted)
			addr := info.Address
			if addr == "" {
				addr = "尚未启动"
			}
			mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, addr).Font(mtheme.MonoFont).TextColor(k.Text).
					Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
				if info.Address != "" {
					if kit.Button(c, "打开", kit.ButtonOptions{
						Variant: kit.Secondary, Icon: icons.Must("external-link"),
					}).Clicked() {
						cw.openWebService()
					}
					if kit.Button(c, "复制", kit.ButtonOptions{
						Variant: kit.Ghost, Icon: icons.Must("copy"),
					}).Clicked() {
						c.WriteClipboard(info.Address)
						cw.toast("Web 地址已复制", mcore.SeveritySuccess)
					}
				}
			})

			if info.Err != "" {
				mgo.Text(c, info.Err).TextColor(k.Danger).FontSize(small).MaxLines(4)
			}

			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				if kit.Button(c, "重启 Web 服务", kit.ButtonOptions{
					Variant: kit.Primary, Icon: icons.Must("refresh-cw"),
				}).Clicked() {
					cw.restartWebService()
					if next := cw.webInfo(); next.Err != "" {
						cw.toastErrorDetail("Web 服务重启失败", next.Err)
					} else {
						cw.toast("Web 服务已重启", mcore.SeveritySuccess)
					}
				}
			})
		})
	})
}
