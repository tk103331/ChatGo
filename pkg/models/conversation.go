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
	ID        string                 `json:"id"`                 // Unique identifier for this tool call
	Name      string                 `json:"name"`               // Tool name
	Arguments string                 `json:"arguments"`          // Tool arguments as JSON string
	Result    string                 `json:"result"`             // Tool execution result
	Error     string                 `json:"error,omitempty"`    // Error message if tool call failed
	Metadata  map[string]interface{} `json:"metadata,omitempty"` // Additional metadata
}

// Message represents a single message in a conversation
type Message struct {
	ID        string     `json:"id"`
	Role      string     `json:"role"` // user, assistant, system
	Content   string     `json:"content"`
	Timestamp time.Time  `json:"timestamp"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"` // Tool calls made by this message
}

// Conversation represents a chat conversation
type Conversation struct {
	ID        string            `json:"id"`
	Title     string            `json:"title"`
	Messages  []Message         `json:"messages"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	Provider  string            `json:"provider"`
	Model     string            `json:"model"`
	Workspace string            `json:"workspace,omitempty"`
	Agent     string            `json:"agent,omitempty"`
	ACPMode   string            `json:"acp_mode,omitempty"`
	ACPConfig map[string]string `json:"acp_config,omitempty"`
	Skills    []string          `json:"skills,omitempty"`
	TaskID    string            `json:"task_id,omitempty"`
}

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
