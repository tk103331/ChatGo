package webapp

import (
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Skill is one skill body shipped with a web app.
type Skill struct {
	Name        string
	Description string
	Body        string
}

// Skills reads the SKILL.md files the manifest declares.
func (cl *Client) Skills() []Skill {
	out := make([]Skill, 0, len(cl.app.Skills))
	for _, s := range cl.app.Skills {
		rel := s.Path
		if rel == "" {
			rel = s.Name
		}
		data, err := cl.app.readFile(filepath.ToSlash(filepath.Join(rel, "SKILL.md")))
		if err != nil {
			continue
		}
		sk := parseSkill(data)
		if sk.Name == "" {
			sk.Name = s.Name
		}
		out = append(out, sk)
	}
	return out
}

// SkillPrompt renders the app's skills for the system prompt.
func (cl *Client) SkillPrompt() string {
	var parts []string
	for _, s := range cl.Skills() {
		if strings.TrimSpace(s.Body) == "" {
			continue
		}
		parts = append(parts, "## Skill: "+s.Name+"\n\n"+s.Body)
	}
	return strings.Join(parts, "\n\n")
}

// parseSkill splits YAML front matter from the body.
func parseSkill(data []byte) Skill {
	text := string(data)
	var sk Skill
	if strings.HasPrefix(text, "---\n") {
		rest := text[len("---\n"):]
		if i := strings.Index(rest, "\n---"); i >= 0 {
			var fm struct {
				Name        string `yaml:"name"`
				Description string `yaml:"description"`
			}
			if err := yaml.Unmarshal([]byte(rest[:i]), &fm); err == nil {
				sk.Name, sk.Description = fm.Name, fm.Description
			}
			body := rest[i+len("\n---"):]
			sk.Body = strings.TrimLeft(body, "\n")
			return sk
		}
	}
	sk.Body = text
	return sk
}
