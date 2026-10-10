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
	panelRightOnce sync.Once
	panelRightIcon *mgo.SVG
)

func panelRight() *mgo.SVG {
	panelRightOnce.Do(func() { panelRightIcon, _ = mgo.ParseSVG(panelRightSVG) })
	return panelRightIcon
}
