package kit

import (
	"slices"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// SlashCommand is one command of a SlashCommandMenu.
type SlashCommand struct {
	// Name is typed after the slash; required, without spaces.
	Name        string
	Description string
}

// SlashCommandResult carries the popup and chosen command.
type SlashCommandResult struct {
	Element mgo.Element
	chosen  SlashCommand
	ok      bool
}

// Chosen returns the command picked by Enter or a click.
func (r SlashCommandResult) Chosen() (SlashCommand, bool) { return r.chosen, r.ok }

// SlashCommandMenu filters commands after a slash trigger. Arrows navigate;
// Enter chooses; Tab completes; Escape dismisses.
func SlashCommandMenu(c *mgo.Context, composer InputResult, value *string, commands []SlashCommand) SlashCommandResult {
	if !composer.Element.Valid() || !composer.Input.Valid() || value == nil {
		panic("kit: SlashCommandMenu needs composer and value")
	}
	data := mgo.Local(composer.Element, "slash-data", func() slashMenuData { return slashMenuData{} })
	query, typing := strings.CutPrefix(*value, "/")
	typing = typing && !strings.ContainsAny(query, " \t\n")
	if !data.built || query != data.query || typing != data.typing || !slices.Equal(data.commands, commands) {
		for _, cmd := range commands {
			if cmd.Name == "" || strings.ContainsAny(cmd.Name, " \t\n") {
				panic("kit: SlashCommand Name must be one word")
			}
		}
		data.built, data.query, data.typing = true, query, typing
		data.commands = append(data.commands[:0], commands...)

		data.shown, data.rows = data.shown[:0], data.rows[:0]
		if typing {
			q := strings.ToLower(query)
			// Prefix matches come before the rest, each in the caller's order.
			for pass := range 2 {
				for _, cmd := range commands {
					n := strings.ToLower(cmd.Name)
					if pass == 0 && strings.HasPrefix(n, q) || pass == 1 && !strings.HasPrefix(n, q) && strings.Contains(n, q) {
						data.shown = append(data.shown, cmd)
						data.rows = append(data.rows, slashMenuRow{label: "/" + cmd.Name, detail: cmd.Description})
					}
				}
			}
		}
	}
	var r SlashCommandResult
	st := mgo.Local(composer.Element, slashMenuKey{"slash"}, func() slashMenuState { return slashMenuState{} })
	shown, rows := data.shown, data.rows
	i, how, panel := slashMenuShow(c, composer, st, query, typing, rows,
		slashMenuOptions{empty: "无匹配命令", label: "命令"})
	r.Element = panel
	if i >= 0 {
		*value = "/" + shown[i].Name + " "
		if how == slashMenuChose {
			r.chosen, r.ok = shown[i], true
		}
	}
	return r
}

// ContextKind is what a piece of context is.
type ContextKind int

const (
	ContextFile ContextKind = iota
	ContextDoc
	ContextWeb
)

func (k ContextKind) icon() string {
	switch k {
	case ContextFile:
		return "file"
	case ContextDoc:
		return "file-text"
	case ContextWeb:
		return "globe"
	}
	panic("kit: unknown ContextKind")
}

func (k ContextKind) group() string {
	return [...]string{"文件", "文档", "网页"}[k]
}

// ContextItem describes a file, document or webpage mention.
type ContextItem struct {
	// ID is the item's stable identity; required.
	ID     string
	Label  string
	Detail string
	Kind   ContextKind
}

// ContextMentionOptions configure a ContextMentionMenu.
type ContextMentionOptions struct {
	// Loading indicates caller-owned asynchronous fetching.
	Loading bool
}

// ContextMentionResult carries the popup, query and chosen item.
type ContextMentionResult struct {
	Element      mgo.Element
	query        string
	queryChanged bool
	chosen       ContextItem
	ok           bool
}

// Query returns mention text; QueryChanged requests fresh results.
func (r ContextMentionResult) Query() string      { return r.query }
func (r ContextMentionResult) QueryChanged() bool { return r.queryChanged }

// Chosen returns the selected item and consumes its trigger.
func (r ContextMentionResult) Chosen() (ContextItem, bool) { return r.chosen, r.ok }

// ContextMentionMenu groups results for the active mention query. The caller
// filters results synchronously or asynchronously.
func ContextMentionMenu(c *mgo.Context, composer InputResult, value *string, items []ContextItem, opts ContextMentionOptions) ContextMentionResult {
	if !composer.Element.Valid() || !composer.Input.Valid() || value == nil {
		panic("kit: ContextMentionMenu needs composer and value")
	}
	data := mgo.Local(composer.Element, "mention-data", func() mentionMenuData { return mentionMenuData{} })
	if !data.built || !slices.Equal(data.items, items) {
		data.built = true
		data.items = append(data.items[:0], items...)
		data.rows, data.order = data.rows[:0], data.order[:0]

		for kind := ContextFile; kind <= ContextWeb; kind++ {
			for i, it := range items {
				if it.ID == "" || it.Label == "" {
					panic("kit: ContextItem needs ID and Label")
				}
				if it.Kind == kind {
					data.rows = append(data.rows, slashMenuRow{
						label: it.Label, detail: it.Detail,
						icon: kind.icon(), group: kind.group(),
					})
					data.order = append(data.order, i)
				}
			}
		}
		if len(data.order) != len(items) {
			panic("kit: unknown ContextKind")
		}
	}
	rows, order := data.rows, data.order
	st := mgo.Local(composer.Element, slashMenuKey{"mention"}, func() slashMenuState { return slashMenuState{} })
	at := contextMentionAt(*value)
	var r ContextMentionResult
	if at >= 0 {
		r.query = (*value)[at+1:]
	}
	// Opening or changing queries requests caller-owned results.
	r.queryChanged = at >= 0 && !composer.Element.IsDisabled() && composer.Input.Focused() &&
		(r.query != st.query || !st.open && st.dismissed == nil)
	empty := "无匹配内容"
	if opts.Loading {
		empty = "加载中…"
	}
	i, _, panel := slashMenuShow(c, composer, st, r.query, at >= 0, rows,
		slashMenuOptions{empty: empty, label: "上下文", loading: opts.Loading})
	r.Element = panel
	if i >= 0 {
		r.chosen, r.ok = items[order[i]], true
		*value = (*value)[:at]
	}
	return r
}

// contextMentionAt returns where the trailing "@query" starts, or -1.
func contextMentionAt(s string) int {
	at := strings.LastIndexByte(s, '@')
	if at < 0 || strings.ContainsAny(s[at+1:], " \t\n") || at > 0 && !strings.ContainsRune(" \t\n", rune(s[at-1])) {
		return -1
	}
	return at
}

type mentionMenuData struct {
	built bool
	items []ContextItem
	rows  []slashMenuRow
	order []int
}

type slashMenuData struct {
	commands, shown []SlashCommand
	query           string
	built, typing   bool
	rows            []slashMenuRow
}

type slashMenuKey struct{ name string }

// slashMenuRow represents an item or titled group.
type slashMenuRow struct {
	label, detail, icon, group string
}

type slashMenuOptions struct {
	empty, label string
	loading      bool
}

// slashMenuState tracks navigation, dismissal and pending choices.
type slashMenuState struct {
	hi, lastHi int
	query      string
	dismissed  *string
	pick       slashMenuHow
	held, open bool
	list       mgo.ListState
}

type slashMenuHow int

const (
	slashMenuNone slashMenuHow = iota
	slashMenuChose
	slashMenuCompleted
)

// slashMenuShow returns the selected row and activation method.
func slashMenuShow(c *mgo.Context, composer InputResult, st *slashMenuState, query string, open bool, rows []slashMenuRow, o slashMenuOptions) (int, slashMenuHow, mgo.Element) {
	if !open {
		st.dismissed = nil
	}
	if query != st.query {
		st.hi, st.query = 0, query
		st.list.ScrollTo(0, mgo.Start)
		if st.dismissed != nil && *st.dismissed != query {
			st.dismissed = nil
		}
	}
	// Panel presses preserve the menu while transferring focus.
	open = open && st.dismissed == nil && !composer.Element.IsDisabled() &&
		(composer.Input.Focused() || st.held)
	pick, how := -1, slashMenuNone
	st.open = open
	if !open {
		st.pick = slashMenuNone
		return pick, how, mgo.Element{}
	}
	st.hi = min(max(st.hi, 0), len(rows)-1)
	if st.pick != slashMenuNone && st.hi >= 0 {
		pick, how = st.hi, st.pick
	}
	st.pick = slashMenuNone
	n := len(rows)
	promptComposerHook(composer, func(ev mgo.InputEvent) bool {
		if ev.Kind != mgo.InputKeyDown || ev.Mods != 0 {
			return false
		}
		switch ev.Key {
		case mgo.KeyDown, mgo.KeyUp:
			if n > 0 {
				d := map[mgo.Key]int{mgo.KeyDown: 1, mgo.KeyUp: -1}[ev.Key]
				st.hi = (max(st.hi, 0) + d + n) % n
			}
		case mgo.KeyEnter:
			st.pick = slashMenuChose
		case mgo.KeyTab:
			st.pick = slashMenuCompleted
		case mgo.KeyEscape:
			q := query
			st.dismissed = &q
		default:
			return false
		}
		c.Invalidate()
		return true
	})
	i, panel := slashMenuPanel(c, composer.Element.Bounds(), st, rows, o)
	if i >= 0 {
		pick, how = i, slashMenuChose
	}
	if pick >= 0 {
		composer.Input.Focus()
	}
	return pick, how, panel
}

// slashMenuPanel builds visible rows and returns clicked choices.
func slashMenuPanel(c *mgo.Context, a mgo.Rect, st *slashMenuState, rows []slashMenuRow, o slashMenuOptions) (int, mgo.Element) {
	k := uikit.Tokens(c)
	d := uikit.Density(c)
	u := d.Unit()
	rowH := d.ControlHeight()
	headH := rowH * 0.75
	h := 2 * u
	group := ""
	for _, r := range rows {
		if r.group != group {
			h, group = h+headH, r.group
		}
		h += rowH
	}
	if len(rows) == 0 {
		h += rowH
	}
	ww, wh := c.Size()
	h = min(h+2*theme.BorderWidth, 280, max(0, wh-8))
	w := min(max(a.W*0.6, 320), max(a.W, 200), max(0, ww-8))
	x, y := uikit.PlaceOverlay(c, a, w, h, u)
	clicked, pressed := -1, false
	var result mgo.Element
	mgo.Overlay(c, func() {
		panel := mgo.Column(c).Absolute().Left(x).Top(y).Width(w).Height(h).
			Role(mgo.RoleList).Label(o.label)
		uikit.OverlayStyle(c, panel, uikit.OverlayAnchored(c, panel, a))
		result = panel
		panel.HandleInput(func(ev mgo.InputEvent) bool {
			if ev.Kind == mgo.InputPointerDown {
				st.held = true
			}
			return false
		})
		panel.Children(func() {
			if st.lastHi != st.hi {
				st.lastHi = st.hi
				st.list.ScrollIntoView(max(st.hi, 0))
			}
			mgo.List(c, &st.list, max(1, len(rows)), func(i int) {
				if len(rows) == 0 {
					mgo.Row(c).MinHeight(rowH).Padding(0, u*3).Gap(u * 2).
						AlignItems(mgo.Center).TextColor(k.TextMuted).Children(func() {
						if o.loading {
							uikit.Spinner(c, uikit.SpinnerOptions{Size: 14})
						}
						mgo.Text(c, o.empty).SingleLine()
					})
					return
				}
				r := rows[i]
				mgo.Column(c).Children(func() {
					if r.group != "" && (i == 0 || r.group != rows[i-1].group) {
						mgo.Text(c, r.group).Height(headH).Padding(u*2, u*3, 0, u*3).
							FontSize(uikit.FontSize(c, theme.CaptionSize)).
							FontWeight(600).TextColor(k.TextMuted).Role(mgo.RoleText)
					}
					row := mgo.Row(c.Key(i)).MinHeight(rowH).Padding(0, u*3, 0, u*2).Gap(u * 2).
						AlignItems(mgo.Center).Radius(theme.ControlRadius).
						Role(mgo.RoleListItem).Label(r.label).Cursor(mgo.CursorPointer).
						Selected(i == st.hi).TextColor(k.Text)
					if row.Clicked() {
						clicked = i
					}
					pressed = pressed || row.Pressed()
					if i == st.hi {
						accent := k.AccentText
						row.Background(k.Selection).Draw(func(p *mgo.Painter, b mgo.Rect) {
							p.Fill(mgo.Rect{X: b.X, Y: b.Y + 4, W: 2, H: b.H - 8}, accent, 1)
						})
					} else if row.Hovered() {
						row.Background(k.SurfaceHover)
					}
					row.Children(func() {
						if r.icon != "" {
							mgo.Icon(c, icons.Must(r.icon)).Size(16, 16).Shrink(0).TextColor(k.TextMuted)
						}
						mgo.Text(c, r.label).SingleLine().Shrink(0).FontWeight(600)
						mgo.Text(c, r.detail).SingleLine().MinWidth(0).Shrink(1).
							TextColor(k.TextMuted).FontSize(uikit.FontSize(c, theme.CaptionSize))
					})
				})
			}).Fill().Padding(u)
		})
	})
	st.held = st.held && pressed
	return clicked, result
}
