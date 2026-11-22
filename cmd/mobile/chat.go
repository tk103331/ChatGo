package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/mobile"

	"chatgo/internal/mobileapi"
	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	"chatgo/pkg/models"
)

func (a *app) chatList(c *ui.Context) {
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		if a.offline() {
			a.offlineState(c)
			return
		}
		if len(a.sessions) == 0 {
			ui.Column(c).Grow(1).MinHeight(0).Justify(ui.Center).Children(func() {
				emptyState(c, "message-square", "还没有会话", "在 ChatGo 里问点什么，或选个项目开始。", func() {
					if primaryButton(c, "新建会话").Clicked() {
						a.newChat()
					}
				})
			})
			return
		}
		a.sessionSearch(c)
		visible := a.visibleSessions()
		if len(visible) == 0 {
			ui.Column(c).Grow(1).MinHeight(0).Justify(ui.Center).Children(func() {
				emptyState(c, "search", "没有匹配的会话", "换个关键词，或清除搜索。", func() {
					if primaryButton(c, "清除搜索").Clicked() {
						a.listQuery = ""
					}
				})
			})
			return
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			// The bottom padding keeps the last row clear of the button
			// floating over the list.
			ui.Column(c).FillWidth().Padding(4, 0, 88, 0).Gap(6).Children(func() {
				for _, group := range sessionGroups(visible) {
					group := group
					mobile.Section(c, group.label, func() {
						for _, session := range group.sessions {
							a.sessionRow(c, session)
						}
					})
				}
			})
		})
		a.newSessionButton(c)
	})
}

// sessionSearch filters the list by title, preview or agent name.
func (a *app) sessionSearch(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().Padding(4, 12).Children(func() {
		ui.Row(c).FillWidth().Height(38).Radius(12).Background(t.Surface).
			Padding(0, 10).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.Must("search")).Size(15, 15).TextColor(t.TextMuted).Shrink(0)
			in := ui.TextInput(c, &a.listQuery).Grow(1).MinHeight(36).
				BorderWidth(0, 0, 0, 0).Background(ui.Transparent).
				Placeholder("搜索会话")
			if in.Changed() {
				c.Invalidate()
			}
			if a.listQuery != "" {
				if smallIconButton(c, "x", "清除搜索").Clicked() {
					a.listQuery = ""
				}
			}
		})
	})
}

// visibleSessions is the list narrowed by the search field.
func (a *app) visibleSessions() []mobileapi.SessionSummary {
	q := strings.ToLower(strings.TrimSpace(a.listQuery))
	if q == "" {
		return a.sessions
	}
	out := make([]mobileapi.SessionSummary, 0, len(a.sessions))
	for _, s := range a.sessions {
		hay := strings.ToLower(sessionTitle(s) + "\n" + sessionPreview(s) + "\n" + a.sessionAgentLabel(s.Agent))
		if strings.Contains(hay, q) {
			out = append(out, s)
		}
	}
	return out
}

// newSessionButton floats over the list: the one thing to do from here is
// start a conversation.
func (a *app) newSessionButton(c *ui.Context) {
	fabButton(c, "plus", "新建会话", a.newChat)
}

// sessionGroup is one day's sessions under a heading, newest first.
type sessionGroup struct {
	label    string
	sessions []mobileapi.SessionSummary
}

func sessionGroups(sessions []mobileapi.SessionSummary) []sessionGroup {
	var groups []sessionGroup
	for _, s := range sessions {
		label := sessionDay(s.UpdatedAt)
		if len(groups) == 0 || groups[len(groups)-1].label != label {
			groups = append(groups, sessionGroup{label: label})
		}
		groups[len(groups)-1].sessions = append(groups[len(groups)-1].sessions, s)
	}
	return groups
}

// sessionDay names the heading a session belongs under.
func sessionDay(t time.Time) string {
	now := time.Now()
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location())
	switch {
	case day.Equal(today):
		return "今天"
	case day.Equal(today.AddDate(0, 0, -1)):
		return "昨天"
	case day.After(today.AddDate(0, 0, -7)):
		return "本周"
	default:
		return "更早"
	}
}

// sessionRow is one conversation: a compact title and a two-line preview,
// with its time and agent at the right.
func (a *app) sessionRow(c *ui.Context, s mobileapi.SessionSummary) {
	t := c.Theme()
	title := sessionTitle(s)
	preview := sessionPreview(s)
	agent := a.sessionAgentLabel(s.Agent)
	row := ui.Row(c).FillWidth().AlignItems(ui.Center).Padding(12, 16).Gap(12).
		Role(ui.RoleButton).Label(title + ", " + agent + ", " + preview)
	mobile.Ink(row)
	if mobile.Current() == mobile.Cupertino && row.Pressed() {
		row.Background(t.SurfacePressed)
	}
	row.Children(func() {
		a.agentMarkBox(c, s.Agent, s.Running)
		ui.Column(c).Grow(1).Basis(0).Gap(3).Children(func() {
			ui.Text(c, title).FontSize(15).FontWeight(600).SingleLine().Ellipsis("…")
			ui.Text(c, preview).FontSize(13).TextColor(t.TextMuted).MaxLines(2).Ellipsis("…")
		})
		ui.Column(c).AlignItems(ui.End).Shrink(0).Gap(4).Children(func() {
			if !s.UpdatedAt.IsZero() {
				ui.Text(c, shortTime(s.UpdatedAt)).FontSize(11).TextColor(t.TextMuted)
			}
			if s.Running {
				ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(6, 6).Radius(3).Background(t.Accent).Shrink(0)
					ui.Text(c, "生成中").FontSize(10).TextColor(t.Accent)
				})
			} else if agent != "" {
				ui.Text(c, agent).FontSize(10).TextColor(t.Accent).SingleLine().Ellipsis("…").MaxWidth(84)
			}
		})
	})
	if row.Clicked() {
		a.openChat(s.ID, "", "")
	}
}

// sessionPreview is the last message as plain, single-spaced text.
func sessionPreview(s mobileapi.SessionSummary) string {
	if text := plainPreview(s.Preview); text != "" {
		return text
	}
	return "暂无消息"
}

// sessionTitle drops the quote or heading marks a generated title may carry.
func sessionTitle(s mobileapi.SessionSummary) string {
	title := strings.TrimLeft(strings.TrimSpace(s.Title), "># ")
	title = strings.TrimSpace(strings.Join(strings.Fields(title), " "))
	if title == "" || looksLikeJSON(title) {
		return "未命名"
	}
	return title
}

// looksLikeJSON spots a title that is really a raw protocol line, which the
// list should not show as a name.
func looksLikeJSON(s string) bool {
	if len(s) < 2 || (s[0] != '{' && s[0] != '[') {
		return false
	}
	return strings.Contains(s, "\":") || strings.Contains(s, "\"type\"")
}

var (
	previewImage = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	previewLink  = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	previewLine  = regexp.MustCompile(`(?m)^\s{0,3}(?:#{1,6}\s+|>\s?|[-*+]\s+|\d+[.)]\s+)`)
)

