package webapp

import (
	"context"
	"encoding/json"
	"time"
)

// Tool is one operation the Agent may run through the page.
type Tool struct {
	Name   string
	Desc   string
	Params []ParamSpec
	Run    func(args map[string]any) (string, error)
}

// jsonFor encodes a value for a JS call.
func jsonFor(v any) ([]byte, error) { return json.Marshal(v) }

// contextWithTimeout bounds a page evaluation; the caller cancels it.
func contextWithTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), callTimeout)
}

// timeAfter is the tool call deadline channel.
func timeAfter() <-chan time.Time { return time.After(callTimeout) }
