// Package agentimport locates skill directories and imports skills, MCP
// servers and model providers from other coding agents' configuration.
package agentimport

import (
	"os"
	"path/filepath"
)

// HomeDir returns ~/.chatgo/skills, the directory ChatGo owns and imports
// into. It is the primary source: a skill here shadows one of the same name
// found anywhere else.
func HomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".chatgo", "skills"), nil
}

// SearchDirs lists the directories scanned for skills, most important first:
// ~/.chatgo/skills, then the shared ~/.agents/skills.
func SearchDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, ".chatgo", "skills"),
		filepath.Join(home, ".agents", "skills"),
	}
}

// Source is one agent whose skills can be imported.
type Source struct {
	// Name is shown in the import dialog.
	Name string
	// Dirs are the places this agent keeps skills; all of them are scanned.
	Dirs []string
}

// Sources lists the agents ChatGo can import skills from. Both the user-level
// and the project-level directory of each agent is scanned, because teams
// commonly check skills into the repository.
func Sources(workdir string) []Source {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	join := func(base string, rels ...string) []string {
		out := make([]string, 0, len(rels))
		for _, rel := range rels {
			out = append(out, filepath.Join(base, filepath.FromSlash(rel)))
		}
		return out
	}
	sources := []Source{
		{Name: "Claude Code", Dirs: join(home, ".claude/skills")},
		{Name: "OpenCode", Dirs: join(home, ".opencode/skill", ".opencode/skills")},
		{Name: "Codex", Dirs: join(home, ".codex/skills", ".codex/skill")},
		{Name: "Agents (共享)", Dirs: join(home, ".agents/skills")},
	}
	if workdir != "" {
		sources = append(sources,
			Source{Name: "当前项目 · Claude Code", Dirs: join(workdir, ".claude/skills")},
			Source{Name: "当前项目 · OpenCode", Dirs: join(workdir, ".opencode/skill", ".opencode/skills")},
			Source{Name: "当前项目 · Agents", Dirs: join(workdir, ".agents/skills")},
		)
	}
	return sources
}

// Available returns the sources that actually hold at least one skill, with
// how many each offers, so the import dialog lists only usable entries.
func Available(workdir string) []SourceStatus {
	var out []SourceStatus
	for _, s := range Sources(workdir) {
		names := Discover(s)
		if len(names) == 0 {
			continue
		}
		out = append(out, SourceStatus{Source: s, Skills: names})
	}
	return out
}

// SourceStatus is a source with the skills found in it.
type SourceStatus struct {
	Source Source
	Skills []Found
}

// Found is one importable skill directory.
type Found struct {
	// Name is the skill's directory name, which is also its identity.
	Name string
	// Dir is the directory holding SKILL.md.
	Dir string
}

// Discover lists the skills a source offers, keeping the first of a name when
// an agent has several directories.
func Discover(s Source) []Found {
	seen := map[string]bool{}
	var out []Found
	for _, dir := range s.Dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || seen[e.Name()] {
				continue
			}
			sub := filepath.Join(dir, e.Name())
			if _, err := os.Stat(filepath.Join(sub, "SKILL.md")); err != nil {
				continue
			}
			seen[e.Name()] = true
			out = append(out, Found{Name: e.Name(), Dir: sub})
		}
	}
	return out
}
