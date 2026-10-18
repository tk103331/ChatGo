// Package chatpanel provides a reusable conversation panel: a scrolling
// message list above a prompt composer. The panel owns its interaction
// state (draft text, scroll position, in-place editing) and renders
// messages supplied by the host through Options.
//
// A panel is embedded in any view that shows a conversation — the main
// chat area, the focused quick window, or an app's chat pane — and is
// driven entirely through the Options callbacks, so the host decides
// where messages come from, what the toolbar shows and what sending does.
package chatpanel

import (
	"strings"
	"time"

	"chatgo/pkg/models"

	mgo "github.com/egoist/mygo/ui"

	mchat "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/chat"
	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// DefaultMaxWidth is the default maximum content column width.
const DefaultMaxWidth float32 = 768

// Panel is a reusable conversation panel. The zero value is not ready;
// use New.
type Panel struct {
	// List is the message list scroll state.
	List mchat.MessageListState
	// Draft is the composer's text.
	Draft string
	// Editing is the index of the message being edited in place, or -1.
	Editing int
	// EditDraft is the in-place edit buffer.
	EditDraft string

	mentionRoot  string
	mentionFiles []mchat.ContextItem
}

// New returns a Panel ready to use.
func New() *Panel {
	return &Panel{Editing: -1}
}

// Reset clears the interaction state for a new conversation.
func (p *Panel) Reset() {
	p.Draft = ""
	p.Editing = -1
	p.EditDraft = ""
	p.List.ScrollToEnd()
}

// InsertMention appends an @-mention to the draft.
func (p *Panel) InsertMention(it mchat.ContextItem) {
	if p.Draft != "" && !strings.HasSuffix(p.Draft, " ") {
		p.Draft += " "
	}
	p.Draft += "@" + it.Label + " "
}

// Options supplies the panel with data and actions from its host.
// Function fields may be nil where noted; the panel adapts.
type Options struct {
	// Placeholder is the composer's placeholder text.
	Placeholder string
	// MaxWidth caps the content column width; zero uses DefaultMaxWidth.
	MaxWidth float32
	// ComposerMaxHeight caps the composer before it scrolls; zero uses 220.
	ComposerMaxHeight float32

	// Messages returns the messages to display.
	Messages func() []models.Message
	// Streaming reports whether the last assistant message is streaming.
	Streaming func() bool
	// Status returns the error/status line text, or "".
	Status func() string

	// Avatar builds the assistant avatar for message bubbles; nil omits it.
	Avatar func(c *mgo.Context) func()
	// ModelLabel names the model that produced a message, or "".
	ModelLabel func(msg models.Message) string
	// TokenLabel formats a message's token usage, or ""; nil omits usage.
	TokenLabel func(msg models.Message) string

	// Toolbar builds the composer's left controls (model, tools, skills…).
	Toolbar func(c *mgo.Context)
	// Loading reports whether the composer is busy starting (e.g. an ACP
	// process); the send button is replaced by a spinner.
	Loading func() bool
	// LoadingText is shown next to the loading spinner.
	LoadingText string

	// Send delivers the drafted text. The host clears the draft on success.
	Send func(text string)
	// Stop cancels the streaming reply.
	Stop func()
	// Regenerate asks for a new reply to the message at index i.
	Regenerate func(i int)
	// EditSaved applies an in-place edit: replaces message i with text and
	// re-answers. If nil, editing is disabled.
	EditSaved func(i int, text string)

	// SlashCommands returns the available slash commands; nil disables the
	// slash menu.
	SlashCommands func() []mchat.SlashCommand
	// OnSlashCommand handles a chosen slash command by name.
	OnSlashCommand func(name string)
	// ContextRoot returns the directory whose files are @-mentionable; ""
	// or nil disables the mention menu.
	ContextRoot func() string

	// OnCopy is called after the user copies a message; nil does nothing.
	OnCopy func(text string)
	// Invalidate requests a redraw.
	Invalidate func()
}

// View renders the panel: the message list above the composer.
func (p *Panel) View(c *mgo.Context, opts Options) {
	msgs := opts.Messages()
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	maxWidth := opts.MaxWidth
	if maxWidth == 0 {
		maxWidth = DefaultMaxWidth
	}
	row := func(i int) { p.messageRow(c, i, msgs[i], len(msgs), opts) }
	mgo.Column(c).Fill().AlignItems(mgo.Stretch).Background(k.Background).Children(func() {
		mgo.Row(c).Grow(1).MinHeight(0).Justify(mgo.Center).Children(func() {
			mgo.Box(c).MaxWidth(maxWidth).Grow(1).MinWidth(0).Children(func() {
				res := mchat.MessageList(c, &p.List, len(msgs), mchat.MessageListOptions{
					ID:    func(i int) string { return msgs[i].ID },
					Date:  func(i int) time.Time { return msgs[i].Timestamp },
					Label: func(i int) string { return msgs[i].Role },
				}, row)
				res.Element.Grow(1).MinHeight(0)
			})
		})
		mgo.Column(c).Shrink(0).Padding(u*3, u*4).BorderWidth(mtheme.BorderWidth, 0, 0, 0).
			BorderColor(k.Border).Children(func() {
			mgo.Row(c).Justify(mgo.Center).Children(func() {
				mgo.Box(c).MaxWidth(maxWidth).Grow(1).MinWidth(0).Children(func() { p.composer(c, opts) })
			})
		})
	})
}

// composer builds the message input with its toolbar and send controls.
func (p *Panel) composer(c *mgo.Context, opts Options) {
	loading := opts.Loading != nil && opts.Loading()
	maxHeight := opts.ComposerMaxHeight
	if maxHeight == 0 {
		maxHeight = 220
	}
	in := Composer(c, &p.Draft, ComposerOptions{
		Placeholder: opts.Placeholder,
		MaxHeight:   maxHeight,
		Disabled:    loading,
		Tools: func() {
			if opts.Toolbar != nil {
				opts.Toolbar(c)
			}
		},
		Actions: func() {
			if loading {
				u := mcore.Density(c).Unit()
				mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
					mcore.Spinner(c, mcore.SpinnerOptions{Size: 12})
					text := opts.LoadingText
					if text == "" {
						text = "正在初始化…"
					}
					mgo.Text(c, text).
						FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(mcore.Tokens(c).TextMuted)
				})
				return
			}
			r := mchat.SendButton(c, mchat.SendButtonOptions{
				Streaming: opts.Streaming != nil && opts.Streaming(),
				Disabled:  strings.TrimSpace(p.Draft) == "",
			})
			if r.Sent() {
				p.send(opts)
			}
			if r.Stopped() && opts.Stop != nil {
				opts.Stop()
			}
		},
	})
	if !loading {
		p.menus(c, in, opts)
	}
	if in.Submitted() && !loading {
		p.send(opts)
	}
	if opts.Status != nil {
		if s := opts.Status(); s != "" {
			mgo.Text(c, s).TextColor(mcore.Tokens(c).Danger).
				FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		}
	}
}

// send delivers the trimmed draft to the host.
func (p *Panel) send(opts Options) {
	text := strings.TrimSpace(p.Draft)
	if text == "" || opts.Send == nil {
		return
	}
	opts.Send(text)
}
