package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/microsoft/go-mssqldb/msdsn"
	_ "github.com/microsoft/go-mssqldb/namedpipe" // the "np" protocol
)

// useLocalDB starts the LocalDB instance, if stopped, and points cfg to its named pipe.
func useLocalDB(cfg *msdsn.Config) error {
	if !isLocalDB(*cfg) {
		return nil
	}
	instance := cfg.Instance
	if instance == "" {
		instance = "MSSQLLocalDB"
	}
	exe := sqlLocalDB()
	if exe == "" {
		return fmt.Errorf("SqlLocalDB.exe not found: is SQL Server LocalDB installed?")
	}
	if _, err := runLocalDB(exe, "start", instance); err != nil {
		return err
	}
	info, err := runLocalDB(exe, "info", instance)
	if err != nil {
		return err
	}
	pipe := localDBPipe(info)
	if pipe == "" {
		return fmt.Errorf("LocalDB instance '%s' has no named pipe", instance)
	}
	np, err := msdsn.Parse("server=" + pipe)
	if err != nil {
		return err
	}
	cfg.Host, cfg.Instance = "localhost", ""
	cfg.Protocols, cfg.ProtocolParameters = np.Protocols, np.ProtocolParameters
	return nil
}

// sqlLocalDB is SqlLocalDB.exe from the PATH or else from the newest SQL Server folder.
func sqlLocalDB() string {
	if p, err := exec.LookPath("SqlLocalDB"); err == nil {
		return p
	}
	found, _ := filepath.Glob(filepath.Join(os.Getenv("ProgramFiles"), `Microsoft SQL Server\*\Tools\Binn\SqlLocalDB.exe`))
	sort.Strings(found)
	if len(found) == 0 {
		return ""
	}
	return found[len(found)-1]
}

// runLocalDB runs SqlLocalDB without opening a console window.
func runLocalDB(exe string, args ...string) (string, error) {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("SqlLocalDB %s: %s", strings.Join(args, " "), msg)
	}
	return string(out), nil
}
