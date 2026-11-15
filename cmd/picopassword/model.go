package main

import (
	"errors"
	"io/fs"
	"os"
	"slices"
	"strings"
	"time"

	"picotools/internal/gui"
)

type screen int

const (
	screenStart    screen = iota // first start: new vault
	screenList                   // the items
	screenItem                   // the page of one item: its data and what can be done with it
	screenHistory                // the old passwords of an item
	screenDelete                 // "Delete Amazon?": type yes
	screenSettings               // the rare operations on the whole vault
	screenShowAs                 // Settings › Show passwords as
	screenSuggest                // Settings › Suggested passwords
	screenEdit                   // Edit: name, user, password and Save on one page
	screenForm                   // questions, one after the other
	screenFatal                  // something went wrong at start: only esc
)

// How long a revealed password stays in clear on screen.
const revealLife = 30 * time.Second

// model holds the state of Pico Password and reacts to keys.
// It knows nothing about the window: the clipboard and quitting go through callbacks.
type model struct {
	vault   *Vault
	path    string // the vault file
	prefs   prefs  // the choices of this device
	screen  screen
	cursor  int  // selected item, in the list as shown
	favOnly bool // the list shows only the favourites

	open       string    // id of the item whose page is open
	itemCursor int       // selected action of the item page
	histCursor int       // selected old password
	delInput   []rune    // what is typed in "Delete Amazon?"
	showCursor int       // selected choice of Show passwords as
	examples   [2]string // Suggested passwords: an example strong and stronger

	quitArmed bool // esc was pressed once in the list: a second esc quits

	revealed   string // what is shown in clear: an item id, or "history:<n>"; "" nothing
	revealedAt time.Time

	form  *form
	edit  *editPage
	fatal string

	status gui.Line // message shown at the bottom, cleared by the next key
	cols   int
	rows   int

	copy func(text string)
	quit func()
	now  func() time.Time
}

func newModel(path string) *model {
	return &model{
		path: path,
		cols: gui.DefaultCols,
		rows: gui.DefaultRows,
		copy: func(string) {},
		quit: func() {},
		now:  time.Now,
	}
}

