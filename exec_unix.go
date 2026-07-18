//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

func execClaude(l Launch) error {
	path, err := exec.LookPath(l.Target)
	if err != nil {
		return err
	}
	return syscall.Exec(path, append([]string{l.Target}, l.Args...), l.Env)
}
