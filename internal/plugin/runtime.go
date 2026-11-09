package plugin

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Calcium-Ion/moejs"
)

// maxMemoryMB caps a plugin's per-call JavaScript allocation.
const maxMemoryMB = 256

// runtimePool compiles a plugin once and keeps a few loaded runtimes so two
// conversations never serialise behind one another. A moejs Runtime is not
// concurrency safe, so each call owns one runtime for its whole duration.
type runtimePool struct {
	mod  *moejs.Module
	opts moejs.Options
	size int

	slots   chan *moejs.Runtime
	mu      sync.Mutex
	created int
}

func newRuntimePool(mod *moejs.Module, memoryMB int, size int) *runtimePool {
	if size < 1 {
		size = 1
	}
	if memoryMB <= 0 {
		memoryMB = 64
	}
	if memoryMB > maxMemoryMB {
		memoryMB = maxMemoryMB
	}
	return &runtimePool{
		mod:   mod,
		size:  size,
		slots: make(chan *moejs.Runtime, size),
		opts:  moejs.Options{MemoryLimit: int64(memoryMB) * 1024 * 1024},
	}
}

// acquire returns a loaded runtime, creating one while the pool is under its
// size limit and waiting for a free one otherwise.
func (p *runtimePool) acquire(ctx context.Context) (*moejs.Runtime, error) {
	select {
	case rt := <-p.slots:
		rt.ClearInterrupt()
		rt.ResetAllocation()
		return rt, nil
	default:
	}
	p.mu.Lock()
	if p.created < p.size {
		p.created++
		p.mu.Unlock()
		rt, err := p.newRuntime()
		if err != nil {
			p.mu.Lock()
			p.created--
			p.mu.Unlock()
			return nil, err
		}
		return rt, nil
	}
	p.mu.Unlock()
	select {
	case rt := <-p.slots:
		rt.ClearInterrupt()
		rt.ResetAllocation()
		return rt, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// release returns a runtime to the pool in a clean state.
func (p *runtimePool) release(rt *moejs.Runtime) {
	rt.ClearInterrupt()
	rt.ResetAllocation()
	select {
	case p.slots <- rt:
	default:
		// A shrink or a double release would land here; dropping the runtime
		// is safe because the pool creates on demand.
	}
}

func (p *runtimePool) newRuntime() (*moejs.Runtime, error) {
	rt := moejs.NewRuntime(p.opts)
	if err := rt.SetGlobal("console", map[string]any{
		"log":   moejs.NativeFunc(consoleFn("log")),
		"info":  moejs.NativeFunc(consoleFn("info")),
		"warn":  moejs.NativeFunc(consoleFn("warn")),
		"error": moejs.NativeFunc(consoleFn("error")),
	}); err != nil {
		return nil, err
	}
	if err := rt.Load(p.mod); err != nil {
		return nil, fmt.Errorf("加载入口模块失败: %w", err)
	}
	return rt, nil
}

func consoleFn(level string) func(*moejs.Realm, moejs.Value, []moejs.Value) (moejs.Value, error) {
	return func(r *moejs.Realm, _ moejs.Value, args []moejs.Value) (moejs.Value, error) {
		parts := make([]string, 0, len(args))
		for _, a := range args {
			if s, err := r.ToString(a); err == nil {
				parts = append(parts, s.GoString())
			}
		}
		log.Printf("[plugin:%s] %s", level, strings.Join(parts, " "))
		return moejs.Undefined(), nil
	}
}

// call invokes a plugin hook with Go arguments and returns its result as a Go
// value. Every call is bounded by timeout; an overrun interrupts the runtime
// instead of blocking the caller.
func (p *runtimePool) call(ctx context.Context, timeout time.Duration, hook moejs.Hook, args ...any) (any, error) {
	rt, err := p.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer p.release(rt)

	callCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	stop := context.AfterFunc(callCtx, func() {
		rt.Interrupt(context.Cause(callCtx))
	})
	vals := make([]moejs.Value, 0, len(args))
	for _, a := range args {
		v, err := rt.FromGo(a)
		if err != nil {
			stop()
			return nil, fmt.Errorf("参数转换失败: %w", err)
		}
		vals = append(vals, v)
	}
	res, err := rt.Call(hook, vals...)
	stop()
	if err != nil {
		return nil, describeJSError(err)
	}
	if state, result, ok := moejs.PromiseResult(res); ok {
		switch state {
		case moejs.PromiseFulfilled:
			res = result
		case moejs.PromiseRejected:
			return nil, fmt.Errorf("插件 Promise 被拒绝: %s", describeJSError(valueError(rt, result)))
		default:
			return nil, fmt.Errorf("插件返回了未完成的 Promise（宿主 API 为同步调用）")
		}
	}
	out, err := rt.ToGo(res)
	if err != nil {
		return nil, describeJSError(err)
	}
	return out, nil
}

// valueError turns a rejected promise's reason into an error.
func valueError(rt *moejs.Runtime, v moejs.Value) error {
	if s, err := rt.ToGo(v); err == nil {
		if str, ok := s.(string); ok {
			return errors.New(str)
		}
	}
	return errors.New("unknown rejection")
}

// describeJSError keeps the plugin author's message (and stack for a thrown
// Error) while making it readable in a tool result.
func describeJSError(err error) error {
	var exc *moejs.Exception
	if errors.As(err, &exc) {
		if exc.Stack != "" {
			return fmt.Errorf("JS 错误: %s\n%s", exc.Message(), exc.Stack)
		}
		return fmt.Errorf("JS 错误: %s", exc.Message())
	}
	var ie *moejs.InterruptedError
	if errors.As(err, &ie) {
		return fmt.Errorf("插件执行超时或被中断")
	}
	return err
}
