package ui

import (
	"path/filepath"
	"strconv"
	"strings"

	mgo "github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
	"chatgo/pkg/models"
)

// centerTab is one tab of the center area. Kind decides which fields matter
// and which body draws it.
type centerTab struct {
	Kind centerKind
	// ID names the tab uniquely, and is what openers look up to focus a tab
	// already showing the same thing.
	ID string
	// Title labels the tab; Path is the file or resource behind it.
	Title string
	Path  string
	// ConvID is the conversation a chat tab shows, and Agent its agent, so
	// the tab names and marks itself without leaning on the window's current
	// conversation.
	ConvID string
	Agent  string

	// Body, From, To and Lang carry text for the text, diff and editor kinds.
	Body     string
	From, To string
	Lang     string

	// Bmp and Svg hold a decoded image.
	Bmp *mgo.Bitmap
	Svg *mgo.SVG

	// Rendered shows a Markdown file as a rendered preview rather than source.
	Rendered bool
	// GotoLine is a one-based line the viewer should scroll to, 0 for none.
	// It survives until the editor has a laid-out view able to honour it.
	GotoLine int

	blame []kit.BlameLine
	view  kit.CodeViewerState
	dview kit.DiffViewerState
	bview kit.BlameViewState

	// Commit is the commit a commit tab shows.
	Commit *commitInfo
	// Web is the page a web tab shows.
	Web *webTab
	// Term is the shell a terminal tab runs.
	Term *termTab
	// Edit is the writable buffer of an editor tab.
	Edit *editorTab
}

// editorTab is an editor tab's buffer and the state it needs to save.
type editorTab struct {
	st editor.State
	// saved is the buffer version last written to disk, so the tab can tell
	// whether it still has unsaved changes.
	saved uint64
}

// itoa is strconv.Itoa under a short name, for tab IDs.
func itoa(n int) string { return strconv.Itoa(n) }

// dirty reports whether an editor tab has changes not yet on disk.
func (t *centerTab) dirty() bool {
	return t.Edit != nil && t.Edit.st.Buffer != nil && t.Edit.st.Buffer.Version() != t.Edit.saved
}

// closable reports whether the tab may be closed. Every tab can: a chat tab
// closes its view, keeping the conversation in the list to reopen.
func (t *centerTab) closable() bool { return true }

// centerPaneView draws one pane: its tab strip and the active tab's body.
func (cw *ChatWindow) centerPaneView(c *mgo.Context, idx int) {
	k := mcore.Tokens(c)
	p := &cw.center.panes[idx]
	p.clampSel()
	focused := cw.center.focus == idx
	box := mgo.Column(c).Fill().MinWidth(0).MinHeight(0).AlignItems(mgo.Stretch).
		Background(k.Background)
	if cw.center.layout != layoutSingle {
		// A focused pane is marked by its border, so the new-tab menu's
		// target is never in doubt.
		border := k.Border
		if focused {
			border = k.Accent
		}
		box.Border(mtheme.BorderWidth, border)
	}
	box.Children(func() {
		cw.centerTabStrip(c, idx)
		body := mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0)
		if body.Clicked() {
			cw.center.focus = idx
		}
		body.Children(func() {
			tab := p.active()
			if tab == nil {
				cw.centerEmpty(c, idx)
				return
			}
			cw.centerTabBody(c, tab)
		})
	})
}

// centerEmpty is what an empty pane shows: a hint and the button that opens
// any tab kind in it.
func (cw *ChatWindow) centerEmpty(c *mgo.Context, idx int) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().Center().Gap(u * 2).Children(func() {
		mgo.Icon(c, icons.Must("layout-grid")).Size(32, 32).TextColor(k.TextMuted)
		mgo.Text(c, "这个窗格还没有标签").TextColor(k.TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		btn := kit.Button(c, "新建标签", kit.ButtonOptions{Variant: kit.Outline, Icon: icons.Must("plus")})
		open := mgo.Local(btn, "chatgo.center.emptynew:"+itoa(idx), func() bool { return false })
		if btn.Clicked() {
			cw.center.focus = idx
			*open = !*open
		}
		cw.newTabChoices(c, btn, open, idx)
	})
}

