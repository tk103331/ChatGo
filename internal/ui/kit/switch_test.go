package kit

import (
	"testing"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/uikit"
)

// The switch's own Label is only an assistive-technology name, so the row
// must draw the text: a bare toggle leaves its setting unreadable.
func TestSwitchRowShowsItsLabel(t *testing.T) {
	cases := map[string]SwitchRowOptions{
		"row":     {},
		"compact": {Compact: true},
	}
	for name, opts := range cases {
		t.Run(name, func(t *testing.T) {
			on := false
			view := func(c *mgo.Context) {
				uikit.Use(c, uikit.Settings{})
				SwitchRow(c, &on, "显示托盘图标", opts)
			}
			tr := mgo.NewTester(view, 400, 80)
			tr.Frame()
			if !tr.HasText("显示托盘图标") {
				t.Fatalf("the label is not drawn; texts: %v", tr.Texts())
			}
		})
	}
}

// The hint under the label explains the setting in the same row.
func TestSwitchRowShowsItsHint(t *testing.T) {
	on := true
	view := func(c *mgo.Context) {
		uikit.Use(c, uikit.Settings{})
		SwitchRow(c, &on, "后台运行", SwitchRowOptions{Hint: "关闭窗口时最小化到托盘"})
	}
	tr := mgo.NewTester(view, 500, 100)
	tr.Frame()
	if !tr.HasText("后台运行") || !tr.HasText("关闭窗口时最小化到托盘") {
		t.Fatalf("the row lost its label or hint; texts: %v", tr.Texts())
	}
}
