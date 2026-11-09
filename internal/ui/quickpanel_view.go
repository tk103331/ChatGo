package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"chatgo/internal/capture"
	"chatgo/internal/ui/assets"
	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"

	mgo "github.com/egoist/mygo/ui"
)

// view draws the panel in whichever shape it currently has.
func (qp *quickPanel) view(c *mgo.Context) {
	mcore.Use(c, mcore.Settings{
		Mode:    qp.cw.themeMode,
		Density: qp.cw.density,
		Locale:  qp.cw.locale,
		Theme:   qp.cw.colorTheme,
	})
	switch qp.mode {
	case quickPanelBar:
		qp.bar(c)
	case quickPanelRunning:
		qp.running(c)
	default:
		qp.compose(c)
	}
}

// panelFrame paints the floating surface the window draws over transparency.
func (qp *quickPanel) panelFrame(c *mgo.Context) mgo.Element {
	k := mcore.Tokens(c)
	return mgo.Column(c).Fill().
		Background(k.Background).
		Radius(12).
		Border(mtheme.BorderWidth, k.Border)
}

// bar is the compact strip that appears next to a selection.
func (qp *quickPanel) bar(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	agents := qp.agents()
	shown := agents
	if len(shown) > 3 {
		shown = shown[:3]
	}
	root := qp.panelFrame(c).Padding(u*1.5, u*2.5).Children(func() {
		mgo.Row(c).Fill().Gap(u * 2).AlignItems(mgo.Center).Children(func() {
			mgo.Icon(c, assets.BrandMark()).Size(16, 16).TextColor(k.Accent).Shrink(0)
			count := utf8.RuneCountInString(strings.TrimSpace(qp.selection.Text))
			mgo.Text(c, fmt.Sprintf("%d 字", count)).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
			mgo.Box(c).Grow(1)
			for _, a := range shown {
				ref := a.Ref
				b := kit.Button(c, "", kit.ButtonOptions{
					Variant: kit.Ghost,
					Label:   "用 " + a.Name + " 执行",
					Leading: qp.cw.agentChipLeading(c, a),
				})
				b.Tooltip(a.Name)
				if b.Clicked() {
					qp.chooseAgent(ref)
				}
			}
			if len(agents) > len(shown) {
				more := kit.Button(c, fmt.Sprintf("+%d", len(agents)-len(shown)), kit.ButtonOptions{
					Variant: kit.Ghost,
					Label:   "更多 Agent",
				})
				more.Tooltip("更多 Agent")
				if more.Clicked() {
					qp.showCompose()
				}
			}
			if mcore.IconAction(c, icons.Must("x"), "关闭").Clicked() {
				qp.hide()
			}
		})
	})
	qp.barHovered = root.Hovered()
}

// compose is the input panel: agent chips, the prompt and the directory.
func (qp *quickPanel) compose(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	qp.panelFrame(c).Padding(u*2, u*3).Gap(u * 2).Children(func() {
		mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Icon(c, assets.BrandMark()).Size(16, 16).TextColor(k.Accent).Shrink(0)
			mgo.Text(c, qp.contextLabel()).FontSize(caption).TextColor(k.TextMuted).
				Grow(1).SingleLine().Ellipsis("…")
			if mcore.IconAction(c, icons.Must("x"), "关闭").Clicked() {
				qp.hide()
			}
		})

		// The agent chips and the model choice share one row: the chips
		// scroll sideways so the model keeps its place at the row's end, even
		// when many agents are shown.
		mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.ScrollHorizontal(c).Grow(1).MinWidth(0).Children(func() {
				mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
					for _, a := range qp.visibleAgents(8) {
						ref := a.Ref
						variant := kit.Ghost
						if ref == qp.agent {
							variant = kit.Secondary
						}
						if kit.Button(c, a.Name, kit.ButtonOptions{
							Variant: variant,
							Leading: qp.cw.agentChipLeading(c, a),
						}).Clicked() {
							qp.setAgent(ref)
						}
					}
				})
			})
			qp.modelSelector(c)
		})

		if qp.hasSelection {
			mgo.Row(c).FillWidth().Wrap().Gap(u).Children(func() {
				for _, p := range quickPresets {
					p := p
					if kit.Button(c, p.Label, kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
						qp.draft = p.Prompt
						// The template replaces the quote block; submit
						// attaches the selection again.
						qp.selectionInDraft = false
						qp.focusInput = true
					}
				}
			})
		}

		in := composer(c, &qp.draft, composerOptions{
			Placeholder: qp.placeholder(),
			MaxHeight:   110,
			Actions: func() {
				r := kit.SendButton(c, kit.SendButtonOptions{
					Disabled: strings.TrimSpace(qp.draft) == "",
					Shortcut: "Enter",
				})
				if r.Sent() {
					qp.submit()
				}
			},
		})
		in.Element.FillWidth().Shrink(0)
		if qp.focusInput {
			in.Input.Focus()
			qp.focusInput = false
		}
		if in.Submitted() {
			qp.submit()
		}

		mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
			label := "选择目录"
			if qp.workspace != "" {
				label = middleEllipsis(qp.workspace, 30)
			}
			if kit.Button(c, label, kit.ButtonOptions{
				Variant: kit.Ghost,
				Icon:    icons.Must("folder"),
			}).Clicked() {
				qp.pickWorkspace()
			}
			mgo.Box(c).Grow(1)
			kit.SwitchRow(c, &qp.worktree, "新建 worktree", kit.SwitchRowOptions{Compact: true})
		})
	})
}

