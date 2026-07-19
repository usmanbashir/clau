package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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

// hashBytes returns the lowercase hex SHA-256 digest of data.
func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}

type ProjectStatus struct {
	Path    string // discovered .clau.toml; "" if none in play
	Trusted bool   // content hash matches the trust store
	Changed bool   // in the store, but content differs
	Applied bool   // layer merged into the returned Config
}

func sameFile(a, b string) bool {
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(fa, fb)
}

// loadEffectiveConfig loads the global config and, when a trusted
// .clau.toml is discovered walking up from cwd, layers it on top.
// enforce=true makes an untrusted or changed project file a hard error
// (launch paths); enforce=false returns the global-only view plus
// status (list, doctor). CLAU_NO_PROJECT (non-empty) skips discovery.
func loadEffectiveConfig(cwd string, enforce bool) (Config, ProjectStatus, error) {
	global, err := loadConfig(configPath())
	if err != nil {
		return Config{}, ProjectStatus{}, err
	}
	if os.Getenv("CLAU_NO_PROJECT") != "" {
		return global, ProjectStatus{}, nil
	}
	proj := discoverProject(cwd)
	if proj == "" || sameFile(proj, configPath()) {
		return global, ProjectStatus{}, nil
	}
	st := ProjectStatus{Path: proj}
	data, err := os.ReadFile(proj)
	if err != nil {
		return Config{}, st, fmt.Errorf("project config %s: %v", proj, err)
	}
	hash := hashBytes(data)
	store, _ := loadTrust(trustPath())
	switch stored, known := store[proj]; {
	case known && stored == hash:
		st.Trusted = true
	case known:
		st.Changed = true
	}
	if !st.Trusted {
		if enforce {
			reason := "is not trusted"
			if st.Changed {
				reason = "changed since it was trusted"
			}
			return Config{}, st, fmt.Errorf(
				"project config %s %s; review with `clau trust --show`, then `clau trust` to allow (CLAU_NO_PROJECT=1 skips project configs)",
				proj, reason)
		}
		return global, st, nil
	}
	layered, err := applyConfigData(global, proj, data)
	if err != nil {
		return Config{}, st, err
	}
	st.Applied = true
	return layered, st, nil
}

// projectDeclarations reports which model keys and profile names the
// project file itself declares, for provenance display. The file has
// already been validated by the time this is called.
func projectDeclarations(path string) (map[string]bool, map[string]bool, error) {
	var raw rawConfig
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return nil, nil, err
	}
	models, profiles := map[string]bool{}, map[string]bool{}
	for k := range raw.Models {
		models[k] = true
	}
	for k := range raw.Profiles {
		profiles[k] = true
	}
	return models, profiles, nil
}
