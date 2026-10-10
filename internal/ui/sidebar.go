package ui

import (
	"strings"
	"time"

	mgo "github.com/egoist/mygo/ui"

	mchat "github.com/ZacharyZhang-NY/MujicaUI/chat"
	mcore "github.com/ZacharyZhang-NY/MujicaUI/core"
	"github.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "github.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "github.com/ZacharyZhang-NY/MujicaUI/theme"
)

// sidebar builds the brand row, new-chat action, search and conversation list.
func (cw *ChatWindow) sidebar(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().Children(func() {
		mgo.Row(c).FillWidth().Padding(u * 2).Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, "ChatGo").Font(mtheme.SerifFont).FontWeight(600).Grow(1)
			cw.appearanceButton(c)
			if mcore.IconAction(c, icons.Must("settings"), "设置").Clicked() {
				cw.settings.open = true
			}
		})
		mgo.Column(c).Padding(u*2, u).Children(func() {
			b := minput.Button(c, "新对话", minput.ButtonOptions{Variant: minput.Secondary, Icon: icons.Must("plus")})
			b.FillWidth()
			if b.Clicked() {
				cw.newConversation()
			}
		})
		cw.sidebarSearch(c)
		if cw.searchQuery == "" {
			cw.conversationList(c)
		}
	})
}

// appearanceButton cycles System -> Dark -> Light and persists the choice.
func (cw *ChatWindow) appearanceButton(c *mgo.Context) {
	name := "跟随系统"
	icon := "palette"
	switch cw.themeMode {
	case mcore.Dark:
		name, icon = "深色", "moon"
	case mcore.Light:
		name, icon = "浅色", "sun"
	}
	if mcore.IconAction(c, icons.Must(icon), "外观："+name).Clicked() {
		cw.cycleTheme()
	}
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
	title  string
	index  int
}

// conversationList shows saved conversations grouped by recency.
func (cw *ChatWindow) conversationList(c *mgo.Context) {
	rows := cw.groupedConvRows()
	cw.convList.Key = func(i int) any {
		if rows[i].header {
			return "h:" + rows[i].title
		}
		return cw.convs[rows[i].index].ID
	}
	cw.convList.Label = func(i int) string {
		if rows[i].header {
			return rows[i].title
		}
		return cw.convs[rows[i].index].Title
	}
	cw.convList.Header = func(i int) bool { return rows[i].header }
	mgo.List(c, &cw.convList, len(rows), func(i int) {
		r := rows[i]
		if r.header {
			k := mcore.Tokens(c)
			mgo.Row(c).MinHeight(mcore.Density(c).ControlHeight()).Padding(0, mcore.Density(c).Unit()*2).
				AlignItems(mgo.Center).Background(k.Surface).Children(func() {
				mgo.Text(c, r.title).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted).SingleLine()
			})
			return
		}
		cw.convRow(c, r.index)
	}).Grow(1).MinHeight(0)
}

func (cw *ChatWindow) groupedConvRows() []convRow {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var today, yesterday, week, older []int
	for i, conv := range cw.convs {
		switch t := convLast(conv); {
		case !t.Before(start):
			today = append(today, i)
		case !t.Before(start.AddDate(0, 0, -1)):
			yesterday = append(yesterday, i)
		case !t.Before(start.AddDate(0, 0, -7)):
			week = append(week, i)
		default:
			older = append(older, i)
		}
	}
	var rows []convRow
	add := func(title string, idxs []int) {
		if len(idxs) == 0 {
			return
		}
		rows = append(rows, convRow{header: true, title: title})
		for _, i := range idxs {
			rows = append(rows, convRow{index: i})
		}
	}
	add("今天", today)
	add("昨天", yesterday)
	add("本周", week)
	add("更早", older)
	return rows
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
