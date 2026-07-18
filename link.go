package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func linkNames(cfg Config) []string {
	names := []string{"c"}
	digits := make([]string, 0, len(cfg.Efforts))
	for d := range cfg.Efforts {
		digits = append(digits, d)
	}
	sort.Strings(digits)
	for letter, spec := range cfg.Models {
		names = append(names, "c"+letter)
		if spec.Efforts {
			for _, d := range digits {
				names = append(names, "c"+letter+d)
			}
		}
	}
	for name := range cfg.Profiles {
		names = append(names, "c"+name)
	}
	sort.Strings(names)
	return names
}

func defaultLinkDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".local", "bin")
	}
	return filepath.Join(home, ".local", "bin")
}

func clauExecutable() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved, nil
	}
	return p, nil
}

const shimMarker = ":: clau-shim"

func shimContent(clauPath, token string) string {
	invoke := "__launch"
	if token != "" {
		invoke = "run " + token
	}
	return "@echo off\r\n" + shimMarker + "\r\n\"" + clauPath + "\" " + invoke + " %*\r\n"
}

// isOwned reports whether path is a link or shim created by clau.
func isOwned(path, clauPath string) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return false
		}
		if target == clauPath {
			return true
		}
		if strings.TrimSuffix(filepath.Base(target), ".exe") != "clau" {
			return false
		}
		// Basename fallback only for dangling targets: a brew upgrade
		// removed the old binary. A live target that isn't ours is foreign.
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		_, statErr := os.Stat(target)
		return os.IsNotExist(statErr)
	}
	if strings.HasSuffix(path, ".cmd") {
		data, err := os.ReadFile(path)
		return err == nil && strings.Contains(string(data), shimMarker)
	}
	return false
}

// defaultWindowsExecExts is used when $PATHEXT is unset or empty.
var defaultWindowsExecExts = []string{".com", ".exe", ".bat", ".cmd"}

// windowsExecExts returns the candidate executable extensions for Windows,
// lowercased: $PATHEXT split on ";" if set and non-empty, else the
// built-in default list.
func windowsExecExts() []string {
	if v := os.Getenv("PATHEXT"); v != "" {
		var exts []string
		for _, e := range strings.Split(v, ";") {
			if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
				exts = append(exts, e)
			}
		}
		if len(exts) > 0 {
			return exts
		}
	}
	return defaultWindowsExecExts
}

func hasExecExt(name string, exts []string) bool {
	lower := strings.ToLower(name)
	for _, ext := range exts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// candidateNames returns the filenames foreignInPath should stat for a
// given token name, in order. On Windows that's name+ext for every
// candidate extension, plus the bare name itself if it already ends in one
// of those extensions (so a caller asking about "tool.exe" doesn't only
// get "tool.exe.exe" checked). On every other goos it's just name, matched
// via the exec permission bit rather than an extension.
func candidateNames(name, goos string) []string {
	if goos != "windows" {
		return []string{name}
	}
	exts := windowsExecExts()
	names := make([]string, 0, len(exts)+1)
	for _, ext := range exts {
		names = append(names, name+ext)
	}
	if hasExecExt(name, exts) {
		names = append(names, name)
	}
	return names
}

// foreignInPath returns the path of an executable `name` reachable via PATH
// that clau does not own, or "" if none. linkDir itself is excluded.
//
// goos selects how "executable" is decided: on Windows, regular files have
// no meaningful permission bits, so a candidate counts if it exists under
// one of the recognized executable extensions; elsewhere, the file must
// exist (extensionless) and carry an exec permission bit.
func foreignInPath(name, linkDir, clauPath, goos string) string {
	names := candidateNames(name, goos)
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		if d == "" || filepath.Clean(d) == filepath.Clean(linkDir) {
			continue
		}
		for _, n := range names {
			p := filepath.Join(d, n)
			fi, err := os.Stat(p)
			if err != nil || fi.IsDir() {
				continue
			}
			if goos != "windows" && fi.Mode()&0o111 == 0 {
				continue
			}
			if isOwned(p, clauPath) {
				continue
			}
			return p
		}
	}
	return ""
}

type linkReport struct {
	Created, Kept, Skipped, Pruned []string
}

func linkFileName(name, goos string) string {
	if goos == "windows" {
		return name + ".cmd"
	}
	return name
}

func writeLink(goos, dir, name, clauPath string) error {
	path := filepath.Join(dir, linkFileName(name, goos))
	_ = os.Remove(path)
	if goos == "windows" {
		token := strings.TrimPrefix(name, "c")
		return os.WriteFile(path, []byte(shimContent(clauPath, token)), 0o755)
	}
	return os.Symlink(clauPath, path)
}

func syncLinks(cfg Config, dir, clauPath, goos string, force bool) (linkReport, error) {
	var rep linkReport
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return rep, err
	}
	desired := map[string]bool{}
	for _, name := range linkNames(cfg) {
		desired[name] = true
		path := filepath.Join(dir, linkFileName(name, goos))
		if foreign := foreignInPath(name, dir, clauPath, goos); foreign != "" && !force {
			rep.Skipped = append(rep.Skipped, fmt.Sprintf("%s (collides with %s)", name, foreign))
			continue
		}
		if fi, err := os.Lstat(path); err == nil {
			if isOwned(path, clauPath) {
				if target, err := os.Readlink(path); err == nil && target == clauPath && goos != "windows" {
					rep.Kept = append(rep.Kept, name)
					continue
				}
				if goos == "windows" {
					data, _ := os.ReadFile(path)
					if string(data) == shimContent(clauPath, strings.TrimPrefix(name, "c")) {
						rep.Kept = append(rep.Kept, name)
						continue
					}
				}
				// Owned but stale target: recreate below.
			} else if !force {
				rep.Skipped = append(rep.Skipped, fmt.Sprintf("%s (existing file at %s)", name, path))
				continue
			}
			_ = fi
		}
		if err := writeLink(goos, dir, name, clauPath); err != nil {
			return rep, fmt.Errorf("creating %s: %w", path, err)
		}
		rep.Created = append(rep.Created, name)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return rep, err
	}
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".cmd")
		path := filepath.Join(dir, e.Name())
		if desired[name] || name == "clau" || !isOwned(path, clauPath) {
			continue
		}
		if err := os.Remove(path); err != nil {
			return rep, err
		}
		rep.Pruned = append(rep.Pruned, name)
	}
	sort.Strings(rep.Pruned)
	return rep, nil
}

func removeOwned(dir, clauPath string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var removed []string
	for _, e := range entries {
		if strings.TrimSuffix(e.Name(), ".cmd") == "clau" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if !isOwned(path, clauPath) {
			continue
		}
		if err := os.Remove(path); err != nil {
			return removed, err
		}
		removed = append(removed, e.Name())
	}
	sort.Strings(removed)
	return removed, nil
}
