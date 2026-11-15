package main

import (
	"crypto/subtle"
	"errors"
	"io/fs"
	"os"

	"picotools/internal/gui"
)

// Delete the local vault, or unlink this computer (docs/picoauth-sync.md, section 5).
// Both leave this computer like new: no vault, nothing of Pico Password in the keychain, and at the
// next start the first-start menu. The vault on GitHub and the other computers do not change.
//   - Delete this vault: only for a vault not synced; its items are lost.
//   - Unlink this device: for a linked computer; the vault stays on GitHub.
// Both ask to type "delete" and the master password of the vault.

func (m *model) deleteVaultForm() {
	m.confirmForget("Delete this vault",
		"This deletes the vault and its "+count(len(m.vault.Items), "item")+" from this device, for ever. Type delete to confirm",
		func() { m.forgotten("Vault deleted") })
}

// unlinkForm reads first the linked computers, to warn when this is the last one.
// Unlinking needs GitHub, like every change of a linked vault: the file of this computer
// is deleted from computers/, so the list stays right.
func (m *model) unlinkForm() {
	m.glupoCreds()
	m.status = gui.Line{gui.Colored("Reading the linked devices from "+m.where()+"…", gui.Muted)}
	l, ctx := m.link, m.background
	go func() {
		cs, err := m.fetchComputers(ctx, l.Repo)
		m.ui(func() {
			m.status = nil
			if m.link != l || m.screen != screenSettings {
				return
			}
			if err != nil {
				m.unlinkFailed(err)
				return
			}
			others := 0
			for _, c := range cs {
				if c.id != l.Computer {
					others++
				}
			}
			warning := "This removes the vault from this device; it stays on " + where(l.Repo) + " and on your other devices. Type delete to confirm"
			if others == 0 {
				warning = "This is the only linked device: after this, nobody can open the vault on " + where(l.Repo) + " any more. Type delete to confirm"
			}
			m.confirmForget("Reset this device", warning, func() { m.unlink(l) })
		})
	}()
}

// unlink deletes the file of this computer from GitHub, then everything here.
func (m *model) unlink(l *linkState) {
	m.status = gui.Line{gui.Colored("Resetting…", gui.Muted)}
	ctx := m.background
	go func() {
		var err error
		if l.Computer != "" {
			var sha string
			_, sha, err = m.rs.getFile(ctx, l.Repo, computerPath(l.Computer))
			if err == nil {
				err = m.rs.deleteFile(ctx, l.Repo, computerPath(l.Computer), sha, "Pico Password: a device is unlinked")
			}
			if errors.Is(err, errNotFound) {
				err = nil
			}
		}
		// On glupo.dev the token of this device is revoked too.
		if err == nil && isGlupo(l.Repo) {
			m.gl.logout(ctx)
		}
		m.ui(func() {
			m.status = nil
			if m.link != l {
				return
			}
			if err != nil {
				m.unlinkFailed(err)
				return
			}
			m.forgotten("This device is reset: the vault is still on " + where(l.Repo))
		})
	}()
}

func (m *model) unlinkFailed(err error) {
	switch {
	case errors.Is(err, errLoginNeeded):
		m.loginNeeded(func() { m.screen = screenSettings; m.unlinkForm() })
	case offline(err):
		m.warn("No connection to " + m.where() + ": nothing was changed. A device is reset only online.")
	default:
		m.warn("Could not reset, nothing was changed: " + err.Error())
	}
}

// forgotten ends a delete or an unlink: everything goes from this computer.
func (m *model) forgotten(msg string) {
	if err := m.forgetHere(); err != nil {
		m.warn("Could not delete everything: " + err.Error())
		return
	}
	m.startPage, m.screen = 0, screenStart
	m.say(msg)
}

// confirmForget asks to type delete and the master password, then runs confirmed.
func (m *model) confirmForget(title, warning string, confirmed func()) {
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
		confirmed()
		return ""
	})
}

// forgetHere removes from this computer the vault, the link and the secrets in the keychain.
func (m *model) forgetHere() error {
	for _, p := range []string{m.path, linkPath(m.path), prefsPath(m.path)} {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	deletePassword()
	deleteSecond()
	m.gh.forget()
	m.gl.forget()
	m.vault, m.link, m.cursor, m.favOnly, m.prefs, m.offline = nil, nil, 0, false, prefs{}, false
	return nil
}
