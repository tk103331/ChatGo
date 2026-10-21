package dockerapp

import (
	"context"
	"io"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	term "github.com/egoist/mygo/plugins/terminal"
)

// dockerTTY adapts a hijacked exec connection to the terminal's Conn.
type dockerTTY struct {
	cli    *client.Client
	execID string
	conn   interface {
		Write([]byte) (int, error)
		Close() error
	}
	reader io.Reader
}

func (d *dockerTTY) Read(p []byte) (int, error)  { return d.reader.Read(p) }
func (d *dockerTTY) Write(p []byte) (int, error) { return d.conn.Write(p) }

// Resize tells the container's PTY the new grid size.
func (d *dockerTTY) Resize(cols, rows int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return d.cli.ContainerExecResize(ctx, d.execID, container.ResizeOptions{
		Width: uint(cols), Height: uint(rows),
	})
}

// Close tears down the exec connection.
func (d *dockerTTY) Close() error {
	err := d.conn.Close()
	d.cli.Close()
	return err
}

// openDockerTerminal starts an interactive TTY shell in a container.
func openDockerTerminal(id, shell string, onChange func()) (*term.Terminal, error) {
	cli, err := newClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	if shell == "" {
		shell = "/bin/sh"
	}
	created, err := cli.ContainerExecCreate(ctx, id, types.ExecConfig{
		Cmd: []string{shell}, Tty: true,
		AttachStdin: true, AttachStdout: true, AttachStderr: true,
	})
	if err != nil {
		cancel()
		cli.Close()
		return nil, err
	}
	attach, err := cli.ContainerExecAttach(ctx, created.ID, types.ExecStartCheck{})
	if err != nil {
		cancel()
		cli.Close()
		return nil, err
	}
	d := &dockerTTY{cli: cli, execID: created.ID, conn: attach.Conn, reader: attach.Reader}
	t, err := term.New(term.Options{
		Conn: d,
		OnTitle: func(string) {
			if onChange != nil {
				onChange()
			}
		},
		OnExit: func(int) {
			if onChange != nil {
				onChange()
			}
		},
	})
	if err != nil {
		cancel()
		d.Close()
		return nil, err
	}
	go func() {
		<-t.Done()
		cancel()
		attach.Close()
	}()
	return t, nil
}