// plainPreview drops the markdown a preview should not show, keeping the
// words: emphasis, code ticks, link targets and line prefixes.
func plainPreview(s string) string {
	s = previewImage.ReplaceAllString(s, "$1")
	s = previewLink.ReplaceAllString(s, "$1")
	s = previewLine.ReplaceAllString(s, "")
	s = strings.NewReplacer("```", " ", "`", "", "**", "", "__", "").Replace(s)
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

func (a *app) sessionAgentLabel(ref string) string {
	if ref == "" {
		return "Chat"
	}
	for _, agent := range a.agents {
		if agent.Ref == ref {
			return firstNonEmpty(agent.Name, ref)
		}
	}
	ref = strings.TrimPrefix(ref, "agent:")
	ref = strings.TrimPrefix(ref, "acp:")
	ref = strings.TrimPrefix(ref, "a2a:")
	return ref
}

// agentRefForName is the configured agent behind an author's byline, so a
// message can carry the same mark the lists give it.
func (a *app) agentRefForName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	for _, agent := range a.agents {
		label := firstNonEmpty(agent.Name, agent.Ref)
		if strings.EqualFold(label, name) {
			return agent.Ref
		}
	}
	return ""
}

// sessionIcon marks what kind of agent runs the session.
func sessionIcon(ref string) string {
	switch {
	case strings.HasPrefix(ref, "acp:"):
		return "terminal"
	case strings.HasPrefix(ref, "a2a:"):
		return "globe"
	default:
		return "message-square"
	}
}

// shortTime keeps the row's right edge short: the time today, the date
// otherwise.
func shortTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	now := time.Now()
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return t.Format("15:04")
	}
	if t.Year() == now.Year() {
		return t.Format("01-02")
	}
	return t.Format("2006-01-02")
}

func (a *app) newChat() {
	if a.client == nil {
		a.pairOpen = true
		return
	}
	a.update(func() {
		a.screen = screenNew
		a.newDraft = ""
		a.newFiles = nil
		a.newACP = acpModels{}
		if a.newModel == "" && len(a.meta.Models) > 0 {
			m := a.meta.Models[0]
			a.newModel = m.Provider + "::" + m.Model
		}
	})
}

// offlineState is a tab's page when there is no usable desktop link: one
// calm explanation with the way back, not a raw transport error.
func (a *app) offlineState(c *ui.Context) {
	title, hint, icon := "桌面未连接", "在设置中切换桌面，或重新连接。", "wifi-off"
	if a.pairRequired {
		// The desktop dropped this phone: the only way forward is a new
		// pairing, so say so instead of offering a retry that cannot work.
		title, hint, icon = "需要重新配对", "桌面已移除这台手机，或令牌已失效。请重新扫码配对。", "key-round"
	}
	if a.pairing {
		title, hint, icon = "正在配对", "正在连接桌面，请稍候…", "key-round"
	}
	if a.pairErr != "" {
		title, hint, icon = "配对失败", a.pairErr, "key-round"
	}
	ui.Column(c).Grow(1).MinHeight(0).Justify(ui.Center).Children(func() {
		emptyState(c, icon, title, hint, func() {
			ui.Row(c).Gap(8).Wrap().Justify(ui.Center).Children(func() {
				if a.pairing {
					return
				}
				if a.pairRequired || a.pairErr != "" {
					if primaryButton(c, "重新配对").Clicked() {
						a.openPairSheet(true)
					}
				} else if len(a.servers) > 0 {
					if primaryButton(c, "重新连接").Clicked() {
						a.connect(a.currentServer())
					}
				} else if primaryButton(c, "添加桌面").Clicked() {
					a.openPairSheet(false)
				}
				if ghostButton(c, "打开设置").Clicked() {
					a.tab = tabSettings
					a.screen = screenSettings
				}
			})
		})
	})
}

// newSessionPage is the chat tab's landing page: a new session, with the
// same composer as the conversation page. The old empty conversation list is
// still reachable from the history button.
func (a *app) newSessionPage(c *ui.Context) {
	if a.offline() {
		a.offlineState(c)
		return
	}
	// The choices stack from the bottom edge, as the reference layout does:
	// a greeting, project, agent, then the composer. The space above stays
	// open for the conversation to come. With the keyboard up the greeting
	// and cards give their room to the composer; Hidden keeps them built, so
	// the field keeps focus.
	_, _, bottom, _ := c.SafeArea()
	keyboard := bottom > 120
	t := c.Theme()
	ui.Column(c).Grow(1).MinHeight(0).Justify(ui.End).Children(func() {
		ui.Column(c).FillWidth().MaxWidth(680).AlignSelf(ui.Center).
			Padding(16, 14).Gap(10).Children(func() {
			ui.Column(c).FillWidth().Gap(3).Hidden(keyboard).Children(func() {
				ui.Text(c, "在 ChatGo 里做点什么？").FontSize(22).FontWeight(650)
				ui.Text(c, "或直接开始，不选项目").FontSize(12).TextColor(t.TextMuted)
			})
			ui.Column(c).FillWidth().Gap(10).Hidden(keyboard).Children(func() {
				a.workspaceCard(c)
				a.agentCard(c)
			})
			a.newSessionComposer(c)
		})
	})
}
func (a *app) workspaceCard(c *ui.Context) {
	t := c.Theme()
	label := "不选项目"
	if a.newWorkspace != "" {
		label = filepath.Base(a.newWorkspace)
	}
	card := optionCard(c, "项目", label, func() {
		ui.Icon(c, icons.Must("folder")).Size(18, 18).TextColor(t.TextMuted).Shrink(0)
	})
	if card.Clicked() {
		a.workspaceOpen = true
	}
}

// agentCard is the Agent picker card of the new-session stack. Its list is a
// sheet: a popover here would be clipped by the scroll and cover the composer.
func (a *app) agentCard(c *ui.Context) {
	card := optionCard(c, "Agent", a.agentLabel(a.newAgent), func() {
		a.agentMark(c, a.newAgent, 18)
	})
	if card.Clicked() {
		a.agentOpen = true
	}
}

// agentChoices lists the agents the sheet offers, with the built-in chat
// first unless the desktop already lists it.
func (a *app) agentChoices() []mobileapi.Agent {
	choices := append([]mobileapi.Agent(nil), a.agents...)
	for _, item := range choices {
		if item.Ref == "" {
			return choices
		}
	}
	return append([]mobileapi.Agent{{Ref: "", Name: "Chat", Kind: "chat"}}, choices...)
}

func (a *app) agentSheet(c *ui.Context) {
	detent := float32(0.45)
	if len(a.agents) > 3 {
		detent = 0.7
	}
	mobile.Sheet(c, &a.agentOpen, mobile.SheetOptions{Detents: []float32{detent}, Label: "选择 Agent"}, func() {
		t := c.Theme()
		ui.Column(c).Grow(1).MinHeight(0).FillWidth().Padding(16).Gap(8).Children(func() {
			ui.Text(c, "选择 Agent").FontSize(18).FontWeight(600)
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				for _, item := range a.agentChoices() {
					if item.Disabled {
						continue
					}
					item := item
					row := mobile.ListRow(c, mobile.RowOptions{
						Title:    a.agentLabel(item.Ref),
						Subtitle: agentSubtitle(item),
						Leading: func() {
							a.agentMark(c, item.Ref, 20)
						},
						Trailing: func() {
							if item.Ref == a.newAgent {
								ui.Icon(c, icons.Must("check")).Size(16, 16).TextColor(t.Accent)
							}
						},
					})
					if row.Clicked() {
						if a.newAgent != item.Ref {
							// Another runtime's models are not this one's.
							a.newACP = acpModels{}
						}
						a.newAgent = item.Ref
						a.agentOpen = false
					}
				}
			})
			if ghostButton(c, "关闭").Clicked() {
				a.agentOpen = false
			}
		})
	})
}

// modelForNew and modelForChat say which composer the model sheet chooses
// for, so one sheet serves both.
const (
	modelForNew  = "new"
	modelForChat = "chat"
)

