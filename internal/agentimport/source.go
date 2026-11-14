// Package agentimport copies skills into ~/.chatgo/skills. Where the skills
// come from is the importers' business — the built-in agent-importers plugin
// and any plugin-declared importer do the finding — this package owns the
// copying and the directories ChatGo itself keeps.
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
