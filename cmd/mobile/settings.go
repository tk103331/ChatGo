package main

import (
	"context"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/mobile"

	"chatgo/internal/mobileapi"
	"chatgo/internal/qrscanner"
	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
)

func (a *app) settingsView(c *ui.Context) {
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Padding(0, 0, 28, 0).Gap(18).Children(func() {
			a.currentDesktopCard(c)

			mobile.Section(c, "桌面", func() {
				for i, srv := range a.servers {
					if i == a.server {
						continue
					}
					i, srv := i, srv
					a.desktopRow(c, i, srv)
				}
				row := mobile.ListRow(c, mobile.RowOptions{
					Title:    "添加桌面",
					Subtitle: "扫码或输入配对码",
					Leading: func() {
						ui.Row(c).Size(28, 28).Radius(9).Background(c.Theme().Accent.Alpha(0.12)).
							AlignItems(ui.Center).Justify(ui.Center).Children(func() {
							ui.Icon(c, icons.Must("plus")).Size(16, 16).TextColor(c.Theme().Accent)
						})
					},
				})
				if row.Clicked() {
					a.openPairSheet(false)
				}
			})

			mobile.Section(c, "连接", func() {
				a.connectionRows(c)
			})

			mobile.Section(c, "关于", func() {
				mobile.ListRow(c, mobile.RowOptions{
					Title:    "ChatGo Mobile",
					Subtitle: versionText(),
					Leading: func() {
						ui.Icon(c, icons.Must("info")).Size(18, 18).TextColor(c.Theme().TextMuted)
					},
				})
				if a.client != nil {
					mobile.ListRow(c, mobile.RowOptions{
						Title:    "中继",
						Subtitle: apiBase(a.currentServer().RelayURL) + "/d/" + a.currentServer().DesktopID,
						Leading: func() {
							ui.Icon(c, icons.Must("globe")).Size(18, 18).TextColor(c.Theme().TextMuted)
						},
					})
				}
			})
		})
	})
}

// currentDesktopCard names the desktop the phone is talking to, its link
// state, and the one action that is useful from here.
func (a *app) currentDesktopCard(c *ui.Context) {
	ui.Row(c).FillWidth().Padding(0, 16).Children(func() {
		a.currentDesktopCardBody(c)
	})
}

func (a *app) currentDesktopCardBody(c *ui.Context) {
	t := c.Theme()
	if len(a.servers) == 0 {
		ui.Column(c).FillWidth().Padding(16).Gap(10).
			Background(t.Surface).Radius(16).Border(1, t.Border).Children(func() {
			ui.Text(c, "还没有桌面").FontSize(16).FontWeight(600)
			ui.Text(c, "配对桌面后，就能在手机上使用会话、群聊和自动任务。").
				FontSize(12).TextColor(t.TextMuted).MaxLines(2)
			if kit.Button(c, "添加桌面", kit.ButtonOptions{Variant: kit.Primary}).
				FillWidth().MinHeight(42).Clicked() {
				a.openPairSheet(false)
			}
		})
		return
	}
	srv := a.currentServer()
	label, color := "未连接", t.TextMuted
	switch {
	case a.connecting:
		label, color = "连接中", t.Accent
	case a.connected && a.eventsConnected:
		label, color = "已连接", t.Success
	case a.connected:
		label, color = "已连接桌面", t.Warning
	}
	ui.Column(c).FillWidth().Padding(16).Gap(12).
		Background(t.Surface).Radius(16).Border(1, t.Border).Children(func() {
		ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Size(42, 42).Radius(13).Background(t.Accent.Alpha(0.12)).
				AlignItems(ui.Center).Justify(ui.Center).Shrink(0).Children(func() {
				ui.Icon(c, icons.Must("columns-2")).Size(22, 22).TextColor(t.Accent)
			})
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, desktopTitle(srv)).FontSize(17).FontWeight(600).
					SingleLine().Ellipsis("…")
				if place := desktopPlace(srv); place != "" {
					ui.Text(c, place).FontSize(12).TextColor(t.TextMuted).
						SingleLine().Ellipsis("…")
				}
				if srv.MAC != "" {
					ui.Text(c, "MAC "+srv.MAC).Font(mtheme.MonoFont).FontSize(11).
						TextColor(t.TextMuted).SingleLine().Ellipsis("…")
				}
			})
			ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
				ui.Box(c).Size(7, 7).Radius(4).Background(color)
				ui.Text(c, label).FontSize(11).TextColor(color)
			})
		})
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.Must("globe")).Size(13, 13).TextColor(t.TextMuted).Shrink(0)
			ui.Text(c, apiBase(srv.RelayURL)+"/d/"+srv.DesktopID).FontSize(11).TextColor(t.TextMuted).
				Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			switch {
			case a.pairRequired:
				if smallGhostButton(c, "重新配对").Clicked() {
					a.openPairSheet(true)
				}
			case !a.connecting && (!a.connected || a.deviceDisabled):
				if smallGhostButton(c, "重连").Clicked() {
					a.connect(srv)
				}
			}
		})
	})
}

