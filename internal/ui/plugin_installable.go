package ui

// The installable catalog section of the plugins page: the shipped plugins
// the release publishes as per-plugin zips. The list comes from the
// embedded index, so it renders offline; installing downloads the zip.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/plugin"
	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// pluginCatalogState is the market's index state: what it shows, where it
// came from and whether a refresh is running.
type pluginCatalogState struct {
	// entries is what the market lists right now — the embedded index, the
	// cached refresh, or the release's latest once a refresh lands.
	entries []plugin.CatalogEntry
	// source names where entries came from, for the market's caption.
	source string
	// refreshed marks that the release's latest index has been merged at
	// least once this run.
	refreshed bool
	// refreshing marks a running index refresh.
	refreshing bool
	// err holds a refresh failure, shown beside the refresh button.
	err string
	// checked marks that this run has already tried the background
	// refresh, so it happens once per launch, not once per frame.
	checked bool
}

// catalogCacheFile is where a refreshed index is cached; the next launch
// starts from it instead of waiting for the network.
func catalogCacheFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".chatgo", "plugin-catalog.json"), nil
}

// CatalogEntries returns the market's current index: the release-refreshed
// list, the cached refresh or the embedded one, in that order.
func (cw *ChatWindow) CatalogEntries() []plugin.CatalogEntry {
	st := &cw.pluginUI.catalog
	if len(st.entries) > 0 {
		return st.entries
	}
	st.entries = plugin.Catalog()
	st.source = "内置"
	return st.entries
}

// maybeRefreshCatalog runs the once-per-launch background index refresh on
// first render: pull the release's latest index, and when it differs from
// what is shown, swap it in and cache it. A failure is silent — the
// embedded or cached list stays on screen.
func (cw *ChatWindow) maybeRefreshCatalog() {
	st := &cw.pluginUI.catalog
	if st.checked || cw.plugins == nil {
		return
	}
	st.checked = true
	// Start from the cache when one exists, so the market opens with the
	// newest list the user has seen even before the network answers.
	if path, err := catalogCacheFile(); err == nil {
		if data, err := os.ReadFile(path); err == nil {
			var cached []plugin.CatalogEntry
			if json.Unmarshal(data, &cached) == nil && len(cached) > 0 {
				st.entries = cached
				st.source = "缓存"
			}
		}
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), catalogIndexTimeout)
		defer cancel()
		entries, err := plugin.FetchCatalogIndex(ctx)
		if err != nil {
			return // silent: the embedded or cached list stays
		}
		cw.postMain(func() {
			st.entries = entries
			st.source = "最新"
			st.refreshed = true
			cw.invalidate()
			if path, err := catalogCacheFile(); err == nil {
				_ = os.MkdirAll(filepath.Dir(path), 0o755)
				if data, err := json.MarshalIndent(entries, "", "  "); err == nil {
					_ = os.WriteFile(path, append(data, '\n'), 0o644)
				}
			}
		})
	}()
}

// refreshCatalog is the manual refresh button: fetch the release's index,
// show it and cache it, reporting a failure inline.
func (cw *ChatWindow) refreshCatalog() {
	st := &cw.pluginUI.catalog
	st.refreshing = true
	st.err = ""
	cw.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), catalogIndexTimeout)
		defer cancel()
		entries, err := plugin.FetchCatalogIndex(ctx)
		cw.postMain(func() {
			st.refreshing = false
			if err != nil {
				st.err = err.Error()
			} else {
				st.entries = entries
				st.source = "最新"
				st.refreshed = true
				if path, err := catalogCacheFile(); err == nil {
					_ = os.MkdirAll(filepath.Dir(path), 0o755)
					if data, err := json.MarshalIndent(entries, "", "  "); err == nil {
						_ = os.WriteFile(path, append(data, '\n'), 0o644)
					}
				}
			}
			cw.invalidate()
		})
	}()
}

// catalogIndexTimeout bounds one index fetch.
const catalogIndexTimeout = 30 * time.Second

// pluginInstallableSection lists the catalog entries not installed yet,
// with a search and a download-and-install button per row. Installed
// entries show their state; the ones from the old embedded releases show
// a re-install affordance when their directory is gone but the flag says
// they were on.
func (cw *ChatWindow) pluginInstallableSection(c *mgo.Context) {
	// The background once-per-launch refresh, and the market list.
	cw.maybeRefreshCatalog()
	entries := cw.CatalogEntries()
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
	header := "插件市场（" + itoa(len(pending)) + " 个可安装）"
	if st.showInstallable {
		header = "收起插件市场（" + itoa(len(pending)) + " 个可安装）"
	}
	mgo.Box(c).FillWidth().Height(1).Background(k.Border)
	mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
		mgo.Text(c, header).FontWeight(600).Grow(1)
		// Where the listed index came from, and a manual refresh beside it.
		if src := st.catalog.source; src != "" {
			mgo.Text(c, "索引 · "+src).FontSize(small).TextColor(k.TextMuted)
		}
		if st.catalog.refreshing {
			mcore.Spinner(c, mcore.SpinnerOptions{Size: 12})
		} else if mcore.IconAction(c, icons.Must("refresh-cw"), "更新索引").
			Tooltip("从 GitHub 拉取最新目录索引").Clicked() {
			cw.refreshCatalog()
		}
		if st.catalog.err != "" {
			mgo.Text(c, st.catalog.err).FontSize(small).TextColor(k.Danger).MaxWidth(240).MaxLines(1).Ellipsis("…")
		}
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
