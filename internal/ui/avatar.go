package ui

import (
	"bytes"
	"hash/fnv"
	"image/color"
	"image/png"

	mgo "github.com/egoist/mygo/ui"
	"github.com/issue9/identicon"
)

// agentBitmap returns the cached offline identicon for an agent name.
func (cw *ChatWindow) agentBitmap(name string) *mgo.Bitmap {
	if name == "" {
		name = "Agent"
	}
	cw.avatarMu.Lock()
	defer cw.avatarMu.Unlock()
	if cw.avatarPNG == nil {
		cw.avatarPNG = map[string]*mgo.Bitmap{}
	}
	if bmp := cw.avatarPNG[name]; bmp != nil {
		return bmp
	}
	bmp := makeIdenticon(name)
	cw.avatarPNG[name] = bmp
	return bmp
}

// drawAgentAvatar draws an agent's generated offline avatar.
func (cw *ChatWindow) drawAgentAvatar(c *mgo.Context, name string, size float32) {
	if bmp := cw.agentBitmap(name); bmp != nil {
		mgo.Image(c, bmp).Size(size, size)
	}
}

// agentLeading returns a leading-image builder for a menu row.
func (cw *ChatWindow) agentLeading(c *mgo.Context, name string) func() {
	return func() { cw.agentMarkByName(c, name, 16) }
}

// makeIdenticon builds an offline identicon bitmap for a seed.
func makeIdenticon(seed string) *mgo.Bitmap {
	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	palette := []color.Color{
		color.NRGBA{R: 0x7D, G: 0x20, B: 0x34, A: 0xFF},
		color.NRGBA{R: 0x32, G: 0x62, B: 0x47, A: 0xFF},
		color.NRGBA{R: 0x2B, G: 0x4C, B: 0x7E, A: 0xFF},
		color.NRGBA{R: 0x80, G: 0x57, B: 0x14, A: 0xFF},
		color.NRGBA{R: 0x5B, G: 0x3A, B: 0x7A, A: 0xFF},
	}
	fore := palette[int(h.Sum32())%len(palette)]
	img, err := identicon.Make(64, color.NRGBA{A: 0}, fore, []byte(seed))
	if err != nil {
		return nil
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	bmp, _ := mgo.DecodeBitmap(buf.Bytes())
	return bmp
}
