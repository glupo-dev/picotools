package main

import (
	"fmt"

	"picotools/internal/gui"
)

type line = gui.Line

func (m *model) Render(cols, rows int) []line {
	m.cols, m.rows = cols, rows
	lines := []line{
		{gui.Strong("╭─────────────────╮", gui.Accent)},
		{gui.Strong("│  Pico Password  │", gui.Accent)},
		{gui.Strong("╰─────────────────╯", gui.Accent)},
		nil,
	}
	switch m.screen {
	case screenStart:
		lines = append(lines, m.renderStart()...)
	case screenList:
		lines = append(lines, m.renderList()...)
	case screenItem:
		lines = append(lines, m.renderItem()...)
	case screenHistory:
		lines = append(lines, m.renderHistory()...)
	case screenDelete:
		lines = append(lines, m.renderDelete()...)
	case screenSettings:
		lines = append(lines, m.renderSettings()...)
	case screenShowAs:
		lines = append(lines, m.renderShowAs()...)
	case screenSuggest:
		lines = append(lines, m.renderSuggest()...)
	case screenEdit:
		lines = append(lines, m.renderEdit()...)
	case screenForm:
		lines = append(lines, m.form.render(cols)...)
	case screenSync:
		lines = append(lines, m.renderSync()...)
	case screenApprove:
		lines = append(lines, m.renderApprove()...)
	case screenComputers:
		lines = append(lines, m.renderComputers()...)
	case screenFatal:
		if m.fatal != "" {
			lines = append(lines, line{gui.Strong("✘ "+m.fatal, gui.Red)}, nil)
		}
		lines = append(lines, line{gui.Colored("esc quit", gui.Muted)})
	}
	return lines
}

// footer is the status, if any, and the hint, as in Pico DB.
func (m *model) footer(hint string) []line {
	lines := []line{nil}
	if m.status != nil {
		lines = append(lines, m.status, nil)
	}
	return append(lines, line{gui.Colored(hint, gui.Muted)})
}

// entry is one row of a menu: its key, its label, the selected one highlighted.
func (m *model) entry(key, label string, selected bool) line {
	if selected {
		return line{gui.Strong("› "+key+"  "+label, gui.Accent)}.Highlighted(m.cols - 2)
	}
	return line{gui.Plain("  "), gui.Strong(key, gui.Accent), gui.Plain("  " + label)}
}

func (m *model) renderStart() []line {
	if m.startPage == 1 {
		return append([]line{
			{gui.Strong("Open an existing vault", nil)},
			nil,
			{gui.Plain("  "), gui.Strong("1", gui.Accent), gui.Plain("  From GitHub")},
			{gui.Colored("     The vault is on GitHub and another device of yours is already linked to it.", gui.Muted)},
			nil,
			{gui.Plain("  "), gui.Strong("2", gui.Accent), gui.Plain("  From glupo.dev")},
			{gui.Colored("     The vault is on glupo.dev and another device of yours is already linked to it.", gui.Muted)},
		}, m.footer("number choose · esc back")...)
	}
	return append([]line{
		{gui.Strong("Welcome", nil)},
		{gui.Colored("The passwords are kept in a file encrypted with a master password of your choice.", gui.Muted)},
		{gui.Colored("This device's keychain remembers it, so you type it only here and on a new device.", gui.Muted)},
		{gui.Colored("If you lose it, the vault cannot be opened by anyone: write it down somewhere safe.", gui.Muted)},
		nil,
		{gui.Plain("  "), gui.Strong("1", gui.Accent), gui.Plain("  New vault")},
		{gui.Plain("  "), gui.Strong("2", gui.Accent), gui.Plain("  Open an existing vault")},
	}, m.footer("number choose · esc quit")...)
}

// listRoom is how many items fit, exactly as the lists of Pico DB: the window less the logo
// (4 lines), the title and the blank under it, the status and the hint; at most 50.
func (m *model) listRoom() int {
	room := m.rows - 9
	if m.status != nil {
		room -= 2
	}
	return min(max(room, 1), 50)
}

