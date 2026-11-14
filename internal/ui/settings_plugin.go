package ui

import (
	"strconv"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/plugin"
	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// pluginsTab lists installed plugins, installs new ones and edits settings.
func (cw *ChatWindow) pluginsTab(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	if cw.plugins == nil {
		mgo.Text(c, "插件系统未初始化").TextColor(k.TextMuted)
		return
	}
	infos := cw.plugins.List()
	visible := filterPlugins(infos, cw.pluginUI.filter, cw.pluginUI.query)
	mgo.Column(c).Fill().Gap(u * 2).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, "插件").FontWeight(600).Grow(1)
			mgo.Text(c, "共 "+strconv.Itoa(len(visible))+" / "+strconv.Itoa(len(infos))+" 个").
				FontSize(small).TextColor(k.TextMuted)
			if kit.Button(c, "安装插件", kit.ButtonOptions{Variant: kit.Primary, Icon: icons.Must("plus")}).Clicked() {
				cw.pluginUI.installOpen = !cw.pluginUI.installOpen
			}
		})
		// The category filter and the name search narrow the list — most of
		// it is the per-vendor built-in catalogs, and finding one plugin
		// among seventy needs both.
		mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
			cw.pluginFilterControl(c)
			in := kit.SearchInput(c, &cw.pluginUI.query, kit.SearchInputOptions{Placeholder: "搜索插件", Label: "搜索插件"})
			in.Element.Grow(1).MinWidth(0)
		})
		if len(infos) > 0 && len(visible) == 0 {
			mgo.Text(c, "没有符合过滤条件的插件").TextColor(k.TextMuted)
			return
		}
		if len(infos) == 0 {
			mgo.Column(c).Grow(1).AlignItems(mgo.Center).Justify(mgo.Center).Gap(u).Children(func() {
				mgo.Icon(c, icons.Must("layout-grid")).Size(40, 40).TextColor(k.TextMuted)
				mgo.Text(c, "还没有插件").TextColor(k.TextMuted)
				mgo.Text(c, "支持目录、ZIP、Git 与 npm 四种来源").FontSize(small).TextColor(k.TextMuted)
			})
			return
		}
		mgo.Row(c).Grow(1).MinHeight(0).Gap(u * 3).AlignItems(mgo.Stretch).Children(func() {
			cw.pluginList(c, visible)
			mgo.Divider(c)
			cw.pluginDetail(c, visible)
		})
	})
	cw.pluginInstallDialog(c)
}

// pluginFilterControl is the category segmented control.
func (cw *ChatWindow) pluginFilterControl(c *mgo.Context) {
	filters := pluginFilterCategories()
	opts := make([]kit.SelectOption[string], 0, len(filters))
	for _, f := range filters {
		opts = append(opts, kit.SelectOption[string]{Value: f.id, Label: f.label})
	}
	kit.Select(c, &cw.pluginUI.filter, opts, kit.SelectOptions{
		Label: "插件类型", Placeholder: "全部类型",
	})
}

// pluginFilter is one selectable category of the plugins page.
type pluginFilter struct {
	id    string
	label string
	// match reports whether an info belongs to the category.
	match func(plugin.Info) bool
}

// pluginFilterCategories lists the filters in display order: everything,
// then the kinds the built-in catalogs and importers fill, then the rest.
func pluginFilterCategories() []pluginFilter {
	return []pluginFilter{
		{"", "全部", func(plugin.Info) bool { return true }},
		{"catalog-providers", "模型目录", func(i plugin.Info) bool { return i.Counts.CatalogProviders > 0 }},
		{"catalog-acp", "ACP 目录", func(i plugin.Info) bool { return i.Counts.CatalogACP > 0 }},
		{"importers", "导入器", func(i plugin.Info) bool { return i.Counts.Importers > 0 }},
		{"builtin", "内置", func(i plugin.Info) bool { return i.Builtin }},
		{"installed", "已安装", func(i plugin.Info) bool { return !i.Builtin }},
	}
}

