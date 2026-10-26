package kit

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// typingPeriod is one cycle of the dots, in milliseconds.
const typingPeriod = 1200

// TypingIndicator shows that name is typing: three fading dots, or static
// text under reduced motion.
func TypingIndicator(c *mgo.Context, name string) mgo.Element {
	if name == "" {
		panic("kit: TypingIndicator needs a name")
	}
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	label := name + " 正在输入…"
	still := c.Preferences().ReduceMotion
	row := mgo.Row(c).Gap(u*2).AlignItems(mgo.Center).Padding(u, u*3).
		Radius(theme.CardRadius).Background(k.Surface).
		Border(theme.BorderWidth, k.Border).Role(mgo.RoleStatus).
		Label(label).AlignSelf(mgo.Start)
	row.Children(func() {
		if still {
			mgo.Text(c, label).FontSize(uikit.FontSize(c, theme.CaptionSize)).
				TextColor(k.TextMuted).SingleLine()
			return
		}
		muted := k.TextMuted
		mgo.Box(c).Size(30, 12).Draw(func(p *mgo.Painter, r mgo.Rect) {
			// Repaints only while shown: the painter asks for the next frame.
			p.AnimationFrame()
			phase := float64(p.Now().UnixMilli()%typingPeriod) / typingPeriod
			for i := range 3 {
				a := 0.35 + 0.65*(0.5+0.5*math.Cos(2*math.Pi*(phase-float64(i)/6)))
				cx := uikit.Snap(p, r.X+5+float32(i)*10)
				p.Fill(mgo.Rect{X: cx - 3, Y: r.Y + r.H/2 - 3, W: 6, H: 6}, muted.Alpha(float32(a)), 3)
			}
		})
	})
	return row
}

// SendButtonOptions configure a SendButton.
type SendButtonOptions struct {
	// Streaming selects Stop while the caller generates a reply.
	Streaming bool
	// Disabled blocks sending; Stop remains available.
	Disabled bool
	// Shortcut supplies tooltip text; empty hides the hint.
	Shortcut string
}

// SendResult carries the button and requested action.
type SendResult struct {
	Element    mgo.Element
	send, stop bool
}

// Sent requests sending; Stopped requests cancelling the reply.
func (r SendResult) Sent() bool    { return r.send }
func (r SendResult) Stopped() bool { return r.stop }

// SendButton sends the prompt, or stops the reply while streaming.
func SendButton(c *mgo.Context, opts SendButtonOptions) SendResult {
	k := uikit.Tokens(c)
	h := uikit.Density(c).ControlHeight()
	label, icon := "发送", "arrow-up"
	if opts.Streaming {
		label, icon = "停止", "square"
	}
	tip := label
	if opts.Shortcut != "" && !opts.Streaming {
		tip += "（" + opts.Shortcut + "）"
	}
	var r SendResult
	var b mgo.Element
	r.Element = mgo.Row(c).Shrink(0).Disabled(opts.Disabled && !opts.Streaming)
	r.Element.Children(func() {
		b = mgo.ButtonBase(c).Size(h, h).Radius(h / 2).Label(label).
			Tooltip(tip).Transition(uikit.ColorFade)
		uikit.Ring(b, k.Focus, h/2)
		bg, fg := k.Accent, k.AccentText
		switch {
		case opts.Streaming:
			bg, fg = k.Surface, k.Text
			b.Border(theme.BorderWidth, k.Border)
			if b.Hovered() {
				bg = k.SurfaceHover
			}
		case b.Pressed():
			bg = k.AccentPressed
		case b.Hovered():
			bg = k.AccentHover
		}
		b.Background(bg).TextColor(fg)
		b.Children(func() { mgo.Icon(c, icons.Must(icon)).Size(16, 16) })
	})
	if b.Clicked() && !b.IsDisabled() {
		r.send, r.stop = !opts.Streaming, opts.Streaming
	}
	return r
}

