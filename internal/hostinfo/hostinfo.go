// Package hostinfo names the machine running ChatGo: its host name, platform,
// hardware address and LAN addresses, so a phone can tell its desktops apart.
package hostinfo

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
)

// Info is one machine's identity as the phone shows it.
type Info struct {
	Name     string   `json:"name"`
	Hostname string   `json:"hostname"`
	Platform string   `json:"platform"`
	MAC      string   `json:"mac,omitempty"`
	IP       string   `json:"ip,omitempty"`
	IPs      []string `json:"ips,omitempty"`
}

// Current describes this machine.
func Current() Info {
	host, _ := os.Hostname()
	host = strings.TrimSpace(host)
	name := computerName()
	if placeholder(name) {
		name = host
	}
	info := Info{Name: name, Hostname: host, Platform: runtime.GOOS}
	info.MAC, info.IPs = interfaces()
	if len(info.IPs) > 0 {
		info.IP = info.IPs[0]
	}
	// A machine left at localhost still needs a name the phone can tell
	// apart: fall back to the platform and the hardware address.
	if placeholder(info.Name) {
		if info.MAC != "" {
			short := strings.ReplaceAll(info.MAC, ":", "")
			info.Name = fmt.Sprintf("%s-%s", platformName(), short[max(0, len(short)-6):])
		} else if info.Hostname != "" && !placeholder(info.Hostname) {
			info.Name = info.Hostname
		}
	}
	if info.Name == "" {
		info.Name = info.Hostname
	}
	return info
}

// computerName returns the name the system shows for this machine, which
// differs from the network host name on macOS and Windows.
func computerName() string {
	switch runtime.GOOS {
	case "darwin":
		for _, args := range [][]string{
			{"scutil", "--get", "ComputerName"},
			{"scutil", "--get", "LocalHostName"},
		} {
			if out, err := exec.Command(args[0], args[1:]...).Output(); err == nil {
				if name := strings.TrimSpace(string(out)); name != "" {
					return name
				}
			}
		}
	case "windows":
		if name := strings.TrimSpace(os.Getenv("COMPUTERNAME")); name != "" {
			return name
		}
	case "linux":
		if data, err := os.ReadFile("/etc/hostname"); err == nil {
			if name := strings.TrimSpace(string(data)); name != "" {
				return name
			}
		}
		if name := strings.TrimSpace(os.Getenv("HOSTNAME")); name != "" {
			return name
		}
	}
	return strings.TrimSpace(mustHostname())
}

func mustHostname() string {
	host, _ := os.Hostname()
	return host
}

// placeholder reports whether a name says nothing about the machine.
func placeholder(name string) bool {
	low := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), ".local"))
	switch low {
	case "", "localhost", "localhost.localdomain", "unknown":
		return true
	}
	return false
}

func platformName() string {
	switch runtime.GOOS {
	case "darwin", "macos":
		return "Mac"
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	}
	return "ChatGo"
}

type candidate struct {
	name  string
	mac   string
	ips   []string
	score int
	has4  bool
}

// interfaces picks the hardware address and addresses of the machine's LAN
// interface, preferring the usual wired and wireless names.
func interfaces() (string, []string) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", nil
	}
	var found []candidate
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		c := candidate{name: iface.Name, score: ifaceScore(iface.Name)}
		if len(iface.HardwareAddr) >= 6 {
			c.mac = strings.ToUpper(iface.HardwareAddr.String())
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ip, _, err := net.ParseCIDR(a.String())
			if err != nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				continue
			}
			c.ips = append(c.ips, ip.String())
			if ip.To4() != nil {
				c.has4 = true
			}
		}
		if c.mac != "" || len(c.ips) > 0 {
			found = append(found, c)
		}
	}
	if len(found) == 0 {
		return "", nil
	}
	// An interface with both a hardware address and an IPv4 address wins,
	// then the usual names, so a Mac shows en0 and not a tunnel or AWDL.
	sort.SliceStable(found, func(i, j int) bool {
		a, b := found[i], found[j]
		if (a.mac != "" && a.has4) != (b.mac != "" && b.has4) {
			return a.mac != "" && a.has4
		}
		if a.score != b.score {
			return a.score < b.score
		}
		return a.name < b.name
	})
	best := found[0]
	ips := append([]string(nil), best.ips...)
	for _, c := range found[1:] {
		ips = append(ips, c.ips...)
	}
	return best.mac, unique(ips)
}

// ifaceScore ranks the names whose address is the LAN one.
func ifaceScore(name string) int {
	switch name {
	case "en0", "eth0", "wlan0":
		return 0
	}
	low := strings.ToLower(name)
	switch {
	case strings.HasPrefix(low, "en"), strings.HasPrefix(low, "eth"), strings.HasPrefix(low, "wl"):
		return 1
	case strings.HasPrefix(low, "utun"), strings.HasPrefix(low, "tun"), strings.HasPrefix(low, "tap"),
		strings.HasPrefix(low, "awdl"), strings.HasPrefix(low, "llw"), strings.HasPrefix(low, "bridge"),
		strings.HasPrefix(low, "docker"), strings.HasPrefix(low, "veth"):
		return 3
	}
	return 2
}

func unique(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := in[:0]
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