// filterPlugins narrows the list by the chosen category and the name query.
func filterPlugins(infos []plugin.Info, filterID, query string) []plugin.Info {
	var category *pluginFilter
	for i := range pluginFilterCategories() {
		cats := pluginFilterCategories()
		if cats[i].id == filterID {
			category = &cats[i]
			break
		}
	}
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]plugin.Info, 0, len(infos))
	for _, info := range infos {
		if category != nil && category.id != "" && !category.match(info) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(info.Name), q) &&
			!strings.Contains(strings.ToLower(info.Description), q) &&
			!strings.Contains(strings.ToLower(info.ID), q) {
			continue
		}
		out = append(out, info)
	}
	return out
}

// pluginList is the left column of installed plugins.
func (cw *ChatWindow) pluginList(c *mgo.Context, infos []plugin.Info) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Width(300).Shrink(0).Gap(u).Children(func() {
		list := mgo.List(c, &cw.pluginUI.list, len(infos), func(i int) {
			info := infos[i]
			b := mgo.ButtonBase(c).Key("plugin:"+info.ID).FillWidth().
				Padding(u, u*2).Gap(u).AlignItems(mgo.Center).Justify(mgo.Start).
				Radius(mtheme.ControlRadius)
			if cw.pluginUI.selected == info.ID {
				b.Background(k.Selection)
			} else if b.Hovered() {
				b.Background(k.SurfaceHover)
			}
			b.Children(func() {
				mgo.Column(c).Grow(1).MinWidth(0).Gap(u * 0.3).Children(func() {
					mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
						mgo.Text(c, info.Name).FontWeight(600).SingleLine().Grow(1).MinWidth(0).Ellipsis("…")
						if info.Builtin {
							mgo.Text(c, "内置").FontSize(small).TextColor(k.Accent).Shrink(0)
						}
					})
					state := "已禁用"
					color := k.TextMuted
					switch info.Status {
					case plugin.StatusLoaded:
						state, color = "已启用", k.Success
					case plugin.StatusError:
						state, color = "错误", k.Danger
					}
					line := state + " · v" + info.Version
					// A catalog plugin's line names what it offers, so the
					// filter's categories read on the rows too.
					if info.Counts.CatalogProviders > 0 {
						line += " · 模型目录"
					}
					if info.Counts.CatalogACP > 0 {
						line += " · ACP 目录"
					}
					if info.Counts.Importers > 0 {
						line += " · 导入器"
					}
					mgo.Text(c, line).FontSize(small).TextColor(color).SingleLine().Ellipsis("…")
				})
			})
			if b.Clicked() {
				cw.pluginUI.selected = info.ID
				cw.pluginUI.settingsFor = ""
				cw.invalidate()
			}
		})
		list.Grow(1).MinHeight(0)
	})
}

