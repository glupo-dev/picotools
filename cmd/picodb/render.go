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
		{gui.Strong("╭────────────────╮", gui.Accent)},
		{gui.Strong("│     Pico DB      │", gui.Accent)},
		{gui.Strong("╰────────────────╯", gui.Accent)},
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
		lines = append(lines, m.renderViewer("1 show CREATE TABLE · 2 select all · "+fav+" · ↑/↓ scroll · esc back")...)
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
// top margin, the list header, the "new" entry, the status and the hint.
func (m *model) listRoom() int {
	return max(m.height-11, 1)
}

// entries draws a list with the selected one marked, scrolled to keep it in view.
// Each entry is its label (padded to the same width) and a detail in grey; a marked
// entry has an asterisk after its number.
func (m *model) entries(labels, details []string, marked []bool, cursor int) []line {
	width := 0
	for _, l := range labels {
		width = max(width, len([]rune(l)))
	}
	room := m.listRoom()
	start := min(max(cursor-room/2, 0), max(len(labels)-room, 0))
	end := min(start+room, len(labels))
	numWidth := len(fmt.Sprint(len(labels)))
	var lines []line
	if start > 0 {
		lines = append(lines, line{gui.Colored(fmt.Sprintf("  ↑ %d more", start), gui.Muted)})
	}
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
	if end < len(labels) {
		lines = append(lines, line{gui.Colored(fmt.Sprintf("  ↓ %d more", len(labels)-end), gui.Muted)})
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

// tablePage is the text of the table page: rows, columns and indexes.
func tablePage(t *tableInfo) string {
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
	rows := [][]string{{"#", "Name", "Type", "Null", "Key"}}
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
		rows = append(rows, []string{fmt.Sprint(i + 1), c.Name, typ, null, strings.Join(keys, " · ")})
	}
	widths := make([]int, len(rows[0]))
	for _, r := range rows {
		for i, v := range r {
			widths[i] = max(widths[i], len([]rune(v)))
		}
	}
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
	return strings.TrimRight(b.String(), "\n")
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
	lines := []line{{gui.Strong(m.crumbs()+" › Select all", nil), gui.Colored(info, gui.Muted)}, nil}

	// Text and width of every cell of the page.
	cells := make([][]string, len(m.rows))
	nulls := make([][]bool, len(m.rows))
	widths := make([]int, len(t.Columns))
	for j, c := range t.Columns {
		widths[j] = min(len([]rune(c.Name)), maxCell)
	}
	for i, r := range m.rows {
		cells[i] = make([]string, len(r))
		nulls[i] = make([]bool, len(r))
		for j, v := range r {
			s, null := m.eng.text(v, t.Columns[j].Base)
			if null {
				s = "NULL"
			}
			s = gui.Truncate(gui.DisplayText(s), maxCell)
			cells[i][j], nulls[i][j] = s, null
			widths[j] = max(widths[j], len([]rune(s)))
		}
	}
	numWidth := len(fmt.Sprint(last))

	// The columns that fit, from gridLeft.
	end := m.gridLeft
	used := numWidth + 2
	for end < len(t.Columns) && (end == m.gridLeft || used+widths[end]+2 <= m.cols-2) {
		used += widths[end] + 2
		end++
	}

	pad := func(s string, w int) string { return s + strings.Repeat(" ", max(w-len([]rune(s)), 0)) }
	header := line{gui.Plain(strings.Repeat(" ", numWidth+2))}
	for j := m.gridLeft; j < end; j++ {
		header = append(header, gui.Strong(pad(gui.Truncate(t.Columns[j].Name, maxCell), widths[j])+"  ", gui.Accent))
	}
	lines = append(lines, header)

	room := m.gridRoom()
	// Scroll to keep the selected row in view.
	m.gridTop = min(m.gridTop, m.gridCursor)
	m.gridTop = max(m.gridTop, m.gridCursor-room+1)
	m.gridTop = max(min(m.gridTop, len(m.rows)-room), 0)
	shown := 0
	for i := m.gridTop; i < len(m.rows) && shown < room; i++ {
		l := line{gui.Colored(fmt.Sprintf("%*d  ", numWidth, m.page*pageSize+i+1), gui.Muted)}
		for j := m.gridLeft; j < end; j++ {
			cell := pad(cells[i][j], widths[j]) + "  "
			if nulls[i][j] {
				l = append(l, gui.Colored(cell, gui.Muted))
			} else {
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
	if end-m.gridLeft < len(t.Columns) {
		hint += fmt.Sprintf(" · ←/→ columns (%d–%d of %d)", m.gridLeft+1, end, len(t.Columns))
	}
	hint += " · e export CSV · i export INSERT · d export DROP + CREATE + INSERT · esc back"
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