// centerTabStrip draws a pane's tabs, its new-tab menu and, on the focused
// pane, the layout switcher.
// centerTabAction is a change to a pane's tabs deferred until the strip has
// drawn every tab, so a context menu action never mutates the slice while the
// strip iterates it.
type centerTabAction struct {
	// close is the tab to close, -1 when none; closeOthers keeps this tab and
	// closes the rest; moveFrom and moveTo move it to another pane.
	close       int
	closeOthers int
	moveFrom    int
	moveTo      int
}

func (cw *ChatWindow) centerTabStrip(c *mgo.Context, idx int) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	p := &cw.center.panes[idx]
	action := centerTabAction{close: -1, closeOthers: -1, moveFrom: -1, moveTo: -1}
	mgo.Row(c).FillWidth().Shrink(0).MinWidth(0).Padding(u, u*2).Gap(u).
		AlignItems(mgo.Center).Background(k.Surface).
		BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
		mgo.ScrollHorizontal(c).Grow(1).MinWidth(0).Children(func() {
			mgo.Row(c).Gap(u).Children(func() {
				for i := range p.tabs {
					cw.centerTabButton(c, idx, i, &action)
				}
			})
		})
		cw.newTabMenu(c, idx)
		if cw.showLayoutMenu(idx) {
			cw.layoutMenu(c)
		}
	})
	// The actions run after the loop, when the slice is not being walked.
	switch {
	case action.close >= 0:
		cw.closeCenterTab(idx, action.close)
	case action.closeOthers >= 0:
		cw.closeOtherCenterTabs(idx, action.closeOthers)
	case action.moveTo >= 0:
		cw.moveCenterTab(idx, action.moveFrom, action.moveTo)
	}
}

// showLayoutMenu reports whether the pane carries the layout switcher. Only
// the Chat pane does, so switching layouts reads as a property of the
// conversation rather than of whichever pane happens to be focused.
// showLayoutMenu reports whether the pane carries the layout switcher. The
// switcher belongs to the leftmost pane, whichever tab it happens to show, so
// it stays reachable after the Chat tab is covered by a file, a terminal or
// any other panel.
func (cw *ChatWindow) showLayoutMenu(idx int) bool {
	return idx == chatPaneIndex
}

// centerTabButton draws one tab, recording any close or move the strip must
// apply after it has drawn every tab.
func (cw *ChatWindow) centerTabButton(c *mgo.Context, pane, i int, action *centerTabAction) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	p := &cw.center.panes[pane]
	tab := p.tabs[i]
	// A web tab is named by its page, which changes as the page navigates.
	if tab.Web != nil && tab.Web.Title != "" {
		tab.Title = tab.Web.Title
	}
	if tab.Term != nil {
		tab.Title = tab.Term.label(i)
	}
	title := cw.centerTabTitle(tab)
	b := mgo.ButtonBase(c).Row().Shrink(0).MinHeight(mcore.Density(c).ControlHeight()).
		Padding(u, u*2).Gap(u).AlignItems(mgo.Center).Radius(mtheme.ControlRadius).Label(title)
	active := i == p.sel
	switch {
	case active:
		b.Background(k.Selection)
	case b.Hovered():
		b.Background(k.SurfaceHover)
	}
	b.Children(func() {
		cw.centerTabIcon(c, tab)
		mgo.Text(c, title).SingleLine().MaxWidth(u * 24).Ellipsis("…")
		if tab.dirty() {
			mgo.Box(c).Size(6, 6).Shrink(0).Role(mgo.RoleNone).Background(k.Accent).Radius(3)
		}
		if tab.closable() {
			if mcore.IconAction(c, icons.Must("x"), "关闭 "+title).Clicked() {
				action.close = i
			}
		}
	})
	if b.Clicked() {
		p.sel = i
		cw.center.focus = pane
	}
	cw.centerTabContextMenu(c, b, pane, i, action)
}

// centerTabTitle is the label a tab shows. A chat tab is named after its
// conversation's agent, so a plain agent and an ACP one are told apart by
// name as well as by icon.
func (cw *ChatWindow) centerTabTitle(tab *centerTab) string {
	if tab.Kind != centerChat {
		return tab.Title
	}
	return agentTabName(tab.Agent)
}

