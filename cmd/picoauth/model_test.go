package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"picotools/internal/gui"
)

type testModel struct {
	*model
	clip string
	at   time.Time
}

func newTestModel(t *testing.T, dir string) *testModel {
	m := newModel(filepath.Join(dir, "picoauth", "vault.picoauth"))
	m.downloads = filepath.Join(dir, "Downloads")
	os.MkdirAll(m.downloads, 0o700)
	m.rows, m.cols = 40, 120
	tm := &testModel{model: m, at: time.Unix(1700000000, 0)}
	m.copy = func(s string) { tm.clip = s }
	m.clipboard = func() string { return tm.clip }
	m.now = func() time.Time { return tm.at }
	m.start()
	return tm
}

func (tm *testModel) typ(text string) {
	for _, r := range text {
		tm.Rune(r)
	}
	tm.Key(gui.KeyEnter)
}

func (tm *testModel) text() string {
	var b strings.Builder
	for _, l := range tm.Render(tm.cols, tm.rows) {
		for _, s := range l {
			b.WriteString(s.Text)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func TestFlow(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	tm := newTestModel(t, dir)
	if tm.screen != screenStart {
		t.Fatalf("screen %v", tm.screen)
	}

	// New vault: the password must be long enough and typed twice the same.
	tm.Rune('1')
	tm.typ("short")
	if !strings.Contains(tm.text(), "at least 8") {
		t.Fatal(tm.text())
	}
	for range 5 {
		tm.Key(gui.KeyBackspace)
	}
	tm.typ("master-pw")
	tm.typ("master-pw")
	if tm.screen != screenList {
		t.Fatalf("screen %v\n%s", tm.screen, tm.text())
	}
	if pw, _ := savedPassword(); pw != "master-pw" {
		t.Fatalf("keychain: %q", pw)
	}

	// New account from an otpauth address: the names are proposed, then the preview.
	tm.Rune('n')
	tm.Paste("otpauth://totp/GitHub:me?secret=JBSWY3DPEHPK3PXP&issuer=GitHub")
	tm.Key(gui.KeyEnter)
	tm.Key(gui.KeyEnter) // keep GitHub
	tm.Key(gui.KeyEnter) // keep me
	if tm.screen != screenPreview {
		t.Fatalf("screen %v\n%s", tm.screen, tm.text())
	}
	code, _ := tm.pending.code(tm.at)
	if !strings.Contains(tm.text(), spaced(code)) {
		t.Fatal(tm.text())
	}
	tm.Rune('y')
	if len(tm.vault.Accounts) != 1 || tm.vault.Accounts[0].Name != "GitHub" {
		t.Fatalf("%+v", tm.vault.Accounts)
	}

	// A secret alone, cancelled at the preview.
	tm.Rune('n')
	tm.typ("jbsw y3dp ehpk 3pxp")
	tm.typ("Test")
	tm.Key(gui.KeyEnter)
	tm.Rune('x')
	if tm.screen != screenList || len(tm.vault.Accounts) != 1 {
		t.Fatal("cancel did not cancel")
	}

	// Copy, and the clipboard is cleared after 30 seconds only if it still holds the code.
	tm.Rune('1')
	if tm.clip != code {
		t.Fatalf("clip %q", tm.clip)
	}
	tm.at = tm.at.Add(29 * time.Second)
	tm.tick()
	if tm.clip != code {
		t.Fatal("cleared too early")
	}
	tm.at = tm.at.Add(2 * time.Second)
	tm.tick()
	if tm.clip != "" {
		t.Fatal("not cleared")
	}
	tm.Rune('1')
	tm.clip = "something else"
	tm.at = tm.at.Add(time.Minute)
	tm.tick()
	if tm.clip != "something else" {
		t.Fatal("cleared what the user copied")
	}

	// Export: wrong password refused, then both files.
	tm.Rune('e')
	tm.Rune('2')
	tm.typ("nope")
	if !strings.Contains(tm.text(), "Wrong password") {
		t.Fatal(tm.text())
	}
	for range 4 {
		tm.Key(gui.KeyBackspace)
	}
	tm.typ("master-pw")
	tm.Key(gui.KeyEnter)
	list, err := os.ReadFile(filepath.Join(tm.downloads, "picoauth-otpauth-2023-11-14.txt"))
	if err != nil || !strings.HasPrefix(string(list), "otpauth://totp/GitHub:me?") {
		t.Fatalf("%q %v", list, err)
	}
	tm.Rune('e')
	tm.Rune('1')
	tm.typ("master-pw")
	tm.Key(gui.KeyEnter)
	backup := filepath.Join(tm.downloads, "picoauth-backup-2023-11-14.picoauth")
	if _, err := os.Stat(backup); err != nil {
		t.Fatal(err)
	}

	// Next start: the keychain opens it without asking.
	tm2 := newTestModel(t, dir)
	if tm2.screen != screenList || len(tm2.vault.Accounts) != 1 {
		t.Fatalf("screen %v", tm2.screen)
	}

	// New computer: empty keychain and the backup restored.
	keyring.MockInit()
	other := t.TempDir()
	tm3 := newTestModel(t, other)
	tm3.Rune('2')
	tm3.typ(backup)
	tm3.typ("bad password")
	if !strings.Contains(tm3.text(), "Wrong password") {
		t.Fatal(tm3.text())
	}
	for range len("bad password") {
		tm3.Key(gui.KeyBackspace)
	}
	tm3.typ("master-pw")
	if tm3.screen != screenList || len(tm3.vault.Accounts) != 1 {
		t.Fatalf("restore: %v\n%s", tm3.screen, tm3.text())
	}

	// Keychain lost: the password is asked once, then remembered.
	keyring.MockInit()
	tm4 := newTestModel(t, other)
	if tm4.screen != screenForm {
		t.Fatalf("screen %v", tm4.screen)
	}
	tm4.typ("master-pw")
	if tm4.screen != screenList {
		t.Fatal(tm4.text())
	}
	if pw, _ := savedPassword(); pw != "master-pw" {
		t.Fatal("not remembered")
	}

	// Edit and delete.
	tm4.Rune('m')
	for range len("GitHub") {
		tm4.Key(gui.KeyBackspace)
	}
	tm4.typ("GitHub work")
	tm4.Key(gui.KeyEnter)
	if tm4.vault.Accounts[0].Name != "GitHub work" {
		t.Fatal(tm4.vault.Accounts[0].Name)
	}
	tm4.Rune('d')
	tm4.Rune('y')
	if len(tm4.vault.Accounts) != 0 {
		t.Fatal("not deleted")
	}
	v, _ := openVault(tm4.path, "master-pw")
	if len(v.Accounts) != 0 {
		t.Fatal("delete not saved")
	}
}
