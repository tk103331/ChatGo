package kit

import (
	"fmt"
	"slices"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// CollapsibleOptions configure a Collapsible.
type CollapsibleOptions struct {
	// Title is the trigger's text; required unless Trigger is set.
	Title string
	// Trigger builds custom trigger content after the arrow.
	Trigger  func()
	Disabled bool
}

// Collapsible shows content below a trigger while *open; closed content is
// not built.
func Collapsible(c *mgo.Context, open *bool, opts CollapsibleOptions, content func()) ChoiceResult {
	if opts.Title == "" && opts.Trigger == nil {
		panic("kit: Collapsible needs Title or Trigger")
	}
	k := uikit.Tokens(c)
	d := uikit.Density(c)
	u := d.Unit()
	var v ChoiceResult
	v.Element = mgo.Column(c).Shrink(0)
	v.Element.Children(func() {
		was := *open
		p := mgo.CollapsibleBase(c, open)
		tr := p.Trigger
		if opts.Disabled {
			*open = was
		}
		v = ChoiceResult{Element: v.Element, changed: *open != was}
		tr.Disabled(opts.Disabled).AlignSelf(mgo.Start).AlignItems(mgo.Center).Gap(u*2).
			MinHeight(d.ControlHeight()).Padding(0, u*3, 0, u*2).Radius(theme.ControlRadius)
		if opts.Title != "" {
			tr.Label(opts.Title)
		}
		uikit.Ring(tr, k.Focus, theme.ControlRadius)
		if tr.Hovered() && !opts.Disabled {
			tr.Background(k.SurfaceHover)
		}
		prog := tr.Animate("chatgo.open", collapsibleTarget(*open), uikit.Motion(c, theme.StateDuration))
		tr.Children(func() {
			mgo.Icon(c, icons.Must("chevron-right")).Size(16, 16).
				Rotate(90 * prog).TextColor(k.TextMuted)
			if opts.Trigger != nil {
				opts.Trigger()
			} else {
				mgo.Text(c, opts.Title).FontWeight(600).SingleLine()
			}
		})
		if panel := collapsiblePanel(c, *open, prog, content); panel.Valid() {
			panel.Padding(u*2, u*2, u*2, u*8).Gap(u * 2)
		}
	})
	return v
}

func collapsibleTarget(open bool) float32 {
	if open {
		return 1
	}
	return 0
}

// collapsiblePanel builds fn while open or animating, clipped to its progress.
func collapsiblePanel(c *mgo.Context, open bool, prog float32, fn func()) mgo.Element {
	if !open && prog == 0 {
		return mgo.Element{}
	}
	// Closing content takes no input or focus while it shrinks away.
	clip := mgo.Column(c).Shrink(0).Disabled(!open)
	var inner mgo.Element
	clip.Children(func() {
		inner = mgo.Column(c).Shrink(0)
		inner.Children(fn)
	})
	if prog < 1 {
		// The content's height from the last frame, grown by progress.
		clip.ClipY().Height(prog * inner.Bounds().H)
	}
	return inner
}

// KeyValue holds an editable pair and enable flag.
type KeyValue struct {
	Key, Value string
	Enabled    bool
}

// KeyValueInputOptions configure a KeyValueInput.
type KeyValueInputOptions struct {
	// KeyPlaceholder and ValuePlaceholder default to "键" and "值".
	KeyPlaceholder, ValuePlaceholder string
	// Label names the editor for assistive technology.
	Label    string
	Disabled bool
	// MaxHeight caps the rows; zero means 320.
	MaxHeight float32
}

// kvVirtualRows is the row count above which the editor virtualizes.
const kvVirtualRows = 200

// KeyValueInput edits pairs with enable, add and remove.
func KeyValueInput(c *mgo.Context, rows *[]KeyValue, opts KeyValueInputOptions) ChoiceResult {
	if rows == nil || opts.MaxHeight < 0 {
		panic("kit: KeyValueInput needs rows")
	}
	kp, vp := opts.KeyPlaceholder, opts.ValuePlaceholder
	if kp == "" {
		kp = "键"
	}
	if vp == "" {
		vp = "值"
	}
	u := uikit.Unit(c)
	var r ChoiceResult
	r.Element = mgo.Column(c).Shrink(0).Gap(u * 2).Disabled(opts.Disabled).Label(opts.Label)
	locked := r.Element.IsDisabled()
	if locked {
		// A locked editor edits a copy, so the rows it shows never change.
		clone := slices.Clone(*rows)
		rows = &clone
	}
	maxH := opts.MaxHeight
	if maxH == 0 {
		maxH = 320
	}
	remove := -1
	r.Element.Children(func() {
		row := func(i int) {
			kv := &(*rows)[i]
			mgo.Row(c.Key(i)).Gap(u * 2).AlignItems(mgo.Center).Children(func() {
				name := fmt.Sprintf("第 %d 行", i+1)
				on := kv.Enabled
				box := Checkbox(c, &on, "", CheckboxOptions{Disabled: locked, Label: name})
				if box.Changed() && !locked {
					kv.Enabled = on
					r.changed = true
				}
				key := TextInput(c, &kv.Key, TextInputOptions{
					Placeholder: kp, Label: name + " " + kp, Disabled: locked,
				})
				key.Element.Grow(1).Basis(0)
				val := TextInput(c, &kv.Value, TextInputOptions{
					Placeholder: vp, Label: name + " " + vp, Disabled: locked,
				})
				val.Element.Grow(2).Basis(0)
				r.changed = r.changed || key.Changed() || val.Changed()
				if uikit.IconAction(c, icons.Must("x"), fmt.Sprintf("移除第 %d 行", i+1)).
					Disabled(locked).Clicked() && !locked {
					remove = i
				}
			})
		}
		var well mgo.Element
		if n := len(*rows); n > kvVirtualRows {
			well = mgo.List(c, nil, n, row)
		} else {
			well = mgo.Scroll(c).Children(func() {
				for i := range n {
					row(i)
				}
			})
		}
		well.Shrink(1).MaxHeight(maxH)
		add := Button(c, "添加", ButtonOptions{
			Variant: Ghost, Icon: icons.Must("plus"), Disabled: locked,
		})
		if add.AlignSelf(mgo.Start).Clicked() && !locked {
			*rows = append(*rows, KeyValue{Enabled: true})
			r.changed = true
		}
	})
	if remove >= 0 {
		*rows = append((*rows)[:remove:remove], (*rows)[remove+1:]...)
		r.changed = true
	}
	return r
}
