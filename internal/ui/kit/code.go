package kit

import (
	"fmt"
	"slices"
	"strings"

	mgo "github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// CodeViewerOptions configure a CodeViewer.
type CodeViewerOptions struct {
	// Language picks the highlighting, by name as "go"; empty falls back to
	// the one Path names.
	Language string
	// Path names the file, used to pick the language and for the label.
	Path string
	// Label names the view for assistive technology.
	Label string
	// Wrap wraps long lines rather than scrolling them sideways.
	Wrap bool
}

// CodeViewerState keeps a CodeViewer's buffer, selection and scroll across
// frames.
type CodeViewerState struct {
	st   editor.State
	text string
	lang string
}

// CodeViewer shows code read-only, with line numbers and highlighting.
func CodeViewer(c *mgo.Context, text string, st *CodeViewerState, opts CodeViewerOptions) mgo.Element {
	if st == nil {
		panic("kit: CodeViewer needs a state")
	}
	lang := opts.Language
	if lang == "" {
		lang = opts.Path
	}
	// The buffer is rebuilt only when the text or language changes, so the
	// selection and scroll survive a frame.
	if st.st.Buffer == nil || st.text != text || st.lang != lang {
		st.text, st.lang = text, lang
		buf := editor.NewBuffer(text)
		buf.SetLanguage(editorLanguage(lang))
		st.st = editor.State{Buffer: buf, ReadOnly: true, Font: codeFont(c)}
	}
	st.st.SoftWrap = opts.Wrap
	label := opts.Label
	if label == "" {
		label = opts.Path
	}
	v := editor.View(c, &st.st)
	if label != "" {
		v.Label(label)
	}
	return v
}

// editorLanguage resolves a language by name, or by a file name's extension.
func editorLanguage(name string) *editor.Language {
	if name == "" {
		return nil
	}
	if strings.ContainsAny(name, "./\\") {
		return editor.LanguageFor(name)
	}
	for _, l := range editor.Languages() {
		if strings.EqualFold(l.Name, name) {
			return l
		}
		for _, ext := range l.Extensions {
			if strings.EqualFold(strings.TrimPrefix(ext, "."), name) {
				return l
			}
		}
	}
	// A bare extension still resolves, as "go" through "x.go".
	return editor.LanguageFor("x." + name)
}

// codeFont is the monospace font code views use.
func codeFont(c *mgo.Context) mgo.Font {
	return mgo.Font{Family: theme.MonoFont, Size: uikit.FontSize(c, theme.CaptionSize+1)}
}

// DiffViewerOptions configure a DiffViewer.
type DiffViewerOptions struct {
	// Language picks the highlighting; empty falls back to Path.
	Language string
	Path     string
	// Unified shows one text over the other rather than side by side.
	Unified bool
	// Wrap wraps long lines rather than scrolling them sideways.
	Wrap bool
}

// DiffViewerState keeps a DiffViewer's buffers and place across frames.
type DiffViewerState struct {
	st             editor.DiffState
	from, to, lang string
	built          bool
}

// DiffViewer shows the changes from one text to another.
func DiffViewer(c *mgo.Context, st *DiffViewerState, from, to string, opts DiffViewerOptions) mgo.Element {
	if st == nil {
		panic("kit: DiffViewer needs a state")
	}
	lang := opts.Language
	if lang == "" {
		lang = opts.Path
	}
	if !st.built || st.from != from || st.to != to || st.lang != lang {
		st.from, st.to, st.lang, st.built = from, to, lang, true
		l := editorLanguage(lang)
		old, new := editor.NewBuffer(from), editor.NewBuffer(to)
		old.SetLanguage(l)
		new.SetLanguage(l)
		st.st = editor.DiffState{Old: old, New: new, Inline: editor.DiffInlineWordAlt, Font: codeFont(c)}
	}
	st.st.Wrap = opts.Wrap
	st.st.Layout = editor.DiffSplit
	if opts.Unified {
		st.st.Layout = editor.DiffUnified
	}
	return editor.DiffView(c, &st.st)
}

// TerminalTab is one tab of TerminalTabs.
type TerminalTab struct {
	ID, Title string
	// Busy asks for confirmation before closing.
	Busy bool
}

// TerminalTabsOptions configure TerminalTabs.
type TerminalTabsOptions struct {
	Disabled bool
	// Actions draw extra controls at the end of the tab strip.
	Actions func()
}

// TerminalTabsResult is TerminalTabs and what happened in this frame.
type TerminalTabsResult struct {
	Element mgo.Element
	// Renamed and Closed hold a tab ID.
	Changed bool
	Renamed string
	Closed  string
}

type terminalTabsState struct {
	editing, draft, closing string
	confirm                 bool
}

// TerminalTabs shows renamable, closable terminal tabs.
func TerminalTabs(c *mgo.Context, tabs *[]TerminalTab, active *string, opts TerminalTabsOptions, content func(id string)) TerminalTabsResult {
	if tabs == nil || active == nil || content == nil {
		panic("kit: TerminalTabs needs tabs, active and content")
	}
	at := slices.IndexFunc(*tabs, func(t TerminalTab) bool { return t.ID == *active })
	if len(*tabs) > 0 && at < 0 {
		panic("kit: TerminalTabs active is not a tab ID")
	}
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	pad := theme.FocusGap + theme.FocusWidth
	var res TerminalTabsResult
	res.Element = mgo.Column(c).MinHeight(0).Disabled(opts.Disabled)
	st := mgo.Local(res.Element, "chatgo.terminal_tabs", func() terminalTabsState { return terminalTabsState{} })
	disabled := res.Element.IsDisabled()
	if disabled {
		st.editing, st.confirm = "", false
	}
	remove := func(id string) {
		i := slices.IndexFunc(*tabs, func(t TerminalTab) bool { return t.ID == id })
		if i < 0 {
			return
		}
		*tabs = slices.Delete(*tabs, i, i+1)
		if *active == id && len(*tabs) > 0 {
			*active = (*tabs)[min(i, len(*tabs)-1)].ID
			res.Changed = true
		}
		res.Closed = id
	}
	askClose := func(t TerminalTab) {
		if t.Busy {
			st.closing, st.confirm = t.ID, true
			return
		}
		remove(t.ID)
	}
	res.Element.Children(func() {
		strip := mgo.ScrollHorizontal(c).Shrink(0).MinWidth(0).Padding(pad, pad, 0, pad)
		border := k.Border
		strip.Draw(func(p *mgo.Painter, r mgo.Rect) {
			p.Fill(mgo.Rect{X: r.X, Y: r.Y + r.H - 1, W: r.W, H: 1}, border, 0)
		})
		strip.Children(func() {
			idx := max(at, 0)
			base := mgo.TabsBase(c, &idx, len(*tabs))
			base.List.Gap(u).Label("终端标签")
			base.List.Children(func() {
				for i, t := range *tabs {
					rename := func(title string) {
						(*tabs)[i].Title = title
						res.Renamed = t.ID
					}
					terminalTab(c, base.Tab(i), t, i == at, st, rename, askClose, disabled)
				}
			})
			if base.List.Changed() && !disabled {
				*active = (*tabs)[idx].ID
				res.Changed = true
			}
			if opts.Actions != nil {
				opts.Actions()
			}
		})
		if len(*tabs) == 0 {
			mgo.Text(c, "没有终端").FontSize(uikit.FontSize(c, theme.CaptionSize)).
				TextColor(k.TextMuted).SingleLine().Padding(u * 4)
			return
		}
		mgo.Column(c).Grow(1).MinHeight(0).Padding(u*2, 0, 0, 0).
			Children(func() { content(*active) })
	})
	if st.confirm {
		title := ""
		for _, t := range *tabs {
			if t.ID == st.closing {
				title = t.Title
			}
		}
		r := AlertDialog(c, &st.confirm, AlertDialogOptions{
			Title:        fmt.Sprintf("关闭「%s」？", title),
			Description:  "该终端还在运行命令。",
			ConfirmLabel: "关闭",
		})
		if r.Confirmed && !disabled {
			remove(st.closing)
		}
	}
	return res
}

// terminalTab builds one tab with its title, busy mark and close button.
func terminalTab(c *mgo.Context, tab mgo.Element, t TerminalTab, on bool, st *terminalTabsState, rename func(string), askClose func(TerminalTab), disabled bool) {
	k := uikit.Tokens(c)
	d := uikit.Density(c)
	u := d.Unit()
	tab.MinHeight(d.ControlHeight()).Padding(0, u, 0, u*3).Gap(u).AlignItems(mgo.Center).
		Radius(theme.ControlRadius, theme.ControlRadius, 0, 0).Label(t.Title)
	if t.Busy {
		tab.Description("正在运行")
	}
	fg := k.TextMuted
	switch {
	case on:
		tab.Background(k.Selection).FontWeight(600)
		fg = k.Text
	case tab.Hovered():
		tab.Background(k.SurfaceHover)
		fg = k.Text
	}
	ease(tab, uikit.Motion(c, theme.StateDuration), fg)
	// FocusVisible is read in the build pass; the paint callback runs later,
	// when this element handle is no longer valid.
	ringVisible := tab.FocusVisible()
	accent, focus := k.AccentText, k.Focus
	tab.FocusRing(false).DrawOver(func(p *mgo.Painter, r mgo.Rect) {
		if on {
			p.Fill(mgo.Rect{X: r.X, Y: r.Y + r.H - 2, W: r.W, H: 2}, accent, 0)
		}
		if ringVisible {
			uikit.FocusRing(p, r, theme.ControlRadius, focus)
		}
	})
	if !disabled && st.editing == "" && (tab.DoubleClicked() || tab.Shortcut(0, mgo.KeyF2)) {
		st.editing, st.draft = t.ID, t.Title
	}
	if !disabled && tab.Shortcut(0, mgo.KeyDelete) {
		askClose(t)
	}
	tab.Children(func() {
		if st.editing == t.ID {
			in := TextInput(c, &st.draft, TextInputOptions{Label: "重命名终端"})
			in.Element.Width(u * 36)
			uikit.AutoFocus(c, in.Input)
			switch {
			case in.Input.Shortcut(0, mgo.KeyEscape):
				st.editing = ""
			case in.Submitted() && strings.TrimSpace(st.draft) != "":
				rename(strings.TrimSpace(st.draft))
				st.editing = ""
				tab.Focus()
			}
			return
		}
		mgo.Text(c, t.Title).SingleLine()
		if t.Busy {
			mgo.Box(c).Size(6, 6).Shrink(0).Role(mgo.RoleNone).Background(k.AccentText).Radius(3)
		}
		if uikit.IconAction(c, icons.Must("x"), "关闭 "+t.Title).Clicked() && !disabled {
			askClose(t)
		}
	})
}
