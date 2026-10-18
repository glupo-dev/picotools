package main

import (
	"fmt"
	"strings"
	"unicode"

	"picotools/internal/gui"
)

// Colors, text pieces and keys come from the gui package, shared with the other Pico apps.
var (
	accent  = gui.Accent
	amber   = gui.Amber
	red     = gui.Red
	muted   = gui.Muted
	valueFg = gui.ValueFg
)

type (
	span = gui.Span
	line = gui.Line
)

var (
	plain   = gui.Plain
	colored = gui.Colored
	strong  = gui.Strong
)

const (
	keyUp        = gui.KeyUp
	keyDown      = gui.KeyDown
	keyEnter     = gui.KeyEnter
	keyEsc       = gui.KeyEsc
	keyBackspace = gui.KeyBackspace
	keyPageUp    = gui.KeyPageUp
	keyPageDown  = gui.KeyPageDown
	keyHome      = gui.KeyHome
	keyEnd       = gui.KeyEnd
	keyOther     = gui.KeyOther
)

type screen int

const (
	screenMenu screen = iota
	screenInput
	screenResults
	screenBusy // waiting for a tool that takes time (e.g. network)
	screenView // a long text to scroll through (Tool.View)
	screenPick // a list of results to choose from, each opened in screenView (Tool.Pick)
)

// model holds the state of the console and reacts to keys.
// It knows nothing about the window: copying and quitting are done through callbacks.
// submenu is an open submenu; cursor is the entry selected in the menu above it.
type submenu struct {
	name   string
	items  []Tool
	cursor int
}

// pickLevel is one list of screenPick: the results to choose from and, for a list opened
// from an entry, that entry's name and info line.
type pickLevel struct {
	title string
	info  string
	items []Result
	pos   int
}

type model struct {
	screen    screen
	cursor    int       // selected menu entry
	submenus  []submenu // open submenus, the last one is shown; empty = main menu
	tool      Tool      // tool currently in use
	input     []rune
	lastInput string
	info      []string // lines from Tool.Info, shown above the prompt
	results   []Result
	err       error       // error returned by the last tool, shown instead of the results
	resCursor int         // selected result
	runID     int         // identifies the current run, so late results of a cancelled run are ignored
	viewer    gui.Viewer  // the text shown in screenView
	viewTitle string      // shown instead of the tool name above the text
	viewText  string      // the text as it is, copied with "c"
	picks     []pickLevel // lists opened in screenPick, the last one is shown
	status    line        // message shown under the menu (e.g. "Copied")
	cols      int         // current width of the console in characters
	rows      int         // current height of the console in lines

	copy func(text string)
	quit func()
	// background runs work outside the user interface and then calls done in it.
	background func(work func() ([]Result, error), done func([]Result, error))
	// ui runs f in the user interface and redraws; it can be called from any goroutine.
	ui func(f func())
}

// ---- Input ----

// items are the entries of the menu currently shown.
func (m *model) items() []Tool {
	if n := len(m.submenus); n > 0 {
		return m.submenus[n-1].items
	}
	return tools
}

// back closes the current submenu, or quits from the main menu.
func (m *model) back() {
	n := len(m.submenus)
	if n == 0 {
		m.quit()
		return
	}
	m.cursor = m.submenus[n-1].cursor
	m.submenus = m.submenus[:n-1]
	m.status = nil
}

