package ui

import (
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/config"
	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// experimentalTab collects the features that are still being shaped: they
// ship off by default and their settings may change between versions.
func (cw *ChatWindow) experimentalTab(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Scroll(c).Fill().MinHeight(0).Children(func() {
		mgo.Column(c).Gap(u * 3).MaxWidth(560).Children(func() {
			mgo.Text(c, "这些功能仍在验证中，默认关闭，设置项可能随版本调整。").
				FontSize(small).TextColor(k.TextMuted)
			cw.checkpointSection(c)
			cw.rtkSection(c)
		})
	})
}

// checkpointSection chooses how often an agent's file changes are committed
// as a checkpoint the chat can roll back to.
func (cw *ChatWindow) checkpointSection(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Gap(u * 2).Children(func() {
		mgo.Text(c, "Agent Checkpoint").FontWeight(600)
		mgo.Text(c, "ACP Agent 改动文件后自动提交为 checkpoint，对话中可「回退到此点」。").
			FontSize(small).TextColor(k.TextMuted)
		level := 0
		switch cw.cfg.AgentCheckpoint {
		case "turn":
			level = 1
		case "batch":
			level = 2
		}
		kit.SegmentedControl(c, &level, []kit.Segment{
			{Label: "关闭"}, {Label: "每轮回复"}, {Label: "工具批次"},
		}, kit.SegmentedControlOptions{Label: "Agent Checkpoint", Equal: true})
		want := []string{"off", "turn", "batch"}[level]
		if cw.cfg.AgentCheckpoint != want {
			cw.cfg.AgentCheckpoint = want
			_ = config.SaveConfig(cw.cfg)
			cw.toast("Agent Checkpoint 已更新", mcore.SeveritySuccess)
		}
	})
}

// rtkSection turns command-output compression on and reports whether the rtk
// CLI it needs is installed.
func (cw *ChatWindow) rtkSection(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	probe, probed := cw.rtkStatus()
	mgo.Column(c).Gap(u * 2).Children(func() {
		mgo.Text(c, "RTK 命令输出压缩").FontWeight(600)
		mgo.Text(c, "通过 rtk 命令行工具压缩 ls、grep、git 等命令的输出，减少喂给模型的内容。").
			FontSize(small).TextColor(k.TextMuted)

		on := cw.cfg.RTKEnabled
		if mgo.Switch(c, &on).Label("启用 RTK 压缩命令输出").Changed() {
			cw.setRTKEnabled(on)
		}

		mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
			switch {
			case !probed:
				mcore.Spinner(c, mcore.SpinnerOptions{Size: 12})
				mgo.Text(c, "正在检查 rtk 环境…").FontSize(small).TextColor(k.TextMuted)
			case probe.OK():
				mgo.Icon(c, icons.Must("check")).Size(14, 14).TextColor(k.Success).Shrink(0)
				mgo.Text(c, "已检测到 "+probe.Version+"（"+probe.Path+"）").
					FontSize(small).TextColor(k.TextMuted).SingleLine().Ellipsis("…")
			default:
				mgo.Icon(c, icons.Must("triangle-alert")).Size(14, 14).TextColor(k.Danger).Shrink(0)
				mgo.Text(c, probe.Err).FontSize(small).TextColor(k.Danger)
			}
		})
		if probed {
			if kit.Button(c, "重新检测", kit.ButtonOptions{Variant: kit.Outline}).Clicked() {
				cw.recheckRTK()
			}
		}
	})
}

// setRTKEnabled saves the switch, refusing to turn compression on while rtk
// is missing: the setting would be on with nothing behind it, and every
// command would have to fall back at run time.
func (cw *ChatWindow) setRTKEnabled(on bool) {
	if on {
		probe, probed := cw.rtkStatus()
		if !probed {
			cw.toast("正在检查 rtk 环境，请稍后再试", mcore.SeverityWarning)
			return
		}
		if !probe.OK() {
			cw.toast(probe.Err, mcore.SeverityError)
			return
		}
	}
	cw.cfg.RTKEnabled = on
	_ = config.SaveConfig(cw.cfg)
	if on {
		cw.toast("已启用 RTK 压缩命令输出", mcore.SeveritySuccess)
		return
	}
	cw.toast("已关闭 RTK 压缩命令输出", mcore.SeveritySuccess)
}
