package main

import (
	"bytes"
	"os"
	"testing"

	"chatgo/internal/ui/assets"
)

// The app icon lives in two places: the embedded logo that the update window
// draws and the icon.png shipped in the bundle's resources, which packaging
// and the updater fall back to. A stale copy showed an old mark in the update
// window, so keep them the same image.
func TestShippedIconMatchesLogo(t *testing.T) {
	shipped, err := os.ReadFile("resources/icon.png")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(shipped, assets.LogoPNG()) {
		t.Fatal("resources/icon.png differs from internal/ui/assets/logo.png")
	}
}
