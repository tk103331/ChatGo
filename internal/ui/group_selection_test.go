package ui

import (
	"testing"

	"chatgo/internal/config"
)

// groupWindow has two providers and a round-robin group over both.
func groupWindow() *ChatWindow {
	cw := testWindow()
	cw.cfg.Providers = []config.Provider{
		{Name: "MiniMax", Type: config.ProviderTypeCustom, APIType: "openai", APIKey: "k",
			BaseURL: "https://a", Model: "MiniMax-M3", Models: []string{"MiniMax-M3"}, Enabled: true},
		{Name: "jdcloud", Type: config.ProviderTypeCustom, APIType: "openai", APIKey: "k",
			BaseURL: "https://b", Model: "DS-Flash", Models: []string{"DS-Flash"}, Enabled: true},
	}
	cw.cfg.ProviderGroups = []config.ProviderGroup{{
		Name: "group1", Strategy: config.StrategyRoundRobin, Enabled: true,
		Members: []config.GroupMember{
			{Provider: "MiniMax", Models: []string{"MiniMax-M3"}},
			{Provider: "jdcloud", Models: []string{"DS-Flash"}},
		},
	}}
	return cw
}

// TestGroupIsASingleModelEntry guards the bug where a group was flattened into
// one entry per member model: picking such an entry pinned the request to that
// model and bypassed the group's routing strategy.
func TestGroupIsASingleModelEntry(t *testing.T) {
	cw := groupWindow()
	refs := cw.allModelRefs()
	groupRefs := make([]string, 0, 1)
	for _, r := range refs {
		if name, _ := splitModelRef(r); name == "group1" {
			groupRefs = append(groupRefs, r)
		}
	}
	if len(groupRefs) != 1 {
		t.Fatalf("a group must be one entry, got %v", groupRefs)
	}
	if _, model := splitModelRef(groupRefs[0]); model != "" {
		t.Fatalf("a group entry must not name a member model, got %q", model)
	}
	if got := modelLabel(groupRefs[0]); got != "group1" {
		t.Fatalf("a group should be labelled by its name, got %q", got)
	}
	if !cw.isGroupRef(groupRefs[0]) {
		t.Fatal("isGroupRef should recognise the group entry")
	}
	// The providers themselves stay listed per model.
	if len(refs) != 3 {
		t.Fatalf("expected two provider models plus one group, got %v", refs)
	}
}

// TestSelectingGroupBuildsRoutingClient checks that choosing the group entry
// routes through the group, with no model pinned.
func TestSelectingGroupBuildsRoutingClient(t *testing.T) {
	cw := groupWindow()
	cw.setModelRef(groupRef("group1"))
	if cw.provider != "group1" {
		t.Fatalf("provider should be the group, got %q", cw.provider)
	}
	cw.setupProvider(cw.provider)
	if cw.groupClient == nil {
		t.Fatal("selecting a group must build the routing client")
	}
	if cw.llmClient != nil {
		t.Fatal("a group must not fall back to a single-provider client")
	}
	if cw.model != "" {
		t.Fatalf("a group must not pin a model, got %q", cw.model)
	}
	if got := cw.modelSetupError(); got != "" {
		t.Fatalf("unexpected setup error %q", got)
	}
}

// TestSwitchingFromGroupToProviderRestoresModel makes sure leaving a group
// gives the provider one of its own models again.
func TestSwitchingFromGroupToProviderRestoresModel(t *testing.T) {
	cw := groupWindow()
	cw.setModelRef(groupRef("group1"))
	cw.setupProvider(cw.provider)
	if cw.model != "" {
		t.Fatalf("the group should clear the model, got %q", cw.model)
	}

	cw.setModelRef(modelRef("MiniMax", "MiniMax-M3"))
	cw.setupProvider(cw.provider)
	if cw.provider != "MiniMax" || cw.model != "MiniMax-M3" {
		t.Fatalf("switching back got %q/%q", cw.provider, cw.model)
	}
	if cw.groupClient != nil {
		t.Fatal("the group client must be dropped when a provider is chosen")
	}
	if cw.llmClient == nil {
		t.Fatal("a provider should build a single client")
	}
}

// TestGroupWithoutModelsReportsSetupError covers a group whose members have no
// model ticked.
func TestGroupWithoutModelsReportsSetupError(t *testing.T) {
	cw := groupWindow()
	cw.cfg.ProviderGroups[0].Members = []config.GroupMember{{Provider: "MiniMax"}}
	cw.setModelRef(groupRef("group1"))
	cw.setupProvider(cw.provider)
	if cw.groupClient != nil {
		t.Fatal("an empty group must not build a client")
	}
	if cw.modelSetupError() == "" {
		t.Fatal("an empty group should explain itself")
	}
}