// agentTabName names a chat tab after its conversation's agent.
func agentTabName(agent string) string {
	switch {
	case strings.HasPrefix(agent, "acp:"):
		return strings.TrimPrefix(agent, "acp:")
	case strings.HasPrefix(agent, "agent:"):
		return strings.TrimPrefix(agent, "agent:")
	default:
		return chatAgentName
	}
}

// centerTabIcon draws a tab's leading mark. A chat tab reuses the agent
// marks, so an ACP agent shows its brand logo and a custom agent its avatar.
func (cw *ChatWindow) centerTabIcon(c *mgo.Context, tab *centerTab) {
	if tab.Kind == centerChat {
		switch {
		case strings.HasPrefix(tab.Agent, "acp:"):
			cw.acpLeading(c, strings.TrimPrefix(tab.Agent, "acp:"), 14)
		case strings.HasPrefix(tab.Agent, "agent:"):
			cw.agentMarkByName(c, strings.TrimPrefix(tab.Agent, "agent:"), 14)
		default:
			mgo.Icon(c, icons.Must(chatAgentIcon)).Size(14, 14).TextColor(mcore.Tokens(c).Accent)
		}
		return
	}
	if icon := centerTabIconName(tab); icon != "" {
		mgo.Icon(c, icons.Must(icon)).Size(13, 13).TextColor(mcore.Tokens(c).TextMuted).Shrink(0)
	}
}

// centerTabIconName is the Lucide icon naming a tab's kind.
func centerTabIconName(tab *centerTab) string {
	switch tab.Kind {
	case centerImage:
		return "image"
	case centerDiff, centerCommit:
		return "git-branch"
	case centerGitGraph:
		return "git-branch"
	case centerWorktree:
		return "git-fork"
	case centerBlame:
		return "history"
	case centerWeb:
		return "globe"
	case centerTerm:
		return "terminal"
	case centerEditor:
		return "pencil"
	case centerCodeSearch:
		return "search"
	case centerText:
		if isMarkdownFile(tab.Path) {
			return "eye"
		}
		return "file-text"
	}
	return ""
}

// centerTabContextMenu adds a tab's right-click actions: closing it, closing
// the others, and moving it to another pane.
func (cw *ChatWindow) centerTabContextMenu(c *mgo.Context, target mgo.Element, pane, i int, action *centerTabAction) {
	if !target.Valid() {
		return
	}
	p := &cw.center.panes[pane]
	if i >= len(p.tabs) {
		return
	}
	tab := p.tabs[i]
	kit.ContextMenu(c, target, kit.ContextMenuOptions{}, func(m *kit.PopupMenu) {
		// A menu action only records what to do; the strip applies it after
		// drawing every tab, so closing here cannot shift the slice mid-loop.
		if tab.closable() && m.Item("关闭", kit.PopupMenuItemOptions{}) {
			action.close = i
		}
		if len(p.tabs) > 1 && m.Item("关闭其他", kit.PopupMenuItemOptions{}) {
			action.closeOthers = i
		}
		for _, target := range cw.center.visiblePanes() {
			if target == pane {
				continue
			}
			// A chat tab may only move back to the leftmost pane.
			if tab.Kind == centerChat && target != chatPaneIndex {
				continue
			}
			label := "移动到窗格 " + itoa(target+1)
			if m.Item(label, kit.PopupMenuItemOptions{}) {
				action.moveFrom, action.moveTo = i, target
			}
		}
	})
}

// newTabMenu is the dropdown that opens a Terminal, WebView, GitGraph,
// Worktree or Editor tab in the pane it belongs to.
func (cw *ChatWindow) newTabMenu(c *mgo.Context, pane int) {
	plus := mcore.IconAction(c, icons.Must("plus"), "新建标签").Tooltip("新建标签")
	open := mgo.Local(plus, "chatgo.center.newtab", func() bool { return false })
	if plus.Clicked() {
		cw.center.focus = pane
		*open = !*open
	}
	cw.newTabChoices(c, plus, open, pane)
}

