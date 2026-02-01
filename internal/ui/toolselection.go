// Package ui provides tool selection components for ChatGo application
package ui

import (
	"chatgo/internal/config"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// ToolSelection represents a selectable tool
type ToolSelection struct {
	ID          string // Unique identifier
	DisplayName string // Display name
	Group       string // Group name (e.g., "Built-in", server name)
	Type        string // "builtin" or "mcp"
	Enabled     bool   // Whether the tool is available
	Description string // Tool description
}

// ToolSelectionManager manages tool selection UI and state
type ToolSelectionManager struct {
	checkGroup *widget.CheckGroup
	button     *widget.Button
	config     *config.Config
	mcpManager *MCPManagerWrapper
	window     fyne.Window
}

// NewToolSelectionManager creates a new tool selection manager
func NewToolSelectionManager(cfg *config.Config, mcpManager *MCPManagerWrapper, window fyne.Window) *ToolSelectionManager {
	return &ToolSelectionManager{
		config:     cfg,
		mcpManager: mcpManager,
		window:     window,
	}
}

// LoadToolSelections returns all available tools organized by group
func (tm *ToolSelectionManager) LoadToolSelections() (builtinTools []ToolSelection, mcpTools map[string][]ToolSelection) {
	builtinTools = []ToolSelection{}
	mcpTools = make(map[string][]ToolSelection)

	// Add enabled built-in tools
	for _, tool := range tm.config.BuiltinTools {
		if tool.Enabled {
			builtinTools = append(builtinTools, ToolSelection{
				ID:          fmt.Sprintf("builtin:%s", tool.Name),
				DisplayName: tool.Name,
				Group:       "Built-in",
				Type:        "builtin",
				Enabled:     true,
				Description: config.GetBuiltinToolDescription(tool.Type),
			})
		}
	}

	// Add MCP tools from each server
	for _, server := range tm.config.MCPServers {
		serverType := string(server.Type)
		if serverType == "" {
			serverType = "stdio"
		}

		// Check if server is initialized
		status, ok := tm.mcpManager.manager.GetServerStatus(server.Name)

		if ok && status.Status == "initialized" && len(status.Tools) > 0 {
			serverTools := []ToolSelection{}
			for _, tool := range status.Tools {
				serverTools = append(serverTools, ToolSelection{
					ID:          fmt.Sprintf("mcp:%s:%s", server.Name, tool.Name),
					DisplayName: tool.Name,
					Group:       fmt.Sprintf("MCP [%s] - %s", serverType, server.Name),
					Type:        "mcp",
					Enabled:     true,
					Description: tool.Description,
				})
			}
			mcpTools[fmt.Sprintf("MCP [%s] - %s", serverType, server.Name)] = serverTools
		} else {
			// Server not initialized, add disabled entry
			// Check if server is enabled in config
			if server.Enabled {
				mcpTools[fmt.Sprintf("MCP [%s] - %s", serverType, server.Name)] = []ToolSelection{
					{
						ID:          fmt.Sprintf("mcp:%s:uninitialized", server.Name),
						DisplayName: fmt.Sprintf("(未初始化) %s", server.Name),
						Group:       fmt.Sprintf("MCP [%s] - %s", serverType, server.Name),
						Type:        "mcp",
						Enabled:     false,
						Description: "请先在设置中初始化此服务器",
					},
				}
			}
		}
	}

	return builtinTools, mcpTools
}

// LoadToolCheckGroup builds and returns the tool check group
func (tm *ToolSelectionManager) LoadToolCheckGroup() *widget.CheckGroup {
	// Load tools to get all available tool IDs
	builtinTools, mcpTools := tm.LoadToolSelections()

	// Flatten to list of IDs for CheckGroup
	toolOptions := []string{}

	for _, tool := range builtinTools {
		toolOptions = append(toolOptions, tool.ID)
	}

	for _, tools := range mcpTools {
		for _, tool := range tools {
			if tool.Enabled {
				toolOptions = append(toolOptions, tool.ID)
			}
		}
	}

	// Create check group with all tools selected by default
	toolCheckGroup := widget.NewCheckGroup(toolOptions, func(selected []string) {
		// Callback when selection changes
		tm.UpdateToolSelectButton(len(selected))
	})

	// Select all tools by default
	if len(toolOptions) > 0 {
		toolCheckGroup.SetSelected(toolOptions)
	}

	return toolCheckGroup
}

// SetCheckGroup sets the check group for this manager
func (tm *ToolSelectionManager) SetCheckGroup(checkGroup *widget.CheckGroup) {
	tm.checkGroup = checkGroup
}

// SetButton sets the button for this manager
func (tm *ToolSelectionManager) SetButton(button *widget.Button) {
	tm.button = button
}

// UpdateToolSelectButton updates the tool selection button text
func (tm *ToolSelectionManager) UpdateToolSelectButton(count int) {
	if tm.button == nil {
		return
	}

	if count == 0 {
		tm.button.SetText("选择工具 (0)")
	} else {
		tm.button.SetText(fmt.Sprintf("选择工具 (%d)", count))
	}
}

// GetSelectedTools returns the list of selected tools
func (tm *ToolSelectionManager) GetSelectedTools() []string {
	if tm.checkGroup == nil {
		return []string{}
	}
	return tm.checkGroup.Selected
}

// RefreshToolCheckGroup refreshes the tool check group with current configuration
func (tm *ToolSelectionManager) RefreshToolCheckGroup() {
	if tm.checkGroup == nil {
		return
	}

	// Get current selections
	currentSelections := tm.checkGroup.Selected
	currentSelectionsMap := make(map[string]bool)
	for _, sel := range currentSelections {
		currentSelectionsMap[sel] = true
	}

	// Reload tools to get all available tool IDs
	builtinTools, mcpTools := tm.LoadToolSelections()

	// Flatten to list of IDs for CheckGroup
	newToolOptions := []string{}

	for _, tool := range builtinTools {
		newToolOptions = append(newToolOptions, tool.ID)
	}

	for _, tools := range mcpTools {
		for _, tool := range tools {
			if tool.Enabled {
				newToolOptions = append(newToolOptions, tool.ID)
			}
		}
	}

	// Update options
	tm.checkGroup.Options = newToolOptions

	// Restore selections that still exist
	validSelections := []string{}
	for _, option := range newToolOptions {
		if currentSelectionsMap[option] {
			validSelections = append(validSelections, option)
		}
	}

	tm.checkGroup.SetSelected(validSelections)
	tm.checkGroup.Refresh()

	// Update button text
	tm.UpdateToolSelectButton(len(validSelections))
}

// ShowToolSelectionDialog displays a dialog for selecting tools with grouped Tree display
func (tm *ToolSelectionManager) ShowToolSelectionDialog() {
	if tm.checkGroup == nil {
		return
	}

	// Get current selections
	currentSelections := make(map[string]bool)
	for _, sel := range tm.checkGroup.Selected {
		currentSelections[sel] = true
	}

	// Load tools by group
	builtinTools, mcpTools := tm.LoadToolSelections()

	// Build tree data structure - using a simple string-based hierarchy
	// We'll use a simple map approach instead of complex struct

	// Build the tree hierarchy as a simple map
	treeHierarchy := make(map[string][]string)

	// Root level (empty string key) contains groups
	rootChildren := []string{}

	// Add built-in tools group
	if len(builtinTools) > 0 {
		groupID := "group:Built-in"
		rootChildren = append(rootChildren, groupID)

		// Add tools under this group
		toolChildren := []string{}
		for _, tool := range builtinTools {
			toolChildren = append(toolChildren, tool.ID)
		}
		treeHierarchy[groupID] = toolChildren
		fmt.Printf("[DEBUG] Group '%s' has %d tools\n", groupID, len(toolChildren))
	}

	// Add MCP server groups
	for groupName, tools := range mcpTools {
		groupID := "group:" + groupName
		rootChildren = append(rootChildren, groupID)

		// Add tools under this group
		toolChildren := []string{}
		for _, tool := range tools {
			toolChildren = append(toolChildren, tool.ID)
		}
		treeHierarchy[groupID] = toolChildren
	}

	// Set root children
	treeHierarchy[""] = rootChildren

	// We need to declare tree variable first so callbacks can reference it
	var tree *widget.Tree

	childUIDs := func(uid widget.TreeNodeID) []widget.TreeNodeID {
		uidStr := string(uid)

		if children, ok := treeHierarchy[uidStr]; ok {
			result := make([]widget.TreeNodeID, len(children))
			for i, child := range children {
				result[i] = widget.TreeNodeID(child)
			}
			return result
		}

		return []widget.TreeNodeID{}
	}

	isBranch := func(uid widget.TreeNodeID) bool {
		uidStr := string(uid)

		// Root is a branch
		if uidStr == "" {
			return true
		}

		// Groups (starts with "group:") are branches
		if strings.HasPrefix(uidStr, "group:") {
			return true
		}

		// Everything else (tools) are leaves
		return false
	}

	// Create a tool lookup map
	toolLookup := make(map[string]*ToolSelection)
	for _, tool := range builtinTools {
		toolLookup[tool.ID] = &tool
	}
	for _, tools := range mcpTools {
		for _, tool := range tools {
			toolLookup[tool.ID] = &tool
		}
	}

	// Create node function
	createNode := func(branch bool) fyne.CanvasObject {
		if branch {
			// Group node: checkbox + label with counts
			check := widget.NewCheck("", nil)
			label := widget.NewLabel("")
			label.TextStyle = fyne.TextStyle{Bold: true}
			return container.NewHBox(check, label)
		} else {
			// Tool node: checkbox + label + description
			// Use a simpler structure that's easier to update
			check := widget.NewCheck("", nil)
			nameLabel := widget.NewLabel("")
			nameLabel.TextStyle = fyne.TextStyle{Bold: true}
			descLabel := widget.NewLabel("")
			descLabel.Wrapping = fyne.TextWrapWord
			descLabel.TextStyle = fyne.TextStyle{Italic: true}
			// Store description label in a way we can access it
			vbox := container.NewVBox(
				container.NewHBox(check, nameLabel),
				descLabel,
			)
			return vbox
		}
	}

	// Update node function
	updateNode := func(uid widget.TreeNodeID, branch bool, obj fyne.CanvasObject) {
		uidStr := string(uid)

		if branch {
			// Group node
			cont := obj.(*fyne.Container)
			// For HBox container
			check := cont.Objects[0].(*widget.Check)
			label := cont.Objects[1].(*widget.Label)

			groupName := strings.TrimPrefix(uidStr, "group:")

			// Get tools for this group
			var tools []ToolSelection
			if groupName == "Built-in" {
				tools = builtinTools
			} else {
				tools = mcpTools[groupName]
			}

			// Count selected tools
			selectedCount := 0
			totalCount := 0
			for _, tool := range tools {
				if tool.Enabled {
					totalCount++
					if currentSelections[tool.ID] {
						selectedCount++
					}
				}
			}

			label.SetText(fmt.Sprintf("%s (%d/%d)", groupName, selectedCount, totalCount))

			// Update checkbox state
			allSelected := selectedCount > 0 && selectedCount == totalCount
			someSelected := selectedCount > 0 && selectedCount < totalCount

			check.Checked = allSelected
			if someSelected {
				// Visual indicator for partial selection
				label.TextStyle = fyne.TextStyle{Bold: true, Italic: true}
			} else {
				label.TextStyle = fyne.TextStyle{Bold: true}
			}

			// Set up checkbox callback
			check.OnChanged = func(checked bool) {
				// Select/deselect all tools in this group
				for _, tool := range tools {
					if tool.Enabled {
						if checked {
							currentSelections[tool.ID] = true
						} else {
							delete(currentSelections, tool.ID)
						}
					}
				}
				// Refresh the tree
				tree.Refresh()
			}
		} else {
			// Tool node
			tool, ok := toolLookup[uidStr]
			if !ok {
				return
			}

			vbox := obj.(*fyne.Container)
			if len(vbox.Objects) < 2 {
				return
			}

			// First object is HBox with checkbox and name
			checkContainer := vbox.Objects[0].(*fyne.Container)
			check := checkContainer.Objects[0].(*widget.Check)
			nameLabel := checkContainer.Objects[1].(*widget.Label)

			// Second object is description label
			descLabel := vbox.Objects[1].(*widget.Label)

			nameLabel.SetText(tool.DisplayName)
			descLabel.SetText(tool.Description)

			if !tool.Enabled {
				check.Hide()
				check.Disable()
				nameLabel.TextStyle = fyne.TextStyle{Italic: true}
				return
			}

			check.Show()
			check.Enable()
			nameLabel.TextStyle = fyne.TextStyle{Bold: true}

			checked := currentSelections[tool.ID]
			check.SetChecked(checked)

			check.OnChanged = func(checked bool) {
				if checked {
					currentSelections[uidStr] = true
				} else {
					delete(currentSelections, uidStr)
				}
				// Refresh parent group to update counts
				tree.RefreshItem(widget.TreeNodeID("group:" + tool.Group))
			}
		}
	}

	// Create tree
	tree = widget.NewTree(childUIDs, isBranch, createNode, updateNode)

	// Check if there are any tools to display
	if len(builtinTools) == 0 && len(mcpTools) == 0 {
		// Show a message that no tools are available
		noToolsLabel := widget.NewLabel("没有可用的工具。\n\n请确保：\n1. 在设置中启用至少一个内置工具\n2. MCP 服务器已成功初始化")
		noToolsLabel.Alignment = fyne.TextAlignCenter

		content := container.NewVBox(
			widget.NewLabel("选择要使用的工具:"),
			widget.NewSeparator(),
			container.NewPadded(noToolsLabel),
		)

		dialog.NewCustomConfirm("选择工具", "确定", "", content, func(bool) {}, tm.window).Show()
		return
	}

	// Create scroll container for tree with proper sizing
	treeScroll := container.NewScroll(tree)
	treeScroll.SetMinSize(fyne.NewSize(550, 400))
	treeScroll.Resize(fyne.NewSize(550, 400))

	// Set minimum size on tree itself
	tree.Resize(fyne.NewSize(550, 400))

	// Create content with proper layout
	titleLabel := widget.NewLabel("选择要使用的工具:")
	titleLabel.TextStyle = fyne.TextStyle{Bold: true}

	// Use VBox layout with scrollable tree
	content := container.NewVBox(
		titleLabel,
		widget.NewSeparator(),
		treeScroll,
	)

	// Show dialog
	d := dialog.NewCustomConfirm("选择工具", "确定", "取消", content, func(confirmed bool) {
		if confirmed {
			// Convert selections to list
			selections := make([]string, 0, len(currentSelections))
			for sel := range currentSelections {
				selections = append(selections, sel)
			}
			// Update the tool check group with selections
			tm.checkGroup.SetSelected(selections)
			tm.UpdateToolSelectButton(len(selections))
		} else {
			// Restore original selection
			tm.UpdateToolSelectButton(len(currentSelections))
		}
	}, tm.window)

	// Resize dialog to ensure content is visible
	d.Resize(fyne.NewSize(600, 500))

	// Show dialog
	d.Show()

	// Open branches after dialog is shown (still in main thread)
	// This should happen after Show() returns
	if len(builtinTools) > 0 {
		tree.OpenBranch("group:Built-in")
	}
	for groupName := range mcpTools {
		groupID := "group:" + groupName
		tree.OpenBranch(groupID)
	}
	tree.Refresh()
}