// start opens the vault with the password in the keychain, or asks for it.
func (m *model) start() {
	m.prefs = loadPrefs(m.path)
	if _, err := os.Stat(m.path); errors.Is(err, fs.ErrNotExist) {
		m.screen = screenStart
		return
	}
	pw, err := savedPassword()
	if err == nil && pw != "" {
		v, err := openVault(m.path, pw)
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

// askPassword asks the master password when the keychain does not have it and keeps it there
// once it opens the vault.
func (m *model) askPassword() {
	m.screen = screenFatal // esc quits
	m.ask("Open Pico Password", []question{{label: "Master password", secret: true}}, func(a []string) string {
		v, err := openVault(m.path, a[0])
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

func (m *model) fail(msg string) { m.fatal, m.screen = capitalize(msg), screenFatal }

// ---- Keys ----

func (m *model) Key(k string) {
	armed := m.quitArmed
	m.quitArmed = false
	if m.screen != screenForm {
		m.status = nil
	}
	switch m.screen {
	case screenStart, screenFatal:
		if k == gui.KeyEsc {
			m.quit()
		}
	case screenList:
		if k == gui.KeyEsc {
			m.escList(armed)
			return
		}
		m.listKey(k)
	case screenItem:
		m.itemKey(k)
	case screenHistory:
		m.historyKey(k)
	case screenDelete:
		m.deleteKey(k)
	case screenSettings:
		if k == gui.KeyEsc {
			m.screen = screenList
		}
	case screenShowAs:
		m.showAsKey(k)
	case screenSuggest:
		if k == gui.KeyEsc {
			m.screen = screenSettings
		}
	case screenEdit:
		m.editKey(k)
	case screenForm:
		m.form.key(k)
	}
}

func (m *model) Rune(r rune) {
	m.quitArmed = false
	if m.screen != screenForm {
		m.status = nil
	}
	switch m.screen {
	case screenStart:
		if r == '1' {
			m.newVaultForm()
		}
	case screenList:
		m.listRune(r)
	case screenItem:
		m.itemRune(r)
	case screenHistory:
		if r == 'r' {
			m.toggleReveal(historyID(m.histCursor))
		}
	case screenDelete:
		m.delInput = append(m.delInput, r)
	case screenSettings:
		m.settingsRune(r)
	case screenShowAs:
		if r >= '1' && int(r-'1') < len(showChoices) {
			m.chooseShow(int(r - '1'))
		}
	case screenSuggest:
		m.suggestRune(r)
	case screenEdit:
		m.editInsert([]rune{r})
	case screenForm:
		m.form.rune(r)
	}
}

func (m *model) Paste(text string) {
	switch m.screen {
	case screenForm:
		m.form.paste(text)
	case screenEdit:
		m.editInsert([]rune(strings.TrimRight(text, "\r\n")))
	}
}

func (m *model) CanPaste() bool { return m.screen == screenForm || m.screen == screenEdit }

// tick runs every second: a revealed password hides again after revealLife.
func (m *model) tick() {
	if m.revealed != "" && m.now().Sub(m.revealedAt) >= revealLife {
		m.revealed = ""
	}
}

func (m *model) toggleReveal(id string) {
	if m.revealed == id {
		m.revealed = ""
		return
	}
	m.revealed, m.revealedAt = id, m.now()
}

// ---- The list ----

func (m *model) listKey(k string) {
	n := len(m.shown())
	switch k {
	case gui.KeyUp, gui.KeyDown:
		m.select_(move(m.cursor, k, n))
	case gui.KeyLeft, gui.KeyRight:
		m.select_(page(m.cursor, k == gui.KeyRight, m.listRoom(), n))
	case gui.KeyEnter:
		if it := m.selected(); it != nil {
			m.copy(it.Password)
			m.say("Password copied: " + it.Name)
		}
	}
}

// escList: esc in the list quits only when pressed twice in a row, since an esc pressed by
// mistake to go "back" would otherwise close the program.
func (m *model) escList(armed bool) {
	if armed {
		m.quit()
		return
	}
	m.quitArmed = true
	m.status = gui.Line{gui.Strong("Press esc again to quit", gui.Amber)}
}

func (m *model) listRune(r rune) {
	it := m.selected()
	switch {
	case r == 'u' && it != nil:
		m.copy(it.User)
		m.say("User copied: " + it.Name)
	case r == 'r' && it != nil:
		m.toggleReveal(it.ID)
	case r == 'o' && it != nil:
		m.openItem(it)
	case r == 'n':
		m.openNew()
	case r == 'f':
		m.toggleFavOnly()
	case r == 's':
		m.revealed = ""
		m.screen = screenSettings
	}
}

// shown is the list as it is on screen: every item, or only the favourites.
func (m *model) shown() []*Item {
	if m.vault == nil {
		return nil
	}
	if !m.favOnly {
		return m.vault.Items
	}
	var out []*Item
	for _, it := range m.vault.Items {
		if it.Favourite {
			out = append(out, it)
		}
	}
	return out
}

// selected is the item under the cursor, nil in an empty list.
func (m *model) selected() *Item {
	shown := m.shown()
	if m.cursor < 0 || m.cursor >= len(shown) {
		return nil
	}
	return shown[m.cursor]
}

// select_ moves the cursor; a revealed password hides when the selection moves.
func (m *model) select_(i int) {
	if i != m.cursor {
		m.revealed = ""
	}
	m.cursor = i
}

// selectItem puts the cursor on an item, if it is in the list as shown.
func (m *model) selectItem(it *Item) {
	if i := slices.Index(m.shown(), it); i >= 0 {
		m.cursor = i
		return
	}
	m.cursor = min(m.cursor, max(len(m.shown())-1, 0))
}

// toggleFavOnly switches between all the items and the favourites, keeping the cursor on the
// same item when it is still in the list.
func (m *model) toggleFavOnly() {
	it := m.selected()
	m.favOnly, m.cursor, m.revealed = !m.favOnly, 0, ""
	if it != nil {
		m.selectItem(it)
	}
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

// page moves the cursor of a list of n entries shown room at a time, as in Pico DB: down, to
// the last entry of its page, or when it is already there to the last entry of the next page;
// up, the same with the first entry and the previous page.
func page(cursor int, down bool, room, n int) int {
	if n == 0 {
		return 0
	}
	first := cursor / room * room
	if down {
		last := min(first+room-1, n-1)
		if cursor < last {
			return last
		}
		return max(min(cursor+room, n-1), 0)
	}
	if cursor > first {
		return first
	}
	return max(cursor-room, 0)
}

// ---- Forms ----

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

func (m *model) say(msg string)  { m.status = gui.Line{gui.Strong("✔ "+msg, gui.Amber)} }
func (m *model) warn(msg string) { m.status = gui.Line{gui.Strong("✘ "+msg, gui.Red)} }

// save writes the vault; if it fails, undo puts back what was changed in memory.
func (m *model) save(undo func()) bool {
	if err := m.vault.save(); err != nil {
		undo()
		m.warn("Could not save: " + err.Error())
		return false
	}
	return true
}

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
		{label: "Type it again", secret: true, same: 1},
	}, func(a []string) string {
		v, err := newVault(m.path, a[0])
		if err == nil {
			err = v.save()
		}
		if err != nil {
			return "Could not create the vault: " + err.Error()
		}
		m.vault, m.screen, m.cursor, m.favOnly = v, screenList, 0, false
		m.say("Vault created")
		if err := savePassword(a[0]); err != nil {
			m.warn(capitalize(err.Error()))
		}
		return ""
	})
}

// sortItems puts the items in alphabetical order by name, ignoring case.
func sortItems(items []*Item) {
	slices.SortStableFunc(items, func(a, b *Item) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
}

// ---- Helpers ----

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
