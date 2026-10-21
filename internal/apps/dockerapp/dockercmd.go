// Package dockerapp is the Apps menu's Docker manager: list and control
// containers, images, volumes and networks through the Docker client
// library (github.com/docker/docker/client), not the Docker CLI.
package dockerapp

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
)

// Container is one Docker container.
type Container struct {
	ID      string
	Names   []string
	Image   string
	State   string
	Status  string
	Ports   string
	Created int64
}

// Name returns the first container name without the leading slash.
func (c Container) Name() string {
	if len(c.Names) == 0 {
		return c.ShortID()
	}
	return strings.TrimPrefix(c.Names[0], "/")
}

// ShortID returns the first 12 characters of the container ID.
func (c Container) ShortID() string {
	if len(c.ID) > 12 {
		return c.ID[:12]
	}
	return c.ID
}

// Image is one Docker image.
type Image struct {
	ID       string
	RepoTags []string
	Size     int64
	Created  int64
}

// Tag returns the first repo tag or "<none>".
func (im Image) Tag() string {
	if len(im.RepoTags) > 0 {
		return im.RepoTags[0]
	}
	return "<none>"
}

// ShortID returns the first 12 characters of the image ID without the
// "sha256:" prefix.
func (im Image) ShortID() string {
	id := strings.TrimPrefix(im.ID, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// Volume is one Docker volume.
type Volume struct {
	Name       string
	Driver     string
	Mountpoint string
}

// Network is one Docker network.
type Network struct {
	ID     string
	Name   string
	Driver string
	Scope  string
}

// ShortID returns the first 12 characters of the network ID.
func (n Network) ShortID() string {
	if len(n.ID) > 12 {
		return n.ID[:12]
	}
	return n.ID
}

// newClient connects to the local Docker daemon using environment
// settings and negotiated API version.
func newClient() (*client.Client, error) {
	return client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
}

// dockerAvailable reports whether the Docker daemon responds to a ping.
func dockerAvailable() bool {
	cli, err := newClient()
	if err != nil {
		return false
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = cli.Ping(ctx)
	return err == nil
}

// listContainers returns all containers (running and stopped).
func listContainers() ([]Container, error) {
	cli, err := newClient()
	if err != nil {
		return nil, err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rows, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, err
	}
	out := make([]Container, 0, len(rows))
	for _, r := range rows {
		var ports []string
		for _, p := range r.Ports {
			if p.PublicPort == 0 {
				ports = append(ports, fmt.Sprintf("%d/%s", p.PrivatePort, p.Type))
			} else {
				ports = append(ports, fmt.Sprintf("%s:%d->%d/%s", p.IP, p.PublicPort, p.PrivatePort, p.Type))
			}
		}
		out = append(out, Container{
			ID: r.ID, Names: r.Names, Image: r.Image, State: r.State,
			Status: r.Status, Ports: strings.Join(ports, " "), Created: r.Created,
		})
	}
	return out, nil
}

// listImages returns all images.
func listImages() ([]Image, error) {
	cli, err := newClient()
	if err != nil {
		return nil, err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rows, err := cli.ImageList(ctx, image.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]Image, 0, len(rows))
	for _, r := range rows {
		out = append(out, Image{ID: r.ID, RepoTags: r.RepoTags, Size: r.Size, Created: r.Created})
	}
	return out, nil
}

// listVolumes returns all volumes.
func listVolumes() ([]Volume, error) {
	cli, err := newClient()
	if err != nil {
		return nil, err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	resp, err := cli.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]Volume, 0, len(resp.Volumes))
	for _, v := range resp.Volumes {
		if v == nil {
			continue
		}
		out = append(out, Volume{Name: v.Name, Driver: v.Driver, Mountpoint: v.Mountpoint})
	}
	return out, nil
}

// listNetworks returns all networks.
func listNetworks() ([]Network, error) {
	cli, err := newClient()
	if err != nil {
		return nil, err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rows, err := cli.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]Network, 0, len(rows))
	for _, r := range rows {
		out = append(out, Network{ID: r.ID, Name: r.Name, Driver: r.Driver, Scope: r.Scope})
	}
	return out, nil
}

// containerAction performs start/stop/restart/pause/unpause/remove on a
// container.
func containerAction(action, id string) (string, error) {
	cli, err := newClient()
	if err != nil {
		return "", err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	switch action {
	case "start":
		return "已启动", cli.ContainerStart(ctx, id, container.StartOptions{})
	case "stop":
		return "已停止", cli.ContainerStop(ctx, id, container.StopOptions{})
	case "restart":
		return "已重启", cli.ContainerRestart(ctx, id, container.StopOptions{})
	case "pause":
		return "已暂停", cli.ContainerPause(ctx, id)
	case "unpause":
		return "已继续", cli.ContainerUnpause(ctx, id)
	case "remove", "rm":
		return "已删除", cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
	}
	return "", fmt.Errorf("未知操作 %q", action)
}

// fetchLogs reads the last n lines of a container's logs once.
func fetchLogs(id string, n int) ([]string, error) {
	cli, err := newClient()
	if err != nil {
		return nil, err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	stream, err := cli.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: true, ShowStderr: true, Tail: strconv.Itoa(n),
	})
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	return demuxLines(stream)
}

// demuxLines decodes Docker's multiplexed stdout/stderr stream and splits
// it into lines.
func demuxLines(r io.Reader) ([]string, error) {
	var buf bytes.Buffer
	if _, err := stdcopy.StdCopy(&buf, &buf, r); err != nil {
		return nil, err
	}
	text := strings.TrimRight(buf.String(), "\n")
	if text == "" {
		return nil, nil
	}
	return strings.Split(text, "\n"), nil
}

// streamLogs follows a container's logs, calling onLine for each new line
// until the returned stop function is called.
func streamLogs(id string, n int, onLine func(string)) (stop func(), err error) {
	cli, err := newClient()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := cli.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: true, ShowStderr: true, Tail: strconv.Itoa(n), Follow: true,
	})
	if err != nil {
		cancel()
		cli.Close()
		return nil, err
	}
	pr, pw := io.Pipe()
	done := make(chan struct{})
	go func() {
		stdcopy.StdCopy(pw, pw, stream)
		pw.Close()
	}()
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(pr)
		scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			onLine(scanner.Text())
		}
	}()
	return func() {
		cancel()
		stream.Close()
		<-done
		pr.Close()
		cli.Close()
	}, nil
}

