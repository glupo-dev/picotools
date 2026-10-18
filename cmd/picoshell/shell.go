package main

import (
	"fmt"
	"image/color"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"github.com/fyne-io/terminal"

	"picotools/internal/gui"
)

// shellBackground is a neutral graphite, so the shell stands apart from the grey-green Pico Tools.
var shellBackground = color.NRGBA{0x1E, 0x1E, 0x1E, 0xFF}

const shellHint = "Shell · type exit to close · scroll up to see earlier output"

// shellView is the system shell (zsh on Mac, PowerShell on Windows) shown inside the window.
// It extends the terminal widget to add the right-click menu and scrolling back:
// when the user scrolls up, a read-only view of the history is shown on top of the shell.
type shellView struct {
	*terminal.Terminal
	app fyne.App
	win fyne.Window

	hint      *canvas.Text
	history   *gui.Console      // read-only view of earlier output
	histLayer fyne.CanvasObject // history with its background, hidden when not scrolled
	offset    int               // lines scrolled back, 0 = live shell
	scrollAcc float32           // scrolling not yet turned into whole lines
}

func newShellView(a fyne.App, w fyne.Window) *shellView {
	s := &shellView{Terminal: terminal.New(), app: a, win: w}
	s.ExtendBaseWidget(s)

	s.hint = canvas.NewText(shellHint, gui.Muted)
	s.hint.TextSize = gui.TextSize
	s.hint.TextStyle.Monospace = true

	s.history = gui.NewConsole(func() {
		if s.offset > 0 {
			s.setOffset(s.offset)
		}
	}, func(pos fyne.Position) { s.showMenu(pos, s.history.SelectedText()) })
	s.history.Margin = 0
	s.history.OnScroll = s.Scrolled
	s.histLayer = container.NewStack(canvas.NewRectangle(shellBackground), s.history)
	s.histLayer.Hide()
	return s
}

// MouseDown skips the library's right-click behavior (paste the selection right away
// and clear it), so that the right click opens our Copy / Paste menu instead.
func (s *shellView) MouseDown(e *desktop.MouseEvent) {
	if e.Button == desktop.MouseButtonSecondary {
		return
	}
	s.Terminal.MouseDown(e)
}

func (s *shellView) TappedSecondary(e *fyne.PointEvent) {
	s.showMenu(e.AbsolutePosition, s.SelectedText())
}

func (s *shellView) showMenu(pos fyne.Position, selected string) {
	copyItem := fyne.NewMenuItem("Copy", func() { s.app.Clipboard().SetContent(selected) })
	copyItem.Disabled = selected == ""
	pasteItem := fyne.NewMenuItem("Paste", func() {
		s.setOffset(0)
		for _, r := range s.app.Clipboard().Content() {
			s.Terminal.TypedRune(r)
		}
	})
	widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", copyItem, pasteItem), s.win.Canvas(), pos)
}

// ---- Scrolling back ----

func (s *shellView) Scrolled(e *fyne.ScrollEvent) {
	if s.AltScreenActive() {
		return // full screen programs such as vi or top handle the screen themselves
	}
	s.scrollAcc += e.Scrolled.DY / gui.CellSize().Height
	lines := int(s.scrollAcc)
	if lines == 0 {
		return
	}
	s.scrollAcc -= float32(lines)
	s.setOffset(s.offset + lines)
}

// setOffset shows the output as it was the given number of lines ago (0 = live shell).
func (s *shellView) setOffset(offset int) {
	rows := s.ScrollbackRows()
	visible := max(1, int(s.Size().Height/gui.CellSize().Height))
	offset = min(max(offset, 0), max(len(rows)-visible, 0))
	if offset == 0 && s.offset == 0 {
		return
	}
	s.offset = offset

	if offset == 0 {
		s.scrollAcc = 0
		s.history.ClearSelection()
		s.histLayer.Hide()
		s.hint.Text = shellHint
		s.hint.Refresh()
		return
	}
	end := len(rows) - offset
	s.history.Resize(s.Size())
	s.history.DrawRows(rows[max(end-visible, 0):end])
	s.histLayer.Show()
	s.hint.Text = fmt.Sprintf("Shell · %d lines back · scroll down or press a key to return", offset)
	s.hint.Refresh()
}

// ---- Keyboard: typing always goes back to the live shell ----

func (s *shellView) TypedRune(r rune) {
	s.setOffset(0)
	s.Terminal.TypedRune(r)
}

func (s *shellView) TypedKey(e *fyne.KeyEvent) {
	s.setOffset(0)
	s.Terminal.TypedKey(e)
}

func (s *shellView) TypedShortcut(sc fyne.Shortcut) {
	if _, ok := sc.(*fyne.ShortcutCopy); ok && s.offset > 0 {
		if text := s.history.SelectedText(); text != "" {
			s.app.Clipboard().SetContent(text)
		}
		return
	}
	s.Terminal.TypedShortcut(sc)
}

// startShell replaces the window content with a shell and returns it. When the shell
// ends (e.g. with "exit"), onExit is called.
func startShell(a fyne.App, w fyne.Window, onExit func(error)) *shellView {
	prepareShellEnv()

	s := newShellView(a, w)
	if home, err := os.UserHomeDir(); err == nil {
		s.SetStartDir(home)
	}

	body := container.NewPadded(container.NewStack(s, s.histLayer))
	content := container.NewBorder(container.NewPadded(s.hint), nil, nil, nil, body)
	w.SetContent(container.NewStack(canvas.NewRectangle(shellBackground), content))
	w.Canvas().Focus(s)

	go func() {
		err := s.RunLocalShell()
		fyne.Do(func() { onExit(err) })
	}()
	return s
}