func (m *model) renderList() []line {
	all, items := m.vault.Items, m.shown()
	info, toggle := "   "+count(len(all), "item"), "f only favourites"
	if m.favOnly {
		info, toggle = fmt.Sprintf("   %d favourites of %s", len(items), count(len(all), "item")), "f all items"
	}
	lines := []line{{gui.Strong("Passwords", nil), gui.Colored(info, gui.Muted)}, nil}
	switch {
	case len(all) == 0:
		lines = append(lines, line{gui.Colored("No passwords yet: press n to add the first one.", gui.Muted)})
	case len(items) == 0:
		lines = append(lines, line{gui.Colored("No favourites yet: open an item with o and press f to add it.", gui.Muted)})
	}

	nameW, userW := 0, 0
	for _, it := range items {
		nameW = max(nameW, len([]rune(it.Name)))
		userW = max(userW, len([]rune(it.User)))
	}
	nameW, userW = min(nameW, 30), min(userW, 40)
	numW := len(fmt.Sprint(len(items)))

	room := m.listRoom()
	start := m.cursor / room * room
	end := min(start+room, len(items))
	for i := start; i < end; i++ {
		it := items[i]
		num := fmt.Sprintf("%*d", numW, i+1)
		gap := "  "
		if it.Favourite && !m.favOnly {
			gap = "* "
		}
		name := fmt.Sprintf("%-*s", nameW, gui.Truncate(it.Name, nameW))
		user := fmt.Sprintf("%-*s", userW, gui.Truncate(it.User, userW))
		pw := gui.Colored(m.mask(it.Password), gui.Muted)
		if it.ID == m.revealed {
			pw = gui.Strong(gui.DisplayText(it.Password), gui.ValueFg)
		}
		if i == m.cursor {
			lines = append(lines, line{gui.Strong("› "+num+gap+name, gui.Accent), gui.Colored("   "+user+"   ", gui.Muted), pw}.Highlighted(m.cols-2))
		} else {
			lines = append(lines, line{gui.Plain("  "), gui.Strong(num+gap[:1], gui.Accent), gui.Plain(gap[1:] + name), gui.Colored("   "+user+"   ", gui.Muted), pw})
		}
	}

	hint := "n new · s settings · esc quit"
	switch {
	case len(items) > 0:
		hint = "↑↓ move · ←→ page · enter copy password · u copy user · r reveal · o open · n new · " + toggle + " · s settings · esc quit"
	case len(all) > 0:
		hint = "n new · " + toggle + " · s settings · esc quit"
	}
	return append(lines, m.footer(hint)...)
}

func (m *model) renderItem() []line {
	it := m.current()
	if it == nil {
		return m.footer("esc back")
	}
	fav := "no"
	favAction := "Add to favourites"
	if it.Favourite {
		fav, favAction = "yes", "Remove from favourites"
	}
	field := func(label, value string) line {
		return line{gui.Colored("  "+padRight(label, 12), gui.Muted), gui.Plain(value)}
	}
	pwField := field("Password", m.mask(it.Password))
	if m.revealed == it.ID {
		pwField = line{gui.Colored("  "+padRight("Password", 12), gui.Muted), gui.Strong(gui.DisplayText(it.Password), gui.ValueFg)}
	}
	lines := []line{
		{gui.Strong(it.Name, nil)},
		nil,
		field("User", it.User),
		pwField,
		field("Favourite", fav),
		nil,
	}
	for i, label := range []string{"Edit", favAction, "History", "Delete"} {
		key := string(itemActions[i])
		if key != "h" && m.changeOff() && i != m.itemCursor {
			// No connection, or a change being saved: the changes are grey (docs 4.4).
			lines = append(lines, line{gui.Colored("  "+key+"  "+label, gui.Muted)})
			continue
		}
		lines = append(lines, m.entry(key, label, i == m.itemCursor))
	}
	return append(lines, m.footer("↑↓ move · enter choose · r reveal · e edit · f favourite · h history · d delete · esc back")...)
}

func (m *model) renderHistory() []line {
	it := m.current()
	if it == nil {
		return m.footer("esc back")
	}
	lines := []line{{gui.Strong(it.Name+" › History", nil)}, nil}
	if len(it.History) == 0 {
		lines = append(lines, line{gui.Colored("No old passwords: they appear here when the password is changed in Edit.", gui.Muted)})
		return append(lines, m.footer("esc back")...)
	}
	for i, old := range it.History {
		pw := gui.Colored(m.mask(old.Password), gui.Muted)
		if m.revealed == historyID(i) {
			pw = gui.Strong(gui.DisplayText(old.Password), gui.ValueFg)
		}
		date := old.Replaced.Local().Format("02 Jan 2006")
		if i == m.histCursor {
			lines = append(lines, line{gui.Strong("› "+date+"   ", gui.Accent), pw}.Highlighted(m.cols-2))
		} else {
			lines = append(lines, line{gui.Plain("  " + date + "   "), pw})
		}
	}
	return append(lines, m.footer("↑↓ move · enter copy password · r reveal · esc back")...)
}

