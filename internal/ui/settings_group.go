package ui

import (
	"fmt"
	"strings"

	"chatgo/internal/config"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// groupCard renders one provider group row with an expandable editor.
func (cw *ChatWindow) groupCard(c *mgo.Context, i int) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	g := cw.cfg.ProviderGroups[i]
	expanded := cw.settings.group.editing == i
	card := mgo.Column(c).Key(fmt.Sprintf("group:%d", i)).FillWidth().Gap(u * 2).Padding(u * 2).Radius(mtheme.ControlRadius)
	if expanded {
		card.Background(k.Selection)
	} else {
		card.Background(k.Surface)
	}
	card.Children(func() {
		mgo.Row(c).FillWidth().Justify(mgo.SpaceBetween).AlignItems(mgo.Center).Children(func() {
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				mgo.Icon(c, icons.Must("sliders-horizontal")).Size(18, 18).TextColor(k.TextMuted).Shrink(0)
				mgo.Text(c, g.Name).FontWeight(600).Shrink(0)
				mgo.Text(c, config.GetStrategyDisplayName(g.EffectiveStrategy())).
					FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
				mgo.Text(c, fmt.Sprintf("%d 个 Provider", g.MemberCount())).
					FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
			})
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				en := g.Enabled
				if mgo.Switch(c, &en).Changed() {
					cw.cfg.ProviderGroups[i].Enabled = en
					cw.settings.group.enabled = en
					_ = config.SaveConfig(cw.cfg)
					if cw.provider == g.Name {
						cw.clientDirty.Store(true)
					}
				}
				if mcore.IconAction(c, icons.Must("chevron-down"), "展开").Clicked() {
					if expanded {
						cw.clearGroupForm()
					} else {
						cw.loadGroupForm(i)
					}
				}
			})
		})
		mgo.Row(c).Gap(u).Wrap().Children(func() {
			for _, m := range g.Members {
				if len(m.Models) == 0 {
					continue
				}
				mgo.Text(c, m.Provider+" · "+strings.Join(m.Models, " / ")).
					FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted).SingleLine()
			}
		})
		if expanded {
			mgo.Box(c).FillWidth().Height(1).Background(k.Border)
			cw.groupFormFields(c)
		}
	})
}

// groupFormFields renders the editor of the expanded group.
func (cw *ChatWindow) groupFormFields(c *mgo.Context) {
	f := &cw.settings.group
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	changed := false
	mgo.Column(c).FillWidth().Gap(u * 2).Children(func() {
		if r := kit.TextInput(c, &f.name, kit.TextInputOptions{Label: "名称", Placeholder: "模型组名称"}); r.Changed() {
			changed = true
		}
		opts := make([]kit.SelectOption[string], 0, len(config.GroupStrategies()))
		for _, s := range config.GroupStrategies() {
			opts = append(opts, kit.SelectOption[string]{Value: s, Label: config.GetStrategyDisplayName(s)})
		}
		if r := kit.Select(c, &f.strategy, opts, kit.SelectOptions{Placeholder: "路由策略", Label: "路由策略"}); r.Changed() {
			changed = true
		}
		mgo.Text(c, "成员").
			FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
		if len(cw.customProviderNames()) == 0 {
			mgo.Text(c, "请先在「模型」页新增模型，再选择它们组成模型组。").
				FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
		}
		for idx := range f.members {
			idx := idx
			cw.groupMemberFields(c, idx, &changed)
		}
		mgo.Row(c).Grow(0).Gap(u).AlignItems(mgo.Center).Children(func() {
			if kit.Button(c, "添加 Provider", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("plus")}).Clicked() {
				cw.addGroupMember()
			}
			if kit.Button(c, "保存", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
				cw.saveGroup()
			}
			if kit.Button(c, "删除", kit.ButtonOptions{Variant: kit.Danger}).Clicked() {
				cw.deleteGroup()
			}
		})
	})
	if changed && f.editing >= 0 {
		cw.persistGroup()
	}
}

