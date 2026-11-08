package ui

import (
	"context"
	"encoding/json"
	"fmt"

	"chatgo/internal/config"
	"chatgo/internal/plugin"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
)

// pluginTool adapts a plugin-declared tool to Eino's tool interface.
type pluginTool struct {
	info    *schema.ToolInfo
	handler func(ctx context.Context, arguments string) (string, error)
}

func (t *pluginTool) Info(context.Context) (*schema.ToolInfo, error) { return t.info, nil }

func (t *pluginTool) InvokableRun(ctx context.Context, arguments string, _ ...tool.Option) (string, error) {
	return t.handler(ctx, arguments)
}

func (t *pluginTool) StreamableRun(context.Context, string) (*schema.StreamReader[string], error) {
	return nil, fmt.Errorf("plugin tool does not support streaming")
}

// newPluginTool builds the Eino tool for a merged plugin builtin entry.
func (cw *ChatWindow) newPluginTool(entry config.BuiltinTool) tool.BaseTool {
	if cw.plugins == nil {
		return nil
	}
	pluginID := entry.Config["plugin"]
	toolName := entry.Config["tool"]
	contrib, ok := cw.plugins.FindTool(pluginID, toolName)
	if !ok {
		return nil
	}
	desc := contrib.Description
	if desc == "" {
		desc = "插件 " + pluginID + " 提供的工具"
	}
	params := schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{})
	if len(contrib.Params) > 0 {
		var js jsonschema.Schema
		if err := json.Unmarshal(contrib.Params, &js); err == nil {
			params = schema.NewParamsOneOfByJSONSchema(&js)
		}
	}
	return &pluginTool{
		info: &schema.ToolInfo{Name: contrib.Name, Desc: desc, ParamsOneOf: params},
		handler: func(ctx context.Context, arguments string) (string, error) {
			return cw.plugins.InvokeTool(ctx, pluginID, toolName, arguments)
		},
	}
}

// pluginToolContribution returns the plugin tool a merged entry describes.
func (cw *ChatWindow) pluginToolContribution(entry config.BuiltinTool) (plugin.ToolContribution, bool) {
	if cw.plugins == nil {
		return plugin.ToolContribution{}, false
	}
	return cw.plugins.FindTool(entry.Config["plugin"], entry.Config["tool"])
}