// pluginDetail is the right column for the selected plugin.
func (cw *ChatWindow) pluginDetail(c *mgo.Context, infos []plugin.Info) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	var info plugin.Info
	found := false
	for _, in := range infos {
		if in.ID == cw.pluginUI.selected {
			info, found = in, true
			break
		}
	}
	if !found && len(infos) > 0 {
		info, found = infos[0], true
		cw.pluginUI.selected = info.ID
	}
	if !found {
		return
	}
	mgo.Scroll(c).Grow(1).MinWidth(0).Children(func() {
		mgo.Column(c).Gap(u * 2).Padding(u).Children(func() {
			mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
				mgo.Column(c).Grow(1).MinWidth(0).Children(func() {
					mgo.Text(c, info.Name).FontWeight(600)
					mgo.Text(c, info.Description).FontSize(small).TextColor(k.TextMuted)
				})
				label := "启用"
				if info.Status == plugin.StatusLoaded {
					label = "禁用"
				}
				if kit.Button(c, label, kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
					cw.togglePlugin(info.ID)
				}
				if kit.Button(c, "重新加载", kit.ButtonOptions{Variant: kit.Secondary, Disabled: info.Status != plugin.StatusLoaded}).Clicked() {
					if err := cw.plugins.Reload(info.ID); err != nil {
						cw.toastErrorDetail("重新加载失败", err.Error())
					} else {
						cw.afterPluginChange()
						cw.toast("插件已重新加载", mcore.SeveritySuccess)
					}
				}
				if kit.Button(c, "卸载", kit.ButtonOptions{Variant: kit.Danger}).Clicked() {
					cw.uninstallPlugin(info)
				}
			})
			if info.Error != "" {
				mgo.Text(c, info.Error).FontSize(small).TextColor(k.Danger)
			}
			for _, w := range info.Warnings {
				mgo.Text(c, "⚠ "+w).FontSize(small).TextColor(k.Warning)
			}
			cw.pluginFactRow(c, "来源", pluginSourceText(info))
			cw.pluginFactRow(c, "贡献", pluginCountsText(info.Counts))
			cw.pluginFactRow(c, "目录", info.Dir)
			perms := info.Permissions
			if len(perms) == 0 {
				perms = []string{"store"}
			}
			cw.pluginFactRow(c, "已授权", strings.Join(perms, " · "))
			if len(info.Missing) > 0 {
				mgo.Text(c, "待授权："+strings.Join(info.Missing, " · ")).FontSize(small).TextColor(k.Warning)
			}
			if len(info.Settings) > 0 {
				mgo.Text(c, "插件设置").FontWeight(600).Padding(u*2, 0, 0, 0)
				cw.pluginSettingsForm(c, info)
			}
			cw.pluginChannelSection(c, info)
		})
	})
}

// pluginSettingsForm renders the manifest-declared settings fields.
func (cw *ChatWindow) pluginSettingsForm(c *mgo.Context, info plugin.Info) {
	values := cw.pluginSettingsFor(info)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Gap(u).MaxWidth(560).Children(func() {
		for _, f := range info.Settings {
			key := f.Key
			label := f.Label
			if label == "" {
				label = key
			}
			v := values[key]
			switch f.Type {
			case "secret":
				kit.PasswordInput(c, &v, kit.PasswordInputOptions{Label: label, Placeholder: f.Placeholder})
			case "bool":
				on := strings.EqualFold(v, "true")
				switchRow(c, &on, label, f.Placeholder)
				v = strconv.FormatBool(on)
			default:
				placeholder := f.Placeholder
				if f.Type == "select" && len(f.Options) > 0 {
					placeholder = strings.Join(f.Options, " / ")
				}
				kit.TextInput(c, &v, kit.TextInputOptions{Label: label, Placeholder: placeholder})
			}
			values[key] = v
		}
		if kit.Button(c, "保存设置", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
			cw.savePluginSettings(info)
		}
	})
}

// pluginFactRow shows one label/value line.
func (cw *ChatWindow) pluginFactRow(c *mgo.Context, label, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	k := mcore.Tokens(c)
	small := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Row(c).FillWidth().Gap(mcore.Density(c).Unit()).Children(func() {
		mgo.Text(c, label).Width(64).Shrink(0).FontSize(small).TextColor(k.TextMuted)
		mgo.Text(c, value).Grow(1).MinWidth(0).FontSize(small).MaxLines(3)
	})
}

