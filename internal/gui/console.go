package gui

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

var selectionBg = color.NRGBA{0x3A, 0x35, 0x70, 0xFF}

// leftMargin is the number of empty columns on the left of every row.
const leftMargin = 2

// cellPos is a position in the console, in characters.
type cellPos struct{ row, col int }

func (p cellPos) before(q cellPos) bool {
	return p.row < q.row || (p.row == q.row && p.col < q.col)
}

// Console is the text area of the window. It follows the window size,
// lets the user select text with the mouse and receives right clicks.
type Console struct {
	widget.BaseWidget
	grid       *widget.TextGrid
	Cols, Rows int
	Margin     int                  // empty columns on the left, not copied with the selection
	base       []widget.TextGridRow // content without the selection highlight

	selecting bool
	selStart  cellPos
	selEnd    cellPos
	hasSel    bool
	onResize  func()
	onRight   func(pos fyne.Position)
	OnScroll  func(e *fyne.ScrollEvent) // optional: mouse wheel / trackpad
}

func NewConsole(onResize func(), onRight func(fyne.Position)) *Console {
	c := &Console{grid: widget.NewTextGrid(), Cols: DefaultCols, Rows: DefaultRows, Margin: leftMargin, onResize: onResize, onRight: onRight}
	c.ExtendBaseWidget(c)
	return c
}

func (c *Console) CreateRenderer() fyne.WidgetRenderer { return widget.NewSimpleRenderer(c.grid) }

func (c *Console) TappedSecondary(e *fyne.PointEvent) { c.onRight(e.AbsolutePosition) }

func (c *Console) Scrolled(e *fyne.ScrollEvent) {
	if c.OnScroll != nil {
		c.OnScroll(e)
	}
}

// MinSize lets the window shrink down to MinCols x MinRows characters.
func (c *Console) MinSize() fyne.Size {
	cell := CellSize()
	return fyne.NewSize(cell.Width*MinCols, cell.Height*MinRows)
}

func (c *Console) Resize(size fyne.Size) {
	c.BaseWidget.Resize(size)
	cell := CellSize()
	cols, rows := int(size.Width/cell.Width), int(size.Height/cell.Height)
	if cols != c.Cols || rows != c.Rows {
		c.Cols, c.Rows = cols, rows
		c.onResize()
	}
}

// ---- Mouse selection ----

func (c *Console) cellAt(p fyne.Position) cellPos {
	cell := CellSize()
	return cellPos{row: max(0, int(p.Y/cell.Height)), col: max(0, int(p.X/cell.Width))}
}

func (c *Console) MouseDown(e *desktop.MouseEvent) {
	if e.Button != desktop.MouseButtonPrimary {
		return
	}
	c.selecting = true
	c.selStart = c.cellAt(e.Position)
	c.selEnd = c.selStart
	c.hasSel = false
	c.render()
}

func (c *Console) MouseUp(*desktop.MouseEvent) { c.selecting = false }

func (c *Console) Dragged(e *fyne.DragEvent) {
	if !c.selecting {
		return
	}
	c.selEnd = c.cellAt(e.Position)
	c.hasSel = c.selEnd != c.selStart
	c.render()
}

func (c *Console) DragEnd() { c.selecting = false }

func (c *Console) ClearSelection() {
	c.selecting, c.hasSel = false, false
}

// selectionRange returns the selection with the start before the end.
func (c *Console) selectionRange() (cellPos, cellPos) {
	if c.selEnd.before(c.selStart) {
		return c.selEnd, c.selStart
	}
	return c.selStart, c.selEnd
}

func (c *Console) selected(p cellPos) bool {
	if !c.hasSel {
		return false
	}
	from, to := c.selectionRange()
	return !p.before(from) && !to.before(p)
}

// SelectedText returns the selected text, one line per row, without the left margin
// and without trailing spaces.
func (c *Console) SelectedText() string {
	if !c.hasSel {
		return ""
	}
	from, to := c.selectionRange()
	var out []string
	for r := from.row; r <= to.row && r < len(c.grid.Rows); r++ {
		var b strings.Builder
		for col, cell := range c.grid.Rows[r].Cells {
			if col >= c.Margin && c.selected(cellPos{r, col}) {
				b.WriteRune(cell.Rune)
			}
		}
		out = append(out, strings.TrimRight(b.String(), " "))
	}
	return strings.Trim(strings.Join(out, "\n"), "\n")
}

// ---- Drawing ----

// DrawRows shows rows that are already built (e.g. from the shell history).
func (c *Console) DrawRows(rows []widget.TextGridRow) {
	c.base = rows
	c.render()
}

// render shows the content, giving the selected characters a highlighted background.
func (c *Console) render() {
	rows := make([]widget.TextGridRow, len(c.base))
	copy(rows, c.base)
	if c.hasSel {
		for r := range rows {
			// Copy the cells before changing their style, so c.base is not modified.
			rows[r].Cells = append([]widget.TextGridCell(nil), rows[r].Cells...)
			for col := range rows[r].Cells {
				if !c.selected(cellPos{r, col}) {
					continue
				}
				cell := &rows[r].Cells[col]
				sel := &widget.CustomTextGridStyle{BGColor: selectionBg, TextStyle: fyne.TextStyle{Monospace: true}}
				if cell.Style != nil {
					sel.FGColor = cell.Style.TextColor()
					sel.TextStyle = cell.Style.Style()
				}
				cell.Style = sel
			}
		}
	}

	c.grid.Rows = rows
	c.grid.Refresh()
}

// Cursor shows the text cursor over the console, as in a terminal.
func (c *Console) Cursor() desktop.Cursor { return desktop.TextCursor }