// desktopTitle is the machine's own name. A pairing made before desktops
// named themselves still carries the phone's name, which must not label the
// desktop, so the hostname wins whenever the phone knows it.
func desktopTitle(srv server) string {
	if host := strings.TrimSpace(srv.Host); host != "" {
		return host
	}
	name := strings.TrimSpace(srv.Name)
	if name != "" && name != "ChatGo Mobile" && name != strings.TrimSpace(srv.DesktopID) {
		return name
	}
	if id := strings.TrimSpace(srv.DesktopID); id != "" {
		return id
	}
	return "桌面"
}

// desktopPlace is the system and LAN address that say where the desktop
// runs, under its name.
func desktopPlace(srv server) string {
	parts := []string{}
	if platform := strings.TrimSpace(srv.Platform); platform != "" {
		parts = append(parts, platformLabel(platform))
	}
	if ip := strings.TrimSpace(srv.IP); ip != "" {
		parts = append(parts, ip)
	}
	return strings.Join(parts, " · ")
}

func versionText() string {
	version := strings.TrimSpace(mygo.App.Version())
	if version == "" {
		return "通过自建中继连接桌面 ChatGo"
	}
	return "版本 " + version
}

// desktopRow is one paired desktop: its name, where it is reached and which
// one the phone is talking to.
func (a *app) desktopRow(c *ui.Context, i int, srv server) {
	t := c.Theme()
	row := mobile.ListRow(c, mobile.RowOptions{
		Title:    desktopTitle(srv),
		Subtitle: desktopPlace(srv),
		Leading: func() {
			ui.Row(c).Size(30, 30).Radius(10).Background(t.Accent.Alpha(0.08)).
				AlignItems(ui.Center).Justify(ui.Center).Shrink(0).Children(func() {
				ui.Icon(c, icons.Must("columns-2")).Size(17, 17).TextColor(t.Accent)
			})
		},
		Trailing: func() {
			ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
				if i == a.server {
					ui.Icon(c, icons.Must("check")).Size(16, 16).TextColor(t.Accent)
				}
				if smallIconButton(c, "ellipsis", "桌面操作").Clicked() {
					a.openDeskActions(i)
				}
			})
		},
	})
	if row.Clicked() {
		// The ellipsis inside the row opened the actions: the row itself
		// means nothing else then.
		if a.deskActionsOpen || i >= len(a.servers) || a.servers[i].DesktopID != srv.DesktopID {
			return
		}
		if i == a.server {
			return
		}
		a.startDesktopSwitch(i, srv)
	}
}

// openDeskActions shows what can be done with one desktop besides tapping
// it to switch.
func (a *app) openDeskActions(i int) {
	if i < 0 || i >= len(a.servers) {
		return
	}
	a.deskAction = i
	a.deskActionsOpen = true
}

