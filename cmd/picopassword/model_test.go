package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"picotools/internal/gui"
)

func typeText(m *model, s string) {
	for _, r := range s {
		m.Rune(r)
	}
}

// newTestModel creates a vault in a temporary folder, as the first start does.
func newTestModel(t *testing.T) *model {
	t.Helper()
	keyring.MockInit()
	m := newModel(filepath.Join(t.TempDir(), "vault.picopassword"))
	m.start()
	if m.screen != screenStart {
		t.Fatalf("first start: screen %v", m.screen)
	}
	m.Rune('1')
	typeText(m, "master-pw")
	m.Key(gui.KeyEnter)
	typeText(m, "master-pw")
	m.Key(gui.KeyEnter)
	if m.screen != screenList || m.vault == nil {
		t.Fatalf("vault not created: screen %v", m.screen)
	}
	return m
}

func addItem(m *model, name, user, pw string) {
	m.Rune('n')
	for _, v := range []string{name, user, pw, pw} {
		typeText(m, v)
		m.Key(gui.KeyEnter)
	}
	m.Key(gui.KeyEnter) // Save
}

func TestAddCopyAndReopen(t *testing.T) {
	m := newTestModel(t)
	var copied string
	m.copy = func(s string) { copied = s }

	addItem(m, "PayPal", "mario@example.com", "pp-secret")
	addItem(m, "amazon", "mario@example.com", "am-secret")
	if len(m.vault.Items) != 2 || m.vault.Items[0].Name != "amazon" {
		t.Fatalf("items not in alphabetical order: %+v", m.vault.Items)
	}
	if m.cursor != 0 {
		t.Fatalf("the new item should be selected, cursor %d", m.cursor)
	}
	m.Key(gui.KeyDown)
	m.Key(gui.KeyEnter)
	if copied != "pp-secret" {
		t.Fatalf("copied %q", copied)
	}
	m.Rune('u')
	if copied != "mario@example.com" {
		t.Fatalf("copied user %q", copied)
	}

	// Opened again with the password in the keychain.
	m2 := newModel(m.path)
	m2.start()
	if m2.screen != screenList || len(m2.vault.Items) != 2 || m2.vault.Items[1].Password != "pp-secret" {
		t.Fatalf("reopen: screen %v items %+v", m2.screen, m2.vault)
	}
	if m2.vault.Items[0].ID == "" || m2.vault.Items[0].ID == m2.vault.Items[1].ID {
		t.Fatalf("items need distinct ids")
	}
}

func TestRepeatDifferentGoesBack(t *testing.T) {
	m := newTestModel(t)
	m.Rune('n')
	for _, v := range []string{"Netflix", "mario", "one", "two"} {
		typeText(m, v)
		m.Key(gui.KeyEnter)
	}
	m.Key(gui.KeyEnter)
	if m.screen != screenEdit || m.edit.row != rowRepeat || len(m.edit.fields[rowRepeat]) != 0 || m.edit.err != "The two passwords are different" {
		t.Fatalf("expected Repeat again, empty: screen %v", m.screen)
	}
	typeText(m, "one")
	m.Key(gui.KeyEnter)
	m.Key(gui.KeyEnter)
	if m.screen != screenList || m.vault.Items[0].Password != "one" {
		t.Fatalf("not saved: %+v", m.vault.Items)
	}
	m.Rune('n')
	m.Key(gui.KeyEsc)
	if m.screen != screenList || len(m.vault.Items) != 1 {
		t.Fatal("esc goes back to the list")
	}
}

func TestFormEditing(t *testing.T) {
	f := newForm("t", []question{{label: "Name", value: "Amazn"}}, func([]string) string { return "" }, func() {})
	f.key(gui.KeyLeft)
	f.rune('o')
	if string(f.input) != "Amazon" {
		t.Fatalf("insert at the cursor: %q", string(f.input))
	}
	f.key(gui.KeyBackspace)
	if string(f.input) != "Amazn" {
		t.Fatalf("backspace deletes one: %q", string(f.input))
	}
	f.paste("ab\n")
	if string(f.input) != "Amazabn" {
		t.Fatalf("paste at the cursor: %q", string(f.input))
	}
}

