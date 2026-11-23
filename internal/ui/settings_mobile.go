package ui

import (
	"net/url"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/relayclient"
	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// mobileTab controls the self-hosted relay and the phones paired with it.
func (cw *ChatWindow) mobileTab(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	info := cw.relayInfo()

	mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		mgo.Column(c).Gap(u * 2).MaxWidth(680).Children(func() {
			mgo.Text(c, "手机连接").FontWeight(600)
			mgo.Text(c, "手机通过自建中继使用桌面上的会话、群聊和自动任务。桌面主动外连中继，不需要公网 IP 或端口映射。").
				FontSize(small).TextColor(k.TextMuted)

			enabled := cw.cfg.Relay.Enabled
			if kit.SwitchRow(c, &enabled, "启用中继", kit.SwitchRowOptions{}).Changed() {
				cw.setRelayEnabled(enabled)
				info = cw.relayInfo()
				if enabled && info.Err != "" {
					cw.toastErrorDetail("中继启用失败", info.Err)
				} else if enabled {
					cw.toast("中继已启用", mcore.SeveritySuccess)
				} else {
					cw.toast("中继已停用", mcore.SeveritySuccess)
				}
			}
			if !cw.cfg.Relay.Enabled {
				mgo.Text(c, "启用后填写中继地址，生成配对码，再用手机 App 扫码或手动输入。").
					FontSize(small).TextColor(k.TextMuted)
				return
			}

			if !cw.settings.relayURL.ready {
				cw.settings.relayURL.ready = true
				cw.settings.relayURL.value = cw.cfg.Relay.URL
			}
			mgo.Text(c, "中继地址").FontWeight(600)
			mgo.Text(c, "例如 wss://relay.example.com。生产环境必须使用 TLS；本地开发可用 ws://127.0.0.1:8787。").
				FontSize(small).TextColor(k.TextMuted)
			parts := kit.InputGroup(c, &cw.settings.relayURL.value, kit.InputGroupOptions{
				Placeholder: "wss://relay.example.com",
				Label:       "中继地址",
				Buttons: []kit.InputGroupButton{
					{Label: "保存并重连"},
				},
			})
			if parts.Clicked == 0 {
				raw := strings.TrimSpace(cw.settings.relayURL.value)
				if raw != "" && !strings.HasPrefix(raw, "ws://") && !strings.HasPrefix(raw, "wss://") {
					cw.toast("中继地址必须以 ws:// 或 wss:// 开头", mcore.SeverityWarning)
				} else {
					cw.setRelayURL(raw)
					cw.settings.relayURL.value = raw
					cw.toast("中继地址已保存", mcore.SeveritySuccess)
				}
			}
			if cw.settings.relayURL.value != cw.cfg.Relay.URL {
				mgo.Text(c, "有未保存的修改").FontSize(small).TextColor(k.TextMuted)
			}

			color := k.TextMuted
			label := "未连接"
			switch info.State {
			case relayclient.StateConnected:
				color, label = k.Success, "已连接"
			case relayclient.StateConnecting:
				color, label = k.Warning, "连接中"
			}
			if info.Err != "" {
				color, label = k.Danger, "连接失败"
			}
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				mgo.Box(c).Size(u*2, u*2).Shrink(0).Radius(u).Background(color)
				mgo.Text(c, label).FontWeight(600)
				if info.URL != "" {
					mgo.Text(c, info.URL).Font(mtheme.MonoFont).FontSize(small).TextColor(k.TextMuted).
						SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
				}
			})
			if info.Err != "" {
				mgo.Text(c, info.Err).FontSize(small).TextColor(k.Danger).MaxLines(4)
			}
			if info.DesktopID != "" {
				mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
					mgo.Text(c, "桌面 ID").FontSize(small).TextColor(k.TextMuted).Width(90).Shrink(0)
					mgo.Text(c, info.DesktopID).Font(mtheme.MonoFont).FontSize(small).TextColor(k.Text).
						Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
					if kit.Button(c, "复制", kit.ButtonOptions{Variant: kit.Ghost, Icon: icons.Must("copy")}).Clicked() {
						c.WriteClipboard(info.DesktopID)
						cw.toast("桌面 ID 已复制", mcore.SeveritySuccess)
					}
				})
			}

			allow := cw.cfg.Relay.AllowPairing
			if kit.SwitchRow(c, &allow, "允许新设备配对", kit.SwitchRowOptions{}).Changed() {
				cw.setRelayAllowPairing(allow)
				info = cw.relayInfo()
			}

			mgo.Text(c, "配对手机").FontWeight(600)
			mgo.Text(c, "生成 8 位配对码（2 分钟有效）。在手机 App 中扫码或输入配对码。").
				FontSize(small).TextColor(k.TextMuted)
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				if kit.Button(c, "生成配对码", kit.ButtonOptions{
					Variant: kit.Primary, Icon: icons.Must("globe"),
				}).Clicked() {
					cw.newRelayPairCode()
					info = cw.relayInfo()
				}
				if kit.Button(c, "重连中继", kit.ButtonOptions{
					Variant: kit.Secondary, Icon: icons.Must("refresh-cw"),
				}).Clicked() {
					cw.restartRelayService()
					info = cw.relayInfo()
				}
			})

			if info.PairCode != "" {
				link := cw.relayPairLink(info)
				mgo.Row(c).FillWidth().Gap(u * 2).AlignItems(mgo.Center).Children(func() {
					if bmp := qrBitmap(link); bmp != nil {
						mgo.Image(c, bmp).Size(180, 180).Shrink(0)
					}
					mgo.Column(c).Grow(1).MinWidth(0).Gap(u).Children(func() {
						mgo.Text(c, "配对码").FontSize(small).TextColor(k.TextMuted)
						mgo.Text(c, info.PairCode).Font(mtheme.MonoFont).FontSize(mcore.FontSize(c, mtheme.H2Size)).
							FontWeight(600).TextColor(k.Text)
						if info.PairExpiry != "" {
							mgo.Text(c, "有效至 "+info.PairExpiry).FontSize(small).TextColor(k.TextMuted)
						}
						mgo.Text(c, "手机扫码后会自动填入中继地址和桌面 ID；也可以手动输入配对码。").
							FontSize(small).TextColor(k.TextMuted).MaxLines(3)
						mgo.Row(c).Gap(8).Wrap().Children(func() {
							if kit.Button(c, "复制配对码", kit.ButtonOptions{
								Variant: kit.Secondary, Icon: icons.Must("copy"),
							}).Clicked() {
								c.WriteClipboard(info.PairCode)
								cw.toast("配对码已复制", mcore.SeveritySuccess)
							}
							if kit.Button(c, "复制配对链接", kit.ButtonOptions{
								Variant: kit.Ghost, Icon: icons.Must("copy"),
							}).Clicked() {
								c.WriteClipboard(link)
								cw.toast("配对链接已复制", mcore.SeveritySuccess)
							}
						})
					})
				})
			}

			mgo.Text(c, "已配对设备").FontWeight(600)
			if len(info.Devices) == 0 {
				mgo.Text(c, "还没有设备。生成配对码后用手机 App 配对。").
					FontSize(small).TextColor(k.TextMuted)
			} else {
				for _, d := range info.Devices {
					device := d
					mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).
						Padding(u).Radius(mtheme.ControlRadius).Background(k.Surface).Children(func() {
						mgo.Column(c).Grow(1).MinWidth(0).Gap(u / 3).Children(func() {
							mgo.Text(c, firstNonEmptyString(device.Name, device.ID)).FontWeight(600)
							detail := strings.TrimSpace(device.Platform)
							if !device.CreatedAt.IsZero() {
								if detail != "" {
									detail += " · "
								}
								detail += "配对于 " + device.CreatedAt.Format("01-02 15:04")
							}
							if detail != "" {
								mgo.Text(c, detail).FontSize(small).TextColor(k.TextMuted)
							}
							if device.Disabled {
								mgo.Text(c, "已停用，这台手机无法连接").FontSize(small).TextColor(k.Warning)
							}
						})
						mgo.Row(c).Gap(u / 2).Shrink(0).Children(func() {
							label := "停用"
							if device.Disabled {
								label = "启用"
							}
							if kit.Button(c, label, kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
								cw.setRelayDeviceDisabled(device.ID, !device.Disabled)
							}
							if kit.Button(c, "吊销", kit.ButtonOptions{
								Variant: kit.Ghost, Icon: icons.Must("x"),
							}).Clicked() {
								cw.revokeRelayDevice(device.ID)
							}
						})
					})
				}
			}

			mgo.Text(c, "说明").FontWeight(600)
			mgo.Text(c, "中继只转发 /api 与事件 WebSocket，不保存会话内容；桌面 Web Token 不会离开本机。手机端使用独立设备令牌，可随时吊销。").
				FontSize(small).TextColor(k.TextMuted)
		})
	})
}

func (cw *ChatWindow) relayPairLink(info relayInfo) string {
	query := url.Values{}
	query.Set("v", "1")
	query.Set("r", cw.cfg.Relay.URL)
	query.Set("d", info.DesktopID)
	query.Set("c", info.PairCode)
	return "chatgo://pair?" + query.Encode()
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
