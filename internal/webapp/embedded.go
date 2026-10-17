package webapp

import (
	"embed"
	"io/fs"
)

//go:embed examples
var examplesFS embed.FS

// bundledApps unpacks the embedded examples so they load like folders on disk.
func bundledApps() []App {
	root, err := fs.Sub(examplesFS, "examples")
	if err != nil {
		return nil
	}
	dirs, err := fs.ReadDir(root, ".")
	if err != nil {
		return nil
	}
	var out []App
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		if a, err := loadFS(root, d.Name()); err == nil {
			out = append(out, a)
		}
	}
	return out
}