func TestTabShowsPassword(t *testing.T) {
	f := newForm("t", []question{{label: "Password", secret: true}}, func([]string) string { return "" }, func() {})
	typeText2 := func(s string) {
		for _, r := range s {
			f.rune(r)
		}
	}
	typeText2("k9#V")
	text := func() string {
		var b strings.Builder
		for _, l := range f.render(100) {
			for _, s := range l {
				b.WriteString(s.Text)
			}
			b.WriteString("\n")
		}
		return b.String()
	}
	if strings.Contains(text(), "k9#V") || !strings.Contains(text(), "••••") {
		t.Fatalf("hidden by default:\n%s", text())
	}
	f.key(gui.KeyTab)
	if !strings.Contains(text(), "k9#V") || !strings.Contains(text(), "tab hide") {
		t.Fatalf("tab shows it:\n%s", text())
	}
}

func TestRevealAndPages(t *testing.T) {
	m := newTestModel(t)
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		addItem(m, n, "u", "pw-"+n)
	}
	m.status = nil
	m.rows = 9 + 2 // two items per page
	m.cursor = 0
	m.Key(gui.KeyRight)
	if m.cursor != 1 {
		t.Fatalf("→ to the last of the page: %d", m.cursor)
	}
	m.Key(gui.KeyRight)
	if m.cursor != 3 {
		t.Fatalf("→ to the last of the next page: %d", m.cursor)
	}
	m.Key(gui.KeyLeft)
	if m.cursor != 2 {
		t.Fatalf("← to the first of the page: %d", m.cursor)
	}

	now := time.Now()
	m.now = func() time.Time { return now }
	m.Rune('r')
	if m.revealed != m.vault.Items[2].ID {
		t.Fatalf("r reveals the selected one")
	}
	m.Key(gui.KeyDown)
	if m.revealed != "" {
		t.Fatalf("moving hides it")
	}
	m.Rune('r')
	now = now.Add(revealLife)
	m.tick()
	if m.revealed != "" {
		t.Fatalf("hidden after %v", revealLife)
	}
}

func TestEditHistoryFavouriteDelete(t *testing.T) {
	m := newTestModel(t)
	addItem(m, "Amazon", "mario", "old-pw")
	addItem(m, "Netflix", "mario", "nf-pw")
	m.cursor = 0
	m.Rune('o')
	if m.screen != screenItem || m.current().Name != "Amazon" {
		t.Fatalf("o opens the item page: screen %v", m.screen)
	}

	// r shows the password on the item page.
	m.Rune('r')
	if m.revealed != m.current().ID {
		t.Fatal("r reveals on the item page")
	}

	// Edit: change the password, which asks Repeat; Save saves.
	m.Rune('e')
	m.Key(gui.KeyDown)
	m.Key(gui.KeyDown)
	typeText(m, "new-pw")
	m.Key(gui.KeyDown)
	if m.edit.row != rowRepeat {
		t.Fatalf("a changed password asks Repeat, row %d", m.edit.row)
	}
	typeText(m, "new-pX")
	m.Key(gui.KeyDown)
	m.Key(gui.KeyEnter)
	if m.screen != screenEdit || m.edit.err != "The two passwords are different" || m.edit.row != rowRepeat {
		t.Fatalf("repeat different: screen %v", m.screen)
	}
	typeText(m, "new-pw")
	m.Key(gui.KeyEnter) // enter on a field goes to the next row
	m.Key(gui.KeyEnter)
	it := m.current()
	if m.screen != screenItem || it.Password != "new-pw" || len(it.History) != 1 || it.History[0].Password != "old-pw" {
		t.Fatalf("edit: screen %v item %+v", m.screen, it)
	}

	// Edit the name only: no Repeat, no history; esc cancels everything.
	m.Rune('e')
	typeText(m, "X")
	m.Key(gui.KeyEsc)
	if m.screen != screenItem || it.Name != "Amazon" {
		t.Fatalf("esc cancels: %+v", it)
	}
	m.Rune('e')
	m.Key(gui.KeyBackspace)
	typeText(m, "n")
	m.Key(gui.KeyUp) // from Name up to Save
	if m.edit.row != rowSave {
		t.Fatalf("up from Name: row %d", m.edit.row)
	}
	m.Key(gui.KeyEnter)
	if m.screen != screenItem || it.Name != "Amazon" || len(it.History) != 1 {
		t.Fatalf("edit name: screen %v item %+v", m.screen, it)
	}

	// History: enter copies the old password.
	var copied string
	m.copy = func(s string) { copied = s }
	m.Rune('h')
	m.Key(gui.KeyEnter)
	if copied != "old-pw" {
		t.Fatalf("history copy: %q", copied)
	}
	m.Key(gui.KeyEsc)

	// Favourite, and the list of favourites only.
	m.Rune('f')
	if !it.Favourite {
		t.Fatal("f makes it a favourite")
	}
	m.Key(gui.KeyEsc)
	m.Rune('f')
	if len(m.shown()) != 1 || m.selected() != it {
		t.Fatalf("only favourites: %d shown", len(m.shown()))
	}
	m.Rune('f')

	// Delete: anything but yes goes back; yes deletes.
	m.Rune('o')
	m.Rune('d')
	typeText(m, "y")
	m.Key(gui.KeyEnter)
	if m.screen != screenItem || len(m.vault.Items) != 2 {
		t.Fatalf("y alone must not delete")
	}
	m.Rune('d')
	typeText(m, "YES")
	m.Key(gui.KeyEnter)
	if m.screen != screenList || len(m.vault.Items) != 1 || m.vault.Items[0].Name != "Netflix" {
		t.Fatalf("delete: screen %v items %+v", m.screen, m.vault.Items)
	}

	// Everything was saved.
	v, err := openVault(m.path, "master-pw")
	if err != nil || len(v.Items) != 1 {
		t.Fatalf("reopen: %v %v", err, v)
	}
}

