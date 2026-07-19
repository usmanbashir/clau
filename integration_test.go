package main

import (
	"crypto/sha256"
	"encoding/hex"
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
		"CLAU_NO_PROJECT=1",
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
	cmd.Env = append(os.Environ(), "CLAU_CONFIG="+filepath.Join(dir, "none.toml"), "CLAU_NO_PROJECT=1")
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
	cmd.Env = append(os.Environ(), "CLAU_CONFIG="+cfg, "CLAU_NO_PROJECT=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected failure, got: %s", out)
	}
	if !strings.Contains(string(out), "clau:") {
		t.Errorf("stderr = %s", out)
	}
}

// runInProject runs bin with cwd set to dir, CLAU_CONFIG pointing at a
// global config written from globalTOML, XDG_STATE_HOME at state, and
// the fake claude on PATH. It returns combined output, the error, and
// whatever the fake claude recorded (zero recorded if it never ran).
func runInProject(t *testing.T, bin, dir, globalTOML, state string, extraEnv []string, args ...string) (string, error, recorded) {
	t.Helper()
	outFile := filepath.Join(t.TempDir(), "out.txt")
	cfgFile := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgFile, []byte(globalTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"PATH="+fakeClaudeDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"CLAU_TEST_OUT="+outFile,
		"CLAU_CONFIG="+cfgFile,
		"XDG_STATE_HOME="+state,
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	out, err := cmd.CombinedOutput()
	rec := recorded{env: map[string]string{}}
	if data, rerr := os.ReadFile(outFile); rerr == nil {
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
	}
	return string(out), err, rec
}

// seedTrust writes a trust store in state covering the given project file.
func seedTrust(t *testing.T, state, proj string) {
	t.Helper()
	data, err := os.ReadFile(proj)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	dir := filepath.Join(state, "clau")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := fmt.Sprintf("[trusted]\n%q = %q\n", proj, hex.EncodeToString(sum[:]))
	if err := os.WriteFile(filepath.Join(dir, "trust.toml"), []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeProject(t *testing.T, body string) (string, string) {
	t.Helper()
	// Resolve symlinks so trust-store keys and output assertions use the
	// same path the clau child sees via os.Getwd (macOS /var -> /private/var).
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(dir, ".clau.toml")
	if err := os.WriteFile(proj, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, proj
}

const projectGlobal = "[profiles.rev]\nmodel = \"opus\"\n"
const projectLayer = "[profiles.rev]\nmodel = \"haiku\"\n"

func TestIntegrationProjectTrustedOverrides(t *testing.T) {
	bin := buildClau(t)
	dir, proj := writeProject(t, projectLayer)
	state := t.TempDir()
	seedTrust(t, state, proj)
	out, err, rec := runInProject(t, bin, dir, projectGlobal, state, nil, "rev")
	if err != nil {
		t.Fatalf("launch failed: %v\n%s", err, out)
	}
	if got := strings.Join(rec.argv, "\t"); got != "--model\thaiku" {
		t.Errorf("argv = %q, want project override --model haiku", got)
	}
}

func TestIntegrationProjectUntrustedFails(t *testing.T) {
	bin := buildClau(t)
	dir, _ := writeProject(t, projectLayer)
	out, err, rec := runInProject(t, bin, dir, projectGlobal, t.TempDir(), nil, "rev")
	if err == nil {
		t.Fatalf("expected failure, got: %s", out)
	}
	if !strings.Contains(out, "not trusted") || !strings.Contains(out, "clau trust") {
		t.Errorf("stderr = %s", out)
	}
	if len(rec.argv) != 0 || len(rec.env) != 0 {
		t.Errorf("claude must not run untrusted, recorded %+v", rec)
	}
}

func TestIntegrationProjectChangedFails(t *testing.T) {
	bin := buildClau(t)
	dir, proj := writeProject(t, projectLayer)
	state := t.TempDir()
	seedTrust(t, state, proj)
	if err := os.WriteFile(proj, []byte("[profiles.rev]\nmodel = \"sonnet\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err, rec := runInProject(t, bin, dir, projectGlobal, state, nil, "rev")
	if err == nil {
		t.Fatalf("expected failure, got: %s", out)
	}
	if !strings.Contains(out, "changed since it was trusted") {
		t.Errorf("stderr = %s", out)
	}
	if len(rec.argv) != 0 || len(rec.env) != 0 {
		t.Errorf("claude must not run on a changed project config, recorded %+v", rec)
	}
}

func TestIntegrationNoProjectEnvSkips(t *testing.T) {
	bin := buildClau(t)
	dir, _ := writeProject(t, projectLayer)
	out, err, rec := runInProject(t, bin, dir, projectGlobal, t.TempDir(),
		[]string{"CLAU_NO_PROJECT=1"}, "rev")
	if err != nil {
		t.Fatalf("launch failed: %v\n%s", err, out)
	}
	if got := strings.Join(rec.argv, "\t"); got != "--model\topus" {
		t.Errorf("argv = %q, want global --model opus", got)
	}
}

func TestIntegrationTrustVerbFlow(t *testing.T) {
	bin := buildClau(t)
	dir, proj := writeProject(t, projectLayer)
	state := t.TempDir()

	out, err, _ := runInProject(t, bin, dir, projectGlobal, state, nil, "trust", "--show")
	if err != nil {
		t.Fatalf("trust --show: %v\n%s", err, out)
	}
	if !strings.Contains(out, proj) || !strings.Contains(out, "haiku") {
		t.Errorf("trust --show output = %s", out)
	}

	out, err, _ = runInProject(t, bin, dir, projectGlobal, state, nil, "trust")
	if err != nil {
		t.Fatalf("trust: %v\n%s", err, out)
	}
	if !strings.Contains(out, "trusted "+proj) {
		t.Errorf("trust output = %s", out)
	}

	out, err, rec := runInProject(t, bin, dir, projectGlobal, state, nil, "rev")
	if err != nil {
		t.Fatalf("launch after trust: %v\n%s", err, out)
	}
	if got := strings.Join(rec.argv, "\t"); got != "--model\thaiku" {
		t.Errorf("argv = %q, want --model haiku", got)
	}

	out, err, _ = runInProject(t, bin, dir, projectGlobal, state, nil, "untrust")
	if err != nil {
		t.Fatalf("untrust: %v\n%s", err, out)
	}
	out, err, _ = runInProject(t, bin, dir, projectGlobal, state, nil, "rev")
	if err == nil {
		t.Fatalf("launch after untrust must fail, got: %s", out)
	}
}

func TestIntegrationTrustRefusesBrokenFile(t *testing.T) {
	bin := buildClau(t)
	dir, _ := writeProject(t, "[models\n")
	out, err, _ := runInProject(t, bin, dir, projectGlobal, t.TempDir(), nil, "trust")
	if err == nil {
		t.Fatalf("expected refusal, got: %s", out)
	}
	if !strings.Contains(out, "refusing to trust") {
		t.Errorf("stderr = %s", out)
	}
}

func TestIntegrationListShowsProvenance(t *testing.T) {
	bin := buildClau(t)
	dir, proj := writeProject(t, projectLayer)
	state := t.TempDir()

	out, err, _ := runInProject(t, bin, dir, projectGlobal, state, nil, "list")
	if err != nil {
		t.Fatalf("list untrusted: %v\n%s", err, out)
	}
	// The default h model row always contains "haiku", so assert on the
	// rev row specifically, not on the whole output.
	if !strings.Contains(out, "NOT trusted") || strings.Contains(out, "SOURCE") {
		t.Errorf("untrusted list must warn and stay in the global shape:\n%s", out)
	}
	if line := lineContaining(out, "rev"); !strings.Contains(line, "opus") {
		t.Errorf("untrusted rev row must show the global launch:\n%s", out)
	}

	seedTrust(t, state, proj)
	out, err, _ = runInProject(t, bin, dir, projectGlobal, state, nil, "list")
	if err != nil {
		t.Fatalf("list trusted: %v\n%s", err, out)
	}
	if !strings.Contains(out, "SOURCE") {
		t.Errorf("trusted list must show the SOURCE column:\n%s", out)
	}
	if line := lineContaining(out, "rev"); !strings.Contains(line, "haiku") || !strings.Contains(line, "project") {
		t.Errorf("trusted rev row must show the project override and source:\n%s", out)
	}
}

// TestIntegrationLinkIgnoresProjectProfiles guards that `clau link` stays
// global-only: even a trusted project file defining its own profile must
// not produce a link for it, while global tokens still link normally.
func TestIntegrationLinkIgnoresProjectProfiles(t *testing.T) {
	bin := buildClau(t)
	dir, proj := writeProject(t, projectLayer+"\n[profiles.deploy]\nflags = [\"-p\"]\n")
	state := t.TempDir()
	seedTrust(t, state, proj)
	linkDir := t.TempDir()

	out, err, _ := runInProject(t, bin, dir, projectGlobal, state, nil, "link", "--dir", linkDir)
	if err != nil {
		t.Fatalf("link: %v\n%s", err, out)
	}

	entries, rerr := os.ReadDir(linkDir)
	if rerr != nil {
		t.Fatal(rerr)
	}
	names := make(map[string]bool, len(entries))
	for _, e := range entries {
		names[e.Name()] = true
	}
	has := func(base string) bool {
		// On Windows, link files get a .cmd suffix.
		return names[base] || names[base+".cmd"]
	}
	if has("cdeploy") {
		t.Errorf("clau link must ignore project-only profiles, but cdeploy exists: %v", names)
	}
	if !has("crev") {
		t.Errorf("clau link must still link global tokens, crev missing: %v", names)
	}
}

// lineContaining returns the first output line containing substr.
func lineContaining(out, substr string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, substr) {
			return line
		}
	}
	return ""
}
