package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"fyne.io/fyne/v2"

	"gltools/internal/gui"
)

// The line above the text: what the keys do in the text and in the command line.
const (
	editorKeys  = "esc commands · ctrl+D + :  D delete · C copy · O/L new line above/below · I/K duplicate above/below"
	commandHint = "Commands · esc back to the text · tab completes :  w [name] save · e name open · q quit · q! quit without saving · wq save and quit · cd · mkdir · pwd"
)

// editorDir is the current folder of the command line: names without a full path are
// taken from here. It starts at the user folder and stays while the program runs.
var editorDir string

func currentDir() string {
	if editorDir == "" {
		editorDir, _ = os.UserHomeDir()
	}
	return editorDir
}

// ---- Keys in the command line ----

// setCommanding moves the keys to the command line (on) or back to the text (off).
func (e *editorView) setCommanding(on bool) {
	e.commanding = on
	e.command = nil
	e.choices = ""
	e.message, e.failed = "", false
	e.render()
}

func (e *editorView) commandKey(k *fyne.KeyEvent) {
	e.choices = ""
	switch k.Name {
	case fyne.KeyEscape:
		e.setCommanding(false)
		return
	case fyne.KeyBackspace:
		if len(e.command) > 0 {
			e.command = e.command[:len(e.command)-1]
		}
	case fyne.KeyTab:
		e.complete()
	case fyne.KeyReturn, fyne.KeyEnter:
		e.runCommand(strings.TrimSpace(string(e.command)))
		return
	}
	e.render()
}

func (e *editorView) commandRune(r rune) {
	e.choices = ""
	e.command = append(e.command, r)
	e.render()
}

// ---- Commands ----

// runCommand runs a command. Commands that work on the text (w, e) go back to it;
// those that work on folders (cd, mkdir, pwd) stay in the command line, ready for the next.
func (e *editorView) runCommand(line string) {
	name, arg, _ := strings.Cut(line, " ")
	arg = strings.TrimSpace(arg)
	e.command = nil
	switch name {
	case "":
		e.setCommanding(false)
	case "pwd":
		e.say(currentDir(), false)
	case "cd":
		e.changeDir(arg)
	case "mkdir":
		e.makeDir(arg)
	case "w":
		if e.save(arg) {
			e.back()
		}
	case "wq":
		if e.save(arg) {
			e.onQuit()
		}
	case "e":
		if e.open(arg) {
			e.back()
		}
	case "q":
		if e.modified {
			e.say("Unsaved changes: w to save them, q! to quit without saving", true)
			return
		}
		e.onQuit()
	case "q!":
		e.onQuit()
	default:
		e.say("Unknown command: "+name, true)
	}
}

// say shows a message at the bottom; an error in red.
func (e *editorView) say(msg string, failed bool) {
	e.message, e.failed = msg, failed
	e.render()
}

// back returns to the text keeping the message of the command just run.
func (e *editorView) back() {
	e.commanding = false
	e.render()
}

func (e *editorView) changeDir(arg string) {
	if arg == "" {
		arg = "~"
	}
	path := resolvePath(arg)
	info, err := os.Stat(path)
	switch {
	case err != nil:
		e.say("No such folder: "+arg, true)
	case !info.IsDir():
		e.say("Not a folder: "+arg, true)
	default:
		editorDir = path
		e.say("", false)
	}
}

func (e *editorView) makeDir(arg string) {
	if arg == "" {
		e.say("Folder name missing: mkdir <name>", true)
		return
	}
	if err := os.MkdirAll(resolvePath(arg), 0o755); err != nil {
		e.say("Could not create the folder: "+err.Error(), true)
		return
	}
	e.say("Created "+displayPath(resolvePath(arg)), false)
}

// save writes the text to the named file, or to the current one when no name is given.
// It does not overwrite another file that already exists.
func (e *editorView) save(arg string) bool {
	path := e.file
	if arg != "" {
		path = resolvePath(arg)
		if path != e.file {
			if _, err := os.Stat(path); err == nil {
				e.say("A file with this name already exists: "+displayPath(path), true)
				return false
			}
		}
	}
	if path == "" {
		e.say("No file name: w <name>", true)
		return false
	}
	text := e.Text
	if e.crlf {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		e.say("Could not save: "+err.Error(), true)
		return false
	}
	e.file = path
	e.modified = false
	e.say("Saved "+displayPath(path), false)
	return true
}

