package main

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var shellEnvOnce sync.Once

// prepareShellEnv makes the shell behave as in the Terminal app. Apps opened
// from the Finder get a minimal PATH, so the user's login PATH is loaded once.
func prepareShellEnv() {
	shellEnvOnce.Do(func() {
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/zsh"
			os.Setenv("SHELL", shell)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, shell, "-l", "-c", `printf %s "$PATH"`).Output()
		if path := strings.TrimSpace(string(out)); err == nil && path != "" {
			os.Setenv("PATH", path)
		}
	})
}
