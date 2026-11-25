package native

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"chatgo/pkg/a2ui"
)

// field returns the local state of a bound control, syncing it from the data
// model when the model changed from the outside.
func (r *Renderer) field(surfaceID, componentID string, value any) *fieldState {
	key := surfaceID + "\x00" + componentID
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.fields[key]
	if st == nil {
		st = &fieldState{}
		r.fields[key] = st
	}
	if !st.seen || !a2ui.PointerEquals(st.value, value) {
		st.value = value
		st.seen = true
		st.text = ""
		if s, ok := value.(string); ok {
			st.text = s
		}
		st.num = a2ui.ResolveFloat(value, value, nil)
		if b, ok := value.(bool); ok {
			st.flag = b
		}
		st.choice = ""
		st.multi = nil
		switch v := value.(type) {
		case string:
			st.choice = v
		case []any:
			st.multi = make([]string, 0, len(v))
			for _, item := range v {
				if s, ok := item.(string); ok {
					st.multi = append(st.multi, s)
				}
			}
		}
	}
	return st
}

// pathOf returns the @path binding of a component property.
func pathOf(v any) (string, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return "", false
	}
	path, ok := m["@path"].(string)
	return path, ok
}

// childrenText resolves the text of a button's child Text component.
func (r *Renderer) childText(s *a2ui.Surface, id string) string {
	child, ok := s.Component(id)
	if !ok {
		return ""
	}
	return a2ui.ResolveString(child.Prop("text"), s.Data, r.funcs)
}

// button draws a Button and sends its action when clicked.
func (r *Renderer) button(c *ui.Context, s *a2ui.Surface, comp a2ui.Component) ui.Element {
	label := comp.PropString("label")
	if label == "" && comp.Child != "" {
		label = r.childText(s, comp.Child)
	}
	if label == "" {
		label = a2ui.ResolveString(comp.Prop("text"), s.Data, r.funcs)
	}
	if label == "" {
		label = "按钮"
	}
	variant := strings.ToLower(comp.PropString("variant"))
	el := ui.ButtonBase(c).Padding(7, 14).Radius(7).Gap(6).AlignItems(ui.Center).
		Role(ui.RoleButton).Label(label)
	switch variant {
	case "primary", "filled", "":
		el.Background(r.theme.Accent)
		el.Children(func() {
			ui.Text(c, label).FontSize(13).FontWeight(600).TextColor(ui.RGBA(255, 255, 255, 1))
		})
	default:
		el.Background(r.theme.Surface).Border(1, r.theme.Border)
		el.Children(func() {
			ui.Text(c, label).FontSize(13).FontWeight(500).TextColor(r.theme.Text)
		})
	}
	if el.Clicked() {
		r.fireAction(s, comp)
	}
	return el
}

// fireAction turns a component's action object into an Action.
func (r *Renderer) fireAction(s *a2ui.Surface, comp a2ui.Component) {
	actionProp, _ := comp.Prop("action").(map[string]any)
	event, _ := actionProp["event"].(map[string]any)
	name, _ := event["name"].(string)
	if name == "" {
		return
	}
	action := a2ui.NewAction(s.ID, comp.ID, name)
	action.UserMessage, _ = event["userMessage"].(string)
	if ctx, ok := a2ui.Resolve(event["context"], s.Data, r.funcs).(map[string]any); ok {
		action.Context = ctx
	}
	if r.opts.OnAction != nil {
		r.opts.OnAction(action)
	}
}

// textField draws a single- or multi-line field and writes its value back.
func (r *Renderer) textField(c *ui.Context, s *a2ui.Surface, comp a2ui.Component) ui.Element {
	binding := comp.Prop("value")
	value := a2ui.Resolve(binding, s.Data, r.funcs)
	st := r.field(s.ID, comp.ID, value)
	label := comp.PropString("label")
	variant := strings.ToLower(comp.PropString("variant"))
	placeholder := comp.PropString("placeholder")
	errors := a2ui.CheckErrors(comp, s.Data, r.funcs)
	return ui.Column(c).Gap(2).Children(func() {
		if label != "" {
			ui.Text(c, label).FontSize(12).TextColor(r.theme.Muted)
		}
		var el ui.Element
		if comp.Type == "TextArea" || variant == "longtext" {
			el = ui.TextArea(c, &st.text).Label(label).Placeholder(placeholder)
		} else {
			el = ui.TextInput(c, &st.text).Label(label).Placeholder(placeholder)
			if variant == "obscured" || comp.PropBool("obscured", false) {
				el.Password()
			}
		}
		if len(errors) > 0 {
			el.Error(strings.Join(errors, "；"))
		}
		if el.Changed() {
			if path, ok := pathOf(binding); ok {
				r.store.SetData(s.ID, path, st.text)
			}
			st.value = st.text
		}
	})
}

func (r *Renderer) checkBox(c *ui.Context, s *a2ui.Surface, comp a2ui.Component) ui.Element {
	binding := comp.Prop("value")
	value := a2ui.ResolveBool(binding, s.Data, r.funcs)
	st := r.field(s.ID, comp.ID, value)
	label := comp.PropString("label")
	el := ui.Checkbox(c, &st.flag, label)
	if el.Changed() {
		if path, ok := pathOf(binding); ok {
			r.store.SetData(s.ID, path, st.flag)
		}
		st.value = st.flag
	}
	return el
}

