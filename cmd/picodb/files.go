package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// fileName is base-YYYYMMDD-HHMM.ext, with spaces turned into "-" and the characters
// not allowed in file names left out. If such a file already exists in dir, -2, -3…
// is added.
func fileName(dir, base, ext string, now time.Time) string {
	base = unsafeChars.ReplaceAllString(strings.ReplaceAll(strings.TrimSpace(base), " ", "-"), "")
	base = strings.Trim(base, "-.") + "-" + now.Format("20060102-1504")
	name := base + ext
	for i := 2; exists(filepath.Join(dir, name)); i++ {
		name = fmt.Sprintf("%s-%d%s", base, i, ext)
	}
	return name
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// resolvePath turns what the user typed into a full path: ~ is the user folder.
func resolvePath(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, _ := os.UserHomeDir()
		p = filepath.Join(home, p[1:])
	}
	return filepath.Clean(p)
}

// displayPath shows a path with ~ in place of the user folder.
func displayPath(p string) string {
	home, _ := os.UserHomeDir()
	if home != "" && (p == home || strings.HasPrefix(p, home+string(filepath.Separator))) {
		return "~" + filepath.ToSlash(p[len(home):])
	}
	return p
}

// createFile makes a new file: an existing file is never overwritten.
func createFile(path string) (*os.File, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("write the full path, e.g. ~/Downloads/name.sql")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return nil, errors.New("a file with this name already exists: " + displayPath(path))
	}
	return f, err
}

// saveFile writes data to a new file.
func saveFile(path string, data []byte) error {
	f, err := createFile(path)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
