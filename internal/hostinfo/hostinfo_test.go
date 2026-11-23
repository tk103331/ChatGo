package hostinfo

import (
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

func TestIfaceScorePrefersLAN(t *testing.T) {
	if ifaceScore("en0") >= ifaceScore("awdl0") {
		t.Fatalf("en0 should rank before awdl0")
	}
	if ifaceScore("utun4") <= ifaceScore("en7") {
		t.Fatalf("utun4 should rank after en7")
	}
}
