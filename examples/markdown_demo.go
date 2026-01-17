package main

import (
	"chatgo/internal/ui"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
)

func main() {
	a := app.New()
	w := a.NewWindow("Markdown Rendering Demo")

	// Example markdown content
	markdownContent := `# Welcome to ChatGo

This is a **demo** of markdown rendering using Fyne's RichText component.

## Features

- **Bold text** and *italic text*
- ` + "`" + `Inline code` + "`" + `
- [Links](https://github.com)

### Code Block

` + "```go" + `
func main() {
    println("Hello, Fyne!")
}
` + "```" + `

### Lists

1. First item
2. Second item
3. Third item

### Blockquotes

> This is a blockquote
> It can span multiple lines

---

Enjoy using ChatGo!
`

	// Create RichText widget from markdown
	config := ui.DefaultRichTextConfig()
	richText := ui.CreateMarkdownRichText(markdownContent, config)

	// Create a scrollable container
	scroll := container.NewScroll(richText)

	// Add some padding
	content := container.NewPadded(scroll)

	w.SetContent(content)
	w.Resize(fyne.NewSize(800, 600))
	w.ShowAndRun()
}
