package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"picotools/internal/gui"
)

// Settings (docs/picopassword.md, section 4.6): how passwords are shown, a choice of this
// device only, kept in a small file next to the vault, not in the vault and not synced; and
// Delete this vault, as in Pico Auth.

// prefs are the choices of this device.
type prefs struct {
	ShowPasswords string   `json:"show_passwords,omitempty"` // a key of showChoices; "" the default
	Strong        *genSpec `json:"strong,omitempty"`         // nil the default
	Stronger      *genSpec `json:"stronger,omitempty"`
}

type showChoice struct {
	key, label, example, note string
}

var showChoices = []showChoice{
	{"8dots", "8 dots", "••••••••", "always 8, whatever the length"},
	{"dots", "One dot per character", "••••••••••••", "a 12-character password"},
	{"first2", "First 2 characters", "Ma••••••••••", "the first 2 in clear, then a dot per character"},
	{"firstlast", "First and last character", "M••••••••••!", "first and last in clear, a dot per character in between"},
}

const defaultShow = "dots"

func prefsPath(vaultPath string) string { return filepath.Join(filepath.Dir(vaultPath), "prefs.json") }

func loadPrefs(vaultPath string) prefs {
	var p prefs
	if raw, err := os.ReadFile(prefsPath(vaultPath)); err == nil {
		json.Unmarshal(raw, &p)
	}
	return p
}

func (m *model) savePrefs() error {
	raw, err := json.MarshalIndent(m.prefs, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(prefsPath(m.path), append(raw, '\n'))
}

func (m *model) showChoice() int {
	for i, c := range showChoices {
		if c.key == m.prefs.ShowPasswords {
			return i
		}
	}
	for i, c := range showChoices {
		if c.key == defaultShow {
			return i
		}
	}
	return 0
}

// mask is a password as the list shows it, by the choice of this device.
func (m *model) mask(pw string) string {
	r := []rune(pw)
	dots := func(n int) string { return strings.Repeat("•", max(n, 0)) }
	switch showChoices[m.showChoice()].key {
	case "8dots":
		return dots(8)
	case "first2":
		if len(r) > 2 {
			return string(r[:2]) + dots(len(r)-2)
		}
	case "firstlast":
		if len(r) > 2 {
			return string(r[:1]) + dots(len(r)-2) + string(r[len(r)-1:])
		}
	}
	return dots(len(r))
}

func (m *model) settingsRune(r rune) {
	switch r {
	case '1':
		m.showCursor, m.screen = m.showChoice(), screenShowAs
	case '2':
		m.newExamples()
		m.screen = screenSuggest
	case '3':
		m.deleteVaultForm()
	}
}

// ---- Suggested passwords ----

func (m *model) newExamples() {
	m.examples = [2]string{m.suggest(false), m.suggest(true)}
}

func (m *model) suggestRune(r rune) {
	switch r {
	case '1':
		m.genForm(false)
	case '2':
		m.genForm(true)
	}
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func checkYesNo(v string, _ []string) string {
	if v = strings.ToLower(v); v != "yes" && v != "no" {
		return "Type yes or no"
	}
	return ""
}

// genForm asks length, digits and symbols of the strong or the stronger passwords.
func (m *model) genForm(stronger bool) {
	s, name := m.genSpec(false), "Strong"
	if stronger {
		s, name = m.genSpec(true), "Stronger"
	}
	m.ask(name+" passwords", []question{
		{label: "Length", value: strconv.Itoa(s.Length), check: func(v string, _ []string) string {
			if n, err := strconv.Atoi(v); err != nil || n < minGenLen || n > maxGenLen {
				return fmt.Sprintf("Type a number from %d to %d", minGenLen, maxGenLen)
			}
			return ""
		}},
		{label: "Digits", value: yesNo(s.Digits), check: checkYesNo},
		{label: "Symbols", value: yesNo(s.Symbols), check: checkYesNo},
	}, func(a []string) string {
		n, _ := strconv.Atoi(a[0])
		spec := &genSpec{Length: n, Digits: strings.ToLower(a[1]) == "yes", Symbols: strings.ToLower(a[2]) == "yes"}
		old := m.prefs
		if stronger {
			m.prefs.Stronger = spec
		} else {
			m.prefs.Strong = spec
		}
		if err := m.savePrefs(); err != nil {
			m.prefs = old
			return "Could not save the choice: " + err.Error()
		}
		m.newExamples()
		m.say(name + " passwords saved")
		return ""
	})
}

func (m *model) showAsKey(k string) {
	switch k {
	case gui.KeyUp, gui.KeyDown:
		m.showCursor = move(m.showCursor, k, len(showChoices))
	case gui.KeyEnter:
		m.chooseShow(m.showCursor)
	case gui.KeyEsc:
		m.screen = screenSettings
	}
}

func (m *model) chooseShow(i int) {
	old := m.prefs
	m.prefs.ShowPasswords = showChoices[i].key
	if err := m.savePrefs(); err != nil {
		m.prefs = old
		m.warn("Could not save the choice: " + err.Error())
		return
	}
	m.screen = screenSettings
	m.say("Passwords shown as: " + showChoices[i].label)
}

// deleteVaultForm asks to type delete and the master password, as in Pico Auth, then removes
// from this device the vault, its preferences and the password in the keychain.
func (m *model) deleteVaultForm() {
	n := len(m.vault.Items)
	what := "1 item"
	if n != 1 {
		what = strings.TrimSpace(count(n, "item"))
	}
	m.ask("Delete this vault", []question{
		{label: "This deletes the vault and its " + what + " from this device, for ever. Type delete to confirm", check: func(v string, _ []string) string {
			if v != "delete" {
				return "Type delete, or esc to go back"
			}
			return ""
		}},
		{label: "Master password", secret: true},
	}, func(a []string) string {
		if subtle.ConstantTimeCompare([]byte(a[1]), []byte(m.vault.password)) != 1 {
			return "Wrong password"
		}
		for _, p := range []string{m.path, prefsPath(m.path)} {
			if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return "Could not delete: " + err.Error()
			}
		}
		deletePassword()
		m.vault, m.prefs, m.cursor, m.favOnly, m.screen = nil, prefs{}, 0, false, screenStart
		m.say("Vault deleted")
		return ""
	})
}