func (m *model) renderDelete() []line {
	it := m.current()
	if it == nil {
		return m.footer("esc back")
	}
	return []line{
		{gui.Strong("Delete "+it.Name+"?", gui.Amber)},
		nil,
		{gui.Strong("› ", gui.Accent), gui.Plain(string(m.delInput)), gui.Colored("█", gui.Accent)},
		nil,
		{gui.Colored("type yes and enter to delete · anything else no", gui.Muted)},
	}
}

func (m *model) renderSettings() []line {
	entry := func(key, label string, value string, note string) []line {
		l := line{gui.Plain("  "), gui.Strong(key, gui.Accent), gui.Plain("  " + label)}
		if value != "" {
			l = append(l, gui.Plain(":  "), gui.Colored(value, gui.ValueFg))
		}
		return []line{l, {gui.Colored("     "+note, gui.Muted)}, nil}
	}
	lines := []line{{gui.Strong("Settings", nil)}, nil}
	if m.link != nil {
		lines = append(lines, m.syncedLine(), nil)
	}
	lines = append(lines, entry("1", "Show passwords as", showChoices[m.showChoice()].label,
		"How the list hides the passwords, on this device only.")...)
	lines = append(lines, entry("2", "Suggested passwords",
		fmt.Sprintf("strong %d · stronger %d", m.genSpec(false).Length, m.genSpec(true).Length),
		"How long and with which characters the passwords suggested with ← and → are.")...)
	if m.link != nil {
		lines = append(lines, entry("3", "Linked devices", "", "The devices that have this vault.")...)
		lines = append(lines, entry("4", "Reset this device", "", "The vault stays on "+m.where()+".")...)
	} else {
		lines = append(lines, entry("3", "Sync with GitHub", "",
			"Put this vault in a private GitHub repository of yours, to have it on your other devices.")...)
		lines = append(lines, entry("4", "Sync with glupo.dev", "",
			"Put this vault on glupo.dev with an invite code, to have it on your other devices.")...)
		lines = append(lines, entry("5", "Delete this vault", "",
			"Deletes the vault and its passwords from this device, for ever.")...)
	}
	return append(lines[:len(lines)-1], m.footer("number choose · esc back")...)
}

func (m *model) renderShowAs() []line {
	lines := []line{{gui.Strong("Show passwords as", nil)}, nil}
	current := m.showChoice()
	for i, c := range showChoices {
		label := c.label
		if i == current {
			label += "  (now)"
		}
		lines = append(lines, m.entry(fmt.Sprint(i+1), label, i == m.showCursor),
			line{gui.Colored("     "+c.example+"   ("+c.note+")", gui.Muted)}, nil)
	}
	return append(lines, m.footer("↑↓ move · enter choose · number choose · esc back")...)
}

func (m *model) renderSuggest() []line {
	st, sr := m.genSpec(false), m.genSpec(true)
	row := func(label, a, b string) line {
		return line{gui.Colored("  "+padRight(label, 17), gui.Muted), gui.Plain(padRight(a, 11) + b)}
	}
	return append([]line{
		{gui.Strong("Suggested passwords", nil)},
		nil,
		{gui.Colored("  "+padRight("", 17)+padRight("Strong", 11)+"Stronger", gui.Muted)},
		row("Length", fmt.Sprint(st.Length), fmt.Sprint(sr.Length)),
		row("Digits", yesNo(st.Digits), yesNo(sr.Digits)),
		row("Symbols", yesNo(st.Symbols), yesNo(sr.Symbols)),
		nil,
		{gui.Colored("  Strong     ←   ", gui.Muted), gui.Plain(gui.DisplayText(m.examples[0]))},
		{gui.Colored("  Stronger   →   ", gui.Muted), gui.Plain(gui.DisplayText(m.examples[1]))},
	}, m.footer("1 strong · 2 stronger · esc back")...)
}

func count(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}
