//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
)

func execClaude(l Launch) error {
	cmd := exec.Command(l.Target, l.Args...)
	cmd.Env = l.Env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		os.Exit(ee.ExitCode())
	}
	if err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
