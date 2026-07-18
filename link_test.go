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
