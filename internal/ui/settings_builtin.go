package ui

import (
	"fmt"

	"chatgo/internal/config"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

func (cw *ChatWindow) builtinTab(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		mgo.Column(c).FillWidth().Shrink(0).Gap(u * 2).Padding(u).MaxWidth(640).Children(func() {
			mgo.Text(c, "工具").
				FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600)
			if len(cw.cfg.BuiltinTools) == 0 {
				mgo.Text(c, "暂无工具").TextColor(k.TextMuted).
					FontSize(mcore.FontSize(c, mtheme.CaptionSize))
			}
			for i := range cw.cfg.BuiltinTools {
				cw.builtinCard(c, i)
			}
		})
	})
}

func (cw *ChatWindow) builtinCard(c *mgo.Context, i int) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	t := cw.cfg.BuiltinTools[i]
	expanded := cw.settings.builtin.editing == i
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
				mgo.Text(c, config.GetBuiltinToolDisplayName(t.Type)).FontWeight(600).Shrink(0)
				mgo.Text(c, t.Type).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
			})
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				en := t.Enabled
				if mgo.Switch(c, &en).Changed() {
					cw.cfg.BuiltinTools[i].Enabled = en
					cw.settings.builtin.enabled = en
					_ = config.SaveConfig(cw.cfg)
					cw.clientDirty.Store(true)
				}
				if mcore.IconAction(c, icons.Must("chevron-down"), "展开").Clicked() {
					if expanded {
						cw.settings.builtin = builtinForm{editing: -1}
					} else {
						cw.loadBuiltinForm(i)
					}
				}
			})
		})
		if expanded {
			mgo.Box(c).FillWidth().Height(1).Background(k.Border)
			cw.builtinFormFields(c)
		}
	})
}

func (cw *ChatWindow) builtinFormFields(c *mgo.Context) {
	f := &cw.settings.builtin
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	if f.editing < 0 || f.editing >= len(cw.cfg.BuiltinTools) {
		return
	}
	tool := cw.cfg.BuiltinTools[f.editing]
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
			v := f.config[field]
			kit.TextInput(c, &v, kit.TextInputOptions{Label: label, Placeholder: field})
			f.config[field] = v
		}
		if kit.Button(c, "保存", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
			cw.saveBuiltin()
		}
	})
}

func (cw *ChatWindow) loadBuiltinForm(i int) {
	t := cw.cfg.BuiltinTools[i]
	cfgMap := map[string]string{}
	for k, v := range t.Config {
		cfgMap[k] = v
	}
	cw.settings.builtin = builtinForm{editing: i, enabled: t.Enabled, config: cfgMap}
}

func (cw *ChatWindow) saveBuiltin() {
	f := &cw.settings.builtin
	if f.editing < 0 || f.editing >= len(cw.cfg.BuiltinTools) {
		return
	}
	t := cw.cfg.BuiltinTools[f.editing]
	t.Enabled = f.enabled
	t.Config = f.config
	if t.Enabled {
		if err := config.ValidateBuiltinToolConfig(t); err != nil {
			cw.fail(err.Error())
			return
		}
	}
	cw.cfg.BuiltinTools[f.editing] = t
	_ = config.SaveConfig(cw.cfg)
	cw.status = ""
	cw.toast("已保存工具配置", mcore.SeveritySuccess)
	cw.clientDirty.Store(true)
}

// shortcutTab edits the global shortcut that opens the focused interface.
