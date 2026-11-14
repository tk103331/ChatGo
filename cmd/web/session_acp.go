package main

import (
	"strings"

	ui "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// ensureSessionACP fetches the open session's own config options once per
// session, caching the answer (or the failure, so a dead CLI does not
// refetch every frame). Clearing sessACPFor makes the next frame refetch.
func (a *app) ensureSessionACP(id string) acpOptionsState {
	if a.sessACPFor == id && (a.sessACP.loading || a.sessACP.options != nil || a.sessACP.err != "") {
		return a.sessACP
	}
	if a.sessACPFor == id && a.sessACP.loading {
		return a.sessACP
	}
	a.sessACPFor = id
	a.sessACP = acpOptionsState{loading: true}
	go func() {
		var resp struct {
			Options []acpConfigOptionInfo `json:"options"`
		}
		err := apiGet(sessionPath(id)+"/acp-options", &resp)
		a.win.Update(func() {
			if a.sessACPFor != id {
				// Another session took the composer over meanwhile.
				return
			}
			cur := acpOptionsState{}
			if err != nil {
				cur.err = err.Error()
			} else {
				cur.options = resp.Options
			}
			a.sessACP = cur
		})
	}()
	return a.sessACP
}

// sessionACPControls shows the running session's config options in the
// composer's row under the same rules the desktop uses: the first three
// inline, the rest behind the settings popover; always selects in the row,
// button groups only inside the popover.
func (a *app) sessionACPControls(c *ui.Context, t *ui.Theme) {
	id := a.current().ID
	st := a.ensureSessionACP(id)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	switch {
	case st.loading:
		ui.Text(c, "正在读取配置…").FontSize(caption).TextColor(t.TextMuted)
		return
	case st.err != "" || len(st.options) == 0:
		// Keep the composer clean; the status line already carries errors.
		return
	}
	items := make([]kit.ConfigItem, len(st.options))
	for i, opt := range st.options {
		rank := 3
		if opt.Kind == "model" {
			rank = 0
		}
		items[i] = kit.ConfigItem{Name: opt.Name, Kind: webACPKindLabel(opt.Kind), Rank: rank}
	}
	scope := "sess:" + id
	kit.ConfigRow(c, items, scope, func(i int, allowButtons bool) {
		a.sessionACPControl(c, t, id, st.options[i], scope, allowButtons)
	})
}

// sessionACPControl renders one option of the open session; the choice goes
// to the runtime, and the refetched state lands on the control whatever the
// runtime answered.
func (a *app) sessionACPControl(c *ui.Context, t *ui.Theme, id string, opt acpConfigOptionInfo, scope string, allowButtons bool) {
	choices := make([]kit.SearchSelectOption[string], 0, len(opt.Options))
	if opt.Kind == "boolean" {
		choices = append(choices,
			kit.SearchSelectOption[string]{Value: "true", Label: "开"},
			kit.SearchSelectOption[string]{Value: "false", Label: "关"},
		)
	} else {
		for _, v := range opt.Options {
			choices = append(choices, kit.SearchSelectOption[string]{Value: v.Value, Label: v.Name})
		}
	}
	cur := opt.Current
	if kit.ConfigControl(c, &cur, choices, kit.ConfigControlOptions{
		Key: scope + ":" + opt.ID, Label: opt.Name, AllowButtons: allowButtons,
	}) {
		a.setSessionConfig(id, opt.ID, cur)
	}
}

// setSessionConfig sends one choice to the runtime. A refusal shows briefly
// in the status line and the options are refetched, so the control rolls
// back to the value the CLI really holds instead of keeping the refusal.
func (a *app) setSessionConfig(id, configID, value string) {
	go func() {
		var out struct {
			OK bool `json:"ok"`
		}
		err := apiPut(sessionPath(id)+"/acp-config", map[string]any{
			"id": configID, "value": value,
		}, &out)
		a.win.Update(func() {
			if err != nil {
				a.status = "配置未生效：" + err.Error()
			}
			// The next frame refetches, so the control follows the runtime
			// whether the change was accepted or refused.
			a.sessACPFor = ""
		})
	}()
}

// webACPKindLabel names an option's kind the way the desktop's popover
// captions do.
func webACPKindLabel(kind string) string {
	switch strings.TrimSpace(kind) {
	case "model":
		return "模型"
	case "boolean":
		return "开关"
	default:
		return "选择"
	}
}