// CapabilityCard explains one assistant capability with example prompts.
type CapabilityCard struct {
	Title, Description string
	Icon               *mgo.SVG
	Examples           []string
}

// CapabilityCardsOptions configure CapabilityCards.
type CapabilityCardsOptions struct {
	// Columns is the most cards per row; zero is 3.
	Columns  int
	Disabled bool
}

// CapabilityCardsView is built CapabilityCards and the example clicked this
// frame.
type CapabilityCardsView struct {
	Element mgo.Element
	// Example is the example prompt chosen, empty for none.
	Example string
}

// CapabilityCards shows capabilities; a clicked example goes to the caller.
func CapabilityCards(c *mgo.Context, cards []CapabilityCard, opts CapabilityCardsOptions) CapabilityCardsView {
	if opts.Columns < 0 {
		panic("kit: CapabilityCards Columns is negative")
	}
	cols := opts.Columns
	if cols == 0 {
		cols = 3
	}
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	g := mgo.Grid(c).Columns(cols).Gap(u * 3).Disabled(opts.Disabled)
	v := CapabilityCardsView{Element: g}
	g.Children(func() {
		for i, card := range cards {
			if card.Title == "" {
				panic("kit: CapabilityCard needs a Title")
			}
			box := mgo.Column(c.Key(i)).Gap(u*2).Padding(u*4).Background(k.Surface).
				Border(theme.BorderWidth, k.Border).Radius(theme.CardRadius).
				Role(mgo.RoleGroup).Label(card.Title)
			box.Children(func() {
				mgo.Row(c).Gap(u * 2).AlignItems(mgo.Center).Children(func() {
					if card.Icon != nil {
						mgo.Icon(c, card.Icon).Size(20, 20).TextColor(k.Accent)
					}
					mgo.Text(c, card.Title).
						FontSize(uikit.FontSize(c, theme.H3Size)).TextColor(k.Text)
				})
				if card.Description != "" {
					mgo.Text(c, card.Description).TextColor(k.TextMuted)
				}
				mgo.Column(c).Gap(u).FocusGroup(mgo.Vertical).Children(func() {
					for _, ex := range card.Examples {
						if capabilityExample(c, k, ex) {
							v.Example = ex
						}
					}
				})
			})
		}
	})
	return v
}

// capabilityExample is a quoted example button reporting a click.
func capabilityExample(c *mgo.Context, k *mgo.Theme, text string) bool {
	d := uikit.Density(c)
	b := mgo.ButtonBase(c).MinHeight(d.ControlHeight()).Padding(d.Unit(), d.Unit()*3).
		Justify(mgo.Start).Radius(theme.ControlRadius).Background(k.Background).
		Border(theme.BorderWidth, k.Border).Label(text)
	uikit.Ring(b, k.Focus, theme.ControlRadius)
	switch {
	case b.Pressed():
		b.Background(k.Selection)
	case b.Hovered():
		b.Background(k.SurfaceHover).Border(theme.BorderWidth, k.Border)
	}
	b.Transition(uikit.ColorFade)
	b.Children(func() { mgo.Text(c, "“"+text+"”").TextColor(k.Text) })
	return b.Clicked() && !b.IsDisabled()
}

// WelcomeScreenOptions configure a WelcomeScreen. Greeting is required.
type WelcomeScreenOptions struct {
	Greeting string
	Subtitle string
	Cards    []CapabilityCard
	// Prompts is the slot for starter prompts.
	Prompts  func()
	Disabled bool
}