// The menu has one entry per tool plus "Exit" (or "Back" in a submenu) as the last one.
func (m *model) key(k string) {
	m.clearStatus()
	switch m.screen {
	case screenMenu:
		items := m.items()
		entries := len(items) + 1
		switch k {
		case keyUp:
			m.cursor = (m.cursor - 1 + entries) % entries
		case keyDown:
			m.cursor = (m.cursor + 1) % entries
		case keyEnter:
			if m.cursor == len(items) {
				m.back()
				return
			}
			m.openTool(m.cursor)
		case keyEsc:
			m.back()
		}

	case screenInput:
		switch k {
		case keyEsc:
			m.screen = screenMenu
		case keyEnter:
			if m.tool.Again && strings.TrimSpace(string(m.input)) == "" {
				m.screen = screenMenu
				return
			}
			m.runTool(string(m.input))
		case keyBackspace:
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
		}

	case screenResults:
		// A report taller than the window scrolls with the arrows; resCursor is its first visible row.
		if m.tool.CopyAll {
			switch k {
			case keyUp:
				m.resCursor = max(m.resCursor-1, 0)
				return
			case keyDown:
				m.resCursor = min(m.resCursor+1, max(len(m.results)-m.resultsRoom(), 0))
				return
			}
		}
		if len(m.results) > 1 && !m.tool.CopyAll {
			switch k {
			case keyUp:
				m.resCursor = (m.resCursor - 1 + len(m.results)) % len(m.results)
				return
			case keyDown:
				m.resCursor = (m.resCursor + 1) % len(m.results)
				return
			case keyEnter:
				m.copyResult(m.resCursor)
			}
		}
		// Anything else returns to the menu without copying.
		m.screen = screenMenu

	case screenBusy:
		if k == keyEsc {
			m.screen = screenMenu
		}

	case screenView:
		if k == keyEsc {
			m.viewBack()
			return
		}
		m.viewer.Key(k, m.cols, m.viewRoom())

	case screenPick:
		lv := &m.picks[len(m.picks)-1]
		page := max(m.pickRoom()-1, 1)
		switch k {
		case keyUp:
			lv.pos--
		case keyDown:
			lv.pos++
		case keyPageUp:
			lv.pos -= page
		case keyPageDown:
			lv.pos += page
		case keyHome:
			lv.pos = 0
		case keyEnd:
			lv.pos = len(lv.items) - 1
		case keyEnter:
			r := lv.items[lv.pos]
			if len(r.Items) > 0 {
				m.picks = append(m.picks, pickLevel{title: r.Name, info: r.Value, items: r.Items})
				return
			}
			var titles []string
			for _, p := range m.picks[1:] {
				titles = append(titles, p.title)
			}
			lang := "JSON"
			if strings.HasPrefix(r.Value, "<") {
				lang = "XML"
			}
			m.viewer.Set(r.Value, lang, !m.tool.PlainView)
			m.viewText = r.Value
			m.viewTitle = strings.Join(append(titles, r.Name), " › ")
			m.screen = screenView
			return
		case keyEsc:
			if m.picks = m.picks[:len(m.picks)-1]; len(m.picks) == 0 {
				m.screen, m.input, m.lastInput, m.results, m.err = screenInput, nil, "", nil, nil
			}
			return
		}
		lv.pos = min(max(lv.pos, 0), len(lv.items)-1)
	}
}

// viewBack leaves the viewer: back to the list it was opened from, or to the prompt.
func (m *model) viewBack() {
	if m.tool.Pick {
		m.screen = screenPick
		return
	}
	m.screen, m.input, m.lastInput, m.results, m.err = screenInput, nil, "", nil, nil
}

// clearStatus removes the "Copied" message shown on the prompt or in a list after the viewer,
// at the next key. In the menu and the results it stays, as before.
func (m *model) clearStatus() {
	if m.screen == screenInput || m.screen == screenPick {
		m.status = nil
	}
}

func (m *model) rune(r rune) {
	m.clearStatus()
	switch m.screen {
	case screenMenu:
		switch {
		case r == '0':
			m.back()
		case r == 'q':
			m.quit()
		case r >= '1' && int(r-'0') <= len(m.items()):
			m.cursor = int(r - '1')
			m.openTool(m.cursor)
		}

	case screenInput:
		m.input = append(m.input, r)

	case screenView:
		if (r == 'c' || r == 'C') && m.tool.ViewCopy {
			if strings.HasPrefix(m.viewText, "// (no") {
				m.status = line{strong("Nothing to copy", amber)}
			} else {
				m.copy(m.viewText)
				m.status = line{strong("✔ Copied", amber)}
			}
			m.viewBack()
		}

	case screenResults:
		switch n := int(r - '0'); {
		case m.tool.CopyAll:
			if (r == 'c' || r == 'C') && m.err == nil {
				m.copyAll()
			}
		case len(m.results) > 1 && n >= 1 && n <= len(m.results):
			m.copyResult(n - 1)
		}
		// Anything other than a valid number returns to the menu without copying.
		m.screen = screenMenu
	}
}

// paste adds text from the clipboard to the input field.
func (m *model) paste(text string) {
	if m.screen == screenInput {
		m.input = append(m.input, []rune(strings.TrimRight(text, "\r\n"))...)
	}
}

