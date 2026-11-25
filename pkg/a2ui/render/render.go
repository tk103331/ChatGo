// Package render defines the interface both A2UI renderers implement.
package render

import (
	"github.com/egoist/mygo/ui"

	"chatgo/pkg/a2ui"
)

// Renderer draws A2UI surfaces. Apply may be called from any goroutine; View
// only from the UI thread.
type Renderer interface {
	Apply(messages ...a2ui.Message) []error
	View(c *ui.Context, surfaceID string)
	Close()
}

// CatalogMatcher lets a renderer say which catalogs it can draw natively.
type CatalogMatcher interface {
	SupportsCatalog(catalogID string) bool
}

// DataModeler exposes a surface's current data model, as a renderer-to-agent
// action may carry it back.
type DataModeler interface {
	DataModel(surfaceID string) (any, bool)
}
