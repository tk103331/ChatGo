package ui

import (
	"fmt"
	"strings"

	"chatgo/internal/agentimport"
	"chatgo/internal/config"
	mcore "chatgo/internal/ui/uikit"
)

// startImportScan reads the other agents' configuration off the UI thread,
// since it walks directories and parses several files.
func (cw *ChatWindow) startImportScan() {
	kind := cw.imp.kind
	workdir := cw.fileRoot()
	cw.imp.scanning = true
	cw.imp.scanned = false
	cw.imp.groups = nil
	go func() {
		groups := scanImportable(kind, workdir)
		cw.postMain(func() {
			if !cw.imp.open || cw.imp.kind != kind {
				return
			}
			cw.imp.scanning = false
			cw.imp.scanned = true
			cw.imp.groups = groups
			// Everything a scan found is pre-ticked: the user opened the
			// dialog to import, and can clear what they do not want.
			for _, g := range groups {
				for _, it := range g.Items {
					cw.imp.chosen[it.ID] = true
				}
			}
			cw.invalidate()
		})
	}()
}

// scanImportable groups what the other agents offer for a kind.
func scanImportable(kind importKind, workdir string) []importGroup {
	byFrom := map[string]*importGroup{}
	order := []string{}
	add := func(from string, it importCandidate) {
		g, ok := byFrom[from]
		if !ok {
			g = &importGroup{From: from}
			byFrom[from] = g
			order = append(order, from)
		}
		g.Items = append(g.Items, it)
	}
	switch kind {
	case importSkills:
		for _, st := range agentimport.Available(workdir) {
			for _, f := range st.Skills {
				add(st.Source.Name, importCandidate{
					ID:    st.Source.Name + "|" + f.Name,
					Title: f.Name,
					Desc:  f.Dir,
				})
			}
		}
	case importMCP:
		for _, c := range agentimport.DiscoverMCP(workdir) {
			add(c.From, importCandidate{
				ID:    c.From + "|" + c.Name,
				Title: c.Name,
				Desc:  mcpCandidateDesc(c),
			})
		}
	case importProviders:
		for _, p := range agentimport.DiscoverProviders(workdir) {
			add(p.From, importCandidate{
				ID:    p.From + "|" + p.Name,
				Title: p.Name,
				Desc:  providerCandidateDesc(p),
				Tags:  modelTags(p.Models),
			})
		}
	}
	out := make([]importGroup, 0, len(order))
	for _, from := range order {
		out = append(out, *byFrom[from])
	}
	return out
}

// mcpCandidateDesc summarizes an MCP server for the list.
func mcpCandidateDesc(c agentimport.MCPCandidate) string {
	target := strings.TrimSpace(c.Command + " " + strings.Join(c.Args, " "))
	if target == "" {
		target = c.URL
	}
	if target == "" {
		return c.Type
	}
	return c.Type + " · " + target
}

// providerCandidateDesc summarizes a provider for the list.
func providerCandidateDesc(p agentimport.ProviderCandidate) string {
	parts := []string{p.BaseURL}
	if p.APIKey != "" {
		parts = append(parts, "含 API Key")
	}
	return strings.Join(parts, " · ")
}

const (
	// maxModelTags bounds the chips shown for a provider, so a list of dozens
	// of models does not take over the dialog.
	maxModelTags = 8
	// maxImportedModels bounds the models an import writes into the
	// configuration; the rest can be added by hand.
	maxImportedModels = 20
)

// importedModels is the slice of models an import actually writes.
func importedModels(models []string) []string {
	if len(models) > maxImportedModels {
		return models[:maxImportedModels]
	}
	return models
}

// modelTags renders the models an import would write as chips, trailing with
// how many more the provider has. The count reflects what is imported, not
// what the source offers, so the chips never promise more than they deliver.
func modelTags(models []string) []string {
	kept := importedModels(models)
	if len(kept) == 0 {
		return nil
	}
	tags := make([]string, 0, maxModelTags+1)
	if len(kept) <= maxModelTags {
		tags = append(tags, kept...)
	} else {
		tags = append(tags, kept[:maxModelTags]...)
		tags = append(tags, fmt.Sprintf("+%d", len(kept)-maxModelTags))
	}
	if len(models) > len(kept) {
		tags = append(tags, fmt.Sprintf("仅导入前 %d 个", len(kept)))
	}
	return tags
}

// runImport applies the ticked entries.
func (cw *ChatWindow) runImport() {
	switch cw.imp.kind {
	case importSkills:
		cw.runSkillImport()
	case importMCP:
		cw.runMCPImport()
	case importProviders:
		cw.runProviderImport()
	}
}

