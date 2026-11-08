package ui

import (
	"runtime"
	"strconv"

	"chatgo/internal/capture"
	"chatgo/internal/config"

	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"

	mgo "github.com/egoist/mygo/ui"
)

// quickPanelSection configures the system-wide quick task panel.
func (cw *ChatWindow) quickPanelSection(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	qp := &cw.cfg.QuickPanel
	qp.Ensure()
	mgo.Column(c).Gap(u * 2).Children(func() {
		mgo.Text(c, "快捷任务面板").FontWeight(600)
		mgo.Text(c, "在任意应用里按 "+cw.quickPanelShortcut()+
			" 打开浮窗：自动带入选中的文本、剪贴板里的目录或当前工作区，选一个 Agent 就能直接执行。").
			FontSize(small).TextColor(k.TextMuted)
		if mgo.Switch(c, qp.Enabled).Label("启用浮窗（替代原来的紧凑窗口）").Changed() {
			cw.applyQuickPanelSetting()
		}
		if !qp.IsEnabled() {
			return
		}
		if mgo.Switch(c, qp.Background).Label("后台执行，不切换主窗口").Changed() {
			_ = config.SaveConfig(cw.cfg)
		}
		if mgo.Switch(c, qp.ClipboardFallback).Label("没有辅助功能权限时读取剪贴板").Changed() {
			_ = config.SaveConfig(cw.cfg)
		}
		if runtime.GOOS == "darwin" {
			if mgo.Switch(c, &qp.AutoPopup).Label("选中文本后自动弹出浮窗（实验）").Changed() {
				cw.applyQuickPanelSetting()
			}
			mgo.Text(c, "自动弹出需要辅助功能权限；未授权时仍可用上面的快捷键。").
				FontSize(small).TextColor(k.TextMuted)
			if qp.AutoPopup {
				cw.quickMinCharsRow(c)
			}
		} else if runtime.GOOS == "linux" {
			mgo.Text(c, "Linux 下会读取 X11 PRIMARY 选区（需要 xclip、xsel 或 wl-clipboard）；自动弹出暂不支持。").
				FontSize(small).TextColor(k.TextMuted)
		}
		if !cw.cfg.RunInBackground {
			mgo.Text(c, "提示：关闭主窗口会退出程序，快捷键也随之失效；可开启上方的「允许后台运行」让浮窗常驻。").
				FontSize(small).TextColor(k.TextMuted)
		}
		cw.quickDefaultAgentRow(c)
		cw.quickFavoritesRow(c)
		if mgo.Switch(c, &qp.DefaultWorktree).Label("默认在新建 worktree 中执行").Changed() {
			_ = config.SaveConfig(cw.cfg)
		}
		if explorerMenuSupported() {
			if mgo.Switch(c, &qp.ExplorerMenu).Label("添加到资源管理器右键菜单").Changed() {
				if err := setExplorerMenu(qp.ExplorerMenu); err != nil {
					cw.toast("右键菜单更新失败："+err.Error(), mcore.SeverityError)
				}
				_ = config.SaveConfig(cw.cfg)
			}
		}
		if runtime.GOOS == "windows" {
			if mgo.Switch(c, &qp.AutoCopySelection).Label("按快捷键时自动复制当前选区").Changed() {
				_ = config.SaveConfig(cw.cfg)
			}
			mgo.Text(c, "会模拟 Ctrl+C 并尽量还原剪贴板文本；剪贴板里的图片或富文本可能被替换。").
				FontSize(small).TextColor(k.TextMuted)
		}
		cw.quickPermissionRow(c)
	})
}

// quickMinCharsRow chooses how long a selection must be for AutoPopup.
func (cw *ChatWindow) quickMinCharsRow(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	levels := []int{1, 2, 3, 5, 10}
	idx := 1
	for i, v := range levels {
		if v == cw.cfg.QuickPanel.MinSelection() {
			idx = i
		}
	}
	segments := make([]kit.Segment, len(levels))
	for i, v := range levels {
		segments[i] = kit.Segment{Label: strconv.Itoa(v) + " 字"}
	}
	mgo.Column(c).Gap(u).Children(func() {
		mgo.Text(c, "最短选区").FontSize(small).TextColor(k.TextMuted)
		kit.SegmentedControl(c, &idx, segments, kit.SegmentedControlOptions{Label: "最短选区", Equal: true})
	})
	if levels[idx] != cw.cfg.QuickPanel.MinChars {
		cw.cfg.QuickPanel.MinChars = levels[idx]
		_ = config.SaveConfig(cw.cfg)
	}
}

