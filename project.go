package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
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

// trustPath returns the trust store location. Like configPath, the
// XDG fallback (~/.local/state) is used on every platform.
func trustPath() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "clau", "trust.toml")
}

type trustFile struct {
	Trusted map[string]string `toml:"trusted"`
}

// loadTrust reads the trust store. A missing file is an empty store;
// corrupt=true means the file existed but did not parse (treated as
// empty — never a crash).
func loadTrust(path string) (map[string]string, bool) {
	var tf trustFile
	if _, err := toml.DecodeFile(path, &tf); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, false
		}
		return map[string]string{}, true
	}
	if tf.Trusted == nil {
		tf.Trusted = map[string]string{}
	}
	return tf.Trusted, false
}

func saveTrust(path string, store map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(trustFile{Trusted: store})
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