// acpModels is one composer's ACP model list, read from the runtime's warmed
// config options. The zero value is an agent whose list has not been asked
// for.
type acpModels struct {
	agent    string
	optionID string
	current  string
	values   []mobileapi.ACPValue
	loading  bool
	err      string
}

// agentKind is what an agent reference runs: chat, agent, acp or a2a.
func (a *app) agentKind(ref string) string {
	if ref == "" {
		return "chat"
	}
	for _, item := range a.agents {
		if item.Ref == ref {
			return firstNonEmpty(item.Kind, "agent")
		}
	}
	switch {
	case strings.HasPrefix(ref, "acp:"):
		return "acp"
	case strings.HasPrefix(ref, "a2a:"):
		return "a2a"
	case strings.HasPrefix(ref, "agent:"):
		return "agent"
	}
	return ""
}

// targetAgent is the agent a composer's model picker is choosing for.
func (a *app) targetAgent(target string) string {
	if target == modelForChat {
		if a.conv != nil {
			return a.conv.Agent
		}
		return ""
	}
	return a.newAgent
}

// acpState is the ACP model list the given composer's picker uses.
func (a *app) acpState(target string) *acpModels {
	if target == modelForChat {
		return &a.chatACP
	}
	return &a.newACP
}

// acpModelLabel names the model an ACP composer shows, from the list its
// runtime gave.
func (a *app) acpModelLabel(st *acpModels) string {
	if st.current != "" {
		for _, v := range st.values {
			if v.Value == st.current {
				return v.Name
			}
		}
		return st.current
	}
	if st.loading {
		return "读取模型…"
	}
	return "选择模型"
}

// ensureACPModels reads an ACP agent's model list from its warmed runtime
// options, once per agent. A conversation asks its own live session, which
// warms it in if it has not started.
func (a *app) ensureACPModels(target string) {
	agent := a.targetAgent(target)
	if !strings.HasPrefix(agent, "acp:") || a.client == nil {
		return
	}
	st := a.acpState(target)
	if st.agent == agent && (st.loading || len(st.values) > 0 || st.err != "") {
		return
	}
	current := ""
	if target == modelForChat && a.conv != nil && a.conv.ACPConfig != nil {
		current = a.conv.ACPConfig["model"]
	}
	*st = acpModels{agent: agent, current: current, loading: true}
	client := a.client
	convID := a.openConv
	a.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		var opts []mobileapi.ACPOption
		var err error
		if target == modelForChat && convID != "" {
			opts, err = client.SessionACPOptions(ctx, convID)
		} else {
			opts, err = client.ACPOptions(ctx, agent)
		}
		a.update(func() {
			if a.client != client || a.targetAgent(target) != agent {
				return
			}
			st.loading = false
			if err != nil {
				st.err = err.Error()
				return
			}
			for _, opt := range opts {
				if opt.Kind != "model" {
					continue
				}
				st.optionID, st.values, st.current = opt.ID, opt.Options, opt.Current
				break
			}
			if len(st.values) == 0 {
				st.err = "该 ACP Agent 没有提供模型选项"
			}
		})
	}()
}

// modelMatches reports whether a model row matches the picker's filter.
func modelMatches(filter string, parts ...string) bool {
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" {
		return true
	}
	for _, part := range parts {
		if strings.Contains(strings.ToLower(part), filter) {
			return true
		}
	}
	return false
}

// modelChip is the model picker in a composer's footer. A plain chat or a
// custom agent picks from the desktop's models; an ACP agent picks from the
// model its runtime offers; an A2A agent brings its own, so its chip only
// says where the model runs.
func (a *app) modelChip(c *ui.Context, ref *string, target string) {
	agent := a.targetAgent(target)
	kind := a.agentKind(agent)
	if kind == "a2a" {
		t := c.Theme()
		ui.Row(c).Grow(1).Shrink(1).MinWidth(0).Height(34).Padding(0, 6).
			Gap(6).AlignItems(ui.Center).Label("远程 Agent 模型").
			Children(func() {
				ui.Icon(c, icons.Must("globe")).Size(14, 14).TextColor(t.TextMuted).Shrink(0)
				ui.Text(c, "远程 Agent 模型").FontSize(13).TextColor(t.TextMuted).
					Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			})
		return
	}
	if kind == "acp" {
		st := a.acpState(target)
		a.ensureACPModels(target)
		t := c.Theme()
		chip := ui.ButtonBase(c).Grow(1).Shrink(1).MinWidth(0).Height(34).Padding(0, 6).
			Radius(12).Gap(6).AlignItems(ui.Center).Role(ui.RoleButton).
			Label("模型 " + a.acpModelLabel(st))
		if chip.Pressed() {
			chip.Background(t.SurfaceHover)
		}
		chip.Children(func() {
			ui.Icon(c, icons.Must("brain")).Size(14, 14).TextColor(t.TextMuted).Shrink(0)
			ui.Text(c, a.acpModelLabel(st)).FontSize(13).Grow(1).MinWidth(0).
				SingleLine().Ellipsis("…")
			ui.Icon(c, icons.Must("chevron-down")).Size(12, 12).TextColor(t.TextMuted).Shrink(0)
		})
		if chip.Clicked() {
			a.modelFilter = ""
			a.modelFor = target
			a.modelOpen = true
			a.ensureACPModels(target)
		}
		return
	}
	if len(a.meta.Models) == 0 {
		return
	}
	t := c.Theme()
	chip := ui.ButtonBase(c).Grow(1).Shrink(1).MinWidth(0).Height(34).Padding(0, 6).
		Radius(12).Gap(6).AlignItems(ui.Center).Role(ui.RoleButton).
		Label("模型 " + a.modelLabel(*ref))
	if chip.Pressed() {
		chip.Background(t.SurfaceHover)
	}
	chip.Children(func() {
		ui.Icon(c, icons.Must("brain")).Size(14, 14).TextColor(t.TextMuted).Shrink(0)
		ui.Text(c, a.modelLabel(*ref)).FontSize(13).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
		ui.Icon(c, icons.Must("chevron-down")).Size(12, 12).TextColor(t.TextMuted).Shrink(0)
	})
	if chip.Clicked() {
		a.modelFilter = ""
		a.modelFor = target
		a.modelOpen = true
	}
}

// filteredModelChoices is the desktop model list narrowed by the picker's
// filter.
func (a *app) filteredModelChoices() []mobileapi.ModelChoice {
	out := make([]mobileapi.ModelChoice, 0, len(a.meta.Models))
	for _, m := range a.meta.Models {
		if modelMatches(a.modelFilter, m.Model, m.Provider) {
			out = append(out, m)
		}
	}
	return out
}

// filteredACPValues is an ACP model list narrowed by the picker's filter.
func (a *app) filteredACPValues(st *acpModels) []mobileapi.ACPValue {
	out := make([]mobileapi.ACPValue, 0, len(st.values))
	for _, v := range st.values {
		if modelMatches(a.modelFilter, v.Name, v.Value) {
			out = append(out, v)
		}
	}
	return out
}

