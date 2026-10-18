//go:build !windows

package main

import (
	"errors"
	"os/exec"
	"runtime"
	"syscall"
)

// pingCommand sends one ping, waiting up to 2 seconds.
func pingCommand(cmd *exec.Cmd, host string) {
	deadline := "-w" // Linux: total time in seconds
	if runtime.GOOS == "darwin" {
		deadline = "-t"
	}
	cmd.Path, _ = exec.LookPath("ping")
	cmd.Args = []string{"ping", "-c", "1", deadline, "2", host}
}

// connRefused reports a closed port.
func connRefused(err error) bool { return errors.Is(err, syscall.ECONNREFUSED) }
