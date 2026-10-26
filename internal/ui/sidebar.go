package ui

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/pkg/models"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// sidebar builds the brand row, new-chat action, search and conversation list.
func (cw *ChatWindow) sidebar(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().Children(func() {
		mgo.Row(c).Padding(u*2, u).Gap(u).AlignItems(mgo.Center).Children(func() {
			w := kit.Button(c, "新增工作区", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("folder")})
			w.Grow(1)
			if w.Clicked() {
				cw.pickWorkspace()
			}
		})
		cw.sidebarSearch(c)
		if cw.searchQuery == "" {
			cw.conversationList(c)
		}
	})
}

func (cw *ChatWindow) cycleTheme() {
	switch cw.themeMode {
	case mcore.System:
		cw.themeMode = mcore.Dark
	case mcore.Dark:
		cw.themeMode = mcore.Light
	default:
		cw.themeMode = mcore.System
	}
	cw.saveAppearance()
}

// pickWorkspace opens a native directory chooser and adds the workspace.
func (cw *ChatWindow) pickWorkspace() {
	cw.winMu.Lock()
	w := cw.window
	cw.winMu.Unlock()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: w, Title: "选择工作区目录", Directory: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		cw.postMain(func() { cw.addWorkspace(paths[0]) })
	}()
}

// pickDirectory opens a native directory chooser and hands the path to cb.
func (cw *ChatWindow) pickDirectory(title string, cb func(string)) {
	cw.winMu.Lock()
	w := cw.window
	cw.winMu.Unlock()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: w, Title: title, Directory: true})
		if err != nil || len(paths) == 0 {
			return
		}
		cw.postMain(func() { cb(paths[0]) })
	}()
}

// sidebarSearch filters conversations and opens a hit.
func (cw *ChatWindow) sidebarSearch(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	mgo.Column(c).Padding(u, u*2).Children(func() {
		v := kit.ConversationSearch(c, &cw.searchQuery, cw.searchHits(), kit.ConversationSearchOptions{
			Placeholder: "搜索会话",
			Label:       "搜索会话",
		})
		if cw.searchQuery == "" {
			v.Element.Shrink(0)
		} else {
			v.Element.Grow(1).MinHeight(0)
		}
		if v.Chosen != "" {
			cw.selectConversation(v.Chosen)
			cw.searchQuery = ""
		}
	})
}

// searchHits matches the query against titles and message text.
func (cw *ChatWindow) searchHits() []kit.ConversationSearchHit {
	q := strings.ToLower(strings.TrimSpace(cw.searchQuery))
	if q == "" {
		return nil
	}
	var hits []kit.ConversationSearchHit
	for _, conv := range cw.convs {
		if strings.Contains(strings.ToLower(conv.Title), q) {
			hits = append(hits, kit.ConversationSearchHit{ID: conv.ID, Title: conv.Title})
			continue
		}
		for _, m := range conv.Messages {
			if strings.Contains(strings.ToLower(m.Content), q) {
				hits = append(hits, kit.ConversationSearchHit{ID: conv.ID, Title: conv.Title, Snippet: snippet(m.Content, q)})
				break
			}
		}
	}
	return hits
}

func snippet(text, q string) string {
	lower := strings.ToLower(text)
	if i := strings.Index(lower, q); i > 40 {
		text = "…" + text[i-40:]
	}
	if len(text) > 120 {
		text = text[:120] + "…"
	}
	return text
}

type convRow struct {
	header bool
	ws     string
	index  int
}