func (m *model) canPaste() bool { return m.screen == screenInput }

// ---- Actions ----

func (m *model) openTool(i int) {
	t := m.items()[i]
	m.status = nil
	if t.Menu != nil {
		m.submenus = append(m.submenus, submenu{name: t.Name, items: t.Menu, cursor: i})
		m.cursor = 0
		return
	}
	m.tool = t
	if t.Prompt != "" {
		m.screen = screenInput
		m.input = nil
		m.lastInput, m.results, m.err = "", nil, nil
		m.info = nil
		if t.Info != nil {
			m.info = t.Info()
		}
		return
	}
	m.runTool("")
}

// runTool runs the current tool in the background. A single result is copied right away.
func (m *model) runTool(text string) {
	m.lastInput = text
	m.runID++
	id, tool := m.runID, m.tool
	m.screen = screenBusy
	m.results = nil
	run := tool.Run
	if tool.Stream != nil {
		run = func(text string) ([]Result, error) {
			return tool.Stream(text, func(partial []Result) {
				m.ui(func() {
					if id == m.runID && m.screen == screenBusy {
						m.results = partial
					}
				})
			})
		}
	}
	m.background(func() ([]Result, error) { return run(text) }, func(res []Result, err error) {
		if id != m.runID || m.screen != screenBusy {
			return // cancelled with esc
		}
		m.results, m.err, m.resCursor, m.screen = res, err, 0, screenResults
		if tool.Pick && err == nil && len(res) > 0 {
			m.picks, m.screen = []pickLevel{{items: res}}, screenPick
			return
		}
		if tool.View && err == nil && len(res) > 0 {
			m.screen = screenView
			m.viewText = res[0].Value
			m.viewer.Set(res[0].Value, res[0].Name, !tool.PlainView) // the tools name their result after the language
			m.viewTitle = ""
			return
		}
		if tool.Again {
			m.screen, m.input = screenInput, nil
			return
		}
		if err == nil && !tool.CopyAll && (len(res) == 1 || tool.CopyFirst && len(res) > 0) {
			m.copyResult(0)
		}
	})
}

func (m *model) copyResult(i int) {
	r := m.results[i]
	m.copy(r.Value)
	m.status = line{strong("✔ Copied: "+strings.Join(strings.Fields(r.Name), " "), amber)}
}

// copyAll copies the whole report: tool, input and one aligned line per result.
func (m *model) copyAll() {
	nameWidth := 0
	for _, r := range m.results {
		nameWidth = max(nameWidth, len([]rune(r.Name)))
	}
	text := strings.TrimSpace(m.tool.Name+" "+m.lastInput) + "\n"
	for _, r := range m.results {
		text += fmt.Sprintf("%-*s  %s\n", nameWidth, r.Name, r.Value)
	}
	m.copy(text)
	m.status = line{strong("✔ Copied: "+m.tool.Name+" results", amber)}
}

// ---- Drawing ----

func (m *model) render() []line {
	lines := []line{
		{strong("╭────────────────╮", accent)},
		{strong("│    Pico Tools    │", accent)},
		{strong("╰────────────────╯", accent)},
		nil,
	}
	switch m.screen {
	case screenMenu:
		lines = append(lines, m.renderMenu()...)
	case screenInput:
		lines = append(lines, m.renderInput()...)
	case screenResults:
		lines = append(lines, m.renderResults()...)
	case screenBusy:
		lines = append(lines, m.renderBusy()...)
	case screenView:
		lines = append(lines, m.renderView()...)
	case screenPick:
		lines = append(lines, m.renderPick()...)
	}
	return lines
}

func (m *model) renderMenu() []line {
	var lines []line
	last, escHint := "Exit", "esc exit"
	if len(m.submenus) > 0 {
		names := make([]string, len(m.submenus))
		for i, s := range m.submenus {
			names[i] = s.name
		}
		lines = append(lines, line{strong(strings.Join(names, " › "), nil)}, nil)
		last, escHint = "Back", "esc back"
	}
	items := m.items()
	for i, t := range items {
		name := t.Name
		if t.Menu != nil {
			name += " ›"
		}
		lines = append(lines, entryLine(i == m.cursor, fmt.Sprint(i+1), name))
	}
	lines = append(lines, entryLine(m.cursor == len(items), "0", last), nil)
	if m.status != nil {
		lines = append(lines, m.status, nil)
	}
	return append(lines, line{colored("↑/↓ move · enter select · number quick pick · "+gui.FullScreenKey+" full screen · "+escHint, muted)})
}

