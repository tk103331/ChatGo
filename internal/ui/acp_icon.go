package ui

import (
	"os"
	"path/filepath"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/config"
	"chatgo/internal/ui/assets"

	"chatgo/internal/ui/icons"
	mcore "chatgo/internal/ui/uikit"
)

// iconImage is a decoded custom icon: an SVG or a bitmap, at most one set.
type iconImage struct {
	svg *mgo.SVG
	bmp *mgo.Bitmap
}

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

// acpLeading draws a connection's mark, preferring a configured icon file, then
// a generated avatar, then the bundled brand logo, then a generic icon.
func (cw *ChatWindow) acpLeading(c *mgo.Context, name string, size float32) {
	if conn, ok := cw.acpConnByName(name); ok {
		if conn.Icon != "" && drawIconImage(c, cw.loadIconImage(conn.Icon), size) {
			return
		}
		if conn.AvatarSeed != "" {
			cw.drawAgentAvatar(c, conn.AvatarSeed, size)
			return
		}
	}
	if acpIcon(c, name, size, size) {
		return
	}
	mgo.Icon(c, icons.Must(chatAgentIcon)).Size(size, size).TextColor(mcore.Tokens(c).AccentText)
}

// acpConnByName finds a configured connection by its display name.
func (cw *ChatWindow) acpConnByName(name string) (config.ACPConnection, bool) {
	for _, a := range cw.cfg.ACPAgents {
		if a.Name == name {
			return a, true
		}
	}
	return config.ACPConnection{}, false
}

// loadIconImage decodes an icon file once and caches the result by path.
func (cw *ChatWindow) loadIconImage(path string) iconImage {
	cw.iconMu.Lock()
	defer cw.iconMu.Unlock()
	if cw.iconCache == nil {
		cw.iconCache = map[string]iconImage{}
	}
	if img, ok := cw.iconCache[path]; ok {
		return img
	}
	var img iconImage
	if data, err := os.ReadFile(path); err == nil {
		if strings.EqualFold(filepath.Ext(path), ".svg") {
			img.svg, _ = mgo.ParseSVG(data)
		} else {
			img.bmp, _ = mgo.DecodeBitmap(data)
		}
	}
	cw.iconCache[path] = img
	return img
}

// drawIconImage renders a decoded icon, scaled to size; reports whether it drew.
func drawIconImage(c *mgo.Context, img iconImage, size float32) bool {
	switch {
	case img.svg != nil:
		mgo.Image(c, img.svg).Size(size, size).Shrink(0)
		return true
	case img.bmp != nil:
		mgo.Image(c, img.bmp).Size(size, size).Shrink(0)
		return true
	}
	return false
}
