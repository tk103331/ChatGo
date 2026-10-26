package ui

import (
	"os"
	"testing"
)

// TestMain runs every test with HOME pointed at a temp directory. Rendering
// the composer and the skill page saves the configuration, which would
// otherwise land in the developer's own ~/.chatgo/config.yaml; those two
// reach nearly every view test, so the whole package is isolated here rather
// than case by case.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "chatgo-ui-test-*")
	if err != nil {
		os.Exit(1)
	}
	os.Setenv("HOME", home)
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