// chooseACPModel records an ACP model, applying it to a live conversation
// before its next turn.
func (a *app) chooseACPModel(target, value string) {
	st := a.acpState(target)
	old := st.current
	st.current = value
	a.modelOpen = false
	a.invalidate()
	if target != modelForChat || a.client == nil || a.openConv == "" || st.optionID == "" {
		return
	}
	client, convID, optionID := a.client, a.openConv, st.optionID
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		err := client.SetSessionConfig(ctx, convID, optionID, value)
		a.update(func() {
			if a.client != client || a.openConv != convID {
				return
			}
			if err != nil {
				st.current = old
				a.status = fmt.Sprintf("切换模型失败: %v", err)
				return
			}
			if a.conv != nil {
				if a.conv.ACPConfig == nil {
					a.conv.ACPConfig = map[string]string{}
				}
				a.conv.ACPConfig[optionID] = value
			}
		})
	}()
}

func (a *app) modelSheet(c *ui.Context) {
	target := a.modelFor
	agent := a.targetAgent(target)
	kind := a.agentKind(agent)
	if kind == "acp" {
		a.ensureACPModels(target)
	}
	ref := &a.newModel
	if target == modelForChat {
		ref = &a.chatModel
	}
	st := a.acpState(target)
	count := len(a.meta.Models)
	if kind == "acp" {
		count = len(st.values)
	}
	detent := float32(0.45)
	if count > 3 || a.modelFilter != "" {
		detent = 0.7
	}
	mobile.Sheet(c, &a.modelOpen, mobile.SheetOptions{Detents: []float32{detent}, Label: "选择模型"}, func() {
		t := c.Theme()
		title := "选择模型"
		if kind == "a2a" {
			title = "模型"
		}
		ui.Column(c).Grow(1).MinHeight(0).FillWidth().Padding(16).Gap(8).Children(func() {
			ui.Text(c, title).FontSize(18).FontWeight(600)
			switch kind {
			case "a2a":
				ui.Text(c, "远程 A2A Agent 自带模型，模型由对端决定，手机端不需要选择。").
					FontSize(13).TextColor(t.TextMuted).MaxLines(3)
			case "acp":
				if st.loading {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "正在从 "+a.agentLabel(agent)+" 预热模型…").
							FontSize(13).TextColor(t.TextMuted).Grow(1)
					})
				} else if st.err != "" {
					ui.Text(c, st.err).FontSize(13).TextColor(t.Danger).MaxLines(3)
					if secondaryButton(c, "重试").Clicked() {
						*st = acpModels{}
						a.ensureACPModels(target)
					}
				} else {
					filter := ui.TextInput(c, &a.modelFilter).Placeholder("筛选模型")
					if filter.Changed() {
						c.Invalidate()
					}
					ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
						values := a.filteredACPValues(st)
						if len(values) == 0 {
							ui.Text(c, "没有匹配的模型").FontSize(13).TextColor(t.TextMuted).Padding(8, 0)
						}
						for _, v := range values {
							v := v
							row := mobile.ListRow(c, mobile.RowOptions{
								Title:    v.Name,
								Subtitle: "ACP · " + a.agentLabel(agent),
								Leading: func() {
									ui.Icon(c, icons.Must("brain")).Size(20, 20).TextColor(t.TextMuted)
								},
								Trailing: func() {
									if v.Value == st.current {
										ui.Icon(c, icons.Must("check")).Size(16, 16).TextColor(t.Accent)
									}
								},
							})
							if row.Clicked() {
								a.chooseACPModel(target, v.Value)
							}
						}
					})
				}
			default:
				filter := ui.TextInput(c, &a.modelFilter).Placeholder("筛选模型")
				if filter.Changed() {
					c.Invalidate()
				}
				ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
					models := a.filteredModelChoices()
					if len(models) == 0 {
						ui.Text(c, "没有匹配的模型").FontSize(13).TextColor(t.TextMuted).Padding(8, 0)
					}
					for _, m := range models {
						choice := m.Provider + "::" + m.Model
						row := mobile.ListRow(c, mobile.RowOptions{
							Title:    m.Model,
							Subtitle: m.Provider,
							Leading: func() {
								ui.Icon(c, icons.Must("brain")).Size(20, 20).TextColor(t.TextMuted)
							},
							Trailing: func() {
								if choice == *ref {
									ui.Icon(c, icons.Must("check")).Size(16, 16).TextColor(t.Accent)
								}
							},
						})
						if row.Clicked() {
							*ref = choice
							a.modelOpen = false
						}
					}
				})
			}
			if ghostButton(c, "关闭").Clicked() {
				a.modelOpen = false
			}
		})
	})
}

func (a *app) newSessionComposer(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(5).Padding(5, 10).Radius(16).
		Background(t.Surface).Children(func() {
		if len(a.newFiles) > 0 {
			ui.Row(c).FillWidth().Gap(6).Wrap().Padding(2, 0).Children(func() {
				for i := range a.newFiles {
					file := a.newFiles[i]
					ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(4, 8).
						Radius(10).Background(t.Background).Children(func() {
						ui.Text(c, file.Name).FontSize(11).SingleLine().Ellipsis("…").MaxWidth(160)
						if kit.Button(c, "", kit.ButtonOptions{
							Variant: kit.Ghost, Icon: icons.Must("x"), Label: "移除附件",
						}).Height(26).Padding(0, 4).Clicked() {
							a.newFiles = append(a.newFiles[:i], a.newFiles[i+1:]...)
						}
					})
				}
			})
		}
		// The field is the card: no border or fill of its own.
		composerField(c, &a.newDraft)
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			attach := kit.Button(c, "", kit.ButtonOptions{
				Variant: kit.Ghost, Icon: icons.Must("plus"), Label: "添加附件",
				Foreground: t.TextMuted,
			}).Size(36, 36).Padding(0)
			if attach.Clicked() {
				a.pickNewAttachments()
			}
			a.modelChip(c, &a.newModel, modelForNew)
			if len(a.meta.Models) == 0 {
				ui.Spacer(c)
			}
			send := kit.IconButton(c, icons.Must("arrow-up"), "创建会话", kit.IconButtonOptions{
				Variant: kit.Primary, Size: 36, Disabled: a.creating || a.connecting || a.connErr != "" || strings.TrimSpace(a.newDraft) == "",
			})
			// Only the button sends: Enter is a newline in the field.
			if send.Clicked() {
				a.createNewChat()
			}
		})
	})
}

func (a *app) workspaceSheet(c *ui.Context) {
	mobile.Sheet(c, &a.workspaceOpen, mobile.SheetOptions{Detents: []float32{0.75}, Label: "选择项目"}, func() {
		t := c.Theme()
		ui.Column(c).Grow(1).MinHeight(0).FillWidth().Padding(16).Gap(8).Children(func() {
			ui.Text(c, "项目").FontSize(18).FontWeight(600)
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				row := mobile.ListRow(c, mobile.RowOptions{
					Title: "不选项目", Subtitle: "在当前目录外开始",
					Trailing: func() {
						if a.newWorkspace == "" {
							ui.Text(c, "当前").FontSize(11).TextColor(t.Accent)
						}
					},
				})
				if row.Clicked() {
					a.newWorkspace = ""
					a.workspaceOpen = false
				}
				for _, ws := range a.workspaces {
					ws := ws
					row := mobile.ListRow(c, mobile.RowOptions{
						Title: filepath.Base(ws), Subtitle: ws,
						Trailing: func() {
							if a.newWorkspace == ws {
								ui.Text(c, "当前").FontSize(11).TextColor(t.Accent)
							}
						},
					})
					if row.Clicked() {
						a.newWorkspace = ws
						a.workspaceOpen = false
					}
				}
			})
		})
	})
}