// slider draws a Slider bound to a number.
func (r *Renderer) slider(c *ui.Context, s *a2ui.Surface, comp a2ui.Component) ui.Element {
	binding := comp.Prop("value")
	value := a2ui.ResolveFloat(binding, s.Data, r.funcs)
	st := r.field(s.ID, comp.ID, value)
	lo := comp.PropFloat("min", 0)
	hi := comp.PropFloat("max", 100)
	if hi <= lo {
		hi = lo + 100
	}
	label := comp.PropString("label")
	el := ui.Column(c).Gap(2).Children(func() {
		if label != "" {
			ui.Text(c, label).FontSize(12).TextColor(r.theme.Muted)
		}
		slider := ui.StepSlider(c, &st.num, lo, hi, 1)
		if slider.Changed() {
			if path, ok := pathOf(binding); ok {
				r.store.SetData(s.ID, path, st.num)
			}
			st.value = st.num
		}
	})
	return el
}

// option is one ChoicePicker/Select choice.
type option struct {
	Label string
	Value any
}

// optionsOf decodes the options property.
func (r *Renderer) optionsOf(comp a2ui.Component) []option {
	raw, _ := comp.Prop("options").([]any)
	out := make([]option, 0, len(raw))
	for _, item := range raw {
		m, _ := item.(map[string]any)
		label, _ := m["label"].(string)
		value := m["value"]
		if label == "" {
			label = a2ui.ResolveString(value, nil, nil)
		}
		out = append(out, option{Label: label, Value: value})
	}
	return out
}

// choice draws Select, ChoicePicker, RadioGroup and SegmentedControl.
func (r *Renderer) choice(c *ui.Context, s *a2ui.Surface, comp a2ui.Component) ui.Element {
	binding := comp.Prop("value")
	value := a2ui.Resolve(binding, s.Data, r.funcs)
	opts := r.optionsOf(comp)
	label := comp.PropString("label")
	multiple := strings.EqualFold(comp.PropString("variant"), "multipleSelection") ||
		strings.EqualFold(comp.PropString("variant"), "multipleselection") ||
		comp.Type == "ChoicePicker" && comp.PropBool("multiple", false)
	st := r.field(s.ID, comp.ID, value)
	write := func(v any) {
		if path, ok := pathOf(binding); ok {
			r.store.SetData(s.ID, path, v)
		}
		st.value = v
	}
	if comp.Type == "Select" && len(opts) > 0 {
		labels := make([]string, len(opts))
		for i, o := range opts {
			labels[i] = o.Label
		}
		el := ui.Column(c).Gap(2).Children(func() {
			if label != "" {
				ui.Text(c, label).FontSize(12).TextColor(r.theme.Muted)
			}
			sel := ui.Select(c, &st.choice, labels)
			if sel.Changed() {
				for _, o := range opts {
					if o.Label == st.choice {
						write(o.Value)
						break
					}
				}
			}
		})
		return el
	}
	if multiple {
		if st.multi == nil {
			st.multi = []string{}
			if list, ok := value.([]any); ok {
				for _, item := range list {
					st.multi = append(st.multi, a2ui.ResolveString(item, s.Data, r.funcs))
				}
			}
		}
		selected := map[string]bool{}
		for _, v := range st.multi {
			selected[v] = true
		}
		return ui.Column(c).Gap(2).Children(func() {
			if label != "" {
				ui.Text(c, label).FontSize(12).TextColor(r.theme.Muted)
			}
			for _, o := range opts {
				key := a2ui.ResolveString(o.Value, s.Data, r.funcs)
				on := selected[key]
				el := ui.Checkbox(c, &on, o.Label)
				if el.Changed() {
					next := make([]any, 0, len(opts))
					selected[key] = on
					for _, candidate := range opts {
						k := a2ui.ResolveString(candidate.Value, s.Data, r.funcs)
						if selected[k] {
							next = append(next, candidate.Value)
						}
					}
					st.multi = nil
					write(next)
				}
			}
		})
	}
	return ui.RadioGroup(c, func() {
		if label != "" {
			ui.Text(c, label).FontSize(12).TextColor(r.theme.Muted)
		}
		for _, o := range opts {
			key := a2ui.ResolveString(o.Value, s.Data, r.funcs)
			el := ui.Radio(c, &st.choice, key, o.Label)
			if el.Changed() {
				write(o.Value)
			}
		}
	})
}

// image draws an Image when the host supplied a loader.
func (r *Renderer) image(c *ui.Context, s *a2ui.Surface, comp a2ui.Component) ui.Element {
	src := a2ui.ResolveString(comp.Prop("url"), s.Data, r.funcs)
	if src == "" {
		src = comp.PropString("src")
	}
	alt := comp.PropString("alt")
	if r.opts.ImageLoader != nil && src != "" {
		if img, err := r.opts.ImageLoader(src); err == nil && img != nil {
			return ui.Image(c, img).Fit(ui.Contain).MaxHeight(320).Radius(8)
		}
	}
	return r.placeholder(c, "[图片] "+firstNonEmpty(alt, src))
}

// icon draws an icon when the host supplied a loader.
func (r *Renderer) icon(c *ui.Context, s *a2ui.Surface, comp a2ui.Component) ui.Element {
	name := a2ui.ResolveString(comp.Prop("name"), s.Data, r.funcs)
	if r.opts.IconLoader != nil && name != "" {
		if img := r.opts.IconLoader(name); img != nil {
			return ui.Image(c, img).Fit(ui.Contain).Width(18).Height(18)
		}
	}
	return r.placeholder(c, "[图标] "+name)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
