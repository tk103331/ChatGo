// The desktop's agent picker pieces: a rail categorising every assistant by
// Agent — Chat, each custom agent, each ACP connection, each with its own
// mark — and a searchable list beside it.
package ui

import (
	"fmt"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// agentPickItem is one row of the agent picker.
type agentPickItem struct {
	ref      string // "", "agent:Name" or "acp:Name"
	label    string
	detail   string
	disabled bool
}

// agentPickItems lists every assistant in the picker's order: the built-in
// Chat, the custom agents, then the enabled ACP connections.
func (cw *ChatWindow) agentPickItems() []agentPickItem {
	out := []agentPickItem{{
		ref: "", label: chatAgentName, detail: "内置助手，可用全部模型与工具",
	}}
	for _, a := range cw.cfg.Agents {
		detail := a.Description
		if detail == "" {
			detail = agentPickModelsSummary(len(a.Models))
		}
		out = append(out, agentPickItem{ref: "agent:" + a.Name, label: a.Name, detail: detail})
	}
	for _, conn := range cw.cfg.ACPAgents {
		if conn.Disabled {
			continue
		}
		cmd := strings.TrimSpace(conn.Command + " " + strings.Join(conn.Args, " "))
		detail := cmd
		disabled := cw.acpModelError(conn.Name) != ""
		if disabled {
			detail = "加载失败"
		}
		out = append(out, agentPickItem{
			ref: "acp:" + conn.Name, label: conn.Name, detail: detail, disabled: disabled,
		})
	}
	return out
}

// agentPickModelsSummary words a whitelist size for a row's detail.
func agentPickModelsSummary(n int) string {
	if n == 0 {
		return "全部模型"
	}
	if n == 1 {
		return "1 个模型"
	}
	return fmt.Sprintf("%d 个模型", n)
}

// agentPickLabel names the agent a picker shows as chosen.
func agentPickLabel(ref string) string {
	if ref == "" {
		return chatAgentName
	}
	return strings.TrimPrefix(strings.TrimPrefix(ref, "agent:"), "acp:")
}

// agentPickMark draws an assistant's mark: the chat icon, the agent's own
// avatar, or the ACP connection's brand.
func (cw *ChatWindow) agentPickMark(c *mgo.Context, ref string, size float32) {
	switch {
	case ref == "":
		mgo.Icon(c, icons.Must(chatAgentIcon)).Size(size, size).Shrink(0).
			TextColor(mcore.Tokens(c).Accent)
	case strings.HasPrefix(ref, "acp:"):
		cw.acpLeading(c, strings.TrimPrefix(ref, "acp:"), size)
	default:
		cw.agentMarkByName(c, strings.TrimPrefix(ref, "agent:"), size)
	}
}

// agentPickerButton draws an agent control: a button naming the current agent,
// opening the searchable picker popover. The model stays in its own picker.
func (cw *ChatWindow) agentPickerButton(c *mgo.Context, current *string, onPick func(string), width float32) {
	// The trigger matches the project and worktree pickers beside it in the
	// 新会话 row: same emphasis, width and height, no leading mark.
	b := kit.Button(c, agentPickLabel(*current), kit.ButtonOptions{Variant: kit.Secondary})
	b.MinWidth(width).MaxWidth(width).MinHeight(createPickerHeight).
		Justify(mgo.Start)
	open := mgo.Local(b, "agentpicker.open", func() bool { return false })
	query := mgo.Local(b, "agentpicker.query", func() string { return "" })
	if b.Clicked() {
		*open = !*open
	}
	if !*open {
		return
	}
	kit.Popover(c, b, open, kit.PopoverOptions{
		Label: "选择 Agent", Width: 460, Up: true,
		MaxHeight: mcore.Density(c).Unit() * 120,
		// The search field is pinned above the list, so it stays put while
		// the agents scroll.
		Header: func() { cw.agentPickerSearch(c, query) },
	}, func() {
		cw.agentPickerList(c, current, onPick, query, func() { *open = false })
	})
}

// agentPickerSearch is the picker's search field, pinned above the list.
func (cw *ChatWindow) agentPickerSearch(c *mgo.Context, query *string) {
	in := kit.TextInput(c, query, kit.TextInputOptions{Placeholder: "搜索 Agent…", Label: "搜索 Agent"})
	in.Element.FillWidth()
}

// agentPickerList is the picker's agent list, narrowed by the search.
func (cw *ChatWindow) agentPickerList(c *mgo.Context, current *string, onPick func(string), query *string, close func()) {
	items := cw.agentPickItems()
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	q := strings.ToLower(strings.TrimSpace(*query))
	var rows []agentPickItem
	for _, it := range items {
		if q != "" && !strings.Contains(strings.ToLower(it.label), q) &&
			!strings.Contains(strings.ToLower(it.detail), q) {
			continue
		}
		rows = append(rows, it)
	}
	if len(rows) == 0 {
		mgo.Text(c, "没有匹配的 Agent").FontSize(caption).TextColor(k.TextMuted).
			Padding(u * 3)
		return
	}
	mgo.Column(c).FillWidth().Gap(u * 0.5).Children(func() {
		for _, it := range rows {
			it := it
			on := it.ref == *current
			row := mgo.ButtonBase(c).FillWidth().MinHeight(mcore.Density(c).ControlHeight()).
				Padding(u, u*2).Gap(u).AlignItems(mgo.Center).Justify(mgo.Start).
				Radius(mtheme.ControlRadius).Label(it.label)
			if it.disabled {
				row.Opacity(0.5)
			} else if on {
				row.Background(k.Selection)
			} else if row.Hovered() {
				row.Background(k.SurfaceHover)
			}
			row.Children(func() {
				cw.agentPickMark(c, it.ref, u*4.5)
				mgo.Text(c, it.label).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
				if on {
					mgo.Icon(c, icons.Must("check")).Size(u*4, u*4).Shrink(0).
						TextColor(k.Accent)
				}
			})
			if row.Clicked() && !it.disabled {
				onPick(it.ref)
				close()
			}
		}
	})
}

// agentPickerBody is the picker's search field and list, factored out for
// tests.
func (cw *ChatWindow) agentPickerBody(c *mgo.Context, current *string, onPick func(string), query *string, close func()) {
	cw.agentPickerSearch(c, query)
	cw.agentPickerList(c, current, onPick, query, close)
}