// WelcomeScreen greets a new conversation with cards and prompts.
func WelcomeScreen(c *mgo.Context, opts WelcomeScreenOptions) CapabilityCardsView {
	if opts.Greeting == "" {
		panic("kit: WelcomeScreen needs a Greeting")
	}
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	var v CapabilityCardsView
	col := mgo.Column(c).FillWidth().MaxWidth(880).AlignSelf(mgo.Center).
		AlignItems(mgo.Center).Gap(u*4).Padding(u*8, u*4).Disabled(opts.Disabled)
	col.Children(func() {
		mgo.Text(c, opts.Greeting).
			FontSize(uikit.FontSize(c, theme.H1Size)).TextColor(k.Text).
			TextAlign(mgo.Center).Role(mgo.RoleText)
		mgo.Divider(c).Width(220)
		if opts.Subtitle != "" {
			mgo.Text(c, opts.Subtitle).TextColor(k.TextMuted).TextAlign(mgo.Center)
		}
		if len(opts.Cards) > 0 {
			v = CapabilityCards(c, opts.Cards, CapabilityCardsOptions{
				Columns: min(3, len(opts.Cards)), Disabled: opts.Disabled,
			})
			v.Element.FillWidth()
		}
		if opts.Prompts != nil {
			mgo.Column(c).FillWidth().AlignItems(mgo.Center).Children(opts.Prompts)
		}
	})
	v.Element = col
	return v
}

// ModelOption is one model of a ModelSelector.
type ModelOption struct {
	// ID is the value chosen; required.
	ID   string
	Name string
	// Capabilities are short tags, as "视觉" or "工具".
	Capabilities []string
	// ContextTokens is the context length; zero hides it.
	ContextTokens int
	// Unavailable explains disabled models; empty permits selection.
	Unavailable string
}

// ModelSelectorOptions configure a ModelSelector.
type ModelSelectorOptions struct {
	// Label names the control for assistive technology.
	Label string
	// Placeholder shows while no option holds the value.
	Placeholder string
	Disabled    bool
}

// ModelSelector picks a model into *id.
func ModelSelector(c *mgo.Context, id *string, models []ModelOption, opts ModelSelectorOptions) ChoiceResult {
	if id == nil {
		panic("kit: ModelSelector needs an id")
	}
	options := make([]SelectOption[string], 0, len(models))
	for _, m := range models {
		if m.ID == "" {
			panic("kit: ModelOption needs an ID")
		}
		label := m.Name
		if label == "" {
			label = m.ID
		}
		if len(m.Capabilities) > 0 {
			label += " · " + strings.Join(m.Capabilities, " ")
		}
		options = append(options, SelectOption[string]{
			Value: m.ID, Label: label, Disabled: m.Unavailable != "",
		})
	}
	label := opts.Label
	if label == "" {
		label = "模型"
	}
	sel := Select(c, id, options, SelectOptions{
		Label: label, Placeholder: opts.Placeholder,
		Disabled: opts.Disabled, Empty: "无可用模型",
	})
	return ChoiceResult{Element: sel.Element, changed: sel.Changed()}
}

// ChatConversation is one conversation of a list.
type ChatConversation struct {
	ID, Title string
	Updated   time.Time
	// Subtitle is a second, muted line under the title; empty keeps the row
	// to one line. The sidebar uses it for the workspace's git branch.
	Subtitle string
	// DiffAdded and DiffDeleted are the workspace's line counts against HEAD,
	// shown colored beside the subtitle.
	DiffAdded, DiffDeleted int
}

// ConversationStatus is a conversation's runtime phase, shown as a light
// beside its title.
type ConversationStatus int

const (
	// ConvIdle shows no light.
	ConvIdle ConversationStatus = iota
	// ConvRunning means the agent is executing: a reply streams or a task runs.
	ConvRunning
	// ConvWaiting means the agent waits for the user, as before a permission.
	ConvWaiting
)

// ConversationItemOptions configure a ConversationItem.
type ConversationItemOptions struct {
	// Today decides the time stamp; zero uses the window clock.
	Today    uikit.Date
	Selected bool
	Disabled bool
	Status   ConversationStatus
}

// ConversationItemView is a built ConversationItem and its requests.
type ConversationItemView struct {
	Element mgo.Element
	// Opened, Deleted and Exported are requests for the caller.
	Opened, Deleted, Exported bool
	// Renamed reports a confirmed new title, already in conv.Title.
	Renamed bool
}

type conversationItemState struct {
	editing, fresh bool
	draft          string
}

