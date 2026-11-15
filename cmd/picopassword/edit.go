package main

import (
	"slices"
	"strings"

	"picotools/internal/gui"
)

// Edit and New (docs/picopassword.md): one page with Name, User, Password and Save. ↑↓ move between
// the rows, enter on a field goes to the next row, enter on Save saves everything, esc
// cancels everything. The password is never shown, only its length in dots: typing or
// pasting on it replaces the old one, and only then a Repeat row appears. On the Password
// row ← suggests a strong password and → a stronger one: a suggested password is shown in
// clear, and Repeat is filled with it.

const (
	rowName = iota
	rowUser
	rowPassword
	rowRepeat // only when the password is changed
	rowSave
)

type editPage struct {
	item      *Item     // nil for a new item
	fields    [4][]rune // name, user, new password, repeat
	pos       [4]int    // the cursor in each field
	changed   bool      // the password was replaced
	suggested bool      // the new password is a suggested one, shown in clear
	row       int
	err       string
}

// openNew opens the page empty, for a new item: the password is new, so Repeat is there.
func (m *model) openNew() {
	m.edit, m.revealed, m.screen = &editPage{changed: true}, "", screenEdit
}

func (m *model) openEdit(it *Item) {
	e := &editPage{item: it}
	e.fields[rowName], e.fields[rowUser] = []rune(it.Name), []rune(it.User)
	e.pos[rowName], e.pos[rowUser] = len(e.fields[rowName]), len(e.fields[rowUser])
	m.edit, m.revealed, m.screen = e, "", screenEdit
}

// rows are the rows on screen, in order.
func (e *editPage) rows() []int {
	if e.changed {
		return []int{rowName, rowUser, rowPassword, rowRepeat, rowSave}
	}
	return []int{rowName, rowUser, rowPassword, rowSave}
}

func (e *editPage) step(k string) {
	rows := e.rows()
	e.row = rows[move(slices.Index(rows, e.row), k, len(rows))]
}

func (m *model) editKey(k string) {
	e := m.edit
	switch k {
	case gui.KeyUp, gui.KeyDown:
		e.step(k)
	case gui.KeyEsc:
		m.edit, m.screen = nil, screenItem
		if e.item == nil {
			m.screen = screenList
		}
	case gui.KeyEnter:
		if e.row == rowSave {
			m.saveEdit()
		} else {
			e.step(gui.KeyDown)
		}
	case gui.KeyLeft, gui.KeyRight:
		switch e.row {
		case rowSave:
		case rowPassword:
			pw := []rune(m.suggest(k == gui.KeyRight))
			e.fields[rowPassword], e.fields[rowRepeat] = pw, slices.Clone(pw)
			e.pos[rowPassword], e.pos[rowRepeat] = len(pw), len(pw)
			e.changed, e.suggested, e.err = true, true, ""
		default:
			if k == gui.KeyLeft {
				e.pos[e.row] = max(e.pos[e.row]-1, 0)
			} else {
				e.pos[e.row] = min(e.pos[e.row]+1, len(e.fields[e.row]))
			}
		}
	case gui.KeyBackspace:
		if e.row == rowSave {
			return
		}
		e.err = ""
		if e.row == rowPassword && (!e.changed || e.suggested) {
			e.clearPassword() // the old or the suggested password goes away
			return
		}
		if p := e.pos[e.row]; p > 0 {
			e.fields[e.row] = append(e.fields[e.row][:p-1], e.fields[e.row][p:]...)
			e.pos[e.row]--
		}
	}
}

func (m *model) editInsert(rs []rune) {
	e := m.edit
	if e.row == rowSave {
		return
	}
	e.err = ""
	if e.row == rowPassword && (!e.changed || e.suggested) {
		e.clearPassword() // typing replaces the old or the suggested password
	}
	f, p := e.fields[e.row], e.pos[e.row]
	e.fields[e.row] = append(f[:p:p], append(rs, f[p:]...)...)
	e.pos[e.row] += len(rs)
}

// clearPassword starts a new password typed by hand: empty, in dots, Repeat to type.
func (e *editPage) clearPassword() {
	e.fields[rowPassword], e.fields[rowRepeat] = nil, nil
	e.pos[rowPassword], e.pos[rowRepeat] = 0, 0
	e.changed, e.suggested = true, false
}

