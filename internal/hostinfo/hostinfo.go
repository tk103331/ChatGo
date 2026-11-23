// Package hostinfo names the machine running ChatGo: its host name, platform,
// hardware address and LAN addresses, so a phone can tell its desktops apart.
package hostinfo

import (
	"net"
	"os"
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
	info := Info{Name: host, Hostname: host, Platform: runtime.GOOS}
	info.MAC, info.IPs = interfaces()
	if len(info.IPs) > 0 {
		info.IP = info.IPs[0]
	}
	return info
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
