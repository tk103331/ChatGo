package ui

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	mchat "github.com/ZacharyZhang-NY/MujicaUI/chat"
	mcore "github.com/ZacharyZhang-NY/MujicaUI/core"
	"github.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "github.com/ZacharyZhang-NY/MujicaUI/input"
	moverlay "github.com/ZacharyZhang-NY/MujicaUI/overlay"
	mtheme "github.com/ZacharyZhang-NY/MujicaUI/theme"
)

// sidebar builds the brand row, new-chat action, search and conversation list.
func (cw *ChatWindow) sidebar(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().Children(func() {
		mgo.Column(c).Padding(u*2, u).Gap(u).Children(func() {
			w := minput.Button(c, "新增工作区", minput.ButtonOptions{Variant: minput.Secondary, Icon: icons.Must("folder")})
			w.FillWidth()
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

// sidebarSearch filters conversations and opens a hit.
func (cw *ChatWindow) sidebarSearch(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	mgo.Column(c).Padding(u, u*2).Children(func() {
		v := mchat.ConversationSearch(c, &cw.searchQuery, cw.searchHits(), mchat.ConversationSearchOptions{
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
func (cw *ChatWindow) searchHits() []mchat.ConversationSearchHit {
	q := strings.ToLower(strings.TrimSpace(cw.searchQuery))
	if q == "" {
		return nil
	}
	var hits []mchat.ConversationSearchHit
	for _, conv := range cw.convs {
		if strings.Contains(strings.ToLower(conv.Title), q) {
			hits = append(hits, mchat.ConversationSearchHit{ID: conv.ID, Title: conv.Title})
			continue
		}
		for _, m := range conv.Messages {
			if strings.Contains(strings.ToLower(m.Content), q) {
				hits = append(hits, mchat.ConversationSearchHit{ID: conv.ID, Title: conv.Title, Snippet: snippet(m.Content, q)})
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
	}).Grow(1).MinHeight(0)
}

func (cw *ChatWindow) groupedConvRows() []convRow {
	groups := map[string][]int{}
	order := []string{}
	seen := map[string]bool{}
	for i, conv := range cw.convs {
		ws := conv.Workspace
		if !seen[ws] {
			seen[ws] = true
			order = append(order, ws)
		}
		groups[ws] = append(groups[ws], i)
	}
	var list []string
	taken := map[string]bool{}
	for _, ws := range cw.workspaces() {
		list = append(list, ws)
		taken[ws] = true
	}
	for _, ws := range order {
		if !taken[ws] {
			list = append(list, ws)
			taken[ws] = true
		}
	}
	var rows []convRow
	for _, ws := range list {
		rows = append(rows, convRow{header: true, ws: ws})
		for _, i := range groups[ws] {
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
	mgo.Row(c).FillWidth().MinHeight(mcore.Density(c).ControlHeight()).Padding(u, u).Gap(u).
		AlignItems(mgo.Center).Background(k.Surface).Children(func() {
		mgo.Icon(c, icons.Must("folder")).Size(14, 14).TextColor(k.TextMuted).Shrink(0)
		mgo.Text(c, workspaceTitle(ws)).FontSize(caption).FontWeight(600).TextColor(k.TextMuted).
			Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Tooltip(ws)
		plus := mcore.IconAction(c, icons.Must("plus"), "新建会话")
		open := mgo.Local(plus, "agents", func() bool { return false })
		if plus.Clicked() {
			*open = !*open
		}
		moverlay.PopupMenuOpen(c, plus, open, func(m *moverlay.PopupMenu) {
			if m.Item(chatAgentName, moverlay.PopupMenuItemOptions{}) {
				cw.newConversationIn(ws, "")
			}
			for _, a := range cw.cfg.Agents {
				name := a.Name
				if m.Item(name, moverlay.PopupMenuItemOptions{}) {
					cw.newConversationIn(ws, "agent:"+name)
				}
			}
			for _, a := range cw.cfg.ACPAgents {
				if m.Item("ACP · "+a.Name, moverlay.PopupMenuItemOptions{}) {
					cw.newConversationIn(ws, "acp:"+a.Name)
				}
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
	mc := mchat.ChatConversation{ID: conv.ID, Title: conv.Title, Updated: convLast(conv)}
	selected := cw.cur != nil && cw.cur.ID == conv.ID
	v := mchat.ConversationItem(c, &mc, mchat.ConversationItemOptions{Selected: selected})
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

// exportConversation copies the conversation as Markdown to the clipboard.
func (cw *ChatWindow) exportConversation(c *mgo.Context, id string) {
	conv, err := cw.convManager.LoadConversation(id)
	if err != nil {
		cw.toast("导出失败："+err.Error(), mcore.SeverityError)
		return
	}
	msgs := make([]mchat.SessionMessage, 0, len(conv.Messages))
	for _, m := range conv.Messages {
		role := mchat.SessionUser
		switch m.Role {
		case "assistant":
			role = mchat.SessionAssistant
		case "system":
			role = mchat.SessionSystem
		}
		msgs = append(msgs, mchat.SessionMessage{Role: role, Text: m.Content, Time: m.Timestamp})
	}
	c.WriteClipboard(mchat.ExportConversation(conv.Title, msgs, mchat.ExportMarkdown))
	cw.toast("已复制「"+conv.Title+"」的 Markdown", mcore.SeveritySuccess)
}
