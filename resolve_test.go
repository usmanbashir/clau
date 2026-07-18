package main

import (
	"reflect"
	"strings"
	"testing"
)

func testConfig() Config {
	cfg := defaultConfig()
	cfg.Profiles["rev"] = Profile{
		Model: "opus", Effort: "high",
		Flags: []string{"--append-system-prompt", "Review code."},
	}
	cfg.Profiles["work"] = Profile{
		Env:    map[string]string{"ANTHROPIC_BASE_URL": "https://x"},
		Claude: "claude-alt",
	}
	cfg.Profiles["o5"] = Profile{Model: "shadow"} // profile shadows grammar
	return cfg
}

func TestResolveToken(t *testing.T) {
	cfg := testConfig()
	cases := []struct {
		token     string
		wantFlags []string
		wantFound bool
		wantErr   string
	}{
		{"o", []string{"--model", "opus"}, true, ""},
		{"s3", []string{"--model", "sonnet", "--effort", "high"}, true, ""},
		{"f5", []string{"--model", "fable", "--effort", "max"}, true, ""},
		{"h", []string{"--model", "haiku"}, true, ""},
		{"h3", nil, true, "no effort levels"},
		{"o5", []string{"--model", "shadow"}, true, ""}, // profile wins
		{"rev", []string{"--model", "opus", "--effort", "high", "--append-system-prompt", "Review code."}, true, ""},
		{"x", nil, false, ""},
		{"x5", nil, false, ""},
		{"o9", nil, false, ""}, // 9 not in ladder, "o9" not a model key
		{"", nil, false, ""},
		{"-c", nil, false, ""},
		{"fix this bug", nil, false, ""},
	}
	for _, c := range cases {
		t.Run(c.token, func(t *testing.T) {
			res, found, err := resolveToken(cfg, c.token)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want %q", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if found != c.wantFound {
				t.Fatalf("found = %v, want %v", found, c.wantFound)
			}
			if found && !reflect.DeepEqual(res.Flags, c.wantFlags) {
				t.Errorf("flags = %v, want %v", res.Flags, c.wantFlags)
			}
		})
	}
}

func TestResolveTokenProfileCarriesEnvAndClaude(t *testing.T) {
	res, found, err := resolveToken(testConfig(), "work")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if res.Env["ANTHROPIC_BASE_URL"] != "https://x" || res.Claude != "claude-alt" {
		t.Errorf("res = %+v", res)
	}
	if len(res.Flags) != 0 {
		t.Errorf("flags = %v, want none", res.Flags)
	}
}

func TestResolveTokenCustomLadder(t *testing.T) {
	cfg := defaultConfig()
	cfg.Efforts = map[string]string{"7": "turbo"}
	res, found, err := resolveToken(cfg, "o7")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	want := []string{"--model", "opus", "--effort", "turbo"}
	if !reflect.DeepEqual(res.Flags, want) {
		t.Errorf("flags = %v, want %v", res.Flags, want)
	}
}
