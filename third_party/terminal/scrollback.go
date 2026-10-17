package terminal

import "fyne.io/fyne/v2/widget"

// GL Tools: scrollback support. Lines that leave the top of the screen are kept here,
// so that the application can show them again when the user scrolls back.

// maxScrollback is the number of lines kept.
const maxScrollback = 5000

func (t *Terminal) keepScrollback(row widget.TextGridRow) {
	cells := make([]widget.TextGridCell, len(row.Cells))
	copy(cells, row.Cells)

	t.scrollbackLock.Lock()
	defer t.scrollbackLock.Unlock()
	t.scrollback = append(t.scrollback, widget.TextGridRow{Cells: cells, Style: row.Style})
	if extra := len(t.scrollback) - maxScrollback; extra > 0 {
		t.scrollback = append(t.scrollback[:0:0], t.scrollback[extra:]...)
	}
}

// ScrollbackRows returns a copy of the lines scrolled off the screen (oldest first)
// followed by the lines currently on screen.
func (t *Terminal) ScrollbackRows() []widget.TextGridRow {
	t.scrollbackLock.Lock()
	rows := append([]widget.TextGridRow(nil), t.scrollback...)
	t.scrollbackLock.Unlock()

	if t.content != nil {
		rows = append(rows, t.content.Rows...)
	}
	return rows
}

// AltScreenActive tells whether a full screen program (e.g. vi, top) is running.
func (t *Terminal) AltScreenActive() bool {
	return t.altBufferActive
}