// runSkillImport copies the chosen skills into ~/.chatgo/skills.
func (cw *ChatWindow) runSkillImport() {
	workdir := cw.fileRoot()
	overwrite := cw.imp.overwrite
	wanted := cw.chosenByGroup()
	cw.imp.busy = true
	go func() {
		var total agentimport.ImportResult
		for _, st := range agentimport.Available(workdir) {
			names := wanted[st.Source.Name]
			if len(names) == 0 {
				continue
			}
			res, err := agentimport.Import(st.Source, names, overwrite)
			if err != nil {
				total.Errors = append(total.Errors, err.Error())
				continue
			}
			total.Imported = append(total.Imported, res.Imported...)
			total.Skipped = append(total.Skipped, res.Skipped...)
			total.Failed = append(total.Failed, res.Failed...)
			total.Errors = append(total.Errors, res.Errors...)
		}
		cw.postMain(func() {
			cw.imp.busy = false
			cw.imp.status = total.Summary()
			if len(total.Imported) > 0 {
				cw.loadSkills()
				cw.toast("已导入 "+fmt.Sprint(len(total.Imported))+" 个 Skill", mcore.SeveritySuccess)
			}
			cw.invalidate()
		})
	}()
}

// runMCPImport adds the chosen MCP servers to the configuration, renaming an
// entry whose name is taken so an existing server is never overwritten.
func (cw *ChatWindow) runMCPImport() {
	wanted := cw.chosenIDs()
	added := 0
	for _, c := range agentimport.DiscoverMCP(cw.fileRoot()) {
		if !wanted[c.From+"|"+c.Name] {
			continue
		}
		srv := config.MCPServer{
			Name:    cw.uniqueMCPName(c.Name),
			Type:    config.MCPServerType(c.Type),
			Enabled: false,
			Command: c.Command,
			Args:    c.Args,
			Env:     c.Env,
			URL:     c.URL,
			Headers: c.Headers,
		}
		cw.cfg.MCPServers = append(cw.cfg.MCPServers, srv)
		added++
	}
	if added == 0 {
		cw.imp.status = "没有新增服务"
		return
	}
	if err := config.SaveConfig(cw.cfg); err != nil {
		cw.imp.status = "保存失败：" + err.Error()
		return
	}
	cw.imp.open = false
	cw.toast(fmt.Sprintf("已导入 %d 个 MCP 服务，请检查后启用", added), mcore.SeveritySuccess)
}

// runProviderImport adds the chosen providers as custom providers.
func (cw *ChatWindow) runProviderImport() {
	wanted := cw.chosenIDs()
	added := 0
	for _, p := range agentimport.DiscoverProviders(cw.fileRoot()) {
		if !wanted[p.From+"|"+p.Name] {
			continue
		}
		models := importedModels(p.Models)
		model := ""
		if len(models) > 0 {
			model = models[0]
		}
		cw.cfg.Providers = append(cw.cfg.Providers, config.Provider{
			Name:    cw.uniqueProviderName(p.Name),
			Type:    config.ProviderTypeCustom,
			APIType: config.ProviderTypeOpenAI,
			APIKey:  p.APIKey,
			BaseURL: p.BaseURL,
			Model:   model,
			Models:  models,
			Enabled: false,
		})
		added++
	}
	if added == 0 {
		cw.imp.status = "没有新增模型"
		return
	}
	if err := config.SaveConfig(cw.cfg); err != nil {
		cw.imp.status = "保存失败：" + err.Error()
		return
	}
	cw.imp.open = false
	cw.toast(fmt.Sprintf("已导入 %d 个模型，请检查后启用", added), mcore.SeveritySuccess)
}

// chosenIDs is the set of ticked entry IDs.
func (cw *ChatWindow) chosenIDs() map[string]bool {
	out := map[string]bool{}
	for id, on := range cw.imp.chosen {
		if on {
			out[id] = true
		}
	}
	return out
}

// chosenByGroup maps a source name to the entry titles ticked under it.
func (cw *ChatWindow) chosenByGroup() map[string][]string {
	out := map[string][]string{}
	for _, g := range cw.imp.groups {
		for _, it := range g.Items {
			if cw.imp.chosen[it.ID] {
				out[g.From] = append(out[g.From], it.Title)
			}
		}
	}
	return out
}

// uniqueMCPName returns name, suffixed when a server already uses it.
func (cw *ChatWindow) uniqueMCPName(name string) string {
	taken := func(n string) bool {
		for _, s := range cw.cfg.MCPServers {
			if strings.EqualFold(s.Name, n) {
				return true
			}
		}
		return false
	}
	return uniqueName(name, taken)
}

// uniqueProviderName returns name, suffixed when a provider already uses it.
func (cw *ChatWindow) uniqueProviderName(name string) string {
	taken := func(n string) bool {
		for _, p := range cw.cfg.Providers {
			if strings.EqualFold(p.Name, n) {
				return true
			}
		}
		return false
	}
	return uniqueName(name, taken)
}

// uniqueName appends a counter until taken reports the name is free.
func uniqueName(name string, taken func(string) bool) string {
	if !taken(name) {
		return name
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", name, i)
		if !taken(candidate) {
			return candidate
		}
	}
}
