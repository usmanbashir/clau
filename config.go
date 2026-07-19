package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/BurntSushi/toml"
)

type ModelSpec struct {
	Model   string
	Efforts bool
}

type Profile struct {
	Model  string
	Effort string
	Flags  []string
	Env    map[string]string
	Claude string
}

type Config struct {
	Claude   string
	Models   map[string]ModelSpec
	Efforts  map[string]string
	Profiles map[string]Profile
}

func defaultConfig() Config {
	return Config{
		Claude: "claude",
		Models: map[string]ModelSpec{
			"o": {Model: "opus", Efforts: true},
			"s": {Model: "sonnet", Efforts: true},
			"f": {Model: "fable", Efforts: true},
			"h": {Model: "haiku", Efforts: false},
		},
		Efforts: map[string]string{
			"1": "low", "2": "medium", "3": "high", "4": "xhigh", "5": "max",
		},
		Profiles: map[string]Profile{},
	}
}

func configPath() string {
	if p := os.Getenv("CLAU_CONFIG"); p != "" {
		return p
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "clau", "config.toml")
}

var (
	modelKeyRe    = regexp.MustCompile(`^[a-z]+$`)
	effortKeyRe   = regexp.MustCompile(`^[1-9]$`)
	profileNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

type rawModel struct {
	Model   string `toml:"model"`
	Efforts *bool  `toml:"efforts"`
}

type rawProfile struct {
	Model  string            `toml:"model"`
	Effort string            `toml:"effort"`
	Flags  []string          `toml:"flags"`
	Env    map[string]string `toml:"env"`
	Claude string            `toml:"claude"`
}

type rawConfig struct {
	Claude   string                    `toml:"claude"`
	Models   map[string]toml.Primitive `toml:"models"`
	Efforts  map[string]string         `toml:"efforts"`
	Profiles map[string]rawProfile     `toml:"profiles"`
}

func loadConfig(path string) (Config, error) {
	return applyConfigFile(defaultConfig(), path)
}

func cloneMap[V any](m map[string]V) map[string]V {
	out := make(map[string]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// applyConfigFile reads path and delegates to applyConfigData. A missing
// file returns cfg unchanged, nil error.
func applyConfigFile(cfg Config, path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return applyConfigData(cfg, path, data)
}

// applyConfigData decodes data and merges it over cfg: models per key,
// profiles per name (wholesale), a non-empty efforts table replaces the
// ladder, claude if set. path is used only to name errors, so a caller
// that must hash and apply the exact same bytes can read the file once
// and pass the bytes to both. The maps of the cfg argument are never
// mutated.
func applyConfigData(cfg Config, path string, data []byte) (Config, error) {
	var raw rawConfig
	md, err := toml.Decode(string(data), &raw)
	if err != nil {
		var pe toml.ParseError
		if errors.As(err, &pe) {
			return Config{}, fmt.Errorf("%s: %s", path, pe.ErrorWithPosition())
		}
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	cfg.Models = cloneMap(cfg.Models)
	cfg.Profiles = cloneMap(cfg.Profiles)
	if raw.Claude != "" {
		cfg.Claude = raw.Claude
	}
	for key, prim := range raw.Models {
		if !modelKeyRe.MatchString(key) {
			return Config{}, fmt.Errorf("%s: invalid model key %q (lowercase letters only)", path, key)
		}
		var name string
		if err := md.PrimitiveDecode(prim, &name); err == nil {
			if name == "" {
				return Config{}, fmt.Errorf("%s: model %q: empty model name", path, key)
			}
			cfg.Models[key] = ModelSpec{Model: name, Efforts: true}
			continue
		}
		var rm rawModel
		if err := md.PrimitiveDecode(prim, &rm); err != nil {
			return Config{}, fmt.Errorf("%s: model %q: %w", path, key, err)
		}
		if rm.Model == "" {
			return Config{}, fmt.Errorf("%s: model %q: missing model name", path, key)
		}
		spec := ModelSpec{Model: rm.Model, Efforts: true}
		if rm.Efforts != nil {
			spec.Efforts = *rm.Efforts
		}
		cfg.Models[key] = spec
	}
	if len(raw.Efforts) > 0 {
		cfg.Efforts = map[string]string{}
		for key, level := range raw.Efforts {
			if !effortKeyRe.MatchString(key) {
				return Config{}, fmt.Errorf("%s: invalid effort key %q (single digit 1-9)", path, key)
			}
			if level == "" {
				return Config{}, fmt.Errorf("%s: effort %q: empty level", path, key)
			}
			cfg.Efforts[key] = level
		}
	}
	for name, rp := range raw.Profiles {
		if !profileNameRe.MatchString(name) {
			return Config{}, fmt.Errorf("%s: invalid profile name %q (want ^[a-z][a-z0-9-]*$)", path, name)
		}
		cfg.Profiles[name] = Profile{
			Model: rp.Model, Effort: rp.Effort, Flags: rp.Flags,
			Env: rp.Env, Claude: rp.Claude,
		}
	}
	if undec := md.Undecoded(); len(undec) > 0 {
		keys := make([]string, len(undec))
		for i, k := range undec {
			keys[i] = k.String()
		}
		sort.Strings(keys)
		return Config{}, fmt.Errorf("%s: unknown key(s): %v", path, keys)
	}
	return cfg, nil
}
