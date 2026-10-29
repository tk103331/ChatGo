package assets

import "testing"

// TestPigLogo checks PiG resolves to its own bundled mark, not the Pi one.
func TestPigLogo(t *testing.T) {
	if key := ACPIconKey("PiG"); key != "pig" {
		t.Fatalf("PiG should map to its own icon key, got %q", key)
	}
	if bmp := ACPLogo("PiG"); bmp == nil {
		t.Fatal("PiG should have a bundled bitmap logo")
	}
	if bmp := ACPLogo("pig-acp"); bmp == nil {
		t.Fatal("the pig-acp command name should resolve too")
	}
	if ACPLogoSVG("PiG") != nil {
		t.Fatal("PiG uses a bitmap, not an SVG silhouette")
	}
	if ACPIconKey("PiAgent") != "pi" {
		t.Fatal("PiAgent must keep the Pi mark")
	}
}
