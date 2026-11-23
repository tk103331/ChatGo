package hostinfo

import (
	"runtime"
	"strings"
	"testing"
)

func TestCurrentNamesTheMachine(t *testing.T) {
	info := Current()
	if info.Hostname == "" || info.Platform == "" {
		t.Fatalf("info = %+v", info)
	}
	if info.MAC != "" && strings.Count(info.MAC, ":") != 5 {
		t.Fatalf("mac = %q", info.MAC)
	}
	if info.IP == "" && len(info.IPs) > 0 {
		t.Fatalf("ip unset with ips = %v", info.IPs)
	}
}

func TestPlaceholder(t *testing.T) {
	for _, name := range []string{"", "localhost", "localhost.localdomain", "LOCALHOST.local"} {
		if !placeholder(name) {
			t.Fatalf("%q should be a placeholder", name)
		}
	}
	for _, name := range []string{"tk-macbook", "studio.local", "VM-0-10-ubuntu"} {
		if placeholder(name) {
			t.Fatalf("%q should name a machine", name)
		}
	}
}

func TestComputerNameIsDistinct(t *testing.T) {
	if runtime.GOOS == "darwin" {
		if name := Current().Name; placeholder(name) || name == "localhost" {
			t.Fatalf("name = %q, want the Mac's computer name", name)
		}
	}
}

func TestIfaceScorePrefersLAN(t *testing.T) {
	if ifaceScore("en0") >= ifaceScore("awdl0") {
		t.Fatalf("en0 should rank before awdl0")
	}
	if ifaceScore("utun4") <= ifaceScore("en7") {
		t.Fatalf("utun4 should rank after en7")
	}
}
