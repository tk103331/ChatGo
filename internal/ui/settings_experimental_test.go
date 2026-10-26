package ui

import (
	"os"
	"path/filepath"
	"testing"

	mgo "github.com/egoist/mygo/ui"
)

// settingsExperimentalTab is the index of the 实验 tab in the settings page.
const settingsExperimentalTab = 6

// withoutRTK puts an empty directory on PATH, so the probe finds no rtk
// however the machine running the test is set up.
func withoutRTK(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

// withFakeRTK puts a stub rtk on PATH that answers both --version and gain.
func withFakeRTK(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "rtk")
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"--version) echo 'rtk 9.9.9' ;;\n" +
		"gain) echo '{}' ;;\n" +
		"*) exit 1 ;;\n" +
		"esac\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return path
}

// RTK compression ships off, so an install never changes how commands run
// until it is asked for.
func TestRTKDefaultsOff(t *testing.T) {
	cw := testWindow()
	if cw.cfg.RTKEnabled {
		t.Fatal("RTK compression is on by default")
	}
}

// A missing rtk is reported instead of being silently ignored.
func TestDetectRTKMissing(t *testing.T) {
	withoutRTK(t)
	p := detectRTK()
	if p.OK() {
		t.Fatalf("the probe passed without rtk on PATH: %+v", p)
	}
	if p.Err == "" {
		t.Fatal("the probe gives no reason why rtk cannot be used")
	}
}

// A working rtk is found with its version and path.
func TestDetectRTKFound(t *testing.T) {
	path := withFakeRTK(t)
	p := detectRTK()
	if !p.OK() {
		t.Fatalf("the probe rejected a working rtk: %+v", p)
	}
	if p.Path != path {
		t.Fatalf("path is %q, want %q", p.Path, path)
	}
	if p.Version != "rtk 9.9.9" {
		t.Fatalf("version is %q, want %q", p.Version, "rtk 9.9.9")
	}
}

// Another project also ships a binary called rtk. One that cannot answer
// "rtk gain" is not the token optimiser and is refused, so enabling the
// setting never points at the wrong tool.
func TestDetectRTKRejectsImpostor(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\n[ \"$1\" = --version ] && echo 'rtk 0.1' || exit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "rtk"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	p := detectRTK()
	if p.OK() {
		t.Fatalf("a binary that fails 'rtk gain' was accepted: %+v", p)
	}
}

// Turning the switch on without rtk installed leaves it off: the setting
// would otherwise claim a compression that cannot run.
func TestRTKEnableRefusedWithoutBinary(t *testing.T) {
	withoutRTK(t)
	cw := testWindow()
	cw.rtk.probe, cw.rtk.probed = detectRTK(), true
	cw.setRTKEnabled(true)
	if cw.cfg.RTKEnabled {
		t.Fatal("the switch turned on while rtk is missing")
	}
	if cw.toastText == "" {
		t.Fatal("refusing to enable RTK says nothing to the user")
	}
}

// With rtk installed the switch turns on and back off.
func TestRTKEnableWithBinary(t *testing.T) {
	withFakeRTK(t)
	t.Setenv("HOME", t.TempDir())
	cw := testWindow()
	cw.rtk.probe, cw.rtk.probed = detectRTK(), true
	cw.setRTKEnabled(true)
	if !cw.cfg.RTKEnabled {
		t.Fatalf("the switch stayed off with rtk installed; toast: %q", cw.toastText)
	}
	cw.setRTKEnabled(false)
	if cw.cfg.RTKEnabled {
		t.Fatal("the switch stayed on after turning it off")
	}
}

// Both experimental features live on one 实验 tab: the checkpoint levels and
// the RTK switch with the outcome of its probe.
func TestSettingsExperimentalTabHoldsBothFeatures(t *testing.T) {
	withoutRTK(t)
	cw := testWindow()
	cw.section = sectionSettings
	cw.settings.tab = settingsExperimentalTab
	tr := mgo.NewTester(cw.view, 1000, 700)
	for range 8 {
		tr.Frame()
	}
	for _, want := range []string{
		"实验", "Agent Checkpoint", "每轮回复", "工具批次",
		"RTK 命令输出压缩", "启用 RTK 压缩命令输出",
	} {
		if !tr.HasText(want) {
			t.Fatalf("the 实验 tab lacks %q; texts: %v", want, tr.Texts())
		}
	}
}

// The checkpoint control left the 系统 tab, so it is configured in one place.
func TestSystemTabDropsCheckpoint(t *testing.T) {
	cw := testWindow()
	cw.section = sectionSettings
	cw.settings.tab = settingsExperimentalTab + 1
	tr := mgo.NewTester(cw.view, 1000, 700)
	for range 6 {
		tr.Frame()
	}
	if !tr.HasText("更新") {
		t.Fatalf("tab %d is not the 系统 tab; texts: %v", cw.settings.tab, tr.Texts())
	}
	if tr.HasText("Agent Checkpoint") {
		t.Fatalf("the 系统 tab still configures checkpoints; texts: %v", tr.Texts())
	}
}

// Adding the RTK tab must not shift the tabs other code selects by index.
func TestSettingsTabIndexesAreStable(t *testing.T) {
	cw := testWindow()
	cw.section = sectionSettings
	for idx, want := range map[int]string{0: "模型", 1: "工具", 2: "ACP", 4: "快捷键", 5: "外观"} {
		cw.settings.tab = idx
		tr := mgo.NewTester(cw.view, 1000, 700)
		for range 4 {
			tr.Frame()
		}
		if !tr.HasText(want) {
			t.Fatalf("tab %d no longer shows %q; texts: %v", idx, want, tr.Texts())
		}
	}
}
