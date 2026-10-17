package gui

import (
	"fmt"
	"strings"
)

// Viewer shows a long text to scroll through: JSON and XML are colored, each line can
// have its number, and a line too long goes on in the next rows, under the text.
type Viewer struct {
	lines   []string
	lang    string // "JSON" or "XML" to color the text, "" for plain text
	numbers bool
	top     int // first visible row, counting wrapped rows
}

// Set puts a new text in the viewer, from the top.
func (v *Viewer) Set(text, lang string, numbers bool) {
	v.lines = strings.Split(strings.ReplaceAll(text, "\t", "    "), "\n")
	v.lang, v.numbers, v.top = lang, numbers, 0
}

// Len is the number of lines of the text.
func (v *Viewer) Len() int { return len(v.lines) }

// rows splits the text into rows of the given width.
func (v *Viewer) rows(cols int) []Line {
	numWidth := 0
	if v.numbers {
		numWidth = len(fmt.Sprint(len(v.lines)))
	}
	width := max(cols-numWidth-4, 10)
	var rows []Line
	for i, l := range v.lines {
		for j, part := range WrapSpans(Highlight(v.lang, l), width) {
			num := strings.Repeat(" ", numWidth)
			if j == 0 && numWidth > 0 {
				num = fmt.Sprintf("%*d", numWidth, i+1)
			}
			rows = append(rows, append(Line{Colored(num+"  ", Muted)}, part...))
		}
	}
	return rows
}

// Scroll moves the text by n rows (negative = up) in a window of cols x room.
func (v *Viewer) Scroll(n, cols, room int) {
	v.top = min(max(v.top+n, 0), max(len(v.rows(cols))-room, 0))
}

// Key scrolls for the keys up, down, pageup, pagedown, home and end; it says whether it used the key.
func (v *Viewer) Key(k string, cols, room int) bool {
	page := max(room-1, 1)
	switch k {
	case KeyUp:
		v.Scroll(-1, cols, room)
	case KeyDown:
		v.Scroll(1, cols, room)
	case KeyPageUp:
		v.Scroll(-page, cols, room)
	case KeyPageDown:
		v.Scroll(page, cols, room)
	case KeyHome:
		v.top = 0
	case KeyEnd:
		v.Scroll(len(v.rows(cols)), cols, room)
	default:
		return false
	}
	return true
}

// Render gives exactly room rows of the text, from the scroll position, and where the
// view is: "all shown" or the percentage reached.
func (v *Viewer) Render(cols, room int) ([]Line, string) {
	rows := v.rows(cols)
	v.top = min(v.top, max(len(rows)-room, 0)) // the window may have grown
	end := min(v.top+room, len(rows))
	out := append([]Line(nil), rows[v.top:end]...)
	for len(out) < room {
		out = append(out, nil)
	}
	where := "all shown"
	if len(rows) > room {
		where = fmt.Sprintf("%d%%", end*100/len(rows))
	}
	return out, where
}
