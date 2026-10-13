package ui

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

// jsSandboxConfig holds the parsed settings for a JS sandbox tool instance.
type jsSandboxConfig struct {
	memoryLimitMB int
	timeout       time.Duration
	typescript    bool
}

// parseJsSandboxConfig reads the tool's config map with sensible defaults.
func parseJsSandboxConfig(cfg map[string]string) jsSandboxConfig {
	c := jsSandboxConfig{
		memoryLimitMB: 64,
		timeout:       5 * time.Second,
		typescript:    false,
	}
	if v := cfg["memory_limit_mb"]; v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			c.memoryLimitMB = n
		}
	}
	if v := cfg["timeout_seconds"]; v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			c.timeout = time.Duration(n) * time.Second
		}
	}
	if v := cfg["typescript"]; v != "" {
		c.typescript = strings.EqualFold(strings.TrimSpace(v), "true")
	}
	return c
}

// jsSandboxResult is the JSON shape returned to the LLM.
type jsSandboxResult struct {
	OK         bool            `json:"ok"`
	Result     json.RawMessage `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
	Name       string          `json:"name,omitempty"`
	Console    string          `json:"console,omitempty"`
	DurationUs int64           `json:"duration_us,omitempty"`
}

// runJsSandbox compiles and runs JavaScript (or TypeScript) code in an
// isolated moejs runtime with memory and time limits, then returns the
// result as JSON. The code must export a function named fnName (default
// "run"); it receives input (parsed JSON) as its argument and its return
// value is serialised back.
func runJsSandbox(ctx context.Context, code, input, fnName string, cfg jsSandboxConfig) (string, error) {
	if fnName == "" {
		fnName = "run"
	}

	start := time.Now()

	var mod *moejs.Module
	var err error
	if cfg.typescript {
		mod, err = moejs.CompileTS("sandbox.ts", code)
	} else {
		mod, err = moejs.Compile("sandbox.js", code)
	}
	if err != nil {
		return jsResultJSON(false, "", err.Error(), "", "", start), nil
	}

	hook, err := mod.Hook(fnName)
	if err != nil {
		return jsResultJSON(false, "", fmt.Sprintf("export %q not found: %s", fnName, err), "", "", start), nil
	}

	opts := moejs.Options{
		MemoryLimit: int64(cfg.memoryLimitMB) * 1024 * 1024,
	}
	rt := moejs.NewRuntime(opts)

	var consoleBuf strings.Builder
	if err := rt.SetGlobal("console", map[string]any{
		"log":   moejs.NativeFunc(makeConsoleLog(&consoleBuf, "log")),
		"info":  moejs.NativeFunc(makeConsoleLog(&consoleBuf, "info")),
		"warn":  moejs.NativeFunc(makeConsoleLog(&consoleBuf, "warn")),
		"error": moejs.NativeFunc(makeConsoleLog(&consoleBuf, "error")),
	}); err != nil {
		return jsResultJSON(false, "", err.Error(), "", "", start), nil
	}

	if err := rt.Load(mod); err != nil {
		return jsResultJSON(false, "", err.Error(), "", "", start), nil
	}

	runCtx, cancel := context.WithTimeout(ctx, cfg.timeout)
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
		return jsResultJSON(false, "", err.Error(), "", consoleBuf.String(), start), nil
	}

	res, err := rt.Call(hook, arg)
	if err != nil {
		var exc *moejs.Exception
		var ie *moejs.InterruptedError
		switch {
		case errors.As(err, &exc):
			return jsResultJSON(false, "", exc.Message(), exc.Name(), consoleBuf.String(), start), nil
		case errors.As(err, &ie):
			cause := "timeout"
			if c := context.Cause(runCtx); c != nil {
				cause = c.Error()
			}
			return jsResultJSON(false, "", cause, "", consoleBuf.String(), start), nil
		default:
			return jsResultJSON(false, "", err.Error(), "", consoleBuf.String(), start), nil
		}
	}

	var raw json.RawMessage
	if err := rt.Unmarshal(res, &raw); err != nil {
		return jsResultJSON(false, "", err.Error(), "", consoleBuf.String(), start), nil
	}

	return jsResultJSON(true, string(raw), "", "", consoleBuf.String(), start), nil
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

// jsResultJSON builds the JSON result string using encoding/json for safety.
func jsResultJSON(ok bool, result, errMsg, name, console string, start time.Time) string {
	o := jsSandboxResult{
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
