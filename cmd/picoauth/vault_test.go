package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "picoauth", "vault.picoauth")
	v, err := newVault(path, "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	v.Accounts = []*Account{{Kind: kindTOTP, Name: "GitHub", User: "me", Secret: "JBSWY3DPEHPK3PXP", Algorithm: "SHA1", Digits: 6, Period: 30}}
	if err := v.save(); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(path)
	for _, leak := range []string{"GitHub", "JBSWY3DPEHPK3PXP", "\"me\""} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("the file shows %s in plain text", leak)
		}
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", info.Mode().Perm())
	}

	if _, err := openVault(path, "wrong"); !errors.Is(err, errWrongPassword) {
		t.Errorf("wrong password: %v", err)
	}
	w, err := openVault(path, "correct horse")
	if err != nil || len(w.Accounts) != 1 || *w.Accounts[0] != *v.Accounts[0] {
		t.Fatalf("open: %v", err)
	}

	// A save after opening keeps the same password.
	w.Accounts[0].Name = "GitLab"
	if err := w.save(); err != nil {
		t.Fatal(err)
	}
	x, err := openVault(path, "correct horse")
	if err != nil || x.Accounts[0].Name != "GitLab" {
		t.Fatalf("reopen: %v", err)
	}

	// Anything else is refused clearly.
	other := filepath.Join(t.TempDir(), "x.json")
	os.WriteFile(other, []byte(`{"hello":1}`), 0o600)
	if _, err := openVault(other, "x"); err == nil || errors.Is(err, errWrongPassword) {
		t.Errorf("not a vault: %v", err)
	}
}