func (a *app) deskActionsSheet(c *ui.Context) {
	if !a.deskActionsOpen {
		return
	}
	if a.deskAction < 0 || a.deskAction >= len(a.servers) {
		a.deskActionsOpen = false
		return
	}
	t := c.Theme()
	srv := a.servers[a.deskAction]
	sheet(c, &a.deskActionsOpen, mobile.SheetOptions{Detents: []float32{0.5}, Label: "桌面操作"}, func() {
		a.sheetBack(c)
		ui.Column(c).FillWidth().Padding(16).Gap(12).Children(func() {
			ui.Column(c).Gap(3).Children(func() {
				ui.Text(c, desktopTitle(srv)).FontSize(17).FontWeight(600)
				if place := desktopPlace(srv); place != "" {
					ui.Text(c, place).FontSize(12).TextColor(t.TextMuted).MaxLines(2)
				}
				if srv.MAC != "" {
					ui.Text(c, "MAC "+srv.MAC).Font(mtheme.MonoFont).FontSize(11).
						TextColor(t.TextMuted).MaxLines(1)
				}
				if srv.IP != "" && srv.IP != srv.Host {
					ui.Text(c, "IP "+srv.IP).Font(mtheme.MonoFont).FontSize(11).
						TextColor(t.TextMuted).MaxLines(1)
				}
			})
			if a.deskAction != a.server {
				if kit.Button(c, "切换到这台桌面", kit.ButtonOptions{Variant: kit.Primary}).
					FillWidth().MinHeight(44).Clicked() {
					i := a.deskAction
					a.deskActionsOpen = false
					a.startDesktopSwitch(i, srv)
				}
			} else if secondaryButton(c, "重新连接").FillWidth().MinHeight(44).Clicked() {
				a.deskActionsOpen = false
				a.connect(srv)
				a.toast("正在重新连接")
			}
			if len(a.servers) > 1 {
				if kit.Button(c, "移除桌面", kit.ButtonOptions{
					Variant: kit.Ghost, Foreground: t.Danger,
				}).FillWidth().MinHeight(42).Clicked() {
					i := a.deskAction
					a.deskActionsOpen = false
					a.removeServer(i)
					a.toast("桌面已移除")
				}
			}
			if ghostButton(c, "取消").FillWidth().Clicked() {
				a.deskActionsOpen = false
			}
		})
	})
}

// platformLabel is the desktop platform as the phone reads it.
func platformLabel(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "darwin", "macos":
		return "macOS"
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	case "android":
		return "Android"
	case "ios":
		return "iOS"
	}
	return platform
}

// connectionRows is what the phone does about the link: its state, and the
// ways out of it.
func (a *app) connectionRows(c *ui.Context) {
	t := c.Theme()
	label, color := "未连接", t.TextMuted
	switch {
	case a.connecting:
		label, color = "连接中", t.Accent
	case a.connected && a.eventsConnected:
		label, color = "已连接", t.Success
	case a.connected:
		label, color = "已连接桌面", t.Warning
	}
	leading := func(icon string, ink ui.Color) {
		ui.Icon(c, icons.Must(icon)).Size(18, 18).TextColor(ink)
	}
	mobile.ListRow(c, mobile.RowOptions{
		Title: "连接状态",
		Leading: func() {
			ui.Box(c).Size(8, 8).Radius(4).Background(color)
		},
		Trailing: func() {
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(7, 7).Radius(4).Background(color).Shrink(0)
				ui.Text(c, label).FontSize(12).TextColor(color)
			})
		},
	})
	if a.connErr != "" {
		mobile.ListRow(c, mobile.RowOptions{
			Title:    "连接错误",
			Subtitle: a.connErr,
			Leading:  func() { leading("triangle-alert", t.Danger) },
		})
	}
	if a.deviceDisabled {
		mobile.ListRow(c, mobile.RowOptions{
			Title:    "桌面已停用",
			Subtitle: "在桌面设置 → 手机里启用后重新连接",
			Leading:  func() { leading("lock", t.Warning) },
		})
	}
	if a.pairRequired {
		row := mobile.ListRow(c, mobile.RowOptions{
			Title:    "重新配对",
			Subtitle: "桌面已移除这台手机或令牌失效，需要新的配对码",
			Leading:  func() { leading("key-round", t.Accent) },
		})
		if row.Clicked() {
			a.openPairSheet(true)
		}
	}
	if !a.connecting && (!a.connected || a.pairRequired) {
		row := mobile.ListRow(c, mobile.RowOptions{
			Title:   "重新连接",
			Leading: func() { leading("refresh-cw", t.Accent) },
		})
		if row.Clicked() && len(a.servers) > 0 {
			a.connect(a.servers[a.server])
			a.toast("正在重新连接")
		}
	}
	// A failed attempt leaves a client behind, but there is no link to cut:
	// only offer the action while a connection exists or is being made.
	if a.connecting || a.connected {
		if mobile.ListRow(c, mobile.RowOptions{
			Title:    "断开连接",
			Subtitle: "保留配对信息，下次可直接重新连接",
			Leading:  func() { leading("wifi-off", t.TextMuted) },
		}).Clicked() {
			a.disconnect()
		}
	}
	row := mobile.ListRow(c, mobile.RowOptions{
		Title:    "通知",
		Subtitle: "自动任务完成、提问和权限审批由系统推送",
		Leading:  func() { leading("message-square", t.TextMuted) },
	})
	if row.Clicked() {
		a.startPush()
		a.toast("已重新注册推送")
	}
}

