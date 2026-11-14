package main

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"picotools/internal/gui"
)

type screen int

const (
	screenStart    screen = iota // first start: new vault
	screenList                   // the accounts with their codes
	screenForm                   // questions, one after the other
	screenConfirm                // a yes/no question
	screenPreview                // a new account with its code, before saving it
	screenSettings               // the rare operations on the whole vault
	screenSync                   // linking the vault to GitHub, while it works
	screenApprove                // a new computer asks for access
	screenFatal                  // something went wrong at start: only esc
)

// How long a copied code stays in the clipboard.
const clipboardLife = 30 * time.Second

// model holds the state of Pico Auth and reacts to keys.
// It knows nothing about the window: the clipboard and quitting go through callbacks.
type model struct {
	vault *Vault
	path  string     // the vault file
	link  *linkState // the GitHub link of this computer, nil if not linked
	gh    *github
	sync  *syncView

	startPage  int // first start: 0 the main menu, 1 open an existing vault
	checkEvery time.Duration
	lastCheck  time.Time
	checking   bool            // reading the requests of new computers
	handled    map[string]bool // requests already approved or refused
	approving  *pendingRequest // the request on screen
	background context.Context // for the checks that run while Pico Auth is open
	screen     screen

	cursor int // selected account

	form    *form
	confirm []line // the yes/no question
	yes     func() // run on "y"
	after   screen // where a question goes back to
	pending *Account
	fatal   string

	copied   string // the code last copied, to clear it from the clipboard
	copiedAt time.Time

	status gui.Line // message shown at the bottom, cleared by the next key
	cols   int
	rows   int

	copy      func(text string)
	clipboard func() string
	quit      func()
	now       func() time.Time
	ui        func(func()) // runs a function in the user interface, from any goroutine
	openURL   func(string)
}

func newModel(path string) *model {
	return &model{
		path:       path,
		cols:       gui.DefaultCols,
		copy:       func(string) {},
		clipboard:  func() string { return "" },
		quit:       func() {},
		now:        time.Now,
		ui:         func(f func()) { f() },
		openURL:    func(string) {},
		gh:         newGitHub(),
		checkEvery: checkEvery,
		handled:    map[string]bool{},
		background: context.Background(),
	}
}

// start opens the vault with the password in the keychain, or asks for it.
func (m *model) start() {
	if _, err := os.Stat(m.path); errors.Is(err, fs.ErrNotExist) {
		m.screen = screenStart
		return
	}
	m.link = loadLink(m.path)
	pw, err := savedPassword()
	if err == nil && pw != "" {
		v, err := m.openWith(pw)
		if err == nil {
			m.vault, m.screen = v, screenList
			return
		}
		if !errors.Is(err, errWrongPassword) {
			m.fail(err.Error())
			return
		}
	}
	m.askPassword()
}

// askPassword asks the master password when the keychain does not have it (a new computer,
// a password changed elsewhere) and keeps it there once it opens the vault.
func (m *model) askPassword() {
	m.screen = screenFatal // esc quits
	m.ask("Open Pico Auth", []question{{label: "Master password", secret: true}}, func(a []string) string {
		v, err := m.openWith(a[0])
		if errors.Is(err, errWrongPassword) {
			return "Wrong password"
		}
		if err != nil {
			return capitalize(err.Error())
		}
		m.vault, m.screen = v, screenList
		if err := savePassword(a[0]); err != nil {
			m.warn(capitalize(err.Error()))
		}
		return ""
	})
	m.form.cancel = m.quit
}

// openWith opens the vault with a password and the second key of the keychain, if any.
func (m *model) openWith(pw string) (*Vault, error) {
	second, err := savedSecond()
	if err != nil {
		return nil, err
	}
	return openVault(m.path, pw, second)
}

func (m *model) fail(msg string) { m.fatal, m.screen = capitalize(msg), screenFatal }

// ---- Keys ----

