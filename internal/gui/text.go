package gui

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// Span is a piece of text with its own color; a Line is a sequence of spans.
type Span struct {
	Text  string
	Color color.Color // nil = default text color
	Bold  bool
}

type Line []Span

func Plain(t string) Span                  { return Span{Text: t} }
func Colored(t string, c color.Color) Span { return Span{Text: t, Color: c} }
func Strong(t string, c color.Color) Span  { return Span{Text: t, Color: c, Bold: true} }

// Truncate cuts s to n characters, ending with "…" when something is left out.
func Truncate(s string, n int) string {
	r := []rune(s)
	if n <= 0 || len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// DisplayText makes line breaks and tabs visible on a single line.
func DisplayText(s string) string {
	return strings.NewReplacer("\r\n", "↵", "\n", "↵", "\r", "↵", "\t", "→").Replace(s)
}

// Draw writes the lines into the console, leaving a margin of one row and con.Margin columns.
// Whatever does not fit in the window is cut.
func Draw(con *Console, lines []Line) {
	rows := make([]widget.TextGridRow, max(con.Rows, 1))
	for i := range rows {
		rows[i].Cells = make([]widget.TextGridCell, con.Margin, max(con.Cols, con.Margin))
		for j := range rows[i].Cells {
			rows[i].Cells[j].Rune = ' '
		}
	}
	for i, l := range lines {
		if i+1 >= len(rows) {
			break
		}
		row := &rows[i+1]
		for _, s := range l {
			style := &widget.CustomTextGridStyle{FGColor: s.Color, TextStyle: fyne.TextStyle{Bold: s.Bold, Monospace: true}}
			for _, r := range s.Text {
				if len(row.Cells) < con.Cols {
					row.Cells = append(row.Cells, widget.TextGridCell{Rune: r, Style: style})
				}
			}
		}
	}

	con.DrawRows(rows)
}
