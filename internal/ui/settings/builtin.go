package settings

import (
	"fmt"

	"chatgo/internal/config"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// builtinTab lists the configured built-in tools.
func builtinTab(c *mgo.Context, state *State, opts Options) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		mgo.Column(c).FillWidth().Shrink(0).Gap(u * 2).Padding(u).MaxWidth(640).Children(func() {
			mgo.Text(c, "工具").Font(mtheme.SerifFont).
				FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600)
			if len(opts.Config().BuiltinTools) == 0 {
				mgo.Text(c, "暂无工具").TextColor(k.TextMuted).
					FontSize(mcore.FontSize(c, mtheme.CaptionSize))
			}
			for i := range opts.Config().BuiltinTools {
				builtinCard(c, state, opts, i)
			}
		})
	})
}

// builtinCard renders one built-in tool with its expandable config form.
func builtinCard(c *mgo.Context, state *State, opts Options, i int) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	t := opts.Config().BuiltinTools[i]
	expanded := state.Builtin.Editing == i
	card := mgo.Column(c).Key(fmt.Sprintf("builtin:%d", i)).FillWidth().Gap(u * 2).Padding(u * 2).Radius(mtheme.ControlRadius)
	if expanded {
		card.Background(k.Selection)
	} else {
		card.Background(k.Surface)
	}
	card.Children(func() {
		mgo.Row(c).FillWidth().Justify(mgo.SpaceBetween).AlignItems(mgo.Center).Children(func() {
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				mgo.Icon(c, icons.Must("settings")).Size(18, 18).TextColor(k.Text).Shrink(0)
				mgo.Text(c, config.GetBuiltinToolDisplayName(t.Type)).Font(mtheme.SerifFont).FontWeight(600).Shrink(0)
				mgo.Text(c, t.Type).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
			})
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				en := t.Enabled
				if minput.Switch(c, &en, "", minput.SwitchOptions{}).Changed() {
					opts.Config().BuiltinTools[i].Enabled = en
					state.Builtin.Enabled = en
					_ = opts.SaveConfig()
					opts.MarkDirty()
				}
				if mcore.IconAction(c, icons.Must("chevron-down"), "展开").Clicked() {
					if expanded {
						state.Builtin = BuiltinForm{Editing: -1}
					} else {
						LoadBuiltinForm(state, opts, i)
					}
				}
			})
		})
		if expanded {
			mgo.Box(c).FillWidth().Height(1).Background(k.Border)
			builtinFormFields(c, state, opts)
		}
	})
}

// builtinFormFields renders the editable config fields of the expanded tool.
func builtinFormFields(c *mgo.Context, state *State, opts Options) {
	f := &state.Builtin
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	if f.Editing < 0 || f.Editing >= len(opts.Config().BuiltinTools) {
		return
	}
	tool := opts.Config().BuiltinTools[f.Editing]
	mgo.Column(c).FillWidth().Gap(u * 2).Children(func() {
		mgo.Text(c, config.GetBuiltinToolDescription(tool.Type)).TextColor(k.TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		required := config.GetRequiredConfigFields(tool.Type)
		for _, field := range config.GetBuiltinToolConfigFields(tool.Type) {
			label := field
			for _, r := range required {
				if r == field {
					label = field + " *"
				}
			}
			v := f.Config[field]
			minput.TextInput(c, &v, minput.TextInputOptions{Label: label, Placeholder: field})
			f.Config[field] = v
		}
		if minput.Button(c, "保存", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
			SaveBuiltin(state, opts)
		}
	})
}

// LoadBuiltinForm fills the form from the tool at index i.
func LoadBuiltinForm(state *State, opts Options, i int) {
	t := opts.Config().BuiltinTools[i]
	cfgMap := map[string]string{}
	for k, v := range t.Config {
		cfgMap[k] = v
	}
	state.Builtin = BuiltinForm{Editing: i, Enabled: t.Enabled, Config: cfgMap}
}

// SaveBuiltin validates and persists the edited tool config.
func SaveBuiltin(state *State, opts Options) {
	f := &state.Builtin
	cfg := opts.Config()
	if f.Editing < 0 || f.Editing >= len(cfg.BuiltinTools) {
		return
	}
	t := cfg.BuiltinTools[f.Editing]
	t.Enabled = f.Enabled
	t.Config = f.Config
	if t.Enabled {
		if err := config.ValidateBuiltinToolConfig(t); err != nil {
			opts.Fail(err.Error())
			return
		}
	}
	cfg.BuiltinTools[f.Editing] = t
	_ = opts.SaveConfig()
	opts.SetStatus("")
	opts.Toast("已保存工具配置", mcore.SeveritySuccess)
	opts.MarkDirty()
}
