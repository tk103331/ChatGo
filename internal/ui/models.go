package ui

import (
	"fmt"
	"strings"

	"chatgo/internal/config"
)

// modelRef identifies a model: "provider::model", or "::model" for a custom
// model name run through the active provider.
func modelRef(provider, model string) string { return provider + "::" + model }

// splitModelRef splits a model reference into its provider and model.
func splitModelRef(ref string) (provider, model string) {
	if i := strings.Index(ref, "::"); i >= 0 {
		return ref[:i], ref[i+2:]
	}
	return "", ref
}

// modelLabel renders a reference for the model selector.
func modelLabel(ref string) string {
	provider, model := splitModelRef(ref)
	switch {
	case provider == "":
		return model
	case model == "":
		return provider
	default:
		return provider + " · " + model
	}
}

// allModelRefs lists every configured provider model, then every enabled
// provider group. A group is one entry with no model of its own: picking a
// member's model would pin the request to it and bypass the group's routing
// strategy, so the model is left to the strategy at call time.
func (cw *ChatWindow) allModelRefs() []string {
	var out []string
	for _, p := range cw.cfg.Providers {
		if !p.Enabled || p.Name == "" {
			continue
		}
		for _, m := range p.ModelsList() {
			out = append(out, modelRef(p.Name, m))
		}
	}
	for _, g := range cw.cfg.EnabledGroups() {
		out = append(out, groupRef(g.Name))
	}
	return out
}

// groupRef is the model reference of a whole provider group.
func groupRef(name string) string { return name + "::" }

// refLabel renders a reference for display, naming a group's routing strategy
// so a group entry is told apart from a provider of the same name.
func (cw *ChatWindow) refLabel(ref string) string {
	if name, model := splitModelRef(ref); model == "" && name != "" {
		if g, ok := cw.cfg.FindProviderGroup(name); ok {
			return name + " · " + config.GetStrategyDisplayName(g.EffectiveStrategy())
		}
	}
	return modelLabel(ref)
}

// isGroupRef reports whether a reference names a group rather than one model.
func (cw *ChatWindow) isGroupRef(ref string) bool {
	name, model := splitModelRef(ref)
	if name == "" || model != "" {
		return false
	}
	_, ok := cw.cfg.FindProviderGroup(name)
	return ok
}

// agentModelRefs is the models the active agent may switch between: the built-in
// Chat uses every configured model, a custom agent its whitelist.
func (cw *ChatWindow) agentModelRefs() []string {
	all := cw.allModelRefs()
	a, ok := cw.currentAgentDef()
	if !ok || len(a.Models) == 0 {
		return all
	}
	return append([]string(nil), a.Models...)
}

// currentModelRef is the reference of the active provider and model.
func (cw *ChatWindow) currentModelRef() string { return modelRef(cw.provider, cw.model) }

// setModelRef applies a reference, keeping the current provider for a custom
// model name.
func (cw *ChatWindow) setModelRef(ref string) {
	provider, model := splitModelRef(ref)
	if provider != "" {
		cw.provider = provider
	}
	cw.model = model
}

// modelOptions is every configured model plus the agent's own custom entries.
func (cw *ChatWindow) modelOptions(custom []string) []string {
	out := cw.allModelRefs()
	for _, ref := range custom {
		if !containsStr(out, ref) {
			out = append(out, ref)
		}
	}
	return out
}

// defaultTarget returns the provider (or group) and model a new conversation
// starts with: the active selection, falling back to the first enabled one.
func (cw *ChatWindow) defaultTarget() (name, model string) {
	name, model = cw.provider, cw.model
	if name == "" {
		if p, ok := cw.firstEnabled(); ok {
			name = p.Name
		} else if g, ok := cw.firstGroup(); ok {
			name = g.Name
		}
	}
	if model == "" {
		model = cw.firstModelOf(name)
	}
	return name, model
}

// firstModelOf returns the first model of the named provider. A group has no
// model of its own, so it reports none: recording a member's model would make
// a replayed turn run on that member instead of the group.
func (cw *ChatWindow) firstModelOf(name string) string {
	if _, ok := cw.cfg.FindProviderGroup(name); ok {
		return ""
	}
	if p, ok := cw.findProvider(name); ok {
		if ms := p.ModelsList(); len(ms) > 0 {
			return ms[0]
		}
	}
	return ""
}

// refDetail describes a model reference under its title: a group lists the
// members it routes over, a provider model names its provider.
func (cw *ChatWindow) refDetail(ref string) (desc string, tags []string) {
	name, model := splitModelRef(ref)
	if model == "" && name != "" {
		if g, ok := cw.cfg.FindProviderGroup(name); ok {
			members := make([]string, 0, len(g.Members))
			for _, m := range g.Members {
				if len(m.Models) > 0 {
					members = append(members, m.Provider)
				}
			}
			return fmt.Sprintf("模型组 · %d 个成员", len(members)), members
		}
	}
	if name == "" {
		return "自定义模型名", nil
	}
	return name, nil
}
