package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// RichTextConfig holds configuration for markdown rendering
type RichTextConfig struct {
	Wrapping   fyne.TextWrap
	TextColor  color.Color
	Inline     bool
	Hyperlinks bool
}

// DefaultRichTextConfig returns default configuration for markdown rendering
func DefaultRichTextConfig() *RichTextConfig {
	return &RichTextConfig{
		Wrapping:   fyne.TextWrapWord,
		TextColor:  nil, // Use default theme color
		Inline:     false,
		Hyperlinks: true,
	}
}

// CreateMarkdownRichText creates a RichText widget configured for markdown rendering
func CreateMarkdownRichText(markdown string, config *RichTextConfig) *widget.RichText {
	richText := widget.NewRichTextFromMarkdown(markdown)

	if config != nil {
		richText.Wrapping = config.Wrapping

		// Apply text color if specified
		if config.TextColor != nil {
			// RichText doesn't have a direct color property, but we can configure segments
			// For now, we'll keep the default theme color
		}
	}

	return richText
}

// CreateMessageBubble creates a styled container for chat messages with markdown content
func CreateMessageBubble(content string, isUser bool) *fyne.Container {
	config := DefaultRichTextConfig()
	richText := CreateMarkdownRichText(content, config)

	// Create a border around the message
	border := canvas.NewRectangle(color.NRGBA{R: 200, G: 200, B: 200, A: 255})
	border.CornerRadius = 5

	contentContainer := container.NewPadded(richText)

	// Add padding and styling
	return container.NewStack(
		border,
		contentContainer,
	)
}

// ParseMarkdownToText parses markdown and returns plain text (for fallback)
// This is useful when you need to extract text content without rendering
func ParseMarkdownToText(markdown string) string {
	// RichText internally parses markdown, but if you need plain text:
	// You could use a markdown parser library, but for now we'll return as-is
	// since RichTextFromMarkdown handles the parsing
	return markdown
}
