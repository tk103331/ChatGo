package ui

import (
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// cardRows lays n cards out in rows of cols equal columns, as the MCP page
// does. A grid is avoided on purpose: inside a grid cell a width of 100%
// resolves against the whole grid rather than the cell, which makes cards
// overlap. Flex columns of Grow(1).Basis(0) size against their row instead.
func cardRows(c *mgo.Context, cols, n int, card func(i int)) {
	if cols < 1 {
		cols = 1
	}
	u := mcore.Density(c).Unit()
	mgo.Column(c).FillWidth().Gap(u * 3).Children(func() {
		for start := 0; start < n; start += cols {
			start := start
			mgo.Row(c).FillWidth().Gap(u * 3).AlignItems(mgo.Stretch).Children(func() {
				for col := 0; col < cols; col++ {
					i := start + col
					cell := mgo.Column(c).Grow(1).Basis(0).MinWidth(0)
					if probe := cardCellProbe; probe != nil && i < n {
						cell.DrawOver(func(_ *mgo.Painter, r mgo.Rect) { probe(r) })
					}
					cell.Children(func() {
						if i < n {
							card(i)
						}
					})
				}
			})
		}
	})
}

// cardCellProbe reports the rect of every laid-out card cell. Tests set it to
// assert that the columns never overlap; it stays nil in the running app.
var cardCellProbe func(mgo.Rect)

// measureCardCells renders view and returns the distinct card cell rects.
// A frame may be laid out more than once, so repeated rects are collapsed.
func measureCardCells(view func(*mgo.Context), w, h int) []mgo.Rect {
	seen := map[mgo.Rect]bool{}
	var out []mgo.Rect
	cardCellProbe = func(r mgo.Rect) {
		if seen[r] {
			return
		}
		seen[r] = true
		out = append(out, r)
	}
	defer func() { cardCellProbe = nil }()
	mgo.Render(view, w, h, 1)
	return out
}

// cardShell is the shared look of the card lists on the Agent, knowledge base,
// auto task and MCP pages: a surface panel whose rows all span its width, so
// long names ellipsize instead of widening the card.
func cardShell(c *mgo.Context, key string, body func()) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Key(key).FillWidth().MinWidth(0).Gap(u*2).Padding(u*3).
		Background(k.Surface).Border(mtheme.BorderWidth, k.Border).Radius(mtheme.CardRadius).
		Children(body)
}

// cardTitleRow is a card's first row: a leading icon slot, the name, and
// trailing controls.
func cardTitleRow(c *mgo.Context, body func()) {
	u := mcore.Density(c).Unit()
	mgo.Row(c).FillWidth().MinWidth(0).AlignItems(mgo.Center).Gap(u).Children(body)
}

// cardTitle renders a card's name, ellipsized to the card's width.
func cardTitle(c *mgo.Context, name string) mgo.Element {
	return mgo.Text(c, name).
		FontSize(mcore.FontSize(c, mtheme.H3Size)).FontWeight(600).
		Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
}

// cardMetaRow holds the tags and small facts under a card's title.
func cardMetaRow(c *mgo.Context, body func()) {
	u := mcore.Density(c).Unit()
	mgo.Row(c).FillWidth().MinWidth(0).Gap(u).Wrap().AlignItems(mgo.Center).Children(body)
}

// cardActionRow holds a card's buttons.
func cardActionRow(c *mgo.Context, body func()) {
	u := mcore.Density(c).Unit()
	mgo.Row(c).FillWidth().MinWidth(0).Gap(u).Wrap().AlignItems(mgo.Center).Children(body)
}

// emptyStateOptions configure an emptyState panel.
type emptyStateOptions struct {
	Icon string
	// Title is the headline, Hint the line explaining what to do next.
	Title string
	Hint  string
	// Actions builds the buttons under the hint.
	Actions func()
}

// emptyState is the shared placeholder of the card list pages: a muted icon,
// a headline, a hint and the actions that fill the list.
func emptyState(c *mgo.Context, opts emptyStateOptions) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Grow(1).MinHeight(0).FillWidth().
		AlignItems(mgo.Center).Justify(mgo.Center).Gap(u*2).Padding(u*8, u*4).
		Children(func() {
			mgo.Icon(c, icons.Must(opts.Icon)).Size(48, 48).TextColor(k.TextMuted)
			mgo.Text(c, opts.Title).
				FontSize(mcore.FontSize(c, mtheme.H3Size)).TextColor(k.TextMuted)
			if opts.Hint != "" {
				mgo.Text(c, opts.Hint).TextColor(k.TextMuted).
					FontSize(mcore.FontSize(c, mtheme.CaptionSize))
			}
			if opts.Actions != nil {
				mgo.Row(c).Gap(u * 2).Wrap().AlignItems(mgo.Center).Children(opts.Actions)
			}
		})
}
