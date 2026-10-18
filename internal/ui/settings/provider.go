package settings

import (
	"fmt"
	"strings"

	"chatgo/internal/config"
	"chatgo/internal/ui/assets"
	"chatgo/internal/ui/common"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// providersTab lists the configured model providers.
func providersTab(c *mgo.Context, state *State, opts Options) {
	u := mcore.Density(c).Unit()
	mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		mgo.Column(c).FillWidth().Shrink(0).Gap(u * 2).Padding(u).MaxWidth(640).Children(func() {
			mgo.Row(c).FillWidth().Justify(mgo.SpaceBetween).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, "模型").Font(mtheme.SerifFont).
					FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600)
				if minput.Button(c, "新增", minput.ButtonOptions{Variant: minput.Secondary, Icon: icons.Must("plus")}).Clicked() {
					AddNewProvider(state, opts)
				}
			})
			if len(opts.Config().Providers) == 0 {
				providerEmptyState(c, state, opts)
			} else {
				for i := range opts.Config().Providers {
					providerCard(c, state, opts, i)
				}
			}
		})
	})
}

// providerEmptyState shows a centered placeholder when no providers exist.
func providerEmptyState(c *mgo.Context, state *State, opts Options) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Grow(1).AlignItems(mgo.Center).Justify(mgo.Center).Gap(u*2).
		Padding(u*8, 0).Children(func() {
		mgo.Icon(c, icons.Must("brain")).Size(48, 48).TextColor(k.TextMuted)
		mgo.Text(c, "还没有模型").Font(mtheme.SerifFont).
			FontSize(mcore.FontSize(c, mtheme.H3Size)).TextColor(k.TextMuted)
		mgo.Text(c, "添加 OpenAI、Anthropic、Ollama 等模型提供商").TextColor(k.TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		if minput.Button(c, "新增模型", minput.ButtonOptions{Variant: minput.Primary, Icon: icons.Must("plus")}).Clicked() {
			AddNewProvider(state, opts)
		}
	})
}

// AddNewProvider appends a disabled placeholder provider and opens its form.
func AddNewProvider(state *State, opts Options) {
	cfg := opts.Config()
	cfg.Providers = append(cfg.Providers, config.Provider{
		Name: "New Provider", Type: "openai", Enabled: false,
	})
	_ = opts.SaveConfig()
	LoadProviderForm(state, opts, len(cfg.Providers)-1)
}

// providerCard renders one provider row with its expandable form.
func providerCard(c *mgo.Context, state *State, opts Options, i int) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	p := opts.Config().Providers[i]
	expanded := state.Prov.Editing == i
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
				if minput.Switch(c, &en, "", minput.SwitchOptions{}).Changed() {
					opts.Config().Providers[i].Enabled = en
					state.Prov.Enabled = en
					_ = opts.SaveConfig()
					if opts.CurrentProvider() == p.Name {
						opts.MarkDirty()
					}
				}
				if mcore.IconAction(c, icons.Must("chevron-down"), "展开").Clicked() {
					if expanded {
						ClearProviderForm(state)
					} else {
						LoadProviderForm(state, opts, i)
					}
				}
			})
		})
		if expanded {
			mgo.Box(c).FillWidth().Height(1).Background(k.Border)
			providerFormFields(c, state, opts)
		}
	})
}

