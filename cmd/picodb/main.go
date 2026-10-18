// Pico DB: look at the tables of a SQL Server database, read only. The connection strings
// are kept in the system keychain.
package main

import (
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"picotools/internal/gui"
)

func main() {
	a, w := gui.NewWindow("local.picodb", "Pico DB")
	path, err := storePath()
	var s *Store
	if err == nil {
		s, err = loadStore(path)
	}
	if err != nil {
		msg := canvas.NewText("✘ Could not read the saved connections: "+err.Error(), gui.Red)
		msg.TextSize = gui.TextSize
		msg.TextStyle.Monospace = true
		w.SetContent(container.NewPadded(container.NewVBox(msg)))
		gui.Run(a, w)
		return
	}

	m := newModel(s)
	m.copy = func(text string) { a.Clipboard().SetContent(text) }
	m.quit = a.Quit
	var screen *gui.Screen
	m.ui = func(f func()) { screen.Do(f) }
	screen = gui.NewScreen(a, w, m)
	gui.Run(a, w)
}
