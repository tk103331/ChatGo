package models

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// ToolCall represents a tool invocation
type ToolCall struct {
	ID         string                 `json:"id"`               // Unique identifier for this tool call
	Name       string                 `json:"name"`             // Tool name
	Arguments  string                 `json:"arguments"`        // Tool arguments as JSON string
	Result     string                 `json:"result"`           // Tool execution result
	Error      string                 `json:"error,omitempty"`  // Error message if tool call failed
	Status     string                 `json:"status,omitempty"` // Runner status: in_progress, completed, failed
	DurationMs int64                  `json:"duration_ms,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"` // Additional metadata
}

// Segment is one ordered piece of an assistant turn: a thinking block, a text
// block or a tool call. Rendering them in order keeps a multi-step ReAct run as
// thinking, tool, text, thinking, tool, ... instead of merging every block.
type Segment struct {
	Kind     string    `json:"kind"` // "reasoning", "text" or "tool"
	Text     string    `json:"text,omitempty"`
	ToolCall *ToolCall `json:"tool_call,omitempty"`
}

// Author kinds. A message's AuthorKind says what produced it, which a group
// conversation needs in order to tell a human apart from an assistant when
// several assistants share one conversation.
const (
	AuthorKindHuman  = "human"
	AuthorKindAgent  = "agent"
	AuthorKindSystem = "system"
)

// AuthorHuman is the AuthorID of the person using the app. Agents are
// identified by the conversation's own agent reference ("agent:Name",
// "acp:Name", "app:Name"), so the two namespaces never collide.
const AuthorHuman = "human"

// Message represents a single message in a conversation
type Message struct {
	ID        string     `json:"id"`
	Role      string     `json:"role"` // user, assistant, system
	Content   string     `json:"content"`
	Reasoning string     `json:"reasoning,omitempty"` // Model thinking process, rendered separately
	Segments  []Segment  `json:"segments,omitempty"`  // Ordered blocks for multi-step replies
	Timestamp time.Time  `json:"timestamp"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"` // Tool calls made by this message

	// AuthorID identifies who wrote this message: AuthorHuman for the person,
	// or an agent reference ("agent:Name", "acp:Name", "app:Name") for an
	// assistant. Role alone cannot carry this, because a group conversation
	// has several assistants and they all write with Role "assistant".
	//
	// Empty on messages written before authorship existed. Read it through
	// Author(), which falls back to Role so an old conversation keeps
	// rendering and keeps replaying exactly as it did.
	AuthorID string `json:"author_id,omitempty"`
	// AuthorName is the display name, shown on the bubble and prefixed onto
	// the content when a group conversation is sent to a model.
	AuthorName string `json:"author_name,omitempty"`
	// AuthorKind is one of the AuthorKind constants.
	AuthorKind string `json:"author_kind,omitempty"`

	// Seq orders messages within a conversation, monotonically and without
	// reuse. A group conversation needs a stable "which message is newer"
	// that survives edits and regeneration, which an index into Messages
	// cannot give: regenerating truncates the slice and reuses indices.
	//
	// Zero on messages written before sequencing existed. Conversation.NextSeq
	// is likewise zero there, so AssignSeq starts numbering from 1 on the
	// first new message and old messages simply sort before every new one.
	Seq int `json:"seq,omitempty"`

	// QuoteID is the message this one replies to, empty for a message that
	// quotes nothing. A group conversation interleaves several threads, so
	// a reply needs to name what it answers rather than rely on adjacency.
	QuoteID string `json:"quote_id,omitempty"`
}

// Author reports who wrote the message, falling back to Role for a message
// stored before authorship was recorded. A legacy assistant message returns
// an empty id: which agent wrote it was never recorded per-message, because
// the conversation held exactly one agent at the time.
func (m Message) Author() (id, kind string) {
	if m.AuthorID != "" {
		kind = m.AuthorKind
		if kind == "" {
			kind = legacyKindOf(m.Role)
		}
		return m.AuthorID, kind
	}
	return legacyAuthorOf(m.Role)
}

// IsHuman reports a message written by the person, by authorship when it is
// recorded and by Role otherwise.
func (m Message) IsHuman() bool {
	_, kind := m.Author()
	return kind == AuthorKindHuman
}

func legacyAuthorOf(role string) (id, kind string) {
	switch role {
	case "user":
		return AuthorHuman, AuthorKindHuman
	case "system":
		return "", AuthorKindSystem
	default:
		return "", AuthorKindAgent
	}
}

func legacyKindOf(role string) string {
	_, kind := legacyAuthorOf(role)
	return kind
}

// Turn is one request/reply round of a conversation: the user's input and the
// assistant's reply, plus the model and token usage that produced it. A Turn
// is the unit that owns model choice and usage, so a session may switch models
// between turns without losing how earlier turns were generated.
type Turn struct {
	ID         string    `json:"id"`
	UserMsgID  string    `json:"user_message_id"`
	ReplyMsgID string    `json:"reply_message_id"`
	User       string    `json:"user,omitempty"`
	Assistant  string    `json:"assistant,omitempty"`
	Provider   string    `json:"provider,omitempty"`
	Model      string    `json:"model,omitempty"`
	Input      int       `json:"input_tokens,omitempty"`
	Output     int       `json:"output_tokens,omitempty"`
	Total      int       `json:"total_tokens,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at,omitempty"`
}

