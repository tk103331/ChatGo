package kit

import (
	"strings"

	mgo "github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
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
	k := uikit.Tokens(c)
	th := markdown.DefaultTheme(c.Theme())
	th.CodeFont = theme.MonoFont
	th.Link = k.AccentText
	th.QuoteBar = k.Ornament
	th.QuoteText = k.TextMuted
	th.Rule = k.Border
	th.CodeBlockBorder = k.Border
	th.TableBorder = k.Border
	th.TableHeader = k.SurfaceHover
	for i := range th.Headings {
		th.Headings[i].Color = k.Text
	}
	o.Theme = th
	o.OnLink = func(url string) { c.OpenURL(url) }
	return o
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
