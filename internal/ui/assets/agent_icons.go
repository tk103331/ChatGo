package assets

import (
	"embed"
	"io/fs"
	"strings"
	"sync"

	mgo "github.com/egoist/mygo/ui"
)

//go:embed agent_*.svg
var agentIconFS embed.FS

var (
	agentIconOnce sync.Once
	agentIcons    map[string]*mgo.SVG
)

// ACPLogoSVG returns the brand SVG for a known ACP agent by name, or nil.
func ACPLogoSVG(name string) *mgo.SVG {
	agentIconOnce.Do(func() {
		agentIcons = map[string]*mgo.SVG{}
		entries, _ := fs.Glob(agentIconFS, "agent_*.svg")
		for _, e := range entries {
			key := strings.TrimSuffix(strings.TrimPrefix(e, "agent_"), ".svg")
			data, err := agentIconFS.ReadFile(e)
			if err != nil {
				continue
			}
			if svg, err := mgo.ParseSVG(data); err == nil {
				agentIcons[key] = svg
			}
		}
	})
	return agentIcons[ACPIconKey(name)]
}

// ACPTiledIcons are icons designed as app tiles with their own background and
// colors; they render with mgo.Image to keep the official look. All other
// icons are single-color silhouettes drawn in the theme text color.
var ACPTiledIcons = map[string]bool{
	"kiro": true, "factory": true, "openhands": true, "qoder": true, "pi": true, "omp": true,
}

// ACPIconKey maps an agent name to its icon file key.
func ACPIconKey(name string) string {
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
	case strings.Contains(low, "deepseek"):
		return "deepseek"
	case strings.Contains(low, "piagent") || strings.Contains(low, "pi-acp"):
		return "pi"
	case strings.Contains(low, "ohmypi") || strings.Contains(low, "oh-my-pi") || low == "omp":
		return "omp"
	}
	return ""
}

//go:embed claude.png
var claudeLogoPNG []byte

//go:embed opencode.png
var opencodeLogoPNG []byte

var (
	logoOnce   sync.Once
	logoClaude *mgo.Bitmap
	logoOpen   *mgo.Bitmap
)

// ACPLogo returns the official bitmap logo of a known ACP agent by name.
func ACPLogo(name string) *mgo.Bitmap {
	logoOnce.Do(func() {
		logoClaude, _ = mgo.DecodeBitmap(claudeLogoPNG)
		logoOpen, _ = mgo.DecodeBitmap(opencodeLogoPNG)
	})
	low := strings.ToLower(name)
	switch {
	case strings.Contains(low, "claude"):
		return logoClaude
	case strings.Contains(low, "opencode"):
		return logoOpen
	}
	return nil
}
