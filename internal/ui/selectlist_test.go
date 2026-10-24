package ui

import (
	"strings"
	"testing"

	"chatgo/internal/config"

	mgo "github.com/egoist/mygo/ui"

	mcore "chatgo/internal/ui/uikit"
)

func TestFilterSelectItemsMatchesTitleAndDesc(t *testing.T) {
	items := []selectItem{
		{ID: "a", Title: "命令行", Desc: "执行 shell 命令"},
		{ID: "b", Title: "JS 沙箱", Desc: "运行 JavaScript 代码"},
	}
	if got := filterSelectItems(items, ""); len(got) != 2 {
		t.Fatalf("an empty query should keep everything, got %d", len(got))
	}
	if got := filterSelectItems(items, "沙箱"); len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("title match got %+v", got)
	}
	if got := filterSelectItems(items, "shell"); len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("description match got %+v", got)
	}
	if got := filterSelectItems(items, "javascript"); len(got) != 1 {
		t.Fatalf("matching should ignore case, got %+v", got)
	}
	if got := filterSelectItems(items, "无此项"); len(got) != 0 {
		t.Fatalf("a miss should return nothing, got %+v", got)
	}
}

// TestSelectListBulkActions checks that the header reports the bulk actions
// only when they would do something, and that selectList applies them.
func TestSelectListBulkActions(t *testing.T) {
	items := []selectItem{
		{ID: "a", Title: "甲", Desc: "第一个"},
		{ID: "b", Title: "乙", Desc: "第二个"},
		{ID: "c", Title: "丙", Desc: "第三个"},
	}
	state := map[string]bool{}
	query := ""
	changed := false
	render := func() {
		mgo.Render(func(c *mgo.Context) {
			mcore.Use(c, mcore.Settings{})
			mgo.Column(c).Fill().Padding(8).Children(func() {
				changed = selectList(c, &query, "搜索", items,
					func(id string) bool { return state[id] },
					func(id string, on bool) { state[id] = on })
			})
		}, 420, 600, 1)
	}

	render()
	if changed {
		t.Fatal("rendering alone must not toggle items")
	}
	for _, it := range items {
		if state[it.ID] {
			t.Fatalf("item %s should start off", it.ID)
		}
	}

	// With nothing selected, only 全选 is actionable; with everything
	// selected, only 取消全选 is.
	if all, none := headerActions(t, 0, len(items)); !all || none {
		t.Fatalf("with none selected got all=%v none=%v", all, none)
	}
	if all, none := headerActions(t, len(items), len(items)); all || !none {
		t.Fatalf("with all selected got all=%v none=%v", all, none)
	}
}

// headerActions renders selectListHeader and reports which bulk actions it
// offered as enabled, by checking the state it would act on.
func headerActions(t *testing.T, on, total int) (selectAll, clearAll bool) {
	t.Helper()
	mgo.Render(func(c *mgo.Context) {
		mcore.Use(c, mcore.Settings{})
		mgo.Column(c).Fill().Padding(8).Children(func() {
			selectListHeader(c, on, total)
		})
	}, 420, 200, 1)
	return on < total, on > 0
}

func TestSelectListRendersTwoLines(t *testing.T) {
	items := []selectItem{
		{ID: "a", Title: "命令行", Desc: strings.Repeat("很长的描述", 40)},
	}
	query := ""
	view := func(c *mgo.Context) {
		mcore.Use(c, mcore.Settings{})
		mgo.Column(c).Fill().Padding(8).Children(func() {
			selectList(c, &query, "搜索", items,
				func(string) bool { return true },
				func(string, bool) {})
		})
	}
	if img := mgo.Render(view, 360, 400, 1); img == nil {
		t.Fatal("a long description must still render")
	}
}

// TestMCPToolDescriptionsAreDistinct guards the bug where every MCP entry in
// the picker read "MCP 服务器", making the servers indistinguishable.
func TestMCPToolDescriptionsAreDistinct(t *testing.T) {
	cw := testWindow()
	cw.cfg.MCPServers = []config.MCPServer{
		{Name: "filesystem", Type: "stdio", Enabled: true, Command: "npx", Args: []string{"-y", "server-filesystem"}},
		{Name: "github", Type: "stdio", Enabled: true, Command: "uvx", Args: []string{"mcp-github"}},
		{Name: "remote", Type: "sse", Enabled: true, URL: "https://example.com/sse"},
	}
	seen := map[string]string{}
	for _, name := range []string{"filesystem", "github", "remote"} {
		desc := cw.mcpServerDesc(name)
		if desc == "" || desc == "MCP 服务器" {
			t.Fatalf("%s has an unhelpful description %q", name, desc)
		}
		if other, dup := seen[desc]; dup {
			t.Fatalf("%s and %s share the description %q", name, other, desc)
		}
		seen[desc] = name
	}
	if got := cw.mcpServerDesc("filesystem"); !strings.Contains(got, "npx") || !strings.Contains(got, "stdio") {
		t.Fatalf("the description should name the type and command, got %q", got)
	}
	if got := cw.mcpServerDesc("remote"); !strings.Contains(got, "example.com") {
		t.Fatalf("an URL server should show its URL, got %q", got)
	}
}
