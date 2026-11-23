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
)

func (a *app) settingsView(c *ui.Context) {
	t := c.Theme()
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Padding(0, 0, 24, 0).Gap(16).Children(func() {
			mobile.Section(c, "桌面", func() {
				for i, srv := range a.servers {
					i, srv := i, srv
					a.desktopRow(c, i, srv)
				}
				row := mobile.ListRow(c, mobile.RowOptions{
					Title:      "添加桌面",
					Subtitle:   "扫描桌面二维码或手动输入配对码",
					Disclosure: true,
					Leading: func() {
						ui.Row(c).Size(28, 28).Radius(14).Background(t.Accent.Alpha(0.12)).
							AlignItems(ui.Center).Justify(ui.Center).Children(func() {
							ui.Icon(c, icons.Must("plus")).Size(16, 16).TextColor(t.Accent)
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
				})
				if a.client != nil {
					mobile.ListRow(c, mobile.RowOptions{
						Title:    "中继",
						Subtitle: apiBase(a.currentServer().RelayURL) + "/d/" + a.currentServer().DesktopID,
					})
				}
			})
		})
	})
}

// versionText is the app's own version, as the build reports it.
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
	subtitle := desktopSubtitle(srv)
	row := mobile.ListRow(c, mobile.RowOptions{
		Title:    firstNonEmpty(srv.Name, srv.DesktopID),
		Subtitle: subtitle,
		Trailing: func() {
			if i == a.server {
				ui.Icon(c, icons.Must("check")).Size(16, 16).TextColor(t.Accent)
			} else if len(a.servers) > 1 {
				if kit.Button(c, "移除", kit.ButtonOptions{
					Variant: kit.Ghost, Foreground: t.Danger, Label: "移除桌面",
				}).Height(30).Padding(0, 8).Clicked() {
					a.removeServer(i)
					a.toast("桌面已移除")
				}
			}
		},
	})
	if row.Clicked() {
		// The row may have been the "移除" button inside it, which takes the
		// desktop away: then this click means nothing else.
		if i >= len(a.servers) || a.servers[i].DesktopID != srv.DesktopID {
			return
		}
		if i == a.server {
			return
		}
		a.update(func() {
			a.server = i
			a.connErr = ""
		})
		a.connect(srv)
		a.saveServers()
		a.toast("已切换桌面")
	}
}

// desktopSubtitle names where a desktop runs and how the phone reaches it,
// which is what tells several desktops of the same name apart.
func desktopSubtitle(srv server) string {
	parts := []string{}
	if host := strings.TrimSpace(srv.Host); host != "" {
		parts = append(parts, host)
	}
	if platform := strings.TrimSpace(srv.Platform); platform != "" {
		parts = append(parts, platformLabel(platform))
	}
	if relay := strings.TrimSpace(srv.RelayURL); relay != "" {
		parts = append(parts, apiBase(relay))
	}
	if len(parts) == 0 {
		return srv.DesktopID
	}
	return strings.Join(parts, " · ")
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
	mobile.ListRow(c, mobile.RowOptions{
		Title: "状态",
		Trailing: func() {
			label, color := "未连接", t.TextMuted
			switch {
			case a.connecting:
				label, color = "连接中", t.Accent
			case a.connected && a.eventsConnected:
				label, color = "已连接", t.Success
			case a.connected:
				label, color = "已连接桌面", t.Warning
			}
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
		})
	}
	if a.deviceDisabled {
		mobile.ListRow(c, mobile.RowOptions{
			Title:    "桌面已停用",
			Subtitle: "这台手机被桌面暂停，在桌面设置 → 手机里启用后重新连接",
		})
	}
	if a.pairRequired {
		row := mobile.ListRow(c, mobile.RowOptions{
			Title:    "重新配对",
			Subtitle: "桌面已移除这台手机或令牌失效，需要新的配对码",
		})
		if row.Clicked() {
			a.openPairSheet(true)
		}
	}
	if !a.connecting && (!a.connected || a.pairRequired) {
		row := mobile.ListRow(c, mobile.RowOptions{
			Title: "重新连接",
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
		}).Clicked() {
			a.disconnect()
		}
	}
	row := mobile.ListRow(c, mobile.RowOptions{
		Title:    "通知",
		Subtitle: "自动任务完成、提问和权限审批由系统推送",
	})
	if row.Clicked() {
		a.startPush()
		a.toast("已重新注册推送")
	}
}

func (a *app) pairSheet(c *ui.Context) {
	mobile.Sheet(c, &a.pairOpen, mobile.SheetOptions{Detents: []float32{0.92}, Label: "配对"}, func() {
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
