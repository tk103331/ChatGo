package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/mobile"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
)

// fabButton floats the one action a list is for over its rows, within reach
// of the thumb.
func fabButton(c *ui.Context, icon, label string, onClick func()) {
	t := c.Theme()
	fab := ui.Row(c).Size(56, 56).Radius(16).Background(t.Accent).
		Shadow(0, 3, 8, 0, ui.RGBA(0, 0, 0, 0.3)).
		AlignItems(ui.Center).Justify(ui.Center).
		Role(ui.RoleButton).Label(label).
		Absolute().Right(16).Bottom(16)
	mobile.Ink(fab)
	if mobile.Current() == mobile.Cupertino {
		fab.Radius(28)
		if fab.Pressed() {
			fab.Background(t.Accent.Alpha(0.85))
		}
	}
	fab.Children(func() {
		ui.Icon(c, icons.Must(icon)).Size(24, 24).TextColor(t.AccentText)
	})
	if fab.Clicked() && onClick != nil {
		onClick()
	}
}

// ChatGo's desktop controls all come from the same kit. These small wrappers
// keep the phone on that visual language instead of MyGo's bare default
// buttons.
func primaryButton(c *ui.Context, label string) ui.Element {
	return kit.Button(c, label, kit.ButtonOptions{Variant: kit.Primary})
}

func secondaryButton(c *ui.Context, label string) ui.Element {
	return kit.Button(c, label, kit.ButtonOptions{Variant: kit.Secondary})
}

func ghostButton(c *ui.Context, label string) ui.Element {
	return kit.Button(c, label, kit.ButtonOptions{Variant: kit.Ghost})
}

func dangerButton(c *ui.Context, label string) ui.Element {
	return kit.Button(c, label, kit.ButtonOptions{Variant: kit.Danger})
}

func smallGhostButton(c *ui.Context, label string) ui.Element {
	return kit.Button(c, label, kit.ButtonOptions{Variant: kit.Ghost}).Height(34).Padding(0, 10)
}

func smallSecondaryButton(c *ui.Context, label string) ui.Element {
	return kit.Button(c, label, kit.ButtonOptions{Variant: kit.Secondary}).Height(34).Padding(0, 12)
}

func smallIconButton(c *ui.Context, icon, label string) ui.Element {
	return kit.IconButton(c, icons.Must(icon), label, kit.IconButtonOptions{
		Variant: kit.Ghost, Size: 34,
	})
}

// optionCardBody draws a large option card's caption and value row: the
// small muted label above, then the mark, the value and the down chevron.
func optionCardBody(c *ui.Context, caption, value string, mark func()) {
	t := c.Theme()
	ui.Column(c).FillWidth().Justify(ui.Center).Children(func() {
		ui.Text(c, caption).FontSize(11).TextColor(t.TextMuted)
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			if mark != nil {
				mark()
			}
			ui.Text(c, value).FontSize(16).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			ui.Icon(c, icons.Must("chevron-down")).Size(14, 14).TextColor(t.TextMuted).Shrink(0)
		})
	})
}

// optionCard is one tappable card of the new-session stack.
func optionCard(c *ui.Context, caption, value string, mark func()) ui.Element {
	t := c.Theme()
	card := ui.ButtonBase(c).FillWidth().Shrink(0).Height(60).Padding(8, 14).
		Radius(16).Background(t.Surface).Role(ui.RoleButton).Label(caption + " " + value)
	if card.Pressed() {
		card.Background(t.SurfaceHover)
	}
	card.Children(func() { optionCardBody(c, caption, value, mark) })
	return card
}

// styleSelectCard makes a SelectBase trigger follow the optionCard look.
func styleSelectCard(c *ui.Context, trigger ui.Element, caption, value string, mark func()) {
	t := c.Theme()
	trigger.FillWidth().Shrink(0).Height(60).Padding(8, 14).Radius(16).
		Background(t.Surface).BorderWidth(0, 0, 0, 0).Justify(ui.Start)
	if trigger.Pressed() {
		trigger.Background(t.SurfaceHover)
	}
	trigger.Children(func() { optionCardBody(c, caption, value, mark) })
}

// emptyState is a list's empty screen: a soft icon, a title, one line of
// guidance and an optional action, centered rather than pinned to the top.
func emptyState(c *ui.Context, icon, title, body string, action func()) {
	t := c.Theme()
	ui.Column(c).FillWidth().Padding(72, 32, 32, 32).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Size(56, 56).Radius(18).Background(t.Accent.Alpha(0.12)).
			AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.Icon(c, icons.Must(icon)).Size(26, 26).TextColor(t.Accent)
		})
		ui.Text(c, title).FontSize(17).FontWeight(600).TextAlign(ui.Center)
		ui.Text(c, body).FontSize(13).TextColor(t.TextMuted).MaxLines(3).TextAlign(ui.Center)
		if action != nil {
			action()
		}
	})
}
