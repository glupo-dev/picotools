package main

import (
	"crypto/subtle"
	"errors"
	"io/fs"
	"os"
)

// Delete the local vault, or unlink this computer (docs/picoauth-sync.md, section 5).
// Both leave this computer like new: no vault, nothing of Pico Auth in the keychain, and at the
// next start the first-start menu. The vault on GitHub and the other computers do not change.
//   - Delete this vault: only for a vault not linked to GitHub; its accounts are lost.
//   - Unlink this computer: for a linked computer; the vault stays on GitHub.
// Both ask to type "delete" and the master password of the vault.

func (m *model) deleteVaultForm() {
	m.confirmForget("Delete this vault",
		"This deletes the vault and its "+count(len(m.vault.Accounts), "account")+" from this computer, for ever. Type delete to confirm",
		"Vault deleted")
}

func (m *model) unlinkForm() {
	m.confirmForget("Unlink this computer",
		"This removes the vault from this computer; it stays on GitHub and on your other computers. Type delete to confirm",
		"This computer is unlinked: the vault is still on GitHub")
}

func (m *model) confirmForget(title, warning, done string) {
	m.ask(title, []question{
		{label: warning, check: func(v string, _ []string) string {
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
		if err := m.forgetHere(); err != nil {
			return "Could not delete everything: " + err.Error()
		}
		m.startPage, m.screen = 0, screenStart
		m.say(done)
		return ""
	})
}

// forgetHere removes from this computer the vault, the link and the secrets in the keychain.
func (m *model) forgetHere() error {
	for _, p := range []string{m.path, linkPath(m.path)} {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	deletePassword()
	deleteSecond()
	deleteToken()
	m.vault, m.link, m.gh.token, m.cursor = nil, nil, nil, 0
	return nil
}
