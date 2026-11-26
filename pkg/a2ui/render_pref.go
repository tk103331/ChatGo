package a2ui

// RenderExtension is the host extension under createSurface.metadata.extensions.
// It records which renderer a surface should use, so replay and remote agents
// make the same choice.
const RenderExtension = "chatgo/render"

// Render modes.
const (
	RenderAuto   = "auto"
	RenderNative = "native"
	RenderWeb    = "web"
)

// RenderPreference is the renderer choice a surface carries.
type RenderPreference struct {
	Mode   string `json:"mode,omitempty"`
	Bundle string `json:"bundle,omitempty"`
}

// RenderPreferenceOf reads the host extension from surface metadata.
func RenderPreferenceOf(meta map[string]any) RenderPreference {
	if meta == nil {
		return RenderPreference{}
	}
	ext, _ := meta["extensions"].(map[string]any)
	raw, _ := ext[RenderExtension].(map[string]any)
	pref := RenderPreference{}
	if mode, ok := raw["mode"].(string); ok {
		pref.Mode = mode
	}
	if bundle, ok := raw["bundle"].(string); ok {
		pref.Bundle = bundle
	}
	return pref
}

// WithRenderPreference writes the host extension into metadata, preserving
// whatever the agent put there.
func WithRenderPreference(meta map[string]any, pref RenderPreference) map[string]any {
	if meta == nil {
		meta = map[string]any{}
	}
	ext, _ := meta["extensions"].(map[string]any)
	if ext == nil {
		ext = map[string]any{}
		meta["extensions"] = ext
	}
	entry, _ := ext[RenderExtension].(map[string]any)
	if entry == nil {
		entry = map[string]any{}
		ext[RenderExtension] = entry
	}
	if pref.Mode != "" {
		entry["mode"] = pref.Mode
	}
	if pref.Bundle != "" {
		entry["bundle"] = pref.Bundle
	}
	return meta
}
