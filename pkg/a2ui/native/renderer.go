// Package native renders A2UI surfaces with MyGo's native widgets. It is the
// renderer for hosts that want no web engine: text, layout, cards, forms and
// the basic inputs draw directly with mygo/ui.
package native

import (
	"fmt"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"

	"chatgo/pkg/a2ui"
)

// Theme is the palette the renderer draws with. Hosts pass their own tokens;
// DefaultTheme is a neutral light palette and DarkTheme its dark counterpart.
type Theme struct {
	Text    ui.Color
	Muted   ui.Color
	Border  ui.Color
	Surface ui.Color
	Accent  ui.Color
	Danger  ui.Color
	Unit    float32
}

// DefaultTheme is a neutral light palette.
func DefaultTheme() Theme {
	return Theme{
		Text:    ui.RGBA(28, 31, 36, 1),
		Muted:   ui.RGBA(107, 114, 128, 1),
		Border:  ui.RGBA(226, 229, 234, 1),
		Surface: ui.RGBA(246, 248, 251, 1),
		Accent:  ui.RGBA(37, 99, 235, 1),
		Danger:  ui.RGBA(220, 38, 38, 1),
		Unit:    4,
	}
}

// DarkTheme is the same palette for a dark window.
func DarkTheme() Theme {
	return Theme{
		Text:    ui.RGBA(243, 244, 246, 1),
		Muted:   ui.RGBA(156, 163, 175, 1),
		Border:  ui.RGBA(55, 65, 81, 1),
		Surface: ui.RGBA(33, 36, 41, 1),
		Accent:  ui.RGBA(56, 189, 248, 1),
		Danger:  ui.RGBA(248, 113, 113, 1),
		Unit:    4,
	}
}

// Options configure a native renderer.
type Options struct {
	Theme Theme
	// Functions extend the built-in @call functions.
	Functions a2ui.Functions
	// OnAction receives button and form actions.
	OnAction func(a2ui.Action)
	// OnError receives protocol errors; Apply also returns them.
	OnError func(error)
	// OnRendererMessage receives renderer-to-agent messages: function
	// responses and renderer-initiated agent function calls.
	OnRendererMessage func(a2ui.RendererMessage)
	// OnAgentFunctionResult completes a renderer-initiated call.
	OnAgentFunctionResult func(callID string, value any, err *a2ui.RendererError)
	// Invalidate asks the host to redraw after messages change a surface.
	Invalidate func()
	// ImageLoader turns a URL into a MyGo image source. Without it images
	// render as a labelled placeholder.
	ImageLoader func(url string) (ui.ImageSource, error)
	// IconLoader draws an icon by name. Without it icons render as text.
	IconLoader func(name string) ui.ImageSource
	// MaxDepth bounds component nesting; zero means 64.
	MaxDepth int
}

// Renderer draws surfaces with native controls.
type Renderer struct {
	store   *a2ui.Store
	opts    Options
	funcs   a2ui.Functions
	maxDeep int

	// theme is the configured palette; frame is the copy View draws with,
	// so a theme change between frames cannot tear a frame in half.
	themeMu sync.RWMutex
	theme   Theme
	frame   Theme

	mu      sync.Mutex
	cache   map[string]a2ui.SurfaceVersion
	fields  map[string]*fieldState
	pending map[string]bool
}

// fieldState is the local value of one form control. Inputs mutate it; the
// data model mirrors it.
type fieldState struct {
	text   string
	num    float64
	flag   bool
	choice string
	multi  []string
	seen   bool
	value  any
}

// New returns a renderer over its own store.
func New(opts Options) *Renderer {
	theme := opts.Theme
	if theme.Text == (ui.Color{}) {
		theme = DefaultTheme()
	}
	if theme.Unit <= 0 {
		theme.Unit = 4
	}
	fns := a2ui.DefaultFunctions()
	for name, fn := range opts.Functions {
		fns[name] = fn
	}
	depth := opts.MaxDepth
	if depth <= 0 {
		depth = 64
	}
	return &Renderer{
		store:   a2ui.NewStore(),
		opts:    opts,
		funcs:   fns,
		theme:   theme,
		maxDeep: depth,
		cache:   map[string]a2ui.SurfaceVersion{},
		fields:  map[string]*fieldState{},
		pending: map[string]bool{},
	}
}

// SetTheme changes the palette the next frame draws with.
func (r *Renderer) SetTheme(theme Theme) {
	if theme.Text == (ui.Color{}) {
		theme = DefaultTheme()
	}
	if theme.Unit <= 0 {
		theme.Unit = 4
	}
	r.themeMu.Lock()
	r.theme = theme
	r.themeMu.Unlock()
	if r.opts.Invalidate != nil {
		r.opts.Invalidate()
	}
}

