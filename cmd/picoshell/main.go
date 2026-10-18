// Pico Shell: the system shell (zsh on Mac, PowerShell on Windows) in its own window.
package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"picotools/internal/gui"
)

func main() {
	a, w := gui.NewWindow("local.picoshell", "Pico Shell")

	// The shell starts once the window is on screen, so it knows its size.
	var s *shellView
	start := func() {
		s = startShell(a, w, func(err error) {
			if err == nil {
				a.Quit()
				return
			}
			s = nil
			msg := canvas.NewText("✘ Could not start the shell: "+err.Error(), gui.Red)
			msg.TextSize = gui.TextSize
			msg.TextStyle.Monospace = true
			w.SetContent(container.NewPadded(container.NewVBox(msg)))
		})
	}

	// Keys that reach the window (e.g. after a click outside the shell took the focus
	// away) are sent back to the shell.
	c := w.Canvas()
	c.SetOnTypedKey(func(e *fyne.KeyEvent) {
		if s != nil {
			c.Focus(s)
			s.TypedKey(e)
		}
	})
	c.SetOnTypedRune(func(r rune) {
		if s != nil {
			c.Focus(s)
			s.TypedRune(r)
		}
	})
	toShell := func(sc fyne.Shortcut) {
		if s != nil {
			c.Focus(s)
			s.TypedShortcut(sc)
		}
	}
	c.AddShortcut(&fyne.ShortcutCopy{}, toShell)
	c.AddShortcut(&fyne.ShortcutPaste{}, toShell)

	w.SetContent(canvas.NewRectangle(shellBackground))
	gui.Run(a, w, start)
}