// conversationList shows saved conversations grouped by workspace.
func (cw *ChatWindow) conversationList(c *mgo.Context) {
	// Each row carries its workspace's git summary, and every workspace in
	// the list groups under its project's main tree. A workspace with no
	// conversation yet — one set from the worktree panel — still needs its
	// summary loaded, or it would fall back to its own path and split off
	// from the project.
	for _, ws := range cw.workspaces() {
		cw.ensureConvGit(ws)
	}
	rows := cw.groupedConvRows()
	cw.convList.Key = func(i int) any {
		if rows[i].header {
			return "w:" + rows[i].ws
		}
		return cw.convs[rows[i].index].ID
	}
	cw.convList.Label = func(i int) string {
		if rows[i].header {
			return workspaceTitle(rows[i].ws)
		}
		return cw.convs[rows[i].index].Title
	}
	cw.convList.Header = func(i int) bool { return false }
	mgo.List(c, &cw.convList, len(rows), func(i int) {
		r := rows[i]
		if r.header {
			cw.workspaceHeader(c, r.ws)
			return
		}
		cw.convRow(c, r.index)
	}).Grow(1).MinHeight(0).Padding(0, 10, 0, 0)
}

func (cw *ChatWindow) groupedConvRows() []convRow {
	// Conversations group under the main working tree of their workspace's
	// repository, so every linked tree's sessions sit together.
	groups := map[string][]int{}
	order := []string{}
	seen := map[string]bool{}
	for i, conv := range cw.convs {
		g := cw.convGitGroup(conv.Workspace)
		if !seen[g] {
			seen[g] = true
			order = append(order, g)
		}
		groups[g] = append(groups[g], i)
	}
	var list []string
	taken := map[string]bool{}
	for _, ws := range cw.workspaces() {
		g := cw.convGitGroup(ws)
		if !taken[g] {
			list = append(list, g)
			taken[g] = true
		}
	}
	for _, g := range order {
		if !taken[g] {
			list = append(list, g)
			taken[g] = true
		}
	}
	var rows []convRow
	for _, g := range list {
		rows = append(rows, convRow{header: true, ws: g})
		if cw.wsCollapsed[g] {
			continue
		}
		for _, i := range groups[g] {
			rows = append(rows, convRow{index: i})
		}
	}
	return rows
}

// workspaceHeader shows a workspace row with new-conversation and remove actions.
func (cw *ChatWindow) workspaceHeader(c *mgo.Context, ws string) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	chev := "chevron-down"
	if cw.wsCollapsed[ws] {
		chev = "chevron-right"
	}
	mgo.Row(c).FillWidth().MinHeight(mcore.Density(c).ControlHeight()*1.2).Padding(u, u).Gap(u).
		AlignItems(mgo.Center).Background(k.Background).Children(func() {
		if mcore.IconAction(c, icons.Must(chev), "折叠/展开工作区").Clicked() {
			cw.wsCollapsed[ws] = !cw.wsCollapsed[ws]
		}
		mgo.Icon(c, icons.Must("folder")).Size(14, 14).TextColor(k.Accent).Shrink(0)
		mgo.Text(c, workspaceTitle(ws)).FontSize(caption).FontWeight(700).TextColor(k.Text).
			Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Tooltip(ws)
		// A Git project's header offers worktrees beside the new-chat action:
		// create a working tree of the project or open one as the workspace.
		if cw.workspaceIsProject(ws) {
			wtBtn := mcore.IconAction(c, icons.Must("git-fork"), "新建/打开 Worktree")
			wtOpen := mgo.Local(wtBtn, "wt:"+ws, func() bool { return false })
			if wtBtn.Clicked() {
				cw.bumpConvGit(ws)
				*wtOpen = true
			}
			cw.projectWorktreeDialog(c, ws, wtOpen)
		}
		plus := mcore.IconAction(c, icons.Must("plus"), "新建会话")
		open := mgo.Local(plus, "agents", func() bool { return false })
		if plus.Clicked() {
			*open = !*open
		}
		pick := func(agent string) {
			*open = false
			cw.newConversationIn(ws, agent)
		}
		kit.Popover(c, plus, open, kit.PopoverOptions{Title: "新建会话", Width: 240}, func() {
			menuItem(c, iconLeading(c, chatAgentIcon), chatAgentName, func() { pick("") })
			for _, a := range cw.cfg.Agents {
				name := a.Name
				menuItem(c, cw.agentLeading(c, name), name, func() { pick("agent:" + name) })
			}
			for _, a := range cw.cfg.ACPAgents {
				if a.Disabled {
					continue
				}
				name := a.Name
				menuItem(c, logoLeading(c, name), name, func() { pick("acp:" + name) })
			}
		})
		if containsStr(cw.cfg.Workspaces, ws) {
			if mcore.IconAction(c, icons.Must("x"), "移除工作区").Clicked() {
				cw.removeWorkspace(ws)
			}
		}
	})
}

