// Package ui provides tool selection components for ChatGo application
package ui

import (
	"chatgo/internal/config"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
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

	fmt.Printf("[DEBUG] ShowToolSelectionDialog: builtinTools=%d, mcpTools=%d\n", len(builtinTools), len(mcpTools))

	// Build tree data structure
	// Root node ID: "root"
	// Group nodes: "group:Built-in", "group:MCP [stdio] - server1", etc.
	// Tool nodes: actual tool IDs

	type ToolNode struct {
		ID       string
		IsBranch bool
		Tool     *ToolSelection
		Children []string
	}

	treeData := make(map[string]*ToolNode)

	// We need to declare tree variable first so callbacks can reference it
	var tree *widget.Tree

	childUIDs := func(uid widget.TreeNodeID) []widget.TreeNodeID {
		uidStr := string(uid)
		fmt.Printf("[DEBUG] childUIDs called for: %s\n", uidStr)

		if uidStr == "root" {
			// Return group IDs
			groups := []widget.TreeNodeID{}
			if len(builtinTools) > 0 {
				groups = append(groups, "group:Built-in")
				fmt.Printf("[DEBUG] Adding Built-in group\n")
			}
			for groupName := range mcpTools {
				groups = append(groups, widget.TreeNodeID("group:"+groupName))
				fmt.Printf("[DEBUG] Adding MCP group: %s\n", groupName)
			}
			fmt.Printf("[DEBUG] Root has %d children\n", len(groups))
			return groups
		}

		node, ok := treeData[uidStr]
		if !ok {
			fmt.Printf("[DEBUG] Node not found: %s\n", uidStr)
			return []widget.TreeNodeID{}
		}

		if !node.IsBranch {
			fmt.Printf("[DEBUG] Node is not a branch: %s\n", uidStr)
			return []widget.TreeNodeID{}
		}

		result := make([]widget.TreeNodeID, len(node.Children))
		for i, child := range node.Children {
			result[i] = widget.TreeNodeID(child)
		}
		fmt.Printf("[DEBUG] Branch %s has %d children\n", uidStr, len(result))
		return result
	}

	// Create root
	treeData["root"] = &ToolNode{
		ID:       "root",
		IsBranch: true,
		Children: []string{},
	}

	// Create built-in tools group
	builtinGroupID := "group:Built-in"
	builtinToolIDs := []string{}
	for _, tool := range builtinTools {
		builtinToolIDs = append(builtinToolIDs, tool.ID)
		treeData[tool.ID] = &ToolNode{
			ID:       tool.ID,
			IsBranch: false,
			Tool:     &tool,
			Children: []string{},
		}
		fmt.Printf("[DEBUG] Added builtin tool: %s\n", tool.ID)
	}
	if len(builtinToolIDs) > 0 {
		treeData[builtinGroupID] = &ToolNode{
			ID:       builtinGroupID,
			IsBranch: true,
			Children: builtinToolIDs,
		}
		treeData["root"].Children = append(treeData["root"].Children, builtinGroupID)
	}

	// Create MCP server groups
	for groupName, tools := range mcpTools {
		groupID := "group:" + groupName
		toolIDs := []string{}
		for _, tool := range tools {
			toolIDs = append(toolIDs, tool.ID)
			treeData[tool.ID] = &ToolNode{
				ID:       tool.ID,
				IsBranch: false,
				Tool:     &tool,
				Children: []string{},
			}
			fmt.Printf("[DEBUG] Added MCP tool: %s\n", tool.ID)
		}
		treeData[groupID] = &ToolNode{
			ID:       groupID,
			IsBranch: true,
			Children: toolIDs,
		}
		treeData["root"].Children = append(treeData["root"].Children, groupID)
	}

	isBranch := func(uid widget.TreeNodeID) bool {
		if node, ok := treeData[string(uid)]; ok {
			return node.IsBranch
		}
		return false
	}

	// Create node function
	createNode := func(branch bool) fyne.CanvasObject {
		if branch {
			// Group node: checkbox + label with counts
			check := widget.NewCheck("", nil)
			label := widget.NewLabel("")
			label.TextStyle = fyne.TextStyle{Bold: true}
			return container.NewBorder(nil, nil, check, label, layout.NewSpacer())
		} else {
			// Tool node: checkbox + label + description
			check := widget.NewCheck("", nil)
			nameLabel := widget.NewLabel("")
			nameLabel.TextStyle = fyne.TextStyle{Bold: true}
			descLabel := widget.NewLabel("")
			descLabel.Wrapping = fyne.TextWrapWord
			descLabel.TextStyle = fyne.TextStyle{Italic: true}
			return container.NewVBox(
				container.NewHBox(check, nameLabel),
				container.NewPadded(descLabel),
			)
		}
	}

	// Update node function
	updateNode := func(uid widget.TreeNodeID, branch bool, obj fyne.CanvasObject) {
		uidStr := string(uid)

		if branch {
			// Group node
			cont := obj.(*fyne.Container)
			// For border container: [left, top, right, bottom, center]
			check := cont.Objects[0].(*widget.Check)
			label := cont.Objects[2].(*widget.Label)

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
			toolNode := treeData[uidStr]
			if toolNode == nil || toolNode.Tool == nil {
				return
			}
			tool := toolNode.Tool

			vbox := obj.(*fyne.Container)
			checkContainer := vbox.Objects[0].(*fyne.Container)
			check := checkContainer.Objects[0].(*widget.Check)
			nameLabel := checkContainer.Objects[1].(*widget.Label)

			nameLabel.SetText(tool.DisplayName)

			if !tool.Enabled {
				check.Hide()
				nameLabel.TextStyle = fyne.TextStyle{Italic: true}
				if len(vbox.Objects) > 1 {
					pad := vbox.Objects[1].(*fyne.Container)
					descLabel := pad.Objects[0].(*widget.Label)
					descLabel.SetText(tool.Description)
				}
				return
			}

			check.Show()
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

			// Update description
			if len(vbox.Objects) > 1 {
				pad := vbox.Objects[1].(*fyne.Container)
				descLabel := pad.Objects[0].(*widget.Label)
				descLabel.SetText(tool.Description)
			}
		}
	}

	// Create tree
	tree = widget.NewTree(
		childUIDs,
		isBranch,
		createNode,
		updateNode,
	)

	// Open all branches by default
	tree.OpenBranch("root")
	if len(builtinTools) > 0 {
		tree.OpenBranch("group:Built-in")
	}
	for groupName := range mcpTools {
		tree.OpenBranch("group:" + groupName)
	}

	// Debug: print tree structure
	fmt.Printf("Tool Selection Dialog - Built-in tools: %d, MCP groups: %d\n", len(builtinTools), len(mcpTools))
	fmt.Printf("Root children: %v\n", childUIDs("root"))
	fmt.Printf("Total tree nodes: %d\n", len(treeData))

	// Create scroll container for tree with proper sizing
	treeScroll := container.NewScroll(tree)
	treeScroll.SetMinSize(fyne.NewSize(500, 350))

	// Create content with proper layout using Border to ensure tree fills space
	titleLabel := widget.NewLabel("选择要使用的工具:")
	titleLabel.TextStyle = fyne.TextStyle{Bold: true}

	// Use Border layout: title on top, tree fills the rest
	content := container.NewBorder(
		container.NewVBox(titleLabel, widget.NewSeparator()), // top
		nil,           // bottom
		nil,           // left
		nil,           // right
		tree,    // center (fills remaining space)
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
	d.Resize(fyne.NewSize(550, 450))

	// Show dialog and refresh to ensure proper rendering
	d.Show()

	// Force a refresh after showing to ensure tree renders
	tree.Refresh()
}
