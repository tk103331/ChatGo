package kit

import (
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// PopupMenuItemOptions configure one item of a PopupMenu.
type PopupMenuItemOptions struct {
	Disabled bool
	// KeepOpen keeps the menu open after the item is chosen.
	KeepOpen bool
}

// PopupMenu builds the items of a ContextMenu, reporting choices as they
// happen.
type PopupMenu struct {
	c     *mgo.Context
	root  *popupMenuRoot
	first bool
}

// popupMenuRoot carries what the open menu shares in a frame.
type popupMenuRoot struct{ close bool }

// Item adds an item and reports whether it was chosen this frame.
func (m *PopupMenu) Item(label string, opts PopupMenuItemOptions) bool {
	it := m.row(mgo.ButtonBase(m.c), opts.Disabled)
	it.Children(func() { mgo.Text(m.c, label).SingleLine().Grow(1) })
	if !it.Clicked() {
		return false
	}
	m.root.close = m.root.close || !opts.KeepOpen
	return true
}

// Separator adds a line between items.
func (m *PopupMenu) Separator() {
	u := uikit.Unit(m.c)
	mgo.Box(m.c).Height(theme.BorderWidth).Margin(u, u*2).
		Background(uikit.Tokens(m.c).Border).Shrink(0)
}

// row styles an item element and runs the hover, focus and highlight logic.
func (m *PopupMenu) row(it mgo.Element, disabled bool) mgo.Element {
	c := m.c
	k := uikit.Tokens(c)
	d := uikit.Density(c)
	it.Justify(mgo.Start).MinHeight(max(theme.MinHitSize, d.ControlHeight()-4)).
		Padding(0, d.Unit()*3).Gap(d.Unit() * 2).Radius(theme.ControlRadius).
		Disabled(disabled).TextColor(k.Text)
	uikit.Ring(it, k.Focus, theme.ControlRadius)
	if disabled {
		return it
	}
	if m.first {
		uikit.AutoFocus(c, it)
		m.first = false
	}
	hovered := it.Hovered()
	was := mgo.Local(it, "chatgo.menu.hover", func() bool { return false })
	if hovered && !*was {
		it.Focus() // focus follows the pointer
	}
	*was = hovered
	if hovered || it.FocusVisible() {
		accent := k.AccentText
		it.Background(k.Selection).Draw(func(p *mgo.Painter, r mgo.Rect) {
			p.Fill(mgo.Rect{X: r.X, Y: r.Y + 4, W: 2, H: r.H - 8}, accent, 1)
		})
	}
	return it
}

// PopupMenuOpen shows the menu build fills below anchor while *open is true.
func PopupMenuOpen(c *mgo.Context, anchor mgo.Element, open *bool, build func(m *PopupMenu)) mgo.Element {
	if !*open {
		return mgo.Element{}
	}
	uikit.OverlayLayer(c, open)
	u := uikit.Unit(c)
	b := anchor.Bounds()
	root := &popupMenuRoot{}
	panel := mgo.PopoverBase(c, anchor, open, func(panel mgo.Element) {
		uikit.OverlayStyle(c, panel, uikit.OverlayAnchored(c, panel, b))
		panel.Margin(4, 0, 0, 0).Padding(u).Gap(1).Role(mgo.RoleMenu).
			MinWidth(popoverWide(c, max(b.W, 200))).MaxWidth(popoverWide(c, 360)).
			MaxHeight(popoverRoom(c, b))
		panel.Children(func() { build(&PopupMenu{c: c, root: root, first: true}) })
		if panel.Shortcut(0, mgo.KeyTab) || panel.Shortcut(mgo.Shift, mgo.KeyTab) {
			root.close = true
		}
	})
	if root.close {
		*open = false
		c.Invalidate()
	}
	return panel
}

// ContextMenuOptions configure a ContextMenu.
type ContextMenuOptions struct {
	Disabled bool
}

// contextMenuState is where the menu opened, relative to the target.
type contextMenuState struct {
	open, fresh bool // fresh: opened by a press the menu has not yet seen
	x, y        float32
}

// ContextMenu gives the container target an in-window menu at the pointer or
// below the focus.
func ContextMenu(c *mgo.Context, target mgo.Element, opts ContextMenuOptions, build func(m *PopupMenu)) mgo.Element {
	if !target.Valid() {
		panic("kit: ContextMenu needs a target")
	}
	if build == nil {
		panic("kit: ContextMenu needs a build function")
	}
	st := mgo.Local(target, "chatgo.contextmenu", func() contextMenuState { return contextMenuState{} })
	if opts.Disabled {
		st.open = false
		return target
	}
	target.HandleInput(func(ev mgo.InputEvent) bool {
		if ev.Kind != mgo.InputPointerDown || ev.Button != 1 {
			return false
		}
		st.open, st.fresh, st.x, st.y = true, true, ev.X, ev.Y
		return true
	})
	if target.Shortcut(0, mgo.KeyContextMenu) || target.Shortcut(mgo.Shift, mgo.KeyF10) {
		st.open, st.x, st.y = true, 0, target.Bounds().H
	}
	if !st.open {
		return target
	}
	var anchor mgo.Element
	target.Children(func() {
		anchor = mgo.Box(c.Key("chatgo.contextmenu.anchor")).Absolute().
			Left(st.x).Top(st.y).Size(1, 1).PassThrough()
	})
	panel := PopupMenuOpen(c, anchor, &st.open, build)
	// The opening press fell outside the new panel; it must not dismiss it.
	if st.fresh && panel.Valid() && panel.Dismissed() {
		st.open = true
	}
	st.fresh = false
	return target
}
