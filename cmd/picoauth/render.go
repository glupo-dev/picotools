package main

import (
	"fmt"

	"picotools/internal/gui"
)

type line = gui.Line

func (m *model) Render(cols, rows int) []line {
	m.cols, m.rows = cols, rows
	lines := []line{
		{gui.Strong("╭──────────────────╮", gui.Accent)},
		{gui.Strong("│    Pico Auth     │", gui.Accent)},
		{gui.Strong("╰──────────────────╯", gui.Accent)},
		nil,
	}
	switch m.screen {
	case screenStart:
		lines = append(lines, m.renderStart()...)
	case screenList:
		lines = append(lines, m.renderList()...)
	case screenForm:
		lines = append(lines, m.form.render(cols)...)
	case screenConfirm:
		lines = append(lines, m.confirm...)
		lines = append(lines, nil, line{gui.Colored("y yes · any other key no", gui.Muted)})
	case screenPreview:
		lines = append(lines, m.renderPreview()...)
	case screenSettings:
		lines = append(lines, m.renderSettings()...)
	case screenSync:
		lines = append(lines, m.renderSync()...)
	case screenApprove:
		lines = append(lines, m.renderApprove()...)
	case screenFatal:
		if m.fatal != "" {
			lines = append(lines, line{gui.Strong("✘ "+m.fatal, gui.Red)}, nil)
		}
		lines = append(lines, line{gui.Colored("esc quit", gui.Muted)})
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

func (m *model) renderStart() []line {
	if m.startPage == 1 {
		return append([]line{
			{gui.Strong("Open an existing vault", nil)},
			nil,
			{gui.Plain("  "), gui.Strong("1", gui.Accent), gui.Plain("  From GitHub")},
			{gui.Colored("     The vault is on GitHub and another computer of yours is already linked to it.", gui.Muted)},
		}, m.footer("number choose · esc back")...)
	}
	return append([]line{
		{gui.Strong("Welcome", nil)},
		{gui.Colored("The accounts are kept in a file encrypted with a master password of your choice.", gui.Muted)},
		{gui.Colored("This computer's keychain remembers it, so you type it only here and on a new computer.", gui.Muted)},
		{gui.Colored("If you lose it, the vault cannot be opened by anyone: write it down somewhere safe.", gui.Muted)},
		nil,
		{gui.Plain("  "), gui.Strong("1", gui.Accent), gui.Plain("  New vault")},
		{gui.Plain("  "), gui.Strong("2", gui.Accent), gui.Plain("  Open an existing vault")},
	}, m.footer("number choose · esc quit")...)
}

// listRoom is how many accounts fit: the window less the title (4 lines),
// top margin, the "new" entry, the status and the hint.
func (m *model) listRoom() int {
	return max(m.rows-10, 1)
}

func (m *model) renderList() []line {
	as := m.vault.Accounts
	var lines []line
	if len(as) == 0 {
		lines = append(lines,
			line{gui.Colored("No accounts yet: press n and paste the secret key that the service shows", gui.Muted)},
			line{gui.Colored("when you turn on two-factor authentication, next to its QR code.", gui.Muted)})
	}

	nameW, userW := 0, 0
	for _, a := range as {
		nameW = max(nameW, len([]rune(a.Name)))
		userW = max(userW, len([]rune(a.User)))
	}
	nameW, userW = min(nameW, 30), min(userW, 40)
	numW := len(fmt.Sprint(len(as)))

	room := m.listRoom()
	start := min(max(m.cursor-room/2, 0), max(len(as)-room, 0))
	end := min(start+room, len(as))
	if start > 0 {
		lines = append(lines, line{gui.Colored(fmt.Sprintf("  ↑ %d more", start), gui.Muted)})
	}
	now := m.now()
	for i := start; i < end; i++ {
		a := as[i]
		code, left := a.code(now)
		num := fmt.Sprintf("%*d", numW, i+1)
		name := fmt.Sprintf("%-*s", nameW, gui.Truncate(a.Name, nameW))
		user := fmt.Sprintf("%-*s", userW, gui.Truncate(a.User, userW))
		codeColor := gui.ValueFg
		if left <= 5 {
			codeColor = gui.Amber
		}
		rest := line{
			gui.Colored("   "+user+"   ", gui.Muted),
			gui.Strong(spaced(code), codeColor),
			gui.Colored(fmt.Sprintf("   %2ds", left), gui.Muted),
		}
		if i == m.cursor {
			lines = append(lines, append(line{gui.Strong("› "+num+"  "+name, gui.Accent)}, rest...))
		} else {
			lines = append(lines, append(line{gui.Plain("  "), gui.Strong(num, gui.Accent), gui.Plain("  " + name)}, rest...))
		}
	}
	if end < len(as) {
		lines = append(lines, line{gui.Colored(fmt.Sprintf("  ↓ %d more", len(as)-end), gui.Muted)})
	}
	lines = append(lines, nil, line{gui.Plain("  "), gui.Strong("n", gui.Accent), gui.Plain("  new account")})
	hint := "n new · s settings · esc quit"
	if len(as) > 0 {
		hint = "↑/↓ move · enter copy the code · number quick copy · n new · m edit · d delete · s settings · " + gui.FullScreenKey + " full screen · esc quit"
	}
	return append(lines, m.footer(hint)...)
}

func (m *model) renderPreview() []line {
	a := m.pending
	code, left := a.code(m.now())
	lines := []line{
		{gui.Strong("New account › "+label(a), nil)},
		nil,
		{gui.Plain("Code now:  "), gui.Strong(spaced(code), gui.ValueFg), gui.Colored(fmt.Sprintf("   %ds", left), gui.Muted)},
	}
	if a.Algorithm != defaultAlgorithm || a.Digits != defaultDigits || a.Period != defaultPeriod {
		lines = append(lines, line{gui.Colored(fmt.Sprintf("%s · %d digits · every %d seconds", a.Algorithm, a.Digits, a.Period), gui.Muted)})
	}
	return append(lines,
		nil,
		line{gui.Strong("Is it the same code as on the phone, or does the service accept it?", gui.Amber)},
		nil,
		line{gui.Colored("y save · any other key cancel", gui.Muted)},
	)
}