type conversationItemKey struct{}

// ConversationItem is a conversation row with rename, export and delete.
func ConversationItem(c *mgo.Context, conv *ChatConversation, opts ConversationItemOptions) ConversationItemView {
	if conv == nil || conv.ID == "" {
		panic("kit: ConversationItem needs a conversation with an ID")
	}
	k := uikit.Tokens(c)
	d := uikit.Density(c)
	u := d.Unit()
	today := opts.Today
	if today.IsZero() {
		today = uikit.DateOf(c.Now())
	}
	row := mgo.Row(c.Key(conv.ID)).FillWidth().MinHeight(d.ControlHeight()).Gap(u).
		AlignItems(mgo.Center).Radius(theme.ControlRadius).Disabled(opts.Disabled)
	v := ConversationItemView{Element: row}
	st := mgo.Local(row, conversationItemKey{}, func() conversationItemState { return conversationItemState{} })
	if row.IsDisabled() {
		st.editing = false
	}
	if opts.Selected {
		row.Background(k.Selection)
	}
	title := conv.Title
	if title == "" {
		title = "新会话"
	}
	rename := false
	row.Children(func() {
		if st.editing {
			conversationItemEditor(c, k, conv, st, &v)
		} else {
			main := mgo.ButtonBase(c).Grow(1).MinWidth(0).MinHeight(d.ControlHeight()).
				Padding(0, u*2).Gap(u * 2).Justify(mgo.Start).
				Radius(theme.ControlRadius).Label(title)
			uikit.Ring(main, k.Focus, theme.ControlRadius)
			if main.Hovered() && !opts.Selected {
				main.Background(k.SurfaceHover)
			}
			main.Transition(uikit.ColorFade)
			main.Children(func() {
				if conv.Subtitle == "" {
					conversationItemMainRow(c, k, opts, title, conv, today)
					return
				}
				// A subtitle makes the row two lines: the status light, title
				// and time on the first, the subtitle under them. The added
				// and deleted line counts ride the second line, in the diff's
				// colors.
				mgo.Column(c).Grow(1).MinWidth(0).Gap(u / 2).Children(func() {
					conversationItemMainRow(c, k, opts, title, conv, today)
					mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
						mgo.Text(c, conv.Subtitle).FontSize(uikit.FontSize(c, theme.CaptionSize)).
							TextColor(k.TextMuted).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
						if conv.DiffAdded > 0 || conv.DiffDeleted > 0 {
							mgo.Text(c, "+"+strconv.Itoa(conv.DiffAdded)).Font(theme.MonoFont).
								FontSize(uikit.FontSize(c, theme.CaptionSize)).TextColor(k.Success).Shrink(0)
							mgo.Text(c, "-"+strconv.Itoa(conv.DiffDeleted)).Font(theme.MonoFont).
								FontSize(uikit.FontSize(c, theme.CaptionSize)).TextColor(k.Danger).Shrink(0)
						}
					})
				})
			})
			if !main.IsDisabled() {
				v.Opened = main.Clicked()
				rename = main.Shortcut(0, mgo.KeyF2)
				v.Deleted = main.Shortcut(0, mgo.KeyDelete)
			}
		}
		conversationItemMenu(c, "会话操作", func(m *PopupMenu) {
			rename = m.Item("重命名", PopupMenuItemOptions{}) || rename
			v.Exported = m.Item("导出 Markdown", PopupMenuItemOptions{})
			m.Separator()
			v.Deleted = m.Item("删除", PopupMenuItemOptions{}) || v.Deleted
		})
	})
	if rename && !row.IsDisabled() {
		st.editing, st.fresh, st.draft = true, true, conv.Title
	}
	return v
}

