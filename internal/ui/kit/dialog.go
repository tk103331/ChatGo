package kit

import (
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// DialogOptions configure a Dialog. Title is required.
type DialogOptions struct {
	Title       string
	Description string
	// Width is 440 DIPs when zero; the window always bounds it.
	Width float32
	// MaxHeight caps the panel in DIPs; zero uses the room below the window
	// edge. A long list then scrolls inside a shorter panel.
	MaxHeight float32
	// Actions builds the footer, usually buttons, aligned to the end.
	Actions func()
	// KeepOnBackdrop ignores clicks on the backdrop.
	KeepOnBackdrop bool
	// KeepOnEscape ignores Escape.
	KeepOnEscape bool
	// HideClose removes the close button from the header.
	HideClose bool
	// Header builds content pinned above the scrolling body, so a search
	// field stays put while a long list scrolls.
	Header func()
}

// Dialog shows a modal dialog while *open is true, trapping and then
// restoring the focus.
func Dialog(c *mgo.Context, open *bool, opts DialogOptions, content func()) mgo.Element {
	if opts.Title == "" {
		panic("kit: Dialog needs a Title")
	}
	site, focus := uikit.OverlaySite(c)
	if !*open {
		focus.Closed(c, site)
		return mgo.Element{}
	}
	uikit.OverlayLayer(c, open)
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	shown, backClicked, closed := true, false, false
	panel := mgo.DialogBase(c, &shown, func(back, panel mgo.Element) {
		backClicked = back.Clicked()
		p := uikit.OverlayEnter(c, panel)
		back.Background(uikit.OverlayScrim(k).Alpha(p)).Padding(16)
		dialogPanel(c, panel, opts.Width, opts.MaxHeight, p)
		panel.Label(opts.Title)
		if opts.Description != "" {
			panel.Description(opts.Description)
		}
		closed = dialogHeader(c, opts.Title, !opts.HideClose)
		if opts.Header != nil {
			opts.Header()
		}
		// Description and content scroll together so the actions keep their room.
		mgo.Scroll(c).Shrink(1).Gap(u * 3).Children(func() {
			if opts.Description != "" {
				mgo.Text(c, opts.Description).TextColor(k.TextMuted)
			}
			if content != nil {
				content()
			}
		})
		if opts.Actions != nil {
			mgo.Row(c).Gap(u*2).Justify(mgo.End).Wrap().Shrink(0).
				Margin(u*2, 0, 0, 0).Children(opts.Actions)
		}
	})
	focus.Shown(panel)
	switch {
	case closed:
		*open = false
	case !shown && backClicked && !opts.KeepOnBackdrop:
		*open = false
	case !shown && !backClicked && !opts.KeepOnEscape:
		*open = false
	}
	return panel
}

// dialogPanel styles a centered dialog panel at entrance progress p.
func dialogPanel(c *mgo.Context, panel mgo.Element, width, maxHeight, p float32) {
	u := uikit.Unit(c)
	if width == 0 {
		width = 440
	}
	w, h := c.Size()
	limit := h - 32
	if maxHeight > 0 && maxHeight < limit {
		limit = maxHeight
	}
	uikit.OverlayStyle(c, panel, p)
	panel.Width(width).MaxWidth(w-32).MaxHeight(limit).Padding(u*6).Gap(u*3).
		Margin(8*(1-p), 0, 0, 0)
}

// dialogHeader builds the title and close button; it reports a close click.
func dialogHeader(c *mgo.Context, title string, closable bool) bool {
	u := uikit.Unit(c)
	clicked := false
	mgo.Column(c).Gap(u).Shrink(0).Children(func() {
		mgo.Row(c).Gap(u * 3).AlignItems(mgo.Start).Children(func() {
			mgo.Text(c, title).
				FontSize(uikit.FontSize(c, theme.H3Size)).FontWeight(600).Grow(1)
			if closable {
				clicked = uikit.IconAction(c, icons.Must("x"), "关闭").
					Margin(-4, -4, 0, 0).Clicked()
			}
		})
	})
	return clicked
}

// AlertDialogOptions configure an AlertDialog. Title is required.
type AlertDialogOptions struct {
	Title       string
	Description string
	// ConfirmLabel is "删除" when empty; CancelLabel is "取消".
	ConfirmLabel string
	CancelLabel  string
}

// AlertDialogResult reports the choice made in this frame.
type AlertDialogResult struct {
	Confirmed bool
	Canceled  bool
}

// AlertDialog confirms a destructive action; Cancel has the focus.
func AlertDialog(c *mgo.Context, open *bool, opts AlertDialogOptions) AlertDialogResult {
	if opts.Title == "" {
		panic("kit: AlertDialog needs a Title")
	}
	var res AlertDialogResult
	site, focus := uikit.OverlaySite(c)
	if !*open {
		focus.Closed(c, site)
		return res
	}
	uikit.OverlayLayer(c, open)
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	confirmLabel, cancelLabel := opts.ConfirmLabel, opts.CancelLabel
	if confirmLabel == "" {
		confirmLabel = "删除"
	}
	if cancelLabel == "" {
		cancelLabel = "取消"
	}
	shown, backClicked := true, false
	panel := mgo.DialogBase(c, &shown, func(back, panel mgo.Element) {
		backClicked = back.Clicked()
		p := uikit.OverlayEnter(c, panel)
		back.Background(uikit.OverlayScrim(k).Alpha(p)).Padding(16)
		dialogPanel(c, panel, 420, 0, p)
		panel.Role(mgo.RoleAlertDialog).Label(opts.Title).Description(opts.Description)
		mgo.Row(c).Gap(u * 3).AlignItems(mgo.Start).Shrink(0).Children(func() {
			mgo.Icon(c, icons.Must("triangle-alert")).Size(22, 22).
				TextColor(k.Danger).Shrink(0).Margin(1, 0, 0, 0)
			mgo.Text(c, opts.Title).
				FontSize(uikit.FontSize(c, theme.H3Size)).FontWeight(600).Grow(1)
		})
		if opts.Description != "" {
			// A long description scrolls so the actions stay in the window.
			mgo.Scroll(c).Shrink(1).Padding(0, 0, 0, 22+u*3).Children(func() {
				mgo.Text(c, opts.Description).TextColor(k.TextMuted)
			})
		}
		mgo.Row(c).Gap(u*2).Justify(mgo.End).Wrap().Shrink(0).
			Margin(u*2, 0, 0, 0).Children(func() {
			cancel := Button(c, cancelLabel, ButtonOptions{Variant: Secondary})
			uikit.AutoFocus(c, cancel)
			if cancel.Clicked() {
				res.Canceled = true
			}
			if Button(c, confirmLabel, ButtonOptions{Variant: Danger}).Clicked() {
				res.Confirmed = true
			}
		})
	})
	focus.Shown(panel)
	if !shown && !backClicked {
		res.Canceled = true // Escape
	}
	if res.Confirmed {
		res.Canceled = false
	}
	if res.Confirmed || res.Canceled {
		*open = false
	}
	return res
}

// PopoverOptions configure a Popover.
type PopoverOptions struct {
	// Title shows in serif at the top and names the panel.
	Title string
	// Width fixes the panel width; zero fits the content, at least the
	// anchor's width.
	Width float32
	// Label names an untitled panel for assistive technology.
	Label string
	// MaxHeight caps the panel height; zero fits the room below or above
	// the anchor. Long lists scroll inside the cap.
	MaxHeight float32
	// Up places the panel above the anchor even when there is room below.
	Up bool
	// Header builds content pinned above the scroll area, so a search field
	// stays put while the list below scrolls.
	Header func()
}

// Popover shows content below anchor, or above it, inside the window while
// *open is true.
func Popover(c *mgo.Context, anchor mgo.Element, open *bool, opts PopoverOptions, content func()) mgo.Element {
	if !anchor.Valid() {
		panic("kit: Popover needs an anchor")
	}
	if !*open {
		return mgo.Element{}
	}
	uikit.OverlayLayer(c, open)
	u := uikit.Unit(c)
	b := anchor.Bounds()
	label := opts.Label
	if opts.Title != "" {
		label = opts.Title
	}
	return mgo.PopoverBase(c, anchor, open, func(panel mgo.Element) {
		uikit.OverlayStyle(c, panel, uikit.OverlayAnchored(c, panel, b))
		panel.Margin(6, 0, 0, 0).Padding(u * 3).Gap(u * 2).
			MinWidth(popoverWide(c, b.W)).MaxHeight(popoverRoom(c, b)).Label(label)
		if opts.MaxHeight > 0 {
			panel.MaxHeight(min(opts.MaxHeight, popoverRoom(c, b)))
		}
		if opts.Up {
			at, self := popoverUpAnchors()
			panel.AttachTo(anchor, at, self)
		}
		if opts.Width > 0 {
			panel.Width(popoverWide(c, opts.Width))
		} else {
			panel.MaxWidth(popoverWide(c, 360))
		}
		if opts.Title != "" {
			mgo.Text(c, opts.Title).
				FontSize(uikit.FontSize(c, theme.BodySize+2)).FontWeight(600).Shrink(0)
		}
		if opts.Header != nil {
			opts.Header()
		}
		mgo.Scroll(c).Shrink(1).Children(content)
	})
}

// popoverUpAnchors are the anchors of a panel opening above its button: the
// panel's bottom start sits on the button's top start, so both leading edges
// line up. Using an end anchor on either side offsets the panel by the
// button's width.
func popoverUpAnchors() (at, self mgo.Anchor) {
	return mgo.AnchorTopStart, mgo.AnchorBottomStart
}

// popoverRoom is the tallest a panel below or above anchor b may be.
func popoverRoom(c *mgo.Context, b mgo.Rect) float32 {
	_, h := c.Size()
	return max(b.Y, h-b.Y-b.H) - 16
}

// popoverWide caps a panel width w to the window.
func popoverWide(c *mgo.Context, w float32) float32 {
	ww, _ := c.Size()
	return min(w, ww-8)
}
