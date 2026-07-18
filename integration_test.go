package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func buildClau(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "clau")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

// writeFakeClaude installs a fake `claude` in dir that records its argv and
// selected env vars into $CLAU_TEST_OUT, one line each.
func writeFakeClaude(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		script := "@echo off\r\n(echo ARGV %*) > \"%CLAU_TEST_OUT%\"\r\n"
		if err := os.WriteFile(filepath.Join(dir, "claude.cmd"), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		return
	}
	script := `#!/bin/sh
{
  printf 'ARGV'
  for a in "$@"; do printf '\t%s' "$a"; done
  printf '\n'
  printf 'ENV\tANTHROPIC_BASE_URL=%s\n' "$ANTHROPIC_BASE_URL"
} > "$CLAU_TEST_OUT"
`
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

type recorded struct {
	argv []string
	env  map[string]string
}

func runShortcut(t *testing.T, bin, invokeAs, configTOML string, args ...string) recorded {
	t.Helper()
	dir := t.TempDir()
	writeFakeClaude(t, dir)
	outFile := filepath.Join(dir, "out.txt")
	cfgFile := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgFile, []byte(configTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	target := bin
	if invokeAs != "" && invokeAs != "clau" {
		if runtime.GOOS == "windows" {
			t.Skip("argv[0] symlink dispatch not exercised on windows")
		}
		target = filepath.Join(dir, invokeAs)
		if err := os.Symlink(bin, target); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(target, args...)
	cmd.Env = append(os.Environ(),
		"PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"CLAU_TEST_OUT="+outFile,
		"CLAU_CONFIG="+cfgFile,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run %s %v: %v\n%s", target, args, err, out)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("fake claude never ran: %v\n%s", err, out)
	}
	rec := recorded{env: map[string]string{}}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Split(line, "\t")
		switch fields[0] {
		case "ARGV":
			rec.argv = fields[1:]
		case "ENV":
			k, v, _ := strings.Cut(fields[1], "=")
			rec.env[k] = v
		}
	}
	return rec
}

const integrationConfig = `
[profiles.rev]
model = "opus"
effort = "high"
flags = ["--append-system-prompt", "Review code."]

[profiles.work]
env = { ANTHROPIC_BASE_URL = "https://gw.example" }
`

func TestIntegrationArgStyle(t *testing.T) {
	bin := buildClau(t)
	rec := runShortcut(t, bin, "clau", integrationConfig, "o5", "hello")
	want := "--model\topus\t--effort\tmax\thello"
	if got := strings.Join(rec.argv, "\t"); got != want {
		t.Errorf("argv = %q, want %q", got, want)
	}
}

func TestIntegrationSymlinkStyle(t *testing.T) {
	bin := buildClau(t)
	rec := runShortcut(t, bin, "cs3", integrationConfig, "hi")
	want := "--model\tsonnet\t--effort\thigh\thi"
	if got := strings.Join(rec.argv, "\t"); got != want {
		t.Errorf("argv = %q, want %q", got, want)
	}
}

func TestIntegrationProfileViaSymlink(t *testing.T) {
	bin := buildClau(t)
	rec := runShortcut(t, bin, "crev", integrationConfig)
	want := "--model\topus\t--effort\thigh\t--append-system-prompt\tReview code."
	if got := strings.Join(rec.argv, "\t"); got != want {
		t.Errorf("argv = %q, want %q", got, want)
	}
}

func TestIntegrationPassthrough(t *testing.T) {
	bin := buildClau(t)
	rec := runShortcut(t, bin, "c", integrationConfig, "fix this bug", "-x")
	want := "fix this bug\t-x"
	if got := strings.Join(rec.argv, "\t"); got != want {
		t.Errorf("argv = %q, want %q", got, want)
	}
}

func TestIntegrationDoubleDashEscape(t *testing.T) {
	bin := buildClau(t)
	rec := runShortcut(t, bin, "c", integrationConfig, "--", "o5")
	if got := strings.Join(rec.argv, "\t"); got != "o5" {
		t.Errorf("argv = %q, want o5 alone", got)
	}
}

func TestIntegrationProfileEnv(t *testing.T) {
	bin := buildClau(t)
	rec := runShortcut(t, bin, "cwork", integrationConfig)
	if rec.env["ANTHROPIC_BASE_URL"] != "https://gw.example" {
		t.Errorf("env = %v", rec.env)
	}
}

func TestIntegrationStaleNameErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink test")
	}
	bin := buildClau(t)
	dir := t.TempDir()
	link := filepath.Join(dir, "cgone")
	if err := os.Symlink(bin, link); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(link)
	cmd.Env = append(os.Environ(), "CLAU_CONFIG="+filepath.Join(dir, "none.toml"))
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected failure, got: %s", out)
	}
	if !strings.Contains(string(out), `shortcut "gone" no longer exists`) {
		t.Errorf("stderr = %s", out)
	}
}

func TestIntegrationBrokenConfigFailsEvenOnPassthrough(t *testing.T) {
	bin := buildClau(t)
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	os.WriteFile(cfg, []byte("[models\n"), 0o644)
	cmd := exec.Command(bin, "hello")
	cmd.Env = append(os.Environ(), "CLAU_CONFIG="+cfg)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected failure, got: %s", out)
	}
	if !strings.Contains(string(out), "clau:") {
		t.Errorf("stderr = %s", out)
	}
}
