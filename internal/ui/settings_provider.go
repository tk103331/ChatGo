package ui

import (
	"fmt"
	"strings"

	"chatgo/internal/config"
	"chatgo/internal/ui/assets"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

func (cw *ChatWindow) providersTab(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		mgo.Column(c).FillWidth().Shrink(0).Gap(u * 2).Padding(u).MaxWidth(640).Children(func() {
			mgo.Row(c).FillWidth().Justify(mgo.SpaceBetween).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, "模型").Font(mtheme.SerifFont).
					FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600)
				if kit.Button(c, "新增", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("plus")}).Clicked() {
					cw.addNewProvider()
				}
			})
			if len(cw.cfg.Providers) == 0 {
				cw.providerEmptyState(c)
			} else {
				for i := range cw.cfg.Providers {
					cw.providerCard(c, i)
				}
			}
		})
	})
}

// providerEmptyState shows a centered placeholder when no providers exist.
func (cw *ChatWindow) providerEmptyState(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Grow(1).AlignItems(mgo.Center).Justify(mgo.Center).Gap(u*2).
		Padding(u*8, 0).Children(func() {
		mgo.Icon(c, icons.Must("brain")).Size(48, 48).TextColor(k.TextMuted)
		mgo.Text(c, "还没有模型").Font(mtheme.SerifFont).
			FontSize(mcore.FontSize(c, mtheme.H3Size)).TextColor(k.TextMuted)
		mgo.Text(c, "添加 OpenAI、Anthropic、Ollama 等模型提供商").TextColor(k.TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		if kit.Button(c, "新增模型", kit.ButtonOptions{Variant: kit.Primary, Icon: icons.Must("plus")}).Clicked() {
			cw.addNewProvider()
		}
	})
}

func (cw *ChatWindow) addNewProvider() {
	cw.cfg.Providers = append(cw.cfg.Providers, config.Provider{
		Name: "New Provider", Type: "openai", Enabled: false,
	})
	_ = config.SaveConfig(cw.cfg)
	cw.loadProviderForm(len(cw.cfg.Providers) - 1)
}

func (cw *ChatWindow) providerCard(c *mgo.Context, i int) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	p := cw.cfg.Providers[i]
	expanded := cw.settings.prov.editing == i
	card := mgo.Column(c).Key(fmt.Sprintf("prov:%d", i)).FillWidth().Gap(u * 2).Padding(u * 2).Radius(mtheme.ControlRadius)
	if expanded {
		card.Background(k.Selection)
	} else {
		card.Background(k.Surface)
	}
	card.Children(func() {
		mgo.Row(c).FillWidth().Justify(mgo.SpaceBetween).AlignItems(mgo.Center).Children(func() {
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				if svg := assets.ProviderLogo(p.Type); svg != nil {
					mgo.Icon(c, svg).Size(18, 18).TextColor(k.Text).Shrink(0)
				} else {
					mgo.Icon(c, icons.Must("sliders-horizontal")).Size(18, 18).TextColor(k.TextMuted).Shrink(0)
				}
				mgo.Text(c, p.Name).Font(mtheme.SerifFont).FontWeight(600).Shrink(0)
				mgo.Text(c, p.Type).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
			})
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				en := p.Enabled
				if kit.Switch(c, &en, "", kit.SwitchOptions{}).Changed() {
					cw.cfg.Providers[i].Enabled = en
					cw.settings.prov.enabled = en
					_ = config.SaveConfig(cw.cfg)
					if cw.provider == p.Name {
						cw.clientDirty.Store(true)
					}
				}
				if mcore.IconAction(c, icons.Must("chevron-down"), "展开").Clicked() {
					if expanded {
						cw.clearProviderForm()
					} else {
						cw.loadProviderForm(i)
					}
				}
			})
		})
		if expanded {
			mgo.Box(c).FillWidth().Height(1).Background(k.Border)
			cw.providerFormFields(c)
		}
	})
}

