package terminal

import (
	"io"
	"log"
	"os"
	"os/exec"
	"syscall"

	"github.com/ActiveState/termtest/conpty"
)

func (t *Terminal) updatePTYSize() {
	if t.pty == nil { // during load
		return
	}
	_ = t.pty.(*conpty.ConPty).Resize(uint16(t.config.Columns), uint16(t.config.Rows))
}

// hangup does nothing on Windows, closing the console will end its processes.
func (t *Terminal) hangup() {
}

func (t *Terminal) startPTY() (io.WriteCloser, io.Reader, io.Closer, error) {
	// GL Tools: create the console with the real size of the window instead of 80x25.
	// The resize done after start did not reach PowerShell ($Host.UI.RawUI.WindowSize
	// stayed 80x25), so long output and the next command were written mid-screen.
	cols, rows := int16(80), int16(25)
	if t.config.Columns > 0 && t.config.Rows > 0 {
		cols, rows = int16(t.config.Columns), int16(t.config.Rows)
	}
	cpty, err := conpty.New(cols, rows)
	if err != nil {
		return nil, nil, nil, err
	}

	pid, _, err := cpty.Spawn(
		"C:\\WINDOWS\\System32\\WindowsPowerShell\\v1.0\\powershell.exe",
		[]string{},
		&syscall.ProcAttr{
			Env: os.Environ(),
			Dir: t.startingDir(),
		},
	)
	if err != nil {
		return nil, nil, nil, err
	}

	t.cmd = &exec.Cmd{}
	t.cmd.Dir = t.startingDir()
	process, err := os.FindProcess(pid)
	if err != nil {
		return nil, nil, nil, err
	}
	go func() {
		ps, err := process.Wait()
		if err != nil {
			log.Fatalf("Error waiting for process: %v", err)
		}
		t.cmd.ProcessState = ps
		if t.pty != nil {
			t.pty = nil
			_ = cpty.Close()
		}
	}()

	return cpty.InPipe(), cpty.OutPipe(), cpty, nil
}