// running shows the reply as it streams.
func (qp *quickPanel) running(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	task := qp.task
	if task == nil {
		qp.compose(c)
		return
	}
	body := "正在思考…"
	if task.note != "" && (task.preparing || task.Conv == nil) {
		body = task.note
	} else if m := task.reply(); m != nil && strings.TrimSpace(m.Content) != "" {
		body = m.Content
	}
	busy := task.running(qp.cw)
	qp.panelFrame(c).Padding(u*2, u*3).Gap(u * 2).Children(func() {
		mgo.Row(c).FillWidth().Gap(u * 2).AlignItems(mgo.Center).Children(func() {
			qp.cw.agentChipLeading(c, quickAgentOption{Ref: task.Agent, Name: authorNameOf(task.Agent)})()
			mgo.Text(c, quickTaskSummary(task.Prompt)).FontSize(caption).
				Grow(1).SingleLine().Ellipsis("…")
			if busy {
				mcore.Spinner(c, mcore.SpinnerOptions{Size: 12})
			}
			if task.Conv != nil {
				if mcore.IconAction(c, icons.Must("maximize"), "在主窗口打开").Clicked() {
					qp.openInMain()
				}
			}
			if mcore.IconAction(c, icons.Must("plus"), "新建任务").Clicked() {
				qp.newTask()
			}
			if mcore.IconAction(c, icons.Must("x"), "关闭").Clicked() {
				qp.hide()
			}
		})

		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			mgo.Text(c, body).Selectable().
				FontSize(mcore.FontSize(c, mtheme.BodySize)).
				TextColor(k.Text)
		})

		mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, quickTaskStatus(qp.cw, task)).FontSize(caption).
				TextColor(k.TextMuted).Grow(1).SingleLine().Ellipsis("…")
			if busy {
				if kit.Button(c, "停止", kit.ButtonOptions{Variant: kit.Danger}).Clicked() {
					task.stop(qp.cw)
				}
			} else if task.Conv != nil {
				if kit.Button(c, "在主窗口打开", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
					qp.openInMain()
				}
			} else if kit.Button(c, "关闭", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
				qp.hide()
			}
		})
	})
}

// quickPreset is one of the instruction templates offered with a selection.
type quickPreset struct {
	Label  string
	Prompt string
}

// quickPresets fill the composer with a common instruction; the selection
// itself is quoted when the task is submitted.
var quickPresets = []quickPreset{
	{"解释", "解释这段内容，说明它的含义和关键点。"},
	{"翻译", "翻译这段内容：原文是中文就翻译成英文，否则翻译成中文。"},
	{"总结", "总结这段内容的要点。"},
}

// modelSelector chooses the model the task runs with, for agents that have a
// choice. ACP agents pick their own model through the protocol.
func (qp *quickPanel) modelSelector(c *mgo.Context) {
	refs := qp.cw.modelRefsForAgent(qp.agent)
	if len(refs) == 0 {
		return
	}
	cur := qp.effectiveModel()
	opts := make([]kit.SearchSelectOption[string], 0, len(refs))
	for _, ref := range refs {
		opts = append(opts, kit.SearchSelectOption[string]{
			Value: ref, Label: qp.cw.refLabel(ref), Leading: qp.cw.providerLeading(c, ref),
		})
	}
	r := kit.SearchSelect(c, &cur, opts, kit.SearchSelectOptions[string]{
		Label: "模型", Placeholder: "跟随 Agent", SearchPlaceholder: "搜索模型",
		MinWidth: 150, MinHeight: 30,
	})
	r.Element.MaxWidth(200).MinWidth(0)
	if r.Changed() {
		qp.model = cur
	}
}

// chooseAgent picks an agent and moves the bar into the compose panel.
func (qp *quickPanel) chooseAgent(ref string) {
	qp.setAgent(ref)
	qp.showCompose()
}

// contextLabel says where the prompt's context came from.
func (qp *quickPanel) contextLabel() string {
	switch {
	case qp.hasSelection && qp.selection.Source == capture.SourceClipboard:
		return "来自剪贴板"
	case qp.hasSelection && qp.selection.Source == capture.SourcePrimary:
		return "来自选中文本"
	case qp.hasSelection && qp.selection.Source == capture.SourceService:
		return "来自服务"
	case qp.hasSelection && qp.selection.App != "":
		return "来自 " + qp.selection.App
	case qp.hasSelection:
		return "已读取选中文本"
	case qp.workspace != "":
		return "工作目录 · " + workspaceTitle(qp.workspace)
	default:
		return "快捷任务"
	}
}
