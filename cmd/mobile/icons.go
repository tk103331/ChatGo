package main

import "github.com/egoist/mygo/ui"

func iconChat(p *ui.Painter, r ui.Rect, c ui.Color) {
	p.Stroke(ui.Rect{X: r.X + 3, Y: r.Y + 4, W: r.W - 6, H: r.H - 8}, c, 5, 2)
	p.Line(r.X+8, r.Y+r.H-5, r.X+13, r.Y+r.H-1, 2, c)
	p.Line(r.X+8, r.Y+r.H-5, r.X+8, r.Y+r.H-11, 2, c)
	p.Line(r.X+9, r.Y+r.H/2-2, r.X+r.W-9, r.Y+r.H/2-2, 2, c)
}

func iconRooms(p *ui.Painter, r ui.Rect, c ui.Color) {
	p.Stroke(ui.Rect{X: r.X + 2, Y: r.Y + 5, W: 12, H: 12}, c, 6, 2)
	p.Stroke(ui.Rect{X: r.X + 10, Y: r.Y + 9, W: 12, H: 12}, c, 6, 2)
}

func iconTasks(p *ui.Painter, r ui.Rect, c ui.Color) {
	p.Stroke(ui.Rect{X: r.X + 3, Y: r.Y + 3, W: r.W - 6, H: r.H - 6}, c, 12, 2)
	p.Line(r.X+r.W/2, r.Y+6, r.X+r.W/2, r.Y+r.H/2, 2, c)
	p.Line(r.X+r.W/2, r.Y+r.H/2, r.X+r.W-7, r.Y+r.H/2-4, 2, c)
}

func iconSettings(p *ui.Painter, r ui.Rect, c ui.Color) {
	p.Stroke(ui.Rect{X: r.X + 3, Y: r.Y + 3, W: r.W - 6, H: r.H - 6}, c, 10, 2)
	p.Stroke(ui.Rect{X: r.X + 9, Y: r.Y + 9, W: r.W - 18, H: r.H - 18}, c, 4, 2)
}