// groupMemberFields renders one provider of the group: a provider choice and
// checkboxes for the models that provider serves.
func (cw *ChatWindow) groupMemberFields(c *mgo.Context, idx int, changed *bool) {
	f := &cw.settings.group
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	customs := cw.customProviderNames()
	providerOpts := make([]kit.SelectOption[string], 0, len(customs))
	for _, name := range customs {
		providerOpts = append(providerOpts, kit.SelectOption[string]{Value: name, Label: name})
	}
	if len(providerOpts) == 0 {
		return
	}
	mgo.Column(c).FillWidth().Gap(u).Padding(u, u*1.5).Radius(mtheme.ControlRadius).
		Background(k.Background).Children(func() {
		mgo.Row(c).Gap(u).AlignItems(mgo.End).Children(func() {
			sel := kit.Select(c, &f.members[idx].Provider, providerOpts,
				kit.SelectOptions{Placeholder: "Provider", Label: "Provider"})
			sel.Element.Grow(1).MinWidth(0)
			if sel.Changed() {
				*changed = true
				f.members[idx].Models = nil
				cw.selectFirstMemberModel(idx)
			}
			if len(f.members) > 1 {
				if mcore.IconAction(c, icons.Must("x"), "移除 Provider").Clicked() {
					f.members = append(f.members[:idx], f.members[idx+1:]...)
					*changed = true
				}
			}
		})
		models := cw.providerModelNames(f.members[idx].Provider)
		if len(models) == 0 {
			mgo.Text(c, "该 Provider 还没有配置模型").
				FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
			return
		}
		mgo.Row(c).Gap(u).Wrap().Children(func() {
			for _, model := range models {
				model := model
				checked := containsStr(f.members[idx].Models, model)
				mgo.Checkbox(c, &checked, model)
				if checked == containsStr(f.members[idx].Models, model) {
					continue
				}
				f.members[idx].Models = setTool(f.members[idx].Models, model, checked)
				*changed = true
			}
		})
	})
}

// providerModelNames lists the model names of the named provider.
func (cw *ChatWindow) providerModelNames(name string) []string {
	p, ok := cw.findProvider(name)
	if !ok {
		return nil
	}
	return p.ModelsList()
}

// selectFirstMemberModel checks the first model of the member's provider.
func (cw *ChatWindow) selectFirstMemberModel(idx int) {
	f := &cw.settings.group
	if idx < 0 || idx >= len(f.members) {
		return
	}
	if models := cw.providerModelNames(f.members[idx].Provider); len(models) > 0 {
		f.members[idx].Models = []string{models[0]}
	}
}

// customProviderNames lists the configured custom providers, the only ones a
// group member may point at.
func (cw *ChatWindow) customProviderNames() []string {
	out := make([]string, 0, len(cw.cfg.Providers))
	for _, p := range cw.cfg.Providers {
		if p.Type == config.ProviderTypeCustom {
			out = append(out, p.Name)
		}
	}
	return out
}

// addGroupMember appends a member starting at the first unused provider.
func (cw *ChatWindow) addGroupMember() {
	f := &cw.settings.group
	used := make(map[string]bool, len(f.members))
	for _, m := range f.members {
		used[m.Provider] = true
	}
	member := config.GroupMember{}
	for _, name := range cw.customProviderNames() {
		if !used[name] {
			member.Provider = name
			break
		}
	}
	if member.Provider == "" {
		customs := cw.customProviderNames()
		if len(customs) > 0 {
			member.Provider = customs[0]
		}
	}
	f.members = append(f.members, member)
	cw.selectFirstMemberModel(len(f.members) - 1)
	cw.persistGroup()
}

func (cw *ChatWindow) loadGroupForm(i int) {
	g := cw.cfg.ProviderGroups[i]
	cw.settings.group = groupForm{
		editing:  i,
		name:     g.Name,
		strategy: g.EffectiveStrategy(),
		members:  cloneGroupMembers(g.Members),
		enabled:  g.Enabled,
	}
}