// conversationItemMainRow is a conversation row's first line: the status
// light, the title and the time it was last updated, laid out horizontally.
func conversationItemMainRow(c *mgo.Context, k *mgo.Theme, opts ConversationItemOptions, title string, conv *ChatConversation, today uikit.Date) {
	u := uikit.Density(c).Unit()
	mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
		switch opts.Status {
		case ConvRunning:
			mgo.Box(c).Size(u*1.5, u*1.5).Shrink(0).Radius(u * 0.75).Background(k.Success)
		case ConvWaiting:
			mgo.Box(c).Size(u*1.5, u*1.5).Shrink(0).Radius(u * 0.75).Background(k.Warning)
		}
		t := mgo.Text(c, title).SingleLine().Grow(1).MinWidth(0)
		if opts.Selected {
			t.TextColor(k.Accent)
		}
		mgo.Text(c, DateSeparatorWhen(conv.Updated, today)).
			FontSize(uikit.FontSize(c, theme.CaptionSize)).
			TextColor(k.TextMuted).Shrink(0)
	})
}

// conversationItemEditor edits the title in place; Escape cancels.
func conversationItemEditor(c *mgo.Context, k *mgo.Theme, conv *ChatConversation, st *conversationItemState, v *ConversationItemView) {
	d := uikit.Density(c)
	in := mgo.TextInputBase(c, &st.draft).Grow(1).MinHeight(d.ControlHeight()-4).
		Padding(0, d.Unit()*2).Radius(theme.ControlRadius).Background(k.Surface).
		Border(theme.BorderWidth, k.Accent).Label("重命名会话")
	uikit.Ring(in, k.Focus, theme.ControlRadius)
	switch {
	case st.fresh:
		st.fresh = false
		in.Focus()
	case in.IsDisabled():
	case in.Submitted():
		if t := strings.TrimSpace(st.draft); t != "" {
			v.Renamed = t != conv.Title
			conv.Title = t
		}
		st.editing = false
	case in.Shortcut(0, mgo.KeyEscape):
		st.editing = false
	}
}

// conversationItemMenu is a borderless ellipsis button opening a menu.
func conversationItemMenu(c *mgo.Context, label string, build func(m *PopupMenu)) mgo.Element {
	b := uikit.IconAction(c, icons.Must("ellipsis"), label).Role(mgo.RoleMenuButton).Shrink(0)
	open := mgo.Local(b, conversationItemKey{}, func() bool { return false })
	if b.IsDisabled() {
		*open = false
	} else if b.Clicked() || b.Shortcut(0, mgo.KeyDown) {
		*open = true
	}
	PopupMenuOpen(c, b, open, build)
	return b
}

// ConversationSearchHit is one match of a conversation search.
type ConversationSearchHit struct {
	ID, Title string
	// Snippet is matching message text, shown under the title.
	Snippet string
}

// ConversationSearchOptions configure a ConversationSearch.
type ConversationSearchOptions struct {
	// Loading shows the caller's search is under way.
	Loading     bool
	Placeholder string
	Label       string
	Disabled    bool
	// Empty replaces the built-in "无匹配会话" text.
	Empty string
}

// ConversationSearchView is a built ConversationSearch and this frame's
// events.
type ConversationSearchView struct {
	Element mgo.Element
	// QueryChanged asks the caller to search for the new query.
	QueryChanged bool
	// Chosen is the hit opened this frame, or empty.
	Chosen string
}

