package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/egoist/mygo/ui"

	"chatgo/internal/ui/assets"
	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// agentPickItems lists every assistant in one list, in the runner's order:
// the built-in Chat, the custom agents, then the ACP connections.
func agentPickItems(agents []agentInfo) []agentInfo {
	return append([]agentInfo{{Ref: "", Name: "Chat", Kind: "chat"}}, agents...)
}

// agentPickLabel names the agent a picker shows as chosen.
func agentPickLabel(agents []agentInfo, ref string) string {
	if ref == "" {
		return "Chat"
	}
	for _, ag := range agents {
		if ag.Ref == ref {
			return ag.Name
		}
	}
	return strings.TrimPrefix(strings.TrimPrefix(ref, "agent:"), "acp:")
}

// agentPickMark draws an assistant's mark: the chat icon, a bot icon for the
// custom agents, or the ACP connection's brand.
func agentPickMark(c *ui.Context, ag agentInfo, size float32) {
	t := uikit.Tokens(c)
	switch {
	case ag.Ref == "":
		ui.Icon(c, icons.Must("message-square")).Size(size, size).Shrink(0).TextColor(t.Accent)
	case ag.Kind == "acp":
		svg := assets.ACPLogoSVG(ag.Name)
		if svg == nil {
			ui.Icon(c, icons.Must("bot")).Size(size, size).Shrink(0).TextColor(t.TextMuted)
			return
		}
		if assets.ACPTiledIcons[assets.ACPIconKey(ag.Name)] {
			ui.Image(c, svg).Size(size, size).Shrink(0)
		} else {
			ui.Icon(c, svg).Size(size, size).Shrink(0).TextColor(t.Text)
		}
	default:
		ui.Icon(c, icons.Must("bot")).Size(size, size).Shrink(0).TextColor(t.TextMuted)
	}
}

// agentRowDetail is the muted trailing text of an agent row: an ACP
// connection's command, or an agent's own description or model count. The
// list carries what the details pane used to show.
func agentRowDetail(ag agentInfo) string {
	if ag.Ref == "" {
		return ""
	}
	if ag.Disabled {
		return "加载失败"
	}
	if ag.Description != "" {
		return ag.Description
	}
	if ag.Kind == "acp" {
		return ""
	}
	if len(ag.Models) == 0 {
		return "全部模型"
	}
	return fmt.Sprintf("%d 个模型", len(ag.Models))
}

// agentPickerOptions style the picker's trigger. The session header asks for
// a ghost control with a chevron; the 新会话 dialog fills its field with the
// same secondary look as the pickers beside it.
type agentPickerOptions struct {
	Up      bool
	Variant kit.ButtonVariant
	Wide    bool
}

// agentPickerButton draws a button naming the current agent and opens the
// picker above or below it.
func (a *app) agentPickerButton(c *ui.Context, t *ui.Theme, current *string, onPick func(string), opts agentPickerOptions) {
	buttonOpts := kit.ButtonOptions{Variant: opts.Variant}
	if !opts.Wide {
		buttonOpts.Icon = icons.Must("chevron-down")
	}
	b := kit.Button(c, agentPickLabel(a.agents, *current), buttonOpts)
	if opts.Wide {
		b.FillWidth()
	} else {
		b.MaxWidth(260).MinWidth(0)
	}
	b.Justify(ui.Start)
	open := ui.Local(b, "agentpicker.open", func() bool { return false })
	if b.Clicked() {
		*open = !*open
	}
	if !*open {
		return
	}
	query := ui.Local(b, "agentpicker.query", func() string { return "" })
	kit.Popover(c, b, open, kit.PopoverOptions{
		Width: 460, MaxHeight: 460, Up: opts.Up, Label: "选择 Agent",
		// The search field is pinned above the list, so it stays put while
		// the agents scroll.
		Header: func() { agentPickerSearch(c, query) },
	}, func() {
		a.agentPickerList(c, t, current, onPick, query, func() { *open = false })
	})
}

// agentPickerSearch is the picker's search field, pinned above the list.
func agentPickerSearch(c *ui.Context, query *string) {
	in := kit.TextInput(c, query, kit.TextInputOptions{
		Placeholder: "搜索 Agent…", Label: "搜索 Agent",
	})
	in.Element.FillWidth()
}