func workspaceTitle(ws string) string {
	if ws == "" {
		return "（未指定工作区）"
	}
	if base := filepath.Base(ws); base != "" && base != "." && base != string(filepath.Separator) {
		return base
	}
	return ws
}

// convRow renders one conversation with open, rename, export and delete actions.
func (cw *ChatWindow) convRow(c *mgo.Context, i int) {
	conv := cw.convs[i]
	mc := kit.ChatConversation{
		ID: conv.ID, Title: conv.Title, Updated: convLast(conv),
		Subtitle: cw.convGitSubtitle(conv.Workspace),
	}
	selected := cw.cur != nil && cw.cur.ID == conv.ID
	var v kit.ConversationItemView
	mgo.Row(c).FillWidth().Margin(0, 0, 0, mcore.Density(c).Unit()*3).AlignItems(mgo.Center).Children(func() {
		cw.convLeading(c, conv, 16)
		mgo.Box(c).Grow(1).MinWidth(0).Children(func() {
			v = kit.ConversationItem(c, &mc, kit.ConversationItemOptions{
				Selected: selected, Status: cw.convStatusOf(conv.ID),
			})
		})
		cw.convHoverCard(c, v.Element, conv)
	})
	switch {
	case v.Opened:
		cw.selectConversation(conv.ID)
	case v.Renamed:
		cw.convs[i].Title = mc.Title
		_ = cw.convManager.SaveConversation(&cw.convs[i])
	case v.Exported:
		cw.exportConversation(c, conv.ID)
	case v.Deleted:
		cw.askDeleteConversation(conv.ID, conv.Title)
	}
}

