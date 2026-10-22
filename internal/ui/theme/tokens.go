// Package theme defines ChatGo's design tokens: colors, type, metrics and
// motion, plus the color themes and appearance-preset parsing.
//
// The palette is ChatGo's own ivory / charcoal with a wine-red accent. Tokens
// is a superset of MyGo's ui.Theme: Use installs the matching ui.Theme on the
// window so native widgets follow along, and Tokens(c) hands the full set to
// hand-drawn parts.
package theme

import (
	"time"

	mgo "github.com/egoist/mygo/ui"
)

// Tokens are the colors of one appearance.
type Tokens struct {
	Dark          bool
	Background    mgo.Color
	Surface       mgo.Color
	SurfaceHover  mgo.Color
	Text          mgo.Color
	TextMuted     mgo.Color
	Border        mgo.Color
	ControlBorder mgo.Color
	Accent        mgo.Color
	AccentHover   mgo.Color
	AccentPressed mgo.Color
	OnAccent      mgo.Color
	AccentText    mgo.Color
	Selection     mgo.Color
	Focus         mgo.Color
	Ornament      mgo.Color
	Success       mgo.Color
	Warning       mgo.Color
	Danger        mgo.Color
	Info          mgo.Color
	DangerFill    mgo.Color
	OnDanger      mgo.Color
}

// Light returns the ivory appearance.
func Light() Tokens {
	return Tokens{
		Background:    mgo.Hex("#F2EBDD"),
		Surface:       mgo.Hex("#FBF7EF"),
		SurfaceHover:  mgo.Hex("#E8DFD0"),
		Text:          mgo.Hex("#211C1D"),
		TextMuted:     mgo.Hex("#6A5B55"),
		Border:        mgo.Hex("#D2C3AF"),
		ControlBorder: mgo.Hex("#877662"),
		Accent:        mgo.Hex("#7D2034"),
		AccentHover:   mgo.Hex("#651729"),
		AccentPressed: mgo.Hex("#511020"),
		OnAccent:      mgo.Hex("#FFF8EE"),
		AccentText:    mgo.Hex("#7D2034"),
		Selection:     mgo.Hex("#EAD7D9"),
		Focus:         mgo.Hex("#7D2034"),
		Ornament:      mgo.Hex("#AD9164"),
		Success:       mgo.Hex("#326247"),
		Warning:       mgo.Hex("#805714"),
		Danger:        mgo.Hex("#A12635"),
		Info:          mgo.Hex("#355E80"),
		DangerFill:    mgo.Hex("#A12635"),
		OnDanger:      mgo.Hex("#FFF8EE"),
	}
}

// Dark returns the charcoal appearance.
func Dark() Tokens {
	return Tokens{
		Dark:          true,
		Background:    mgo.Hex("#191618"),
		Surface:       mgo.Hex("#242024"),
		SurfaceHover:  mgo.Hex("#342C32"),
		Text:          mgo.Hex("#F2EBDD"),
		TextMuted:     mgo.Hex("#BCAEAA"),
		Border:        mgo.Hex("#51424A"),
		ControlBorder: mgo.Hex("#968278"),
		Accent:        mgo.Hex("#9C3C53"),
		AccentHover:   mgo.Hex("#AA425C"),
		AccentPressed: mgo.Hex("#813047"),
		OnAccent:      mgo.Hex("#FFF8EE"),
		AccentText:    mgo.Hex("#E4A0AE"),
		Selection:     mgo.Hex("#492A36"),
		Focus:         mgo.Hex("#E4A0AE"),
		Ornament:      mgo.Hex("#BDA172"),
		Success:       mgo.Hex("#94C6A4"),
		Warning:       mgo.Hex("#DFC080"),
		Danger:        mgo.Hex("#F0A1AA"),
		Info:          mgo.Hex("#9DBDDA"),
		DangerFill:    mgo.Hex("#A12635"),
		OnDanger:      mgo.Hex("#FFF8EE"),
	}
}

// HighContrast returns t with secondary text and borders raised for readability.
func (t Tokens) HighContrast() Tokens {
	t.TextMuted = t.Text
	t.Border = t.ControlBorder
	return t
}

// Density sets control heights and the spacing unit.
type Density int

const (
	DefaultDensity Density = iota
	Compact
	Comfortable
)

// Unit is the base spacing unit in DIPs.
func (d Density) Unit() float32 {
	switch d {
	case Compact:
		return 3
	case Comfortable:
		return 5
	}
	return 4
}

// ControlHeight is the minimum height of single-line controls and data rows.
func (d Density) ControlHeight() float32 {
	switch d {
	case Compact:
		return 28
	case Comfortable:
		return 44
	}
	return 36
}

func (d Density) String() string {
	switch d {
	case Compact:
		return "Compact"
	case Comfortable:
		return "Comfortable"
	}
	return "Default"
}

// Type sizes and families.
const (
	H1Size      float32 = 28
	H2Size      float32 = 22
	H3Size      float32 = 18
	BodySize    float32 = 14
	CaptionSize float32 = 12
	LineHeight  float32 = 1.5

	SerifFont = "Georgia, Songti SC, SimSun, Noto Serif CJK SC, serif"
	MonoFont  = "monospace"
)

// Shape metrics in DIPs.
const (
	ControlRadius float32 = 2
	CardRadius    float32 = 4
	PopupRadius   float32 = 6
	BorderWidth   float32 = 1
	FocusWidth    float32 = 2
	FocusGap      float32 = 2
	MinHitSize    float32 = 28
)

// Motion durations.
const (
	StateDuration = 120 * time.Millisecond
	PopupDuration = 160 * time.Millisecond
)