// agentPickerList is the picker's agent list, narrowed by the search.
func (a *app) agentPickerList(c *ui.Context, t *ui.Theme, current *string, onPick func(string), query *string, close func()) {
	items := agentPickItems(a.agents)
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	q := strings.ToLower(strings.TrimSpace(*query))
	var rows []agentInfo
	for _, ag := range items {
		detail := agentRowDetail(ag)
		if q != "" && !strings.Contains(strings.ToLower(ag.Name), q) &&
			!strings.Contains(strings.ToLower(detail), q) {
			continue
		}
		rows = append(rows, ag)
	}
	if len(rows) == 0 {
		ui.Text(c, "没有匹配的 Agent").FontSize(caption).TextColor(t.TextMuted).
			Padding(u * 3)
		return
	}
	ui.Column(c).FillWidth().Gap(u * 0.5).Children(func() {
		for _, ag := range rows {
			ag := ag
			on := ag.Ref == *current
			row := ui.ButtonBase(c).FillWidth().MinHeight(uikit.Density(c).ControlHeight()).
				Padding(u, u*2).Gap(u).AlignItems(ui.Center).Justify(ui.Start).
				Radius(mtheme.ControlRadius).Label(ag.Name)
			if ag.Disabled {
				row.Opacity(0.5)
			} else if on {
				row.Background(t.Selection)
			} else if row.Hovered() {
				row.Background(t.SurfaceHover)
			}
			row.Children(func() {
				agentPickMark(c, ag, 18)
				ui.Text(c, ag.Name).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
				if on {
					ui.Icon(c, icons.Must("check")).Size(16, 16).Shrink(0).
						TextColor(t.Accent)
				}
			})
			if row.Clicked() && !ag.Disabled {
				onPick(ag.Ref)
				close()
			}
		}
	})
}

// agentPickerBody is the picker's search field and list, factored out so
// tests can drive the query directly.
func (a *app) agentPickerBody(c *ui.Context, t *ui.Theme, current *string, onPick func(string), query *string, close func()) {
	agentPickerSearch(c, query)
	a.agentPickerList(c, t, current, onPick, query, close)
}

// ensureACPOptions fetches an ACP agent's session config options once,
// caching the answer (or the failure, so a dead CLI does not refetch every
// frame).
func (a *app) ensureACPOptions(name string) acpOptionsState {
	if a.acpOptions == nil {
		a.acpOptions = map[string]acpOptionsState{}
	}
	st := a.acpOptions[name]
	if st.loading || st.options != nil || st.err != "" {
		return st
	}
	st.loading = true
	a.acpOptions[name] = st
	go func() {
		var resp struct {
			Options []acpConfigOptionInfo `json:"options"`
		}
		err := apiGet("/api/acp-options?agent="+url.QueryEscape("acp:"+name), &resp)
		a.win.Update(func() {
			cur := a.acpOptions[name]
			cur.loading = false
			if err != nil {
				cur.err = err.Error()
			} else {
				cur.options = resp.Options
			}
			a.acpOptions[name] = cur
		})
	}()
	return st
}

// ensureModelsFor fetches one agent reference's model list for the
// new-session picker, cached per reference.
func (a *app) ensureModelsFor(agent string) modelsByAgentState {
	if a.modelsByAgent == nil {
		a.modelsByAgent = map[string]modelsByAgentState{}
	}
	st := a.modelsByAgent[agent]
	if st.loading || st.models != nil || st.err != "" {
		return st
	}
	st.loading = true
	a.modelsByAgent[agent] = st
	go func() {
		var resp struct {
			Models []modelInfo `json:"models"`
		}
		err := apiGet("/api/models?agent="+url.QueryEscape(agent), &resp)
		a.win.Update(func() {
			cur := a.modelsByAgent[agent]
			cur.loading = false
			if err != nil {
				// A failed fetch is remembered: the dialog renders every
				// frame and must not refetch on each one.
				cur.err = err.Error()
			} else {
				cur.models = resp.Models
			}
			a.modelsByAgent[agent] = cur
		})
	}()
	return st
}