func (m *model) Key(k string) {
	if m.screen != screenForm {
		m.status = nil
	}
	switch m.screen {
	case screenStart:
		if k == gui.KeyEsc {
			if m.startPage == 1 {
				m.startPage = 0
			} else {
				m.quit()
			}
		}

	case screenFatal:
		if k == gui.KeyEsc {
			m.quit()
		}

	case screenList:
		switch k {
		case gui.KeyUp, gui.KeyDown:
			m.cursor = move(m.cursor, k, len(m.vault.Accounts))
		case gui.KeyEnter:
			m.copyCode(m.cursor)
		case gui.KeyEsc:
			m.quit()
		}

	case screenForm:
		m.form.key(k)

	case screenSettings:
		if k == gui.KeyEsc {
			m.screen = screenList
		}

	case screenSync:
		m.syncKey(k)

	case screenConfirm, screenPreview:
		m.screen = m.after // any key other than y is no

	}
}

func (m *model) Rune(r rune) {
	if m.screen != screenForm {
		m.status = nil
	}
	switch m.screen {
	case screenStart:
		switch {
		case m.startPage == 0 && r == '1':
			m.newVaultForm()
		case m.startPage == 0 && r == '2':
			m.startPage = 1
		case m.startPage == 1 && r == '1':
			m.startJoin()
		}

	case screenApprove:
		m.approveRune(r)

	case screenList:
		has := len(m.vault.Accounts) > 0
		switch {
		case r == 'n':
			m.addForm()
		case r == 'm' && has:
			m.editForm()
		case r == 'd' && has:
			m.deleteAccount()
		case r == 's':
			m.openSettings()
		case r == 'q':
			m.quit()
		case r >= '1' && r <= '9':
			m.copyCode(int(r - '1'))
		}

	case screenForm:
		m.form.rune(r)

	case screenSettings:
		m.settingsRune(r)

	case screenSync:
		m.syncRune(r)

	case screenConfirm, screenPreview:
		m.screen = m.after
		if r == 'y' || r == 'Y' {
			m.yes()
		}

	}
}

func (m *model) Paste(text string) {
	if m.screen == screenForm {
		m.form.paste(text)
	}
}

func (m *model) CanPaste() bool { return m.screen == screenForm }

// tick runs every second: the codes on screen change by themselves, and a copied code
// leaves the clipboard after clipboardLife, if nothing else was copied in the meantime.
func (m *model) tick() {
	if m.now().Sub(m.lastCheck) >= m.checkEvery {
		m.lastCheck = m.now()
		m.checkRequests()
	}
	if m.copied == "" || m.now().Sub(m.copiedAt) < clipboardLife {
		return
	}
	if m.clipboard() == m.copied {
		m.copy("")
	}
	m.copied = ""
}

// move takes the cursor one step up or down in a list of n entries, going round.
func move(cursor int, k string, n int) int {
	if n == 0 {
		return 0
	}
	if k == gui.KeyUp {
		return (cursor - 1 + n) % n
	}
	return (cursor + 1) % n
}

// ---- Forms and questions ----

// ask opens a form; when it ends (done or esc) the screen goes back to the current one,
// unless done moved somewhere else.
func (m *model) ask(title string, qs []question, done func([]string) string) {
	back := m.screen
	m.form = newForm(title, qs, func(a []string) string {
		m.screen = back
		msg := done(a)
		if msg != "" {
			m.screen = screenForm
		}
		return msg
	}, func() { m.screen = back })
	if !m.form.finished() {
		m.screen = screenForm
	}
}

func (m *model) askYes(question []line, yes func()) {
	m.confirm, m.yes, m.after, m.screen = question, yes, m.screen, screenConfirm
}

func (m *model) saveVault() bool {
	if err := m.vault.save(); err != nil {
		m.status = gui.Line{gui.Strong("✘ Could not save: "+err.Error(), gui.Red)}
		return false
	}
	return true
}

func (m *model) say(msg string)  { m.status = gui.Line{gui.Strong("✔ "+msg, gui.Amber)} }
func (m *model) warn(msg string) { m.status = gui.Line{gui.Strong("✘ "+msg, gui.Red)} }

