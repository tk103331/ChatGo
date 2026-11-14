package ui

// The "从插件添加" model-provider dialog: the suggestions come from the
// plugin manager's catalog (ProviderCatalog), so the shipped presets are
// built-in plugins a user can disable, and a third-party plugin can offer
// its own vendors the same way.

import (
	"strings"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/config"
	"chatgo/internal/plugin"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// addProviderFromPreset appends the preset as a new provider and opens its
// form, so the next thing to do is paste the key. The provider starts
// disabled — the switch flips once the key is in.
func (cw *ChatWindow) addProviderFromPreset(e plugin.ProviderPresetEntry) {
	name := e.Name
	// A taken name gets a numeric suffix rather than a clash, the way a
	// file copy does.
	for n := 2; cw.providerNameTaken(name); n++ {
		name = e.Name + " " + itoa(n)
	}
	typ := e.Type
	if typ == "" {
		typ = config.ProviderTypeCustom
	}
	p := config.Provider{
		Name: name, Type: typ, BaseURL: e.BaseURL,
		Enabled: false,
	}
	if typ == config.ProviderTypeCustom {
		p.APIType = e.APIType
		if p.APIType == "" {
			p.APIType = config.ProviderTypeOpenAI
		}
	}
	cw.cfg.Providers = append(cw.cfg.Providers, p)
	_ = config.SaveConfig(cw.cfg)
	cw.loadProviderForm(len(cw.cfg.Providers) - 1)
	cw.presetCatalogOpen = false
	cw.toast("已添加 "+name+"，请填写 API Key 后启用", mcore.SeveritySuccess)
}

// providerNameTaken reports whether a provider of that name exists.
func (cw *ChatWindow) providerNameTaken(name string) bool {
	for _, p := range cw.cfg.Providers {
		if p.Name == name {
			return true
		}
	}
	return false
}

// providerPresetCatalogDialog lists the plugin catalog's provider presets,
// grouped by the plugin that offers them. One click appends the provider
// preconfigured with its address; a disabled plugin's group is absent
// because the manager only reports what is enabled.
func (cw *ChatWindow) providerPresetCatalogDialog(c *mgo.Context) {
	if !cw.presetCatalogOpen {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	kit.Dialog(c, &cw.presetCatalogOpen, kit.DialogOptions{
		Title: "从插件添加模型",
		Width: 600,
	}, func() {
		mgo.Column(c).Gap(u * 2).Children(func() {
			in := kit.SearchInput(c, &cw.presetCatalogQuery, kit.SearchInputOptions{Placeholder: "搜索 Provider", Label: "搜索"})
			in.Element.FillWidth()
			q := strings.ToLower(strings.TrimSpace(cw.presetCatalogQuery))
			entries := cw.pluginProviderCatalog()
			groups := plugin.CatalogGroupOrder(entries, func(e plugin.ProviderPresetEntry) string { return e.Group })
			shown := 0
			for _, group := range groups {
				var items []plugin.ProviderPresetEntry
				for _, e := range entries {
					if e.Group != group {
						continue
					}
					if q != "" && !strings.Contains(strings.ToLower(e.Name), q) &&
						!strings.Contains(strings.ToLower(e.Note), q) {
						continue
					}
					items = append(items, e)
				}
				if len(items) == 0 {
					continue
				}
				shown += len(items)
				mgo.Text(c, group).FontSize(caption).FontWeight(600).TextColor(k.TextMuted)
				mgo.Scroll(c).MaxHeight(320).Children(func() {
					mgo.Column(c).Gap(u).Children(func() {
						for _, e := range items {
							e := e
							taken := cw.providerNameTaken(e.Name)
							mgo.Row(c).Key(e.Name).FillWidth().Gap(u*2).AlignItems(mgo.Center).
								Padding(u, u*2).Radius(mtheme.CardRadius).Border(mtheme.BorderWidth, k.Border).
								Children(func() {
									mgo.Column(c).Grow(1).MinWidth(0).Gap(u / 2).Children(func() {
										mgo.Text(c, e.Name).FontWeight(600).SingleLine()
										if e.BaseURL != "" {
											mgo.Text(c, e.BaseURL).Font(mtheme.MonoFont).FontSize(caption).
												TextColor(k.TextMuted).SingleLine().Ellipsis("…").Tooltip(e.BaseURL)
										}
										if e.Note != "" {
											mgo.Text(c, e.Note).FontSize(caption).TextColor(k.TextMuted).MaxLines(1)
										}
									})
									if taken {
										mgo.Text(c, "已添加").FontSize(caption).TextColor(k.Success).Shrink(0)
									} else if kit.Button(c, "添加", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
										cw.addProviderFromPreset(e)
									}
								})
						}
					})
				})
			}
			if shown == 0 {
				mgo.Text(c, "没有匹配的 Provider。目录由内置插件提供，可在插件设置中启用。").TextColor(k.TextMuted)
			}
		})
	})
}

// pluginProviderCatalog reads the provider suggestions from the plugin
// manager, nil when no manager is up (tests without plugins).
func (cw *ChatWindow) pluginProviderCatalog() []plugin.ProviderPresetEntry {
	if cw.plugins == nil {
		return nil
	}
	return cw.plugins.ProviderCatalog()
}