// newTabChoices lists the tab kinds that can be opened, closing the menu and
// opening the chosen kind in pane. The Agent item opens a second level that
// starts a conversation with the chosen agent, like the sidebar's own.
func (cw *ChatWindow) newTabChoices(c *mgo.Context, anchor mgo.Element, open *bool, pane int) {
	pick := func(run func(int)) {
		*open = false
		run(pane)
	}
	stage := mgo.Local(anchor, "chatgo.center.newtab.stage", func() int { return 0 })
	if !*open || pane != chatPaneIndex {
		*stage = 0
	}
	newAgentConv := func(agent string) {
		*stage = 0
		*open = false
		cw.newConversationIn(cw.workspace, agent)
	}
	title := "新建标签"
	if *stage == 1 {
		title = "Agent 会话"
	}
	kit.Popover(c, anchor, open, kit.PopoverOptions{Title: title, Width: 220}, func() {
		if *stage == 1 {
			menuItem(c, iconLeading(c, "chevron-left"), "返回", func() { *stage = 0 })
			menuItem(c, iconLeading(c, chatAgentIcon), chatAgentName, func() { newAgentConv("") })
			for _, a := range cw.cfg.Agents {
				name := a.Name
				menuItem(c, cw.agentLeading(c, name), name, func() { newAgentConv("agent:" + name) })
			}
			for _, a := range cw.cfg.ACPAgents {
				if a.Disabled {
					continue
				}
				name := a.Name
				menuItem(c, logoLeading(c, name), name, func() { newAgentConv("acp:" + name) })
			}
			return
		}
		// A chat tab only opens in the leftmost pane, so its menu entry
		// belongs to that pane's new-tab menu alone.
		if pane == chatPaneIndex {
			menuItem(c, iconLeading(c, "bot"), "Agent 会话", func() { *stage = 1 })
		}
		menuItem(c, iconLeading(c, "search"), "CodeSearch", func() { pick(cw.openCodeSearchTab) })
		menuItem(c, iconLeading(c, "terminal"), "Terminal", func() { pick(cw.openTerminalTab) })
		menuItem(c, iconLeading(c, "globe"), "WebView", func() { pick(cw.openWebTabIn) })
		menuItem(c, iconLeading(c, "git-branch"), "GitGraph", func() { pick(cw.openGitGraphTab) })
		menuItem(c, iconLeading(c, "git-fork"), "Worktree", func() { pick(cw.openWorktreeTab) })
		menuItem(c, iconLeading(c, "pencil"), "Editor", func() { pick(cw.openEditorTabPick) })
	})
}

// layoutMenu switches the center area between its layouts.
func (cw *ChatWindow) layoutMenu(c *mgo.Context) {
	btn := mcore.IconAction(c, icons.Must(layoutIcon(cw.center.layout)), "分屏布局").Tooltip("分屏布局")
	open := mgo.Local(btn, "chatgo.center.layout", func() bool { return false })
	if btn.Clicked() {
		*open = !*open
	}
	pick := func(l centerLayout) {
		*open = false
		cw.setLayout(l)
	}
	kit.Popover(c, btn, open, kit.PopoverOptions{Title: "分屏布局", Width: 240}, func() {
		for _, l := range []centerLayout{layoutSingle, layoutCols, layoutLeftWide} {
			menuItem(c, iconLeading(c, layoutIcon(l)), layoutLabel(l), func() { pick(l) })
		}
	})
}

// layoutIcon is the mark naming a layout.
func layoutIcon(l centerLayout) string {
	switch l {
	case layoutCols:
		return "columns-2"
	case layoutLeftWide:
		return "layout-panel-left"
	default:
		return "square"
	}
}

// layoutLabel names a layout in the menu.
func layoutLabel(l centerLayout) string {
	switch l {
	case layoutCols:
		return "两列"
	case layoutLeftWide:
		return "左一右二"
	default:
		return "单列"
	}
}

