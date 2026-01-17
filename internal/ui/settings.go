package ui

import (
	"chatgo/internal/config"
	"chatgo/internal/mcp"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// showSettings displays the settings dialog with Providers, MCP Servers, and Built-in Tools tabs.
func (cw *ChatWindow) showSettings() {
	// Create tabs for Providers, MCP Servers, and Built-in Tools
	providersTab := cw.createProvidersTab(cw.window)
	mcpServersTab := cw.createMCPServersTab(cw.window)
	builtinToolsTab := cw.createBuiltinToolsTab(cw.window)

	tabs := container.NewAppTabs(
		container.NewTabItem("Providers", providersTab),
		container.NewTabItem("MCP Servers", mcpServersTab),
		container.NewTabItem("Built-in Tools", builtinToolsTab),
	)

	// Create close button for top-right corner
	closeBtn := widget.NewButtonWithIcon("", theme.CancelIcon(), func() {})

	// Create content with close button in top-right
	content := container.NewBorder(
		nil, // top
		nil, // bottom
		nil, // left
		closeBtn, // right
		tabs, // center
	)

	// Show as dialog without buttons
	d := dialog.NewCustomWithoutButtons("Settings", content, cw.window)

	// Hook up close button to hide dialog
	closeBtn.OnTapped = func() {
		// Update tool check group when settings close
		cw.toolSelectionMgr.RefreshToolCheckGroup()
		d.Hide()
	}

	d.Resize(fyne.NewSize(800, 500))
	d.Show()
}

// createBuiltinToolsTab creates the Built-in Tools configuration tab.
// It displays a list of configured built-in tools from Eino framework and allows adding, editing, and deleting them.

// createBuiltinToolsTab creates the Built-in Tools configuration tab.
// Tools cannot be added or removed, only enabled/disabled and configured.
func (cw *ChatWindow) createBuiltinToolsTab(parentWindow fyne.Window) fyne.CanvasObject {
	var selectedTool *config.BuiltinTool
	var selectedToolIndex int = -1
	enabledCheck := widget.NewCheck("Enabled", nil)
	configContainer := container.NewVBox()
	var configEntries []*widget.Entry
	var configFields []string

	recreateConfigFields := func(toolType string) {
		configContainer.Objects = nil
		configEntries = nil
		configFields = nil
		fields := config.GetBuiltinToolConfigFields(toolType)
		requiredFields := config.GetRequiredConfigFields(toolType)
		configFields = fields

		if len(fields) == 0 {
			configContainer.Add(widget.NewLabel("No additional configuration required for this tool type."))
			configContainer.Refresh()
			return
		}

		form := container.NewGridWithColumns(2)
		for _, field := range fields {
			labelText := field + ":"
			if contains(requiredFields, field) {
				labelText = field + " *:"
			}
			label := widget.NewLabel(labelText)
			entry := widget.NewEntry()
			if selectedTool != nil && selectedTool.Config != nil {
				if val, ok := selectedTool.Config[field]; ok {
					entry.SetText(val)
				}
			}
			configEntries = append(configEntries, entry)
			form.Add(label)
			form.Add(entry)
		}
		configContainer.Add(form)
		configContainer.Refresh()
	}

	toolList := widget.NewList(
		func() int { return len(cw.config.BuiltinTools) },
		func() fyne.CanvasObject {
			return container.NewHBox(widget.NewIcon(theme.ComputerIcon()), widget.NewLabel(""))
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			cont := obj.(*fyne.Container)
			label := cont.Objects[1].(*widget.Label)
			if id < len(cw.config.BuiltinTools) {
				tool := cw.config.BuiltinTools[id]
				status := "disabled"
				if tool.Enabled {
					status = "enabled"
				}
				label.SetText(fmt.Sprintf("%s - %s", tool.Type, status))
			}
		},
	)

	toolTypeLabel := widget.NewLabel("Tool Type:")
	descLabel := widget.NewLabel("(Select a tool from the list)")

	toolList.OnSelected = func(id widget.ListItemID) {
		if id >= 0 && id < len(cw.config.BuiltinTools) {
			selectedTool = &cw.config.BuiltinTools[id]
			selectedToolIndex = id
			enabledCheck.SetChecked(selectedTool.Enabled)
			toolTypeLabel.SetText(fmt.Sprintf("Tool Type: %s", selectedTool.Type))
			descLabel.SetText(config.GetBuiltinToolDescription(selectedTool.Type))
			recreateConfigFields(selectedTool.Type)
		}
	}

	toolList.OnUnselected = func(id widget.ListItemID) {
		if selectedToolIndex == id {
			selectedTool = nil
			selectedToolIndex = -1
			enabledCheck.SetChecked(false)
			toolTypeLabel.SetText("Tool Type:")
			descLabel.SetText("(Select a tool from the list)")
			configContainer.Objects = nil
			configContainer.Refresh()
		}
	}

	form := container.NewVBox(
		widget.NewLabel("Built-in Tool Configuration"),
		widget.NewSeparator(),
		toolTypeLabel,
		descLabel,
		widget.NewSeparator(),
		container.NewGridWithColumns(2, widget.NewLabel(""), enabledCheck, widget.NewLabel(""), widget.NewLabel("")),
		widget.NewSeparator(),
		widget.NewLabel("Tool Configuration:"),
		widget.NewLabel("* = Required field"),
		configContainer,
	)

	saveBtn := widget.NewButton("Save Configuration", func() {
		if selectedTool == nil {
			dialog.ShowError(fmt.Errorf("Please select a tool to save"), parentWindow)
			return
		}
		configMap := make(map[string]string)
		for i, entry := range configEntries {
			if i < len(configFields) {
				configMap[configFields[i]] = entry.Text
			}
		}
		selectedTool.Enabled = enabledCheck.Checked
		selectedTool.Config = configMap
		if selectedTool.Enabled {
			if err := config.ValidateBuiltinToolConfig(*selectedTool); err != nil {
				dialog.ShowError(fmt.Errorf("validation failed: %w", err), parentWindow)
				return
			}
		}
		config.SaveConfig(cw.config)
		toolList.Refresh()
		dialog.ShowInformation("Success", fmt.Sprintf("Configuration for '%s' has been saved.", selectedTool.Type), parentWindow)
	})

	rightPanel := container.NewBorder(nil, container.NewHBox(saveBtn), nil, nil, form)
	split := container.NewHSplit(toolList, rightPanel)
	split.SetOffset(0.4)
	return split
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
func (cw *ChatWindow) createProvidersTab(parentWindow fyne.Window) fyne.CanvasObject {
	// Track selected provider
	var selectedProvider *config.Provider
	var selectedProviderIndex int = -1

	// Create form entries
	nameEntry := widget.NewEntry()
	typeEntry := widget.NewSelect([]string{"openai", "anthropic", "claude", "ollama", "custom", "qwen", "deepseek", "gemini"}, nil)
	apiKeyEntry := widget.NewEntry()
	apiKeyEntry.Password = true
	baseURLEntry := widget.NewEntry()
	modelEntry := widget.NewEntry()
	enabledCheck := widget.NewCheck("Enabled", nil)

	// Provider list
	providerList := widget.NewList(
		func() int { return len(cw.config.Providers) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(theme.DocumentIcon()),
				widget.NewLabel(""),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			container := obj.(*fyne.Container)
			label := container.Objects[1].(*widget.Label)
			if id < len(cw.config.Providers) {
				provider := cw.config.Providers[id]
				status := "enabled"
				if !provider.Enabled {
					status = "disabled"
				}
				label.SetText(fmt.Sprintf("%s (%s) - %s", provider.Name, provider.Type, status))
			}
		},
	)

	providerList.OnSelected = func(id widget.ListItemID) {
		if id >= 0 && id < len(cw.config.Providers) {
			selectedProvider = &cw.config.Providers[id]
			selectedProviderIndex = id

			// Populate form
			nameEntry.SetText(selectedProvider.Name)
			typeEntry.SetSelected(selectedProvider.Type)
			apiKeyEntry.SetText(selectedProvider.APIKey)
			baseURLEntry.SetText(selectedProvider.BaseURL)
			modelEntry.SetText(selectedProvider.Model)
			enabledCheck.SetChecked(selectedProvider.Enabled)
		}
	}

	providerList.OnUnselected = func(id widget.ListItemID) {
		if selectedProviderIndex == id {
			selectedProvider = nil
			selectedProviderIndex = -1

			// Clear form
			nameEntry.SetText("")
			typeEntry.SetSelected("")
			apiKeyEntry.SetText("")
			baseURLEntry.SetText("")
			modelEntry.SetText("")
			enabledCheck.SetChecked(false)
		}
	}

	// Form
	form := container.NewVBox(
		widget.NewLabel("Provider Details"),
		widget.NewSeparator(),
		container.NewGridWithColumns(2,
			widget.NewLabel("Name:"), nameEntry,
			widget.NewLabel("Type:"), typeEntry,
			widget.NewLabel("API Key:"), apiKeyEntry,
			widget.NewLabel("Base URL:"), baseURLEntry,
			widget.NewLabel("Model:"), modelEntry,
			widget.NewLabel(""), enabledCheck,
		),
	)

	// Buttons
	addBtn := widget.NewButton("Add New", func() {
		// Clear form and deselect
		selectedProvider = nil
		selectedProviderIndex = -1
		providerList.UnselectAll()
		nameEntry.SetText("")
		typeEntry.SetSelected("")
		apiKeyEntry.SetText("")
		baseURLEntry.SetText("")
		modelEntry.SetText("")
		enabledCheck.SetChecked(true)
	})

	saveBtn := widget.NewButton("Save", func() {
		if nameEntry.Text == "" {
			dialog.ShowError(fmt.Errorf("Provider name cannot be empty"), parentWindow)
			return
		}
		if typeEntry.Selected == "" {
			dialog.ShowError(fmt.Errorf("Provider type must be selected"), parentWindow)
			return
		}

		newProvider := config.Provider{
			Name:    nameEntry.Text,
			Type:    typeEntry.Selected,
			APIKey:  apiKeyEntry.Text,
			BaseURL: baseURLEntry.Text,
			Model:   modelEntry.Text,
			Enabled: enabledCheck.Checked,
		}

		if selectedProvider != nil {
			// Update existing provider
			*selectedProvider = newProvider
		} else {
			// Add new provider
			cw.config.Providers = append(cw.config.Providers, newProvider)
			selectedProviderIndex = len(cw.config.Providers) - 1
			selectedProvider = &cw.config.Providers[selectedProviderIndex]
		}

		config.SaveConfig(cw.config)
		providerList.Refresh()
		cw.updateProviderSelector()

		// Select the updated/new provider
		providerList.Select(selectedProviderIndex)
	})

	deleteBtn := widget.NewButton("Delete", func() {
		if selectedProvider == nil {
			dialog.ShowError(fmt.Errorf("Please select a provider to delete"), parentWindow)
			return
		}

		dialog.ShowConfirm(
			"Delete Provider",
			fmt.Sprintf("Are you sure you want to delete provider '%s'?", selectedProvider.Name),
			func(confirmed bool) {
				if confirmed {
					// Remove provider
					cw.config.Providers = append(cw.config.Providers[:selectedProviderIndex], cw.config.Providers[selectedProviderIndex+1:]...)
					config.SaveConfig(cw.config)

					// Reset selection and clear form
					selectedProvider = nil
					selectedProviderIndex = -1
					nameEntry.SetText("")
					typeEntry.SetSelected("")
					apiKeyEntry.SetText("")
					baseURLEntry.SetText("")
					modelEntry.SetText("")
					enabledCheck.SetChecked(false)

					// Update UI
					providerList.Refresh()
					cw.updateProviderSelector()
				}
			},
			parentWindow,
		)
	})

	buttonContainer := container.NewHBox(addBtn, saveBtn, deleteBtn)

	// Right side container with form and buttons
	rightPanel := container.NewBorder(
		nil,
		buttonContainer,
		nil,
		nil,
		form,
	)

	// Split left and right
	split := container.NewHSplit(
		providerList,
		rightPanel,
	)
	split.SetOffset(0.4)

	return split
}

// showProviderDialog displays a dialog for adding or editing a provider.
func (cw *ChatWindow) showProviderDialog(settingsWin fyne.Window, provider *config.Provider, providerList *widget.List) {
	title := "Add Provider"
	if provider != nil {
		title = "Edit Provider"
	}

	nameEntry := widget.NewEntry()
	typeEntry := widget.NewSelect([]string{"openai", "anthropic", "claude", "ollama", "custom", "qwen", "deepseek", "gemini"}, nil)
	apiKeyEntry := widget.NewEntry()
	apiKeyEntry.Password = true
	baseURLEntry := widget.NewEntry()
	modelEntry := widget.NewEntry()
	enabledCheck := widget.NewCheck("Enabled", nil)

	if provider != nil {
		nameEntry.SetText(provider.Name)
		typeEntry.SetSelected(provider.Type)
		apiKeyEntry.SetText(provider.APIKey)
		baseURLEntry.SetText(provider.BaseURL)
		modelEntry.SetText(provider.Model)
		enabledCheck.SetChecked(provider.Enabled)
	} else {
		enabledCheck.SetChecked(true)
	}

	form := container.NewGridWithColumns(2,
		widget.NewLabel("Name:"), nameEntry,
		widget.NewLabel("Type:"), typeEntry,
		widget.NewLabel("API Key:"), apiKeyEntry,
		widget.NewLabel("Base URL:"), baseURLEntry,
		widget.NewLabel("Model:"), modelEntry,
		widget.NewLabel(""), enabledCheck,
	)

	saveBtn := widget.NewButton("Save", func() {
		if nameEntry.Text == "" {
			dialog.ShowError(fmt.Errorf("Provider name cannot be empty"), settingsWin)
			return
		}
		if typeEntry.Selected == "" {
			dialog.ShowError(fmt.Errorf("Provider type must be selected"), settingsWin)
			return
		}

		newProvider := config.Provider{
			Name:    nameEntry.Text,
			Type:    typeEntry.Selected,
			APIKey:  apiKeyEntry.Text,
			BaseURL: baseURLEntry.Text,
			Model:   modelEntry.Text,
			Enabled: enabledCheck.Checked,
		}

		if provider != nil {
			// Update existing provider
			*provider = newProvider
		} else {
			// Add new provider
			cw.config.Providers = append(cw.config.Providers, newProvider)
		}

		config.SaveConfig(cw.config)
		providerList.Refresh()
		cw.updateProviderSelector()
	})

	content := container.NewVBox(
		form,
		container.NewHBox(layout.NewSpacer(), saveBtn),
	)

	d := dialog.NewCustomConfirm(title, "Save", "Cancel", content, func(response bool) {
		if response {
			// Save is handled in saveBtn
		}
	}, settingsWin)

	// Hook up save button to close dialog
	saveBtn.OnTapped = func() {
		if nameEntry.Text != "" && typeEntry.Selected != "" {
			newProvider := config.Provider{
				Name:    nameEntry.Text,
				Type:    typeEntry.Selected,
				APIKey:  apiKeyEntry.Text,
				BaseURL: baseURLEntry.Text,
				Model:   modelEntry.Text,
				Enabled: enabledCheck.Checked,
			}

			if provider != nil {
				*provider = newProvider
			} else {
				cw.config.Providers = append(cw.config.Providers, newProvider)
			}

			config.SaveConfig(cw.config)
			providerList.Refresh()
			cw.updateProviderSelector()
			d.Hide()
		}
	}

	d.Show()
}

// updateProviderSelector updates the provider selector dropdown with current providers.
func (cw *ChatWindow) updateProviderSelector() {
	providerNames := make([]string, len(cw.config.Providers))
	for i, p := range cw.config.Providers {
		providerNames[i] = p.Name
	}
	cw.providerSelect.Options = providerNames
	cw.providerSelect.Refresh()
}

// createMCPServersTab creates the MCP Servers configuration tab.
// It displays a list of configured MCP servers and allows adding, editing, and deleting them.
// Also shows initialization status and tool list for each server.
func (cw *ChatWindow) createMCPServersTab(parentWindow fyne.Window) fyne.CanvasObject {
	// Track selected MCP server
	var selectedServer *config.MCPServer
	var selectedServerIndex int = -1
	var currentTools []mcp.MCPTool
	enabledCheck := widget.NewCheck("Enabled", nil)

	// Status and tools display
	statusLabel := widget.NewLabel("状态: 未选择")
	statusLabel.TextStyle = fyne.TextStyle{Bold: true}
	toolsLabel := widget.NewLabel("工具列表: 未选择")

	// Tools list
	toolsList := widget.NewList(
		func() int { return len(currentTools) },
		func() fyne.CanvasObject {
			nameLabel := widget.NewLabel("")
			nameLabel.TextStyle = fyne.TextStyle{Bold: true}
			descLabel := widget.NewLabel("")
			descLabel.Wrapping = fyne.TextWrapWord
			return container.NewVBox(
				nameLabel,
				descLabel,
				widget.NewSeparator(),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			cont := obj.(*fyne.Container)
			if id < len(currentTools) {
				tool := currentTools[id]
				nameLabel := cont.Objects[0].(*widget.Label)
				descLabel := cont.Objects[1].(*widget.Label)
				nameLabel.SetText(fmt.Sprintf("• %s", tool.Name))
				descLabel.SetText(tool.Description)
			}
		},
	)

	// Refresh status and tools for selected server
	refreshServerStatus := func(serverName string) {
		if serverName == "" {
			statusLabel.SetText("状态: 未选择")
			toolsLabel.SetText("工具列表: 未选择")
			currentTools = nil
			toolsList.Refresh()
			return
		}

		status, ok := cw.mcpManager.manager.GetServerStatus(serverName)
		if !ok {
			statusLabel.SetText("状态: 未初始化")
			toolsLabel.SetText("工具列表: 未初始化")
			currentTools = nil
			toolsList.Refresh()
			return
		}

		// Update status
		statusText := fmt.Sprintf("状态: %s", status.Status)
		if status.Error != nil {
			statusText += fmt.Sprintf(" - %s", status.Error.Error())
		}
		statusLabel.SetText(statusText)

		// Update tools
		if status.Status == "initialized" && len(status.Tools) > 0 {
			toolsLabel.SetText(fmt.Sprintf("工具列表 (%d 个工具):", len(status.Tools)))
			currentTools = status.Tools
		} else {
			toolsLabel.SetText("工具列表: 无可用工具")
			currentTools = nil
		}
		toolsList.Refresh()
	}

	// Initialize server button
	initBtn := widget.NewButton("初始化", func() {
		if selectedServer == nil {
			dialog.ShowError(fmt.Errorf("请先选择一个服务器"), parentWindow)
			return
		}

		// Show loading dialog
		progress := dialog.NewProgress("正在初始化", fmt.Sprintf("正在初始化 MCP 服务器 '%s'...", selectedServer.Name), parentWindow)
		progress.Resize(fyne.NewSize(300, 100))
		progress.Show()

		// Initialize in goroutine to avoid blocking UI
		go func() {
			status, err := cw.mcpManager.manager.InitializeServer(*selectedServer)
			progress.Hide()

			if err != nil {
				dialog.ShowError(fmt.Errorf("初始化失败: %w", err), parentWindow)
			} else {
				dialog.ShowInformation("成功", fmt.Sprintf("服务器 '%s' 初始化成功，获取到 %d 个工具", selectedServer.Name, len(status.Tools)), parentWindow)
			}

			// Refresh status display
			refreshServerStatus(selectedServer.Name)
		}()
	})

	// Disconnect server button
	disconnectBtn := widget.NewButton("断开连接", func() {
		if selectedServer == nil {
			dialog.ShowError(fmt.Errorf("请先选择一个服务器"), parentWindow)
			return
		}

		err := cw.mcpManager.manager.DisconnectServer(selectedServer.Name)
		if err != nil {
			dialog.ShowError(fmt.Errorf("断开连接失败: %w", err), parentWindow)
		} else {
			dialog.ShowInformation("成功", fmt.Sprintf("服务器 '%s' 已断开连接", selectedServer.Name), parentWindow)
		}

		// Refresh status display
		refreshServerStatus(selectedServer.Name)
	})

	// Create form entries
	nameEntry := widget.NewEntry()
	typeSelect := widget.NewSelect([]string{"stdio", "sse", "streamable_http"}, nil)

	// StdIO fields
	commandEntry := widget.NewEntry()
	argsEntry := widget.NewMultiLineEntry()
	argsEntry.SetPlaceHolder("Enter arguments separated by new lines\ne.g.:\n-y\n@modelcontextprotocol/server-filesystem\n/path/to/files")
	envEntry := widget.NewMultiLineEntry()
	envEntry.SetPlaceHolder("Enter environment variables as KEY=VALUE, one per line\ne.g.:\nPATH=/usr/local/bin\nNODE_ENV=production")

	// SSE and StreamableHTTP fields
	urlEntry := widget.NewEntry()
	headersEntry := widget.NewMultiLineEntry()
	headersEntry.SetPlaceHolder("Enter HTTP headers as KEY=VALUE, one per line\ne.g.:\nAuthorization=Bearer token\nContent-Type=application/json")
	timeoutEntry := widget.NewEntry()
	timeoutEntry.SetPlaceHolder("30")
	timeoutEntry.SetText("30")

	// Containers for different type fields
	stdioContainer := container.NewVBox()
	httpContainer := container.NewVBox()

	// Function to update form fields visibility based on selected type
	updateFormFields := func(serverType string) {
		if serverType == "stdio" {
			stdioContainer.Objects = []fyne.CanvasObject{
				widget.NewSeparator(),
				widget.NewLabel("StdIO Configuration:"),
				container.NewGridWithColumns(2,
					widget.NewLabel("Command:"), commandEntry,
				),
				container.NewGridWithColumns(2,
					widget.NewLabel("Args:"),
					container.NewScroll(argsEntry),
				),
				container.NewGridWithColumns(2,
					widget.NewLabel("Env:"),
					container.NewScroll(envEntry),
				),
			}
			httpContainer.Objects = nil
		} else {
			stdioContainer.Objects = nil
			httpContainer.Objects = []fyne.CanvasObject{
				widget.NewSeparator(),
				widget.NewLabel(serverType + " Configuration:"),
				container.NewGridWithColumns(2,
					widget.NewLabel("URL:"), urlEntry,
				),
				container.NewGridWithColumns(2,
					widget.NewLabel("Headers:"),
					container.NewScroll(headersEntry),
				),
				container.NewGridWithColumns(2,
					widget.NewLabel("Timeout (seconds):"), timeoutEntry,
				),
			}
		}
		stdioContainer.Refresh()
		httpContainer.Refresh()
	}

	// MCP Server list
	mcpList := widget.NewList(
		func() int { return len(cw.config.MCPServers) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(theme.ComputerIcon()),
				widget.NewLabel(""),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			container := obj.(*fyne.Container)
			label := container.Objects[1].(*widget.Label)
			if id < len(cw.config.MCPServers) {
				server := cw.config.MCPServers[id]
				serverType := string(server.Type)
				if serverType == "" {
					serverType = "stdio"
				}
				status := "enabled"
				if !server.Enabled {
					status = "disabled"
				}
				label.SetText(fmt.Sprintf("%s (%s) - %s", server.Name, serverType, status))
			}
		},
	)

	mcpList.OnSelected = func(id widget.ListItemID) {
		if id >= 0 && id < len(cw.config.MCPServers) {
			selectedServer = &cw.config.MCPServers[id]
			selectedServerIndex = id

			// Populate form
			nameEntry.SetText(selectedServer.Name)
			serverType := string(selectedServer.Type)
			if serverType == "" {
				serverType = "stdio"
			}
			typeSelect.SetSelected(serverType)
			enabledCheck.SetChecked(selectedServer.Enabled)
			updateFormFields(serverType)

			// Populate StdIO fields
			commandEntry.SetText(selectedServer.Command)
			if len(selectedServer.Args) > 0 {
				argsEntry.SetText(strings.Join(selectedServer.Args, "\n"))
			} else {
				argsEntry.SetText("")
			}
			if len(selectedServer.Env) > 0 {
				envLines := make([]string, 0, len(selectedServer.Env))
				for k, v := range selectedServer.Env {
					envLines = append(envLines, fmt.Sprintf("%s=%s", k, v))
				}
				envEntry.SetText(strings.Join(envLines, "\n"))
			} else {
				envEntry.SetText("")
			}

			// Populate HTTP fields
			urlEntry.SetText(selectedServer.URL)
			if len(selectedServer.Headers) > 0 {
				headerLines := make([]string, 0, len(selectedServer.Headers))
				for k, v := range selectedServer.Headers {
					headerLines = append(headerLines, fmt.Sprintf("%s=%s", k, v))
				}
				headersEntry.SetText(strings.Join(headerLines, "\n"))
			} else {
				headersEntry.SetText("")
			}
			if selectedServer.TimeoutSeconds > 0 {
				timeoutEntry.SetText(fmt.Sprintf("%d", selectedServer.TimeoutSeconds))
			} else {
				timeoutEntry.SetText("30")
			}

			// Refresh status and tools display
			refreshServerStatus(selectedServer.Name)
		}
	}

	mcpList.OnUnselected = func(id widget.ListItemID) {
		if selectedServerIndex == id {
			selectedServer = nil
			selectedServerIndex = -1

			// Clear form
			nameEntry.SetText("")
			typeSelect.SetSelected("")
			commandEntry.SetText("")
			argsEntry.SetText("")
			envEntry.SetText("")
			urlEntry.SetText("")
			headersEntry.SetText("")
			timeoutEntry.SetText("30")
			updateFormFields("stdio")

			// Clear status and tools display
			refreshServerStatus("")
		}
	}

	// Form
	form := container.NewVBox(
		widget.NewLabel("MCP Server Details"),
		widget.NewSeparator(),
		container.NewGridWithColumns(2,
			widget.NewLabel("Name:"), nameEntry,
			widget.NewLabel("Type:"), typeSelect,
			widget.NewLabel(""), enabledCheck,
		),
	)

	// Add type change handler
	typeSelect.OnChanged = func(serverType string) {
		updateFormFields(serverType)
	}

	form.Add(stdioContainer)
	form.Add(httpContainer)

	// Set minimum sizes for multi-line entries
	argsEntry.SetMinRowsVisible(3)
	envEntry.SetMinRowsVisible(3)
	headersEntry.SetMinRowsVisible(3)

	// Buttons
	addBtn := widget.NewButton("Add New", func() {
		// Clear form and deselect
		selectedServer = nil
		selectedServerIndex = -1
		mcpList.UnselectAll()
		nameEntry.SetText("")
		typeSelect.SetSelected("stdio")
		enabledCheck.SetChecked(true)
		commandEntry.SetText("")
		argsEntry.SetText("")
		envEntry.SetText("")
		urlEntry.SetText("")
		headersEntry.SetText("")
		timeoutEntry.SetText("30")
		updateFormFields("stdio")
		refreshServerStatus("")
	})

	saveBtn := widget.NewButton("Save", func() {
		if nameEntry.Text == "" {
			dialog.ShowError(fmt.Errorf("Server name cannot be empty"), parentWindow)
			return
		}
		if typeSelect.Selected == "" {
			dialog.ShowError(fmt.Errorf("Server type must be selected"), parentWindow)
			return
		}

		newServer := config.MCPServer{
			Name:    nameEntry.Text,
			Type:    config.MCPServerType(typeSelect.Selected),
			Enabled: enabledCheck.Checked,
		}

		// Set type-specific fields
		if typeSelect.Selected == "stdio" {
			if commandEntry.Text == "" {
				dialog.ShowError(fmt.Errorf("Command cannot be empty for StdIO type"), parentWindow)
				return
			}
			newServer.Command = commandEntry.Text

			// Parse args
			if strings.TrimSpace(argsEntry.Text) != "" {
				newServer.Args = strings.Split(strings.TrimSpace(argsEntry.Text), "\n")
			}

			// Parse env
			if strings.TrimSpace(envEntry.Text) != "" {
				env := make(map[string]string)
				envLines := strings.Split(strings.TrimSpace(envEntry.Text), "\n")
				for _, line := range envLines {
					parts := strings.SplitN(line, "=", 2)
					if len(parts) == 2 {
						env[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
					}
				}
				newServer.Env = env
			}
		} else {
			// SSE and StreamableHTTP
			if urlEntry.Text == "" {
				dialog.ShowError(fmt.Errorf("URL cannot be empty for %s type", typeSelect.Selected), parentWindow)
				return
			}
			newServer.URL = urlEntry.Text

			// Parse headers
			if strings.TrimSpace(headersEntry.Text) != "" {
				headers := make(map[string]string)
				headerLines := strings.Split(strings.TrimSpace(headersEntry.Text), "\n")
				for _, line := range headerLines {
					parts := strings.SplitN(line, "=", 2)
					if len(parts) == 2 {
						headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
					}
				}
				newServer.Headers = headers
			}

			// Parse timeout
			if strings.TrimSpace(timeoutEntry.Text) != "" {
				var timeout int
				if _, err := fmt.Sscanf(timeoutEntry.Text, "%d", &timeout); err == nil {
					newServer.TimeoutSeconds = timeout
				}
			}
		}

		if selectedServer != nil {
			// Update existing server
			oldName := selectedServer.Name
			*selectedServer = newServer

			// If name changed, disconnect old connection
			if oldName != newServer.Name {
				_ = cw.mcpManager.manager.DisconnectServer(oldName)
			}
		} else {
			// Add new server
			cw.config.MCPServers = append(cw.config.MCPServers, newServer)
			selectedServerIndex = len(cw.config.MCPServers) - 1
			selectedServer = &cw.config.MCPServers[selectedServerIndex]
		}

		config.SaveConfig(cw.config)
		mcpList.Refresh()

		// Select the updated/new server
		mcpList.Select(selectedServerIndex)
	})

	deleteBtn := widget.NewButton("Delete", func() {
		if selectedServer == nil {
			dialog.ShowError(fmt.Errorf("Please select a server to delete"), parentWindow)
			return
		}

		dialog.ShowConfirm(
			"Delete MCP Server",
			fmt.Sprintf("Are you sure you want to delete MCP server '%s'?", selectedServer.Name),
			func(confirmed bool) {
				if confirmed {
					// Disconnect if connected
					_ = cw.mcpManager.manager.DisconnectServer(selectedServer.Name)

					// Remove MCP server
					cw.config.MCPServers = append(cw.config.MCPServers[:selectedServerIndex], cw.config.MCPServers[selectedServerIndex+1:]...)
					config.SaveConfig(cw.config)

					// Reset selection and clear form
					selectedServer = nil
					selectedServerIndex = -1
					nameEntry.SetText("")
					typeSelect.SetSelected("")
					enabledCheck.SetChecked(false)
					commandEntry.SetText("")
					argsEntry.SetText("")
					envEntry.SetText("")
					urlEntry.SetText("")
					headersEntry.SetText("")
					timeoutEntry.SetText("30")
					updateFormFields("stdio")
					refreshServerStatus("")

					mcpList.Refresh()
				}
			},
			parentWindow,
		)
	})

	buttonContainer := container.NewVBox(
		container.NewHBox(addBtn, saveBtn, deleteBtn),
		container.NewHBox(initBtn, disconnectBtn),
	)

	// Right side container with form and buttons
	rightPanel := container.NewBorder(
		nil,
		buttonContainer,
		nil,
		nil,
		form,
	)

	// Split left and right
	split := container.NewHSplit(
		mcpList,
		rightPanel,
	)
	split.SetOffset(0.4)

	return split
}

// showMCPServerDialog displays a dialog for adding or editing an MCP server.
func (cw *ChatWindow) showMCPServerDialog(settingsWin fyne.Window, server *config.MCPServer, mcpList *widget.List) {
	title := "Add MCP Server"
	if server != nil {
		title = "Edit MCP Server"
	}

	nameEntry := widget.NewEntry()
	typeSelect := widget.NewSelect([]string{"stdio", "sse", "streamable_http"}, nil)
	enabledCheck := widget.NewCheck("Enabled", nil)

	// StdIO fields
	commandEntry := widget.NewEntry()
	argsEntry := widget.NewMultiLineEntry()
	argsEntry.SetPlaceHolder("Enter arguments separated by new lines\ne.g.:\n-y\n@modelcontextprotocol/server-filesystem\n/path/to/files")
	envEntry := widget.NewMultiLineEntry()
	envEntry.SetPlaceHolder("Enter environment variables as KEY=VALUE, one per line\ne.g.:\nPATH=/usr/local/bin\nNODE_ENV=production")

	// HTTP fields
	urlEntry := widget.NewEntry()
	headersEntry := widget.NewMultiLineEntry()
	headersEntry.SetPlaceHolder("Enter HTTP headers as KEY=VALUE, one per line\ne.g.:\nAuthorization=Bearer token")
	timeoutEntry := widget.NewEntry()
	timeoutEntry.SetText("30")

	// Containers for type-specific fields
	stdioContainer := container.NewVBox()
	httpContainer := container.NewVBox()

	// Function to update form fields visibility
	updateFormFields := func(serverType string) {
		if serverType == "stdio" {
			stdioContainer.Objects = []fyne.CanvasObject{
				widget.NewLabel("Command:"), commandEntry,
				widget.NewLabel("Args:"), container.NewGridWithColumns(1, argsEntry),
				widget.NewLabel("Env:"), container.NewGridWithColumns(1, envEntry),
			}
			httpContainer.Objects = nil
		} else {
			stdioContainer.Objects = nil
			httpContainer.Objects = []fyne.CanvasObject{
				widget.NewLabel("URL:"), urlEntry,
				widget.NewLabel("Headers:"), container.NewGridWithColumns(1, headersEntry),
				widget.NewLabel("Timeout (sec):"), timeoutEntry,
			}
		}
		stdioContainer.Refresh()
		httpContainer.Refresh()
	}

	// Set initial state
	typeSelect.SetSelected("stdio")
	typeSelect.OnChanged = updateFormFields
	updateFormFields("stdio")

	if server != nil {
		enabledCheck.SetChecked(server.Enabled)
	} else {
		enabledCheck.SetChecked(true)
	}

	if server != nil {
		nameEntry.SetText(server.Name)
		serverType := string(server.Type)
		if serverType == "" {
			serverType = "stdio"
		}
		typeSelect.SetSelected(serverType)
		commandEntry.SetText(server.Command)
		if len(server.Args) > 0 {
			argsEntry.SetText(strings.Join(server.Args, "\n"))
		}
		if len(server.Env) > 0 {
			envLines := make([]string, 0, len(server.Env))
			for k, v := range server.Env {
				envLines = append(envLines, fmt.Sprintf("%s=%s", k, v))
			}
			envEntry.SetText(strings.Join(envLines, "\n"))
		}
		urlEntry.SetText(server.URL)
		if len(server.Headers) > 0 {
			headerLines := make([]string, 0, len(server.Headers))
			for k, v := range server.Headers {
				headerLines = append(headerLines, fmt.Sprintf("%s=%s", k, v))
			}
			headersEntry.SetText(strings.Join(headerLines, "\n"))
		}
		if server.TimeoutSeconds > 0 {
			timeoutEntry.SetText(fmt.Sprintf("%d", server.TimeoutSeconds))
		}
	}

	form := container.NewVBox(
		container.NewGridWithColumns(2,
			widget.NewLabel("Name:"), nameEntry,
			widget.NewLabel("Type:"), typeSelect,
			widget.NewLabel(""), enabledCheck,
		),
	)
	form.Add(stdioContainer)
	form.Add(httpContainer)

	content := container.NewVBox(
		form,
	)

	var d dialog.Dialog

	saveBtn := widget.NewButton("Save", func() {
		if nameEntry.Text == "" {
			dialog.ShowError(fmt.Errorf("Server name cannot be empty"), settingsWin)
			return
		}
		if typeSelect.Selected == "" {
			dialog.ShowError(fmt.Errorf("Server type must be selected"), settingsWin)
			return
		}

		newServer := config.MCPServer{
			Name:    nameEntry.Text,
			Type:    config.MCPServerType(typeSelect.Selected),
			Enabled: enabledCheck.Checked,
		}

		if typeSelect.Selected == "stdio" {
			if commandEntry.Text == "" {
				dialog.ShowError(fmt.Errorf("Command cannot be empty for StdIO type"), settingsWin)
				return
			}
			newServer.Command = commandEntry.Text

			if argsEntry.Text != "" {
				newServer.Args = strings.Split(strings.TrimSpace(argsEntry.Text), "\n")
			}

			if envEntry.Text != "" {
				env := make(map[string]string)
				envLines := strings.Split(strings.TrimSpace(envEntry.Text), "\n")
				for _, line := range envLines {
					parts := strings.SplitN(line, "=", 2)
					if len(parts) == 2 {
						env[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
					}
				}
				newServer.Env = env
			}
		} else {
			if urlEntry.Text == "" {
				dialog.ShowError(fmt.Errorf("URL cannot be empty for %s type", typeSelect.Selected), settingsWin)
				return
			}
			newServer.URL = urlEntry.Text

			if headersEntry.Text != "" {
				headers := make(map[string]string)
				headerLines := strings.Split(strings.TrimSpace(headersEntry.Text), "\n")
				for _, line := range headerLines {
					parts := strings.SplitN(line, "=", 2)
					if len(parts) == 2 {
						headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
					}
				}
				newServer.Headers = headers
			}

			if timeoutEntry.Text != "" {
				var timeout int
				if _, err := fmt.Sscanf(timeoutEntry.Text, "%d", &timeout); err == nil {
					newServer.TimeoutSeconds = timeout
				}
			}
		}

		if server != nil {
			*server = newServer
		} else {
			cw.config.MCPServers = append(cw.config.MCPServers, newServer)
		}

		config.SaveConfig(cw.config)
		mcpList.Refresh()
		d.Hide()
	})

	d = dialog.NewCustomConfirm(title, "Save", "Cancel", content, func(response bool) {
		if response {
			saveBtn.OnTapped()
		}
	}, settingsWin)

	d.Show()
}