func (m *model) saveEdit() {
	e, it := m.edit, m.edit.item
	name := strings.TrimSpace(string(e.fields[rowName]))
	user := strings.TrimSpace(string(e.fields[rowUser]))
	pw, repeat := string(e.fields[rowPassword]), string(e.fields[rowRepeat])
	switch {
	case name == "":
		e.row, e.err = rowName, "The name cannot be empty"
		return
	case user == "":
		e.row, e.err = rowUser, "The user cannot be empty"
		return
	case e.changed && pw == "":
		e.row, e.err = rowPassword, "The password cannot be empty"
		return
	case e.changed && pw != repeat:
		e.fields[rowRepeat], e.pos[rowRepeat] = nil, 0
		e.row, e.err = rowRepeat, "The two passwords are different"
		return
	}
	if it == nil {
		m.addItem(name, user, pw)
		return
	}
	old := *it
	old.History = slices.Clone(it.History)
	it.Name, it.User = name, user
	if e.changed && pw != it.Password {
		it.History = append([]OldPassword{{Password: it.Password, Replaced: m.now()}}, it.History...)
		if len(it.History) > maxHistory {
			it.History = it.History[:maxHistory]
		}
		it.Password = pw
	}
	order := slices.Clone(m.vault.Items)
	sortItems(m.vault.Items)
	if err := m.vault.save(); err != nil {
		*it = old
		m.vault.Items = order
		e.err = "Could not save: " + err.Error()
		return
	}
	m.edit, m.screen = nil, screenItem
	m.say(it.Name + " saved")
}

// addItem saves the new item of the page and goes back to the list, on it.
func (m *model) addItem(name, user, pw string) {
	it := &Item{ID: newID(), Name: name, User: user, Password: pw}
	before := m.vault.Items
	m.vault.Items = append(slices.Clone(before), it)
	sortItems(m.vault.Items)
	if err := m.vault.save(); err != nil {
		m.vault.Items = before
		m.edit.err = "Could not save: " + err.Error()
		return
	}
	m.edit, m.screen = nil, screenList
	m.favOnly = false // a new item is not a favourite: show it
	m.selectItem(it)
	m.say(it.Name + " added")
}

func (m *model) renderEdit() []line {
	e := m.edit
	title := "New password"
	if e.item != nil {
		title = "Edit " + e.item.Name
	}
	lines := []line{{gui.Strong(title, nil)}, nil}
	labels := []string{"Name", "User", "Password", "Repeat"}
	for _, r := range e.rows() {
		if r == rowSave {
			if e.row == rowSave {
				lines = append(lines, line{gui.Strong("› Save", gui.Accent)}.Highlighted(m.cols-2))
			} else {
				lines = append(lines, line{gui.Plain("  Save")})
			}
			continue
		}
		text := []rune(gui.DisplayText(string(e.fields[r])))
		switch {
		case r == rowPassword && !e.changed:
			text = []rune(strings.Repeat("•", len([]rune(e.item.Password))))
		case (r == rowPassword || r == rowRepeat) && !e.suggested:
			text = []rune(strings.Repeat("•", len(e.fields[r])))
		}
		label := gui.Colored(padRight(labels[r], 11), gui.Muted)
		if r != e.row {
			lines = append(lines, line{gui.Plain("  "), label, gui.Plain(string(text))})
			continue
		}
		pos := min(e.pos[r], len(text))
		if r == rowPassword && !e.changed {
			pos = len(text)
		}
		under := " "
		if pos < len(text) {
			under = string(text[pos])
		}
		l := line{gui.Strong("› ", gui.Accent), label, gui.Plain(string(text[:pos])), {Text: under, Color: gui.Background, Bg: gui.Accent}}
		if pos+1 < len(text) {
			l = append(l, gui.Plain(string(text[pos+1:])))
		}
		lines = append(lines, l)
	}
	lines = append(lines, nil)
	if e.err != "" {
		lines = append(lines, line{gui.Strong("✘ "+e.err, gui.Red)}, nil)
	}
	hint := "↑↓ move · enter next · esc back"
	switch e.row {
	case rowPassword:
		hint = "↑↓ move · ← suggest strong · → suggest stronger · enter next · esc back"
	case rowSave:
		hint = "↑↓ move · enter save · esc back"
	}
	return append(lines, line{gui.Colored(hint, gui.Muted)})
}
