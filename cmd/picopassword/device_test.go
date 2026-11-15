package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"picotools/internal/gui"
)

// The helpers of the sync tests, as in Pico Auth (cmd/picoauth/model_test.go): a device with
// its own folder, a clipboard and a clock.

type testModel struct {
	*model
	clip string
	at   time.Time
}

// newDevice starts Pico Password on a device whose vault is in dir.
func newDevice(t *testing.T, dir string) *testModel {
	m := newModel(filepath.Join(dir, "picopassword", "vault.picopassword"))
	m.rows, m.cols = 40, 160
	tm := &testModel{model: m, at: time.Unix(1700000000, 0)}
	m.copy = func(s string) { tm.clip = s }
	m.now = func() time.Time { return tm.at }
	m.start()
	return tm
}

// typ types text and enter.
func (tm *testModel) typ(text string) {
	for _, r := range text {
		tm.Rune(r)
	}
	tm.Key(gui.KeyEnter)
}

func (tm *testModel) text() string {
	var b strings.Builder
	for _, l := range tm.Render(tm.cols, tm.rows) {
		for _, s := range l {
			b.WriteString(s.Text)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// editName changes the name of the item whose page is open, and saves.
func editName(tm *syncModel, name string) {
	tm.Rune('e')
	for range tm.edit.fields[rowName] {
		tm.Key(gui.KeyBackspace)
	}
	for _, r := range name {
		tm.Rune(r)
	}
	tm.Key(gui.KeyUp) // from Name up to Save
	tm.Key(gui.KeyEnter)
}

// deleteOpen deletes the item whose page is open.
func deleteOpen(tm *syncModel) {
	tm.Rune('d')
	tm.typ("yes")
}
