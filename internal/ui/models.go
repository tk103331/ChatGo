package ui

import (
	"strings"

	"chatgo/internal/ui/common"
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

// allModelRefs lists every configured provider model.
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
	return out
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
		if !common.ContainsStr(out, ref) {
			out = append(out, ref)
		}
	}
	return out
}
