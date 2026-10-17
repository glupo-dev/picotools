package main

import (
	"image/color"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"gltools/internal/gui"
)

// The editor page is light, like the usual text editors: paper grey with black text.
// Around it the window keeps the console background.
var (
	editorBackground = color.NRGBA{0xF2, 0xF2, 0xF2, 0xFF}
	editorTextColor  = color.NRGBA{0x00, 0x00, 0x00, 0xFF}
)

// editorTheme is the light theme used only inside the editor; sizes are the same as the console.
type editorTheme struct{ gui.ConsoleTheme }

func (t editorTheme) Color(n fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameBackground, theme.ColorNameInputBackground:
		return editorBackground
	case theme.ColorNameForeground:
		return editorTextColor
	}
	return theme.DefaultTheme().Color(n, theme.VariantLight)
}

// editorView is a simple text editor: type, select, cut, copy, paste, undo.
// It is the standard multi-line entry. Esc moves to the command line at the bottom,
// like : in vi; there esc goes back to the text and enter runs the command.
type editorView struct {
	widget.Entry
	ctrlD bool // Ctrl+D was pressed and Ctrl is still held: waiting for the second key

	commanding bool   // the keys go to the command line
	command    []rune // the command typed so far
	message    string // shown at the bottom: the outcome of the last command
	failed     bool   // the message is an error
	choices    string // the names Tab found, shown above the command line
	file       string // full path of the file being edited, "" for a new one
	modified   bool   // the text changed since it was opened or saved
	loading    bool   // the text is being replaced by a file: not a change by the user
	crlf       bool   // the file ends lines with \r\n (Windows): saved back the same way

	hint    *canvas.Text
	info    *canvas.Text // above the command line: messages and Tab choices
	cmdLine *canvas.Text
	onQuit  func()
}

func (e *editorView) TypedKey(k *fyne.KeyEvent) {
	e.ctrlD = false
	if e.commanding {
		e.commandKey(k)
		return
	}
	if k.Name == fyne.KeyEscape {
		e.setCommanding(true)
		return
	}
	e.Entry.TypedKey(k)
}

func (e *editorView) TypedRune(r rune) {
	e.ctrlD = false
	if e.commanding {
		e.commandRune(r)
		return
	}
	e.Entry.TypedRune(r)
}

// KeyUp forgets a pending Ctrl+D when Ctrl is released.
func (e *editorView) KeyUp(k *fyne.KeyEvent) {
	if k.Name == desktop.KeyControlLeft || k.Name == desktop.KeyControlRight {
		e.ctrlD = false
	}
	e.Entry.KeyUp(k)
}

// Holding Ctrl, D followed by a second key runs a line command:
//
//	D  delete the line          C  copy the line
//	O  new line above           L  new line below
//	I  duplicate the line above K  duplicate the line below
//
// O/L and I/K sit one above the other on the keyboard: the upper key works above.
func (e *editorView) TypedShortcut(s fyne.Shortcut) {
	if e.commanding {
		return
	}
	pending := e.ctrlD
	e.ctrlD = false
	key := fyne.KeyName("")
	switch c := s.(type) {
	case *desktop.CustomShortcut:
		if c.Modifier == fyne.KeyModifierControl {
			key = c.KeyName
		}
	case *fyne.ShortcutCopy:
		// On Windows Ctrl+C arrives as copy: after Ctrl+D it is our C.
		if runtime.GOOS != "darwin" && !c.Secondary {
			key = fyne.KeyC
		}
	}
	if key == fyne.KeyD && !pending {
		e.ctrlD = true
		return
	}
	if pending {
		switch key {
		case fyne.KeyD:
			e.deleteLine()
		case fyne.KeyC:
			e.copyLine()
		case fyne.KeyO:
			e.openLine(true)
		case fyne.KeyL:
			e.openLine(false)
		case fyne.KeyI:
			e.duplicateLine(true)
		case fyne.KeyK:
			e.duplicateLine(false)
		}
		return // any other key after Ctrl+D does nothing
	}
	e.Entry.TypedShortcut(s)
}

// line gives the text, the cursor position and where the line of the cursor starts and ends
// (the end is before the line break).
func (e *editorView) line() (text []rune, pos, start, end int) {
	e.dropSelection()
	text = []rune(e.Text)
	pos = e.CursorTextOffset()
	start, end = pos, pos
	for start > 0 && text[start-1] != '\n' {
		start--
	}
	for end < len(text) && text[end] != '\n' {
		end++
	}
	return text, pos, start, end
}

