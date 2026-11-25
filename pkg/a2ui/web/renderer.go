package web

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"chatgo/pkg/a2ui"
)

// Options configure a web renderer.
type Options struct {
	// OnAction receives an action with the surface's data model.
	OnAction func(a2ui.Action, map[string]any)
	// OnError receives protocol and page errors.
	OnError func(error)
	// OnRendererMessage receives renderer-to-agent messages from a page.
	OnRendererMessage func(a2ui.RendererMessage)
	// Invalidate asks the host to redraw after a size or message change.
	Invalidate func()
	// ScrollToContainer sends the wheel to the surrounding native scroll
	// instead of the page. Use it for surfaces inline in a message list.
	ScrollToContainer bool
	// BackgroundColor fills the page before it paints.
	BackgroundColor string
	// DefaultHeight is the initial height of a surface, in DIPs.
	DefaultHeight float32
	// Theme forces "light" or "dark"; empty follows the system.
	Theme string
}

// Renderer shows A2UI surfaces with the web bundle.
type Renderer struct {
	win  *mygo.Window
	mgr  *Manager
	opts Options

	applyMu sync.Mutex
	mu      sync.Mutex
	pages   map[string]*page
	theme   string
}

type page struct {
	view   *View
	web    *mygo.WebView
	height float32
}

// New returns a web renderer for a window. Register mgr with
// mygo.Protocol.Handle(Scheme, mgr) before the window is created.
func New(win *mygo.Window, mgr *Manager, opts Options) *Renderer {
	if mgr == nil {
		mgr = NewManager(nil)
	}
	if opts.DefaultHeight <= 0 {
		opts.DefaultHeight = 360
	}
	if opts.BackgroundColor == "" {
		opts.BackgroundColor = "light-dark(#ffffff, #17191c)"
	}
	return &Renderer{win: win, mgr: mgr, opts: opts, pages: map[string]*page{}, theme: opts.Theme}
}

// Apply sends messages to their surface pages, creating one for createSurface.
// It is safe from any goroutine.
func (r *Renderer) Apply(messages ...a2ui.Message) []error {
	r.applyMu.Lock()
	defer r.applyMu.Unlock()
	var out []error
	for _, msg := range messages {
		if err := msg.Validate(); err != nil {
			out = append(out, err)
			continue
		}
		if err := r.applyOne(msg); err != nil {
			out = append(out, err)
		}
	}
	if len(out) > 0 && r.opts.OnError != nil {
		for _, err := range out {
			r.opts.OnError(err)
		}
	}
	return out
}

func (r *Renderer) applyOne(msg a2ui.Message) error {
	if msg.CreateSurface != nil {
		return r.create(msg.CreateSurface)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case msg.AgentFunctionResponse != nil:
		for _, p := range r.pages {
			if err := r.send(p, msg); err != nil {
				return err
			}
		}
		return nil
	case msg.CallRendererFunction != nil:
		p := r.pages[msg.CallRendererFunction.SurfaceID]
		if p == nil {
			return fmt.Errorf("a2ui web: callRendererFunction for unknown surface %q", msg.CallRendererFunction.SurfaceID)
		}
		return r.send(p, msg)
	case msg.UpdateComponents != nil:
		p := r.pages[msg.UpdateComponents.SurfaceID]
		if p == nil {
			return fmt.Errorf("a2ui web: updateComponents for unknown surface %q", msg.UpdateComponents.SurfaceID)
		}
		return r.send(p, msg)
	case msg.UpdateDataModel != nil:
		p := r.pages[msg.UpdateDataModel.SurfaceID]
		if p == nil {
			return fmt.Errorf("a2ui web: updateDataModel for unknown surface %q", msg.UpdateDataModel.SurfaceID)
		}
		return r.send(p, msg)
	case msg.DeleteSurface != nil:
		p := r.pages[msg.DeleteSurface.SurfaceID]
		if p == nil {
			return nil
		}
		delete(r.pages, msg.DeleteSurface.SurfaceID)
		p.web.Destroy()
		p.view.Close()
		if r.opts.Invalidate != nil {
			r.opts.Invalidate()
		}
		return nil
	default:
		return fmt.Errorf("a2ui web: %s is not a surface message", msg.PayloadName())
	}
}

