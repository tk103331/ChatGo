// Package mcp provides MCP (Model Context Protocol) client management
package mcp

import (
	"chatgo/internal/config"
	"context"
	"fmt"
	"sync"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// MCPServerStatus represents the initialization status of an MCP server
type MCPServerStatus struct {
	Name     string
	Type     config.MCPServerType
	Status   string // "initialized", "error", "disconnected"
	Error    error
	Tools    []MCPTool
	Client   *client.Client
}

// MCPTool represents a tool from an MCP server
type MCPTool struct {
	Name        string
	Description string
	InputSchema map[string]interface{}
}

// Manager manages MCP client connections and tools
type Manager struct {
	servers map[string]*MCPServerStatus
	mu      sync.RWMutex
}

// NewManager creates a new MCP manager
func NewManager() *Manager {
	return &Manager{
		servers: make(map[string]*MCPServerStatus),
	}
}

// InitializeServer initializes a single MCP server connection
func (m *Manager) InitializeServer(cfg config.MCPServer) (*MCPServerStatus, error) {
	fmt.Printf("[MCP] Initializing server '%s' (type: %s)\n", cfg.Name, cfg.Type)

	// First check if already initialized (with read lock to avoid blocking)
	m.mu.RLock()
	if existing, ok := m.servers[cfg.Name]; ok && existing.Status == "initialized" {
		m.mu.RUnlock()
		fmt.Printf("[MCP] Server '%s' already initialized\n", cfg.Name)
		return existing, nil
	}
	m.mu.RUnlock()

	status := &MCPServerStatus{
		Name:   cfg.Name,
		Type:   cfg.Type,
		Status: "disconnected",
	}

	var mcpClient *client.Client
	var err error

	// Create client (outside of lock to avoid blocking other operations)
	switch cfg.Type {
	case config.MCPServerTypeStdIO:
		argsStr := ""
		if len(cfg.Args) > 0 {
			argsStr = " " + fmt.Sprintf("%v", cfg.Args)
		}
		fmt.Printf("[MCP] Type: StdIO\n")
		fmt.Printf("[MCP]   Command: %s%s\n", cfg.Command, argsStr)
		if len(cfg.Env) > 0 {
			fmt.Printf("[MCP]   Env: %v\n", cfg.Env)
		}

		// Convert env map to []string
		env := []string{}
		for k, v := range cfg.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}

		// Initialize stdio client
		// Note: NewStdioMCPClient automatically starts the connection internally
		mcpClient, err = client.NewStdioMCPClient(cfg.Command, env, cfg.Args...)
		if err != nil {
			fmt.Printf("[MCP] Failed to create stdio client: %v\n", err)
			status.Status = "error"
			status.Error = fmt.Errorf("failed to create stdio client: %w", err)
			m.setStatus(cfg.Name, status)
			return status, status.Error
		}
		fmt.Printf("[MCP] Stdio client created successfully\n")

	case config.MCPServerTypeSSE:
		fmt.Printf("[MCP] Type: SSE\n")
		fmt.Printf("[MCP]   URL: %s\n", cfg.URL)
		if len(cfg.Headers) > 0 {
			fmt.Printf("[MCP]   Headers: %v\n", cfg.Headers)
		}

		// Initialize SSE client
		mcpClient, err = client.NewSSEMCPClient(cfg.URL)
		if err != nil {
			fmt.Printf("[MCP] Failed to create SSE client: %v\n", err)
			status.Status = "error"
			status.Error = fmt.Errorf("failed to create SSE client: %w", err)
			m.setStatus(cfg.Name, status)
			return status, status.Error
		}
		fmt.Printf("[MCP] SSE client created successfully\n")

	case config.MCPServerTypeStreamableHTTP:
		fmt.Printf("[MCP] Type: StreamableHTTP\n")
		fmt.Printf("[MCP]   URL: %s\n", cfg.URL)
		if len(cfg.Headers) > 0 {
			fmt.Printf("[MCP]   Headers: %v\n", cfg.Headers)
		}
		if cfg.TimeoutSeconds > 0 {
			fmt.Printf("[MCP]   Timeout: %d seconds\n", cfg.TimeoutSeconds)
		}

		// Initialize streamable HTTP client
		mcpClient, err = client.NewStreamableHttpClient(cfg.URL)
		if err != nil {
			fmt.Printf("[MCP] Failed to create HTTP stream client: %v\n", err)
			status.Status = "error"
			status.Error = fmt.Errorf("failed to create HTTP stream client: %w", err)
			m.setStatus(cfg.Name, status)
			return status, status.Error
		}
		fmt.Printf("[MCP] StreamableHTTP client created successfully\n")

	default:
		fmt.Printf("[MCP] Unsupported MCP server type: %s\n", cfg.Type)
		status.Status = "error"
		status.Error = fmt.Errorf("unsupported MCP server type: %s", cfg.Type)
		m.setStatus(cfg.Name, status)
		return status, status.Error
	}

	status.Client = mcpClient

	// Start the client (only for SSE and StreamableHTTP types)
	// Note: Stdio client is already started by NewStdioMCPClient
	ctx := context.Background()
	if cfg.Type != config.MCPServerTypeStdIO {
		fmt.Printf("[MCP] Starting client connection...\n")
		err = mcpClient.Start(ctx)
		if err != nil {
			fmt.Printf("[MCP] Failed to start client: %v\n", err)
			status.Status = "error"
			status.Error = fmt.Errorf("failed to start MCP client: %w", err)
			m.setStatus(cfg.Name, status)
			return status, status.Error
		}
	}

	fmt.Printf("[MCP] Initializing MCP protocol handshake...\n")
	// Initialize the connection (outside of lock - this is a slow operation)
	initReq := mcp.InitializeRequest{}
	_, err = mcpClient.Initialize(ctx, initReq)
	if err != nil {
		fmt.Printf("[MCP] Failed to initialize MCP connection: %v\n", err)
		status.Status = "error"
		status.Error = fmt.Errorf("failed to initialize MCP connection: %w", err)
		m.setStatus(cfg.Name, status)
		mcpClient.Close()
		return status, status.Error
	}
	fmt.Printf("[MCP] MCP protocol handshake successful\n")

	// Get tools from the server (outside of lock - this is a slow operation)
	fmt.Printf("[MCP] Requesting tools list...\n")
	toolsReq := mcp.ListToolsRequest{}
	toolsResult, err := mcpClient.ListTools(ctx, toolsReq)
	if err != nil {
		fmt.Printf("[MCP] Failed to get tools: %v\n", err)
		status.Status = "error"
		status.Error = fmt.Errorf("failed to get tools: %w", err)
		m.setStatus(cfg.Name, status)
		mcpClient.Close()
		return status, status.Error
	}
	fmt.Printf("[MCP] Received %d tools\n", len(toolsResult.Tools))

	// Parse tools
	status.Tools = make([]MCPTool, 0, len(toolsResult.Tools))
	for _, tool := range toolsResult.Tools {
		mcpTool := MCPTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: map[string]interface{}{"inputSchema": tool.InputSchema},
		}
		status.Tools = append(status.Tools, mcpTool)
		fmt.Printf("[MCP]   - %s: %s\n", tool.Name, tool.Description)
	}

	status.Status = "initialized"
	status.Error = nil

	// Store the final status (with minimal time holding the lock)
	m.setStatus(cfg.Name, status)

	fmt.Printf("[MCP] Server '%s' initialization complete\n", cfg.Name)
	return status, nil
}

