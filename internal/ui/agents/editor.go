package agents

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"chatgo/internal/config"
	"chatgo/internal/ui/common"
	skillspkg "chatgo/internal/ui/skills"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mdata "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/data"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// editor is the create/edit page, with collapsible capability sections.
func editor(c *mgo.Context, state *State, opts Options) {
	f := &state.Form
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	models := opts.ModelOptions(f.Models)
	tools := opts.EnabledBuiltinTools()
	servers := opts.EnabledMCPServers()
	skillsEnabled := opts.EnabledSkills()
	names := make([]string, 0, len(skillsEnabled))
	for _, s := range skillsEnabled {
		names = append(names, s.Name)
	}
	title := "新建 Agent"
	if f.Editing >= 0 {
		title = "编辑 Agent"
	}
	mgo.Column(c).Fill().Padding(u * 4).Gap(u * 3).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
			if minput.Button(c, "返回", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("chevron-left")}).Clicked() {
				state.EditorOpen = false
			}
			mgo.Text(c, title).Font(mtheme.SerifFont).
				FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600).Grow(1)
			if minput.Button(c, "保存", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
				if Save(state, opts) {
					state.EditorOpen = false
				}
			}
			if minput.Button(c, "删除", minput.ButtonOptions{Variant: minput.Danger, Disabled: f.Editing < 0}).Clicked() {
				Delete(state, opts)
			}
		})
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			mgo.Column(c).Gap(u * 3).Children(func() {
				mgo.Row(c).Gap(u * 3).AlignItems(mgo.Start).Children(func() {
					mgo.Column(c).Shrink(0).Gap(u).AlignItems(mgo.Center).Children(func() {
						opts.DrawMark(c, f.Icon, f.AvatarSeed, f.Name, 56)
						mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
							if mcore.IconAction(c, icons.Must("image"), "选择图片").Tooltip("选择图片").Clicked() {
								opts.PickIconFile(func(path string) {
									f.Icon = path
									f.AvatarSeed = ""
								})
							}
							if mcore.IconAction(c, icons.Must("refresh-cw"), "生成图标").Tooltip("生成图标").Clicked() {
								base := strings.TrimSpace(f.Name)
								if base == "" {
									base = "Agent"
								}
								f.AvatarSeed = fmt.Sprintf("%s#%d", base, time.Now().UnixNano())
								f.Icon = ""
								opts.Invalidate()
							}
							if f.Icon != "" || f.AvatarSeed != "" {
								if mcore.IconAction(c, icons.Must("x"), "清除图标").Tooltip("清除图标").Clicked() {
									f.Icon = ""
									f.AvatarSeed = ""
								}
							}
						})
					})
					mgo.Column(c).Grow(1).MinWidth(0).Gap(u * 2).Children(func() {
						minput.TextInput(c, &f.Name, minput.TextInputOptions{Label: "名称", Placeholder: "Agent 名称"})
						minput.TextInput(c, &f.Description, minput.TextInputOptions{Label: "描述", Placeholder: "一句话说明"})
					})
				})
				minput.TextArea(c, &f.SystemPrompt, minput.TextAreaOptions{
					Label: "系统提示词", Placeholder: "设定该 Agent 的角色与行为", MinHeight: 96,
				})
				mgo.Row(c).Gap(u).AlignItems(mgo.End).Children(func() {
					in := minput.TextInput(c, &f.Workspace, minput.TextInputOptions{Label: "工作目录", Placeholder: "留空使用当前项目"})
					in.Element.Grow(1)
					if minput.Button(c, "选择", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
						opts.PickDirectory("选择工作目录", func(path string) { f.Workspace = path })
					}
				})
				on := countSelected(f.Models, models)
				group(c, &state.SecModels, fmt.Sprintf("可用模型  %d/%d", on, len(models)), func() {
					action(c, on, len(models), func(all bool) { f.Models = common.ToggleAll(models, all) },
						func() { opts.GoConfigure(NavModels) })
					mgo.Row(c).Gap(u).Wrap().Children(func() {
						for _, ref := range models {
							ref := ref
							on := common.ContainsStr(f.Models, ref)
							minput.Checkbox(c, &on, opts.ModelLabel(ref), minput.CheckboxOptions{})
							f.Models = common.SetTool(f.Models, ref, on)
						}
					})
				})
				on = countSelected(f.BuiltinTools, tools)
				group(c, &state.SecTools, fmt.Sprintf("工具  %d/%d", on, len(tools)), func() {
					action(c, on, len(tools), func(all bool) { f.BuiltinTools = common.ToggleAll(tools, all) },
						func() { opts.GoConfigure(NavTools) })
					common.SearchField(c, &f.ToolQuery, "搜索工具")
					mgo.Grid(c).Columns(2).Gap(u).Children(func() {
						for _, t := range common.FilterStrings(tools, f.ToolQuery) {
							t := t
							mgo.Column(c).Gap(0).MinWidth(0).Children(func() {
								on := common.ContainsStr(f.BuiltinTools, t)
								minput.Checkbox(c, &on, t, minput.CheckboxOptions{})
								f.BuiltinTools = common.SetTool(f.BuiltinTools, t, on)
								mgo.Text(c, config.GetBuiltinToolDescription(t)).FontSize(small).TextColor(k.TextMuted).
									MaxLines(1).Ellipsis("…")
							})
						}
					})
				})
				on = countSelected(f.MCPServers, servers)
				group(c, &state.SecMCP, fmt.Sprintf("MCP 服务器  %d/%d", on, len(servers)), func() {
					action(c, on, len(servers), func(all bool) { f.MCPServers = common.ToggleAll(servers, all) },
						func() { opts.GoConfigure(NavMCP) })
					if len(servers) == 0 {
						mgo.Text(c, "尚未配置 MCP 服务器。").FontSize(small).TextColor(k.TextMuted)
					}
					common.SearchField(c, &f.MCPQuery, "搜索 MCP 服务器")
					mgo.Grid(c).Columns(2).Gap(u).Children(func() {
						for _, s := range common.FilterStrings(servers, f.MCPQuery) {
							on := common.ContainsStr(f.MCPServers, s)
							minput.Checkbox(c, &on, s, minput.CheckboxOptions{})
							f.MCPServers = common.SetTool(f.MCPServers, s, on)
						}
					})
				})
				on = countSelected(f.Skills, names)
				group(c, &state.SecSkills, fmt.Sprintf("Skills  %d/%d", on, len(names)), func() {
					action(c, on, len(names), func(all bool) { f.Skills = common.ToggleAll(names, all) },
						func() { opts.GoConfigure(NavSkills) })
					if len(names) == 0 {
						mgo.Text(c, "未发现 Skill。").FontSize(small).TextColor(k.TextMuted)
					}
					common.SearchField(c, &f.SkillQuery, "搜索 Skill")
					mgo.Column(c).Gap(u).Children(func() {
						for _, s := range filterSkills(skillsEnabled, f.SkillQuery) {
							s := s
							on := common.ContainsStr(f.Skills, s.Name)
							mgo.Row(c).FillWidth().Gap(u * 2).AlignItems(mgo.Center).Children(func() {
								minput.Checkbox(c, &on, "", minput.CheckboxOptions{})
								body := mgo.ButtonBase(c).Grow(1).MinWidth(0).Justify(mgo.Start).Children(func() {
									mgo.Column(c).Grow(1).MinWidth(0).Children(func() {
										mgo.Text(c, s.Name).SingleLine()
										if s.Description != "" {
											mgo.Text(c, s.Description).FontSize(small).TextColor(k.TextMuted).
												SingleLine().Ellipsis("…")
										}
									})
								})
								if body.Clicked() {
									on = !on
								}
							})
							f.Skills = common.SetTool(f.Skills, s.Name, on)
						}
					})
				})
			})
		})
	})
}