// centerTabBody draws the active tab's content.
func (cw *ChatWindow) centerTabBody(c *mgo.Context, tab *centerTab) {
	switch tab.Kind {
	case centerChat:
		cw.attachConvTab(tab)
		cw.messageArea(c)
	case centerTerm:
		cw.centerTermBody(c, tab)
	case centerWeb:
		cw.centerWebBody(c, tab)
	case centerGitGraph:
		cw.gitGraphBody(c)
	case centerWorktree:
		cw.worktreeBody(c)
	case centerEditor:
		cw.centerEditorBody(c, tab)
	case centerCodeSearch:
		cw.codeSearchBody(c)
	case centerCommit:
		if tab.Commit != nil {
			cw.commitDiffView(c, tab.Commit)
			return
		}
		cw.centerLoading(c)
	case centerDiff:
		cw.centerDiffBody(c, tab)
	case centerBlame:
		kit.BlameView(c, &tab.bview, tab.blame, kit.BlameViewOptions{Language: tab.Lang}).
			Grow(1).MinHeight(0)
	case centerImage:
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			switch {
			case tab.Bmp != nil:
				mgo.Image(c, tab.Bmp).Fit(mgo.Contain)
			case tab.Svg != nil:
				mgo.Image(c, tab.Svg).Fit(mgo.Contain)
			}
		})
	default:
		cw.centerTextBody(c, tab)
	}
}

// centerLoading is the placeholder a tab shows while its content loads.
func (cw *ChatWindow) centerLoading(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	mgo.Row(c).Fill().Center().Gap(u).Children(func() {
		mcore.Spinner(c, mcore.SpinnerOptions{Size: 16, Label: "加载中"})
		mgo.Text(c, "正在加载…").TextColor(mcore.Tokens(c).TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
	})
}

// centerTextBody shows a file: its source, or a rendered Markdown preview,
// with the actions that apply to it.
func (cw *ChatWindow) centerTextBody(c *mgo.Context, tab *centerTab) {
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, cw.workspaceRel(tab.Path)).Font(mtheme.MonoFont).
				FontSize(mcore.FontSize(c, mtheme.CaptionSize)).
				TextColor(mcore.Tokens(c).TextMuted).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
			if isMarkdownFile(tab.Path) {
				label := "预览"
				if tab.Rendered {
					label = "源码"
				}
				if kit.Button(c, label, kit.ButtonOptions{Variant: kit.Ghost, Icon: icons.Must("eye")}).Clicked() {
					tab.Rendered = !tab.Rendered
				}
			}
			if kit.Button(c, "编辑", kit.ButtonOptions{Variant: kit.Ghost, Icon: icons.Must("pencil")}).Clicked() {
				cw.openEditor(tab.Path)
			}
			if cw.gitRepo {
				if kit.Button(c, "追溯", kit.ButtonOptions{Variant: kit.Ghost, Icon: icons.Must("history")}).Clicked() {
					cw.openBlame(cw.workspaceRel(tab.Path))
				}
			}
		})
		if isMarkdownFile(tab.Path) && tab.Rendered {
			mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				mgo.Column(c).FillWidth().Padding(u*4, u*4, u*3, u*4).Children(func() {
					kit.MarkdownPreview(c, tab.Body)
				})
			})
			return
		}
		kit.CodeViewer(c, tab.Body, &tab.view, kit.CodeViewerOptions{
			Language: tab.Lang, Path: tab.Path,
		}).Grow(1).MinHeight(0)
		// The scroll is asked for after the viewer is built, when the editor
		// has the laid-out view it needs to find the line; until it takes,
		// the request is kept for the next frame.
		if tab.GotoLine > 0 {
			if tab.view.ScrollToLine(tab.GotoLine) {
				tab.GotoLine = 0
			} else {
				c.Invalidate()
			}
		}
	})
}

