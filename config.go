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
	Bases    map[string]Profile
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
		Bases:    map[string]Profile{},
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
	Model   string            `toml:"model"`
	Effort  string            `toml:"effort"`
	Flags   []string          `toml:"flags"`
	Env     map[string]string `toml:"env"`
	Claude  string            `toml:"claude"`
	Extends *string           `toml:"extends"`
}

// rawBase is rawProfile without extends: bases cannot chain, and the
// missing field makes `extends` inside [bases.*] an unknown-key error.
type rawBase struct {
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
	Bases    map[string]rawBase        `toml:"bases"`
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

// flattenProfile fills child fields from base where the child leaves
// them unset: scalars fill in, flags concatenate base-first, env merges
// key-wise with the child winning. The result shares no slice or map
// storage with base, so later layers cannot mutate stored bases.
func flattenProfile(base, child Profile) Profile {
	out := child
	if out.Model == "" {
		out.Model = base.Model
	}
	if out.Effort == "" {
		out.Effort = base.Effort
	}
	if out.Claude == "" {
		out.Claude = base.Claude
	}
	if len(base.Flags) > 0 {
		out.Flags = append(append([]string{}, base.Flags...), child.Flags...)
	}
	if len(base.Env) > 0 {
		env := cloneMap(base.Env)
		for k, v := range child.Env {
			env[k] = v
		}
		out.Env = env
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
// bases and profiles per name (wholesale) — each profile's extends
// flattened against the bases accumulated so far, then discarded — a
// non-empty efforts table replaces the ladder, claude if set. path is
// used only to name errors, so a caller
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
	cfg.Bases = cloneMap(cfg.Bases)
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
	for name, rb := range raw.Bases {
		if !profileNameRe.MatchString(name) {
			return Config{}, fmt.Errorf("%s: invalid base name %q (want ^[a-z][a-z0-9-]*$)", path, name)
		}
		cfg.Bases[name] = Profile{
			Model: rb.Model, Effort: rb.Effort, Flags: rb.Flags,
			Env: rb.Env, Claude: rb.Claude,
		}
	}
	for name, rp := range raw.Profiles {
		if !profileNameRe.MatchString(name) {
			return Config{}, fmt.Errorf("%s: invalid profile name %q (want ^[a-z][a-z0-9-]*$)", path, name)
		}
		p := Profile{
			Model: rp.Model, Effort: rp.Effort, Flags: rp.Flags,
			Env: rp.Env, Claude: rp.Claude,
		}
		if rp.Extends != nil {
			parent := *rp.Extends
			if parent == "" {
				return Config{}, fmt.Errorf("%s: profile %q: empty extends", path, name)
			}
			base, ok := cfg.Bases[parent]
			if !ok {
				_, inFile := raw.Profiles[parent]
				_, inCfg := cfg.Profiles[parent]
				if inFile || inCfg {
					return Config{}, fmt.Errorf("%s: profile %q: extends %q, which is a profile; extends references [bases]", path, name, parent)
				}
				return Config{}, fmt.Errorf("%s: profile %q: extends unknown base %q", path, name, parent)
			}
			p = flattenProfile(base, p)
		}
		cfg.Profiles[name] = p
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