// TestGroupTurnReplayStaysOnGroup covers a conversation recorded on a group:
// replaying its turn must route through the group again, not through whichever
// member happened to answer.
func TestGroupTurnReplayStaysOnGroup(t *testing.T) {
	cw := groupWindow()
	cw.setModelRef(groupRef("group1"))
	cw.setupProvider(cw.provider)

	name, model := cw.defaultTarget()
	if name != "group1" {
		t.Fatalf("a new conversation should record the group, got %q", name)
	}
	if model != "" {
		t.Fatalf("a group turn must not record a member model, got %q", model)
	}

	// Switch away, then replay the group turn.
	cw.applyModel("MiniMax", "MiniMax-M3")
	if cw.groupClient != nil {
		t.Fatal("the group client should be gone after switching to a provider")
	}
	cw.applyModel(name, model)
	if cw.provider != "group1" {
		t.Fatalf("replay should restore the group, got %q", cw.provider)
	}
	if cw.groupClient == nil {
		t.Fatal("replaying a group turn must rebuild the routing client")
	}
}

// TestApplyModelToGroupClearsStaleModel covers switching from a provider to a
// group: the provider's model must not linger, or it would look like the group
// is pinned to it.
func TestApplyModelToGroupClearsStaleModel(t *testing.T) {
	cw := groupWindow()
	cw.applyModel("MiniMax", "MiniMax-M3")
	if cw.model != "MiniMax-M3" {
		t.Fatalf("setup failed, model=%q", cw.model)
	}
	cw.applyModel("group1", "")
	if cw.provider != "group1" {
		t.Fatalf("provider should be the group, got %q", cw.provider)
	}
	if cw.model != "" {
		t.Fatalf("the provider's model must be cleared, got %q", cw.model)
	}
	if cw.groupClient == nil {
		t.Fatal("the group routing client should be built")
	}
}

// TestAgentWhitelistAcceptsGroupEntry covers an Agent whose allowed models
// include a group: the task client must route through the group.
func TestAgentWhitelistAcceptsGroupEntry(t *testing.T) {
	cw := groupWindow()
	cw.cfg.Agents = []config.AgentDef{{
		Name:   "router",
		Models: []string{groupRef("group1")},
	}}
	provider, model, _, _, _ := cw.taskAgentSpec("agent:router")
	if provider != "group1" {
		t.Fatalf("the agent should target the group, got %q", provider)
	}
	if model != "" {
		t.Fatalf("a group entry carries no model, got %q", model)
	}
	client, react, groupClient, groupReact, err := cw.taskClient("agent:router", "")
	if err != nil {
		t.Fatal(err)
	}
	if groupClient == nil && groupReact == nil {
		t.Fatalf("a group agent should get a routing client, got client=%v react=%v", client != nil, react != nil)
	}
}

// TestGroupLabelNamesItsStrategy checks a group entry is shown with its
// routing strategy, so it is told apart from a plain provider.
func TestGroupLabelNamesItsStrategy(t *testing.T) {
	cw := groupWindow()
	ref := groupRef("group1")
	got := cw.refLabel(ref)
	if got != "group1 · 依次轮询" {
		t.Fatalf("a group should be labelled with its strategy, got %q", got)
	}

	// Each strategy has its own short name.
	for strategy, want := range map[string]string{
		config.StrategyFallback:   "group1 · 故障迁移",
		config.StrategyRandom:     "group1 · 随机访问",
		config.StrategyRoundRobin: "group1 · 依次轮询",
	} {
		cw.cfg.ProviderGroups[0].Strategy = strategy
		if got := cw.refLabel(ref); got != want {
			t.Fatalf("strategy %q labelled %q, want %q", strategy, got, want)
		}
	}
	// An empty strategy falls back, as EffectiveStrategy says.
	cw.cfg.ProviderGroups[0].Strategy = ""
	if got := cw.refLabel(ref); got != "group1 · 故障迁移" {
		t.Fatalf("the default strategy should show, got %q", got)
	}

	// A provider model keeps the plain "provider · model" label.
	if got := cw.refLabel(modelRef("MiniMax", "MiniMax-M3")); got != "MiniMax · MiniMax-M3" {
		t.Fatalf("a provider model label changed: %q", got)
	}
}

// TestGroupDetailListsMembers checks the group's detail line and chips name
// the providers it routes over.
func TestGroupDetailListsMembers(t *testing.T) {
	cw := groupWindow()
	desc, tags := cw.refDetail(groupRef("group1"))
	if desc != "模型组 · 2 个成员" {
		t.Fatalf("unexpected detail %q", desc)
	}
	if len(tags) != 2 || tags[0] != "MiniMax" || tags[1] != "jdcloud" {
		t.Fatalf("the chips should name the members, got %v", tags)
	}
	// A member with no model ticked does not count.
	cw.cfg.ProviderGroups[0].Members[1].Models = nil
	if desc, tags := cw.refDetail(groupRef("group1")); desc != "模型组 · 1 个成员" || len(tags) != 1 {
		t.Fatalf("an empty member should be excluded, got %q %v", desc, tags)
	}
	// A provider model is described by its provider.
	if desc, tags := cw.refDetail(modelRef("MiniMax", "MiniMax-M3")); desc != "MiniMax" || tags != nil {
		t.Fatalf("unexpected provider detail %q %v", desc, tags)
	}
}
