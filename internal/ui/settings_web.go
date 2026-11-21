package ui

import (
	"strings"

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
			if kit.SwitchRow(c, &enabled, "启用 Web 服务", kit.SwitchRowOptions{}).Changed() {
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

			if !cw.settings.webToken.ready {
				cw.settings.webToken.ready = true
				cw.settings.webToken.value = cw.cfg.WebToken
			}
			applyToken := func(token, okMessage string) {
				token = strings.TrimSpace(token)
				cw.settings.webToken.value = token
				cw.setWebToken(token)
				if cw.cfg.WebEnabled {
					if next := cw.webInfo(); next.Err != "" {
						cw.toastErrorDetail("Web 服务重启失败", next.Err)
						return
					}
				}
				info = cw.webInfo()
				cw.toast(okMessage, mcore.SeveritySuccess)
			}
			mgo.Text(c, "当前有效令牌").FontWeight(600)
			tokenState := "长期令牌（已保存）"
			if info.TokenEphemeral {
				tokenState = "随机令牌（仅本次运行）"
			}
			mgo.Text(c, "Web 服务始终要求令牌认证，当前为"+tokenState+"。浏览器首次访问需输入令牌；MCP、AG-UI、A2A 客户端使用 Authorization: Bearer <token> 或 X-ChatGo-Token。").
				FontSize(small).TextColor(k.TextMuted)
			currentToken := info.Token
			if currentToken == "" {
				currentToken = "Web 服务启动后自动生成"
			}
			mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, currentToken).Font(mtheme.MonoFont).TextColor(k.Text).
					Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
				if info.Token != "" {
					if kit.Button(c, "复制", kit.ButtonOptions{
						Variant: kit.Ghost, Icon: icons.Must("copy"),
					}).Clicked() {
						c.WriteClipboard(info.Token)
						cw.toast("当前访问令牌已复制", mcore.SeveritySuccess)
					}
				}
			})

			mgo.Text(c, "长期访问令牌").FontWeight(600)
			mgo.Text(c, "保存后长期有效，重启后继续使用；清空保存值后，下次启动会重新随机生成令牌。").
				FontSize(small).TextColor(k.TextMuted)
			tokenParts := kit.InputGroup(c, &cw.settings.webToken.value, kit.InputGroupOptions{
				Placeholder: "留空：每次启动随机生成",
				Label:       "Web 访问令牌",
				Buttons: []kit.InputGroupButton{
					{Label: "保存"},
					{Label: "生成"},
					{Label: "复制", Disabled: info.Token == ""},
					{Label: "清除", Disabled: cw.cfg.WebToken == ""},
				},
			})
			switch tokenParts.Clicked {
			case 0:
				token := strings.TrimSpace(cw.settings.webToken.value)
				if token == "" {
					applyToken("", "已清除长期令牌，下次启动将随机生成")
				} else if strings.ContainsAny(token, " \t\r\n") {
					cw.toast("访问令牌不能包含空白字符", mcore.SeverityWarning)
				} else {
					applyToken(token, "长期访问令牌已保存")
				}
			case 1:
				applyToken(newWebToken(), "已生成并保存长期访问令牌")
			case 2:
				if info.Token == "" {
					cw.toast("当前没有可复制的访问令牌", mcore.SeverityWarning)
				} else {
					c.WriteClipboard(info.Token)
					cw.toast("当前访问令牌已复制", mcore.SeveritySuccess)
				}
			case 3:
				applyToken("", "已清除长期令牌，下次启动将随机生成")
			}
			if cw.settings.webToken.value != cw.cfg.WebToken {
				mgo.Text(c, "有未保存的修改").FontSize(small).TextColor(k.TextMuted)
			}

			mgo.Text(c, "外部 Agent 端点").FontWeight(600)
			mgo.Text(c, "MCP、AG-UI 与 A2A 客户端可直接连接这些地址；服务仅监听本机时外部设备无法访问。").
				FontSize(small).TextColor(k.TextMuted)
			baseURL := info.Address
			if baseURL == "" {
				baseURL = "（Web 未启动）"
			}
			for _, ep := range []struct{ label, path string }{
				{"MCP Streamable HTTP", "/mcp"},
				{"AG-UI", "/agui"},
				{"A2A Agent Card", "/.well-known/agent-card.json"},
				{"A2A JSON-RPC", "/a2a/invoke"},
				{"A2A REST", "/a2a"},
			} {
				ep := ep
				endpoint := ""
				if info.Address != "" {
					endpoint = baseURL + ep.path
				}
				mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
					mgo.Text(c, ep.label).FontSize(small).TextColor(k.TextMuted).Width(150).Shrink(0)
					mgo.Text(c, baseURL+ep.path).Font(mtheme.MonoFont).TextColor(k.Text).
						Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
					if kit.Button(c, "复制", kit.ButtonOptions{
						Variant: kit.Ghost, Icon: icons.Must("copy"), Disabled: endpoint == "",
					}).Clicked() && endpoint != "" {
						c.WriteClipboard(endpoint)
						cw.toast(ep.label+" 地址已复制", mcore.SeveritySuccess)
					}
				})
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
