package relay

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
)

// wsConn presents a WebSocket connection as a byte stream for yamux. Each
// Write is one binary message; Read drains one message at a time.
type wsConn struct {
	c   *websocket.Conn
	r   io.Reader
	wmu sync.Mutex
}

func NewWSConn(c *websocket.Conn) net.Conn { return &wsConn{c: c} }

func (c *wsConn) Read(p []byte) (int, error) {
	for {
		if c.r == nil {
			_, r, err := c.c.NextReader()
			if err != nil {
				return 0, err
			}
			c.r = r
		}
		n, err := c.r.Read(p)
		if n > 0 {
			if err == io.EOF {
				c.r = nil
				err = nil
			}
			return n, err
		}
		if err == io.EOF {
			c.r = nil
			continue
		}
		return 0, err
	}
}

func (c *wsConn) Write(p []byte) (int, error) {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	w, err := c.c.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return 0, err
	}
	n, err := w.Write(p)
	if err != nil {
		_ = w.Close()
		return n, err
	}
	return n, w.Close()
}

func (c *wsConn) Close() error                     { return c.c.Close() }
func (c *wsConn) LocalAddr() net.Addr              { return wsAddr("local") }
func (c *wsConn) RemoteAddr() net.Addr             { return wsAddr("remote") }
func (c *wsConn) SetDeadline(time.Time) error      { return nil }
func (c *wsConn) SetReadDeadline(time.Time) error  { return nil }
func (c *wsConn) SetWriteDeadline(time.Time) error { return nil }

type wsAddr string

func (a wsAddr) Network() string { return "websocket" }
func (a wsAddr) String() string  { return string(a) }

// yamuxListener adapts a yamux session to net.Listener so net/http can serve
// streams the peer opens.
type yamuxListener struct {
	session *yamux.Session
}

func NewYamuxListener(session *yamux.Session) net.Listener { return &yamuxListener{session: session} }

func (l *yamuxListener) Accept() (net.Conn, error) { return l.session.Accept() }
func (l *yamuxListener) Close() error              { return l.session.Close() }
func (l *yamuxListener) Addr() net.Addr            { return wsAddr("yamux") }
