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

//go:embed brand.svg
var brandSVG []byte

//go:embed panel-right.svg
var panelRightSVG []byte

//go:embed mcp.svg
var mcpSVG []byte

//go:embed tray.png
var trayPNG []byte

var (
	markOnce sync.Once
	markIcon *mgo.SVG
)

// LogoPNG is the application icon, for the dock and the window.
func LogoPNG() []byte { return logoPNG }

// TrayPNG is the menu-bar / system-tray icon.
func TrayPNG() []byte { return trayPNG }

// BrandMark is the app's mark as a single-color icon, tinted by TextColor so
// it follows the window theme. The interface draws this; the window, dock and
// tray icons stay bitmaps, which their platforms require.
func BrandMark() *mgo.SVG {
	markOnce.Do(func() { markIcon, _ = mgo.ParseSVG(brandSVG) })
	return markIcon
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