// openLine adds an empty line above (or below) the line of the cursor and moves the cursor there.
// It moves with the arrow keys and adds the line with Enter, so that undo works as usual.
func (e *editorView) openLine(above bool) {
	_, pos, start, end := e.line()
	if above {
		e.press(fyne.KeyLeft, pos-start)
		e.press(fyne.KeyReturn, 1)
		e.press(fyne.KeyLeft, 1) // back to the new empty line
		return
	}
	e.press(fyne.KeyRight, end-pos)
	e.press(fyne.KeyReturn, 1)
}

// copyLine copies the line of the cursor, without the line break.
func (e *editorView) copyLine() {
	text, _, start, end := e.line()
	fyne.CurrentApp().Clipboard().SetContent(string(text[start:end]))
}

// duplicateLine adds a copy of the line of the cursor above (or below) it and moves
// the cursor to the copy, at the same column.
func (e *editorView) duplicateLine(above bool) {
	text, pos, start, end := e.line()
	line := text[start:end]
	col := pos - start
	if above {
		e.press(fyne.KeyLeft, col)
		e.typeText(string(line) + "\n")
		e.press(fyne.KeyLeft, 1+len(line)-col)
		return
	}
	e.press(fyne.KeyRight, end-pos)
	e.typeText("\n" + string(line))
	e.press(fyne.KeyLeft, len(line)-col)
}

// typeText inserts the text at the cursor in one go, so that one undo takes it away.
// It pastes from a private clipboard: the real one is left as it is.
func (e *editorView) typeText(text string) {
	e.Entry.TypedShortcut(&fyne.ShortcutPaste{Clipboard: &textClipboard{text}})
}

// textClipboard is a clipboard holding a fixed text, used by typeText.
type textClipboard struct{ text string }

func (c *textClipboard) Content() string   { return c.text }
func (c *textClipboard) SetContent(string) {}

// deleteLine deletes the line of the cursor, with its line break.
func (e *editorView) deleteLine() {
	text, pos, start, end := e.line()
	switch {
	case end < len(text):
		end++ // the break after the line
	case start > 0:
		start-- // the last line has no break after it: take the one before it
	}
	e.deleteRange(pos, start, end)
	e.press(fyne.KeyHome, 1)
}

// deleteRange deletes the text from start to end; the cursor is at pos.
// It selects the text and deletes it in one go, so that one undo brings it back.
func (e *editorView) deleteRange(pos, start, end int) {
	if start == end {
		return
	}
	e.press(fyne.KeyLeft, pos-start)
	e.Entry.KeyDown(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	e.press(fyne.KeyRight, end-start)
	e.Entry.KeyUp(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	e.press(fyne.KeyDelete, 1)
}

func (e *editorView) dropSelection() {
	if e.SelectedText() != "" {
		e.Entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	}
}

func (e *editorView) press(key fyne.KeyName, times int) {
	for range times {
		e.Entry.TypedKey(&fyne.KeyEvent{Name: key})
	}
}

// startEditor replaces the window content with the editor and returns it.
// The command q calls onExit.
func startEditor(w fyne.Window, onExit func()) *editorView {
	e := &editorView{onQuit: onExit}
	e.MultiLine = true
	e.Wrapping = fyne.TextWrapWord
	e.TextStyle.Monospace = true
	e.ExtendBaseWidget(e)
	e.OnChanged = func(string) {
		if !e.loading && !e.modified {
			e.modified = true
			e.render()
		}
	}

	e.hint = consoleText("")
	e.info = consoleText("")
	e.cmdLine = consoleText("")
	e.render()

	top := container.NewPadded(e.hint)
	// The command line keeps its height when empty: a blank text of the same size is under it.
	bottom := container.NewPadded(container.NewVBox(e.info, container.NewStack(consoleText(" "), e.cmdLine)))
	content := container.NewBorder(top, bottom, nil, nil, container.NewPadded(e))
	page := container.NewStack(canvas.NewRectangle(gui.Background), content)
	w.SetContent(container.NewThemeOverride(page, editorTheme{gui.ConsoleTheme{Theme: theme.DefaultTheme()}}))
	w.Canvas().Focus(e)
	return e
}

// consoleText is a line of text in the console style, used around the editor.
func consoleText(s string) *canvas.Text {
	t := canvas.NewText(s, gui.Muted)
	t.TextSize = gui.TextSize
	t.TextStyle.Monospace = true
	return t
}
