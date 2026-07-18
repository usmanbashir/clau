package main

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestLinkNames(t *testing.T) {
	cfg := defaultConfig()
	cfg.Profiles["rev"] = Profile{Model: "opus"}
	got := linkNames(cfg)
	want := []string{
		"c",
		"cf", "cf1", "cf2", "cf3", "cf4", "cf5",
		"ch", // efforts=false: no ch1..ch5
		"co", "co1", "co2", "co3", "co4", "co5",
		"crev",
		"cs", "cs1", "cs2", "cs3", "cs4", "cs5",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("linkNames =\n%v\nwant\n%v", got, want)
	}
}

func TestShimContent(t *testing.T) {
	got := shimContent(`C:\bin\clau.exe`, "o5")
	for _, want := range []string{":: clau-shim", `"C:\bin\clau.exe" run o5 %*`} {
		if !strings.Contains(got, want) {
			t.Errorf("shim missing %q:\n%s", want, got)
		}
	}
	bare := shimContent(`C:\bin\clau.exe`, "")
	if !strings.Contains(bare, `"C:\bin\clau.exe" __launch %*`) {
		t.Errorf("bare shim:\n%s", bare)
	}
}

func fakeClauBinary(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "clau")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSyncLinksCreatesAndPrunes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink test")
	}
	clauPath := fakeClauBinary(t)
	dir := t.TempDir()
	t.Setenv("PATH", dir) // nothing foreign anywhere
	cfg := defaultConfig()
	cfg.Profiles["rev"] = Profile{Model: "opus"}

	rep, err := syncLinks(cfg, dir, clauPath, runtime.GOOS, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Created) != 21 || len(rep.Pruned) != 0 {
		t.Fatalf("report = %+v", rep)
	}
	if target, err := os.Readlink(filepath.Join(dir, "co5")); err != nil || target != clauPath {
		t.Errorf("co5 -> %q, %v", target, err)
	}

	// Second sync: everything kept, nothing created.
	rep, err = syncLinks(cfg, dir, clauPath, runtime.GOOS, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Created) != 0 || len(rep.Kept) != 21 {
		t.Fatalf("resync report = %+v", rep)
	}

	// Remove the profile: crev gets pruned.
	delete(cfg.Profiles, "rev")
	rep, err = syncLinks(cfg, dir, clauPath, runtime.GOOS, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rep.Pruned, []string{"crev"}) {
		t.Fatalf("pruned = %v", rep.Pruned)
	}
	if _, err := os.Lstat(filepath.Join(dir, "crev")); !os.IsNotExist(err) {
		t.Error("crev still exists")
	}
}

func TestSyncLinksSkipsForeignCollision(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink test")
	}
	clauPath := fakeClauBinary(t)
	dir := t.TempDir()
	other := t.TempDir()
	// A foreign executable named "cat" on PATH, and a profile "at" that
	// would generate the command name "cat".
	if err := os.WriteFile(filepath.Join(other, "cat"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", other)
	cfg := defaultConfig()
	cfg.Profiles["at"] = Profile{Model: "opus"}

	rep, err := syncLinks(cfg, dir, clauPath, runtime.GOOS, false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range rep.Skipped {
		if strings.Contains(s, "cat") {
			found = true
		}
	}
	if !found {
		t.Fatalf("cat not skipped: %+v", rep)
	}
	if _, err := os.Lstat(filepath.Join(dir, "cat")); !os.IsNotExist(err) {
		t.Error("cat link was created over a foreign binary")
	}

	// --force creates it anyway.
	rep, err = syncLinks(cfg, dir, clauPath, runtime.GOOS, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "cat")); err != nil {
		t.Error("--force did not create the link")
	}
}

func TestSyncLinksNeverTouchesForeignFilesInDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink test")
	}
	clauPath := fakeClauBinary(t)
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	// A foreign regular file occupying a desired name.
	if err := os.WriteFile(filepath.Join(dir, "co"), []byte("mine"), 0o755); err != nil {
		t.Fatal(err)
	}
	rep, err := syncLinks(defaultConfig(), dir, clauPath, runtime.GOOS, false)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "co"))
	if string(data) != "mine" {
		t.Error("foreign file was overwritten without --force")
	}
	found := false
	for _, s := range rep.Skipped {
		if strings.Contains(s, "co") {
			found = true
		}
	}
	if !found {
		t.Errorf("co not reported skipped: %+v", rep)
	}
}

