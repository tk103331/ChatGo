package ui

import (
	"embed"
	"io/fs"
	"strings"
	"sync"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
)

//go:embed assets/agent_*.svg
var agentIconFS embed.FS

var (
	agentIconOnce sync.Once
	agentIcons    map[string]*mgo.SVG
)

// acpLogoSVG returns the brand SVG for a known ACP agent by name, or nil.
func acpLogoSVG(name string) *mgo.SVG {
	agentIconOnce.Do(func() {
		agentIcons = map[string]*mgo.SVG{}
		entries, _ := fs.Glob(agentIconFS, "assets/agent_*.svg")
		for _, e := range entries {
			key := strings.TrimSuffix(strings.TrimPrefix(e, "assets/agent_"), ".svg")
			data, err := agentIconFS.ReadFile(e)
			if err != nil {
				continue
			}
			if svg, err := mgo.ParseSVG(data); err == nil {
				agentIcons[key] = svg
			}
		}
	})
	return agentIcons[acpIconKey(name)]
}

// acpTiledIcons are icons designed as app tiles with their own background and
// colors; they render with mgo.Image to keep the official look. All other
// icons are single-color silhouettes drawn in the theme text color.
var acpTiledIcons = map[string]bool{
	"kiro": true, "factory": true, "openhands": true, "qoder": true,
}

// acpIcon renders the agent's brand icon (SVG first, then PNG bitmap) at the
// given size. Returns true if an icon was drawn.
func acpIcon(c *mgo.Context, name string, w, h float32) bool {
	if svg := acpLogoSVG(name); svg != nil {
		if acpTiledIcons[acpIconKey(name)] {
			mgo.Image(c, svg).Size(w, h).Shrink(0)
		} else {
			mgo.Icon(c, svg).Size(w, h).Shrink(0).TextColor(mcore.Tokens(c).Text)
		}
		return true
	}
	if bmp := acpLogo(name); bmp != nil {
		mgo.Image(c, bmp).Size(w, h).Shrink(0)
		return true
	}
	return false
}

// acpIconKey maps an agent name to its icon file key.
func acpIconKey(name string) string {
	low := strings.ToLower(name)
	switch {
	case strings.Contains(low, "claude"):
		return "claude"
	case strings.Contains(low, "opencode"):
		return ""
	case strings.Contains(low, "codex") || strings.Contains(low, "openai"):
		return "openai"
	case strings.Contains(low, "cursor"):
		return "cursor"
	case strings.Contains(low, "gemini"):
		return "gemini"
	case strings.Contains(low, "kiro"):
		return "kiro"
	case strings.Contains(low, "copilot"):
		return "copilot"
	case strings.Contains(low, "goose"):
		return "goose"
	case strings.Contains(low, "qwen"):
		return "qwen"
	case strings.Contains(low, "kimi"):
		return "kimi"
	case strings.Contains(low, "cline"):
		return "cline"
	case strings.Contains(low, "docker") || strings.Contains(low, "cagent"):
		return "docker"
	case strings.Contains(low, "factory") || strings.Contains(low, "droid"):
		return "factory"
	case strings.Contains(low, "openhands"):
		return "openhands"
	case strings.Contains(low, "augment"):
		return "augment"
	case strings.Contains(low, "qoder"):
		return "qoder"
	}
	return ""
}
