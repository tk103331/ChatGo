package kit

import (
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// ToastDuration is how long a toast stays before it fades; errors stay until
// they are closed.
const ToastDuration = 5 * time.Second

// ToastMessage is one notice to show over the window.
type ToastMessage struct {
	Severity uikit.Severity
	Message  string
	// Duration is ToastDuration when zero; errors then stay until closed.
	Duration time.Duration
}

// life is how long m stays, or zero to stay until closed.
func (m ToastMessage) life() time.Duration {
	if m.Duration > 0 {
		return m.Duration
	}
	if m.Severity == uikit.SeverityError {
		return 0
	}
	return ToastDuration
}

// toastState is the window's current toast and when it was shown.
type toastState struct {
	msg   ToastMessage
	shown time.Time
	open  bool
}

type toastKey struct{}

func toasts(c *mgo.Context) *toastState {
	return mgo.Local(c.Root(), toastKey{}, func() toastState { return toastState{} })
}

// ShowToast puts m over the window, replacing any toast already there.
func ShowToast(c *mgo.Context, m ToastMessage) {
	*toasts(c) = toastState{msg: m, shown: c.Now(), open: true}
	c.Invalidate()
}

// Toaster draws the window's toast, never taking the focus; call it last in
// the view each frame.
func Toaster(c *mgo.Context) {
	q := toasts(c)
	if !q.open {
		return
	}
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	w, _ := c.Size()
	closed := false
	mgo.Overlay(c, func() {
		stack := mgo.Column(c).Absolute().Right(u * 4).Bottom(u * 4).
			AlignItems(mgo.End).PassThrough().Role(mgo.RoleGroup).Label("通知")
		stack.Children(func() {
			closed = toastCard(c, k, u, min(360, w-u*8), q.msg)
		})
	})
	if closed {
		q.open = false
		c.Invalidate()
		return
	}
	if life := q.msg.life(); life > 0 {
		if left := life - c.Now().Sub(q.shown); left > 0 {
			c.After(left)
		} else {
			q.open = false
			c.Invalidate()
		}
	}
}

// toastCard builds the toast and reports a close.
func toastCard(c *mgo.Context, k theme.Tokens, u, width float32, m ToastMessage) bool {
	tone, icon := uikit.SeverityLook(k, m.Severity)
	card := mgo.Row(c).Width(width).AlignItems(mgo.Center).Gap(u*3).
		Padding(u*2, u*2, u*2, u*4).Radius(theme.PopupRadius).
		Background(k.Surface).Border(theme.BorderWidth, k.ControlBorder).
		Role(mgo.RoleStatus).Label(m.Message)
	card.Shadow(0, 4, 12, 0, mgo.RGBA(0, 0, 0, 0.14))
	card.Transition(mgo.ElementTransition{
		Duration: theme.PopupDuration,
		Enter:    &mgo.Motion{Y: 8},
		Exit:     &mgo.Motion{},
	})
	card.Draw(func(p *mgo.Painter, r mgo.Rect) {
		p.Fill(mgo.Rect{X: r.X, Y: r.Y + theme.PopupRadius, W: 3, H: r.H - 2*theme.PopupRadius}, tone, 0)
	})
	closed := false
	card.Children(func() {
		mgo.Icon(c, icon).Size(20, 20).TextColor(tone)
		mgo.Text(c, m.Message).TextColor(k.Text).Grow(1).Shrink(1).LineHeight(theme.LineHeight)
		x := Button(c, "", ButtonOptions{Variant: Ghost, Icon: icons.Must("x"), Label: "关闭"})
		x.TextColor(k.TextMuted).Padding(0, u)
		if x.Clicked() {
			closed = true
		}
	})
	if card.Shortcut(0, mgo.KeyEscape) {
		closed = true
	}
	return closed
}

// Status is a system or item state. The zero value is StatusSuccess.
type Status int

const (
	StatusSuccess Status = iota
	StatusWarning
	StatusError
	StatusOffline
	StatusBusy
)

func (s Status) String() string {
	switch s {
	case StatusSuccess:
		return "success"
	case StatusWarning:
		return "warning"
	case StatusError:
		return "error"
	case StatusOffline:
		return "offline"
	case StatusBusy:
		return "busy"
	}
	return "unknown"
}

// StatusIndicatorOptions configure a StatusIndicator.
type StatusIndicatorOptions struct {
	// Label replaces the status's built-in word, as "数据库在线".
	Label string
}

// StatusIndicator shows a status by its own shape and a word, never by color
// alone.
func StatusIndicator(c *mgo.Context, status Status, opts StatusIndicatorOptions) mgo.Element {
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	label := opts.Label
	if label == "" {
		label = statusWord(status)
	}
	var tone mgo.Color
	var icon *mgo.SVG
	switch status {
	case StatusSuccess:
		tone, icon = k.Success, icons.Must("circle-check")
	case StatusWarning:
		tone, icon = k.Warning, icons.Must("triangle-alert")
	case StatusError:
		tone, icon = k.Danger, icons.Must("circle-x")
	case StatusOffline:
		tone, icon = k.TextMuted, icons.Must("wifi-off")
	case StatusBusy:
		tone = k.Info
	}
	row := mgo.Row(c).AlignItems(mgo.Center).Gap(u * 1.5).Role(mgo.RoleStatus).Label(label)
	row.Children(func() {
		if status == StatusBusy {
			uikit.Spinner(c, uikit.SpinnerOptions{Size: 16, Color: tone, Label: label})
		} else {
			mgo.Icon(c, icon).Size(16, 16).TextColor(tone)
		}
		mgo.Text(c, label).TextColor(k.Text).
			FontSize(uikit.FontSize(c, theme.BodySize)).SingleLine()
	})
	return row
}

func statusWord(s Status) string {
	switch s {
	case StatusSuccess:
		return "正常"
	case StatusWarning:
		return "警告"
	case StatusError:
		return "异常"
	case StatusOffline:
		return "离线"
	case StatusBusy:
		return "进行中"
	}
	return ""
}
