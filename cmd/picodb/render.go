package main

import (
	"fmt"
	"strings"
	"time"

	"picotools/internal/gui"
)

type line = gui.Line

func (m *model) Render(cols, rows int) []line {
	m.cols, m.height = cols, rows
	lines := []line{
		{gui.Strong("╭─────────────────╮", gui.Accent)},
		{gui.Strong("│     Pico DB     │", gui.Accent)},
		{gui.Strong("╰─────────────────╯", gui.Accent)},
		nil,
	}
	switch m.screen {
	case screenConnections:
		lines = append(lines, m.renderConnections()...)
	case screenDatabases:
		lines = append(lines, m.renderDatabases()...)
	case screenMenu:
		lines = append(lines, m.renderMenu()...)
	case screenTables:
		lines = append(lines, m.renderTables()...)
	case screenTable:
		fav := "3 add to favourites"
		if m.isFavourite(m.table.full()) {
			fav = "3 remove from favourites"
		}
		lines = append(lines, m.renderTable("1 show CREATE TABLE · 2 select all · "+fav+" · ↑/↓ move · n/p or pgdn/pgup page · f favourite column · esc back")...)
	case screenCreate:
		lines = append(lines, m.renderViewer("c copy · s save · ↑/↓ scroll · pgup/pgdn page · esc back")...)
	case screenData:
		lines = append(lines, m.renderData()...)
	case screenForm:
		lines = append(lines, m.form.render(cols)...)
	case screenConfirm:
		for _, l := range wrap(m.confirm, cols-2) {
			lines = append(lines, line{gui.Strong(l, gui.Amber)})
		}
		lines = append(lines, nil, line{gui.Colored("y yes · any other key no", gui.Muted)})
	case screenBusy:
		lines = append(lines, m.renderBusy()...)
	}
	return lines
}

// crumbs is where we are: connection › database › table.
func (m *model) crumbs() string {
	parts := []string{}
	if m.conn != nil {
		parts = append(parts, m.conn.Name)
	}
	if m.dbName != "" && (m.screen == screenMenu || m.screen == screenTables || m.table != nil) {
		parts = append(parts, m.dbName)
	}
	if m.table != nil && m.screen != screenTables && m.screen != screenMenu {
		parts = append(parts, m.table.full())
	}
	return strings.Join(parts, " › ")
}

// listRoom is how many entries of a list fit: the window less the title (4 lines),
// top margin, the list header, the status and the hint, and the "new" entry of the connections.
// A page has at most pageSize entries, as in Select all.
func (m *model) listRoom() int {
	room := m.height - 9
	if m.screen == screenConnections {
		room -= 2
	}
	if m.status != nil {
		room -= 2
	}
	return min(max(room, 1), pageSize)
}

// entries draws a list with the selected one marked, a page at a time: the page that holds it.
// Each entry is its label (padded to the same width) and a detail in grey; a marked
// entry has an asterisk after its number.
func (m *model) entries(labels, details []string, marked []bool, cursor int) []line {
	width := 0
	for _, l := range labels {
		width = max(width, len([]rune(l)))
	}
	room := m.listRoom()
	start := cursor / room * room
	end := min(start+room, len(labels))
	numWidth := len(fmt.Sprint(len(labels)))
	var lines []line
	for i := start; i < end; i++ {
		num := fmt.Sprintf("%*d", numWidth, i+1)
		label := fmt.Sprintf("%-*s", width, labels[i])
		gap := "  "
		if marked != nil && marked[i] {
			gap = "* "
		}
		detail := gui.Truncate(details[i], m.cols-width-numWidth-10)
		if i == cursor {
			lines = append(lines, line{gui.Strong("› "+num+gap+label, gui.Accent), gui.Colored("   "+detail, gui.Muted)}.Highlighted(m.cols-2))
		} else {
			lines = append(lines, line{gui.Plain("  "), gui.Strong(num+gap[:1], gui.Accent), gui.Plain(gap[1:] + label), gui.Colored("   "+detail, gui.Muted)})
		}
	}
	return lines
}

// footer is the status, if any, and the hint.
func (m *model) footer(hint string) []line {
	lines := []line{nil}
	if m.status != nil {
		lines = append(lines, m.status, nil)
	}
	return append(lines, line{gui.Colored(hint, gui.Muted)})
}

// pickHint says how numbers work in a list of n entries.
func (m *model) pickHint(n int) string {
	if n <= 9 {
		return "number quick pick"
	}
	if m.number != "" {
		return "number " + m.number + "… enter open · backspace"
	}
	return "type a number and enter"
}

