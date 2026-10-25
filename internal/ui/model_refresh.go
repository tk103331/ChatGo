package ui

import (
	"context"
	"fmt"
	"sort"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/config"
	"chatgo/internal/llm"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// modelRefreshState holds one run of "refresh the model list": the provider
// it was started for, what the endpoint returned and which names are ticked.
type modelRefreshState struct {
	open bool
	// provider is the index into cfg.Providers the run belongs to, so a
	// result that arrives after the user moved on can be discarded.
	provider int
	// name is that provider's name, shown in the dialog title.
	name    string
	loading bool
	// found are the model names the endpoint returned, sorted.
	found []string
	// chosen holds the ticked names.
	chosen map[string]bool
	// configured marks the names the provider already had, so they can be
	// shown as such and kept ticked.
	configured map[string]bool
	query      string
}

// startModelRefresh asks the provider being edited which models it serves.
// The request runs off the UI thread, as a provider that is unreachable would
// otherwise freeze the window for the whole timeout.
func (cw *ChatWindow) startModelRefresh() {
	f := &cw.settings.prov
	if f.editing < 0 || f.editing >= len(cw.cfg.Providers) {
		return
	}
	// The form is the source of truth: the user may have pasted a key or a
	// base URL without saving yet, and the refresh should use it.
	p := config.Provider{
		Name: f.name, Type: f.typ, APIType: f.apiType,
		APIKey: f.apiKey, BaseURL: f.baseURL,
	}
	if !llm.SupportsListModels(p) {
		cw.toast("「"+f.typ+"」类型不支持获取模型列表", mcore.SeverityWarning)
		return
	}
	if p.NeedsAPIKey() && f.apiKey == "" {
		cw.toast("请先填写 API Key，再获取模型列表", mcore.SeverityWarning)
		return
	}

	configured := make(map[string]bool, len(f.models))
	for _, m := range f.models {
		configured[m] = true
	}
	cw.modelRef = modelRefreshState{
		open: true, provider: f.editing, name: f.name, loading: true,
		chosen: map[string]bool{}, configured: configured,
	}
	idx := f.editing
	go func() {
		models, err := llm.ListModels(context.Background(), p)
		cw.postMain(func() {
			// Discard a result whose provider is no longer the one being
			// refreshed, so a slow endpoint cannot write into another row.
			if !cw.modelRef.open || cw.modelRef.provider != idx {
				return
			}
			cw.modelRef.loading = false
			if err != nil {
				cw.modelRef.open = false
				cw.toast(err.Error(), mcore.SeverityError)
				cw.invalidate()
				return
			}
			cw.modelRef.found = models
			// Only what the provider already had is ticked: a gateway can
			// serve hundreds of models, and ticking them all would bury the
			// ones the user actually picked.
			for _, m := range models {
				if cw.modelRef.configured[m] {
					cw.modelRef.chosen[m] = true
				}
			}
			cw.invalidate()
		})
	}()
}

// modelRefreshDialog lets the user pick which of the returned models to keep.
func (cw *ChatWindow) modelRefreshDialog(c *mgo.Context) {
	if !cw.modelRef.open {
		return
	}
	st := &cw.modelRef
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	kit.Dialog(c, &st.open, kit.DialogOptions{
		Title:       "获取模型列表",
		Description: modelRefreshHint(st),
		Width:       680,
		Actions: func() {
			if kit.Button(c, "取消", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
				st.open = false
			}
			n := cw.modelRefreshChosenCount()
			label := "添加"
			if n > 0 {
				label = fmt.Sprintf("保留 %d 个模型", n)
			}
			if kit.Button(c, label, kit.ButtonOptions{
				Variant: kit.Primary, Disabled: st.loading || len(st.found) == 0,
			}).Clicked() {
				cw.applyModelRefresh()
			}
		},
	}, func() {
		mgo.Column(c).Gap(u * 2).Children(func() {
			if st.loading {
				mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
					mcore.Spinner(c, mcore.SpinnerOptions{Size: 14})
					mgo.Text(c, "正在获取模型列表…").FontSize(caption).TextColor(k.TextMuted)
				})
				return
			}
			if len(st.found) == 0 {
				mgo.Text(c, "该服务商没有返回任何模型。").FontSize(caption).TextColor(k.TextMuted)
				return
			}
			items := cw.modelRefreshItems()
			changed := selectListGrouped(c, &st.query, "搜索模型", items, nil,
				func(id string) bool { return st.chosen[id] },
				func(id string, on bool) { st.chosen[id] = on })
			if changed {
				c.Invalidate()
			}
		})
	})
}

// modelRefreshHint says what was found and reminds that nothing is removed.
func modelRefreshHint(st *modelRefreshState) string {
	if st.loading {
		return "正在向「" + st.name + "」获取可用模型。"
	}
	if len(st.found) == 0 {
		return "「" + st.name + "」没有返回模型。"
	}
	return fmt.Sprintf("「%s」提供 %d 个模型。勾选要加入的模型；已配置的模型不会被删除。", st.name, len(st.found))
}

// modelRefreshItems lists the found models, the already configured ones
// first, each tagged so the user can tell them apart.
func (cw *ChatWindow) modelRefreshItems() []selectItem {
	st := &cw.modelRef
	var have, fresh []selectItem
	for _, m := range st.found {
		it := selectItem{ID: m, Title: m}
		if st.configured[m] {
			it.Tags = []string{"已配置"}
			have = append(have, it)
			continue
		}
		fresh = append(fresh, it)
	}
	return append(have, fresh...)
}

// modelRefreshChosenCount counts the ticked models still on offer.
func (cw *ChatWindow) modelRefreshChosenCount() int {
	n := 0
	for _, m := range cw.modelRef.found {
		if cw.modelRef.chosen[m] {
			n++
		}
	}
	return n
}

// applyModelRefresh merges the ticked models into the form. The provider's
// existing models are kept whether or not the endpoint listed them: a name
// the user typed by hand may still work, and dropping it silently would
// change which model a saved conversation runs on.
func (cw *ChatWindow) applyModelRefresh() {
	st := &cw.modelRef
	f := &cw.settings.prov
	if f.editing != st.provider {
		st.open = false
		return
	}
	added := 0
	for _, m := range st.found {
		if !st.chosen[m] || containsStr(f.models, m) {
			continue
		}
		f.models = append(f.models, m)
		added++
	}
	sort.Strings(f.models)
	st.open = false
	if added == 0 {
		cw.toast("没有新增模型", mcore.SeverityInfo)
		return
	}
	if cw.persistProvider() {
		cw.toast(fmt.Sprintf("已新增 %d 个模型", added), mcore.SeveritySuccess)
	}
}