// ConversationSearch edits a query and lists the hits the caller found.
func ConversationSearch(c *mgo.Context, query *string, hits []ConversationSearchHit, opts ConversationSearchOptions) ConversationSearchView {
	if query == nil {
		panic("kit: ConversationSearch needs a query")
	}
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	col := mgo.Column(c).Gap(u * 2).MinHeight(0).Disabled(opts.Disabled)
	v := ConversationSearchView{Element: col}
	col.Children(func() {
		in := SearchInput(c, query, SearchInputOptions{
			Placeholder: opts.Placeholder, Label: opts.Label, Disabled: opts.Disabled,
		})
		in.Element.FillWidth()
		v.QueryChanged = in.Changed()
		switch {
		case opts.Loading:
			uikit.Spinner(c, uikit.SpinnerOptions{Size: 16, Label: "搜索中", ShowLabel: true})
			return
		case *query == "":
			return
		case len(hits) == 0:
			text := opts.Empty
			if text == "" {
				text = "无匹配会话"
			}
			mgo.Text(c, text).FontSize(uikit.FontSize(c, theme.CaptionSize)).
				TextColor(k.TextMuted).Padding(u * 2)
			return
		}
		mgo.Scroll(c).Shrink(1).MinHeight(0).Gap(u).Children(func() {
			for _, hit := range hits {
				if hit.ID == "" {
					panic("kit: ConversationSearchHit needs an ID")
				}
				row := mgo.ButtonBase(c.Key(hit.ID)).FillWidth().MinWidth(0).
					Padding(u, u*2).Justify(mgo.Start).AlignItems(mgo.Start).
					Radius(theme.ControlRadius).Label(hit.Title)
				uikit.Ring(row, k.Focus, theme.ControlRadius)
				if row.Hovered() {
					row.Background(k.SurfaceHover)
				}
				if row.Clicked() && !row.IsDisabled() {
					v.Chosen = hit.ID
				}
				row.Children(func() {
					mgo.Column(c).Grow(1).MinWidth(0).Children(func() {
						mgo.Text(c, hit.Title).SingleLine().Ellipsis("…")
						if hit.Snippet != "" {
							mgo.Text(c, hit.Snippet).
								FontSize(uikit.FontSize(c, theme.CaptionSize)).
								TextColor(k.TextMuted).SingleLine().Ellipsis("…")
						}
					})
				})
			}
		})
	})
	return v
}

// MessageRole is who sent a message.
type MessageRole int

const (
	MessageUser MessageRole = iota
	MessageAssistant
	MessageSystem
	MessageError
)

func (r MessageRole) word() string {
	switch r {
	case MessageUser:
		return "你"
	case MessageAssistant:
		return "助手"
	case MessageSystem:
		return "系统"
	case MessageError:
		return "错误"
	}
	panic(fmt.Sprintf("kit: unknown MessageRole %d", int(r)))
}

// SessionRole is who wrote a message of an exported conversation.
type SessionRole int

const (
	SessionUser SessionRole = iota
	SessionAssistant
	SessionSystem
)

var sessionRoleNames = map[SessionRole]string{
	SessionUser: "用户", SessionAssistant: "助手", SessionSystem: "系统",
}

// SessionMessage is one message of an exported conversation.
type SessionMessage struct {
	Role SessionRole
	// Author names the writer; empty uses the role's name.
	Author string
	Text   string
	Time   time.Time
}

// ExportFormat is an export's text form; the zero value is ExportMarkdown.
type ExportFormat int

const (
	ExportMarkdown ExportFormat = iota
	ExportText
)

// ExportConversation writes messages as Markdown or plain text.
func ExportConversation(title string, messages []SessionMessage, format ExportFormat) string {
	var b strings.Builder
	switch format {
	case ExportMarkdown:
		fmt.Fprintf(&b, "# %s\n", title)
		for _, m := range messages {
			fmt.Fprintf(&b, "\n**%s**%s\n\n%s\n", exportAuthor(m), exportTime(m, " · "), m.Text)
		}
	case ExportText:
		fmt.Fprintf(&b, "%s\n", title)
		for _, m := range messages {
			fmt.Fprintf(&b, "\n%s%s\n%s\n", exportAuthor(m), exportTime(m, " · "), m.Text)
		}
	default:
		panic(fmt.Sprintf("kit: unknown ExportFormat %d", int(format)))
	}
	return b.String()
}

func exportAuthor(m SessionMessage) string {
	if m.Author != "" {
		return m.Author
	}
	name, ok := sessionRoleNames[m.Role]
	if !ok {
		panic("kit: SessionMessage Role out of range")
	}
	return name
}

func exportTime(m SessionMessage, sep string) string {
	if m.Time.IsZero() {
		return ""
	}
	return sep + "(" + m.Time.Format("2006-01-02 15:04") + ")"
}
