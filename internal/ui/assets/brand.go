// Package assets embeds and caches ChatGo's brand marks, provider logos and
// ACP agent icons.
package assets

import (
	_ "embed"
	"sync"

	mgo "github.com/egoist/mygo/ui"
)

//go:embed logo.png
var logoPNG []byte

//go:embed panel-right.svg
var panelRightSVG []byte

//go:embed mcp.svg
var mcpSVG []byte

//go:embed tray.png
var trayPNG []byte

var (
	brandOnce sync.Once
	brandBmp  *mgo.Bitmap
)

// LogoPNG is the application icon, for the dock and the window.
func LogoPNG() []byte { return logoPNG }

// TrayPNG is the menu-bar / system-tray icon.
func TrayPNG() []byte { return trayPNG }

// BrandBitmap decodes and caches the app logo.
func BrandBitmap() *mgo.Bitmap {
	brandOnce.Do(func() { brandBmp, _ = mgo.DecodeBitmap(logoPNG) })
	return brandBmp
}

var (
	mcpOnce sync.Once
	mcpIcon *mgo.SVG
)

// MCPLogo is the official Model Context Protocol mark, tinted by TextColor.
func MCPLogo() *mgo.SVG {
	mcpOnce.Do(func() { mcpIcon, _ = mgo.ParseSVG(mcpSVG) })
	return mcpIcon
}

var (
	panelRightOnce sync.Once
	panelRightIcon *mgo.SVG
)

// PanelRight is the collapsible-panel arrow icon.
func PanelRight() *mgo.SVG {
	panelRightOnce.Do(func() { panelRightIcon, _ = mgo.ParseSVG(panelRightSVG) })
	return panelRightIcon
}