func (a *app) pickNewAttachments() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title: "添加附件", Multiple: true,
			Filters: []mygo.FileFilter{{Name: "文件", Extensions: []string{"*"}}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		for _, path := range paths {
			data, err := os.ReadFile(path)
			if err != nil || len(data) > 8<<20 {
				if err != nil {
					a.errorf("读取附件失败: %v", err)
				} else {
					a.errorf("附件超过 8 MB: %s", filepath.Base(path))
				}
				continue
			}
			name := filepath.Base(path)
			a.update(func() { a.newFiles = append(a.newFiles, newFile{Name: name, Data: data}) })
		}
	}()
}

func (a *app) agentLabel(ref string) string {
	if ref == "" {
		return "Chat"
	}
	for _, agent := range a.agents {
		if agent.Ref == ref {
			return firstNonEmpty(agent.Name, ref)
		}
	}
	return ref
}

func agentSubtitle(item mobileapi.Agent) string {
	if item.Description != "" {
		return item.Description
	}
	switch item.Kind {
	case "", "chat":
		return "内置助手，可用全部模型与工具"
	case "acp":
		return "ACP 编程 Agent"
	case "a2a":
		return "远程 A2A Agent"
	}
	return item.Kind
}

func (a *app) modelLabel(ref string) string {
	for _, m := range a.meta.Models {
		if m.Provider+"::"+m.Model == ref {
			return m.Model
		}
	}
	return ref
}

// splitModelRef splits a "provider::model" choice.
func splitModelRef(ref string) (provider, model string) {
	if parts := strings.SplitN(ref, "::", 2); len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", ""
}

// syncChatModel points the open conversation's composer at the model it last
// used, falling back to the new-session choice and then the first offered one.
func (a *app) syncChatModel(conv *models.Conversation) {
	if conv != nil && (strings.HasPrefix(conv.Agent, "acp:") || strings.HasPrefix(conv.Agent, "a2a:")) {
		// Those runtimes carry their own model; the desktop's list means
		// nothing to them.
		a.chatModel = ""
		return
	}
	if conv != nil && conv.Provider != "" && conv.Model != "" {
		ref := conv.Provider + "::" + conv.Model
		for _, m := range a.meta.Models {
			if m.Provider+"::"+m.Model == ref {
				a.chatModel = ref
				return
			}
		}
	}
	if a.newModel != "" {
		a.chatModel = a.newModel
		return
	}
	if len(a.meta.Models) > 0 {
		a.chatModel = a.meta.Models[0].Provider + "::" + a.meta.Models[0].Model
	}
}

type newFile struct {
	Name string
	Data []byte
}

func (a *app) createNewChat() {
	if a.client == nil || a.connecting || a.creating || a.connErr != "" {
		return
	}
	client := a.client
	text := strings.TrimSpace(a.newDraft)
	if text == "" {
		a.toast("请先输入内容")
		return
	}
	files := append([]newFile(nil), a.newFiles...)
	workspace := a.newWorkspace
	agent := a.newAgent
	// ACP and A2A carry their own runtime; only a plain chat or a custom
	// agent picks one of the desktop's models.
	provider, model := "", ""
	var acpConfig map[string]string
	switch a.agentKind(agent) {
	case "acp":
		if a.newACP.current != "" {
			acpConfig = map[string]string{"model": a.newACP.current}
		}
	case "a2a":
	default:
		provider, model = splitModelRef(a.newModel)
	}
	a.creating = true
	a.status = "正在创建会话…"
	go func() {
		defer a.update(func() {
			if a.client == client {
				a.creating = false
			}
		})
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		resp, err := client.CreateSessionWithACP(ctx, "新会话", agent, workspace, provider, model, acpConfig)
		if err != nil {
			a.connectionError(client, "新建会话失败: %v", err)
			return
		}
		id := resp.Session.ID
		var refs []mobileapi.Attachment
		for _, file := range files {
			att, err := client.UploadAttachment(ctx, id, file.Name,
				base64.StdEncoding.EncodeToString(file.Data))
			if err != nil {
				a.connectionError(client, "上传附件失败: %v", err)
				return
			}
			refs = append(refs, att)
		}
		if _, err := client.Send(ctx, id, text, refs, provider, model); err != nil {
			a.connectionError(client, "发送失败: %v", err)
			return
		}
		a.update(func() {
			if a.client != client {
				return
			}
			a.screen = screenChat
			a.roomID, a.roomName = "", ""
			a.openConv = id
			a.conv = &resp.Session
			a.chatModel = ""
			a.chatACP = acpModels{}
			if a.agentKind(a.conv.Agent) == "acp" {
				a.chatACP = a.newACP
			}
			a.syncChatModel(a.conv)
			a.running = true
			a.interaction = nil
			a.newDraft = ""
			a.newFiles = nil
			a.status = ""
			a.refreshSessions()
			a.loadChat(id)
		})
	}()
}

func (a *app) openChat(id, roomID, roomName string) {
	if a.client == nil {
		return
	}
	a.stopWatch()
	a.update(func() {
		a.screen = screenChat
		a.openConv = id
		a.roomID = roomID
		a.roomName = roomName
		a.conv = nil
		a.running = false
		a.interaction = nil
		a.draft = ""
		a.attachments = nil
		a.status = ""
		a.chatModel = ""
		a.chatACP = acpModels{}
		a.followEnd = true
	})
	a.loadChat(id)
}

func (a *app) loadChat(id string) {
	client := a.client
	if client == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		resp, err := client.Session(ctx, id)
		if err != nil {
			a.connectionError(client, "读取会话失败: %v", err)
			return
		}
		a.update(func() {
			if a.openConv != id || a.client != client {
				return
			}
			a.conv = &resp.Session
			switch a.agentKind(a.conv.Agent) {
			case "acp":
				if a.chatACP.agent != a.conv.Agent {
					current := ""
					if a.conv.ACPConfig != nil {
						current = a.conv.ACPConfig["model"]
					}
					a.chatACP = acpModels{agent: a.conv.Agent, current: current}
				}
				a.chatModel = ""
			case "a2a":
				a.chatACP = acpModels{}
				a.chatModel = ""
			default:
				if a.chatModel == "" {
					a.syncChatModel(a.conv)
				}
			}
			a.running = resp.Running
			a.interaction = resp.Interaction
			if resp.Interaction != nil {
				a.askOpen = true
			}
			if resp.Running {
				a.startWatch(id)
			}
		})
	}()
}

func (a *app) refreshCurrent(id string) {
	if a.client == nil {
		return
	}
	client := a.client
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		resp, err := client.Session(ctx, id)
		if err != nil {
			return
		}
		a.update(func() {
			if a.openConv != id || a.client != client {
				return
			}
			a.conv = &resp.Session
			a.running = resp.Running
			a.interaction = resp.Interaction
			if resp.Interaction != nil {
				a.askOpen = true
			}
		})
	}()
}

func (a *app) startWatch(id string) {
	a.stopWatch()
	client := a.client
	if client == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.watchCancel = cancel
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
			resp, err := client.Session(ctx, id)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				continue
			}
			a.update(func() {
				if a.openConv != id || a.client != client {
					return
				}
				a.conv = &resp.Session
				a.running = resp.Running
				a.interaction = resp.Interaction
				if resp.Interaction != nil {
					a.askOpen = true
				}
			})
			if !resp.Running {
				return
			}
		}
	}()
}

