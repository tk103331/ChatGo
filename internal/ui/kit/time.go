package kit

import (
	"fmt"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/uikit"
)

// TimeOfDay is a clock time without a date; the zero value is midnight.
type TimeOfDay struct {
	Hour, Minute, Second int
}

// Valid reports whether t is a time between 00:00:00 and 23:59:59.
func (t TimeOfDay) Valid() bool {
	return t.Hour >= 0 && t.Hour < 24 && t.Minute >= 0 && t.Minute < 60 &&
		t.Second >= 0 && t.Second < 60
}

// String formats t as 15:04.
func (t TimeOfDay) String() string { return fmt.Sprintf("%02d:%02d", t.Hour, t.Minute) }

// TimePickerOptions configure a TimePicker.
type TimePickerOptions struct {
	// Label names the picker for assistive technology.
	Label string
	// MinuteStep is the gap between the minutes offered; zero means 5.
	MinuteStep int
	Disabled   bool
}

// TimePicker chooses the hour and minute of *value.
func TimePicker(c *mgo.Context, value *TimeOfDay, opts TimePickerOptions) ChoiceResult {
	if value == nil {
		panic("kit: TimePicker needs a value")
	}
	step := opts.MinuteStep
	if step == 0 {
		step = 5
	}
	if step < 1 || step > 30 {
		panic("kit: TimePicker needs 1 <= MinuteStep <= 30")
	}
	label := opts.Label
	if label == "" {
		label = "时间"
	}
	hours := make([]SelectOption[int], 0, 24)
	for h := range 24 {
		hours = append(hours, SelectOption[int]{Value: h, Label: fmt.Sprintf("%02d", h)})
	}
	minutes := make([]SelectOption[int], 0, 60/step+1)
	for m := 0; m < 60; m += step {
		minutes = append(minutes, SelectOption[int]{Value: m, Label: fmt.Sprintf("%02d", m)})
	}
	// A value off the step keeps its own option, so it shows instead of blank.
	if value.Minute%step != 0 {
		minutes = append(minutes, SelectOption[int]{
			Value: value.Minute, Label: fmt.Sprintf("%02d", value.Minute),
		})
	}
	u := uikit.Unit(c)
	var r ChoiceResult
	r.Element = mgo.Row(c).Gap(u).AlignItems(mgo.Center).Shrink(0).Label(label)
	r.Element.Children(func() {
		h := Select(c, &value.Hour, hours, SelectOptions{
			Label: label + " 时", Disabled: opts.Disabled,
		})
		h.Element.Grow(1).Basis(0)
		mgo.Text(c, ":").TextColor(uikit.Tokens(c).TextMuted).Shrink(0)
		m := Select(c, &value.Minute, minutes, SelectOptions{
			Label: label + " 分", Disabled: opts.Disabled,
		})
		m.Element.Grow(1).Basis(0)
		r.changed = h.Changed() || m.Changed()
	})
	return r
}