func entryLine(selected bool, number, label string) line {
	if selected {
		return line{strong("› "+number+"  "+label, accent)}
	}
	return line{plain("  "), strong(number, accent), plain("  " + label)}
}

func (m *model) renderInput() []line {
	t := m.tool
	// Show only the end of the text if it is longer than the line.
	text := displayText(string(m.input))
	if room := max(m.cols-10, 10); len([]rune(text)) > room {
		r := []rune(text)
		text = "…" + string(r[len(r)-room+1:])
	}
	lines := []line{{strong(t.Name, nil)}, nil}
	for _, l := range m.info {
		lines = append(lines, line{colored(truncate(l, m.cols-2), muted)})
	}
	if len(m.info) > 0 {
		lines = append(lines, nil)
	}
	hint := "enter confirm · esc back · paste with ctrl+v or right click"
	if t.Again {
		if m.status != nil {
			lines = append(lines, m.status, nil)
		}
		lines = append(lines, m.outcome()...)
		hint = "enter check · enter on empty or esc back · paste with ctrl+v or right click"
	}
	return append(lines, []line{
		{plain(t.Prompt + ":")},
		{strong("› ", accent), plain(text), colored("█", accent)},
		nil,
		{colored(hint, muted)},
	}...)
}

// outcome shows the result of the last run of an Again tool, above its prompt.
func (m *model) outcome() []line {
	if m.lastInput == "" {
		return nil
	}
	lines := []line{{colored("Checked: "+truncate(displayText(m.lastInput), m.cols-14), muted)}}
	if m.err != nil {
		lines = append(lines, line{strong("✘ "+capitalize(m.err.Error()), red)})
	}
	for _, r := range m.results {
		lines = append(lines, line{strong(r.Value, accent)})
	}
	return append(lines, nil)
}

// toolHeader is the tool name followed by the text the user typed, if any.
func (m *model) toolHeader() []line {
	lines := []line{{strong(m.tool.Name, nil)}}
	if m.lastInput != "" {
		lines = append(lines, line{colored("Input: "+truncate(displayText(m.lastInput), m.cols-14), muted)})
	}
	return append(lines, nil)
}

func (m *model) renderBusy() []line {
	lines := m.toolHeader()
	// Results shown so far by a tool that streams them.
	nameWidth := 0
	for _, r := range m.results {
		nameWidth = max(nameWidth, len([]rune(r.Name)))
	}
	for _, r := range m.results {
		lines = append(lines, line{plain(fmt.Sprintf("  %-*s   ", nameWidth, r.Name)), colored(truncate(r.Value, m.cols-nameWidth-8), valueFg)})
	}
	if len(m.results) > 0 {
		lines = append(lines, nil)
	}
	return append(lines,
		line{strong("Working…", accent)},
		nil,
		line{colored("esc back to menu", muted)},
	)
}