func (cw *ChatWindow) providerFormFields(c *mgo.Context) {
	f := &cw.settings.prov
	u := mcore.Density(c).Unit()
	changed := false
	mgo.Column(c).FillWidth().Gap(u * 2).Children(func() {
		if r := kit.TextInput(c, &f.name, kit.TextInputOptions{Label: "名称", Placeholder: "Provider 名称", ReadOnly: f.preset}); r.Changed() {
			changed = true
		}
		opts := make([]kit.SelectOption[string], 0, len(providerTypes))
		for _, t := range providerTypes {
			opts = append(opts, kit.SelectOption[string]{Value: t, Label: t})
		}
		if r := kit.Select(c, &f.typ, opts, kit.SelectOptions{Placeholder: "类型", Label: "类型", Disabled: f.preset}); r.Changed() {
			changed = true
		}
		if r := kit.PasswordInput(c, &f.apiKey, kit.PasswordInputOptions{Label: "API Key", Placeholder: "API Key"}); r.Changed() {
			changed = true
		}
		if r := kit.TextInput(c, &f.baseURL, kit.TextInputOptions{Label: "Base URL", Placeholder: "Base URL"}); r.Changed() {
			changed = true
		}
		mgo.Text(c, "模型").FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(mcore.Tokens(c).TextMuted)
		for _, m := range f.models {
			m := m
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, m).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
				if mcore.IconAction(c, icons.Must("x"), "移除模型").Clicked() {
					f.models = removeStr(f.models, m)
					changed = true
				}
			})
		}
		mgo.Row(c).Gap(u).AlignItems(mgo.End).Children(func() {
			in := kit.TextInput(c, &f.newModel, kit.TextInputOptions{Placeholder: "添加模型名"})
			in.Element.Grow(1)
			if kit.Button(c, "添加", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
				if name := strings.TrimSpace(f.newModel); name != "" {
					if !containsStr(f.models, name) {
						f.models = append(f.models, name)
					}
					f.newModel = ""
					changed = true
				}
			}
		})
		mgo.Row(c).Grow(0).Gap(u).AlignItems(mgo.Center).Children(func() {
			if kit.Button(c, "保存", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
				cw.saveProvider()
			}
			if kit.Button(c, "删除", kit.ButtonOptions{Variant: kit.Danger}).Clicked() {
				cw.deleteProvider()
			}
		})
	})
	if changed && f.editing >= 0 {
		cw.persistProvider()
	}
}

func (cw *ChatWindow) loadProviderForm(i int) {
	p := cw.cfg.Providers[i]
	cw.settings.prov = providerForm{
		editing: i, name: p.Name, typ: p.Type, apiKey: p.APIKey,
		baseURL: p.BaseURL, models: append([]string(nil), p.ModelsList()...), enabled: p.Enabled,
		preset: p.Preset || providerIsPreset(p),
	}
}

// providerIsPreset reports whether a provider matches a shipped example.

func providerIsPreset(p config.Provider) bool {
	for _, pr := range config.ProviderPresets() {
		if pr.Name == p.Name && pr.Type == p.Type {
			return true
		}
	}
	return false
}

func (cw *ChatWindow) clearProviderForm() {
	cw.settings.prov = providerForm{editing: -1, typ: "openai", enabled: false}
}

func (cw *ChatWindow) saveProvider() {
	if cw.persistProvider() {
		cw.status = ""
		cw.toast("已保存模型 "+cw.settings.prov.name, mcore.SeveritySuccess)
	}
}

// persistProvider writes the edited provider into cfg and saves it.

func (cw *ChatWindow) persistProvider() bool {
	f := &cw.settings.prov
	if strings.TrimSpace(f.name) == "" {
		return false
	}
	model := ""
	if len(f.models) > 0 {
		model = f.models[0]
	}
	p := config.Provider{
		Name: f.name, Type: f.typ, APIKey: f.apiKey,
		BaseURL: f.baseURL, Model: model, Models: append([]string(nil), f.models...), Enabled: f.enabled, Preset: f.preset,
	}
	if f.editing >= 0 && f.editing < len(cw.cfg.Providers) {
		cw.cfg.Providers[f.editing] = p
	} else {
		cw.cfg.Providers = append(cw.cfg.Providers, p)
		f.editing = len(cw.cfg.Providers) - 1
	}
	if err := config.SaveConfig(cw.cfg); err != nil {
		cw.fail("保存失败: " + err.Error())
		return false
	}
	if cw.provider == p.Name {
		cw.clientDirty.Store(true)
	}
	return true
}

// removeStr returns s without v.

func (cw *ChatWindow) deleteProvider() {
	f := &cw.settings.prov
	if f.editing < 0 || f.editing >= len(cw.cfg.Providers) {
		return
	}
	idx := f.editing
	name := cw.cfg.Providers[idx].Name
	cw.askConfirm("删除模型", "确定删除「"+name+"」？", func() {
		cw.cfg.Providers = append(cw.cfg.Providers[:idx], cw.cfg.Providers[idx+1:]...)
		_ = config.SaveConfig(cw.cfg)
		cw.clearProviderForm()
		if cw.provider == name {
			cw.clientDirty.Store(true)
		}
	})
}

// mcpTab shows the configured MCP servers as a card grid.
