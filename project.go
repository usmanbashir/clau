package main

import (
	"os"
	"path/filepath"
)

// discoverProject walks from dir toward the filesystem root and returns
// the nearest regular file named .clau.toml, or "" if there is none.
func discoverProject(dir string) string {
	for {
		p := filepath.Join(dir, ".clau.toml")
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
