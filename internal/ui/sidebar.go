package ui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
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
			w := kit.Button(c, "新会话", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("message-square")})
			w.Grow(1)
			if w.Clicked() {
				cw.openCreateTab()
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
		cw.postMain(func() {
			cw.addWorkspace(paths[0])
			// A directory picked here becomes the active project at once, and
			// with it the center tabs and git surfaces of that directory.
			cw.pointAtWorkspace(paths[0])
		})
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
	// sub marks a working tree's own header under its project's, indented so
	// the project keeps one group while each tree stays apart; first then
	// says it is the project's main tree, drawn right under the project.
	sub   bool
	first bool
	ws    string
	// key names the header's collapse state, which differs from ws for the
	// first tree of a labelled project.
	key string
	// branchShown marks a session whose branch its group header already
	// names, so the row leaves it out.
	branchShown bool
	index       int
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
			// A tree's header shares its path with the project's own header
			// when the tree is the main one, so the kind is part of the key.
			if rows[i].sub {
				return "wt:" + rows[i].ws
			}
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
			cw.workspaceHeaderOf(c, r)
			return
		}
		cw.convRow(c, r.index, r.branchShown)
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
	// Projects keep the order they were added to the configuration, so the
	// sidebar stays where the user left it: neither switching projects nor
	// running a session moves a group up the list.
	var list []string
	taken := map[string]bool{}
	for _, ws := range cw.cfg.Workspaces {
		g := cw.convGitGroup(ws)
		if g != "" && !taken[g] {
			list = append(list, g)
			taken[g] = true
		}
	}
	// A project holding sessions but absent from the configuration — opened
	// once and never saved — follows, in the order its sessions are listed.
	for _, g := range order {
		if !taken[g] {
			list = append(list, g)
			taken[g] = true
		}
	}
	var rows []convRow
	for _, g := range list {
		// The project header's collapse state lives under the group key: the
		// header toggles cw.wsCollapsed[key], so the same key must be what the
		// row below checks, or clicking the chevron writes one key and the
		// list reads another and nothing folds.
		rows = append(rows, convRow{header: true, ws: g, key: g})
		if cw.wsCollapsed[g] {
			continue
		}
		// A project's working trees are its own directories, so each tree
		// holding sessions gets a header of its own under the project; a
		// tree with none is not listed, and a project with a single tree
		// keeps its sessions directly under its own header.
		members := map[string][]int{}
		for _, i := range groups[g] {
			ws := cw.convs[i].Workspace
			members[ws] = append(members[ws], i)
		}
		trees := cw.groupTrees(g, members)
		labelled := len(trees) > 1
		for ti, t := range trees {
			if labelled {
				key := cw.treeCollapseKey(t, ti)
				rows = append(rows, convRow{header: true, ws: t, sub: true, first: ti == 0, key: key})
				if cw.wsCollapsed[key] {
					continue
				}
			}
			for _, i := range members[t] {
				rows = append(rows, convRow{index: i, branchShown: labelled})
			}
		}
	}
	return rows
}

// groupTrees lists the working trees that hold sessions of the project keyed
// by g, in the order git reports them — the main tree first, then the linked
// ones as they were created. The order is deliberately independent of when a
// session last ran, so a tree does not jump up the sidebar just because it
// was used; its place stays where the user learned to find it.
func (cw *ChatWindow) groupTrees(g string, members map[string][]int) []string {
	var trees []string
	seen := map[string]bool{}
	for _, t := range cw.projectWorktrees(g) {
		if len(members[t]) == 0 || seen[t] {
			continue
		}
		seen[t] = true
		trees = append(trees, t)
	}
	// A tree git no longer reports — one removed behind the app's back —
	// still has sessions to show, so it follows in path order.
	var rest []string
	for ws := range members {
		if ws == "" || seen[ws] {
			continue
		}
		rest = append(rest, ws)
	}
	sort.Strings(rest)
	return append(trees, rest...)
}

// projectWorktrees is the tree list git reported for the project, with its
// main tree first. It falls back to the group path alone before the summary
// has loaded.
func (cw *ChatWindow) projectWorktrees(g string) []string {
	if p := cw.convGitProject(g); p != nil && len(p.Worktrees) > 0 {
		return p.Worktrees
	}
	return []string{g}
}

// treeCollapseKey names a tree header's collapse state. The main tree's header
// sits right under the project's, which is keyed by the same path, so the
// first tree of a labelled project takes a key of its own.
func (cw *ChatWindow) treeCollapseKey(ws string, rank int) string {
	if rank == 0 {
		return "tree:" + ws
	}
	return ws
}

