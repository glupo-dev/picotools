package main

import (
	"errors"
	"os/exec"
	"syscall"
)

// pingCommand sends one ping, waiting up to 2 seconds, without opening a console window.
func pingCommand(cmd *exec.Cmd, host string) {
	cmd.Path, _ = exec.LookPath("ping")
	cmd.Args = []string{"ping", "-n", "1", "-w", "2000", host}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

// connRefused reports a closed port. Windows uses its own code (WSAECONNREFUSED).
func connRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.Errno(10061))
}