func (a *app) sendCurrent() {
	text := strings.TrimSpace(a.draft)
	if text == "" || a.client == nil || a.openConv == "" {
		return
	}
	id := a.openConv
	client := a.client
	roomID := a.roomID
	atts := append([]mobileapi.Attachment(nil), a.attachments...)
	// A room picks its own members' models; a plain chat sends with the one
	// chosen in its composer. ACP and A2A bring their own.
	provider, model := "", ""
	kind := a.agentKind(a.targetAgent(modelForChat))
	if roomID == "" && kind != "acp" && kind != "a2a" {
		provider, model = splitModelRef(a.chatModel)
	}
	a.update(func() {
		a.draft = ""
		a.attachments = nil
		a.status = "发送中…"
		a.followEnd = true
	})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var err error
		if roomID != "" {
			_, err = client.RoomSend(ctx, roomID, id, text, atts)
		} else {
			_, err = client.Send(ctx, id, text, atts, provider, model)
		}
		if err != nil {
			a.update(func() {
				if a.client != client || a.openConv != id {
					return
				}
				if a.draft == "" {
					a.draft = text
					a.attachments = atts
				}
				a.status = "发送失败，请重试: " + err.Error()
			})
			return
		}
		a.update(func() {
			if a.client != client || a.openConv != id {
				return
			}
			a.running = true
			a.status = ""
			a.loadChat(id)
		})
	}()
}

func (a *app) stopCurrent() {
	if a.client == nil || a.openConv == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := a.client.Cancel(ctx, a.openConv); err != nil {
			a.errorf("停止失败: %v", err)
		}
	}()
}

func (a *app) chatView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		if a.conv == nil {
			ui.Column(c).Grow(1).MinHeight(0).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "加载中…").TextColor(t.TextMuted)
			})
			return
		}
		// Follow the conversation's end while a turn runs, unless the reader
		// scrolled up from it; a conversation just opened or a message just
		// sent always comes into view.
		if a.followEnd || a.chatScroll.MaxY == 0 || (a.running && a.chatScroll.Y >= a.chatScroll.MaxY) {
			a.chatScroll.Y = math.MaxFloat32
			a.followEnd = false
		}
		ui.Scroll(c).Grow(1).MinHeight(0).TrackScroll(&a.chatScroll).Children(func() {
			ui.Column(c).FillWidth().Padding(12, 14, 12, 14).Gap(14).Children(func() {
				lastAuthor := ""
				for i := range a.conv.Messages {
					m := &a.conv.Messages[i]
					author := messageAuthor(m)
					// The user's own messages need no byline: their bubble
					// already sits on the right.
					a.messageView(c, m, author != lastAuthor && m.Role != "user")
					lastAuthor = author
				}
				if len(a.conv.Plan) > 0 {
					ui.Text(c, "执行计划").FontSize(12).TextColor(t.TextMuted)
					for _, step := range a.conv.Plan {
						mark := "○"
						switch step.Status {
						case models.PlanCompleted:
							mark = "✓"
						case models.PlanInProgress:
							mark = "●"
						}
						ui.Text(c, mark+" "+step.Content).FontSize(12).TextColor(t.TextMuted).MaxLines(3)
					}
				}
				if a.running && !lastAssistantHasText(a.conv) {
					ui.Text(c, "正在生成…").FontSize(13).TextColor(t.TextMuted)
				}
			})
		})
		a.composer(c)
	})
}

func lastAssistantHasText(conv *models.Conversation) bool {
	for i := len(conv.Messages) - 1; i >= 0; i-- {
		m := conv.Messages[i]
		if m.Role == "assistant" {
			return strings.TrimSpace(m.Content) != "" || len(m.Segments) > 0
		}
	}
	return false
}

func messageAuthor(m *models.Message) string {
	if m.Role == "user" {
		return "你"
	}
	return firstNonEmpty(m.AuthorName, "ChatGo")
}

// agentMarkBox is an agent's icon in the list's tinted square, or the mark
// of its kind when the desktop configured none.
func (a *app) agentMarkBox(c *ui.Context, ref string, running bool) {
	t := c.Theme()
	fg := t.TextMuted
	if running {
		fg = t.Accent
	}
	ui.Row(c).Size(34, 34).Radius(11).Background(t.Accent.Alpha(0.10)).
		AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		if src := a.agentIcons[ref]; src != nil {
			ui.Image(c, src).Size(20, 20)
			return
		}
		ui.Icon(c, icons.Must(sessionIcon(ref))).Size(18, 18).TextColor(fg)
	})
}

// listMark is the soft icon a list row leads with, accented while its item
// runs.
func listMark(c *ui.Context, icon string, running bool) {
	t := c.Theme()
	fg := t.TextMuted
	if running {
		fg = t.Accent
	}
	ui.Row(c).Size(34, 34).Radius(11).Background(t.Accent.Alpha(0.10)).
		AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		ui.Icon(c, icons.Must(icon)).Size(18, 18).TextColor(fg)
	})
}

func (a *app) messageView(c *ui.Context, m *models.Message, showAuthor bool) {
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		if showAuthor {
			author := messageAuthor(m)
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Row(c).Size(20, 20).Radius(10).Background(t.Accent.Alpha(0.14)).
					AlignItems(ui.Center).Justify(ui.Center).Children(func() {
					// A named agent carries its own mark; the built-in chat
					// keeps the chat icon, never a letter.
					a.agentMarkColor(c, a.agentRefForName(author), 14, t.Accent)
				})
				ui.Text(c, author).FontSize(12).FontWeight(600).TextColor(t.TextMuted)
			})
		}
		if m.Role == "user" {
			ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
				ui.Text(c, m.Content).FontSize(15).Padding(10, 14).Radius(mtheme.CardRadius * 2).
					Background(t.Accent.Alpha(0.16))
			})
			return
		}
		if len(m.Segments) > 0 {
			a.segmentsView(c, m)
			return
		}
		a.legacyView(c, m)
	})
}

// segmentsView renders one assistant turn the way the desktop does: the whole
// execution process — thinking, tool calls and intermediate messages — behind
// one fold, then the final answer as the message body. The process stays open
// while the turn runs and folds itself away when it ends, so a finished reply
// reads as the answer with its trace one tap away.
func (a *app) segmentsView(c *ui.Context, m *models.Message) {
	t := c.Theme()
	final := -1
	for i := len(m.Segments) - 1; i >= 0; i-- {
		seg := m.Segments[i]
		if seg.Kind == "text" && strings.TrimSpace(seg.Text) != "" {
			final = i
			break
		}
	}
	proc := make([]models.Segment, 0, len(m.Segments))
	thinking, tools := 0, 0
	for i, seg := range m.Segments {
		if i == final {
			continue
		}
		proc = append(proc, seg)
		switch {
		case seg.Kind == "reasoning" && strings.TrimSpace(seg.Text) != "":
			thinking++
		case seg.Kind == "tool" && seg.ToolCall != nil:
			tools++
		}
	}
	streaming := a.streamingMessage(m.ID)
	live := streaming && (final < 0 || final < len(m.Segments)-1)
	if thinking > 0 || tools > 0 {
		open := a.processFold(m.ID, live)
		row := ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Padding(4, 2).
			Role(ui.RoleButton).Label("执行过程")
		if row.Pressed() {
			row.Background(t.SurfaceHover)
		}
		row.Children(func() {
			name := "chevron-right"
			if open {
				name = "chevron-down"
			}
			ui.Icon(c, icons.Must(name)).Size(13, 13).TextColor(t.TextMuted).Shrink(0)
			ui.Text(c, fmt.Sprintf("思考 %d 次 · 工具调用 %d 次", thinking, tools)).
				FontSize(12).TextColor(t.TextMuted)
			if d := a.turnDuration(m.ID, streaming); d != "" {
				ui.Text(c, "用时 "+d).FontSize(12).TextColor(t.TextMuted)
			}
			if live {
				ui.Text(c, "…").FontSize(12).TextColor(t.Accent)
			}
		})
		if row.Clicked() {
			a.foldOpen[m.ID] = !open
			a.invalidate()
		}
		if open {
			ui.Column(c).FillWidth().Gap(4).Padding(0, 2).Children(func() {
				for i, seg := range proc {
					switch {
					case seg.Kind == "reasoning" && strings.TrimSpace(seg.Text) != "":
						a.reasoningRow(c, m.ID+":think:"+strconv.Itoa(i), seg.Text)
					case seg.Kind == "tool" && seg.ToolCall != nil:
						a.toolView(c, m.ID+":seg:"+strconv.Itoa(i), seg.ToolCall)
					case seg.Kind == "text" && strings.TrimSpace(seg.Text) != "":
						kit.MarkdownView(c, seg.Text).Padding(2, 8)
					}
				}
			})
		}
	}
	if final >= 0 {
		kit.MarkdownView(c, m.Segments[final].Text)
	} else if streaming {
		ui.Text(c, "正在生成…").FontSize(13).TextColor(t.TextMuted)
	}
}