// Conversation represents a chat conversation
type Conversation struct {
	ID        string            `json:"id"`
	Title     string            `json:"title"`
	Messages  []Message         `json:"messages"`
	Turns     []Turn            `json:"turns,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	Provider  string            `json:"provider"`
	Model     string            `json:"model"`
	Workspace string            `json:"workspace,omitempty"`
	Agent     string            `json:"agent,omitempty"`
	ACPMode   string            `json:"acp_mode,omitempty"`
	ACPConfig map[string]string `json:"acp_config,omitempty"`
	// Plan is the agent's current execution plan. An ACP agent replaces it
	// whole on every update; a conversation that never planned leaves it nil.
	Plan   []PlanEntry `json:"plan,omitempty"`
	Skills []string    `json:"skills,omitempty"`
	TaskID string      `json:"task_id,omitempty"`
	// TaskStatus is the last auto-task run's outcome on this conversation:
	// "success" or "failed", empty for a conversation without a run.
	TaskStatus string `json:"task_status,omitempty"`

	// NextSeq is the sequence number AssignSeq hands to the next message.
	// Zero on a conversation stored before sequencing existed, which makes
	// numbering start at 1 and sorts every legacy message before every new
	// one.
	NextSeq int `json:"next_seq,omitempty"`

	// RoomID names the group room this conversation belongs to, empty for a
	// one-to-one conversation. A room conversation is listed on the room's
	// own page, never in the chat session list, the same way a scheduled
	// task's conversation is kept out of it by TaskID.
	RoomID string `json:"room_id,omitempty"`
}

// IsRoom reports a conversation that belongs to a group room.
func (c *Conversation) IsRoom() bool { return c.RoomID != "" }

// AssignSeq stamps a message with the conversation's next sequence number.
// Numbers are never reused, so truncating Messages for a regeneration cannot
// make a new message compare as older than one it replaced.
func (c *Conversation) AssignSeq(m *Message) {
	if c.NextSeq < 1 {
		c.NextSeq = 1
	}
	m.Seq = c.NextSeq
	c.NextSeq++
}

// NewHumanMessage builds a sequenced message authored by the person.
func (c *Conversation) NewHumanMessage(id, content string, at time.Time) Message {
	m := Message{
		ID: id, Role: "user", Content: content, Timestamp: at,
		AuthorID: AuthorHuman, AuthorKind: AuthorKindHuman,
	}
	c.AssignSeq(&m)
	return m
}

// NewAgentMessage builds a sequenced, empty reply attributed to agentID, the
// conversation-agent reference ("agent:Name", "acp:Name", "app:Name", or ""
// for the built-in chat). name is the display name shown on the bubble.
func (c *Conversation) NewAgentMessage(id, agentID, name string, at time.Time) Message {
	m := Message{
		ID: id, Role: "assistant", Timestamp: at,
		AuthorID: agentID, AuthorName: name, AuthorKind: AuthorKindAgent,
	}
	c.AssignSeq(&m)
	return m
}

// PlanEntry is one task in an agent's execution plan, as ACP defines it.
// The agent sends the whole list on every update, so the client replaces the
// plan rather than merging into it.
type PlanEntry struct {
	Content  string `json:"content"`
	Priority string `json:"priority,omitempty"`
	Status   string `json:"status"`
}

// Plan entry statuses.
const (
	PlanPending    = "pending"
	PlanInProgress = "in_progress"
	PlanCompleted  = "completed"
)

// ConversationManager manages conversation storage
type ConversationManager struct {
	dataDir string
}

// NewConversationManager creates a new conversation manager
func NewConversationManager() (*ConversationManager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	chatgoDir := filepath.Join(homeDir, ".chatgo", "conversations")
	if err := os.MkdirAll(chatgoDir, 0755); err != nil {
		return nil, err
	}

	return &ConversationManager{dataDir: chatgoDir}, nil
}

// NewConversationManagerAt creates a conversation manager rooted at dir, for
// callers that supply their own storage location (tests, alternate profiles).
func NewConversationManagerAt(dir string) (*ConversationManager, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return &ConversationManager{dataDir: dir}, nil
}

// ListConversations returns all conversations
func (cm *ConversationManager) ListConversations() ([]Conversation, error) {
	entries, err := os.ReadDir(cm.dataDir)
	if err != nil {
		return nil, err
	}

	var conversations []Conversation
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		data, err := os.ReadFile(filepath.Join(cm.dataDir, entry.Name()))
		if err != nil {
			continue
		}

		var conv Conversation
		if err := json.Unmarshal(data, &conv); err != nil {
			continue
		}

		conversations = append(conversations, conv)
	}

	return conversations, nil
}

// LoadConversation loads a conversation by ID
func (cm *ConversationManager) LoadConversation(id string) (*Conversation, error) {
	data, err := os.ReadFile(filepath.Join(cm.dataDir, id+".json"))
	if err != nil {
		return nil, err
	}

	var conv Conversation
	if err := json.Unmarshal(data, &conv); err != nil {
		return nil, err
	}

	return &conv, nil
}

// SaveConversation saves a conversation
func (cm *ConversationManager) SaveConversation(conv *Conversation) error {
	conv.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(conv, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(cm.dataDir, conv.ID+".json"), data, 0644)
}

// DeleteConversation deletes a conversation
func (cm *ConversationManager) DeleteConversation(id string) error {
	return os.Remove(filepath.Join(cm.dataDir, id+".json"))
}

// CreateConversation creates a new conversation
func (cm *ConversationManager) CreateConversation(title, provider, model string) (*Conversation, error) {
	conv := &Conversation{
		ID:        generateID(),
		Title:     title,
		Messages:  []Message{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Provider:  provider,
		Model:     model,
	}

	if err := cm.SaveConversation(conv); err != nil {
		return nil, err
	}

	return conv, nil
}

func generateID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().Format("20060102150405.000000000")
	}
	return time.Now().Format("20060102150405") + "-" + hex.EncodeToString(b[:])
}
