// Pico Auth: the two-factor codes (TOTP) of my accounts, kept in a file encrypted
// with a master password that the system keychain remembers.
package main

import (
	"time"

	"picotools/internal/gui"
)

func main() {
	a, w := gui.NewWindow("local.picoauth", "Pico Auth")
	m := newModel("")
	path, err := vaultPath()
	if err != nil {
		m.fail("Could not find the folder for the vault: " + err.Error())
	} else {
		m.path = path
		m.start()
	}

	m.copy = func(text string) { a.Clipboard().SetContent(text) }
	m.clipboard = func() string { return a.Clipboard().Content() }
	m.quit = a.Quit
	// A copied code does not stay in the clipboard after Pico Auth is closed.
	w.SetOnClosed(func() {
		if m.copied != "" && m.clipboard() == m.copied {
			m.copy("")
		}
	})
	screen := gui.NewScreen(a, w, m)

	// The codes and the seconds left change by themselves.
	go func() {
		for range time.Tick(time.Second) {
			screen.Do(m.tick)
		}
	}()
	gui.Run(a, w)
}
