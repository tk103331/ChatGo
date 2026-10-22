// markdown_demo renders a sample document with MyGo's native UI.
//
//	go run ./examples/markdown_demo.go
package main

import (
	"log"

	"github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/kit"
	mcore "chatgo/internal/ui/uikit"
)

const markdownContent = `# Welcome to ChatGo

This is a **demo** of markdown rendering with ChatGo's MarkdownView.

## Features

- **Bold text** and *italic text*
- ` + "`Inline code`" + `
- [Links](https://github.com)

### Code Block

` + "```go" + `
func main() {
    println("Hello, MyGo!")
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

type demo struct{}

func (demo) view(c *mgo.Context) {
	mcore.Use(c, mcore.Settings{Locale: mcore.ZhCN})
	mgo.Scroll(c).Fill().Padding(24).Children(func() {
		kit.MarkdownView(c, markdownContent)
	})
}

func main() {
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:   "Markdown Rendering Demo",
			Width:   800,
			Height:  600,
			Content: mgo.View(demo{}.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