func TestSyncLinksWindowsWritesShims(t *testing.T) {
	clauPath := filepath.Join(t.TempDir(), "clau.exe")
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	cfg := defaultConfig()
	rep, err := syncLinks(cfg, dir, clauPath, "windows", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Created) == 0 {
		t.Fatalf("report = %+v", rep)
	}
	data, err := os.ReadFile(filepath.Join(dir, "co5.cmd"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "run o5 %*") || !strings.Contains(string(data), ":: clau-shim") {
		t.Errorf("shim content:\n%s", data)
	}
	bare, err := os.ReadFile(filepath.Join(dir, "c.cmd"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bare), "__launch %*") {
		t.Errorf("bare shim content:\n%s", bare)
	}
}

func TestRemoveOwned(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink test")
	}
	clauPath := fakeClauBinary(t)
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	if _, err := syncLinks(defaultConfig(), dir, clauPath, runtime.GOOS, false); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "unrelated"), []byte("x"), 0o755)
	removed, err := removeOwned(dir, clauPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 20 { // linkNames(defaultConfig()) yields 20 names, all owned
		t.Errorf("removed %d: %v", len(removed), removed)
	}
	if _, err := os.Stat(filepath.Join(dir, "unrelated")); err != nil {
		t.Error("unrelated file was removed")
	}
}

func TestIsOwnedBasenameFallbackOnlyWhenDangling(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink test")
	}
	clauPath := fakeClauBinary(t)
	dir := t.TempDir()
	otherDir := t.TempDir()
	other := filepath.Join(otherDir, "clau")
	if err := os.WriteFile(other, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(dir, "foo")
	if err := os.Symlink(other, live); err != nil {
		t.Fatal(err)
	}
	if isOwned(live, clauPath) {
		t.Error("live foreign clau-named target must not be owned")
	}
	dangling := filepath.Join(dir, "co5")
	if err := os.Symlink(filepath.Join(otherDir, "gone", "clau"), dangling); err != nil {
		t.Fatal(err)
	}
	if !isOwned(dangling, clauPath) {
		t.Error("dangling clau-named target must be owned")
	}
	rel := filepath.Join(dir, "cs3")
	if err := os.Symlink("missing/clau", rel); err != nil {
		t.Fatal(err)
	}
	if !isOwned(rel, clauPath) {
		t.Error("relative dangling clau-named target must be owned")
	}
}

