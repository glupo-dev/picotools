// Pico Password: my passwords, kept in a file encrypted with a master password that the system
// keychain remembers. The design is in docs/picopassword.md.
package main

import (
	"time"

	"picotools/internal/gui"
)

func main() {
	a, w := gui.NewWindow("local.picopassword", "Pico Password")
	m := newModel("")
	path, err := vaultPath()
	if err != nil {
		m.fail("Could not find the folder for the vault: " + err.Error())
	} else {
		m.path = path
		m.start()
	}

	m.copy = func(text string) { a.Clipboard().SetContent(text) }
	m.quit = a.Quit
	screen := gui.NewScreen(a, w, m)

	// A revealed password hides again by itself.
	go func() {
		for range time.Tick(time.Second) {
			screen.Do(m.tick)
		}
	}()
	gui.Run(a, w)
}