// legacyView renders a message written before segments existed: thinking,
// then tool calls, then the reply — the desktop's order.
func (a *app) legacyView(c *ui.Context, m *models.Message) {
	if strings.TrimSpace(m.Reasoning) != "" {
		a.reasoningRow(c, m.ID+":reasoning", m.Reasoning)
	}
	for i := range m.ToolCalls {
		a.toolView(c, m.ID+":tool:"+strconv.Itoa(i), &m.ToolCalls[i])
	}
	if strings.TrimSpace(m.Content) != "" {
		kit.MarkdownView(c, m.Content)
	}
}

// reasoningRow is one thinking block inside the process fold: a one-line
// summary that opens to the whole thought.
func (a *app) reasoningRow(c *ui.Context, key, text string) {
	t := c.Theme()
	if a.thinkOpen == nil {
		a.thinkOpen = map[string]bool{}
	}
	expanded := a.thinkOpen[key]
	summary := reasoningSummary(text)
	row := ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Padding(6, 8).
		Radius(mtheme.ControlRadius).Role(ui.RoleButton).Label("思考 " + summary)
	if row.Pressed() {
		row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		ui.Text(c, "✳").FontSize(12).TextColor(t.TextMuted).Shrink(0)
		ui.Text(c, "思考").FontSize(12).FontWeight(600).TextColor(t.TextMuted).Shrink(0)
		ui.Text(c, summary).FontSize(12).TextColor(t.TextMuted).
			Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
		arrow := "›"
		if expanded {
			arrow = "⌄"
		}
		ui.Text(c, arrow).FontSize(14).TextColor(t.TextMuted).Shrink(0)
	})
	if row.Clicked() {
		a.thinkOpen[key] = !expanded
		a.invalidate()
	}
	if !expanded {
		return
	}
	// The opened thought sits in the same box as an opened tool call.
	ui.Column(c).FillWidth().Gap(6).Padding(8, 10).Radius(mtheme.ControlRadius).
		Background(t.Background).Border(1, t.Border).Children(func() {
		ui.Text(c, text).FontSize(12).TextColor(t.Text)
	})
}

// reasoningSummary keeps the newest non-empty line of a thought, which is
// where the thinking ended up.
func reasoningSummary(text string) string {
	summary := ""
	for _, line := range strings.Split(text, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			summary = t
		}
	}
	return clipText(summary, 60)
}

// streamingMessage reports whether the turn that produced msg is still being
// written: the conversation is running and msg is its newest reply.
func (a *app) streamingMessage(id string) bool {
	if !a.running || a.conv == nil {
		return false
	}
	for i := len(a.conv.Messages) - 1; i >= 0; i-- {
		if a.conv.Messages[i].Role == "assistant" {
			return a.conv.Messages[i].ID == id
		}
	}
	return false
}

// processFold follows the turn: open while it runs, folded once it ends.
func (a *app) processFold(id string, live bool) bool {
	if a.foldOpen == nil {
		a.foldOpen = map[string]bool{}
	}
	if a.foldLive == nil {
		a.foldLive = map[string]bool{}
	}
	if prev, ok := a.foldLive[id]; !ok || prev != live {
		a.foldOpen[id] = live
		a.foldLive[id] = live
	}
	return a.foldOpen[id]
}

// turnDuration reports how long the turn that produced msg ran, or has been
// running while it still streams.
func (a *app) turnDuration(msgID string, streaming bool) string {
	if a.conv == nil {
		return ""
	}
	for i := len(a.conv.Turns) - 1; i >= 0; i-- {
		turn := a.conv.Turns[i]
		if turn.ReplyMsgID != msgID {
			continue
		}
		if turn.StartedAt.IsZero() {
			return ""
		}
		end := turn.EndedAt
		if end.IsZero() {
			if !streaming {
				return ""
			}
			end = time.Now()
		}
		if end.Before(turn.StartedAt) {
			return ""
		}
		return kit.AgentDuration(end.Sub(turn.StartedAt))
	}
	return ""
}

func (a *app) toolView(c *ui.Context, key string, tc *models.ToolCall) {
	t := c.Theme()
	label := firstNonEmpty(tc.Name, "工具")
	status := tc.Status
	if status == "" {
		status = "completed"
	}
	statusText := map[string]string{
		"completed": "completed", "failed": "failed", "error": "failed",
		"in_progress": "running", "running": "running",
	}[status]
	if statusText == "" {
		statusText = status
	}
	statusColor := t.TextMuted
	switch status {
	case "completed":
		statusColor = t.Success
	case "failed", "error":
		statusColor = t.Danger
	case "in_progress", "running":
		statusColor = t.Accent
	}
	if a.toolOpen == nil {
		a.toolOpen = map[string]bool{}
	}
	expanded := a.toolOpen[key]
	expandable := tc.Arguments != "" || tc.Result != "" || tc.Error != ""
	// The header reads like a thought's: an icon, a bold kind, the summary
	// and an arrow, with no card of its own.
	row := ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Padding(6, 8).
		Radius(mtheme.ControlRadius)
	if expandable {
		row.Role(ui.RoleButton).Label(label + " " + statusText)
		if row.Pressed() {
			row.Background(t.SurfaceHover)
		}
		if row.Clicked() {
			a.toolOpen[key] = !expanded
			a.invalidate()
		}
	}
	row.Children(func() {
		ui.Text(c, "⚙").FontSize(12).TextColor(statusColor).Shrink(0)
		ui.Text(c, "工具").FontSize(12).FontWeight(600).TextColor(t.TextMuted).Shrink(0)
		ui.Text(c, label).FontSize(12).TextColor(t.TextMuted).
			Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
		ui.Text(c, statusText).FontSize(11).TextColor(statusColor).Shrink(0)
		if expandable {
			arrow := "›"
			if expanded {
				arrow = "⌄"
			}
			ui.Text(c, arrow).FontSize(14).TextColor(t.TextMuted).Shrink(0)
		}
	})
	if !expanded {
		return
	}
	ui.Column(c).FillWidth().Gap(6).Padding(8, 10).Radius(mtheme.ControlRadius).
		Background(t.Background).Border(1, t.Border).Children(func() {
		if tc.Arguments != "" {
			ui.Text(c, "参数").FontSize(11).TextColor(t.TextMuted)
			ui.Text(c, clipText(tc.Arguments, 4000)).Font(mtheme.MonoFont).
				FontSize(11).TextColor(t.Text)
		}
		if tc.Result != "" {
			ui.Text(c, "结果").FontSize(11).TextColor(t.TextMuted)
			ui.Text(c, clipText(tc.Result, 4000)).Font(mtheme.MonoFont).
				FontSize(11).TextColor(t.Text)
		}
		if tc.Error != "" {
			ui.Text(c, "错误").FontSize(11).TextColor(t.Danger)
			ui.Text(c, clipText(tc.Error, 4000)).Font(mtheme.MonoFont).
				FontSize(11).TextColor(t.Danger)
		}
	})
}

