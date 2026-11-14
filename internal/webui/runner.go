package webui

import (
	"context"

	"chatgo/pkg/models"
)

// Capabilities names the features the embedded runtime can offer. A server
// without a Runner (the standalone webd) reports none of them and falls back
// to a plain chat client.
const (
	CapAgents      = "agents"
	CapTools       = "tools"
	CapACP         = "acp"
	CapAsk         = "ask"
	CapPermission  = "permission"
	CapCheckpoints = "checkpoints"
	CapAttachments = "attachments"
	CapSkills      = "skills"
	CapRooms       = "rooms"
	CapTasks       = "tasks"
)

// Runner executes a turn with the desktop's full agent runtime. The embedded
// server passes the ChatWindow, so tools, MCP servers, custom agents and ACP
// connections behave exactly as they do in the app. A nil Runner means the
// standalone server, which only runs a plain chat model.
type Runner interface {
	// Capabilities reports the features the runtime supports.
	Capabilities() map[string]bool
	// Run streams a reply for conv into the placeholder at replyIdx. All
	// conversation changes go through live.Update, so the browser polls a
	// consistent snapshot.
	Run(ctx context.Context, conv *models.Conversation, replyIdx int, req RunRequest, live Live) error
	// Agents lists the assistants a web session may use.
	Agents() []AgentInfo
	// Tools lists the built-in tools and MCP servers a session may enable,
	// with the defaults an agent reference starts from.
	Tools(agent string) []ToolInfo
	// Workspaces lists the directories a new session may use.
	Workspaces() []string
	// Projects lists the projects and their worktrees a new session may run
	// in, the way the desktop's 新会话 page picks one.
	Projects() []ProjectInfo
	// Models lists the models an agent reference may run, with their labels.
	// An ACP reference returns nil: it brings its own runtime.
	Models(agent string) []ModelInfo
	// ACPOptions lists the ACP connection's session config options — the
	// model, the modes, the toggles — with the values each one offers. Empty
	// when the CLI offers none or the warm-up has not landed yet.
	ACPOptions(agent string) []ACPConfigOptionInfo
	// SessionACPOptions lists the config options of a live session, warming
	// the session in first when it has not started yet.
	SessionACPOptions(convID string) []ACPConfigOptionInfo
	// SetSessionConfig applies one config choice to a live session and
	// reports the CLI's refusal, so the caller can roll its control back.
	SetSessionConfig(convID, configID, value string) error
	// CreateWorktree adds a working tree of a project on branch and returns
	// its path. newBranch creates the branch first.
	CreateWorktree(project, branch string, newBranch bool) (string, error)
	// Skills lists the skills available to an agent reference.
	Skills(agent string) []SkillInfo
	// Revert rewinds the conversation workspace to the checkpoint of a reply.
	Revert(ctx context.Context, conv *models.Conversation, messageID string) error
}

// RunRequest is what the browser asks a turn to run with. Explicit values win
// over the conversation's own fields.
type RunRequest struct {
	Provider  string   `json:"provider,omitempty"`
	Model     string   `json:"model,omitempty"`
	Agent     string   `json:"agent,omitempty"`
	Workspace string   `json:"workspace,omitempty"`
	Tools     []string `json:"tools,omitempty"`
	Skills    []string `json:"skills,omitempty"`
}

// Live is one running turn's channel to the browser: it applies changes to the
// conversation the browser polls, and asks the user through the session's
// pending interaction.
type Live interface {
	// Update changes the live conversation under the run's lock.
	Update(func(*models.Conversation))
	// Ask blocks until the browser answers the questions or ctx ends.
	Ask(ctx context.Context, items []AskItem) (string, error)
	// Permission blocks until the browser chooses an ACP permission option.
	Permission(ctx context.Context, req PermissionRequest) (PermissionDecision, error)
}

// AskItem is one question an agent asks.
type AskItem struct {
	Question string   `json:"question"`
	Options  []string `json:"options,omitempty"`
}

// PermissionRequest is one ACP permission prompt.
type PermissionRequest struct {
	Title   string             `json:"title"`
	Detail  string             `json:"detail,omitempty"`
	Options []PermissionOption `json:"options"`
}

// PermissionOption is one choice of a permission prompt.
type PermissionOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind,omitempty"`
}

// PermissionDecision is the chosen option, or a cancel.
type PermissionDecision struct {
	OptionID string `json:"option_id,omitempty"`
	Canceled bool   `json:"canceled,omitempty"`
}

// AgentInfo is one assistant a session can run.
type AgentInfo struct {
	Ref         string   `json:"ref"` // "", "agent:Name" or "acp:Name"
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Kind        string   `json:"kind"` // chat, agent or acp
	Disabled    bool     `json:"disabled,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	Models      []string `json:"models,omitempty"`
	Tools       []string `json:"tools,omitempty"`
	Workspace   string   `json:"workspace,omitempty"`
	Skills      []string `json:"skills,omitempty"`
}

// ToolInfo is one built-in tool or MCP server a session can enable.
type ToolInfo struct {
	ID          string `json:"id"` // "builtin:Name" or "mcp:Name"
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Kind        string `json:"kind"` // builtin or mcp
	Enabled     bool   `json:"enabled"`
}

// SkillInfo is one skill the slash menu can offer.
type SkillInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Token       string `json:"token"`
	Description string `json:"description,omitempty"`
}

// TreeInfo is one working tree of a project.
type TreeInfo struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
}

// ProjectInfo is one project a new session may run in: its main working
// tree, every worktree with the branch it has checked out, and the local
// branch list for creating a new tree.
type ProjectInfo struct {
	Main      string     `json:"main"`
	Worktrees []TreeInfo `json:"worktrees,omitempty"`
	Branches  []string   `json:"branches,omitempty"`
}

// ModelInfo is one model a session may run.
type ModelInfo struct {
	// Ref is a "Provider::Model" or "::Model" reference, the same shape the
	// run request's provider and model split into.
	Ref string `json:"ref"`
	// Label is the model's own name, without its provider prefix.
	Label string `json:"label"`
	// Provider names the model's provider and ProviderType carries its
	// protocol, which the picker draws as the provider's brand mark.
	Provider     string `json:"provider,omitempty"`
	ProviderType string `json:"provider_type,omitempty"`
}

// ACPConfigOptionInfo is one session config option an ACP agent's runtime
// exposes, with the values it can take.
type ACPConfigOptionInfo struct {
	ID      string           `json:"id"`
	Name    string           `json:"name"`
	Current string           `json:"current"`
	Kind    string           `json:"kind,omitempty"`
	Options []ACPConfigValue `json:"options,omitempty"`
}

// ACPConfigValue is one value a config option can take.
type ACPConfigValue struct {
	Value string `json:"value"`
	Name  string `json:"name"`
}

// InteractionInfo is the question or permission a running turn waits on.
type InteractionInfo struct {
	ID         string             `json:"id"`
	Kind       string             `json:"kind"` // ask or permission
	Items      []AskItem          `json:"items,omitempty"`
	Permission *PermissionRequest `json:"permission,omitempty"`
}
