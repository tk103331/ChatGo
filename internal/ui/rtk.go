package ui

import (
	"os/exec"
	"strings"
	"sync"
)

// rtkProbe is what the app knows about the rtk CLI on this machine.
type rtkProbe struct {
	// Path is where rtk was found, empty when it is not installed.
	Path string
	// Version is the first line of "rtk --version".
	Version string
	// Err explains why rtk cannot be used, empty when it can.
	Err string
}

// OK reports whether rtk is installed and usable.
func (p rtkProbe) OK() bool { return p.Path != "" && p.Err == "" }

type rtkState struct {
	mu      sync.Mutex
	probe   rtkProbe
	probed  bool
	probing bool
}

// detectRTK looks for the rtk CLI. A different project also ships a binary
// named rtk, so finding the name on PATH is not enough: only the token
// optimiser answers "rtk gain", which is what tells the two apart.
func detectRTK() rtkProbe {
	path, err := exec.LookPath("rtk")
	if err != nil {
		return rtkProbe{Err: "未找到 rtk 命令，请先安装：brew install rtk"}
	}
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return rtkProbe{Path: path, Err: "rtk 无法执行：" + err.Error()}
	}
	version := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if _, err := exec.Command(path, "gain", "--format", "json").Output(); err != nil {
		return rtkProbe{
			Path: path, Version: version,
			Err: "这个 rtk 不是 token 优化工具（同名的其他项目），请安装 rtk-ai/rtk",
		}
	}
	return rtkProbe{Path: path, Version: version}
}

// rtkStatus returns the last probe, starting one in the background the first
// time. Probing runs rtk twice, too slow for the render thread.
func (cw *ChatWindow) rtkStatus() (rtkProbe, bool) {
	cw.rtk.mu.Lock()
	probe, probed, probing := cw.rtk.probe, cw.rtk.probed, cw.rtk.probing
	if !probed && !probing {
		cw.rtk.probing = true
	}
	cw.rtk.mu.Unlock()
	if !probed && !probing {
		go func() {
			p := detectRTK()
			cw.postMain(func() {
				cw.rtk.mu.Lock()
				cw.rtk.probe, cw.rtk.probed, cw.rtk.probing = p, true, false
				cw.rtk.mu.Unlock()
				cw.invalidate()
			})
		}()
	}
	return probe, probed
}

// recheckRTK drops the cached probe so the next status call runs a fresh one,
// for the Recheck button after installing rtk.
func (cw *ChatWindow) recheckRTK() {
	cw.rtk.mu.Lock()
	cw.rtk.probe, cw.rtk.probed = rtkProbe{}, false
	cw.rtk.mu.Unlock()
	cw.invalidate()
}

// rtkRewrite asks the rtk CLI to rewrite a raw command, returning the
// rewritten form and whether rtk forbids the command. Exit codes follow the
// CLI's contract: 2 forbids, 1 means nothing to rewrite, 0 and 3 both print
// a rewrite. Any failure degrades to the original command so a slow or
// broken rtk never blocks the user's bash call.
func (cw *ChatWindow) rtkRewrite(command string) (rewritten string, blocked bool) {
	probe, probed := cw.rtkStatus()
	if !probed || !probe.OK() {
		return "", false
	}
	cmd := exec.Command(probe.Path, "rewrite", command)
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	}
	switch code {
	case 2:
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = "rtk 拒绝了该命令"
		}
		return msg, true
	case 1:
		return "", false
	}
	return strings.TrimSpace(string(out)), false
}
