package ui

import (
	"strings"

	"chatgo/internal/ui/assets"
	"chatgo/pkg/models"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
)

const trayRecentMax = 5

func (cw *ChatWindow) shouldShowTray() bool {
	if cw.cfg.RunInBackground {
		return true
	}
	return cw.cfg.ShowTrayIcon == nil || *cw.cfg.ShowTrayIcon
}

func (cw *ChatWindow) syncTray() {
	if cw.shouldShowTray() {
		if cw.tray == nil {
			tray, err := mygo.NewTray(mygo.TrayOptions{
				Icon:           assets.TrayPNG(),
				IconIsTemplate: false,
				ToolTip:        "ChatGo",
			})
			if err != nil {
				cw.status = "托盘图标创建失败: " + err.Error()
			} else {
				cw.tray = tray
				tray.OnClick(func() { cw.openNormal() })
				cw.trayMenuKeyCached = ""
				cw.updateTrayMenu()
			}
		}
	} else if cw.tray != nil {
		cw.tray.Destroy()
		cw.tray = nil
		cw.trayMenuKeyCached = ""
	}
	cw.syncTaskbar()
}

func (cw *ChatWindow) syncTaskbar() {
	cw.winMu.Lock()
	w := cw.window
	cw.winMu.Unlock()
	if w != nil {
		w.SetSkipTaskbar(false)
	}
	mygo.App.Dock.Show()
}

func (cw *ChatWindow) showMainWindow() {
	cw.winMu.Lock()
	w := cw.window
	cw.winMu.Unlock()
	if w == nil || w.IsDestroyed() {
		return
	}
	if w.IsMinimized() {
		w.Restore()
	}
	if !w.IsVisible() {
		w.Show()
	}
	w.Focus()
}

func (cw *ChatWindow) updateTrayMenu() {
	if cw.tray == nil {
		return
	}
	key := cw.trayMenuKey()
	if key == cw.trayMenuKeyCached {
		return
	}
	cw.trayMenuKeyCached = key
	cw.tray.SetMenu(cw.buildTrayMenu())
}

func (cw *ChatWindow) trayMenuKey() string {
	var b strings.Builder
	for _, a := range apps() {
		b.WriteString("app:")
		b.WriteString(a.ID)
		b.WriteByte('|')
		b.WriteString(a.Title)
		b.WriteByte('\n')
	}
	for _, conv := range cw.recentConvs(trayRecentMax) {
		b.WriteString(conv.ID)
		b.WriteByte('|')
		b.WriteString(conv.Title)
		b.WriteByte('|')
		b.WriteString(conv.Workspace)
		b.WriteByte('\n')
	}
	return b.String()
}

func (cw *ChatWindow) buildTrayMenu() *mygo.Menu {
	items := []*mygo.MenuItem{
		{Label: "打开 ChatGo", Click: func(*mygo.MenuItem, *mygo.Window) { cw.showMainWindow() }},
		{Label: "新建会话", Click: func(*mygo.MenuItem, *mygo.Window) { cw.trayNewConversation() }},
		mygo.Separator(),
	}
	if recent := cw.recentConvs(trayRecentMax); len(recent) > 0 {
		sub := make([]*mygo.MenuItem, 0, len(recent))
		for _, conv := range recent {
			conv := conv
			sub = append(sub, &mygo.MenuItem{Label: trayConvLabel(conv), Click: func(*mygo.MenuItem, *mygo.Window) {
				cw.trayOpenConversation(conv.ID)
			}})
		}
		items = append(items, &mygo.MenuItem{Label: "最近会话", Submenu: sub})
	}
	if list := apps(); len(list) > 0 {
		sub := make([]*mygo.MenuItem, 0, len(list))
		for _, a := range list {
			a := a
			sub = append(sub, &mygo.MenuItem{Label: a.Title, Click: func(*mygo.MenuItem, *mygo.Window) {
				cw.openStandaloneApp(a.ID)
			}})
		}
		items = append(items, &mygo.MenuItem{Label: "应用", Submenu: sub})
	}
	items = append(items, &mygo.MenuItem{Label: "检查更新…", Click: func(*mygo.MenuItem, *mygo.Window) {
		updater.CheckForUpdates()
	}})
	items = append(items, mygo.Separator(), &mygo.MenuItem{Label: "退出 ChatGo", Role: mygo.RoleQuit})
	return mygo.NewMenu(items)
}

func trayConvLabel(conv models.Conversation) string {
	label := convTitleLabel(conv.Title)
	if ws := conv.Workspace; ws != "" {
		label += " · " + workspaceTitle(ws)
	}
	r := []rune(label)
	if len(r) > 40 {
		return string(r[:40]) + "…"
	}
	return label
}

func (cw *ChatWindow) recentConvs(n int) []models.Conversation {
	if n <= 0 {
		return nil
	}
	out := make([]models.Conversation, 0, min(n, len(cw.convs)))
	for _, conv := range cw.convs {
		out = append(out, conv)
		if len(out) >= n {
			break
		}
	}
	return out
}

func (cw *ChatWindow) trayNewConversation() {
	cw.postMain(func() {
		if cw.quickMode {
			cw.toggleQuickMode()
		}
		cw.section = sectionChat
		cw.newConversation()
		cw.showMainWindow()
	})
}

func (cw *ChatWindow) trayOpenConversation(id string) {
	cw.postMain(func() {
		cw.selectConversation(id)
		if cw.quickMode {
			cw.toggleQuickMode()
		}
		cw.section = sectionChat
		cw.showMainWindow()
	})
}