// centerDiffBody shows a file's diff, switchable between split and inline.
func (cw *ChatWindow) centerDiffBody(c *mgo.Context, tab *centerTab) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	unified := mgo.Local(c.Root(), "center-diff-unified:"+tab.ID, func() bool { return false })
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Gap(u).Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).AlignItems(mgo.Center).Gap(u).Padding(u, u*2).Children(func() {
			mgo.Icon(c, icons.Must("file-text")).Size(13, 13).TextColor(k.TextMuted).Shrink(0)
			mgo.Text(c, strings.TrimPrefix(tab.Path, "diff:")).Font(mtheme.MonoFont).
				FontSize(caption).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
			label := "Inline"
			if *unified {
				label = "Split"
			}
			if kit.Button(c, label, kit.ButtonOptions{Variant: kit.Ghost, Icon: icons.Must("columns-2")}).Clicked() {
				*unified = !*unified
			}
		})
		mgo.Box(c).FillWidth().Grow(1).MinHeight(0).Clip().Children(func() {
			kit.DiffViewer(c, &tab.dview, tab.From, tab.To, kit.DiffViewerOptions{
				Language: tab.Lang, Unified: *unified,
			}).Fill()
		})
	})
}

// centerTermBody draws a terminal tab's shell.
func (cw *ChatWindow) centerTermBody(c *mgo.Context, tab *centerTab) {
	if tab.Term == nil {
		cw.centerLoading(c)
		return
	}
	u := mcore.Density(c).Unit()
	cw.termBody(c, tab.Term, mcore.FontSize(c, mtheme.CaptionSize), u)
}

// centerWebBody draws a web tab: its address bar and page.
func (cw *ChatWindow) centerWebBody(c *mgo.Context, tab *centerTab) {
	if tab.Web == nil {
		cw.centerLoading(c)
		return
	}
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		cw.webToolbar(c, tab.Web)
		mgo.Box(c).Grow(1).MinHeight(0).Children(func() {
			if tab.Web.URL == webLandingURL {
				cw.webLanding(c, tab.Web)
				return
			}
			mgo.WebView(c, tab.Web.view).Grow(1).MinHeight(0)
		})
	})
}

// openCenterTab adds a tab to a pane and selects it, focusing that pane.
func (cw *ChatWindow) openCenterTab(pane int, tab *centerTab) *centerTab {
	// A chat tab always lives in the leftmost pane, first in its strip, so
	// the conversation sits where it is expected whatever else is open.
	if tab.Kind == centerChat {
		pane = chatPaneIndex
		p := &cw.center.panes[pane]
		p.tabs = append([]*centerTab{tab}, p.tabs...)
		p.sel = 0
		cw.center.focus = pane
		cw.invalidate()
		return tab
	}
	if pane < 0 || pane >= cw.center.layout.paneCount() {
		pane = cw.center.focus
	}
	p := &cw.center.panes[pane]
	p.tabs = append(p.tabs, tab)
	p.sel = len(p.tabs) - 1
	cw.center.focus = pane
	cw.invalidate()
	return tab
}

// focusCenterTab selects the tab with the given ID in any pane, and reports
// whether it found one.
func (cw *ChatWindow) focusCenterTab(id string) *centerTab {
	for pi := range cw.center.visiblePanes() {
		p := &cw.center.panes[pi]
		for i, t := range p.tabs {
			if t.ID != id {
				continue
			}
			p.sel = i
			cw.center.focus = pi
			cw.invalidate()
			return t
		}
	}
	return nil
}

// closeCenterTab closes a pane's tab, releasing whatever it owns.
func (cw *ChatWindow) closeCenterTab(pane, i int) {
	if pane < 0 || pane >= maxPanes {
		return
	}
	p := &cw.center.panes[pane]
	if i < 0 || i >= len(p.tabs) {
		return
	}
	tab := p.tabs[i]
	if !tab.closable() {
		return
	}
	if tab.dirty() {
		cw.askConfirm("关闭标签", "「"+tab.Title+"」还有未保存的修改，确定关闭？", func() {
			cw.forceCloseCenterTab(pane, tab)
		})
		return
	}
	cw.forceCloseCenterTab(pane, tab)
}

// forceCloseCenterTab closes a tab without asking, freeing its resources.
func (cw *ChatWindow) forceCloseCenterTab(pane int, tab *centerTab) {
	p := &cw.center.panes[pane]
	i := p.indexOf(tab)
	if i < 0 {
		return
	}
	switch {
	case tab.Web != nil && tab.Web.view != nil:
		tab.Web.view.Destroy()
	case tab.Term != nil:
		cw.releaseTerm(tab.Term)
	}
	p.tabs = append(p.tabs[:i], p.tabs[i+1:]...)
	p.clampSel()
	cw.invalidate()
}