func (r *Renderer) create(in *a2ui.CreateSurface) error {
	r.mu.Lock()
	if _, exists := r.pages[in.SurfaceID]; exists {
		r.mu.Unlock()
		return fmt.Errorf("a2ui web: surface %q already exists", in.SurfaceID)
	}
	r.mu.Unlock()

	view, err := r.mgr.Open(OpenOptions{
		SurfaceID: in.SurfaceID, CatalogID: in.CatalogID, SendDataModel: in.SendDataModel,
		OnAction: r.opts.OnAction,
		OnResize: func(width, height int) {
			r.mu.Lock()
			p := r.pages[in.SurfaceID]
			if p != nil && height > 0 {
				p.height = clampHeight(float32(height))
			}
			r.mu.Unlock()
			if r.opts.Invalidate != nil {
				r.opts.Invalidate()
			}
		},
		OnError:           r.opts.OnError,
		OnRendererMessage: r.opts.OnRendererMessage,
	})
	if err != nil {
		return err
	}
	web, err := r.win.NewWebView(mygo.WebViewOptions{
		ScrollToContainer: r.opts.ScrollToContainer,
		BackgroundColor:   r.opts.BackgroundColor,
		Page:              mygo.PageOptions{Untrusted: true},
	})
	if err != nil {
		view.Close()
		return err
	}
	if p := web.Page(); p != nil {
		// The renderer's own origin is the only page this view may show.
		p.OnWillNavigate(func(e *mygo.NavigateEvent) {
			if strings.HasPrefix(e.URL, view.URL()) {
				return
			}
			e.PreventDefault()
		})
		if err := p.LoadURL(view.URL()); err != nil {
			view.Close()
			web.Destroy()
			return err
		}
	}
	pg := &page{view: view, web: web, height: r.opts.DefaultHeight}
	r.mu.Lock()
	r.pages[in.SurfaceID] = pg
	r.mu.Unlock()
	if p := web.Page(); p != nil {
		p.OnDidFinishLoad(func() { r.applyTheme(pg) })
	}
	r.applyTheme(pg)
	if err := r.send(pg, a2ui.Message{CreateSurface: in}); err != nil {
		return err
	}
	if r.opts.Invalidate != nil {
		r.opts.Invalidate()
	}
	return nil
}

// send queues one message for a page.
func (r *Renderer) send(p *page, msg a2ui.Message) error {
	raw, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	p.view.Send(raw)
	return nil
}

// SetTheme forces the page's color scheme for the next and current frames.
// Empty follows the system's light or dark preference.
func (r *Renderer) SetTheme(theme string) {
	r.mu.Lock()
	r.theme = theme
	pages := make([]*page, 0, len(r.pages))
	for _, p := range r.pages {
		pages = append(pages, p)
	}
	r.mu.Unlock()
	for _, p := range pages {
		r.applyTheme(p)
	}
	if r.opts.Invalidate != nil {
		r.opts.Invalidate()
	}
}

// applyTheme writes the color scheme into a page.
func (r *Renderer) applyTheme(p *page) {
	if p == nil || p.web == nil || p.web.Page() == nil {
		return
	}
	scheme := r.theme
	if scheme != "light" && scheme != "dark" {
		scheme = ""
	}
	_, _ = p.web.Page().Eval("document.documentElement.style.colorScheme = " + jsonQuote(scheme))
}

// jsonQuote is a tiny JSON string encoder for a fixed set of theme names.
func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// View places one surface's page. Main-thread only.
func (r *Renderer) View(c *ui.Context, surfaceID string) {
	r.mu.Lock()
	p := r.pages[surfaceID]
	height := r.opts.DefaultHeight
	if p != nil {
		height = p.height
	}
	r.mu.Unlock()
	if p == nil || p.web == nil {
		ui.Text(c, "正在加载交互界面…").FontSize(12)
		return
	}
	ui.WebView(c, p.web).FillWidth().Height(height)
}

// CallAgentFunction asks a page's bundle to call an agent function. The page
// creates the call id and posts the call through the bridge; the response
// arrives at the bundle and then at Options.OnRendererMessage.
func (r *Renderer) CallAgentFunction(surfaceID, name string, args map[string]any) error {
	r.mu.Lock()
	p := r.pages[surfaceID]
	r.mu.Unlock()
	if p == nil || p.web == nil || p.web.Page() == nil {
		return fmt.Errorf("a2ui web: no page for surface %q", surfaceID)
	}
	payload, err := json.Marshal(args)
	if err != nil {
		return err
	}
	script := "window.a2ui && window.a2ui.callAgentFunction(" + jsonQuote(name) + "," + string(payload) + ")"
	_, err = p.web.Page().Eval(script)
	return err
}

// SupportsCatalog reports true: the bundle decides which components it can
// draw, so a surface routed here renders what it knows and shows placeholders
// for the rest.
func (r *Renderer) SupportsCatalog(string) bool { return true }

// Close destroys every page.
func (r *Renderer) Close() {
	r.mu.Lock()
	pages := r.pages
	r.pages = map[string]*page{}
	r.mu.Unlock()
	for _, p := range pages {
		if p.web != nil {
			p.web.Destroy()
		}
		if p.view != nil {
			p.view.Close()
		}
	}
}

func clampHeight(h float32) float32 {
	if h < 120 {
		return 120
	}
	if h > 2000 {
		return 2000
	}
	return h
}