// TestClauNamedEntryIsNeverTouched guards against a real hazard: a user's
// own hand-made `~/.local/bin/clau -> <clau binary>` symlink is exact-path
// "owned" by isOwned's definition, so without a guard syncLinks would prune
// it, removeOwned would delete it, and doctor would misparse its "token" as
// the name with a leading "c" stripped ("lau") and warn about a stale link.
// An entry literally named clau (or clau.cmd) must be left alone by all
// three.
func TestClauNamedEntryIsNeverTouched(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink test")
	}
	clauPath := fakeClauBinary(t)
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	clauLink := filepath.Join(dir, "clau")
	if err := os.Symlink(clauPath, clauLink); err != nil {
		t.Fatal(err)
	}

	rep, err := syncLinks(defaultConfig(), dir, clauPath, runtime.GOOS, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range [][]string{rep.Created, rep.Kept, rep.Skipped, rep.Pruned} {
		for _, name := range group {
			if name == "clau" {
				t.Errorf("syncLinks touched clau: %+v", rep)
			}
		}
	}
	if target, err := os.Readlink(clauLink); err != nil || target != clauPath {
		t.Fatalf("clau symlink disturbed by syncLinks: target=%q err=%v", target, err)
	}

	removed, err := removeOwned(dir, clauPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range removed {
		if name == "clau" {
			t.Errorf("removeOwned removed clau: %v", removed)
		}
	}
	if target, err := os.Readlink(clauLink); err != nil || target != clauPath {
		t.Fatalf("clau symlink removed by removeOwned: target=%q err=%v", target, err)
	}

	fs := doctorFindings(defaultConfig(), nil, dir, clauPath, runtime.GOOS)
	for _, f := range fs {
		if strings.Contains(f.Msg, "stale link clau") {
			t.Errorf("doctor flagged clau as a stale link: %s", f.Msg)
		}
	}
}

// TestForeignInPathWindowsDetectsWithoutExecBit covers the Windows branch
// of foreignInPath: a .cmd file has no meaningful exec permission bit on
// Windows, so detection must rely purely on the file existing under a
// recognized executable extension. On any other goos, that same file must
// not be found at all: no extension search happens there.
func TestForeignInPathWindowsDetectsWithoutExecBit(t *testing.T) {
	clauPath := fakeClauBinary(t)
	linkDir := t.TempDir()
	pathDir := t.TempDir()
	t.Setenv("PATH", pathDir)
	t.Setenv("PATHEXT", "") // force the built-in default extension list

	cmdFile := filepath.Join(pathDir, "foo.cmd")
	if err := os.WriteFile(cmdFile, []byte("@echo off\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := foreignInPath("foo", linkDir, clauPath, "windows"); got != cmdFile {
		t.Errorf("windows: foreignInPath(%q) = %q, want %q", "foo", got, cmdFile)
	}
	if got := foreignInPath("foo", linkDir, clauPath, "linux"); got != "" {
		t.Errorf("linux: foreignInPath(%q) = %q, want \"\" (no extension search, no exec bit)", "foo", got)
	}
}

// TestForeignInPathHonorsPathext checks that the Windows candidate
// extension list comes from $PATHEXT when it's set, instead of always
// falling back to the .com/.exe/.bat/.cmd default.
func TestForeignInPathHonorsPathext(t *testing.T) {
	clauPath := fakeClauBinary(t)
	linkDir := t.TempDir()
	pathDir := t.TempDir()
	t.Setenv("PATH", pathDir)
	t.Setenv("PATHEXT", ".XYZ")

	custom := filepath.Join(pathDir, "foo.xyz")
	if err := os.WriteFile(custom, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Not in PATHEXT, so it must be ignored even though it would match
	// the built-in default list.
	if err := os.WriteFile(filepath.Join(pathDir, "foo.cmd"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := foreignInPath("foo", linkDir, clauPath, "windows"); got != custom {
		t.Errorf("foreignInPath = %q, want %q (PATHEXT-driven extension list)", got, custom)
	}
}

// TestForeignInPathWindowsBareNameNeedsExecExtension checks the bare-name
// fallback: an extensionless file is never a candidate on Windows, but if
// the queried name already carries a recognized executable extension, the
// unmodified name itself is also tried (not just name+ext).
func TestForeignInPathWindowsBareNameNeedsExecExtension(t *testing.T) {
	clauPath := fakeClauBinary(t)
	linkDir := t.TempDir()
	pathDir := t.TempDir()
	t.Setenv("PATH", pathDir)
	t.Setenv("PATHEXT", "")

	if err := os.WriteFile(filepath.Join(pathDir, "co5"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := foreignInPath("co5", linkDir, clauPath, "windows"); got != "" {
		t.Errorf("windows: extensionless file must not be a candidate for a bare token: got %q", got)
	}

	preExtd := filepath.Join(pathDir, "tool.exe")
	if err := os.WriteFile(preExtd, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := foreignInPath("tool.exe", linkDir, clauPath, "windows"); got != preExtd {
		t.Errorf("windows: foreignInPath(%q) = %q, want %q", "tool.exe", got, preExtd)
	}
}
