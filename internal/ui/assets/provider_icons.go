package assets

import (
	"embed"
	"io/fs"
	"strings"
	"sync"

	mgo "github.com/egoist/mygo/ui"
)

//go:embed provider_*.svg
var providerFS embed.FS

var (
	providerLogoOnce sync.Once
	providerLogos    map[string]*mgo.SVG
)

// ProviderLogo returns the brand mark for a provider type, or nil.
func ProviderLogo(typ string) *mgo.SVG {
	providerLogoOnce.Do(func() {
		providerLogos = map[string]*mgo.SVG{}
		entries, _ := fs.Glob(providerFS, "provider_*.svg")
		for _, e := range entries {
			name := strings.TrimSuffix(strings.TrimPrefix(e, "provider_"), ".svg")
			data, err := providerFS.ReadFile(e)
			if err != nil {
				continue
			}
			if svg, err := mgo.ParseSVG(data); err == nil {
				providerLogos[name] = svg
			}
		}
	})
	return providerLogos[typ]
}