// Theme returns the configured palette.
func (r *Renderer) Theme() Theme {
	r.themeMu.RLock()
	defer r.themeMu.RUnlock()
	return r.theme
}

// Store exposes the protocol store, for hosts that also feed a web renderer.
func (r *Renderer) Store() *a2ui.Store { return r.store }

// Apply merges messages. It is safe from any goroutine.
func (r *Renderer) Apply(messages ...a2ui.Message) []error {
	var errs []error
	var ui []a2ui.Message
	for _, msg := range messages {
		if err := msg.Validate(); err != nil {
			errs = append(errs, err)
			continue
		}
		switch {
		case msg.CallRendererFunction != nil:
			if err := r.callRendererFunction(msg.CallRendererFunction); err != nil {
				errs = append(errs, err)
			}
		case msg.AgentFunctionResponse != nil:
			r.completeAgentFunction(msg.AgentFunctionResponse)
		default:
			ui = append(ui, msg)
		}
	}
	if len(ui) > 0 {
		errs = append(errs, r.store.Apply(ui...)...)
		r.mu.Lock()
		r.cache = map[string]a2ui.SurfaceVersion{}
		r.mu.Unlock()
	}
	if r.opts.Invalidate != nil {
		r.opts.Invalidate()
	}
	if r.opts.OnError != nil {
		for _, err := range errs {
			r.opts.OnError(err)
		}
	}
	return errs
}

// SupportsCatalog reports whether this renderer draws the catalog natively.
// The basic catalog is drawn natively, with unknown components as
// placeholders; an unknown custom catalog is left to a web renderer.
func (r *Renderer) SupportsCatalog(id string) bool {
	return id == "" || id == a2ui.BasicCatalogID || id == a2ui.NativeCatalog().ID
}

// callRendererFunction executes an agent-requested renderer function and
// reports the result back through OnRendererMessage.
func (r *Renderer) callRendererFunction(call *a2ui.CallRendererFunction) error {
	name := functionName(call.CallFunction)
	if name == "" {
		return fmt.Errorf("a2ui: callRendererFunction without @call")
	}
	fn, ok := r.funcs[name]
	if !ok {
		return fmt.Errorf("a2ui: renderer function %q is not registered", name)
	}
	data, _ := r.store.DataModel(call.SurfaceID)
	args := map[string]any{}
	if raw, ok := call.CallFunction["args"].(map[string]any); ok {
		for k, v := range raw {
			args[k] = a2ui.Resolve(v, data, r.funcs)
		}
	}
	response := &a2ui.FunctionResponse{FunctionCallID: call.FunctionCallID}
	value, err := fn(args)
	if err != nil {
		response.Error = &a2ui.RendererError{Code: "INVALID_FUNCTION_CALL", Message: err.Error()}
	} else {
		response.Value = value
	}
	if r.opts.OnRendererMessage != nil {
		r.opts.OnRendererMessage(a2ui.RendererMessage{FunctionResponse: response})
	}
	return nil
}

// completeAgentFunction resolves a renderer-initiated agent function call.
func (r *Renderer) completeAgentFunction(resp *a2ui.AgentFunctionResponse) {
	r.mu.Lock()
	known := r.pending[resp.FunctionCallID]
	delete(r.pending, resp.FunctionCallID)
	r.mu.Unlock()
	if !known {
		return
	}
	if r.opts.OnAgentFunctionResult != nil {
		var rerr *a2ui.RendererError
		if resp.Error != nil {
			rerr = &a2ui.RendererError{Code: "AGENT_FUNCTION_ERROR", Message: fmt.Sprint(resp.Error["message"]), Details: resp.Error}
		}
		r.opts.OnAgentFunctionResult(resp.FunctionCallID, resp.Value, rerr)
	}
}

// CallAgentFunction asks the agent to run a function. The result arrives via
// Apply as agentFunctionResponse and then OnAgentFunctionResult.
func (r *Renderer) CallAgentFunction(surfaceID, name, catalogID string, args map[string]any) (string, error) {
	callID, err := functionCallID()
	if err != nil {
		return "", err
	}
	r.mu.Lock()
	r.pending[callID] = true
	r.mu.Unlock()
	if r.opts.OnRendererMessage == nil {
		r.mu.Lock()
		delete(r.pending, callID)
		r.mu.Unlock()
		return "", fmt.Errorf("a2ui: no renderer message sink")
	}
	call := map[string]any{"@call": name, "catalogId": catalogID}
	if len(args) > 0 {
		call["args"] = args
	}
	r.opts.OnRendererMessage(a2ui.RendererMessage{CallAgentFunction: &a2ui.CallAgentFunction{
		FunctionCallID: callID, SurfaceID: surfaceID, CallFunction: call,
	}})
	return callID, nil
}