func (m *model) renderResults() []line {
	lines := m.toolHeader()
	if m.err != nil {
		return append(lines,
			line{strong("✘ "+capitalize(m.err.Error()), red)},
			nil,
			line{colored("press any key to return to the menu", muted)},
		)
	}

	nameWidth := 0
	for _, r := range m.results {
		nameWidth = max(nameWidth, len([]rune(r.Name)))
	}
	// The full value is always copied, even if it is cut on screen.
	valueWidth := m.cols - nameWidth - 14

	many := len(m.results) > 1 && !m.tool.CopyAll
	numWidth := len(fmt.Sprint(len(m.results)))
	var items []line
	for i, r := range m.results {
		name := fmt.Sprintf("%-*s", nameWidth, r.Name)
		num := fmt.Sprintf("%*d", numWidth, i+1)
		var l line
		switch {
		case many && i == m.resCursor:
			l = line{strong("› "+num+"  "+name, accent)}
		case many:
			l = line{plain("  "), strong(num, accent), plain("  " + name)}
		default:
			l = line{plain("  " + name)}
		}
		items = append(items, append(l, plain("   "), colored(truncate(r.Value, valueWidth), valueFg)))
	}

	// When the list is taller than the window, show the part around the selected result
	// (or, for a report, starting from the scroll position).
	if room := m.resultsRoom(); m.rows > 0 && len(items) > room {
		start := min(max(m.resCursor-room/2, 0), len(items)-room)
		if m.tool.CopyAll {
			start = min(m.resCursor, len(items)-room)
		}
		end := start + room
		lines = append(lines, line{colored(moreLine("↑", start), muted)})
		lines = append(lines, items[start:end]...)
		items = []line{{colored(moreLine("↓", len(m.results)-end), muted)}}
	}
	lines = append(lines, items...)

	lines = append(lines, nil)
	if m.tool.CopyAll {
		hint := "c copy all · any other key back to menu"
		if len(m.results) > m.resultsRoom() {
			hint = "↑/↓ scroll · " + hint
		}
		return append(lines, line{colored(hint, muted)})
	}
	if !many {
		return append(lines, m.status, nil, line{colored("press any key to return to the menu", muted)})
	}
	return append(lines, line{colored("↑/↓ move · enter copy · number quick copy · any other key back to menu", muted)})
}

// viewRoom is how many rows of text fit: the window minus the title (4 lines),
// top margin, header and footer.
func (m *model) viewRoom() int {
	return max(m.rows-9, 1)
}

func (m *model) renderView() []line {
	room := m.viewRoom()
	rows, where := m.viewer.Render(m.cols, room)
	header := line{strong(m.tool.Name, nil)}
	if m.viewTitle != "" {
		header = line{strong(strings.Join(strings.Fields(m.viewTitle), " "), nil)}
	}
	if !m.tool.PlainView {
		header = append(header, colored(fmt.Sprintf("   %d lines", m.viewer.Len()), muted))
	}
	lines := []line{header, nil}
	lines = append(lines, rows...)
	hint := "↑/↓ scroll · pgup/pgdn page · home/end start/end · esc back · "
	if m.tool.ViewCopy {
		hint = "↑/↓ scroll · pgup/pgdn page · home/end start/end · c copy · esc back · "
	}
	return append(lines, nil, line{colored(hint+where, muted)})
}

// pickRoom is how many entries of screenPick fit, like viewRoom, less the info line if any.
func (m *model) pickRoom() int {
	room := m.rows - 9
	if len(m.picks) > 0 && m.picks[len(m.picks)-1].info != "" {
		room -= 2
	}
	if m.status != nil {
		room--
	}
	return max(room, 1)
}

func (m *model) renderPick() []line {
	lv := m.picks[len(m.picks)-1]
	room := m.pickRoom()
	start := min(max(lv.pos-room/2, 0), max(len(lv.items)-room, 0))
	end := min(start+room, len(lv.items))

	title := m.tool.Name
	if len(m.picks) > 1 {
		title = strings.Join(strings.Fields(lv.title), " ")
	}
	lines := []line{{strong(title, nil), colored(fmt.Sprintf("   %d entries", len(lv.items)), muted)}, nil}
	if lv.info != "" {
		lines = append(lines, line{colored(truncate(lv.info, m.cols-2), muted)}, nil)
	}
	for i := start; i < end; i++ {
		name := truncate(lv.items[i].Name, m.cols-4)
		if i == lv.pos {
			lines = append(lines, line{strong("› "+name, accent)})
		} else {
			lines = append(lines, line{plain("  " + name)})
		}
	}
	for i := end - start; i < room; i++ {
		lines = append(lines, nil)
	}
	lines = append(lines, nil)
	if m.status != nil {
		lines = append(lines, m.status)
	}
	return append(lines, line{colored("↑/↓ move · pgup/pgdn page · enter open · esc back", muted)})
}

// resultsRoom is how many results fit in the window: what is left after the title (4 lines),
// top margin, tool header, the two "more" lines and the footer.
func (m *model) resultsRoom() int {
	return max(m.rows-9-len(m.toolHeader()), 1)
}

func moreLine(arrow string, n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("  %s %d more", arrow, n)
}

var (
	displayText = gui.DisplayText
	truncate    = gui.Truncate
)

func capitalize(s string) string {
	r := []rune(s)
	if len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
	}
	return string(r)
}