// convHoverCard is the card shown beside a conversation while the pointer
// rests on its row: the title, the branch and change count, the change
// breakdown, the directory and whether the workspace is the main working tree
// of its project.
func (cw *ChatWindow) convHoverCard(c *mgo.Context, item mgo.Element, conv models.Conversation) {
	if !item.Valid() {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.TooltipBase(c, item, func(tip mgo.Element) {
		tip.AttachTo(item, mgo.AnchorEnd, mgo.AnchorStart).Margin(0, 0, 0, u*2)
		tip.MaxWidth(360).Padding(u*2, u*3).Gap(u).Background(k.Surface).
			Border(mtheme.BorderWidth, k.Border).Radius(mtheme.PopupRadius).
			Shadow(0, 6, 18, 0, mgo.RGBA(0, 0, 0, 0.16))
		tip.Children(func() {
			mgo.Text(c, conv.Title).FontWeight(600).SingleLine().Ellipsis("…")
			if sub := cw.convGitSubtitle(conv.Workspace); sub != "" {
				mgo.Text(c, sub).FontSize(caption).TextColor(k.Accent).SingleLine().Ellipsis("…")
			}
			if conv.Workspace == "" {
				return
			}
			mgo.Text(c, "分析："+cw.convGitAnalysis(conv.Workspace)).
				FontSize(caption).TextColor(k.TextMuted)
			mgo.Text(c, conv.Workspace).Font(mtheme.MonoFont).FontSize(caption).
				TextColor(k.TextMuted).SingleLine().Ellipsis("…")
			label := "工作树"
			if cw.convGitMain(conv.Workspace) {
				label = "主工作树"
			}
			mgo.Text(c, label).FontSize(caption).TextColor(k.Accent).Padding(0, u/2).
				Radius(mtheme.ControlRadius).Border(mtheme.BorderWidth, k.Border).
				AlignSelf(mgo.Start).SingleLine().Shrink(0)
		})
	})
}

// convStatusOf reports the conversation's runtime phase: a waiting permission
// outranks a running reply or task, and an idle conversation shows no light.
func (cw *ChatWindow) convStatusOf(convID string) kit.ConversationStatus {
	if convID == "" {
		return kit.ConvIdle
	}
	cw.acp.mu.Lock()
	waiting := false
	if cw.acp.permissionShow && cw.acp.permissionReq != nil {
		waiting = cw.acp.convBySess[cw.acp.permissionReq.SessionId] == convID
	}
	cw.acp.mu.Unlock()
	if waiting {
		return kit.ConvWaiting
	}
	if cw.streaming && cw.streamingConvID == convID {
		return kit.ConvRunning
	}
	for _, t := range cw.cfg.AutoTasks {
		if t.ConvID == convID && cw.autoRunning[t.ID] {
			return kit.ConvRunning
		}
	}
	return kit.ConvIdle
}

// exportConversation copies the conversation as Markdown to the clipboard.
func (cw *ChatWindow) exportConversation(c *mgo.Context, id string) {
	conv, err := cw.convManager.LoadConversation(id)
	if err != nil {
		cw.toast("导出失败："+err.Error(), mcore.SeverityError)
		return
	}
	msgs := make([]kit.SessionMessage, 0, len(conv.Messages))
	for _, m := range conv.Messages {
		role := kit.SessionUser
		switch m.Role {
		case "assistant":
			role = kit.SessionAssistant
		case "system":
			role = kit.SessionSystem
		}
		msgs = append(msgs, kit.SessionMessage{Role: role, Text: m.Content, Time: m.Timestamp})
	}
	c.WriteClipboard(kit.ExportConversation(conv.Title, msgs, kit.ExportMarkdown))
	cw.toast("已复制「"+conv.Title+"」的 Markdown", mcore.SeveritySuccess)
}

// menuItem is one row of a new-conversation menu, with an optional leading mark.
func menuItem(c *mgo.Context, leading func(), label string, action func()) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	b := mgo.ButtonBase(c).FillWidth().Justify(mgo.Start).Gap(u).AlignItems(mgo.Center).
		MinHeight(mcore.Density(c).ControlHeight()).Padding(0, u*2).Radius(mtheme.ControlRadius)
	if b.Hovered() {
		b.Background(k.SurfaceHover)
	}
	b.Children(func() {
		if leading != nil {
			leading()
		}
		mgo.Text(c, label).SingleLine().Grow(1).MinWidth(0).Ellipsis("…")
	})
	if b.Clicked() {
		action()
	}
}

func iconLeading(c *mgo.Context, name string) func() {
	return func() {
		mgo.Icon(c, icons.Must(name)).Size(16, 16).TextColor(mcore.Tokens(c).Accent)
	}
}

func logoLeading(c *mgo.Context, name string) func() {
	return func() {
		acpIcon(c, name, 16, 16)
	}
}

// convLeading shows the conversation agent's logo or generated avatar at size.
func (cw *ChatWindow) convLeading(c *mgo.Context, conv models.Conversation, size float32) {
	u := mcore.Density(c).Unit()
	switch {
	case strings.HasPrefix(conv.Agent, "acp:"):
		name := strings.TrimPrefix(conv.Agent, "acp:")
		mgo.Row(c).Margin(0, 0, 0, u).Children(func() { cw.acpLeading(c, name, size) })
	case strings.HasPrefix(conv.Agent, "agent:"):
		name := strings.TrimPrefix(conv.Agent, "agent:")
		mgo.Row(c).Margin(0, 0, 0, u).Children(func() { cw.agentMarkByName(c, name, size) })
	default:
		mgo.Icon(c, icons.Must(chatAgentIcon)).Size(size, size).TextColor(mcore.Tokens(c).Accent).Margin(0, 0, 0, u)
	}
}