// SupportsComponent reports whether the native catalog draws a component.
// Unknown components still render as placeholders, but a hybrid router can
// send a surface with them to the web renderer instead.
func (r *Renderer) SupportsComponent(name, catalogID string) bool {
	if catalogID != "" && !r.SupportsCatalog(catalogID) {
		return false
	}
	return a2ui.NativeCatalog().Has(name)
}

// View draws one surface at the current frame. It is main-thread only.
func (r *Renderer) View(c *ui.Context, surfaceID string) {
	r.frame = r.Theme()
	snap, ok := r.snapshot(surfaceID)
	if !ok {
		r.placeholder(c, "surface 不存在："+surfaceID)
		return
	}
	s := snap.Surface
	if s == nil || len(s.Components) == 0 {
		r.placeholder(c, "surface 为空："+surfaceID)
		return
	}
	ui.Column(c).FillWidth().Gap(r.frame.Unit * 2).Children(func() {
		r.component(c, s, "root", 0, map[string]bool{})
	})
}

// snapshot returns the cached copy of a surface, refreshing it when the store
// revision changed.
func (r *Renderer) snapshot(id string) (a2ui.SurfaceVersion, bool) {
	rev, exists := r.store.Revision(id)
	r.mu.Lock()
	defer r.mu.Unlock()
	if v, ok := r.cache[id]; ok && exists && v.Revision == rev {
		return v, v.Surface != nil
	}
	if !exists {
		delete(r.cache, id)
		return a2ui.SurfaceVersion{}, false
	}
	v, ok := r.store.Snapshot(id)
	if !ok {
		return a2ui.SurfaceVersion{}, false
	}
	r.cache[id] = v
	return v, true
}

// DataModel returns a surface's current data model.
func (r *Renderer) DataModel(surfaceID string) (any, bool) {
	return r.store.DataModel(surfaceID)
}

// Close releases cached snapshots. The native renderer owns no web views.
func (r *Renderer) Close() {
	r.mu.Lock()
	r.cache = map[string]a2ui.SurfaceVersion{}
	r.fields = map[string]*fieldState{}
	r.mu.Unlock()
}

// component draws one component and returns its element.
func (r *Renderer) component(c *ui.Context, s *a2ui.Surface, id string, depth int, path map[string]bool) ui.Element {
	comp, ok := s.Component(id)
	if !ok {
		return r.placeholder(c, "缺少组件："+id)
	}
	if depth > r.maxDeep || path[id] {
		return r.placeholder(c, "组件嵌套过深："+id)
	}
	next := make(map[string]bool, len(path)+1)
	for k, v := range path {
		next[k] = v
	}
	next[id] = true
	switch comp.Type {
	case "Text", "Title", "Caption", "Label":
		return r.text(c, s, comp)
	case "Row":
		return r.layout(c, s, comp, true, next, depth)
	case "Column", "Box":
		return r.layout(c, s, comp, false, next, depth)
	case "Card", "Section":
		return r.card(c, s, comp, next, depth)
	case "List":
		return r.list(c, s, comp, next, depth)
	case "Divider":
		return ui.Box(c).FillWidth().Height(1).Background(r.frame.Border)
	case "Button":
		return r.button(c, s, comp)
	case "TextField", "TextArea":
		return r.textField(c, s, comp)
	case "CheckBox", "Switch":
		return r.checkBox(c, s, comp)
	case "Slider":
		return r.slider(c, s, comp)
	case "Select", "ChoicePicker", "RadioGroup", "SegmentedControl":
		return r.choice(c, s, comp)
	case "Image":
		return r.image(c, s, comp)
	case "Icon":
		return r.icon(c, s, comp)
	default:
		return r.placeholder(c, "原生渲染暂不支持："+comp.Type)
	}
}

