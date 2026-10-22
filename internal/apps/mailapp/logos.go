package mailapp

import (
	"embed"
	"sync"

	mgo "github.com/egoist/mygo/ui"
)

//go:embed logos/*.png
var logoFS embed.FS

var (
	logoMu    sync.Mutex
	logoCache = map[string]*mgo.Bitmap{}
	logoTried = map[string]bool{}
)

// logoBitmap returns a provider's logo bitmap, or nil when missing.
func logoBitmap(name string) *mgo.Bitmap {
	logoMu.Lock()
	defer logoMu.Unlock()
	if logoTried[name] {
		return logoCache[name]
	}
	logoTried[name] = true
	data, err := logoFS.ReadFile("logos/" + name + ".png")
	if err != nil {
		return nil
	}
	bmp, err := mgo.DecodeBitmap(data)
	if err != nil {
		return nil
	}
	logoCache[name] = bmp
	return bmp
}
