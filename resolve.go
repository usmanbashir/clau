package main

import "fmt"

type TokenResolution struct {
	Flags  []string
	Env    map[string]string
	Claude string
}

// resolveToken maps a token to claude flags/env. Exact profile-name match
// wins over the grammar. found=false means the token is not a shortcut;
// err is non-nil only for a matched-but-invalid token.
func resolveToken(cfg Config, token string) (TokenResolution, bool, error) {
	if token == "" {
		return TokenResolution{}, false, nil
	}
	if p, ok := cfg.Profiles[token]; ok {
		var flags []string
		if p.Model != "" {
			flags = append(flags, "--model", p.Model)
		}
		if p.Effort != "" {
			flags = append(flags, "--effort", p.Effort)
		}
		flags = append(flags, p.Flags...)
		return TokenResolution{Flags: flags, Env: p.Env, Claude: p.Claude}, true, nil
	}
	last := token[len(token)-1]
	if last >= '0' && last <= '9' {
		level, inLadder := cfg.Efforts[string(last)]
		rest := token[:len(token)-1]
		spec, isModel := cfg.Models[rest]
		if inLadder && isModel {
			if !spec.Efforts {
				return TokenResolution{}, true,
					fmt.Errorf("model %q (%s) supports no effort levels", rest, spec.Model)
			}
			return TokenResolution{Flags: []string{"--model", spec.Model, "--effort", level}}, true, nil
		}
		return TokenResolution{}, false, nil
	}
	if spec, ok := cfg.Models[token]; ok {
		return TokenResolution{Flags: []string{"--model", spec.Model}}, true, nil
	}
	return TokenResolution{}, false, nil
}