// setStatus stores the status of a server (helper to reduce lock holding time)
func (m *Manager) setStatus(name string, status *MCPServerStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.servers[name] = status
}

// InitializeAll initializes all enabled MCP servers
func (m *Manager) InitializeAll(servers []config.MCPServer) map[string]*MCPServerStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	results := make(map[string]*MCPServerStatus)

	for _, server := range servers {
		// Skip disabled servers
		if !server.Enabled {
			continue
		}

		status, err := m.InitializeServer(server)
		if err != nil {
			// Keep the error status but add to results
			results[server.Name] = status
		} else {
			results[server.Name] = status
		}
	}

	return results
}

// GetServerStatus returns the status of a specific server
func (m *Manager) GetServerStatus(name string) (*MCPServerStatus, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status, ok := m.servers[name]
	return status, ok
}

// GetAllStatus returns all server statuses
func (m *Manager) GetAllStatus() map[string]*MCPServerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to avoid concurrent modification
	result := make(map[string]*MCPServerStatus, len(m.servers))
	for k, v := range m.servers {
		result[k] = v
	}
	return result
}

// GetServerClient returns the MCP client for a specific server
func (m *Manager) GetServerClient(name string) (*client.Client, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if status, ok := m.servers[name]; ok && status.Status == "initialized" {
		return status.Client, true
	}
	return nil, false
}

// GetServerTools returns the tools for a specific server
func (m *Manager) GetServerTools(name string) ([]MCPTool, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if status, ok := m.servers[name]; ok && status.Status == "initialized" {
		return status.Tools, true
	}
	return nil, false
}

// GetAllTools returns all tools from all initialized servers
func (m *Manager) GetAllTools() map[string][]MCPTool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string][]MCPTool)
	for name, status := range m.servers {
		if status.Status == "initialized" {
			result[name] = status.Tools
		}
	}
	return result
}

// DisconnectServer disconnects a specific server
func (m *Manager) DisconnectServer(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if status, ok := m.servers[name]; ok {
		if status.Client != nil {
			err := status.Client.Close()
			status.Status = "disconnected"
			status.Client = nil
			status.Tools = nil
			status.Error = fmt.Errorf("disconnected")
			return err
		}
	}
	return fmt.Errorf("server not found")
}

// DisconnectAll disconnects all servers
func (m *Manager) DisconnectAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, status := range m.servers {
		if status.Client != nil {
			_ = status.Client.Close()
			status.Status = "disconnected"
			status.Client = nil
			status.Tools = nil
			status.Error = fmt.Errorf("disconnected")
		}
	}
}

// ReinitializeServer reinitializes a server (disconnects first if needed)
func (m *Manager) ReinitializeServer(cfg config.MCPServer) (*MCPServerStatus, error) {
	// Disconnect if exists
	_ = m.DisconnectServer(cfg.Name)

	// Reinitialize
	return m.InitializeServer(cfg)
}
