package webapp

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// loadFS reads a web app from a filesystem whose root holds every app.
func loadFS(root fs.FS, name string) (App, error) {
	data, err := fs.ReadFile(root, filepath.ToSlash(filepath.Join(name, "webapp.yaml")))
	if err != nil {
		return App{}, err
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return App{}, err
	}
	a := App{Manifest: m, Dir: filepath.ToSlash(filepath.Join(name))}
	if a.ID == "" {
		a.ID = name
	}
	if a.Name == "" {
		a.Name = a.ID
	}
	if a.Entry == "" {
		a.Entry = "index.html"
	}
	if _, err := fs.Stat(root, filepath.ToSlash(filepath.Join(name, a.Entry))); err != nil {
		return App{}, fmt.Errorf("入口文件不存在: %s", a.Entry)
	}
	if a.Icon == "" {
		a.Icon = "globe"
	}
	a.embedded = root
	return a, nil
}

// readFile reads a file of the app, from its folder or from the embedded copy.
func (a App) readFile(rel string) ([]byte, error) {
	if a.embedded != nil {
		return fs.ReadFile(a.embedded, filepath.ToSlash(filepath.Join(a.Dir, rel)))
	}
	return os.ReadFile(filepath.Join(a.Dir, rel))
}

// data returns the app's files as an fs.FS.
func (a App) data() (fs.FS, error) {
	return os.DirFS(a.Dir), nil
}

// Embedded reports whether the app ships inside the binary.
func (a App) Embedded() bool { return a.embedded != nil }
