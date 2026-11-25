package a2ui

// Catalog names a set of components and functions an agent and renderer
// agreed on. It is metadata only: each renderer decides how to draw the
// names it knows.
type Catalog struct {
	ID          string
	Title       string
	Description string
	Components  []string
	Functions   []string
	// Native reports whether this renderer package draws the catalog itself.
	// A false value means a surface using it must go to a web renderer.
	Native bool
}

// Has reports whether the catalog lists a component.
func (c Catalog) Has(component string) bool {
	for _, name := range c.Components {
		if name == component {
			return true
		}
	}
	return false
}

// BasicCatalog lists the A2UI 1.0 reference catalog, with the components the
// native renderer supports marked by Native.
func BasicCatalog() Catalog {
	return Catalog{
		ID:    BasicCatalogID,
		Title: "A2UI Basic Catalog",
		Components: []string{
			"Text", "Image", "Icon", "Video", "AudioPlayer",
			"Row", "Column", "List", "Card", "Tabs", "Modal", "Divider",
			"Button", "TextField", "CheckBox", "ChoicePicker", "Slider", "DateTimeInput",
		},
		Functions: []string{
			"formatString", "formatNumber", "formatCurrency", "formatDate",
			"pluralize", "required", "regex", "length", "numeric", "email",
			"openUrl", "and", "or", "not",
		},
		Native: true,
	}
}

// NativeCatalog is a smaller catalog a native renderer can promise to draw.
// Components outside it are shown as placeholders.
func NativeCatalog() Catalog {
	return Catalog{
		ID:    BasicCatalogID,
		Title: "ChatGo Native Catalog",
		Components: []string{
			"Text", "Title", "Caption", "Label", "Image", "Icon",
			"Row", "Column", "Box", "List", "Card", "Section", "Divider",
			"Button", "TextField", "TextArea", "CheckBox", "Switch", "Slider",
			"Select", "ChoicePicker", "RadioGroup", "SegmentedControl",
		},
		Functions: []string{"required", "length", "numeric", "email", "regex", "and", "or", "not"},
		Native:    true,
	}
}

// SupportsCatalog reports whether id is this catalog's ID.
func (c Catalog) SupportsCatalog(id string) bool {
	return id == "" || id == c.ID
}
