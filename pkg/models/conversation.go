package models

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Message represents a single message in a conversation
type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"` // user, assistant, system
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// Conversation represents a chat conversation
type Conversation struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Messages    []Message `json:"messages"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Provider    string    `json:"provider"`
	Model       string    `json:"model"`
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
	return time.Now().Format("20060102150405")
}