func TestHistoryKeepsTen(t *testing.T) {
	m := newTestModel(t)
	addItem(m, "A", "u", "pw0")
	m.Rune('o')
	for i := 1; i <= 12; i++ {
		m.Rune('e')
		m.Key(gui.KeyEnter)
		m.Key(gui.KeyEnter)
		pw := "pw" + string(rune('a'+i))
		typeText(m, pw)
		m.Key(gui.KeyEnter)
		typeText(m, pw)
		m.Key(gui.KeyEnter)
		m.Key(gui.KeyEnter)
	}
	h := m.current().History
	if len(h) != maxHistory || h[0].Password != "pwl" {
		t.Fatalf("history: %d, first %q", len(h), h[0].Password)
	}
}

func TestShowPasswordsAs(t *testing.T) {
	m := newTestModel(t)
	if got := m.mask("Mario1975!"); got != "••••••••••" {
		t.Fatalf("default: %q", got)
	}
	m.Rune('s')
	m.Rune('1')
	m.Rune('3')
	if got := m.mask("Mario1975!"); got != "Ma••••••••" {
		t.Fatalf("first 2: %q", got)
	}
	m.prefs.ShowPasswords = "firstlast"
	if got := m.mask("Mario1975!"); got != "M••••••••!" {
		t.Fatalf("first and last: %q", got)
	}
	m.prefs.ShowPasswords = "8dots"
	if got := m.mask("ab"); got != "••••••••" {
		t.Fatalf("8 dots: %q", got)
	}
	// Saved on this device.
	if loadPrefs(m.path).ShowPasswords != "first2" {
		t.Fatalf("prefs not saved")
	}
}

func TestDeleteVault(t *testing.T) {
	m := newTestModel(t)
	addItem(m, "A", "u", "pw")
	m.Rune('s')
	m.Rune('3')
	typeText(m, "delete")
	m.Key(gui.KeyEnter)
	typeText(m, "master-pw")
	m.Key(gui.KeyEnter)
	if m.screen != screenStart || m.vault != nil {
		t.Fatalf("delete vault: screen %v", m.screen)
	}
	if pw, _ := savedPassword(); pw != "" {
		t.Fatal("the keychain still has the password")
	}
}

func TestEscTwiceQuits(t *testing.T) {
	m := newTestModel(t)
	quit := false
	m.quit = func() { quit = true }
	m.Key(gui.KeyEsc)
	if quit || m.status == nil {
		t.Fatal("the first esc only asks")
	}
	m.Key(gui.KeyDown)
	m.Key(gui.KeyEsc)
	if quit {
		t.Fatal("another key in between starts again")
	}
	m.Key(gui.KeyEsc)
	if !quit {
		t.Fatal("the second esc quits")
	}
}

