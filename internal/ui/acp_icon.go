package ui

import (
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/assets"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
)

// acpIcon renders the agent's brand icon (SVG first, then PNG bitmap) at the
// given size. Returns true if an icon was drawn.
func acpIcon(c *mgo.Context, name string, w, h float32) bool {
	if svg := assets.ACPLogoSVG(name); svg != nil {
		if assets.ACPTiledIcons[assets.ACPIconKey(name)] {
			mgo.Image(c, svg).Size(w, h).Shrink(0)
		} else {
			mgo.Icon(c, svg).Size(w, h).Shrink(0).TextColor(mcore.Tokens(c).Text)
		}
		return true
	}
	if bmp := assets.ACPLogo(name); bmp != nil {
		mgo.Image(c, bmp).Size(w, h).Shrink(0)
		return true
	}
	return false
}