// open loads the named file in place of the text. A name that does not exist yet
// starts a new empty file with that name, saved by w.
func (e *editorView) open(arg string) bool {
	if arg == "" {
		e.say("File name missing: e <name>", true)
		return false
	}
	if e.modified {
		e.say("Unsaved changes: save them with w first, or quit with q!", true)
		return false
	}
	path := resolvePath(arg)
	data, err := os.ReadFile(path)
	msg := "Opened " + displayPath(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		data, msg = nil, "New file "+displayPath(path)
	case err != nil:
		if info, statErr := os.Stat(path); statErr == nil && info.IsDir() {
			e.say("This is a folder: "+arg, true)
		} else {
			e.say("Could not open: "+err.Error(), true)
		}
		return false
	}
	text := string(data)
	// Files written on Windows end lines with \r\n: shown as \n, saved back as they were.
	e.crlf = strings.Contains(text, "\r\n")
	text = strings.ReplaceAll(text, "\r\n", "\n")

	e.loading = true
	e.SetText(text)
	e.loading = false
	e.CursorRow, e.CursorColumn = 0, 0
	e.Refresh()
	e.file = path
	e.modified = false
	e.say(msg, false)
	return true
}

// ---- Tab completion ----

// complete finishes the folder or file name being typed after cd, mkdir, w, wq or e.
// With one match it is written in full; with more, the part they share is written
// and their names are shown above the command line.
func (e *editorView) complete() {
	line := string(e.command)
	name, arg, found := strings.Cut(line, " ")
	if !found {
		return
	}
	onlyDirs := name == "cd" || name == "mkdir"
	if !onlyDirs && name != "w" && name != "wq" && name != "e" {
		return
	}
	arg = strings.TrimLeft(arg, " ")

	// The folder part of what is typed, and the start of the name inside it.
	cut := strings.LastIndexAny(arg, `/\`) + 1
	typedDir, start := arg[:cut], arg[cut:]
	dir := currentDir()
	if typedDir != "" {
		dir = resolvePath(typedDir)
	} else if start == "~" {
		e.command = []rune(name + " ~/")
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var matches []string
	for _, en := range entries {
		n := en.Name()
		if strings.HasPrefix(n, ".") && !strings.HasPrefix(start, ".") {
			continue // hidden files only when asked for
		}
		if !strings.HasPrefix(strings.ToLower(n), strings.ToLower(start)) {
			continue
		}
		isDir := en.IsDir()
		if !isDir && en.Type()&fs.ModeSymlink != 0 {
			if info, err := os.Stat(filepath.Join(dir, n)); err == nil {
				isDir = info.IsDir()
			}
		}
		if isDir {
			n += "/"
		} else if onlyDirs {
			continue
		}
		matches = append(matches, n)
	}
	if len(matches) == 0 {
		return
	}
	sort.Slice(matches, func(i, j int) bool { return strings.ToLower(matches[i]) < strings.ToLower(matches[j]) })
	e.command = []rune(name + " " + typedDir + commonPrefix(matches))
	if len(matches) > 1 {
		e.choices = e.fitChoices(matches)
	}
}

// commonPrefix is the start shared by all the names, ignoring upper and lower case.
func commonPrefix(names []string) string {
	p := []rune(names[0])
	for _, n := range names[1:] {
		r := []rune(n)
		i := 0
		for i < len(p) && i < len(r) && strings.EqualFold(string(p[i]), string(r[i])) {
			i++
		}
		p = p[:i]
	}
	return string(p)
}

// fitChoices puts the names on one line, as many as fit, then "…".
func (e *editorView) fitChoices(names []string) string {
	width := int(e.Size().Width/gui.CellSize().Width) - 2
	line := ""
	for i, n := range names {
		next := n
		if i > 0 {
			next = "  " + n
		}
		if len([]rune(line+next)) > width-3 {
			return line + "  …"
		}
		line += next
	}
	return line
}

// ---- Paths ----

// resolvePath turns what the user typed into a full path: ~ is the user folder,
// a name without a full path is inside the current folder.
func resolvePath(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, _ := os.UserHomeDir()
		p = filepath.Join(home, p[1:])
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(currentDir(), p)
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

// ---- Screen ----

// render updates the line above the text and the lines below it.
func (e *editorView) render() {
	msgColor := gui.Muted
	if e.failed {
		msgColor = gui.Red
	}
	if e.commanding {
		e.hint.Text = commandHint
		e.info.Text, e.info.Color = e.message, msgColor
		if e.choices != "" {
			e.info.Text, e.info.Color = e.choices, gui.Muted
		}
		e.cmdLine.Text, e.cmdLine.Color = displayPath(currentDir())+" :"+string(e.command)+"█", gui.Muted
	} else {
		name := "new file"
		if e.file != "" {
			name = filepath.Base(e.file)
		}
		if e.modified {
			name += " *"
		}
		e.hint.Text = "Editor · " + name + " · " + editorKeys
		e.info.Text = ""
		e.cmdLine.Text, e.cmdLine.Color = e.message, msgColor
	}
	if e.info.Text == "" {
		e.info.Hide()
	} else {
		e.info.Show()
	}
	e.hint.Refresh()
	e.info.Refresh()
	e.cmdLine.Refresh()
}