// runContainer creates and starts a container from an image.
func runContainer(imageName, name string, ports []string, detach bool, cmd []string) (string, error) {
	cli, err := newClient()
	if err != nil {
		return "", err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cfg := &container.Config{Image: imageName, Cmd: cmd, Tty: false}
	hc := &container.HostConfig{}
	if len(ports) > 0 {
		bindings := nat.PortMap{}
		for _, p := range ports {
			hostPort, containerPort := p, p
			if i := strings.LastIndex(p, ":"); i >= 0 {
				hostPort, containerPort = p[:i], p[i+1:]
			}
			port := nat.Port(containerPort)
			bindings[port] = append(bindings[port], nat.PortBinding{HostPort: hostPort})
		}
		hc.PortBindings = bindings
	}
	resp, err := cli.ContainerCreate(ctx, cfg, hc, nil, nil, name)
	if err != nil {
		return "", err
	}
	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", err
	}
	if detach {
		return resp.ID[:12], nil
	}
	waitCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case <-waitCh:
		logs, _ := fetchLogs(resp.ID, 100)
		return strings.Join(logs, "\n"), nil
	case err := <-errCh:
		return "", err
	}
}

// execContainer runs a command inside a running container and returns its
// combined output.
func execContainer(id string, cmd []string) (string, error) {
	cli, err := newClient()
	if err != nil {
		return "", err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	created, err := cli.ContainerExecCreate(ctx, id, types.ExecConfig{
		Cmd: cmd, AttachStdout: true, AttachStderr: true,
	})
	if err != nil {
		return "", err
	}
	attach, err := cli.ContainerExecAttach(ctx, created.ID, types.ExecStartCheck{})
	if err != nil {
		return "", err
	}
	defer attach.Close()
	lines, err := demuxLines(attach.Reader)
	if err != nil {
		return "", err
	}
	inspect, err := cli.ContainerExecInspect(ctx, created.ID)
	if err == nil && inspect.ExitCode != 0 {
		return strings.Join(lines, "\n"), fmt.Errorf("退出码 %d", inspect.ExitCode)
	}
	return strings.Join(lines, "\n"), nil
}

// systemInfoText returns docker system info as formatted text.
func systemInfoText() (string, error) {
	cli, err := newClient()
	if err != nil {
		return "", err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	info, err := cli.Info(ctx)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Server Version: %s\n", info.ServerVersion)
	fmt.Fprintf(&b, "Storage Driver: %s\n", info.Driver)
	fmt.Fprintf(&b, "Cgroup Driver: %s\n", info.CgroupDriver)
	fmt.Fprintf(&b, "Containers: %d (Running: %d, Paused: %d, Stopped: %d)\n",
		info.Containers, info.ContainersRunning, info.ContainersPaused, info.ContainersStopped)
	fmt.Fprintf(&b, "Images: %d\n", info.Images)
	fmt.Fprintf(&b, "Operating System: %s\n", info.OperatingSystem)
	fmt.Fprintf(&b, "Kernel Version: %s\n", info.KernelVersion)
	fmt.Fprintf(&b, "Total Memory: %.1f GiB\n", float64(info.MemTotal)/(1024*1024*1024))
	fmt.Fprintf(&b, "CPUs: %d\n", info.NCPU)
	fmt.Fprintf(&b, "Docker Root Dir: %s\n", info.DockerRootDir)
	return strings.TrimRight(b.String(), "\n"), nil
}

// diskUsageText returns docker disk usage as formatted text.
func diskUsageText() (string, error) {
	cli, err := newClient()
	if err != nil {
		return "", err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	du, err := cli.DiskUsage(ctx, types.DiskUsageOptions{})
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Images: %d  (%s)\n", len(du.Images), humanSize(du.LayersSize))
	var imgTotal int64
	for _, im := range du.Images {
		if im != nil {
			imgTotal += im.Size
		}
	}
	fmt.Fprintf(&b, "  可回收: %s\n", humanSize(imgTotal))
	var conTotal int64
	for _, ct := range du.Containers {
		if ct != nil {
			conTotal += ct.SizeRw
		}
	}
	fmt.Fprintf(&b, "Containers: %d  (%s)\n", len(du.Containers), humanSize(conTotal))
	var volTotal int64
	for _, v := range du.Volumes {
		if v != nil && v.UsageData != nil {
			volTotal += v.UsageData.Size
		}
	}
	fmt.Fprintf(&b, "Volumes: %d  (%s)\n", len(du.Volumes), humanSize(volTotal))
	var cacheTotal int64
	for _, bc := range du.BuildCache {
		if bc != nil {
			cacheTotal += bc.Size
		}
	}
	fmt.Fprintf(&b, "Build Cache: %s\n", humanSize(cacheTotal))
	return strings.TrimRight(b.String(), "\n"), nil
}

// prune runs a docker prune through the API; scope is
// system/image/container/volume.
func prune(scope string) (string, error) {
	cli, err := newClient()
	if err != nil {
		return "", err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	switch scope {
	case "system":
		r, err := cli.ContainersPrune(ctx, filters.Args{})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("已清理 %d 个容器，回收 %s", len(r.ContainersDeleted), humanSize(int64(r.SpaceReclaimed))), nil
	case "image", "images":
		r, err := cli.ImagesPrune(ctx, filters.Args{})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("已清理 %d 个镜像，回收 %s", len(r.ImagesDeleted), humanSize(int64(r.SpaceReclaimed))), nil
	case "container", "containers":
		r, err := cli.ContainersPrune(ctx, filters.Args{})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("已清理 %d 个容器，回收 %s", len(r.ContainersDeleted), humanSize(int64(r.SpaceReclaimed))), nil
	case "volume", "volumes":
		r, err := cli.VolumesPrune(ctx, filters.Args{})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("已清理 %d 个卷，回收 %s", len(r.VolumesDeleted), humanSize(int64(r.SpaceReclaimed))), nil
	}
	return "", fmt.Errorf("未知清理范围 %q", scope)
}

// removeImage removes an image by ID or tag.
func removeImage(id string) (string, error) {
	cli, err := newClient()
	if err != nil {
		return "", err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err = cli.ImageRemove(ctx, id, image.RemoveOptions{Force: true})
	if err != nil {
		return "", err
	}
	return "已删除镜像 " + id, nil
}

// removeVolume removes a volume by name.
func removeVolume(name string) (string, error) {
	cli, err := newClient()
	if err != nil {
		return "", err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := cli.VolumeRemove(ctx, name, false); err != nil {
		return "", err
	}
	return "已删除卷 " + name, nil
}

// removeNetwork removes a network by name or ID.
func removeNetwork(id string) (string, error) {
	cli, err := newClient()
	if err != nil {
		return "", err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := cli.NetworkRemove(ctx, id); err != nil {
		return "", err
	}
	return "已删除网络 " + id, nil
}

// humanSize formats a byte count for display.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// inspectText renders a container's key metadata as text.
func inspectText(id string) (string, error) {
	cli, err := newClient()
	if err != nil {
		return "", err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	info, err := cli.ContainerInspect(ctx, id)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "ID:   %s\n", info.ID)
	fmt.Fprintf(&b, "名称: %s\n", strings.TrimPrefix(info.Name, "/"))
	fmt.Fprintf(&b, "镜像: %s\n", info.Config.Image)
	fmt.Fprintf(&b, "创建: %s\n", info.Created)
	if st := info.State; st != nil {
		fmt.Fprintf(&b, "状态: %s\n", st.Status)
		fmt.Fprintf(&b, "运行: %v\n", st.Running)
		fmt.Fprintf(&b, "暂停: %v\n", st.Paused)
		fmt.Fprintf(&b, "重启: %v\n", st.Restarting)
		fmt.Fprintf(&b, "退出码: %d\n", st.ExitCode)
		fmt.Fprintf(&b, "OOM: %v\n", st.OOMKilled)
		if st.Error != "" {
			fmt.Fprintf(&b, "错误: %s\n", st.Error)
		}
		if st.StartedAt != "" {
			fmt.Fprintf(&b, "启动: %s\n", st.StartedAt)
		}
		if st.FinishedAt != "" {
			fmt.Fprintf(&b, "结束: %s\n", st.FinishedAt)
		}
	}
	if cfg := info.Config; cfg != nil {
		if len(cfg.Cmd) > 0 {
			fmt.Fprintf(&b, "命令: %s\n", strings.Join(cfg.Cmd, " "))
		}
		if len(cfg.Entrypoint) > 0 {
			fmt.Fprintf(&b, "入口: %s\n", strings.Join(cfg.Entrypoint, " "))
		}
		if cfg.WorkingDir != "" {
			fmt.Fprintf(&b, "工作目录: %s\n", cfg.WorkingDir)
		}
		if cfg.User != "" {
			fmt.Fprintf(&b, "用户: %s\n", cfg.User)
		}
		if len(cfg.Env) > 0 {
			fmt.Fprintf(&b, "环境变量:\n")
			for _, e := range cfg.Env {
				fmt.Fprintf(&b, "  %s\n", e)
			}
		}
	}
	if hc := info.HostConfig; hc != nil {
		if len(hc.Binds) > 0 {
			fmt.Fprintf(&b, "挂载:\n")
			for _, m := range hc.Binds {
				fmt.Fprintf(&b, "  %s\n", m)
			}
		}
		if hc.RestartPolicy.Name != "" {
			fmt.Fprintf(&b, "重启策略: %s\n", hc.RestartPolicy.Name)
		}
		if hc.Memory > 0 {
			fmt.Fprintf(&b, "内存限制: %s\n", humanSize(hc.Memory))
		}
	}
	if len(info.Mounts) > 0 {
		fmt.Fprintf(&b, "挂载点:\n")
		for _, m := range info.Mounts {
			fmt.Fprintf(&b, "  %s:%s (%s)\n", m.Source, m.Destination, m.Type)
		}
	}
	if ns := info.NetworkSettings; ns != nil {
		if ns.IPAddress != "" {
			fmt.Fprintf(&b, "IP: %s\n", ns.IPAddress)
		}
		if len(ns.Ports) > 0 {
			fmt.Fprintf(&b, "端口:\n")
			for port, binds := range ns.Ports {
				for _, pb := range binds {
					fmt.Fprintf(&b, "  %s -> %s:%s\n", port, pb.HostIP, pb.HostPort)
				}
			}
		}
		if len(ns.Networks) > 0 {
			fmt.Fprintf(&b, "网络:\n")
			for name, ep := range ns.Networks {
				fmt.Fprintf(&b, "  %s (%s)\n", name, ep.IPAddress)
			}
		}
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// openTerminal execs an interactive shell in a container and returns the
// hijacked connection plus a read loop feeding output lines.
func openTerminal(id, shell string) (*termConn, error) {
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
	tc := &termConn{
		cli: cli, conn: attach.Conn, reader: attach.Reader,
		done: make(chan struct{}),
	}
	go tc.readLoop(cancel)
	return tc, nil
}

// termConn is an attached interactive terminal to a container.
type termConn struct {
	cli    *client.Client
	conn   interface {
		Write([]byte) (int, error)
		Close() error
	}
	reader interface {
		Read([]byte) (int, error)
	}
	done  chan struct{}
	lines []string
	mu    sync.Mutex
}

// readLoop reads the terminal output into lines until the stream closes.
func (t *termConn) readLoop(cancel context.CancelFunc) {
	defer close(t.done)
	defer cancel()
	scanner := bufio.NewScanner(t.reader)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		t.mu.Lock()
		t.lines = append(t.lines, line)
		if len(t.lines) > 5000 {
			t.lines = t.lines[len(t.lines)-5000:]
		}
		t.mu.Unlock()
	}
}

// Lines returns a copy of the collected output lines.
func (t *termConn) Lines() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, len(t.lines))
	copy(out, t.lines)
	return out
}

// Send writes a command line to the terminal.
func (t *termConn) Send(line string) error {
	_, err := t.conn.Write([]byte(line + "\n"))
	return err
}

// Close tears down the terminal session.
func (t *termConn) Close() {
	t.conn.Close()
	select {
	case <-t.done:
	case <-time.After(2 * time.Second):
	}
	t.cli.Close()
}

// fileEntry is one line of a container directory listing.
type fileEntry struct {
	Name    string
	Mode    string
	Size    int64
	IsDir   bool
	ModTime string
}

// listDir lists a container directory via ls -la.
func listDir(id, path string) ([]fileEntry, error) {
	out, err := execContainer(id, []string{"ls", "-la", path})
	if err != nil {
		return nil, err
	}
	return parseLS(out), nil
}

// parseLS parses ls -la output into entries.
func parseLS(out string) []fileEntry {
	var entries []fileEntry
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}
		mode := fields[0]
		if len(mode) != 10 || !strings.HasPrefix(mode, "-") && !strings.HasPrefix(mode, "d") && !strings.HasPrefix(mode, "l") {
			continue
		}
		size, _ := strconv.ParseInt(fields[4], 10, 64)
		name := strings.Join(fields[8:], " ")
		name = strings.TrimSuffix(name, "*")
		if i := strings.Index(name, " -> "); i >= 0 {
			name = name[:i]
		}
		if name == "." || name == ".." {
			continue
		}
		isDir := strings.HasPrefix(mode, "d")
		if strings.HasSuffix(name, "/") {
			isDir = true
			name = strings.TrimSuffix(name, "/")
		}
		entries = append(entries, fileEntry{
			Name: name, Mode: mode, Size: size, IsDir: isDir,
			ModTime: fields[5] + " " + fields[6],
		})
	}
	return entries
}

// readFile reads one file from a container via CopyFromContainer.
func readFile(id, path string, max int) (string, error) {
	cli, err := newClient()
	if err != nil {
		return "", err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, _, err := cli.CopyFromContainer(ctx, id, path)
	if err != nil {
		return "", err
	}
	defer r.Close()
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				return "", fmt.Errorf("文件 %s 为空或不存在", path)
			}
			return "", err
		}
		if hdr.Typeflag == tar.TypeReg {
			var buf bytes.Buffer
			if _, err := io.CopyN(&buf, tr, int64(max)); err != nil && err != io.EOF {
				return "", err
			}
			return buf.String(), nil
		}
	}
}