func (m *model) renderConnections() []line {
	cs := m.store.Connections
	labels := make([]string, len(cs))
	details := make([]string, len(cs))
	for i, c := range cs {
		labels[i] = c.Name
		details[i] = m.whereOf(c)
	}
	var lines []line
	if len(cs) == 0 {
		lines = append(lines, line{gui.Colored("No connections yet: press n to add the first one.", gui.Muted)})
	}
	lines = append(lines, m.entries(labels, details, nil, m.connCursor)...)
	lines = append(lines, nil, line{gui.Plain("  "), gui.Strong("n", gui.Accent), gui.Plain("  new connection")})
	hint := "n new · esc quit"
	if len(cs) > 0 {
		hint = "↑/↓ move · enter open · number quick pick · n new · m edit · d delete · " + gui.FullScreenKey + " full screen · esc quit"
	}
	return append(lines, m.footer(hint)...)
}

func (m *model) renderDatabases() []line {
	lines := []line{{gui.Strong(m.conn.Name, nil), gui.Colored("   "+m.info, gui.Muted)}, nil}
	if len(m.dbs) == 0 {
		lines = append(lines, line{gui.Colored("No databases this user can open.", gui.Muted)})
	}
	lines = append(lines, m.entries(m.dbs, make([]string, len(m.dbs)), nil, m.dbCursor)...)
	return append(lines, m.footer("↑/↓ move · enter open · "+m.pickHint(len(m.dbs))+" · esc back")...)
}

func (m *model) renderMenu() []line {
	lines := []line{{gui.Strong(m.crumbs(), nil), gui.Colored("   "+m.info, gui.Muted)}, nil}
	lines = append(lines, m.entries([]string{"Tables"}, []string{""}, nil, m.menuCursor)...)
	return append(lines, m.footer("↑/↓ move · enter open · number quick pick · esc back")...)
}

func (m *model) renderTables() []line {
	shown := m.shownTables()
	info, toggle := fmt.Sprintf("   %d tables", len(m.tables)), "f only favourites"
	if m.favOnly {
		info, toggle = fmt.Sprintf("   %d favourites of %d tables", len(shown), len(m.tables)), "f all tables"
	}
	lines := []line{{gui.Strong(m.crumbs()+" › Tables", nil), gui.Colored(info, gui.Muted)}, nil}
	switch {
	case len(m.tables) == 0:
		lines = append(lines, line{gui.Colored("No tables in this database.", gui.Muted)})
	case len(shown) == 0:
		lines = append(lines, line{gui.Colored("No favourites yet: open a table and press 3 to add it.", gui.Muted)})
	}
	labels := make([]string, len(shown))
	details := make([]string, len(shown))
	marked := make([]bool, len(shown))
	for i, j := range shown {
		t := m.tables[j]
		labels[i] = t.full()
		if !m.favOnly && m.isFavourite(t.full()) {
			marked[i] = true
		}
		details[i] = thousands(t.Rows) + " rows"
		if t.Rows == 1 {
			details[i] = "1 row"
		}
	}
	lines = append(lines, m.entries(labels, details, marked, m.tableCursor)...)
	return append(lines, m.footer("↑/↓ move · n/p or pgdn/pgup page · enter open · "+m.pickHint(len(shown))+" · "+toggle+" · esc back")...)
}

// viewRoom is how many rows of the viewer fit: the window less the title (4 lines),
// top margin, the crumbs and the blank under them, the status and the hint.
func (m *model) viewRoom() int {
	room := m.height - 9
	if m.status != nil {
		room -= 2
	}
	return max(room, 1)
}

func (m *model) renderViewer(hint string) []line {
	title := m.crumbs()
	if m.screen == screenCreate {
		title += " › CREATE TABLE"
	}
	lines := []line{{gui.Strong(title, nil)}, nil}
	rows, where := m.viewer.Render(m.cols, m.viewRoom())
	lines = append(lines, rows...)
	return append(lines, m.footer(hint+" · "+where)...)
}

// renderTable draws the table page with the selected column highlighted.
func (m *model) renderTable(hint string) []line {
	lines := []line{{gui.Strong(m.crumbs(), nil)}, nil}
	room := m.viewRoom()
	m.tableScroll()
	end := min(m.tableTop+room, len(m.tableLines))
	for i := m.tableTop; i < end; i++ {
		l := line{gui.Plain(gui.Truncate(m.tableLines[i], m.cols-2))}
		if i == m.colFirst+m.colCursor && m.colCursor < len(m.table.Columns) {
			l = l.Highlighted(m.cols - 2)
		}
		lines = append(lines, l)
	}
	for i := end - m.tableTop; i < room; i++ {
		lines = append(lines, nil)
	}
	return append(lines, m.footer(hint)...)
}

