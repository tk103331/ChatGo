package ui

import (
	"chatgo/internal/config"
	settingspkg "chatgo/internal/ui/settings"
	"chatgo/internal/ui/theme"

	"github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mfeedback "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/feedback"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	moverlay "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/overlay"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// confirmState is a reusable confirmation dialog.
type confirmState struct {
	open   bool
	active bool
	title  string
	desc   string
	action func()
}

// settingsOptions builds the settings page Options from host state.
func (cw *ChatWindow) settingsOptions() settingspkg.Options {
	return settingspkg.Options{
		Config:            func() *config.Config { return cw.cfg },
		Status:            func() string { return cw.status },
		SetStatus:         func(s string) { cw.status = s },
		Fail:              cw.fail,
		Toast:             cw.toast,
		AskConfirm:        cw.askConfirm,
		SaveConfig:        func() error { return config.SaveConfig(cw.cfg) },
		MarkDirty:         func() { cw.clientDirty.Store(true) },
		PickDirectory:     cw.pickDirectory,
		Invalidate:        cw.invalidate,
		ChatAgentName:     chatAgentName,
		CurrentProvider:   func() string { return cw.provider },
		FeishuStatusView:  func() (mfeedback.Status, string) { return feishuStatusView(cw.feishuStatus()) },
		WechatStatusView:  func() (mfeedback.Status, string) { return wechatStatusView(cw.wechatStatus()) },
		StartWechatLogin:  cw.startWechatLogin,
		StartFeishuRegister: cw.startFeishuRegister,
		RestartChannels:   cw.restartChannels,
		ACPTab:            cw.acpTab,
		ThemeMode:         func() mcore.Mode { return cw.themeMode },
		SetThemeMode:      func(m mcore.Mode) { cw.themeMode = m; cw.saveAppearance() },
		ColorThemeName:    func() string { return cw.colorTheme.Name },
		SetColorTheme:     func(name string) { cw.colorTheme = theme.ByName(name); cw.saveAppearance() },
		DefaultShortcut:   defaultQuickShortcut,
		SaveQuickShortcut: cw.saveQuickShortcut,
		AppVersion:        cw.appVersion,
		SaveSystemSettings: cw.saveSystemSettings,
	}
}

// confirmDialog shows the pending confirmation and runs its action. It keeps
// building for one frame after closing so the dialog releases the focus.
func (cw *ChatWindow) confirmDialog(c *mgo.Context) {
	if !cw.confirm.open && !cw.confirm.active {
		return
	}
	res := moverlay.AlertDialog(c, &cw.confirm.open, moverlay.AlertDialogOptions{
		Title:        cw.confirm.title,
		Description:  cw.confirm.desc,
		ConfirmLabel: "删除",
	})
	cw.confirm.active = cw.confirm.open
	if res.Confirmed && cw.confirm.action != nil {
		cw.confirm.action()
		cw.confirm.action = nil
	}
}

func (cw *ChatWindow) askConfirm(title, desc string, action func()) {
	cw.confirm = confirmState{open: true, active: true, title: title, desc: desc, action: action}
}

func (cw *ChatWindow) askDeleteConversation(id, title string) {
	cw.askConfirm("删除会话", "确定删除「"+title+"」？此操作不可撤销。", func() {
		cw.acpConvClose(id)
		_ = cw.convManager.DeleteConversation(id)
		if cw.cur != nil && cw.cur.ID == id {
			cw.cur = nil
			cw.mode = modeHome
		}
		cw.reloadConvs()
		cw.toast("会话已删除", mcore.SeveritySuccess)
	})
}

func (cw *ChatWindow) fail(msg string) {
	cw.status = msg
	cw.toast(msg, mcore.SeverityError)
}

// feishuStatusView maps a feishu connection state to an indicator status.
func feishuStatusView(state feishuConnState, note string) (mfeedback.Status, string) {
	switch state {
	case feishuConnected:
		return mfeedback.StatusSuccess, "已连接 · " + note
	case feishuConnecting:
		return mfeedback.StatusBusy, "正在连接…"
	case feishuReconnecting:
		return mfeedback.StatusBusy, "重连中…"
	case feishuError:
		return mfeedback.StatusError, "连接失败：" + note
	default:
		return mfeedback.StatusOffline, "未连接"
	}
}

// acpTab configures the ACP connections used by ACP agents.
func (cw *ChatWindow) acpTab(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		mgo.Column(c).FillWidth().Shrink(0).Gap(u * 2).Padding(u).MaxWidth(640).Children(func() {
			mgo.Row(c).FillWidth().Justify(mgo.SpaceBetween).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, "ACP").Font(mtheme.SerifFont).
					FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600)
				mgo.Row(c).Gap(u).Children(func() {
					if minput.Button(c, "从目录添加", minput.ButtonOptions{Variant: minput.Secondary, Icon: icons.Must("layout-grid")}).Clicked() {
						cw.acpCatalogOpen = true
					}
					if minput.Button(c, "新增", minput.ButtonOptions{Variant: minput.Secondary, Icon: icons.Must("plus")}).Clicked() {
						cw.addNewACP()
					}
				})
			})
			if len(cw.cfg.ACPAgents) == 0 {
				mgo.Text(c, "暂无连接，从目录添加或手动新增").TextColor(k.TextMuted).
					FontSize(mcore.FontSize(c, mtheme.CaptionSize))
			}
			for i := range cw.cfg.ACPAgents {
				cw.acpCard(c, i)
			}
		})
	})
	cw.acpCatalogDialog(c)
}

// restartChannels restarts all enabled IM channels after a config change.
func (cw *ChatWindow) restartChannels() {
	cw.stopFeishuChannel()
	cw.stopWechatChannel()
	cw.startFeishuChannel()
	cw.startWechatChannel()
}

// appVersion returns the packaged version, or a dev marker when unbundled.
func (cw *ChatWindow) appVersion() string {
	if v := mygo.App.Version(); v != "" {
		return v
	}
	return "(开发版)"
}

func (cw *ChatWindow) saveSystemSettings() {
	if cw.cfg.RunInBackground {
		cw.cfg.ShowTrayIcon = config.BoolPtr(true)
	}
	_ = config.SaveConfig(cw.cfg)
	cw.syncTray()
	cw.toast("系统设置已保存", mcore.SeveritySuccess)
}
