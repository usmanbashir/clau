package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// clauBin and fakeClaudeDir are built once in TestMain and shared read-only
// across every test in this file, instead of each test shelling out to `go
// build` for its own private copy.
var (
	clauBin       string
	fakeClaudeDir string
)

func TestMain(m *testing.M) {
	os.Exit(runIntegrationTests(m))
}

// runIntegrationTests builds clau and the fake `claude` (see
// testdata/fakeclaude) once into a shared temp dir, then runs the test
// suite. It's a separate function from TestMain so the `defer
// os.RemoveAll` actually runs: os.Exit skips deferred calls, so TestMain
// itself must not defer anything.
func runIntegrationTests(m *testing.M) int {
	dir, err := os.MkdirTemp("", "clau-integration")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)

	clauBin = filepath.Join(dir, "clau")
	fakeClaudeName := "claude"
	if runtime.GOOS == "windows" {
		clauBin += ".exe"
		fakeClaudeName = "claude.exe"
	}
	fakeClaudeDir = dir

	if out, err := exec.Command("go", "build", "-o", clauBin, ".").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build clau: %v\n%s", err, out)
		return 1
	}
	if out, err := exec.Command("go", "build", "-o", filepath.Join(fakeClaudeDir, fakeClaudeName), "./testdata/fakeclaude").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build fake claude: %v\n%s", err, out)
		return 1
	}

	return m.Run()
}

// buildClau returns the path to the clau binary built once in TestMain.
// It keeps the `bin := buildClau(t)` call sites in every test below intact.
func buildClau(t *testing.T) string {
	t.Helper()
	return clauBin
}

type recorded struct {
	argv []string
	env  map[string]string
}

func runShortcut(t *testing.T, bin, invokeAs, configTOML string, args ...string) recorded {
	t.Helper()
	dir := t.TempDir()
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
		// The fake claude lives in the shared dir built once by TestMain,
		// not in this test's own per-run dir.
		"PATH="+fakeClaudeDir+string(os.PathListSeparator)+os.Getenv("PATH"),
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
	if len(rec.argv) != 0 {
		t.Errorf("zero-arg launch recorded argv %q, want none", rec.argv)
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
