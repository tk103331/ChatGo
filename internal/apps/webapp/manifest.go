// Package webapp is the generic WebApp framework: each web app is a folder
// with a manifest, an SPA rendered in a WebView, and optional resources
// (tools, WebMCP endpoints and Skills) exposed to the Agent.
package webapp

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Manifest describes one web app.
type Manifest struct {
	ID          string      `yaml:"id" json:"id"`
	Name        string      `yaml:"name" json:"name"`
	Description string      `yaml:"description" json:"description"`
	Icon        string      `yaml:"icon" json:"icon"`
	Entry       string      `yaml:"entry" json:"entry"`
	System      string      `yaml:"system" json:"system"`
	Tools       []ToolSpec  `yaml:"tools" json:"tools"`
	WebMCP      []WebMCP    `yaml:"webmcp" json:"webmcp"`
	Skills      []SkillSpec `yaml:"skills" json:"skills"`
}

// ToolSpec is one capability the app exposes to the Agent. The app handles
// the call in JavaScript through window.chatgo.handle(name, args).
type ToolSpec struct {
	Name        string      `yaml:"name" json:"name"`
	Description string      `yaml:"description" json:"description"`
	Params      []ParamSpec `yaml:"params" json:"params"`
}

// ParamSpec is one tool parameter.
type ParamSpec struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Required    bool   `yaml:"required" json:"required"`
}

// WebMCP is an MCP endpoint the app serves. A relative url is resolved
// against the app's own protocol, so "/mcp" becomes webapp-<id>://localhost/mcp.
type WebMCP struct {
	Name string `yaml:"name" json:"name"`
	URL  string `yaml:"url" json:"url"`
	// Transport is "streamable_http" (default) or "sse".
	Transport string `yaml:"transport" json:"transport"`
}

// SkillSpec points at a SKILL.md shipped with the app.
type SkillSpec struct {
	Name string `yaml:"name" json:"name"`
	Path string `yaml:"path" json:"path"`
}

// App is a discovered web app and its folder.
type App struct {
	Manifest
	Dir      string
	embedded fs.FS
}

var validID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// Roots are the folders searched for web apps, first match wins: the
// project's own folders, then the user's, then the bundled examples.
func Roots() []string {
	var out []string
	if wd, err := os.Getwd(); err == nil {
		out = append(out, filepath.Join(wd, ".chatgo", "webapps"), filepath.Join(wd, "webapps"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(home, ".chatgo", "webapps"))
	}
	if exe, err := os.Executable(); err == nil {
		out = append(out, filepath.Join(filepath.Dir(exe), "webapps"))
	}
	return out
}

// Discover reads every web app under the roots, deduplicating by id.
func Discover() []App {
	seen := map[string]bool{}
	var out []App
	for _, root := range Roots() {
		for _, dir := range listDirs(root) {
			a, err := Load(dir)
			if err != nil || seen[a.ID] {
				continue
			}
			seen[a.ID] = true
			out = append(out, a)
		}
	}
	for _, a := range bundledApps() {
		if seen[a.ID] {
			continue
		}
		seen[a.ID] = true
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Find returns the web app with the given id, discovered from all roots.
func Find(id string) (App, bool) {
	for _, a := range Discover() {
		if a.ID == id {
			return a, true
		}
	}
	return App{}, false
}

// Load reads one web app folder, applying defaults and validating it.
func Load(dir string) (App, error) {
	var m Manifest
	if data, err := os.ReadFile(filepath.Join(dir, "webapp.yaml")); err == nil {
		if err := yaml.Unmarshal(data, &m); err != nil {
			return App{}, err
		}
	} else if data, err := os.ReadFile(filepath.Join(dir, "webapp.json")); err == nil {
		if err := decodeJSON(data, &m); err != nil {
			return App{}, err
		}
	} else {
		return App{}, fmt.Errorf("缺少 webapp.yaml 或 webapp.json")
	}
	a := App{Manifest: m, Dir: dir}
	if a.ID == "" {
		a.ID = filepath.Base(dir)
	}
	if !validID.MatchString(a.ID) {
		return App{}, fmt.Errorf("id 只能包含小写字母、数字和连字符: %q", a.ID)
	}
	if a.Name == "" {
		a.Name = a.ID
	}
	if a.Entry == "" {
		a.Entry = "index.html"
	}
	if _, err := os.Stat(filepath.Join(dir, a.Entry)); err != nil {
		return App{}, fmt.Errorf("入口文件不存在: %s", a.Entry)
	}
	if a.Icon == "" {
		a.Icon = "globe"
	}
	return a, nil
}

// SystemPrompt builds the Agent prompt from the manifest and its resources.
func (a App) SystemPrompt() string {
	if strings.TrimSpace(a.System) != "" {
		return a.System
	}
	var b strings.Builder
	fmt.Fprintf(&b, "你是 %s 应用助手。应用界面在 WebView 中运行，你可以调用它导出的工具完成用户的任务。\n", a.Name)
	if a.Description != "" {
		b.WriteString(a.Description + "\n")
	}
	if len(a.Tools) > 0 {
		b.WriteString("\n可用工具：\n")
		for _, t := range a.Tools {
			fmt.Fprintf(&b, "- %s：%s\n", t.Name, t.Description)
		}
	}
	if len(a.Skills) > 0 {
		b.WriteString("\n应用内置技能：\n")
		for _, s := range a.Skills {
			fmt.Fprintf(&b, "- %s（%s）\n", s.Name, s.Path)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func listDirs(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, filepath.Join(root, e.Name()))
		}
	}
	return out
}
