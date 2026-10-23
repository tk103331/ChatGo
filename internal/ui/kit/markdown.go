package kit

import (
	"strings"

	mgo "github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"chatgo/internal/ui/theme"
)

// markdownOptions is the window's Markdown look and link handling.
//
// Options carries the parse and image caches, so the same value is kept on
// the window root and handed back each frame rather than copied: a copy would
// drop the caches every frame.
func markdownOptions(c *mgo.Context) *markdown.Options {
	o := *mgo.Local(c.Root(), "chatgo.markdown", func() *markdown.Options {
		return &markdown.Options{}
	})
	// DefaultTheme derives every color from the window theme, so the view
	// follows ui.LightTheme and ui.DarkTheme with nothing set here.
	th := markdown.DefaultTheme(c.Theme())
	th.CodeFont = theme.MonoFont
	o.Theme = th
	o.OnLink = func(url string) {
		if h := linkOpener(c); h != nil {
			h(url)
			return
		}
		c.OpenURL(url)
	}
	return o
}

// previewMarkdownOptions is markdownOptions whose links go to the preview's
// web view, so a document read in the panel does not leave the app.
func previewMarkdownOptions(c *mgo.Context) *markdown.Options {
	o := *markdownOptions(c)
	o.OnLink = func(url string) {
		if h := previewLinkOpener(c); h != nil {
			h(url)
			return
		}
		c.OpenURL(url)
	}
	return &o
}

// linkOpener returns the window's link handler, which shows a link in the
// app's web view rather than the desktop browser; nil keeps the browser.
func linkOpener(c *mgo.Context) func(string) {
	v := mgo.Local(c.Root(), "chatgo.linkopener", func() func(string) { return nil })
	if v == nil {
		return nil
	}
	return *v
}

// SetLinkOpener hands the window's Markdown views their link handler. Passing
// nil restores the desktop browser.
func SetLinkOpener(c *mgo.Context, fn func(string)) {
	v := mgo.Local(c.Root(), "chatgo.linkopener", func() func(string) { return nil })
	if v != nil {
		*v = fn
	}
}

// previewLinkOpener is the preview column's link handler, tried before the
// window's own; nil falls back to it.
func previewLinkOpener(c *mgo.Context) func(string) {
	v := mgo.Local(c.Root(), "chatgo.previewlinkopener", func() func(string) { return nil })
	if v == nil {
		return nil
	}
	return *v
}

// SetPreviewLinkOpener hands the preview column's Markdown views their link
// handler. Passing nil restores the window's own.
func SetPreviewLinkOpener(c *mgo.Context, fn func(string)) {
	v := mgo.Local(c.Root(), "chatgo.previewlinkopener", func() func(string) { return nil })
	if v != nil {
		*v = fn
	}
}

// MarkdownPreview shows finished Markdown whose links open in the preview's
// web view.
func MarkdownPreview(c *mgo.Context, src string) mgo.Element {
	return markdown.View(c, src, previewMarkdownOptions(c)).MinWidth(0)
}

// MarkdownView shows finished Markdown, following the window's theme. Links
// open in the desktop browser.
func MarkdownView(c *mgo.Context, src string) mgo.Element {
	return markdown.View(c, src, markdownOptions(c)).MinWidth(0)
}

// MarkdownStream shows Markdown while it streams in, revealing it by word as
// it arrives.
func MarkdownStream(c *mgo.Context, src string) mgo.Element {
	return markdown.Stream(c, src, markdownOptions(c)).MinWidth(0)
}

// CodeBlockOptions configure a CodeBlock.
type CodeBlockOptions struct {
	// Language picks the highlighting, as "go" or "python".
	Language string
	// Wrap wraps long lines rather than scrolling them sideways.
	Wrap bool
}

// CodeBlock shows code with a language label, highlighting and a copy button.
//
// It wraps the code in a fence and hands it to the Markdown view, which
// highlights code blocks incrementally and gives them their copy button.
func CodeBlock(c *mgo.Context, code string, opts CodeBlockOptions) mgo.Element {
	o := markdownOptions(c)
	o.WrapCode = opts.Wrap
	var b strings.Builder
	b.WriteString("```")
	b.WriteString(opts.Language)
	b.WriteByte('\n')
	b.WriteString(code)
	if !strings.HasSuffix(code, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("```")
	return markdown.View(c, b.String(), o).MinWidth(0)
}
