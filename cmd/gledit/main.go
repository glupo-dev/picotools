// GL Edit: a simple text editor in its own window.
package main

import (
	"fyne.io/fyne/v2"

	"gltools/internal/gui"
)

func main() {
	a, w := gui.NewWindow("local.gledit", "GL Edit")
	e := startEditor(w, a.Quit)

	// Keys that reach the window (e.g. after a click outside the editor took the focus
	// away) are put back in the editor.
	c := w.Canvas()
	c.SetOnTypedKey(func(k *fyne.KeyEvent) {
		c.Focus(e)
		e.TypedKey(k)
	})
	c.SetOnTypedRune(func(r rune) {
		c.Focus(e)
		e.TypedRune(r)
	})
	toEditor := func(sc fyne.Shortcut) {
		c.Focus(e)
		e.TypedShortcut(sc)
	}
	c.AddShortcut(&fyne.ShortcutCopy{}, toEditor)
	c.AddShortcut(&fyne.ShortcutPaste{}, toEditor)
	gui.Run(a, w)
}
