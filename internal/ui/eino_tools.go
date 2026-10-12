package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"

	bingsearch "github.com/cloudwego/eino-ext/components/tool/bingsearch"
	browseruse "github.com/cloudwego/eino-ext/components/tool/browseruse"
	ddgv2 "github.com/cloudwego/eino-ext/components/tool/duckduckgo/v2"
	googleSearch "github.com/cloudwego/eino-ext/components/tool/googlesearch"
	"github.com/cloudwego/eino-ext/components/tool/sequentialthinking"
	"github.com/cloudwego/eino-ext/components/tool/wikipedia"
)

// einoBuiltinTool builds an Eino-ext tool for a builtin type, or returns
// (nil, false) when the type is served by ChatGo's own implementation.
func einoBuiltinTool(toolType string, cfg map[string]string) (tool.BaseTool, bool, error) {
	ctx := context.Background()
	switch toolType {
	case "wikipedia":
		t, err := wikipedia.NewTool(ctx, &wikipedia.Config{
			Language:  cfg["language"],
			UserAgent: "ChatGo (https://github.com/tk103331/ChatGo)",
		})
		return t, true, err
	case "duckduckgosearch":
		t, err := ddgv2.NewTextSearchTool(ctx, &ddgv2.Config{Timeout: 30 * time.Second})
		return t, true, err
	case "bingsearch":
		key := strings.TrimSpace(cfg["api_key"])
		if key == "" {
			return nil, true, fmt.Errorf("bingsearch 需要 api_key")
		}
		t, err := bingsearch.NewTool(ctx, &bingsearch.Config{APIKey: key})
		return t, true, err
	case "googlesearch":
		key := strings.TrimSpace(cfg["api_key"])
		id := strings.TrimSpace(cfg["search_engine_id"])
		if key == "" || id == "" {
			return nil, true, fmt.Errorf("googlesearch 需要 api_key 与 search_engine_id")
		}
		t, err := googleSearch.NewTool(ctx, &googleSearch.Config{APIKey: key, SearchEngineID: id})
		return t, true, err
	case "browseruse":
		t, err := browseruse.NewBrowserUseTool(ctx, &browseruse.Config{
			Headless:      boolConfig(cfg["headless"], true),
			DDGSearchTool: newDDGSearch(ctx),
		})
		if err != nil {
			return nil, true, err
		}
		return t, true, nil
	case "sequentialthinking":
		t, err := sequentialthinking.NewTool()
		return t, true, err
	}
	return nil, false, nil
}

// newDDGSearch builds the DuckDuckGo search the browser tool uses for its
// web_search action, or nil when it cannot be created.
func newDDGSearch(ctx context.Context) ddgv2.Search {
	s, err := ddgv2.NewSearch(ctx, &ddgv2.Config{Timeout: 30 * time.Second})
	if err != nil {
		return nil
	}
	return s
}

// boolConfig parses a truthy config value, falling back to def.
func boolConfig(v string, def bool) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// einoToolBuiltinTypes lists the builtin types Eino-ext serves.
var einoToolBuiltinTypes = map[string]bool{
	"wikipedia":          true,
	"duckduckgosearch":   true,
	"bingsearch":         true,
	"googlesearch":       true,
	"browseruse":         true,
	"sequentialthinking": true,
}

// isEinoBuiltinTool reports whether ChatGo delegates a builtin type to Eino.
func isEinoBuiltinTool(toolType string) bool { return einoToolBuiltinTypes[toolType] }