// closeConvTabs closes every chat tab showing a conversation, as when the
// conversation itself is deleted.
func (cw *ChatWindow) closeConvTabs(id string) {
	for _, pi := range cw.center.visiblePanes() {
		p := &cw.center.panes[pi]
		for i := len(p.tabs) - 1; i >= 0; i-- {
			if p.tabs[i].Kind == centerChat && p.tabs[i].ConvID == id {
				cw.forceCloseCenterTab(pi, p.tabs[i])
			}
		}
	}
}

// closeOtherCenterTabs closes every closable tab of the pane but the one at i.
func (cw *ChatWindow) closeOtherCenterTabs(pane, i int) {
	p := &cw.center.panes[pane]
	if i < 0 || i >= len(p.tabs) {
		return
	}
	keep := p.tabs[i]
	for _, t := range append([]*centerTab(nil), p.tabs...) {
		if t == keep || !t.closable() {
			continue
		}
		cw.forceCloseCenterTab(pane, t)
	}
	p.sel = p.indexOf(keep)
	cw.invalidate()
}

// moveCenterTab moves a tab from one pane to another.
func (cw *ChatWindow) moveCenterTab(from, i, to int) {
	if from == to || to < 0 || to >= cw.center.layout.paneCount() {
		return
	}
	src := &cw.center.panes[from]
	if i < 0 || i >= len(src.tabs) {
		return
	}
	tab := src.tabs[i]
	if !tab.closable() {
		return
	}
	// A chat tab never leaves the leftmost pane.
	if tab.Kind == centerChat && to != chatPaneIndex {
		return
	}
	src.tabs = append(src.tabs[:i], src.tabs[i+1:]...)
	src.clampSel()
	dst := &cw.center.panes[to]
	dst.tabs = append(dst.tabs, tab)
	dst.sel = len(dst.tabs) - 1
	cw.center.focus = to
	cw.invalidate()
}

// focusConvTab brings a conversation's own tab to the front, opening it in
// pane 0 when it is not open yet.
func (cw *ChatWindow) focusConvTab(conv *models.Conversation) {
	if conv == nil {
		return
	}
	id := "conv:" + conv.ID
	if t := cw.focusCenterTab(id); t != nil {
		return
	}
	cw.openCenterTab(0, &centerTab{
		Kind: centerChat, ID: id, Title: agentTabName(conv.Agent),
		ConvID: conv.ID, Agent: conv.Agent,
	})
}

// attachConvTab makes the window follow a chat tab's conversation, so the
// message area shows the right one no matter which tab is active.
func (cw *ChatWindow) attachConvTab(tab *centerTab) {
	if tab.ConvID == "" || cw.cur != nil && cw.cur.ID == tab.ConvID {
		return
	}
	conv, err := cw.convManager.LoadConversation(tab.ConvID)
	if err != nil {
		return
	}
	cw.cur = conv
	cw.mode = modeChat
	if conv.Workspace != "" {
		cw.workspace = conv.Workspace
	}
	cw.applyConversationAgent(conv)
	cw.applyConversationModel(conv)
}

// centerTabsOfKind collects the open tabs of a kind across every pane.
func (cw *ChatWindow) centerTabsOfKind(kind centerKind) []*centerTab {
	var out []*centerTab
	for _, pi := range cw.center.visiblePanes() {
		for _, t := range cw.center.panes[pi].tabs {
			if t.Kind == kind {
				out = append(out, t)
			}
		}
	}
	return out
}

// paneHoldingTab finds the pane and index of a tab, or -1, -1.
func (cw *ChatWindow) paneHoldingTab(tab *centerTab) (int, int) {
	for _, pi := range cw.center.visiblePanes() {
		if i := cw.center.panes[pi].indexOf(tab); i >= 0 {
			return pi, i
		}
	}
	return -1, -1
}

// titleOfPath names a tab after its file.
func titleOfPath(path string) string {
	if base := filepath.Base(path); base != "" && base != "." {
		return base
	}
	return path
}
