// The desktop's model picker: the composer's model control draws a button and
// opens a searchable popover listing every model the agent may run. The chat
// composer shares it.
package ui

import (
	"strings"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/assets"
	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// modelPickGroup is one provider's models, kept together for the shared brand
// mark its rows show. An empty name groups provider-less references; a group
// reference is its own pseudo-provider.
type modelPickGroup struct {
	name   string
	typ    string // the provider's protocol, for its brand mark
	models []string
}

// logo returns the provider's brand mark, or a generic icon.
func (g modelPickGroup) logo() *mgo.SVG {
	if g.typ != "" {
		if svg := assets.ProviderLogo(g.typ); svg != nil {
			return svg
		}
	}
	return icons.Must("layout-grid")
}

// modelPickGroups groups model refs by provider, in first-appearance order.
// providerType names a provider's protocol, nil when unknown.
func modelPickGroups(refs []string, providerType func(string) string) []modelPickGroup {
	index := map[string]int{}
	out := make([]modelPickGroup, 0, 8)
	for _, ref := range refs {
		name, _ := splitModelRef(ref)
		i, ok := index[name]
		if !ok {
			i = len(out)
			index[name] = i
			typ := ""
			if providerType != nil {
				typ = providerType(name)
			}
			out = append(out, modelPickGroup{name: name, typ: typ})
		}
		out[i].models = append(out[i].models, ref)
	}
	return out
}

// modelPickGroups is modelPickGroups over the configured providers.
func (cw *ChatWindow) modelPickGroups(refs []string) []modelPickGroup {
	return modelPickGroups(refs, func(name string) string {
		if name == "" {
			return ""
		}
		if p, ok := cw.findProvider(name); ok {
			return p.Type
		}
		return ""
	})
}

// modelPickRowLabel names a row: a group's display name, else the model's own
// name without its provider prefix.
func (cw *ChatWindow) modelPickRowLabel(ref string) string {
	if isGroupModelRef(ref) {
		return cw.refLabel(ref)
	}
	_, model := splitModelRef(ref)
	if model != "" {
		return model
	}
	return ref
}

// modelPickerButton draws the composer's model control: a ghost button naming
// the current model, opening the picker popover above it.
func (cw *ChatWindow) modelPickerButton(c *mgo.Context, refs []string) {
	// A ref outside the list (the chat selection's provider with no model,
	// say) reads as the list's first entry, the way the old select showed it.
	cur := cw.currentModelRef()
	// The button names the model alone; its provider is in the picker's rows.
	label := cw.modelPickRowLabel(cur)
	if !containsStr(refs, cur) && len(refs) > 0 {
		label = cw.modelPickRowLabel(refs[0])
	}
	// The control is a plain button, like the ACP options beside it in the
	// composer's row.
	b := kit.Button(c, label, kit.ButtonOptions{Variant: kit.Secondary})
	b.MaxWidth(280).MinWidth(0)
	open := mgo.Local(b, "modelpicker.open", func() bool { return false })
	query := mgo.Local(b, "modelpicker.query", func() string { return "" })
	if b.Clicked() {
		*open = !*open
	}
	if !*open {
		return
	}
	kit.Popover(c, b, open, kit.PopoverOptions{
		Label: "选择模型", Width: 480, Up: true,
		MaxHeight: mcore.Density(c).Unit() * 120,
		// The search field is pinned above the list, so it stays put while
		// the models scroll.
		Header: func() { cw.modelPickerSearch(c, query) },
	}, func() {
		cw.modelPickerList(c, refs, query, func() { *open = false })
	})
}

// modelPickerSearch is the picker's search field, pinned above the list.
func (cw *ChatWindow) modelPickerSearch(c *mgo.Context, query *string) {
	in := kit.TextInput(c, query, kit.TextInputOptions{Placeholder: "搜索模型…", Label: "搜索模型"})
	in.Element.FillWidth()
}

// modelPickerList is the picker's model list, narrowed by the search.
func (cw *ChatWindow) modelPickerList(c *mgo.Context, refs []string, query *string, close func()) {
	groups := cw.modelPickGroups(refs)
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	current := cw.currentModelRef()
	q := strings.ToLower(strings.TrimSpace(*query))
	var rows []string
	for _, ref := range refs {
		if q != "" && !strings.Contains(strings.ToLower(cw.modelPickRowLabel(ref)), q) &&
			!strings.Contains(strings.ToLower(groupOf(ref)), q) {
			continue
		}
		rows = append(rows, ref)
	}
	if len(rows) == 0 {
		mgo.Text(c, "没有匹配的模型").FontSize(caption).TextColor(k.TextMuted).
			Padding(u * 3)
		return
	}
	mgo.Column(c).FillWidth().Gap(u * 0.5).Children(func() {
		for _, ref := range rows {
			ref := ref
			on := ref == current
			row := mgo.ButtonBase(c).FillWidth().MinHeight(mcore.Density(c).ControlHeight()).
				Padding(u, u*2).Gap(u).AlignItems(mgo.Center).Justify(mgo.Start).
				Radius(mtheme.ControlRadius).Label(cw.modelPickRowLabel(ref))
			if on {
				row.Background(k.Selection)
			} else if row.Hovered() {
				row.Background(k.SurfaceHover)
			}
			row.Children(func() {
				mgo.Icon(c, pickRowLogo(groups, ref)).Size(18, 18).Shrink(0).
					TextColor(k.TextMuted)
				mgo.Text(c, cw.modelPickRowLabel(ref)).Grow(1).MinWidth(0).
					SingleLine().Ellipsis("…")
				if !isGroupModelRef(ref) {
					if name := groupOf(ref); name != "" {
						mgo.Text(c, name).Shrink(0).MaxWidth(140).
							FontSize(caption).TextColor(k.TextMuted).
							SingleLine().Ellipsis("…")
					}
				}
				if on {
					mgo.Icon(c, icons.Must("check")).Size(16, 16).Shrink(0).
						TextColor(k.Accent)
				}
			})
			if row.Clicked() {
				cw.pickModelRef(ref)
				close()
			}
		}
	})
}

// modelPickerBody is the picker's search field and list, factored out so
// tests can drive the query directly.
func (cw *ChatWindow) modelPickerBody(c *mgo.Context, refs []string, query *string, close func()) {
	cw.modelPickerSearch(c, query)
	cw.modelPickerList(c, refs, query, close)
}

// pickModelRef applies a row's choice, the same path the old select took.
func (cw *ChatWindow) pickModelRef(ref string) {
	cw.setModelRef(ref)
	if cw.currentAgent == "" {
		cw.saveChatSelection()
	}
	cw.switchProvider(cw.provider)
	cw.invalidate()
}

// pickRowLogo finds a model's provider group for its brand mark.
func pickRowLogo(groups []modelPickGroup, ref string) *mgo.SVG {
	for _, g := range groups {
		if g.name == groupOf(ref) {
			return g.logo()
		}
	}
	return icons.Must("layout-grid")
}

// groupOf is the provider part of a model reference.
func groupOf(ref string) string {
	name, _ := splitModelRef(ref)
	return name
}
