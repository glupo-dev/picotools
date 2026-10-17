package main

import (
	"fyne.io/fyne/v2"

	"gltools/internal/gui"
)

func runWindow() {
	a, w := gui.NewWindow("local.gltools", "GL Tools")
	setupWindow(a, w)
	gui.Run(a, w)
}

// setupWindow puts the console in the window and connects keys, mouse and clipboard to it.
func setupWindow(a fyne.App, w fyne.Window) *model {
	m := &model{
		cols: gui.DefaultCols,
		copy: func(text string) { a.Clipboard().SetContent(text) },
		quit: a.Quit,
	}
	var s *gui.Screen
	m.background = func(work func() ([]Result, error), done func([]Result, error)) {
		go func() {
			res, err := work()
			s.Do(func() { done(res, err) })
		}()
	}
	m.ui = func(f func()) { s.Do(f) }
	s = gui.NewScreen(a, w, m)
	return m
}

// The model as a gui.ConsoleApp.

func (m *model) Key(k string)      { m.key(k) }
func (m *model) Rune(r rune)       { m.rune(r) }
func (m *model) Paste(text string) { m.paste(text) }
func (m *model) CanPaste() bool    { return m.canPaste() }

func (m *model) Render(cols, rows int) []line {
	m.cols, m.rows = cols, rows
	return m.render()
}
