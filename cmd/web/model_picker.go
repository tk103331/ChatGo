package main

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"chatgo/internal/ui/assets"
	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// pickGroup is one provider's models, kept together for the shared brand
// mark their rows show. An empty name groups provider-less references.
type pickGroup struct {
	name   string
	typ    string
	models []modelInfo
}

// pickLogo returns the provider's brand mark, or a generic icon.
func (g pickGroup) logo() *ui.SVG {
	if g.typ != "" {
		if svg := assets.ProviderLogo(g.typ); svg != nil {
			return svg
		}
	}
	return icons.Must("layout-grid")
}

// groupModels groups a model list by provider, in first-appearance order.
func groupModels(models []modelInfo) []pickGroup {
	index := map[string]int{}
	out := make([]pickGroup, 0, 8)
	for _, m := range models {
		i, ok := index[m.Provider]
		if !ok {
			i = len(out)
			index[m.Provider] = i
			out = append(out, pickGroup{name: m.Provider, typ: m.ProviderType})
		} else if out[i].typ == "" {
			out[i].typ = m.ProviderType
		}
		out[i].models = append(out[i].models, m)
	}
	return out
}

// currentModelLabel names the model a picker shows as chosen.
func currentModelLabel(models []modelInfo, ref string) string {
	if ref == "" {
		return "默认模型"
	}
	for _, m := range models {
		if m.Ref == ref {
			return m.Label
		}
	}
	if _, model := splitModelRef(ref); model != "" {
		return model
	}
	return ref
}

// modelInList reports whether ref is one of the list's references.
func modelInList(models []modelInfo, ref string) bool {
	for _, m := range models {
		if m.Ref == ref {
			return true
		}
	}
	return false
}

// modelPickerOptions style the picker's trigger. The composer asks for a
// ghost control with a chevron; the 新会话 dialog fills its field with the
// same secondary look as the pickers beside it.
type modelPickerOptions struct {
	Up      bool
	Variant kit.ButtonVariant
	Wide    bool
}

// modelPickerButton draws a button naming the current model and opens the
// picker above or below it.
func (a *app) modelPickerButton(c *ui.Context, t *ui.Theme, label string, models []modelInfo, current *string, onPick func(string), opts modelPickerOptions) {
	// The control is a plain button, like the other controls in the
	// composer's row.
	b := kit.Button(c, label, kit.ButtonOptions{Variant: opts.Variant})
	if opts.Wide {
		b.FillWidth()
	} else {
		b.MaxWidth(260).MinWidth(0)
	}
	b.Justify(ui.Start)
	open := ui.Local(b, "modelpicker.open", func() bool { return false })
	if b.Clicked() {
		*open = !*open
	}
	a.modelPickerPopover(c, t, b, open, models, current, onPick, opts.Up)
}

// modelPickerPopover draws the picker around its anchor: one searchable list
// of every model the agent may run. The query lives on the anchor, so two
// pickers do not share state.
func (a *app) modelPickerPopover(c *ui.Context, t *ui.Theme, anchor ui.Element, open *bool, models []modelInfo, current *string, onPick func(string), up bool) {
	if !*open {
		return
	}
	query := ui.Local(anchor, "modelpicker.query", func() string { return "" })
	kit.Popover(c, anchor, open, kit.PopoverOptions{
		Width: 480, MaxHeight: 460, Up: up, Label: "选择模型",
		// The search field is pinned above the list, so it stays put while
		// the models scroll.
		Header: func() { modelPickerSearch(c, query) },
	}, func() {
		a.modelPickerList(c, t, models, current, onPick, query, func() { *open = false })
	})
}

// modelPickerBody is the picker's content, factored out so tests can render it
// without an anchor.
// modelPickerSearch is the picker's search field, pinned above the list.
func modelPickerSearch(c *ui.Context, query *string) {
	in := kit.TextInput(c, query, kit.TextInputOptions{
		Placeholder: "搜索模型…", Label: "搜索模型",
	})
	in.Element.FillWidth()
}

// modelPickerList is the picker's model list, narrowed by the search.
func (a *app) modelPickerList(c *ui.Context, t *ui.Theme, models []modelInfo, current *string, onPick func(string), query *string, close func()) {
	groups := groupModels(models)
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	q := strings.ToLower(strings.TrimSpace(*query))
	var rows []modelInfo
	for _, m := range models {
		if q != "" && !strings.Contains(strings.ToLower(m.Label), q) &&
			!strings.Contains(strings.ToLower(m.Provider), q) {
			continue
		}
		rows = append(rows, m)
	}
	if len(rows) == 0 {
		ui.Text(c, "没有匹配的模型").FontSize(caption).TextColor(t.TextMuted).
			Padding(u * 3)
		return
	}
	ui.Column(c).FillWidth().Gap(u * 0.5).Children(func() {
		for _, m := range rows {
			m := m
			on := m.Ref == *current
			row := ui.ButtonBase(c).FillWidth().MinHeight(uikit.Density(c).ControlHeight()).
				Padding(u, u*2).Gap(u).AlignItems(ui.Center).Justify(ui.Start).
				Radius(mtheme.ControlRadius).Label(m.Label + " " + m.Provider)
			if on {
				row.Background(t.Selection)
			} else if row.Hovered() {
				row.Background(t.SurfaceHover)
			}
			row.Children(func() {
				ui.Icon(c, pickRowLogo(groups, m)).Size(18, 18).Shrink(0).
					TextColor(t.TextMuted)
				ui.Text(c, m.Label).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
				if m.Provider != "" {
					ui.Text(c, m.Provider).Shrink(0).MaxWidth(140).
						FontSize(caption).TextColor(t.TextMuted).
						SingleLine().Ellipsis("…")
				}
				if on {
					ui.Icon(c, icons.Must("check")).Size(16, 16).Shrink(0).
						TextColor(t.Accent)
				}
			})
			if row.Clicked() {
				onPick(m.Ref)
				close()
			}
		}
	})
}

// modelPickerBody is the picker's search field and list, factored out so
// tests can render it without an anchor.
func (a *app) modelPickerBody(c *ui.Context, t *ui.Theme, models []modelInfo, current *string, onPick func(string), query *string, close func()) {
	modelPickerSearch(c, query)
	a.modelPickerList(c, t, models, current, onPick, query, close)
}

// pickRowLogo finds a model's provider group for its brand mark.
func pickRowLogo(groups []pickGroup, m modelInfo) *ui.SVG {
	for _, g := range groups {
		if g.name == m.Provider {
			return g.logo()
		}
	}
	return icons.Must("layout-grid")
}
