package git

import (
	"github.com/egoist/mygo/ui"

	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/data"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/overlay"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// GitTag is a tag and the commit it names.
type GitTag struct {
	Name   string
	Target string
	// Annotation is the tag message; empty for a lightweight tag.
	Annotation string
}

// TagListOptions configure a TagList.
type TagListOptions struct {
	// RowMenu adds a context menu to every row; item is the tag name.
	RowMenu func(c *ui.Context, item string)
	Status  data.DataStatus
	Error   string
}

// TagList shows tags; Action "jump" carries the target hash.
func TagList(c *ui.Context, s *data.ListState[string], tags []GitTag, opts TagListOptions) GitListResult {
	var r GitListResult
	k := core.Tokens(c)
	v := data.List(c, s, len(tags), data.ListOptions[string]{
		Key:    func(i int) string { return tagListKey(tags[i]) },
		Label:  func(i int) string { return tags[i].Name },
		Status: opts.Status, Error: opts.Error, Empty: core.Msg(c, "tag_list.empty"),
	}, func(i int) {
		t := tags[i]
		small := core.FontSize(c, theme.CaptionSize)
		row := ui.Row(c).AlignItems(ui.Center).Gap(core.Density(c).Unit()).MinWidth(0).Grow(1)
		row.Children(func() {
			ui.Icon(c, icons.Must("tag")).Size(14, 14).Shrink(0).TextColor(k.TextMuted)
			ui.Text(c, t.Name).FontWeight(600).SingleLine().Shrink(0).MaxWidth(200)
			ui.Text(c, t.Annotation).FontSize(small).TextColor(k.TextMuted).SingleLine().Grow(1).MinWidth(0)
			ui.Text(c, gitShortHash(t.Target)).Font(theme.MonoFont).FontSize(small).TextColor(k.AccentText).Shrink(0)
			gitRowActions(c, []GitAction{{ID: "jump", Label: core.Msg(c, "tag_list.jump"), Icon: "git-commit-horizontal"}}, t.Target, &r)
		})
		if opts.RowMenu != nil {
			overlay.ContextMenu(c, row, overlay.ContextMenuOptions{}, func(*overlay.PopupMenu) {
				opts.RowMenu(c, t.Name)
			})
		}
	})
	r.Element, r.changed, r.submitted = v.Element, v.Changed(), v.Submitted()
	return r
}

func tagListKey(t GitTag) string {
	if t.Name == "" || t.Target == "" {
		panic("mujica: GitTag needs a Name and a Target")
	}
	return t.Name
}
