// Package hybrid routes A2UI surfaces to a native or web renderer. The
// protocol store and the renderers stay independent; this package only picks
// one per surface and forwards messages in order.
package hybrid

import (
	"fmt"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"

	"chatgo/pkg/a2ui"
	"chatgo/pkg/a2ui/render"
)

// Router sends each surface to the renderer that can draw its catalog.
type Router struct {
	native render.Renderer
	web    render.Renderer

	mu     sync.Mutex
	routes map[string]render.Renderer
}

// New builds a router. Either renderer may be nil; a surface that needs a
// missing one is an error rather than a silent blank.
func New(native, web render.Renderer) *Router {
	return &Router{native: native, web: web, routes: map[string]render.Renderer{}}
}

// Apply routes messages and forwards each to its surface's renderer.
func (r *Router) Apply(messages ...a2ui.Message) []error {
	var errs []error
	for _, msg := range messages {
		if err := msg.Validate(); err != nil {
			errs = append(errs, err)
			continue
		}
		// An agent function response names only its call id, so both
		// renderers see it and the one waiting completes the call.
		if msg.AgentFunctionResponse != nil {
			if r.native != nil {
				if e := r.native.Apply(msg); len(e) > 0 {
					errs = append(errs, e...)
				}
			}
			if r.web != nil {
				if e := r.web.Apply(msg); len(e) > 0 {
					errs = append(errs, e...)
				}
			}
			continue
		}
		surfaceID := surfaceOf(msg)
		if msg.CreateSurface != nil {
			choice, err := r.choose(msg.CreateSurface)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			r.mu.Lock()
			r.routes[surfaceID] = choice
			r.mu.Unlock()
		}
		r.mu.Lock()
		target := r.routes[surfaceID]
		r.mu.Unlock()
		if target == nil {
			errs = append(errs, fmt.Errorf("a2ui hybrid: no route for surface %q", surfaceID))
			continue
		}
		if applied := target.Apply(msg); len(applied) > 0 {
			errs = append(errs, applied...)
		}
		if msg.DeleteSurface != nil {
			r.mu.Lock()
			delete(r.routes, surfaceID)
			r.mu.Unlock()
		}
	}
	return errs
}

// choose picks the renderer for a new surface. A catalog the native renderer
// announces wins; anything else, including a per-component @catalogId, goes
// to the web renderer.
func (r *Router) choose(in *a2ui.CreateSurface) (render.Renderer, error) {
	// A surface may pin its renderer, so replay and every host agree.
	pref := a2ui.RenderPreferenceOf(in.Metadata)
	switch pref.Mode {
	case a2ui.RenderNative:
		if r.native == nil {
			return nil, fmt.Errorf("a2ui hybrid: surface %q requests native rendering, but no native renderer is configured", in.SurfaceID)
		}
		var unsupported []string
		for _, comp := range in.Components {
			if !supportsComponent(r.native, comp.Type, comp.CatalogID) {
				unsupported = append(unsupported, comp.Type)
			}
		}
		if len(unsupported) > 0 {
			return nil, fmt.Errorf("a2ui hybrid: native renderer cannot draw %s on surface %q", strings.Join(unsupported, ", "), in.SurfaceID)
		}
		return r.native, nil
	case a2ui.RenderWeb:
		if r.web == nil {
			return nil, fmt.Errorf("a2ui hybrid: surface %q requests web rendering, but no web renderer is configured", in.SurfaceID)
		}
		return r.web, nil
	case "", a2ui.RenderAuto:
	default:
		return nil, fmt.Errorf("a2ui hybrid: unknown render mode %q on surface %q", pref.Mode, in.SurfaceID)
	}

	nativeOK := r.native != nil && supports(r.native, in.CatalogID)
	if nativeOK {
		for _, comp := range in.Components {
			if comp.CatalogID != "" && !supports(r.native, comp.CatalogID) {
				nativeOK = false
				break
			}
			if !supportsComponent(r.native, comp.Type, comp.CatalogID) {
				nativeOK = false
				break
			}
		}
	}
	if nativeOK {
		return r.native, nil
	}
	if r.web != nil {
		return r.web, nil
	}
	if r.native != nil {
		return r.native, nil // draw what it knows, placeholders for the rest
	}
	return nil, fmt.Errorf("a2ui hybrid: no renderer for catalog %q", in.CatalogID)
}

// supports asks a renderer whether it draws a catalog natively.
func supports(r render.Renderer, catalogID string) bool {
	if m, ok := r.(render.CatalogMatcher); ok {
		return m.SupportsCatalog(catalogID)
	}
	return catalogID == ""
}

// supportsComponent asks a renderer whether it draws a component natively.
func supportsComponent(r render.Renderer, name, catalogID string) bool {
	if m, ok := r.(interface {
		SupportsComponent(name, catalogID string) bool
	}); ok {
		return m.SupportsComponent(name, catalogID)
	}
	return true
}

// surfaceOf returns the surface a message addresses.
func surfaceOf(msg a2ui.Message) string {
	switch {
	case msg.CreateSurface != nil:
		return msg.CreateSurface.SurfaceID
	case msg.UpdateComponents != nil:
		return msg.UpdateComponents.SurfaceID
	case msg.UpdateDataModel != nil:
		return msg.UpdateDataModel.SurfaceID
	case msg.DeleteSurface != nil:
		return msg.DeleteSurface.SurfaceID
	case msg.CallRendererFunction != nil:
		return msg.CallRendererFunction.SurfaceID
	}
	return ""
}

// View draws the surface with the renderer that owns it.
func (r *Router) View(c *ui.Context, surfaceID string) {
	r.mu.Lock()
	target := r.routes[surfaceID]
	r.mu.Unlock()
	if target == nil {
		ui.Text(c, "surface 未路由："+surfaceID).FontSize(12)
		return
	}
	target.View(c, surfaceID)
}

// Close releases both renderers.
func (r *Router) Close() {
	if r.native != nil {
		r.native.Close()
	}
	if r.web != nil {
		r.web.Close()
	}
	r.mu.Lock()
	r.routes = map[string]render.Renderer{}
	r.mu.Unlock()
}

// DataModel returns a surface's data model from its renderer.
func (r *Router) DataModel(surfaceID string) (any, bool) {
	r.mu.Lock()
	target := r.routes[surfaceID]
	r.mu.Unlock()
	if m, ok := target.(interface{ DataModel(string) (any, bool) }); ok {
		return m.DataModel(surfaceID)
	}
	return nil, false
}

// RouteOf reports which renderer owns a surface, for tests and diagnostics.
// The name is "native", "web" or "".
func (r *Router) RouteOf(surfaceID string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	target := r.routes[surfaceID]
	switch {
	case target == nil:
		return ""
	case target == r.native:
		return "native"
	case target == r.web:
		return "web"
	}
	return "other"
}