// pluginInstallDialog is the modal install form. It stages the source and
// only closes once the install succeeds; errors stay visible in the dialog.
func (cw *ChatWindow) pluginInstallDialog(c *mgo.Context) {
	if !cw.pluginUI.installOpen {
		return
	}
	st := &cw.pluginUI
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	kinds := []string{"dir", "zip", "git", "npm"}
	idx := 0
	for i, kind := range kinds {
		if st.installKind == kind {
			idx = i
		}
	}
	kit.Dialog(c, &st.installOpen, kit.DialogOptions{
		Title:       "安装插件",
		Description: "支持目录、ZIP、Git 与 npm 四种来源。安装后默认禁用，确认权限后再启用。",
		Width:       560,
		Actions: func() {
			if kit.Button(c, "取消", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
				st.installOpen = false
			}
			if kit.Button(c, "开始安装", kit.ButtonOptions{
				Variant: kit.Primary, Disabled: st.installBusy, Loading: st.installBusy,
			}).Clicked() {
				cw.installPluginFromUI()
			}
		},
	}, func() {
		mgo.Column(c).FillWidth().Gap(u * 2).Children(func() {
			kit.SegmentedControl(c, &idx, []kit.Segment{
				{Label: "目录"}, {Label: "ZIP"}, {Label: "Git"}, {Label: "npm"},
			}, kit.SegmentedControlOptions{Label: "来源", Equal: true})
			st.installKind = kinds[idx]
			placeholder := "本地目录路径"
			switch st.installKind {
			case "zip":
				placeholder = "/path/to/plugin.zip"
			case "git":
				placeholder = "https://github.com/user/repo.git"
			case "npm":
				placeholder = "chatgo-plugin-weather@1.0.0"
			}
			kit.TextInput(c, &st.installPath, kit.TextInputOptions{
				Label: "路径 / 地址", Placeholder: placeholder,
			})
			if st.installBusy {
				mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
					mcore.Spinner(c, mcore.SpinnerOptions{Size: 14})
					mgo.Text(c, "安装中…").FontSize(small).TextColor(k.TextMuted)
				})
			}
			if st.installErr != "" {
				mgo.Text(c, st.installErr).FontSize(small).TextColor(k.Danger).MaxLines(4)
			}
			mgo.Text(c, "安装只复制文件，不会执行插件代码；启用时才编译并请求权限。").
				FontSize(small).TextColor(k.TextMuted)
		})
	})
}

func pluginSourceText(info plugin.Info) string {
	src := info.Source
	if src.Kind == "" {
		return "本地"
	}
	parts := []string{pluginKindLabel(src.Kind)}
	if src.URL != "" {
		parts = append(parts, src.URL)
	}
	if src.Path != "" {
		parts = append(parts, src.Path)
	}
	if src.Ref != "" {
		parts = append(parts, src.Ref)
	}
	return strings.Join(parts, " · ")
}

func pluginCountsText(c plugin.Counts) string {
	var parts []string
	add := func(n int, label string) {
		if n > 0 {
			parts = append(parts, strconv.Itoa(n)+" "+label)
		}
	}
	add(c.Tools, "工具")
	add(c.Providers, "模型")
	add(c.Middleware, "中间件")
	add(c.MCPServers, "MCP")
	add(c.Skills, "Skill")
	add(c.Agents, "Agent")
	add(c.Themes, "主题")
	add(c.Commands, "命令")
	add(c.Contexts, "上下文")
	add(c.Triggers, "触发器")
	add(c.Channels, "渠道")
	add(c.Events, "事件")
	return strings.Join(parts, " · ")
}

// pluginChannelSection offers one "add channel" action per channel type.
func (cw *ChatWindow) pluginChannelSection(c *mgo.Context, info plugin.Info) {
	if cw.plugins == nil {
		return
	}
	var chans []plugin.ChannelContribution
	for _, ch := range cw.plugins.Channels() {
		if ch.PluginID == info.ID {
			chans = append(chans, ch)
		}
	}
	if len(chans) == 0 {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Text(c, "消息渠道").FontWeight(600).Padding(u*2, 0, 0, 0)
	mgo.Text(c, "添加后到设置 → 渠道填写凭证；Webhook 地址在渠道卡片里显示。").
		FontSize(small).TextColor(k.TextMuted)
	for _, ch := range chans {
		ch := ch
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
			mgo.Text(c, ch.Spec.Name+"（"+ch.Spec.Type+"）").Grow(1).MinWidth(0).SingleLine()
			if kit.Button(c, "添加渠道", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("plus")}).Clicked() {
				cw.addPluginChannel(ch)
			}
		})
	}
}
