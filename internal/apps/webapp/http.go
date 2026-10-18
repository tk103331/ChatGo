package webapp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// localServer serves an app's mounted backend over loopback TCP, so MCP
// clients that only speak http:// can reach it.
type localServer struct {
	mu   sync.Mutex
	srv  *http.Server
	addr string
}

// start brings the loopback server up once and returns its base URL.
func (l *localServer) start(routes map[string]http.Handler, bridge *bridge) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.srv != nil {
		return l.addr, nil
	}
	mux := http.NewServeMux()
	for prefix, h := range routes {
		mux.Handle(prefix, h)
		mux.Handle(prefix+"/", h)
	}
	mux.Handle("/__chatgo/", http.HandlerFunc(bridge.serve))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	l.srv = srv
	l.addr = "http://" + ln.Addr().String()
	go func() { _ = srv.Serve(ln) }()
	return l.addr, nil
}

// close stops the server.
func (l *localServer) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = l.srv.Shutdown(ctx)
	l.srv, l.addr = nil, ""
}

// ensureHTTP returns the loopback base URL for the app's mounted routes.
func (cl *Client) ensureHTTP() (string, error) {
	cl.httpOnce.Do(func() {
		cl.httpAddr, cl.httpErr = cl.local.start(cl.routes, cl.br)
	})
	if cl.httpErr != nil {
		return "", cl.httpErr
	}
	if cl.httpAddr == "" {
		return "", fmt.Errorf("本地服务未启动")
	}
	return cl.httpAddr, nil
}