func (cw *ChatWindow) clearGroupForm() {
	cw.settings.group = groupForm{editing: -1, strategy: config.StrategyFallback}
}

func (cw *ChatWindow) saveGroup() {
	if cw.persistGroup() {
		cw.toast("已保存模型组 "+cw.settings.group.name, mcore.SeveritySuccess)
	}
}

// persistGroup writes the edited group into cfg and saves it.
func (cw *ChatWindow) persistGroup() bool {
	f := &cw.settings.group
	name := strings.TrimSpace(f.name)
	if name == "" {
		return false
	}
	for i, g := range cw.cfg.ProviderGroups {
		if i != f.editing && g.Name == name {
			cw.fail("已存在同名模型组: " + name)
			return false
		}
	}
	members := make([]config.GroupMember, 0, len(f.members))
	for _, m := range f.members {
		if m.Provider == "" || len(m.Models) == 0 {
			continue
		}
		members = append(members, config.GroupMember{
			Provider: m.Provider,
			Models:   append([]string(nil), m.Models...),
		})
	}
	g := config.ProviderGroup{
		Name: name, Strategy: f.strategy, Members: members, Enabled: f.enabled,
	}
	if f.editing >= 0 && f.editing < len(cw.cfg.ProviderGroups) {
		cw.cfg.ProviderGroups[f.editing] = g
	} else {
		cw.cfg.ProviderGroups = append(cw.cfg.ProviderGroups, g)
		f.editing = len(cw.cfg.ProviderGroups) - 1
	}
	if err := config.SaveConfig(cw.cfg); err != nil {
		cw.fail("保存失败: " + err.Error())
		return false
	}
	if cw.provider == name {
		cw.clientDirty.Store(true)
	}
	return true
}

func (cw *ChatWindow) addNewGroup() {
	customs := cw.customProviderNames()
	g := config.ProviderGroup{
		Name:     cw.nextGroupName(),
		Strategy: config.StrategyFallback,
		Enabled:  false,
	}
	if len(customs) > 0 {
		member := config.GroupMember{Provider: customs[0]}
		if p, ok := cw.findProvider(customs[0]); ok {
			if ms := p.ModelsList(); len(ms) > 0 {
				member.Models = []string{ms[0]}
			}
		}
		g.Members = []config.GroupMember{member}
	}
	cw.cfg.ProviderGroups = append(cw.cfg.ProviderGroups, g)
	if err := config.SaveConfig(cw.cfg); err != nil {
		cw.fail("保存失败: " + err.Error())
		return
	}
	cw.loadGroupForm(len(cw.cfg.ProviderGroups) - 1)
}

// cloneGroupMembers copies group members and their model slices.
func cloneGroupMembers(in []config.GroupMember) []config.GroupMember {
	out := make([]config.GroupMember, 0, len(in))
	for _, m := range in {
		out = append(out, config.GroupMember{
			Provider: m.Provider,
			Models:   append([]string(nil), m.Models...),
		})
	}
	return out
}

// nextGroupName returns an unused default name for a new group.
func (cw *ChatWindow) nextGroupName() string {
	for i := 1; ; i++ {
		name := fmt.Sprintf("模型组 %d", i)
		if _, ok := cw.cfg.FindProviderGroup(name); !ok {
			return name
		}
	}
}

func (cw *ChatWindow) deleteGroup() {
	f := &cw.settings.group
	if f.editing < 0 || f.editing >= len(cw.cfg.ProviderGroups) {
		return
	}
	idx := f.editing
	name := cw.cfg.ProviderGroups[idx].Name
	cw.askConfirm("删除模型组", "确定删除「"+name+"」？", func() {
		cw.cfg.ProviderGroups = append(cw.cfg.ProviderGroups[:idx], cw.cfg.ProviderGroups[idx+1:]...)
		_ = config.SaveConfig(cw.cfg)
		cw.clearGroupForm()
		if cw.provider == name {
			cw.clientDirty.Store(true)
		}
	})
}
