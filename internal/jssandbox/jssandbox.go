// Package jssandbox runs untrusted JavaScript (or TypeScript) in an isolated
// moejs runtime with memory and time limits, returning the result as JSON.
package jssandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Calcium-Ion/moejs"
)

// Config holds the parsed settings for a JS sandbox tool instance.
type Config struct {
	MemoryLimitMB int
	Timeout       time.Duration
	TypeScript    bool
}

// ParseConfig reads the tool's config map with sensible defaults.
func ParseConfig(cfg map[string]string) Config {
	c := Config{
		MemoryLimitMB: 64,
		Timeout:       5 * time.Second,
		TypeScript:    false,
	}
	if v := cfg["memory_limit_mb"]; v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			c.MemoryLimitMB = n
		}
	}
	if v := cfg["timeout_seconds"]; v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			c.Timeout = time.Duration(n) * time.Second
		}
	}
	if v := cfg["typescript"]; v != "" {
		c.TypeScript = strings.EqualFold(strings.TrimSpace(v), "true")
	}
	return c
}

// result is the JSON shape returned to the LLM.
type sandboxResult struct {
	OK         bool            `json:"ok"`
	Result     json.RawMessage `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
	Name       string          `json:"name,omitempty"`
	Console    string          `json:"console,omitempty"`
	DurationUs int64           `json:"duration_us,omitempty"`
}

// Run compiles and runs code in an isolated moejs runtime with memory and time
// limits, then returns the result as JSON. The code must export a function
// named fnName (default "run"); it receives input (parsed JSON) as its argument
// and its return value is serialised back.
func Run(ctx context.Context, code, input, fnName string, cfg Config) (string, error) {
	if fnName == "" {
		fnName = "run"
	}

	start := time.Now()

	var mod *moejs.Module
	var err error
	if cfg.TypeScript {
		mod, err = moejs.CompileTS("sandbox.ts", code)
	} else {
		mod, err = moejs.Compile("sandbox.js", code)
	}
	if err != nil {
		return resultJSON(false, "", err.Error(), "", "", start), nil
	}

	hook, err := mod.Hook(fnName)
	if err != nil {
		return resultJSON(false, "", fmt.Sprintf("export %q not found: %s", fnName, err), "", "", start), nil
	}

	opts := moejs.Options{
		MemoryLimit: int64(cfg.MemoryLimitMB) * 1024 * 1024,
	}
	rt := moejs.NewRuntime(opts)

	var consoleBuf strings.Builder
	if err := rt.SetGlobal("console", map[string]any{
		"log":   moejs.NativeFunc(makeConsoleLog(&consoleBuf, "log")),
		"info":  moejs.NativeFunc(makeConsoleLog(&consoleBuf, "info")),
		"warn":  moejs.NativeFunc(makeConsoleLog(&consoleBuf, "warn")),
		"error": moejs.NativeFunc(makeConsoleLog(&consoleBuf, "error")),
	}); err != nil {
		return resultJSON(false, "", err.Error(), "", "", start), nil
	}

	if err := rt.Load(mod); err != nil {
		return resultJSON(false, "", err.Error(), "", "", start), nil
	}

	runCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	interrupted := make(chan struct{})
	stop := context.AfterFunc(runCtx, func() {
		rt.Interrupt(context.Cause(runCtx))
		close(interrupted)
	})
	defer func() {
		if !stop() {
			<-interrupted
		}
	}()

	var arg moejs.Value
	if strings.TrimSpace(input) != "" {
		arg, err = rt.ParseJSONString(input)
	} else {
		arg, err = rt.ParseJSONString("null")
	}
	if err != nil {
		return resultJSON(false, "", err.Error(), "", consoleBuf.String(), start), nil
	}

	res, err := rt.Call(hook, arg)
	if err != nil {
		var exc *moejs.Exception
		var ie *moejs.InterruptedError
		switch {
		case errors.As(err, &exc):
			return resultJSON(false, "", exc.Message(), exc.Name(), consoleBuf.String(), start), nil
		case errors.As(err, &ie):
			cause := "timeout"
			if c := context.Cause(runCtx); c != nil {
				cause = c.Error()
			}
			return resultJSON(false, "", cause, "", consoleBuf.String(), start), nil
		default:
			return resultJSON(false, "", err.Error(), "", consoleBuf.String(), start), nil
		}
	}

	var raw json.RawMessage
	if err := rt.Unmarshal(res, &raw); err != nil {
		return resultJSON(false, "", err.Error(), "", consoleBuf.String(), start), nil
	}

	return resultJSON(true, string(raw), "", "", consoleBuf.String(), start), nil
}

// makeConsoleLog returns a host function that appends to the buffer.
func makeConsoleLog(buf *strings.Builder, level string) func(*moejs.Realm, moejs.Value, []moejs.Value) (moejs.Value, error) {
	return func(r *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
		var parts []string
		for _, a := range args {
			s, err := r.ToString(a)
			if err != nil {
				parts = append(parts, "[?]")
				continue
			}
			parts = append(parts, s.GoString())
		}
		buf.WriteString("[" + level + "] " + strings.Join(parts, " ") + "\n")
		return moejs.Undefined(), nil
	}
}

// resultJSON builds the JSON result string using encoding/json for safety.
func resultJSON(ok bool, result, errMsg, name, console string, start time.Time) string {
	o := sandboxResult{
		OK:         ok,
		Error:      errMsg,
		Name:       name,
		Console:    console,
		DurationUs: time.Since(start).Microseconds(),
	}
	if result != "" {
		o.Result = json.RawMessage(result)
	}
	b, _ := json.Marshal(o)
	return string(b)
}
