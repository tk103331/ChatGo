package ui

import (
	"fmt"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// selectItem is one row of a selection list: a checkbox with a title and, on a
// second line, the detail that explains it.
type selectItem struct {
	ID    string
	Title string
	Desc  string
	// Tags are short facts shown as chips under the detail, for entries whose
	// contents matter at a glance: the models a provider offers, say.
	Tags []string
}

// selectListHeader renders the "n/total" count with the select-all and
// clear-all text buttons, and reports which one was pressed.
func selectListHeader(c *mgo.Context, on, total int) (selectAll, clearAll bool) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Row(c).FillWidth().MinWidth(0).Gap(u).AlignItems(mgo.Center).Children(func() {
		mgo.Text(c, fmt.Sprintf("已选 %d/%d", on, total)).
			FontSize(caption).TextColor(k.TextMuted).Grow(1).MinWidth(0).SingleLine()
		selectAll = textAction(c, "全选", on < total)
		clearAll = textAction(c, "取消全选", on > 0)
	})
	return selectAll, clearAll
}

// textAction is a compact text-only button for a list's bulk actions.
func textAction(c *mgo.Context, label string, enabled bool) bool {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	b := mgo.ButtonBase(c).Key("textaction:"+label).Shrink(0).
		Padding(u/2, u).Radius(mtheme.ControlRadius).Label(label).
		Disabled(!enabled).Cursor(mgo.CursorPointer)
	color := k.Accent
	if !enabled {
		color = k.TextMuted
	} else if b.Hovered() {
		b.Background(k.SurfaceHover)
	}
	b.Children(func() {
		mgo.Text(c, label).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).
			TextColor(color).SingleLine()
	})
	return b.Clicked() && enabled
}

// selectRow renders one two-line entry: a checkbox with the title, then the
// detail truncated to one line with the full text as its tooltip.
func selectRow(c *mgo.Context, item selectItem, checked bool, onToggle func(bool)) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	row := mgo.Column(c).Key("sel:"+item.ID).FillWidth().MinWidth(0).Gap(u/3).
		Padding(u/2, u).Radius(mtheme.ControlRadius)
	if row.Hovered() {
		row.Background(k.SurfaceHover)
	}
	row.Children(func() {
		v := checked
		if mgo.Checkbox(c, &v, item.Title).FillWidth().MinWidth(0).Changed() {
			onToggle(v)
		}
		if item.Desc != "" {
			// The detail is indented to the checkbox's label and tooltipped,
			// so a long description stays readable without widening the list.
			mgo.Text(c, item.Desc).FontSize(caption).TextColor(k.TextMuted).
				Padding(0, 0, 0, u*3).MinWidth(0).SingleLine().Ellipsis("…").
				Tooltip(item.Desc)
		}
		if len(item.Tags) > 0 {
			mgo.Row(c).FillWidth().MinWidth(0).Gap(u/2).Wrap().
				Padding(0, 0, 0, u*3).Children(func() {
				for _, tag := range item.Tags {
					agentTag(c, tag)
				}
			})
		}
	})
}

// selectList splits a searchable two-line selection list into a pinned part
// and a scrolling part. The pinned part carries the search field and bulk
// actions, so they stay put while the entries scroll; the body reports
// whether anything changed this frame. isOn reads an item's state and setOn
// writes it; both are called with the item's ID.
func selectListSplit(c *mgo.Context, query *string, placeholder string, items []selectItem,
	isOn func(id string) bool, setOn func(id string, on bool)) (func(), func() bool) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	changed := false
	pinned := func() {
		mgo.Column(c).FillWidth().MinWidth(0).Gap(u * 2).Children(func() {
			agentSearch(c, query, placeholder)
			shown := filterSelectItems(items, *query)
			on := 0
			for _, it := range shown {
				if isOn(it.ID) {
					on++
				}
			}
			all, none := selectListHeader(c, on, len(shown))
			if all || none {
				for _, it := range shown {
					setOn(it.ID, all)
				}
				changed = true
			}
		})
	}
	body := func() bool {
		shown := filterSelectItems(items, *query)
		mgo.Column(c).FillWidth().MinWidth(0).Gap(u).Children(func() {
			if len(shown) == 0 {
				mgo.Text(c, "没有匹配的项").FontSize(mcore.FontSize(c, mtheme.CaptionSize)).
					TextColor(k.TextMuted)
				return
			}
			for _, it := range shown {
				it := it
				selectRow(c, it, isOn(it.ID), func(v bool) {
					setOn(it.ID, v)
					changed = true
				})
			}
		})
		return changed
	}
	return pinned, body
}

// selectList renders the whole list in one container, for a place that
// scrolls as a whole rather than pinning its search field.
func selectList(c *mgo.Context, query *string, placeholder string, items []selectItem,
	isOn func(id string) bool, setOn func(id string, on bool)) bool {
	pinned, body := selectListSplit(c, query, placeholder, items, isOn, setOn)
	pinned()
	return body()
}

// filterSelectItems narrows items to those matching query by title or detail.
func filterSelectItems(items []selectItem, query string) []selectItem {
	q := lowerTrim(query)
	if q == "" {
		return items
	}
	out := make([]selectItem, 0, len(items))
	for _, it := range items {
		if containsFold(it.Title, q) || containsFold(it.Desc, q) {
			out = append(out, it)
		}
	}
	return out
}

func lowerTrim(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func containsFold(s, lowerNeedle string) bool {
	return strings.Contains(strings.ToLower(s), lowerNeedle)
}

// selectListGrouped is selectList with the entries split under the heading of
// the source they came from, which is how the import dialogs tell apart the
// same name offered by two agents.
func selectListGrouped(c *mgo.Context, query *string, placeholder string, items []selectItem,
	groupOf map[string]string, isOn func(id string) bool, setOn func(id string, on bool)) bool {
	pinned, body := selectListGroupedSplit(c, query, placeholder, items, groupOf, isOn, setOn)
	pinned()
	return body()
}

// selectListGroupedSplit is selectListGrouped with the search field and bulk
// actions apart from the scrolling entries, for a dialog that pins them.
func selectListGroupedSplit(c *mgo.Context, query *string, placeholder string, items []selectItem,
	groupOf map[string]string, isOn func(id string) bool, setOn func(id string, on bool)) (func(), func() bool) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	changed := false
	pinned := func() {
		mgo.Column(c).FillWidth().MinWidth(0).Gap(u * 2).Children(func() {
			agentSearch(c, query, placeholder)
			shown := filterSelectItems(items, *query)
			on := 0
			for _, it := range shown {
				if isOn(it.ID) {
					on++
				}
			}
			all, none := selectListHeader(c, on, len(shown))
			if all || none {
				for _, it := range shown {
					setOn(it.ID, all)
				}
				changed = true
			}
		})
	}
	body := func() bool {
		shown := filterSelectItems(items, *query)
		mgo.Column(c).FillWidth().MinWidth(0).Gap(u).Children(func() {
			if len(shown) == 0 {
				mgo.Text(c, "没有匹配的项").FontSize(mcore.FontSize(c, mtheme.CaptionSize)).
					TextColor(k.TextMuted)
				return
			}
			last := ""
			for _, it := range shown {
				it := it
				if g := groupOf[it.ID]; g != last {
					last = g
					mgo.Text(c, g).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).
						FontWeight(600).TextColor(k.TextMuted).Padding(u, 0, 0, 0)
				}
				selectRow(c, it, isOn(it.ID), func(v bool) {
					setOn(it.ID, v)
					changed = true
				})
			}
		})
		return changed
	}
	return pinned, body
}
