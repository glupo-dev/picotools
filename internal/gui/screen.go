package gui

import (
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// Keys that the window forwards to the app (printable characters arrive as runes).
const (
	KeyUp        = "up"
	KeyDown      = "down"
	KeyLeft      = "left"
	KeyRight     = "right"
	KeyEnter     = "enter"
	KeyEsc       = "esc"
	KeyBackspace = "backspace"
	KeyPageUp    = "pageup"
	KeyPageDown  = "pagedown"
	KeyHome      = "home"
	KeyEnd       = "end"
	KeyOther     = "other"
)

// FullScreenKey is the key shown in the hints to switch full screen on and off.
var FullScreenKey = "F11"

// ConsoleApp is an app made of text screens driven by the keyboard, such as GL Tools.
// It knows nothing about the window: the Screen gives it the keys and draws what it renders.
type ConsoleApp interface {
	Key(k string) // one of the Key... names
	Rune(r rune)
	Paste(text string)
	CanPaste() bool
	// Render gives the lines to show in a console of cols x rows characters.
	Render(cols, rows int) []Line
}

// Screen connects a ConsoleApp to a window: keys, mouse selection, clipboard and drawing.
type Screen struct {
	con *Console
	app ConsoleApp
}

// NewScreen puts a console in the window and sends its keys to app.
func NewScreen(a fyne.App, w fyne.Window, app ConsoleApp) *Screen {
	s := &Screen{app: app}
	paste := func() {
		app.Paste(a.Clipboard().Content())
		s.Redraw()
	}
	copySelection := func() {
		if text := s.con.SelectedText(); text != "" {
			a.Clipboard().SetContent(text)
		}
	}
	s.con = NewConsole(
		func() { s.Redraw() },
		func(pos fyne.Position) {
			copyItem := fyne.NewMenuItem("Copy", copySelection)
			copyItem.Disabled = s.con.SelectedText() == ""
			pasteItem := fyne.NewMenuItem("Paste", paste)
			pasteItem.Disabled = !app.CanPaste()
			widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", copyItem, pasteItem), w.Canvas(), pos)
		},
	)

	c := w.Canvas()
	c.SetOnTypedKey(func(e *fyne.KeyEvent) {
		s.con.ClearSelection()
		switch e.Name {
		case fyne.KeyUp:
			app.Key(KeyUp)
		case fyne.KeyDown:
			app.Key(KeyDown)
		case fyne.KeyLeft:
			app.Key(KeyLeft)
		case fyne.KeyRight:
			app.Key(KeyRight)
		case fyne.KeyReturn, fyne.KeyEnter:
			app.Key(KeyEnter)
		case fyne.KeyEscape:
			app.Key(KeyEsc)
		case fyne.KeyBackspace:
			app.Key(KeyBackspace)
		case fyne.KeyPageUp:
			app.Key(KeyPageUp)
		case fyne.KeyPageDown:
			app.Key(KeyPageDown)
		case fyne.KeyHome:
			app.Key(KeyHome)
		case fyne.KeyEnd:
			app.Key(KeyEnd)
		case fyne.KeyF11:
			w.SetFullScreen(!w.FullScreen())
			return
		case fyne.KeySpace:
			return // arrives as a rune
		default:
			if len(e.Name) == 1 {
				return // letters and digits arrive as runes
			}
			app.Key(KeyOther)
		}
		s.Redraw()
	})
	c.SetOnTypedRune(func(r rune) {
		s.con.ClearSelection()
		app.Rune(r)
		s.Redraw()
	})
	// Ctrl+C / Ctrl+V on Windows and Cmd+C / Cmd+V on Mac; on Mac also accept Ctrl+V.
	c.AddShortcut(&fyne.ShortcutCopy{}, func(fyne.Shortcut) { copySelection() })
	c.AddShortcut(&fyne.ShortcutPaste{}, func(fyne.Shortcut) { paste() })
	if runtime.GOOS == "darwin" {
		c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyV, Modifier: fyne.KeyModifierControl}, func(fyne.Shortcut) { paste() })
		// Ctrl+Cmd+F is the usual full screen shortcut on Mac.
		FullScreenKey = "ctrl+cmd+F"
		c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: fyne.KeyModifierControl | fyne.KeyModifierSuper}, func(fyne.Shortcut) {
			w.SetFullScreen(!w.FullScreen())
		})
	}

	w.SetContent(container.NewPadded(s.con))
	s.Redraw()
	return s
}

// Redraw renders the app again; it must be called in the user interface.
func (s *Screen) Redraw() {
	Draw(s.con, s.app.Render(s.con.Cols-s.con.Margin, s.con.Rows))
}

// Do runs f in the user interface and redraws; it can be called from any goroutine.
func (s *Screen) Do(f func()) {
	fyne.Do(func() {
		f()
		s.Redraw()
	})
}