func TestSuggestedPassword(t *testing.T) {
	m := newTestModel(t)
	m.Rune('n')
	for _, v := range []string{"Amazon", "mario"} {
		typeText(m, v)
		m.Key(gui.KeyEnter)
	}
	m.Key(gui.KeyLeft)
	strong := string(m.edit.fields[rowPassword])
	if len(strong) != 20 || strings.ContainsAny(strong, symbols) {
		t.Fatalf("strong: %q", strong)
	}
	m.Key(gui.KeyRight)
	stronger := string(m.edit.fields[rowPassword])
	if len(stronger) != 30 || !strings.ContainsAny(stronger, symbols) || string(m.edit.fields[rowRepeat]) != stronger {
		t.Fatalf("stronger: %q", stronger)
	}
	m.Key(gui.KeyDown)
	m.Key(gui.KeyDown)
	m.Key(gui.KeyEnter)
	if m.screen != screenList || m.vault.Items[0].Password != stronger {
		t.Fatalf("not saved: screen %v", m.screen)
	}
}

func TestSuggestedSettings(t *testing.T) {
	m := newTestModel(t)
	m.Rune('s')
	m.Rune('2')
	if m.screen != screenSuggest || m.examples[0] == "" {
		t.Fatalf("screen %v", m.screen)
	}
	m.Rune('1')
	for range "20" {
		m.Key(gui.KeyBackspace)
	}
	typeText(m, "12")
	m.Key(gui.KeyEnter)
	m.Key(gui.KeyEnter)
	for range "no" {
		m.Key(gui.KeyBackspace)
	}
	typeText(m, "yes")
	m.Key(gui.KeyEnter)
	if m.screen != screenSuggest {
		t.Fatalf("back to the page: %v", m.screen)
	}
	if s := loadPrefs(m.path).Strong; s == nil || s.Length != 12 || !s.Symbols {
		t.Fatalf("not saved: %+v", s)
	}
	if pw := m.suggest(false); len(pw) != 12 || !strings.ContainsAny(pw, symbols) || !strings.ContainsAny(pw, digits) {
		t.Fatalf("suggested %q", pw)
	}
}

func TestEditSuggest(t *testing.T) {
	m := newTestModel(t)
	addItem(m, "Amazon", "mario", "old-pw")
	m.Rune('o')
	m.Rune('e')
	m.Key(gui.KeyDown)
	m.Key(gui.KeyDown)
	m.Key(gui.KeyLeft)
	strong := string(m.edit.fields[rowPassword])
	if len(strong) != 20 || string(m.edit.fields[rowRepeat]) != strong {
		t.Fatalf("← strong, repeat filled: %q", strong)
	}
	m.Key(gui.KeyRight)
	stronger := string(m.edit.fields[rowPassword])
	if len(stronger) != 30 || string(m.edit.fields[rowRepeat]) != stronger {
		t.Fatalf("→ stronger: %q", stronger)
	}
	var b strings.Builder
	for _, l := range m.Render(100, 40) {
		for _, s := range l {
			b.WriteString(s.Text)
		}
	}
	if !strings.Contains(b.String(), stronger) {
		t.Fatal("a suggested password is shown")
	}
	m.Key(gui.KeyDown)
	m.Key(gui.KeyDown)
	m.Key(gui.KeyEnter)
	if m.screen != screenItem || m.current().Password != stronger || m.current().History[0].Password != "old-pw" {
		t.Fatalf("not saved: screen %v", m.screen)
	}

	// Typing after a suggestion starts again by hand.
	m.Rune('e')
	m.Key(gui.KeyDown)
	m.Key(gui.KeyDown)
	m.Key(gui.KeyLeft)
	typeText(m, "abc")
	if string(m.edit.fields[rowPassword]) != "abc" || len(m.edit.fields[rowRepeat]) != 0 || m.edit.suggested {
		t.Fatalf("typed: %q", string(m.edit.fields[rowPassword]))
	}
}
