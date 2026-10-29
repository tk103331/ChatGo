package ui

import (
	"testing"

	acp "github.com/coder/acp-go-sdk"

	"chatgo/pkg/models"
)

// TestACPPConfigRank checks the composer ordering: model first, then thought
// level, then mode, then anything else.
func TestACPPConfigRank(t *testing.T) {
	m := acp.SessionConfigOptionCategoryModel
	th := acp.SessionConfigOptionCategoryThoughtLevel
	mo := acp.SessionConfigOptionCategoryMode
	cases := []struct {
		cat  *acp.SessionConfigOptionCategory
		want int
	}{
		{&m, 0}, {&th, 1}, {&mo, 2}, {nil, 3},
	}
	for _, tc := range cases {
		if got := acpConfigRank(tc.cat); got != tc.want {
			t.Fatalf("category %v should rank %d, got %d", tc.cat, tc.want, got)
		}
	}
	if got := acpConfigName(0); got != "模型" {
		t.Fatalf("rank 0 should be 模型, got %q", got)
	}
}

// TestACPToolKind checks the adapter's tool category is read back from the
// call's metadata and named in Chinese, with unknown kinds degrading to a
// plain row.
func TestACPToolKind(t *testing.T) {
	tc := models.ToolCall{Metadata: map[string]interface{}{"acp_kind": "execute"}}
	if got := acpToolKind(tc); got != "execute" {
		t.Fatalf("kind should come from metadata, got %q", got)
	}
	if got := acpToolKindLabel("execute"); got != "执行" {
		t.Fatalf("execute should read as 执行, got %q", got)
	}
	if got := acpToolKindIcon("edit"); got != "pencil" {
		t.Fatalf("edit should use the pencil icon, got %q", got)
	}
	if got := acpToolKindLabel("unknown"); got != "" {
		t.Fatalf("an unknown kind should have no label, got %q", got)
	}
	if got := acpToolKindIcon("unknown"); got != "wrench" {
		t.Fatalf("an unknown kind should fall back to the wrench icon, got %q", got)
	}
	if got := acpToolKind(models.ToolCall{}); got != "" {
		t.Fatalf("a call without metadata has no kind, got %q", got)
	}
}

// TestACPShutdownClearsSessions checks the quit hook drops every slot, so a
// reap afterwards finds nothing and no CLI is left tracked.
func TestACPShutdownClearsSessions(t *testing.T) {
	cw := testWindow()
	rt1 := &acpConv{key: "c1", sessionID: "s1"}
	rt2 := &acpConv{key: "c2\x00acp:claude", sessionID: "s2"}
	cw.acp.convs["c1"] = rt1
	cw.acp.convs["c2\x00acp:claude"] = rt2
	cw.acp.convBySess["s1"] = "c1"
	cw.acp.convBySess["s2"] = "c2\x00acp:claude"

	cw.acpShutdown()

	if len(cw.acp.convs) != 0 {
		t.Fatalf("every session slot should be gone, got %d", len(cw.acp.convs))
	}
	if len(cw.acp.convBySess) != 0 {
		t.Fatalf("session ids should be dropped too, got %d", len(cw.acp.convBySess))
	}
}
