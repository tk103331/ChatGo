package kit

import (
	"fmt"
	"math"
	"slices"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
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

// Collapsible shows content below a trigger while *open, looking as MyGo's
// own Collapsible does; Trigger allows richer trigger content than a title.
func Collapsible(c *mgo.Context, open *bool, opts CollapsibleOptions, content func()) ChoiceResult {
	if opts.Title == "" && opts.Trigger == nil {
		panic("kit: Collapsible needs Title or Trigger")
	}
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
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
		tr.Disabled(opts.Disabled).AlignSelf(mgo.Start).AlignItems(mgo.Center).
			Gap(u).Padding(u, 0).PaddingStart(u * 0.5).PaddingEnd(u * 2).
			Radius(k.Radius)
		if opts.Title != "" {
			tr.Label(opts.Title)
		}
		if tr.Hovered() && !opts.Disabled {
			tr.Background(k.SurfaceHover)
		}
		tr.Children(func() {
			disclosureArrow(c, k, 90*p.Progress())
			if opts.Trigger != nil {
				opts.Trigger()
			} else {
				mgo.Text(c, opts.Title).SingleLine()
			}
		})
		if panel := p.Panel(content); panel.Valid() {
			// Below the label, as MyGo lays its own out.
			panel.Padding(u, 0).PaddingStart(u * 5.5).Gap(u * 2)
		}
	})
	return v
}

// disclosureArrow draws the arrow of a disclosure: pointing right, turned by
// deg degrees clockwise, as MyGo draws its own.
func disclosureArrow(c *mgo.Context, k *mgo.Theme, deg float32) mgo.Element {
	u := uikit.Unit(c)
	color := k.TextMuted
	return mgo.Box(c).Size(u*4, u*4).Shrink(0).Draw(func(p *mgo.Painter, r mgo.Rect) {
		cx, cy, d := r.X+r.W/2, r.Y+r.H/2, r.W/8
		rad := float64(deg) * math.Pi / 180
		cos, sin := float32(math.Cos(rad)), float32(math.Sin(rad))
		turn := func(x, y float32) (float32, float32) {
			return cx + x*cos - y*sin, cy + x*sin + y*cos
		}
		var path mgo.Path
		x0, y0 := turn(-d, -2*d)
		x1, y1 := turn(d, 0)
		x2, y2 := turn(-d, 2*d)
		path.MoveTo(x0, y0).LineTo(x1, y1).LineTo(x2, y2)
		p.StrokePath(&path, 1.5, color)
	})
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
				box := mgo.Checkbox(c, &on, "").Disabled(locked).Label(name)
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