// quickDefaultAgentRow chooses the agent a new panel starts with.
func (cw *ChatWindow) quickDefaultAgentRow(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	qp := &cw.cfg.QuickPanel
	opts := make([]kit.SearchSelectOption[string], 0, len(cw.cfg.Agents)+len(cw.cfg.ACPAgents)+1)
	opts = append(opts, kit.SearchSelectOption[string]{
		Value: "", Label: chatAgentName, Leading: iconLeading(c, chatAgentIcon),
	})
	for _, a := range cw.cfg.Agents {
		if a.Name == "" {
			continue
		}
		opts = append(opts, kit.SearchSelectOption[string]{
			Value: "agent:" + a.Name, Label: a.Name, Leading: cw.agentLeading(c, a.Name),
		})
	}
	for _, a := range cw.cfg.ACPAgents {
		if a.Disabled || a.Name == "" {
			continue
		}
		opts = append(opts, kit.SearchSelectOption[string]{
			Value: "acp:" + a.Name, Label: a.Name, Leading: logoLeading(c, a.Name),
		})
	}
	mgo.Column(c).Gap(u).Children(func() {
		mgo.Text(c, "默认 Agent").FontSize(small).TextColor(k.TextMuted)
		sel := kit.SearchSelect(c, &qp.DefaultAgent, opts, kit.SearchSelectOptions[string]{
			Label: "默认 Agent", Placeholder: "跟随当前会话",
			SearchPlaceholder: "搜索 Agent", MinWidth: 200, MinHeight: 32,
		})
		sel.Element.Justify(mgo.Start)
		if sel.Changed() {
			_ = config.SaveConfig(cw.cfg)
		}
	})
}

// quickFavoritesRow chooses which agents the panel shows as chips.
func (cw *ChatWindow) quickFavoritesRow(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	qp := &cw.cfg.QuickPanel
	agents := allQuickAgents(cw.cfg)
	mgo.Column(c).Gap(u).Children(func() {
		mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, "浮窗里显示的 Agent").FontSize(small).TextColor(k.TextMuted)
			if len(qp.Favorites) > 0 {
				if kit.Button(c, "显示全部", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
					qp.Favorites = nil
					_ = config.SaveConfig(cw.cfg)
				}
			}
		})
		mgo.Text(c, "不选则显示全部，最多 8 个").FontSize(small).TextColor(k.TextMuted)
		mgo.Row(c).Wrap().Gap(u).Children(func() {
			for _, a := range agents {
				ref := a.Ref
				on := containsStr(qp.Favorites, ref)
				variant := kit.Ghost
				if on {
					variant = kit.Secondary
				}
				if kit.Button(c, a.Name, kit.ButtonOptions{
					Variant: variant,
					Leading: cw.agentChipLeading(c, a),
				}).Clicked() {
					qp.Favorites = toggleRef(qp.Favorites, ref)
					_ = config.SaveConfig(cw.cfg)
				}
			}
		})
	})
}

// toggleRef adds ref to list, or removes it when it is already there.
func toggleRef(list []string, ref string) []string {
	out := make([]string, 0, len(list)+1)
	found := false
	for _, item := range list {
		if item == ref {
			found = true
			continue
		}
		out = append(out, item)
	}
	if !found {
		out = append(out, ref)
	}
	return out
}

// quickPermissionRow reports, and opens, the system capture permission.
func (cw *ChatWindow) quickPermissionRow(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	perm := capture.Permission{State: capture.PermissionUnsupported}
	if cw.grabber != nil {
		perm = cw.grabber.Permission()
	}
	if runtime.GOOS != "darwin" {
		mgo.Text(c, capturePlatformLabel(perm)).FontSize(small).TextColor(k.TextMuted)
		return
	}
	mgo.Row(c).Gap(u * 2).AlignItems(mgo.Center).Wrap().Children(func() {
		switch perm.State {
		case capture.PermissionGranted:
			mgo.Text(c, "辅助功能：已授权").FontSize(small).TextColor(k.Success)
		case capture.PermissionDenied:
			mgo.Text(c, "辅助功能：未授权").FontSize(small).TextColor(k.Danger)
			if cw.grabber != nil &&
				kit.Button(c, "请求权限", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
				cw.grabber.RequestPermission()
			}
			if perm.SettingsURL != "" &&
				kit.Button(c, "打开系统设置", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
				c.OpenURL(perm.SettingsURL)
			}
			if kit.Button(c, "重新检测", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
				// Rebuild the watcher too: granting Accessibility while the
				// app runs is common, and the monitor installs only when it
				// is granted.
				cw.applyQuickPanelSetting()
			}
			mgo.Text(c, "授权后如未生效，请重启 ChatGo").FontSize(small).TextColor(k.TextMuted)
		default:
			mgo.Text(c, "当前平台暂不支持系统级取词").FontSize(small).TextColor(k.TextMuted)
		}
	})
}

// capturePlatformLabel says what selection capture means on this platform.
func capturePlatformLabel(perm capture.Permission) string {
	switch runtime.GOOS {
	case "linux":
		if perm.State == capture.PermissionGranted {
			return "X11 PRIMARY 选区：可用"
		}
		return "X11 PRIMARY 选区：需要安装 xclip、xsel 或 wl-clipboard"
	case "windows":
		return "系统级取词：暂不支持，可用复制后按快捷键"
	default:
		return "当前平台暂不支持系统级取词"
	}
}
