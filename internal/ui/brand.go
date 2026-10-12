package ui

import (
	_ "embed"
	"sync"

	mgo "github.com/egoist/mygo/ui"
)

//go:embed assets/logo.png
var logoPNG []byte

//go:embed assets/panel-right.svg
var panelRightSVG []byte

//go:embed assets/mcp.svg
var mcpSVG []byte

var (
	brandOnce sync.Once
	brandBmp  *mgo.Bitmap
)

// LogoPNG is the application icon, for the dock and the window.
func LogoPNG() []byte { return logoPNG }

func brandBitmap() *mgo.Bitmap {
	brandOnce.Do(func() { brandBmp, _ = mgo.DecodeBitmap(logoPNG) })
	return brandBmp
}

var (
	mcpOnce sync.Once
	mcpIcon *mgo.SVG
)

// mcpLogo is the official Model Context Protocol mark, tinted by TextColor.
func mcpLogo() *mgo.SVG {
	mcpOnce.Do(func() { mcpIcon, _ = mgo.ParseSVG(mcpSVG) })
	return mcpIcon
}

var (
	panelRightOnce sync.Once
	panelRightIcon *mgo.SVG
)

func panelRight() *mgo.SVG {
	panelRightOnce.Do(func() { panelRightIcon, _ = mgo.ParseSVG(panelRightSVG) })
	return panelRightIcon
}
