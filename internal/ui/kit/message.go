package kit

import (
	"fmt"
	"strings"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// MessageListState keeps a MessageList's place and unread count.
type MessageListState struct {
	list    mgo.ListState
	seen    int
	started bool
}

// ScrollToEnd shows the newest message and follows new ones again.
func (s *MessageListState) ScrollToEnd() { s.list.ScrollToEnd() }

// AtEnd reports whether the newest message is in view.
func (s *MessageListState) AtEnd() bool { return s.list.AtEnd() }

// MessageListOptions configure a MessageList.
type MessageListOptions struct {
	// ID returns message i's stable identity; required.
	ID func(i int) string
	// Date returns message i's time; set, a separator heads each new day.
	Date func(i int) time.Time
	// Label returns message i's text for assistive technology and for
	// typing to find a message.
	Label func(i int) string
}

// MessageListResult is a MessageList's element and its unread count.
type MessageListResult struct {
	Element mgo.Element
	unread  int
}

// Unread returns how many messages arrived since the user scrolled away from
// the end.
func (r MessageListResult) Unread() int { return r.unread }

// MessageList shows n messages, building only those in view, and follows the
// end until the user scrolls up.
func MessageList(c *mgo.Context, s *MessageListState, n int, opts MessageListOptions, row func(i int)) MessageListResult {
	if opts.ID == nil || row == nil {
		panic("kit: MessageList needs ID and row")
	}
	if n < 0 {
		panic("kit: MessageList with negative message count")
	}
	u := uikit.Unit(c)
	s.list.Key = func(i int) any { return opts.ID(i) }
	s.list.Label = opts.Label
	s.list.FollowEnd = true
	at := !s.started || s.list.AtEnd()
	if at || n < s.seen {
		s.seen = n
	}
	s.started = true
	r := MessageListResult{unread: n - s.seen}
	r.Element = mgo.Box(c).Grow(1).MinHeight(0)
	r.Element.Children(func() {
		mgo.List(c, &s.list, n, func(i int) {
			mgo.Column(c).Padding(u*2, u*4).Gap(u * 2).Children(func() {
				if opts.Date != nil && (i == 0 || !sameDay(opts.Date(i-1), opts.Date(i))) {
					messageListDay(c, opts.Date(i))
				}
				row(i)
			})
		}).Fill().Justify(mgo.End).Label("消息")
		jump := ScrollToBottomButton(c, !at, r.unread)
		if jump.Clicked() && !r.Element.IsDisabled() {
			s.list.ScrollToEnd()
			s.seen = n
		}
	})
	return r
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// messageListDay is a separator naming the day: today, yesterday or the date.
func messageListDay(c *mgo.Context, t time.Time) {
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	label := t.Format("2006年1月2日")
	now := c.Now().In(t.Location())
	switch {
	case sameDay(t, now):
		label = "今天"
	case sameDay(t, now.AddDate(0, 0, -1)):
		label = "昨天"
	}
	mgo.Row(c).Gap(u*3).AlignItems(mgo.Center).Padding(u*2, 0).
		Role(mgo.RoleText).Label(label).Children(func() {
		rule := func() {
			mgo.Box(c).Grow(1).Height(theme.BorderWidth).Background(k.Border)
		}
		rule()
		mgo.Text(c, label).FontSize(uikit.FontSize(c, theme.CaptionSize)).
			TextColor(k.TextMuted).SingleLine()
		rule()
	})
}

// ScrollToBottomButton floats over a list scrolled away from its end, with
// the unread count when there is one.
func ScrollToBottomButton(c *mgo.Context, away bool, unread int) mgo.Element {
	if unread < 0 {
		panic("kit: ScrollToBottomButton unread must not be negative")
	}
	if !away {
		return mgo.Box(c).Absolute().Invisible()
	}
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	label := "回到最新"
	if unread > 0 {
		label = fmt.Sprintf("%d 条新消息", unread)
	}
	wrap := mgo.Row(c).Absolute().Left(0).Right(0).Bottom(u * 3).
		Justify(mgo.Center).PassThrough()
	var b mgo.Element
	wrap.Children(func() {
		b = mgo.ButtonBase(c).MinHeight(theme.MinHitSize).MinWidth(theme.MinHitSize).
			Padding(0, u*3).Gap(u*2).Radius(theme.MinHitSize/2).Label(label).
			Background(k.Surface).Border(theme.BorderWidth, k.Border).
			TextColor(k.Text).Shadow(0, 2, 8, 0, mgo.RGBA(0, 0, 0, 0.14)).
			Transition(uikit.ColorFade)
		b.Opacity(uikit.OverlayEnter(c, b))
		uikit.Ring(b, k.Focus, theme.MinHitSize/2)
		if b.Hovered() {
			b.Background(k.SurfaceHover)
		}
		b.Children(func() {
			mgo.Icon(c, icons.Must("arrow-down")).Size(16, 16)
			if unread > 0 {
				mgo.Text(c, fmt.Sprint(unread)).FontFeatures("tnum").
					FontWeight(600).TextColor(k.AccentText).SingleLine()
			}
		})
	})
	return b
}

// MessageBubbleOptions configure a MessageBubble.
type MessageBubbleOptions struct {
	// Name shows above the bubble; empty shows none.
	Name string
	// Avatar builds the picture beside the bubble.
	Avatar func()
	// MaxWidth caps the bubble in DIPs; zero means 560.
	MaxWidth float32
}

// MessageBubble frames one chat message by role: users right, others left,
// system centered.
func MessageBubble(c *mgo.Context, role MessageRole, opts MessageBubbleOptions, content func()) mgo.Element {
	if opts.MaxWidth < 0 {
		panic("kit: MessageBubble MaxWidth must not be negative")
	}
	if content == nil {
		panic("kit: MessageBubble needs content")
	}
	maxW := opts.MaxWidth
	if maxW == 0 {
		maxW = 560
	}
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	label := role.word()
	if opts.Name != "" {
		label += ", " + opts.Name
	}
	row := mgo.Row(c).Gap(u * 2).AlignItems(mgo.Start).Role(mgo.RoleGroup).Label(label)
	switch role {
	case MessageUser:
		row.Justify(mgo.End)
	case MessageSystem:
		row.Justify(mgo.Center)
	}
	row.Children(func() {
		if role == MessageSystem {
			mgo.Row(c).MaxWidth(maxW).Padding(u, u*3).Gap(u * 2).
				Radius(theme.ControlRadius).Background(k.SurfaceHover).
				TextColor(k.TextMuted).FontSize(uikit.FontSize(c, theme.CaptionSize)).
				Children(content)
			return
		}
		if opts.Avatar != nil && role != MessageUser {
			opts.Avatar()
		}
		align := mgo.Start
		if role == MessageUser {
			align = mgo.End
		}
		mgo.Column(c).Gap(u).MaxWidth(maxW).Shrink(1).MinWidth(0).
			AlignItems(align).Children(func() {
			if opts.Name != "" {
				mgo.Text(c, opts.Name).FontSize(uikit.FontSize(c, theme.CaptionSize)).
					TextColor(k.TextMuted).SingleLine()
			}
			b := mgo.Column(c).Padding(u*2, u*3).Gap(u * 2).
				Radius(theme.CardRadius).MinWidth(0)
			switch role {
			case MessageUser:
				b.Background(k.Selection)
			case MessageError:
				b.Background(k.Surface).Border(theme.BorderWidth, k.Danger)
			default:
				b.Background(k.Surface).Border(theme.BorderWidth, k.Border)
			}
			b.Children(func() {
				if role != MessageError {
					content()
					return
				}
				mgo.Row(c).Gap(u * 2).AlignItems(mgo.Start).Children(func() {
					mgo.Icon(c, icons.Must("circle-alert")).Size(16, 16).
						Shrink(0).TextColor(k.Danger)
					mgo.Column(c).Gap(u * 2).Grow(1).MinWidth(0).Children(content)
				})
			})
		})
		if opts.Avatar != nil && role == MessageUser {
			opts.Avatar()
		}
	})
	return row
}

// MessageStatus is where a chat message is in its delivery.
type MessageStatus int

const (
	// MessageNoStatus shows no status.
	MessageNoStatus MessageStatus = iota
	MessageSending
	MessageStreaming
	MessageSent
	MessageFailed
)

// MessageHeaderOptions configure a MessageHeader; zero fields are hidden.
type MessageHeaderOptions struct {
	Model  string
	Time   time.Time
	Status MessageStatus
}

// MessageHeader shows a message's model, time and status as icon and text.
func MessageHeader(c *mgo.Context, opts MessageHeaderOptions) mgo.Element {
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	small := uikit.FontSize(c, theme.CaptionSize)
	row := mgo.Row(c).Gap(u * 2).AlignItems(mgo.Center).FontSize(small).TextColor(k.TextMuted)
	row.Children(func() {
		if opts.Model != "" {
			mgo.Text(c, opts.Model).FontWeight(600).TextColor(k.Text).SingleLine()
		}
		if !opts.Time.IsZero() {
			mgo.Text(c, opts.Time.Format("15:04")).FontFeatures("tnum").SingleLine()
		}
		if opts.Status == MessageNoStatus {
			return
		}
		word, icon, color := "", "", k.TextMuted
		switch opts.Status {
		case MessageSending:
			word, icon = "发送中", "clock"
		case MessageStreaming:
			word = "生成中"
		case MessageSent:
			word, icon = "已发送", "check"
		case MessageFailed:
			word, icon, color = "发送失败", "circle-alert", k.Danger
		default:
			panic("kit: unknown MessageStatus")
		}
		mgo.Row(c).Gap(u).AlignItems(mgo.Center).TextColor(color).Children(func() {
			if icon == "" {
				uikit.Spinner(c, uikit.SpinnerOptions{Size: 12, Label: word})
			} else {
				mgo.Icon(c, icons.Must(icon)).Size(12, 12)
			}
			mgo.Text(c, word).SingleLine()
		})
	})
	return row
}

// MessageEditorOptions configure a MessageEditor.
type MessageEditorOptions struct {
	// Original is the sent text; saving it unchanged is not allowed.
	Original string
}

// MessageEditorResult is the editor's element and its outcome this frame.
type MessageEditorResult struct {
	Element         mgo.Element
	saved, canceled bool
}

// Saved reports a save; the caller makes a new branch from the draft.
func (r MessageEditorResult) Saved() bool { return r.saved }

// Canceled reports Cancel or Escape.
func (r MessageEditorResult) Canceled() bool { return r.canceled }

// MessageEditor edits a sent message in *draft; Cmd+Enter saves, Escape
// cancels.
func MessageEditor(c *mgo.Context, draft *string, opts MessageEditorOptions) MessageEditorResult {
	if draft == nil {
		panic("kit: MessageEditor needs a draft")
	}
	u := uikit.Unit(c)
	var r MessageEditorResult
	r.Element = mgo.Column(c).Gap(u * 2).MinWidth(0)
	disabled := r.Element.IsDisabled()
	r.Element.Children(func() {
		area := TextArea(c, draft, TextAreaOptions{Label: "编辑消息", MaxHeight: 240})
		ok := strings.TrimSpace(*draft) != "" && *draft != opts.Original
		escape := area.Input.Shortcut(0, mgo.KeyEscape)
		mgo.Row(c).Gap(u * 2).Justify(mgo.End).Children(func() {
			if Button(c, "取消", ButtonOptions{Variant: Secondary}).Clicked() || escape {
				r.canceled = !disabled
			}
			if Button(c, "保存", ButtonOptions{Disabled: !ok}).Clicked() || area.Submitted() && ok {
				r.saved = !disabled
			}
		})
	})
	return r
}
