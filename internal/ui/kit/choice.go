package kit

import (
	"strings"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// SelectOption is one choice of a Select.
type SelectOption[T comparable] struct {
	Value    T
	Label    string
	Disabled bool
}

// SelectOptions configure a Select.
type SelectOptions struct {
	// Placeholder shows while no option holds the value; empty uses "请选择…".
	Placeholder string
	Disabled    bool
	// Empty shows in the popup when there are no options; empty uses "无选项".
	Empty string
	// Label names the select for assistive technology.
	Label string
	// MinWidth is the trigger's minimum width in DIPs; zero uses the default.
	MinWidth float32
}

// SelectResult is what a Select built and reported this frame.
type SelectResult struct {
	Element mgo.Element
	changed bool
}

// Changed reports that the user chose another option this frame.
func (r SelectResult) Changed() bool { return r.changed }

// selectChevron draws the arrow of a select, as MyGo draws its own.
func selectChevron(c *mgo.Context, k *mgo.Theme) {
	u := uikit.Unit(c)
	color := k.TextMuted
	mgo.Box(c).Size(u*2.5, u*2.5).Shrink(0).Draw(func(p *mgo.Painter, r mgo.Rect) {
		var path mgo.Path
		path.MoveTo(r.X+r.W*0.1, r.Y+r.H*0.3).
			LineTo(r.X+r.W*0.5, r.Y+r.H*0.7).
			LineTo(r.X+r.W*0.9, r.Y+r.H*0.3)
		p.StrokePath(&path, 1.5, color)
	})
}

// Select chooses one of opts into *value, looking as MyGo's own Select does.
// MyGo's Select takes strings only, so this carries the value's type.
func Select[T comparable](c *mgo.Context, value *T, opts []SelectOption[T], o SelectOptions) SelectResult {
	if value == nil {
		panic("kit: Select needs a value")
	}
	k := uikit.Tokens(c)
	u := uikit.Unit(c)

	label := o.Placeholder
	if label == "" {
		label = "请选择…"
	}
	chosen := false
	for _, opt := range opts {
		if opt.Value == *value {
			label, chosen = opt.Label, true
			break
		}
	}

	minWidth := o.MinWidth
	if minWidth <= 0 {
		minWidth = u * 35
	}
	sel := mgo.SelectBase(c, value)
	trigger := sel.Trigger.Justify(mgo.SpaceBetween).MinWidth(minWidth).
		Padding(u*1.5, u*3.5).Gap(u * 1.5).Radius(k.Radius).
		Background(k.Surface).TextColor(k.Text).Disabled(o.Disabled)
	if o.Label != "" {
		trigger.Label(o.Label)
	}
	if trigger.Hovered() && !o.Disabled {
		trigger.Background(k.SurfaceHover)
	}
	trigger.Children(func() {
		text := mgo.Text(c, label).SingleLine()
		if !chosen {
			text.TextColor(k.TextMuted)
		}
		selectChevron(c, k)
	})

	sel.Popup(func(panel mgo.Element) {
		panel.Margin(u, 0, 0, 0).Padding(u).Radius(k.Radius+2).
			Background(k.Background).Border(theme.BorderWidth, k.Border).
			Shadow(0, 6, 20, 0, mgo.RGBA(0, 0, 0, 0.18)).
			MaxHeight(u * 72)
		if len(opts) == 0 {
			empty := o.Empty
			if empty == "" {
				empty = "无选项"
			}
			mgo.Text(c, empty).Padding(u*1.5, u*2.5).
				FontSize(uikit.FontSize(c, theme.CaptionSize)).TextColor(k.TextMuted)
			return
		}
		mgo.Scroll(c).MaxHeight(u * 64).Children(func() {
			mgo.Column(c).Children(func() {
				for _, opt := range opts {
					item := sel.Item(opt.Value).Padding(u*1.5, u*2.5).Radius(k.Radius).
						Disabled(opt.Disabled)
					switch {
					case item.Highlighted():
						item.Background(k.Accent).TextColor(k.AccentText)
					case opt.Value == *value:
						item.Background(k.Surface)
					}
					item.Children(func() { mgo.Text(c, opt.Label).SingleLine() })
				}
			})
		})
	})
	return SelectResult{Element: trigger, changed: trigger.Changed()}
}

// SearchSelectOption is one choice of a SearchSelect.
type SearchSelectOption[T comparable] struct {
	Value T
	Label string
	// Note is a second, muted line under the label; empty shows one line.
	Note string
	// Leading draws a mark before the label, as an agent's avatar or a
	// provider's logo; nil shows none. It is drawn in the row and, for the
	// chosen option, in the trigger.
	Leading func()
}

// SearchSelectOptions configure a SearchSelect.
type SearchSelectOptions[T comparable] struct {
	// Placeholder shows in the trigger while no option holds the value;
	// empty uses "请选择…".
	Placeholder string
	// SearchPlaceholder is the filter field's; empty uses "搜索…".
	SearchPlaceholder string
	// Empty shows when the filter matches nothing; empty uses "无匹配项".
	Empty string
	// Label names the select for assistive technology.
	Label string
	// CreateLabel, when set, offers the typed text as a new choice with this
	// label, and CreateValue turns that text into the created value; Created
	// reports it. Without CreateValue the typed text itself is the value.
	CreateLabel string
	CreateValue func(string) T
	Disabled    bool
	// MinWidth is the trigger's minimum width in DIPs; zero grows freely.
	MinWidth float32
	// MinHeight is the trigger's minimum height in DIPs; zero uses the
	// standard control height.
	MinHeight float32
}

// SearchSelectResult is a built SearchSelect and its events.
type SearchSelectResult[T comparable] struct {
	Element mgo.Element
	changed bool
	created T
	hasNew  bool
}

// Changed reports a pick of another option.
func (r SearchSelectResult[T]) Changed() bool { return r.changed }

// Created reports the value the user asked to create, when the selector
// offers creating.
func (r SearchSelectResult[T]) Created() (T, bool) { return r.created, r.hasNew }

// SearchSelect is a searchable dropdown: a trigger that opens a filter field
// and the options under it, for picking from a list too long to scan. It is
// the same shape as the branch selector, usable for any value.
func SearchSelect[T comparable](c *mgo.Context, value *T, opts []SearchSelectOption[T], o SearchSelectOptions[T]) SearchSelectResult[T] {
	if value == nil {
		panic("kit: SearchSelect needs a value")
	}
	var r SearchSelectResult[T]
	openKey, queryKey := searchSelectKeys(o.Label)
	open := mgo.Local(c.Root(), openKey, func() bool { return false })
	query := mgo.Local(c.Root(), queryKey, func() string { return "" })

	label := o.Placeholder
	if label == "" {
		label = "请选择…"
	}
	var leading func()
	for _, opt := range opts {
		if opt.Value == *value {
			label, leading = opt.Label, opt.Leading
			break
		}
	}
	trigger := Button(c, label, ButtonOptions{
		Variant: Secondary, Disabled: o.Disabled, Leading: leading,
	})
	if o.Label != "" {
		// The trigger shows the chosen value, so the control's own name has
		// to come from Label or assistive technology hears only the value.
		trigger.Description(o.Label)
	}
	if o.MinWidth > 0 {
		trigger.MinWidth(o.MinWidth)
	}
	if o.MinHeight > 0 {
		trigger.MinHeight(o.MinHeight)
	}
	r.Element = trigger
	if trigger.Clicked() {
		*open = !*open
		*query = ""
	}
	if !*open {
		return r
	}
	u := uikit.Unit(c)
	k := uikit.Tokens(c)
	q := strings.ToLower(strings.TrimSpace(*query))
	placeholder := o.SearchPlaceholder
	if placeholder == "" {
		placeholder = "搜索…"
	}
	empty := o.Empty
	if empty == "" {
		empty = "无匹配项"
	}
	Popover(c, trigger, open, PopoverOptions{Label: o.Label}, func() {
		// The filter field is named by its own placeholder, so two selectors
		// on screen are told apart by assistive technology and in tests.
		in := SearchInput(c, query, SearchInputOptions{Placeholder: placeholder, Label: placeholder})
		in.Element.FillWidth()
		uikit.AutoFocus(c, in.Input)
		shown := 0
		hasAny := false
		mgo.Scroll(c).MaxHeight(u * 56).Children(func() {
			mgo.Column(c).FillWidth().Gap(1).Children(func() {
				for _, opt := range opts {
					hasAny = true
					if q != "" && !strings.Contains(strings.ToLower(opt.Label), q) &&
						!strings.Contains(strings.ToLower(opt.Note), q) {
						continue
					}
					shown++
					selected := opt.Value == *value
					row := mgo.ButtonBase(c).FillWidth().MinHeight(uikit.Density(c).ControlHeight()).
						Padding(u, u*2).Gap(u).AlignItems(mgo.Center).
						Justify(mgo.Start).Radius(theme.ControlRadius).
						Cursor(mgo.CursorPointer).Selected(selected)
					switch {
					case selected:
						row.Background(k.Selection)
					case row.Hovered():
						row.Background(k.SurfaceHover)
					}
					if row.Clicked() && opt.Value != *value {
						*value, r.changed, *open = opt.Value, true, false
					}
					row.Children(func() {
						if opt.Leading != nil {
							opt.Leading()
						}
						// A Note makes the row two lines, the label above it,
						// so a long second line never crowds the first.
						mgo.Column(c).Grow(1).MinWidth(0).Gap(u / 2).Children(func() {
							mgo.Text(c, opt.Label).SingleLine().FillWidth().
								MinWidth(0).Ellipsis("…")
							if opt.Note != "" {
								mgo.Text(c, opt.Note).SingleLine().FillWidth().MinWidth(0).
									FontSize(uikit.FontSize(c, theme.CaptionSize)).
									TextColor(k.TextMuted).Ellipsis("…")
							}
						})
						if selected {
							mgo.Icon(c, icons.Must("check")).Size(14, 14).
								TextColor(k.Accent).Shrink(0)
						}
					})
				}
			})
		})
		name := strings.TrimSpace(*query)
		switch {
		case !hasAny:
			gitCaption(c, "没有可选项").Padding(u * 2)
		case shown == 0 && o.CreateLabel != "" && name != "":
			if Button(c, o.CreateLabel+" "+name, ButtonOptions{
				Variant: Ghost, Icon: icons.Must("plus"),
			}).AlignSelf(mgo.Start).Clicked() {
				if make := o.CreateValue; make != nil {
					r.created, r.hasNew, *open = make(name), true, false
				} else if typed, ok := any(name).(T); ok {
					// The typed text is the value, which only fits a string
					// choice; a non-string selector supplies CreateValue.
					r.created, r.hasNew, *open = typed, true, false
				}
			}
		case shown == 0:
			gitCaption(c, empty).Padding(u * 2)
		}
	})
	return r
}

// searchSelectKeys names the local state a SearchSelect keeps, per Label so
// two selectors on screen stay independent.
func searchSelectKeys(label string) (open, query string) {
	open, query = "chatgo.searchselect.open", "chatgo.searchselect.query"
	if label != "" {
		open += "." + label
		query += "." + label
	}
	return open, query
}