// tablePage is the text of the table page: rows, columns and indexes, one line each,
// and the line with the first column. A favourite column has an asterisk after its number.
func tablePage(t *tableInfo, fav func(name string) bool) ([]string, int) {
	var b strings.Builder
	fmt.Fprintf(&b, "Rows: %s\n", thousands(t.Rows))
	if t.PartitionBy != "" {
		fmt.Fprintf(&b, "Partitioned by %s · %d partitions\n", t.PartitionBy, len(t.Partitions))
	}
	b.WriteString("\n")

	pk := map[string]bool{}
	for _, k := range t.primaryKey() {
		pk[k] = true
	}
	fk := map[string]string{}
	for _, f := range t.FKs {
		for i, c := range f.Cols {
			fk[c] = "FK → " + f.RefTable + "." + f.RefCols[i]
		}
	}
	rows := [][]string{{"# ", "Name", "Type", "Null", "Key"}}
	for i, c := range t.Columns {
		typ := c.Type
		switch {
		case c.Computed != "":
			typ = "AS " + c.Computed
		case c.Serial:
			typ += " serial"
		case c.Identity:
			typ += fmt.Sprintf(" identity(%s,%s)", c.Seed, c.Increment)
		}
		null := "no"
		if c.Nullable {
			null = "yes"
		}
		var keys []string
		if pk[c.Name] {
			keys = append(keys, "PK")
		}
		if f, ok := fk[c.Name]; ok {
			keys = append(keys, f)
		}
		num := fmt.Sprint(i+1) + " "
		if fav(c.Name) {
			num = fmt.Sprint(i+1) + "*"
		}
		rows = append(rows, []string{num, c.Name, typ, null, strings.Join(keys, " · ")})
	}
	widths := make([]int, len(rows[0]))
	for _, r := range rows {
		for i, v := range r {
			widths[i] = max(widths[i], len([]rune(v)))
		}
	}
	first := strings.Count(b.String(), "\n") + 1
	for _, r := range rows {
		b.WriteString(" ")
		for i, v := range r {
			if i == 0 {
				fmt.Fprintf(&b, " %*s", widths[i], v)
			} else if i < len(r)-1 {
				fmt.Fprintf(&b, "  %-*s", widths[i], v)
			} else {
				b.WriteString("  " + v)
			}
		}
		b.WriteString("\n")
	}

	if len(t.Indexes) > 0 {
		b.WriteString("\nIndexes:\n")
		width := 0
		for _, ix := range t.Indexes {
			width = max(width, len([]rune(ix.Name)))
		}
		for _, ix := range t.Indexes {
			what := strings.ToLower(ix.Kind)
			switch {
			case ix.Primary:
				what += " · primary key"
			case ix.Unique:
				what += " · unique"
			}
			keys := make([]string, len(ix.Keys))
			for i, k := range ix.Keys {
				keys[i] = k
				if ix.Desc[i] {
					keys[i] += " DESC"
				}
			}
			cols := strings.Join(keys, ", ")
			if len(ix.Include) > 0 {
				cols += " · include " + strings.Join(ix.Include, ", ")
			}
			fmt.Fprintf(&b, "  %-*s  (%s) · %s\n", width, ix.Name, cols, what)
		}
	}
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n"), first
}

// ---- Select all ----

// gridRoom is how many rows of the page fit: the window less the title (4 lines),
// top margin, the crumbs, the blank under them, the column names, the status and the hint.
func (m *model) gridRoom() int {
	room := m.height - 10
	if m.status != nil {
		room -= 2
	}
	return max(room, 1)
}

// maxCell is the widest a value is shown; the export writes it whole.
const maxCell = 40