// group renders a collapsible capability section.
func group(c *mgo.Context, open *bool, title string, body func()) {
	mdata.Collapsible(c, open, mdata.CollapsibleOptions{Title: title}, body)
}

// action shows a group's count with a select-all/clear button and an
// optional shortcut to the global configuration page.
func action(c *mgo.Context, on, total int, toggle func(all bool), configure func()) {
	k := mcore.Tokens(c)
	small := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Row(c).FillWidth().AlignItems(mgo.Center).Children(func() {
		mgo.Text(c, "已选 "+strconv.Itoa(on)).FontSize(small).TextColor(k.TextMuted).Grow(1)
		label := "全选"
		if total > 0 && on == total {
			label = "清空"
		}
		if minput.Button(c, label, minput.ButtonOptions{Variant: minput.Ghost}).Clicked() {
			toggle(on != total)
		}
		if configure != nil {
			if minput.Button(c, "去配置", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("sliders-horizontal")}).Clicked() {
				configure()
			}
		}
	})
}

// countSelected counts how many of want are in have.
func countSelected(have, want []string) int {
	n := 0
	for _, w := range want {
		if common.ContainsStr(have, w) {
			n++
		}
	}
	return n
}

// filterSkills returns the skills whose name or description contains query.
func filterSkills(list []skillspkg.Skill, query string) []skillspkg.Skill {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return list
	}
	var out []skillspkg.Skill
	for _, s := range list {
		if strings.Contains(strings.ToLower(s.Name), q) || strings.Contains(strings.ToLower(s.Description), q) {
			out = append(out, s)
		}
	}
	return out
}
