package ui

// The installable catalog section of the plugins page: the shipped plugins
// the release publishes as per-plugin zips. The list comes from the
// embedded index, so it renders offline; installing downloads the zip.

import (
	"context"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/plugin"
	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// pluginInstallableSection lists the catalog entries not installed yet,
// with a search and a download-and-install button per row. Installed
// entries show their state; the ones from the old embedded releases show
// a re-install affordance when their directory is gone but the flag says
// they were on.
func (cw *ChatWindow) pluginInstallableSection(c *mgo.Context) {
	entries := plugin.Catalog()
	if len(entries) == 0 {
		return
	}
	st := &cw.pluginUI
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)

	// The section folds behind its header, so the long list stays out of
	// the way until the user means to browse it.
	installed := map[string]bool{}
	for _, info := range cw.plugins.List() {
		installed[info.ID] = true
	}
	var pending []plugin.CatalogEntry
	for _, e := range entries {
		if !installed[e.ID] {
			pending = append(pending, e)
		}
	}
	header := "可安装目录（" + itoa(len(pending)) + "）"
	if st.showInstallable {
		header = "收起可安装目录（" + itoa(len(pending)) + "）"
	}
	mgo.Box(c).FillWidth().Height(1).Background(k.Border)
	mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
		mgo.Text(c, header).FontWeight(600).Grow(1)
		if st.catalogErr != "" {
			mgo.Text(c, st.catalogErr).FontSize(small).TextColor(k.Danger).MaxWidth(240).MaxLines(1).Ellipsis("…")
		}
		if mcore.IconAction(c, icons.Must(st.chevron()), "展开").Tooltip(header).Clicked() {
			st.showInstallable = !st.showInstallable
			st.catalogErr = ""
		}
	})
	if !st.showInstallable {
		return
	}
	in := kit.SearchInput(c, &st.installQuery, kit.SearchInputOptions{Placeholder: "搜索可安装插件", Label: "搜索可安装插件"})
	in.Element.FillWidth()
	q := strings.ToLower(strings.TrimSpace(st.installQuery))
	var shown []plugin.CatalogEntry
	for _, e := range pending {
		if q != "" && !strings.Contains(strings.ToLower(e.Name), q) &&
			!strings.Contains(strings.ToLower(e.Description), q) &&
			!strings.Contains(strings.ToLower(e.ID), q) {
			continue
		}
		shown = append(shown, e)
	}
	if len(shown) == 0 {
		mgo.Text(c, "没有匹配的可安装插件").FontSize(small).TextColor(k.TextMuted)
		return
	}
	mgo.Scroll(c).MaxHeight(300).Children(func() {
		mgo.Column(c).Gap(u).Children(func() {
			for _, e := range shown {
				e := e
				mgo.Row(c).Key(e.ID).FillWidth().Gap(u*2).AlignItems(mgo.Center).
					Padding(u, u*2).Radius(mtheme.CardRadius).Border(mtheme.BorderWidth, k.Border).
					Children(func() {
						mgo.Column(c).Grow(1).MinWidth(0).Gap(u / 2).Children(func() {
							mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
								mgo.Text(c, e.Name).FontWeight(600).SingleLine()
								mgo.Text(c, catalogKindLabel(e.Kind)).FontSize(small).TextColor(k.Accent)
							})
							if e.Description != "" {
								mgo.Text(c, e.Description).FontSize(small).TextColor(k.TextMuted).MaxLines(1)
							}
						})
						if st.installing == e.ID {
							mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
								mcore.Spinner(c, mcore.SpinnerOptions{Size: 12})
								mgo.Text(c, "下载中…").FontSize(small).TextColor(k.TextMuted)
							})
						} else if kit.Button(c, "安装", kit.ButtonOptions{
							Variant: kit.Secondary, Disabled: st.installing != "",
						}).Clicked() {
							cw.installCatalogEntry(e)
						}
					})
			}
		})
	})
}

// chevron picks the disclosure arrow's direction.
func (st *pluginUIState) chevron() string {
	if st.showInstallable {
		return "chevron-down"
	}
	return "chevron-right"
}

// installCatalogEntry downloads and installs one catalog entry off the UI
// thread, enabled right away — a catalog plugin needs no permissions
// beyond the store default and the point of installing it is to use it.
func (cw *ChatWindow) installCatalogEntry(e plugin.CatalogEntry) {
	st := &cw.pluginUI
	st.installing = e.ID
	st.catalogErr = ""
	cw.invalidate()
	go func() {
		_, err := cw.plugins.InstallCatalog(context.Background(), e, true)
		cw.postMain(func() {
			if st.installing != e.ID {
				return
			}
			st.installing = ""
			if err != nil {
				st.catalogErr = e.Name + "：" + acpFirstLine(err.Error())
			} else {
				cw.toast("已安装并启用 "+e.Name, mcore.SeveritySuccess)
			}
			cw.invalidate()
		})
	}()
}

// catalogKindLabel names a catalog entry's category in Chinese.
func catalogKindLabel(kind string) string {
	switch kind {
	case "catalog-providers":
		return "模型目录"
	case "catalog-acp":
		return "ACP 目录"
	case "importers":
		return "导入器"
	default:
		return ""
	}
}