// ---- First start ----

const minPasswordLen = 8

func (m *model) newVaultForm() {
	m.ask("New vault", []question{
		{label: "Choose the master password (at least 8 characters)", secret: true, check: func(v string, _ []string) string {
			if len([]rune(v)) < minPasswordLen {
				return "Use at least 8 characters"
			}
			return ""
		}},
		{label: "Type it again", secret: true, check: func(v string, a []string) string {
			if v != a[0] {
				return "The two passwords are different"
			}
			return ""
		}},
	}, func(a []string) string {
		v, err := newVault(m.path, a[0])
		if err == nil {
			err = v.save()
		}
		if err != nil {
			return "Could not create the vault: " + err.Error()
		}
		m.vault, m.screen = v, screenList
		m.say("Vault created")
		if err := savePassword(a[0]); err != nil {
			m.warn(capitalize(err.Error()))
		}
		return ""
	})
}

// ---- Accounts ----

func (m *model) copyCode(i int) {
	if i < 0 || i >= len(m.vault.Accounts) {
		return
	}
	m.cursor = i
	a := m.vault.Accounts[i]
	code, _ := a.code(m.now())
	m.copy(code)
	m.copied, m.copiedAt = code, m.now()
	m.say(a.Name + " code copied: it leaves the clipboard in 30 seconds")
}

// addForm asks the secret first, then the names, proposing those found in an otpauth:// address.
func (m *model) addForm() {
	var acc *Account
	m.ask("New account", []question{{
		label: "Secret key or otpauth:// address (paste it)",
		check: func(v string, _ []string) string {
			a, err := parseInput(v)
			if err != nil {
				return capitalize(err.Error())
			}
			acc = a
			return ""
		},
	}}, func([]string) string {
		m.ask("New account", []question{
			{label: "Name of the service", value: acc.Name},
			{label: "Account on the service", value: acc.User, optional: true},
		}, func(a []string) string {
			acc.Name, acc.User = a[0], a[1]
			m.pending, m.after, m.screen = acc, screenList, screenPreview
			m.yes = func() {
				m.vault.Accounts = append(m.vault.Accounts, acc)
				if m.saveVault() {
					m.cursor = len(m.vault.Accounts) - 1
					m.say(acc.Name + " saved")
				} else {
					m.vault.Accounts = m.vault.Accounts[:len(m.vault.Accounts)-1]
				}
			}
			return ""
		})
		return ""
	})
}

func (m *model) editForm() {
	a := m.vault.Accounts[m.cursor]
	m.ask(a.Name+" › edit", []question{
		{label: "Name of the service", value: a.Name},
		{label: "Account on the service", value: a.User, optional: true},
	}, func(ans []string) string {
		old := *a
		a.Name, a.User = ans[0], ans[1]
		if m.saveVault() {
			m.say(a.Name + " saved")
		} else {
			*a = old
		}
		return ""
	})
}

func (m *model) deleteAccount() {
	i := m.cursor
	a := m.vault.Accounts[i]
	m.askYes([]line{
		{gui.Strong("Delete "+label(a)+"?", gui.Amber)},
		{gui.Colored("Its codes can no longer be made here: check that the phone still has it,", gui.Muted)},
		{gui.Colored("or that you can log in to the service in another way.", gui.Muted)},
	}, func() {
		old := m.vault.Accounts
		m.vault.Accounts = append(append([]*Account{}, old[:i]...), old[i+1:]...)
		if !m.saveVault() {
			m.vault.Accounts = old
			return
		}
		m.cursor = min(m.cursor, max(len(m.vault.Accounts)-1, 0))
		m.say(a.Name + " deleted")
	})
}

// ---- Helpers ----

func label(a *Account) string {
	if a.User == "" {
		return a.Name
	}
	return a.Name + " (" + a.User + ")"
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func count(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return strconv.Itoa(n) + " " + what + "s"
}