// text renders Text and its aliases. A leading # sequence styles a heading,
// matching the simple Markdown the catalog allows.
func (r *Renderer) text(c *ui.Context, s *a2ui.Surface, comp a2ui.Component) ui.Element {
	raw := a2ui.ResolveString(comp.Prop("text"), s.Data, r.funcs)
	if raw == "" {
		raw = comp.PropString("text")
	}
	variant := strings.ToLower(comp.PropString("variant"))
	size := float32(14)
	weight := 400
	color := r.frame.Text
	if n := headingLevel(raw); n > 0 {
		raw = strings.TrimSpace(raw[n:])
		switch n {
		case 1:
			size, weight = 26, 700
		case 2:
			size, weight = 22, 700
		case 3:
			size, weight = 18, 650
		case 4:
			size, weight = 16, 650
		default:
			size, weight = 15, 600
		}
	}
	switch variant {
	case "h1":
		size, weight = 26, 700
	case "h2":
		size, weight = 22, 700
	case "h3":
		size, weight = 18, 650
	case "h4", "h5":
		size, weight = 16, 600
	case "caption":
		size, color = 12, r.frame.Muted
	}
	if comp.Type == "Title" {
		size, weight = 18, 650
	}
	if comp.Type == "Caption" {
		size, color = 12, r.frame.Muted
	}
	if comp.Type == "Label" {
		size, weight = 13, 600
	}
	return ui.Text(c, raw).FontSize(size).FontWeight(weight).TextColor(color)
}

// headingLevel counts leading # characters in "### Title".
func headingLevel(s string) int {
	n := 0
	for n < len(s) && s[n] == '#' {
		n++
	}
	if n > 0 && n < len(s) && s[n] == ' ' && n <= 6 {
		return n
	}
	return 0
}

// layout renders Row and Column.
func (r *Renderer) layout(c *ui.Context, s *a2ui.Surface, comp a2ui.Component, row bool, path map[string]bool, depth int) ui.Element {
	align := alignOf(comp.PropString("align"), ui.Stretch)
	justify := alignOf(comp.PropString("justify"), ui.Start)
	el := ui.Column(c)
	if row {
		el = ui.Row(c)
	}
	el.FillWidth().Gap(r.gap(comp)).AlignItems(align).Justify(justify).Children(func() {
		r.children(c, s, comp, path, depth)
	})
	return el
}

// card draws a bordered panel.
func (r *Renderer) card(c *ui.Context, s *a2ui.Surface, comp a2ui.Component, path map[string]bool, depth int) ui.Element {
	u := r.frame.Unit
	return ui.Column(c).FillWidth().Padding(u*3).Gap(u*2).Radius(10).
		Background(r.frame.Surface).Border(1, r.frame.Border).Children(func() {
		r.children(c, s, comp, path, depth)
	})
}

// list draws children in order.
func (r *Renderer) list(c *ui.Context, s *a2ui.Surface, comp a2ui.Component, path map[string]bool, depth int) ui.Element {
	u := r.frame.Unit
	return ui.Column(c).FillWidth().Gap(u).Children(func() {
		r.children(c, s, comp, path, depth)
	})
}

// children draws child IDs, plus any single child.
func (r *Renderer) children(c *ui.Context, s *a2ui.Surface, comp a2ui.Component, path map[string]bool, depth int) {
	if comp.Child != "" {
		el := r.component(c, s, comp.Child, depth+1, path)
		if w := comp.Weight(); w > 0 {
			el.Grow(w)
		}
	}
	for _, id := range comp.Children {
		child, ok := s.Component(id)
		el := r.component(c, s, id, depth+1, path)
		if ok {
			if w := child.Weight(); w > 0 {
				el.Grow(w)
			}
		}
	}
}

// gap is the spacing property of a container.
func (r *Renderer) gap(comp a2ui.Component) float32 {
	if g := comp.PropFloat("gap", 0); g > 0 {
		return float32(g)
	}
	return r.frame.Unit * 2
}

// alignOf maps A2UI alignment names to MyGo's.
func alignOf(name string, def ui.Align) ui.Align {
	switch strings.ToLower(name) {
	case "start", "left", "top":
		return ui.Start
	case "center":
		return ui.Center
	case "end", "right", "bottom":
		return ui.End
	case "stretch":
		return ui.Stretch
	case "spacebetween", "space_between":
		return ui.SpaceBetween
	case "spacearound", "space_around":
		return ui.SpaceAround
	case "spaceevenly", "space_evenly":
		return ui.SpaceEvenly
	}
	return def
}

// placeholder draws an explicit fallback instead of hiding a component the
// renderer does not know.
func (r *Renderer) placeholder(c *ui.Context, text string) ui.Element {
	u := r.frame.Unit
	return ui.Text(c, text).FontSize(12).TextColor(r.frame.Muted).
		Padding(u, u*2).Background(r.frame.Surface).Radius(6)
}

// functionName reads the @call name from a callFunction object.
func functionName(call map[string]any) string {
	if name, ok := call["@call"].(string); ok {
		return name
	}
	if nested, ok := call["@call"].(map[string]any); ok {
		name, _ := nested["name"].(string)
		return name
	}
	return ""
}
