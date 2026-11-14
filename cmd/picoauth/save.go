package main

import (
	"errors"
	"net"
	"net/url"

	"picotools/internal/gui"
)

// Everyday use of a linked vault (docs/picoauth-sync.md, section 7):
//   - every change is uploaded at once; without GitHub nothing is changed (the codes still work);
//   - if another computer uploaded meanwhile, GitHub refuses: Pico Auth downloads the newer vault,
//     applies the same single change again on it and uploads;
//   - while the account list is on screen, Pico Auth reads the repository at start and every
//     10 seconds: a newer vault, and requests of new computers.

// maxRetries is how many times a change is applied again after a conflict.
const maxRetries = 5

// change is one operation on the accounts. It gets a copy of the accounts and gives them changed.
type change func(accounts []*Account) ([]*Account, error)

func cloneAccounts(as []*Account) []*Account {
	out := make([]*Account, len(as))
	for i, a := range as {
		c := *a
		out[i] = &c
	}
	return out
}

// findAccount is the index of the account equal to a, or -1: after a download the accounts are new
// objects, so the account changed here is found again by its contents.
func findAccount(as []*Account, a Account) int {
	for i, x := range as {
		if *x == a {
			return i
		}
	}
	return -1
}

var errDeletedElsewhere = errors.New("the account was deleted on another computer")

// apply makes a change: on this computer only, or on GitHub first when the vault is linked.
// done runs once the change is saved.
func (m *model) apply(c change, done func()) {
	accs, err := c(cloneAccounts(m.vault.Accounts))
	if err != nil {
		m.warn(capitalize(err.Error()))
		return
	}
	if m.link == nil {
		old := m.vault.Accounts
		m.vault.Accounts = accs
		if !m.saveVault() {
			m.vault.Accounts = old
			return
		}
		done()
		return
	}
	m.upload(c, accs, done, 0)
}

// upload sends the changed vault to GitHub, replacing the version read last.
func (m *model) upload(c change, accs []*Account, done func(), attempt int) {
	cp := *m.vault
	cp.Accounts = accs
	data, err := cp.encrypted()
	if err != nil {
		m.warn(capitalize(err.Error()))
		return
	}
	m.saving = true
	m.status = gui.Line{gui.Colored("Saving on GitHub…", gui.Muted)}
	repo, sha, ctx := m.link.Repo, m.link.SHA, m.background
	go func() {
		newSHA, err := m.gh.putFile(ctx, repo, vaultFile, data, sha, "Pico Auth: vault changed")
		var raw []byte
		if errors.Is(err, errConflict) && attempt < maxRetries {
			raw, newSHA, err = m.gh.getFile(ctx, repo, vaultFile)
			if err == nil {
				err = errConflict
			}
		}
		m.ui(func() {
			m.saving = false
			m.status = nil
			if m.vault == nil || m.link == nil {
				return // the vault was removed from this computer meanwhile
			}
			switch {
			case errors.Is(err, errConflict) && raw != nil:
				// Another computer saved meanwhile: take its vault and do the change again on it.
				if err := m.applyRemote(raw, newSHA); err != nil {
					m.warn("Could not read the newer vault on GitHub, nothing was changed: " + err.Error())
					return
				}
				accs, err := c(cloneAccounts(m.vault.Accounts))
				if err != nil {
					m.warn(capitalize(err.Error()))
					return
				}
				m.upload(c, accs, done, attempt+1)
			case errors.Is(err, errLoginNeeded):
				m.relogin(func() { m.apply(c, done) })
			case offline(err):
				m.warn("No connection to GitHub: nothing was changed. The codes still work.")
			case err != nil:
				m.warn("Could not save on GitHub, nothing was changed: " + err.Error())
			default:
				if err := writeFile(m.path, data); err != nil {
					m.warn("Saved on GitHub, but not on this computer: " + err.Error())
				}
				m.vault.Accounts = accs
				m.link.SHA = newSHA
				m.link.save(m.path)
				m.clampCursor()
				done()
			}
		})
	}()
}

// offline says whether err means GitHub could not be reached.
func offline(err error) bool {
	var ue *url.Error
	var ne net.Error
	return errors.As(err, &ue) || errors.As(err, &ne)
}

// applyRemote takes the vault downloaded from GitHub as the local copy.
func (m *model) applyRemote(raw []byte, sha string) error {
	v, err := decryptVault(raw, m.path, m.vault.password, m.vault.second)
	if err != nil {
		return err
	}
	if err := writeFile(m.path, raw); err != nil {
		return err
	}
	m.vault.Accounts = v.Accounts
	m.link.SHA = sha
	m.link.save(m.path)
	m.clampCursor()
	return nil
}

func (m *model) clampCursor() {
	m.cursor = min(m.cursor, max(len(m.vault.Accounts)-1, 0))
}

// checkRemote reads the repository: a newer vault and requests of new computers.
// It runs only on a linked computer with the account list on screen, and finds nothing new
// without a connection: it tries again at the next check.
func (m *model) checkRemote() {
	if m.link == nil || m.vault == nil || m.vault.second == nil || m.checking || m.saving || m.screen != screenList {
		return
	}
	m.register()
	m.markUsed()
	m.checking = true
	repo, sha, ctx, now := m.link.Repo, m.link.SHA, m.background, m.now()
	go func() {
		// One call tells both: the version of the vault and whether there are requests.
		es, err := m.gh.listDir(ctx, repo, "")
		var raw []byte
		var newSHA string
		var found []*pendingRequest
		if err == nil {
			for _, e := range es {
				switch {
				case e.Name == vaultFile && e.SHA != sha:
					raw, newSHA, _ = m.gh.getFile(ctx, repo, vaultFile)
				case e.Name == requestsDir && e.Type == "dir":
					found = m.fetchRequests(ctx, repo, now)
				}
			}
		}
		m.ui(func() {
			m.checking = false
			if m.screen != screenList || m.saving || m.link == nil {
				return // at the next check
			}
			if raw != nil && newSHA != m.link.SHA {
				if err := m.applyRemote(raw, newSHA); err != nil {
					m.warn("Could not read the newer vault on GitHub: " + err.Error())
				}
			}
			m.showRequest(found)
		})
	}()
}