// workspaceHeaderOf shows a workspace row with a new-conversation action.
//
// A project is named by its directory and marked with an accent folder. A
// working tree under it is named by the branch it has checked out, not by its
// directory: the main tree's directory is the project's own, so showing the
// path would repeat the project's name on the line below it.
func (cw *ChatWindow) workspaceHeaderOf(c *mgo.Context, r convRow) {
	ws, sub := r.ws, r.sub
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	chev := "chevron-down"
	if cw.wsCollapsed[r.key] {
		chev = "chevron-right"
	}
	if sub {
		cw.worktreeHeaderRow(c, r, chev)
		return
	}
	mgo.Row(c).FillWidth().MinHeight(mcore.Density(c).ControlHeight()*1.1).Padding(u/2, u).Gap(u / 2).
		AlignItems(mgo.Center).Children(func() {
		if mcore.IconAction(c, icons.Must(chev), "折叠/展开项目").Clicked() {
			cw.wsCollapsed[r.key] = !cw.wsCollapsed[r.key]
		}
		mgo.Icon(c, icons.Must("folder")).Size(14, 14).TextColor(k.Accent).Shrink(0)
		mgo.Text(c, workspaceTitle(ws)).FontSize(caption).FontWeight(700).TextColor(k.Text).
			Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Tooltip(ws)
		if mcore.IconAction(c, icons.Must("plus"), "新建会话").Clicked() {
			cw.openCreateTabIn(ws)
		}
	})
}

// worktreeHeaderRow draws one working tree under its project: the branch it
// has checked out, indented and muted, so a project with several trees reads
// as one group split by branch.
func (cw *ChatWindow) worktreeHeaderRow(c *mgo.Context, r convRow, chev string) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Row(c).FillWidth().MinHeight(mcore.Density(c).ControlHeight()).Padding(0, u, 0, u*3).Gap(u / 2).
		AlignItems(mgo.Center).Children(func() {
		if mcore.IconAction(c, icons.Must(chev), "折叠/展开工作树").Clicked() {
			cw.wsCollapsed[r.key] = !cw.wsCollapsed[r.key]
		}
		mgo.Icon(c, icons.Must("git-branch")).Size(12, 12).TextColor(k.TextMuted).Shrink(0)
		mgo.Text(c, cw.worktreeHeaderLabel(r.ws)).FontSize(caption).TextColor(k.TextMuted).
			Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Tooltip(r.ws)
		// The change counts belong to the tree, not to each session in it, so
		// they are summarised once here.
		cw.worktreeDiffSummary(c, r.ws)
		if mcore.IconAction(c, icons.Must("plus"), "新建会话").Clicked() {
			cw.openCreateTabIn(r.ws)
		}
	})
}

// worktreeDiffSummary shows a tree's uncommitted line counts, drawn only when
// the tree has changes.
func (cw *ChatWindow) worktreeDiffSummary(c *mgo.Context, ws string) {
	added, deleted := cw.convGitDiff(ws)
	if added == 0 && deleted == 0 {
		return
	}
	k := mcore.Tokens(c)
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Text(c, "+"+shortLineCount(added)).Font(mtheme.MonoFont).
		FontSize(caption).TextColor(k.Success).Shrink(0)
	mgo.Text(c, "-"+shortLineCount(deleted)).Font(mtheme.MonoFont).
		FontSize(caption).TextColor(k.Danger).Shrink(0)
}

// shortLineCount abbreviates a line count the way the conversation rows do,
// so the sidebar reads the same whichever row carries the counts.
func shortLineCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fm", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	}
	return strconv.Itoa(n)
}

// worktreeHeaderLabel is the branch a tree has checked out, falling back to
// the directory when the summary has not loaded yet.
func (cw *ChatWindow) worktreeHeaderLabel(ws string) string {
	if branch := cw.convGitSubtitle(ws); branch != "" {
		return branch
	}
	return workspaceTitle(ws)
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
func (cw *ChatWindow) convRow(c *mgo.Context, i int, branchShown bool) {
	conv := cw.convs[i]
	branch := cw.convGitSubtitle(conv.Workspace)
	added, deleted := cw.convGitDiff(conv.Workspace)
	subtitle := branch
	// A tree header above the row already names the branch and summarises the
	// tree's changes, so the row drops both rather than saying them twice.
	if branchShown {
		subtitle, added, deleted = "", 0, 0
	}
	mc := kit.ChatConversation{
		ID: conv.ID, Title: conv.Title, Updated: convLast(conv),
		Subtitle: subtitle, DiffAdded: added, DiffDeleted: deleted,
		Workspace: conv.Workspace, Branch: branch,
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
	case v.CopyPath:
		c.WriteClipboard(conv.Workspace)
	case v.CopyBranch:
		if mc.Branch != "" {
			c.WriteClipboard(mc.Branch)
		}
	case v.Reveal:
		revealPath(conv.Workspace)
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
	if cw.streamingConv(convID) {
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
