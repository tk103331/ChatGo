package mcp

import (
	"chatgo/internal/config"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// Server represents an MCP server connection
type Server struct {
	config    config.MCPServer
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.Reader
	running   bool
	mu        sync.Mutex
}

// Manager manages MCP servers
type Manager struct {
	servers map[string]*Server
	mu      sync.RWMutex
}

// NewManager creates a new MCP manager
func NewManager() *Manager {
	return &Manager{
		servers: make(map[string]*Server),
	}
}

// StartServer starts an MCP server
func (m *Manager) StartServer(serverConfig config.MCPServer) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.servers[serverConfig.Name]; exists {
		return fmt.Errorf("server %s already running", serverConfig.Name)
	}

	cmd := exec.Command(serverConfig.Command, serverConfig.Args...)

	// Set environment variables
	if len(serverConfig.Env) > 0 {
		env := []string{}
		for k, v := range serverConfig.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		cmd.Env = env
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	server := &Server{
		config:  serverConfig,
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		running: true,
	}

	m.servers[serverConfig.Name] = server

	go func() {
		cmd.Wait()
		m.mu.Lock()
		delete(m.servers, serverConfig.Name)
		m.mu.Unlock()
	}()

	return nil
}

// StopServer stops an MCP server
func (m *Manager) StopServer(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	server, exists := m.servers[name]
	if !exists {
		return fmt.Errorf("server %s not found", name)
	}

	server.mu.Lock()
	server.running = false
	server.mu.Unlock()

	if err := server.stdin.Close(); err != nil {
		return fmt.Errorf("failed to close stdin: %w", err)
	}

	if err := server.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("failed to kill process: %w", err)
	}

	delete(m.servers, name)

	return nil
}

// CallTool calls a tool on an MCP server
func (m *Manager) CallTool(serverName, toolName string, args map[string]interface{}) (interface{}, error) {
	m.mu.RLock()
	server, exists := m.servers[serverName]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("server %s not found", serverName)
	}

	server.mu.Lock()
	defer server.mu.Unlock()

	if !server.running {
		return nil, fmt.Errorf("server %s is not running", serverName)
	}

	// Prepare JSON-RPC request
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": toolName,
			"arguments": args,
		},
	}

	// Send request
	encoder := json.NewEncoder(server.stdin)
	if err := encoder.Encode(request); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Read response
	decoder := json.NewDecoder(server.stdout)
	var response map[string]interface{}
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if err, exists := response["error"]; exists {
		return nil, fmt.Errorf("MCP error: %v", err)
	}

	return response["result"], nil
}

// ListTools lists available tools on an MCP server
func (m *Manager) ListTools(serverName string) ([]map[string]interface{}, error) {
	m.mu.RLock()
	server, exists := m.servers[serverName]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("server %s not found", serverName)
	}

	server.mu.Lock()
	defer server.mu.Unlock()

	if !server.running {
		return nil, fmt.Errorf("server %s is not running", serverName)
	}

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
	}

	encoder := json.NewEncoder(server.stdin)
	if err := encoder.Encode(request); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	decoder := json.NewDecoder(server.stdout)
	var response map[string]interface{}
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if err, exists := response["error"]; exists {
		return nil, fmt.Errorf("MCP error: %v", err)
	}

	result, ok := response["result"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}

	tools, ok := result["tools"].([]interface{})
	if !ok {
		return []map[string]interface{}{}, nil
	}

	var toolList []map[string]interface{}
	for _, tool := range tools {
		if t, ok := tool.(map[string]interface{}); ok {
			toolList = append(toolList, t)
		}
	}

	return toolList, nil
}

// StopAll stops all running servers
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name := range m.servers {
		m.StopServer(name)
	}
}
