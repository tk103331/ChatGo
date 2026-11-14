package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

// ImportCandidate is one MCP server or provider a plugin importer found in
// another agent's configuration.
type ImportCandidate struct {
	// From is the dialog group, "plugin: <plugin> · <importer>".
	From string
	// Kind is mcp, providers or skills.
	Kind string
	Name string
	// Path is the skill directory for kind skills.
	Path string

	// MCP fields.
	Type    string
	Command string
	Args    []string
	Env     map[string]string
	URL     string
	Headers map[string]string

	// Provider fields.
	BaseURL string
	APIKey  string
	Models  []string
}

// DiscoverImports scans the enabled plugins' declarative importers for one
// kind (mcp or providers).
func (m *Manager) DiscoverImports(workdir, kind string) []ImportCandidate {
	m.mu.RLock()
	insts := m.loadedInstancesLocked()
	m.mu.RUnlock()
	var out []ImportCandidate
	seen := map[string]bool{}
	for _, inst := range insts {
		for _, spec := range inst.Manifest.Importers {
			if !importerHasKind(spec, kind) {
				continue
			}
			from := "plugin: " + inst.Manifest.ID + " · " + firstNonEmpty(spec.Label, spec.ID)
			for _, cand := range inst.readImporter(spec, workdir, kind) {
				key := strings.ToLower(cand.Name)
				if cand.Name == "" || seen[key] {
					continue
				}
				seen[key] = true
				cand.From = from
				cand.Kind = kind
				out = append(out, cand)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func importerHasKind(spec ImporterSpec, kind string) bool {
	if len(spec.Kinds) == 0 {
		return true
	}
	for _, k := range spec.Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// readImporter parses every file the importer declares and maps its entries.
func (in *InstanceState) readImporter(spec ImporterSpec, workdir, kind string) []ImportCandidate {
	var out []ImportCandidate
	for _, f := range spec.Files {
		if f.OS != "" && f.OS != "any" && f.OS != runtime.GOOS {
			continue
		}
		path, err := importerPath(f.Path, workdir)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil || len(data) > 2<<20 {
			continue
		}
		root, err := parseImportFile(data, spec.Format)
		if err != nil {
			continue
		}
		out = append(out, mapImporterEntries(root, spec.Mapping, kind)...)
	}
	return out
}

// importerPath expands ~ and resolves relative paths, rejecting escapes.
func importerPath(raw, workdir string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("空路径")
	}
	home, _ := os.UserHomeDir()
	if raw == "~" || strings.HasPrefix(raw, "~/") {
		raw = filepath.Join(home, strings.TrimPrefix(raw, "~"))
	} else if !filepath.IsAbs(raw) {
		raw = filepath.Join(workdir, raw)
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", err
	}
	if home != "" && withinPath(abs, home) {
		return abs, nil
	}
	if workdir != "" {
		if wd, err := filepath.Abs(workdir); err == nil && withinPath(abs, wd) {
			return abs, nil
		}
	}
	return "", fmt.Errorf("路径越界: %s", raw)
}

func withinPath(path, root string) bool {
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

// parseImportFile decodes json, yaml or toml into a generic tree.
func parseImportFile(data []byte, format string) (any, error) {
	var out any
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "yaml", "yml":
		return out, yaml.Unmarshal(data, &out)
	case "toml":
		return out, toml.Unmarshal(data, &out)
	default:
		return out, json.Unmarshal(data, &out)
	}
}

// mapImporterEntries turns the parsed tree into candidates for one kind.
func mapImporterEntries(root any, mapping map[string]any, kind string) []ImportCandidate {
	entry, _ := mapping[kind].(map[string]any)
	if entry == nil {
		return nil
	}
	container, ok := resolvePtr(root, stringValue(entry["root"]))
	if !ok {
		return nil
	}
	obj, ok := container.(map[string]any)
	if !ok {
		// A root that resolves to a list of objects is supported too.
		list, listOK := container.([]any)
		if !listOK {
			return nil
		}
		var out []ImportCandidate
		for _, item := range list {
			if m, ok := item.(map[string]any); ok {
				if c, ok := candidateFrom(m, entry, kind, ""); ok {
					out = append(out, c)
				}
			}
		}
		return out
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []ImportCandidate
	for _, key := range keys {
		item, ok := obj[key].(map[string]any)
		if !ok {
			continue
		}
		if c, ok := candidateFrom(item, entry, kind, key); ok {
			out = append(out, c)
		}
	}
	return out
}

func candidateFrom(item map[string]any, entry map[string]any, kind, key string) (ImportCandidate, bool) {
	c := ImportCandidate{}
	namePtr := stringValue(entry["name"])
	if namePtr == "@key" {
		c.Name = key
	} else {
		c.Name = stringAt(item, namePtr)
		if c.Name == "" {
			c.Name = key
		}
	}
	switch kind {
	case "skills":
		if c.Path == "" {
			return c, false
		}
		return c, true
	case "mcp":
		c.Type = firstNonEmpty(stringAt(item, stringValue(entry["type"])), "stdio")
		c.Command = stringAt(item, stringValue(entry["command"]))
		c.Args = stringsAt(item, stringValue(entry["args"]))
		c.Env = mapAt(item, stringValue(entry["env"]))
		c.URL = stringAt(item, stringValue(entry["url"]))
		c.Headers = mapAt(item, stringValue(entry["headers"]))
		if c.Command == "" && c.URL == "" {
			return c, false
		}
	case "providers":
		c.BaseURL = stringAt(item, stringValue(entry["baseURL"]))
		c.APIKey = stringAt(item, stringValue(entry["apiKey"]))
		c.Models = stringsAt(item, stringValue(entry["models"]))
		if c.BaseURL == "" {
			return c, false
		}
	}
	return c, c.Name != ""
}

// resolvePtr walks a JSON-pointer-like path ("/a/b") in a decoded tree.
func resolvePtr(root any, ptr string) (any, bool) {
	ptr = strings.TrimSpace(ptr)
	if ptr == "" || ptr == "/" {
		return root, true
	}
	cur := root
	for _, part := range strings.Split(strings.Trim(ptr, "/"), "/") {
		if part == "" {
			continue
		}
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func stringAt(item map[string]any, ptr string) string {
	v, ok := resolvePtr(item, ptr)
	if !ok {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	}
	return ""
}

func stringsAt(item map[string]any, ptr string) []string {
	v, ok := resolvePtr(item, ptr)
	if !ok {
		return nil
	}
	switch x := v.(type) {
	case []any:
		out := make([]string, 0, len(x))
		for _, s := range x {
			if str, ok := s.(string); ok {
				out = append(out, str)
			}
		}
		return out
	case map[string]any:
		// An object as a model list — OpenCode's providers hold
		// {"model-id": {}} — reads as its keys.
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return keys
	}
	return nil
}

func mapAt(item map[string]any, ptr string) map[string]string {
	v, ok := resolvePtr(item, ptr)
	if !ok {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, x := range m {
		if s, ok := x.(string); ok {
			out[k] = s
		}
	}
	return out
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// DiscoverSkillImports scans the enabled plugins' skill importers.
func (m *Manager) DiscoverSkillImports(workdir string) []ImportCandidate {
	m.mu.RLock()
	insts := m.loadedInstancesLocked()
	m.mu.RUnlock()
	var out []ImportCandidate
	seen := map[string]bool{}
	for _, inst := range insts {
		for _, spec := range inst.Manifest.Importers {
			if !importerHasKind(spec, "skills") || len(spec.Dirs) == 0 {
				continue
			}
			from := "plugin: " + inst.Manifest.ID + " · " + firstNonEmpty(spec.Label, spec.ID)
			for _, dir := range spec.Dirs {
				root, err := importerPath(dir, workdir)
				if err != nil {
					continue
				}
				entries, err := os.ReadDir(root)
				if err != nil {
					continue
				}
				for _, e := range entries {
					if !e.IsDir() {
						continue
					}
					skillDir := filepath.Join(root, e.Name())
					data, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
					if err != nil {
						continue
					}
					name, _, _ := parseSkillFile(e.Name(), data)
					key := strings.ToLower(name)
					if name == "" || seen[key] {
						continue
					}
					seen[key] = true
					out = append(out, ImportCandidate{
						From: from, Kind: "skills", Name: name, Path: skillDir,
					})
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}