func (m *model) renderData() []line {
	t := m.table
	first := m.page*pageSize + 1
	last := m.page*pageSize + len(m.rows)
	pages := (int(t.Rows) + pageSize - 1) / pageSize
	info := fmt.Sprintf("   rows %s–%s of %s · page %d of %d", thousands(int64(first)), thousands(int64(last)), thousands(t.Rows), m.page+1, max(pages, m.page+1))
	if len(m.rows) == 0 {
		info = "   no rows"
	}
	if len(m.marked) == 1 {
		info += " · 1 marked"
	} else if len(m.marked) > 1 {
		info += fmt.Sprintf(" · %d marked", len(m.marked))
	}
	lines := []line{{gui.Strong(m.crumbs()+" › Select all", nil), gui.Colored(info, gui.Muted)}, nil}

	// Text and width of every cell of the page, for the columns shown.
	shownCols := m.shownColumns()
	cols := make([]column, len(shownCols))
	for j, k := range shownCols {
		cols[j] = t.Columns[k]
	}
	cells := make([][]string, len(m.rows))
	nulls := make([][]bool, len(m.rows))
	widths := make([]int, len(cols))
	names := make([]string, len(cols))
	for j, c := range cols {
		names[j] = gui.Truncate(c.Name, maxCell)
		if a := m.sortArrow(shownCols[j]); a != "" {
			names[j] = a + " " + names[j]
		}
		widths[j] = len([]rune(names[j]))
	}
	for i, r := range m.rows {
		cells[i] = make([]string, len(cols))
		nulls[i] = make([]bool, len(cols))
		for j, k := range shownCols {
			s, null := m.eng.text(r[k], cols[j].Base)
			if null {
				s = "NULL"
			}
			s = gui.Truncate(gui.DisplayText(s), maxCell)
			cells[i][j], nulls[i][j] = s, null
			widths[j] = max(widths[j], len([]rune(s)))
		}
	}
	numWidth := len(fmt.Sprint(last))

	// The columns that fit, from gridLeft, scrolled to keep the selected one in view.
	fit := func(left int) int {
		end, used := left, numWidth+2
		for end < len(cols) && (end == left || used+widths[end]+2 <= m.cols-2) {
			used += widths[end] + 2
			end++
		}
		return end
	}
	m.gridCol = max(min(m.gridCol, len(cols)-1), 0)
	m.gridLeft = max(min(m.gridLeft, m.gridCol), 0)
	end := fit(m.gridLeft)
	for m.gridCol >= end && m.gridLeft < m.gridCol {
		m.gridLeft++
		end = fit(m.gridLeft)
	}

	pad := func(s string, w int) string { return s + strings.Repeat(" ", max(w-len([]rune(s)), 0)) }
	header := line{gui.Plain(strings.Repeat(" ", numWidth+2))}
	for j := m.gridLeft; j < end; j++ {
		name := gui.Strong(pad(names[j], widths[j]), gui.Accent)
		if j == m.gridCol {
			name.Bg = gui.SelectedBg
		}
		header = append(header, name, gui.Plain("  "))
	}
	lines = append(lines, header)

	room := m.gridRoom()
	// Scroll to keep the selected row in view.
	m.gridTop = min(m.gridTop, m.gridCursor)
	m.gridTop = max(m.gridTop, m.gridCursor-room+1)
	m.gridTop = max(min(m.gridTop, len(m.rows)-room), 0)
	shown := 0
	for i := m.gridTop; i < len(m.rows) && shown < room; i++ {
		marked := m.isMarked(i)
		l := line{gui.Colored(fmt.Sprintf("%*d  ", numWidth, m.page*pageSize+i+1), gui.Muted)}
		if marked {
			l = line{gui.Strong(fmt.Sprintf("%*d* ", numWidth, m.page*pageSize+i+1), gui.Amber)}
		}
		for j := m.gridLeft; j < end; j++ {
			cell := pad(cells[i][j], widths[j]) + "  "
			switch {
			case nulls[i][j]:
				l = append(l, gui.Colored(cell, gui.Muted))
			case marked:
				l = append(l, gui.Colored(cell, gui.Amber))
			default:
				l = append(l, gui.Plain(cell))
			}
		}
		if i == m.gridCursor {
			l = l.Highlighted(m.cols - 2)
		}
		lines = append(lines, l)
		shown++
	}
	for ; shown < room; shown++ {
		lines = append(lines, nil)
	}

	hint := "↑/↓ move · n/p or pgdn/pgup page"
	if len(cols) > 1 {
		hint += fmt.Sprintf(" · ←/→ column (%d of %d)", m.gridCol+1, len(cols))
	}
	hint += " · o order by"
	if m.hasFavColumns(true) {
		if m.favCols {
			hint += " · f all columns"
		} else {
			hint += " · f only favourite columns"
		}
	}
	hint += " · space mark row"
	if len(m.marked) > 0 {
		hint += " · u unmark all · c copy marked · j copy marked as JSON ·"
	} else {
		hint += " · c copy row · j copy row as JSON ·"
	}
	hint += " e export CSV · i export INSERT · d export DROP + CREATE + INSERT · esc back"
	return append(lines, m.footer(hint)...)
}

// ---- Busy ----

var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (m *model) renderBusy() []line {
	waited := m.now().Sub(m.started)
	frame := spinner[int(waited/(250*time.Millisecond))%len(spinner)]
	msg := fmt.Sprintf("%s %s %ds", frame, m.busy, int(waited.Seconds()))
	if m.done != nil {
		msg += " · " + m.done()
	}
	return []line{
		{gui.Strong(msg, gui.Accent)},
		nil,
		{gui.Colored("esc stop", gui.Muted)},
	}
}

// wrap cuts a text into lines of at most width characters, at spaces when it can.
func wrap(s string, width int) []string {
	width = max(width, 10)
	var out []string
	for _, para := range strings.Split(s, "\n") {
		r := []rune(para)
		for len(r) > width {
			cut := width
			for i := width; i > width/2; i-- {
				if r[i] == ' ' {
					cut = i
					break
				}
			}
			out = append(out, string(r[:cut]))
			r = []rune(strings.TrimLeft(string(r[cut:]), " "))
		}
		out = append(out, string(r))
	}
	return out
}
