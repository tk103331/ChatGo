// Package theme holds the metrics ChatGo's widgets share. Colors come from
// MyGo's own theme (ui.LightTheme and ui.DarkTheme), read through
// uikit.Tokens.
package theme

import "time"

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

// Type sizes in DIPs. Body text takes the window theme's own size; these are
// the steps around it.
const (
	H1Size      float32 = 28
	H2Size      float32 = 22
	H3Size      float32 = 18
	BodySize    float32 = 14
	CaptionSize float32 = 12
	LineHeight  float32 = 1.5

	// MonoFont is the family of code. Headings and body text leave the
	// family empty, which is the system's own.
	MonoFont = "monospace"
)

// Shape metrics in DIPs; the radii match MyGo's own Theme.Radius.
const (
	ControlRadius float32 = 6
	CardRadius    float32 = 6
	PopupRadius   float32 = 8
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
