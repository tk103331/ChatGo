package kit

import (
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// SwitchRowOptions configure a SwitchRow.
type SwitchRowOptions struct {
	// Hint explains the setting under its label; empty draws no hint.
	Hint string
	// Compact puts the label beside the switch instead of spreading the two
	// to the row's ends, for a toolbar of controls.
	Compact bool
}

// SwitchRowResult is what a SwitchRow built and reported this frame.
type SwitchRowResult struct {
	Element mgo.Element
	changed bool
}

// Changed reports that the user toggled the switch this frame.
func (r SwitchRowResult) Changed() bool { return r.changed }

// SwitchRow draws a switch with the text that names it. MyGo's Label only
// names a control for assistive technology and draws nothing, so a bare
// switch leaves its setting unreadable.
func SwitchRow(c *mgo.Context, on *bool, label string, opts SwitchRowOptions) SwitchRowResult {
	if on == nil {
		panic("kit: SwitchRow needs a value")
	}
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, theme.CaptionSize)
	row := mgo.Row(c).AlignItems(mgo.Center).Gap(u * 2)
	if !opts.Compact {
		row = row.FillWidth()
	}
	var changed bool
	row.Children(func() {
		if opts.Compact {
			mgo.Text(c, label).TextColor(k.Text).Shrink(0)
		} else {
			mgo.Column(c).Grow(1).MinWidth(0).Gap(u / 2).Children(func() {
				mgo.Text(c, label).TextColor(k.Text)
				if opts.Hint != "" {
					mgo.Text(c, opts.Hint).FontSize(caption).TextColor(k.TextMuted)
				}
			})
		}
		changed = mgo.Switch(c, on).Label(label).Changed()
	})
	return SwitchRowResult{Element: row, changed: changed}
}