func clipText(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func (a *app) composer(c *ui.Context) {
	t := c.Theme()
	// The window's root already keeps its content above the system's bars and
	// the keyboard, so the composer adds no bottom inset of its own: padding
	// it again would lift the field off the keyboard by its full height.
	ui.Column(c).FillWidth().Shrink(0).Padding(8, 10, 8, 10).Gap(6).Background(t.Background).Children(func() {
		if a.interaction != nil {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Padding(8, 10).Radius(mtheme.ControlRadius).
				Background(t.Warning.Alpha(0.14)).Children(func() {
				ui.Text(c, interactionLabel(a.interaction)).FontSize(12).TextColor(t.Text).Grow(1).MaxLines(2)
				if secondaryButton(c, "处理").Clicked() {
					a.askOpen = true
				}
			})
		}
		ui.Column(c).FillWidth().Gap(5).Padding(5, 10).Radius(16).
			Background(t.Surface).Children(func() {
			if len(a.attachments) > 0 || a.attachBusy {
				ui.Row(c).FillWidth().Gap(6).Wrap().Padding(2, 0).Children(func() {
					if a.attachBusy {
						ui.Text(c, "上传中…").FontSize(11).TextColor(t.TextMuted)
					}
					for i := range a.attachments {
						att := a.attachments[i]
						ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(4, 8).
							Radius(10).Background(t.Background).Children(func() {
							ui.Text(c, att.Name).FontSize(11).SingleLine().Ellipsis("…").MaxWidth(160)
							if kit.Button(c, "", kit.ButtonOptions{
								Variant: kit.Ghost, Icon: icons.Must("x"), Label: "移除附件",
							}).Height(26).Padding(0, 4).Clicked() {
								a.attachments = append(a.attachments[:i], a.attachments[i+1:]...)
							}
						})
					}
				})
			}
			// The field is the card: no border or fill of its own.
			composerField(c, &a.draft)
			ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
				attach := kit.Button(c, "", kit.ButtonOptions{
					Variant: kit.Ghost, Icon: icons.Must("plus"), Label: "添加附件",
					Foreground: t.TextMuted,
				}).Size(36, 36).Padding(0)
				if attach.Clicked() {
					a.pickAttachments()
				}
				// A room's members each bring their own model, so only a plain
				// chat picks one here.
				if a.roomID == "" {
					a.modelChip(c, &a.chatModel, modelForChat)
				}
				if a.roomID != "" || len(a.meta.Models) == 0 {
					ui.Spacer(c)
				}
				if a.running {
					stop := kit.IconButton(c, icons.Must("square"), "停止", kit.IconButtonOptions{
						Variant: kit.Danger, Size: 36,
					})
					if stop.Clicked() {
						a.stopCurrent()
					}
				} else {
					send := kit.IconButton(c, icons.Must("arrow-up"), "发送", kit.IconButtonOptions{
						Variant: kit.Primary, Size: 36,
						Disabled: strings.TrimSpace(a.draft) == "",
					})
					// Only the button sends: Enter is a newline in the field.
					if send.Clicked() {
						a.sendCurrent()
					}
				}
			})
		})
		if a.status != "" {
			ui.Text(c, a.status).FontSize(11).TextColor(t.TextMuted).MaxLines(2)
		}
	})
}

func (a *app) pickAttachments() {
	if a.client == nil || a.openConv == "" {
		return
	}
	id := a.openConv
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Title:    "添加附件",
			Multiple: true,
			Filters:  []mygo.FileFilter{{Name: "文件", Extensions: []string{"*"}}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.update(func() { a.attachBusy = true })
		defer a.update(func() { a.attachBusy = false })
		for _, path := range paths {
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			if len(data) > 8<<20 {
				a.errorf("附件超过 8 MB: %s", filepath.Base(path))
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			att, err := a.client.UploadAttachment(ctx, id, filepath.Base(path), base64.StdEncoding.EncodeToString(data))
			cancel()
			if err != nil {
				a.errorf("上传附件失败: %v", err)
				continue
			}
			a.update(func() { a.attachments = append(a.attachments, att) })
		}
	}()
}

func interactionLabel(i *mobileapi.Interaction) string {
	if i == nil {
		return ""
	}
	if i.Kind == "permission" && i.Permission != nil {
		return "需要审批: " + i.Permission.Title
	}
	if len(i.Items) > 0 {
		return "Agent 提问: " + i.Items[0].Question
	}
	return "Agent 需要你的输入"
}

func (a *app) askSheet(c *ui.Context) {
	if a.interaction == nil {
		a.askOpen = false
		return
	}
	mobile.Sheet(c, &a.askOpen, mobile.SheetOptions{Detents: []float32{0.5, 0.92}, Label: "Agent 交互"}, func() {
		ui.Column(c).FillWidth().Padding(16).Gap(12).Children(func() {
			i := a.interaction
			if i.Kind == "permission" && i.Permission != nil {
				ui.Text(c, i.Permission.Title).FontSize(17).FontWeight(600)
				if i.Permission.Detail != "" {
					ui.Text(c, i.Permission.Detail).FontSize(13).TextColor(c.Theme().TextMuted).MaxLines(8)
				}
				for _, opt := range i.Permission.Options {
					option := opt
					if secondaryButton(c, option.Name).Clicked() {
						a.decide(option.ID, false)
					}
				}
				if ghostButton(c, "取消").Clicked() {
					a.decide("", true)
				}
				return
			}
			if len(i.Items) > 0 {
				ui.Text(c, i.Items[0].Question).FontSize(16).FontWeight(600)
				if len(i.Items[0].Options) > 0 {
					for _, opt := range i.Items[0].Options {
						option := opt
						if secondaryButton(c, option).Clicked() {
							a.answerQuestion(option)
						}
					}
				}
				answerInput := ui.TextInput(c, &a.answer).Placeholder("输入回答")
				if primaryButton(c, "提交").Clicked() || answerInput.Submitted() {
					a.answerQuestion(a.answer)
				}
			}
		})
	})
}

func (a *app) answerQuestion(answer string) {
	if a.interaction == nil || strings.TrimSpace(answer) == "" {
		return
	}
	id := a.openConv
	interaction := a.interaction.ID
	a.update(func() {
		a.askOpen = false
		a.answer = ""
		a.interaction = nil
	})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := a.client.Answer(ctx, id, interaction, answer); err != nil {
			a.errorf("提交回答失败: %v", err)
			return
		}
		a.loadChat(id)
	}()
}

func (a *app) decide(option string, canceled bool) {
	if a.interaction == nil {
		return
	}
	id := a.openConv
	interaction := a.interaction.ID
	a.update(func() {
		a.askOpen = false
		a.interaction = nil
	})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := a.client.Decide(ctx, id, interaction, option, canceled); err != nil {
			a.errorf("提交审批失败: %v", err)
			return
		}
		a.loadChat(id)
	}()
}