// providerFormFields renders the editable fields of the expanded provider.
func providerFormFields(c *mgo.Context, state *State, opts Options) {
	f := &state.Prov
	u := mcore.Density(c).Unit()
	changed := false
	mgo.Column(c).FillWidth().Gap(u * 2).Children(func() {
		if r := minput.TextInput(c, &f.Name, minput.TextInputOptions{Label: "名称", Placeholder: "Provider 名称", ReadOnly: f.Preset}); r.Changed() {
			changed = true
		}
		opts2 := make([]minput.SelectOption[string], 0, len(ProviderTypes))
		for _, t := range ProviderTypes {
			opts2 = append(opts2, minput.SelectOption[string]{Value: t, Label: t})
		}
		if r := minput.Select(c, &f.Typ, opts2, minput.SelectOptions{Placeholder: "类型", Label: "类型", Disabled: f.Preset}); r.Changed() {
			changed = true
		}
		if r := minput.PasswordInput(c, &f.APIKey, minput.PasswordInputOptions{Label: "API Key", Placeholder: "API Key"}); r.Changed() {
			changed = true
		}
		if r := minput.TextInput(c, &f.BaseURL, minput.TextInputOptions{Label: "Base URL", Placeholder: "Base URL"}); r.Changed() {
			changed = true
		}
		mgo.Text(c, "模型").FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(mcore.Tokens(c).TextMuted)
		for _, m := range f.Models {
			m := m
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, m).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
				if mcore.IconAction(c, icons.Must("x"), "移除模型").Clicked() {
					f.Models = common.RemoveStr(f.Models, m)
					changed = true
				}
			})
		}
		mgo.Row(c).Gap(u).AlignItems(mgo.End).Children(func() {
			in := minput.TextInput(c, &f.NewModel, minput.TextInputOptions{Placeholder: "添加模型名"})
			in.Element.Grow(1)
			if minput.Button(c, "添加", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
				if name := strings.TrimSpace(f.NewModel); name != "" {
					if !common.ContainsStr(f.Models, name) {
						f.Models = append(f.Models, name)
					}
					f.NewModel = ""
					changed = true
				}
			}
		})
		mgo.Row(c).Grow(0).Gap(u).AlignItems(mgo.Center).Children(func() {
			if minput.Button(c, "保存", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
				SaveProvider(state, opts)
			}
			if minput.Button(c, "删除", minput.ButtonOptions{Variant: minput.Danger}).Clicked() {
				DeleteProvider(state, opts)
			}
		})
	})
	if changed && f.Editing >= 0 {
		PersistProvider(state, opts)
	}
}

// LoadProviderForm fills the form from the provider at index i.
func LoadProviderForm(state *State, opts Options, i int) {
	p := opts.Config().Providers[i]
	state.Prov = ProviderForm{
		Editing: i, Name: p.Name, Typ: p.Type, APIKey: p.APIKey,
		BaseURL: p.BaseURL, Models: append([]string(nil), p.ModelsList()...), Enabled: p.Enabled,
		Preset: p.Preset || ProviderIsPreset(p),
	}
}

// ProviderIsPreset reports whether a provider matches a shipped example.
func ProviderIsPreset(p config.Provider) bool {
	for _, pr := range config.ProviderPresets() {
		if pr.Name == p.Name && pr.Type == p.Type {
			return true
		}
	}
	return false
}

// ClearProviderForm closes the expanded provider form.
func ClearProviderForm(state *State) {
	state.Prov = ProviderForm{Editing: -1, Typ: "openai", Enabled: false}
}

// SaveProvider persists the edited provider and shows a toast.
func SaveProvider(state *State, opts Options) {
	if PersistProvider(state, opts) {
		opts.SetStatus("")
		opts.Toast("已保存模型 "+state.Prov.Name, mcore.SeveritySuccess)
	}
}

// PersistProvider writes the edited provider into cfg and saves it.
func PersistProvider(state *State, opts Options) bool {
	f := &state.Prov
	if strings.TrimSpace(f.Name) == "" {
		return false
	}
	model := ""
	if len(f.Models) > 0 {
		model = f.Models[0]
	}
	p := config.Provider{
		Name: f.Name, Type: f.Typ, APIKey: f.APIKey,
		BaseURL: f.BaseURL, Model: model, Models: append([]string(nil), f.Models...), Enabled: f.Enabled, Preset: f.Preset,
	}
	cfg := opts.Config()
	if f.Editing >= 0 && f.Editing < len(cfg.Providers) {
		cfg.Providers[f.Editing] = p
	} else {
		cfg.Providers = append(cfg.Providers, p)
		f.Editing = len(cfg.Providers) - 1
	}
	if err := opts.SaveConfig(); err != nil {
		opts.Fail("保存失败: " + err.Error())
		return false
	}
	if opts.CurrentProvider() == p.Name {
		opts.MarkDirty()
	}
	return true
}

// DeleteProvider removes the edited provider after confirmation.
func DeleteProvider(state *State, opts Options) {
	f := &state.Prov
	cfg := opts.Config()
	if f.Editing < 0 || f.Editing >= len(cfg.Providers) {
		return
	}
	idx := f.Editing
	name := cfg.Providers[idx].Name
	opts.AskConfirm("删除模型", "确定删除「"+name+"」？", func() {
		cfg.Providers = append(cfg.Providers[:idx], cfg.Providers[idx+1:]...)
		_ = opts.SaveConfig()
		ClearProviderForm(state)
		if opts.CurrentProvider() == name {
			opts.MarkDirty()
		}
	})
}