func (a *app) pairSheet(c *ui.Context) {
	sheet(c, &a.pairOpen, mobile.SheetOptions{Detents: []float32{0.92}, Label: "配对"}, func() {
		a.sheetBack(c)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).FillWidth().Padding(16).Gap(12).Children(func() {
				ui.Text(c, "连接桌面").FontSize(18).FontWeight(600)
				ui.Text(c, "在桌面 ChatGo 设置 → 手机里生成配对二维码，扫描后自动连接。").
					FontSize(13).TextColor(c.Theme().TextMuted).MaxLines(3)
				if kit.Button(c, "扫描桌面二维码", kit.ButtonOptions{
					Variant: kit.Primary, Icon: icons.Must("layout-grid"), Disabled: a.scanning || a.pairing,
				}).FillWidth().MinHeight(44).Clicked() {
					a.scanPairCode()
				}
				ui.Text(c, "也可以手动输入").FontSize(12).TextColor(c.Theme().TextMuted)
				ui.TextInput(c, &a.pairRelay).Placeholder("中继地址，如 wss://relay.example.com")
				ui.TextInput(c, &a.pairDesktop).Placeholder("桌面 ID，如 d_abcdef…")
				ui.TextInput(c, &a.pairCode).Placeholder("8 位配对码")
				ui.TextInput(c, &a.pairName).Placeholder("设备名称（可选）")
				if a.pairErr != "" {
					ui.Text(c, a.pairErr).FontSize(12).TextColor(c.Theme().Danger).MaxLines(4)
				}
				ui.Row(c).Gap(8).Children(func() {
					if a.pairing {
						ui.Text(c, "配对中…").FontSize(13).TextColor(c.Theme().TextMuted)
					} else if primaryButton(c, "配对").Clicked() {
						a.pair()
					}
					if ghostButton(c, "取消").Clicked() {
						a.pairOpen = false
					}
				})
			})
		})
	})
}

func (a *app) pair() {
	if a.pairing {
		return
	}
	relay := apiBase(a.pairRelay)
	desktop := strings.TrimSpace(a.pairDesktop)
	code := strings.ToUpper(strings.TrimSpace(a.pairCode))
	if relay == "" || desktop == "" || code == "" {
		a.update(func() { a.pairErr = "请填写中继地址、桌面 ID 和配对码" })
		return
	}
	name := strings.TrimSpace(a.pairName)
	if name == "" {
		name = "ChatGo Mobile"
	}
	a.update(func() {
		a.pairing = true
		a.pairErr = ""
	})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		res, err := mobileapi.Pair(ctx, relay, desktop, code, name, platformName())
		if err != nil {
			a.update(func() {
				a.pairing = false
				a.pairErr = err.Error()
			})
			return
		}
		srv := server{
			ID:        res.DesktopID,
			Name:      firstNonEmpty(res.DesktopName, res.DesktopID),
			Host:      res.DesktopName,
			Platform:  res.Platform,
			MAC:       res.MAC,
			IP:        res.IP,
			RelayURL:  relay,
			DesktopID: res.DesktopID,
		}
		saveToken(res.DesktopID, res.Token)
		if len(res.E2EKey) > 0 {
			saveE2EKey(res.DesktopID, res.E2EKey)
		}
		srv.Token = res.Token
		srv.DeviceID = res.DeviceID
		srv.E2EKey = res.E2EKey
		a.update(func() {
			a.pairing = false
			a.pairOpen = false
			a.pairErr = ""
			a.pairRequired = false
			a.deviceDisabled = false
			a.addServer(srv)
			a.tab = tabChat
			a.screen = screenNew
			a.connect(srv)
			a.startPush()
		})
	}()
}

func (a *app) scanPairCode() {
	if a.scanning || a.pairing {
		return
	}
	a.scanning, a.pairErr = true, ""
	go func() {
		text, err := qrscanner.Scan(context.Background())
		a.update(func() {
			a.scanning = false
			if err != nil {
				a.pairErr = "无法扫码：" + err.Error()
				return
			}
			if strings.TrimSpace(text) == "" {
				return
			}
			link, err := parsePairQR(text)
			if err != nil {
				a.pairErr = err.Error()
				return
			}
			a.pairRelay, a.pairDesktop, a.pairCode = link.relay, link.desktop, link.code
			a.pair()
		})
	}()
}

func platformName() string {
	if name := mygo.App.Name(); name != "" {
		return name
	}
	return "ChatGo Mobile"
}
