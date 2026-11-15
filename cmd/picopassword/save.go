package main

import (
	"errors"
	"net"
	"net/url"
	"slices"

	"picotools/internal/gui"
)

// Everyday use of a synced vault, exactly as in Pico Auth (docs/picoauth-sync.md, section 7; for
// Pico Password docs/picopassword.md, section 5.2):
//   - every change is uploaded at once; without GitHub nothing is changed (the codes still work);
//   - if another computer uploaded meanwhile, GitHub refuses: Pico Password downloads the newer vault,
//     applies the same single change again on it and uploads;
//   - while the list is on screen, Pico Password reads the repository at start and every
//     10 seconds: a newer vault, and requests of new computers.

// maxRetries is how many times a change is applied again after a conflict.
const maxRetries = 5

// change is one operation on the items. It gets a copy of the items and gives them changed.
type change func(items []*Item) ([]*Item, error)

func cloneItems(items []*Item) []*Item {
	out := make([]*Item, len(items))
	for i, it := range items {
		c := *it
		c.History = slices.Clone(it.History)
		out[i] = &c
	}
	return out
}

// findItem is the index of the item with this id, or -1: after a download the items are new
// objects, so the item changed here is found again by its hidden id (unlike Pico Auth, which
// finds an account by its contents: here the password itself is what changes).
func findItem(items []*Item, id string) int {
	return slices.IndexFunc(items, func(it *Item) bool { return it.ID == id })
}

var errDeletedElsewhere = errors.New("the item was deleted on another device")

// apply makes a change: on this computer only, or on GitHub first when the vault is linked.
// done runs once the change is saved.
func (m *model) apply(c change, done func()) {
	accs, err := c(cloneItems(m.vault.Items))
	if err != nil {
		m.warn(capitalize(err.Error()))
		return
	}
	if m.link == nil {
		old := m.vault.Items
		m.vault.Items = accs
		if !m.saveVault() {
			m.vault.Items = old
			return
		}
		done()
		return
	}
	m.upload(c, accs, done, 0)
}

// upload sends the changed vault to GitHub, replacing the version read last.
func (m *model) upload(c change, accs []*Item, done func(), attempt int) {
	m.glupoCreds()
	cp := *m.vault
	cp.Items = accs
	data, err := cp.encrypted()
	if err != nil {
		m.warn(capitalize(err.Error()))
		return
	}
	m.saving = true
	m.status = gui.Line{gui.Colored("Saving on "+m.where()+"…", gui.Muted)}
	repo, sha, ctx := m.link.Repo, m.link.SHA, m.background
	go func() {
		newSHA, err := m.rs.putFile(ctx, repo, vaultFile, data, sha, "Pico Password: vault changed")
		var raw []byte
		if errors.Is(err, errConflict) && attempt < maxRetries {
			raw, newSHA, err = m.rs.getFile(ctx, repo, vaultFile)
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
					m.warn("Could not read the newer vault on " + where(repo) + ", nothing was changed: " + err.Error())
					return
				}
				accs, err := c(cloneItems(m.vault.Items))
				if err != nil {
					m.warn(capitalize(err.Error()))
					return
				}
				m.upload(c, accs, done, attempt+1)
			case errors.Is(err, errLoginNeeded):
				m.loginNeeded(func() { m.apply(c, done) })
			case offline(err):
				m.offline = true
				m.warn("Cannot reach " + where(repo) + ": the change was not saved. Try again later.")
			case err != nil:
				m.warn("Could not save on " + where(repo) + ", nothing was changed: " + err.Error())
			default:
				if err := writeFile(m.path, data); err != nil {
					m.warn("Saved on " + where(repo) + ", but not on this device: " + err.Error())
				}
				m.vault.Items = accs
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
	var sel string
	if it := m.selected(); it != nil {
		sel = it.ID
	}
	m.vault.Items = v.Items
	if sel != "" {
		m.selectID(sel)
	}
	m.link.SHA = sha
	m.link.save(m.path)
	m.clampCursor()
	return nil
}

func (m *model) clampCursor() {
	m.cursor = min(m.cursor, max(len(m.shown())-1, 0))
}

// checkRemote reads the repository: a newer vault and requests of new devices.
// It runs only on a linked device with the list on screen, and finds nothing new
// without a connection: it tries again at the next check.
func (m *model) checkRemote() {
	if m.link == nil || m.vault == nil || m.vault.second == nil || m.checking || m.saving || m.screen != screenList {
		return
	}
	m.glupoCreds()
	m.register()
	m.markUsed()
	m.checking = true
	repo, sha, ctx, now := m.link.Repo, m.link.SHA, m.background, m.now()
	go func() {
		// One call tells both: the version of the vault and whether there are requests.
		es, err := m.rs.listDir(ctx, repo, "")
		var raw []byte
		var newSHA string
		var found []*pendingRequest
		if err == nil {
			for _, e := range es {
				switch {
				case e.Name == vaultFile && e.SHA != sha:
					raw, newSHA, _ = m.rs.getFile(ctx, repo, vaultFile)
				case e.Name == requestsDir && e.Type == "dir":
					found = m.fetchRequests(ctx, repo, now)
				}
			}
		}
		m.ui(func() {
			m.checking = false
			m.offline = err != nil && offline(err) // the change keys are grey until a check works
			if m.screen != screenList || m.saving || m.link == nil {
				return // at the next check
			}
			if raw != nil && newSHA != m.link.SHA {
				if err := m.applyRemote(raw, newSHA); err != nil {
					m.warn("Could not read the newer vault on " + where(repo) + ": " + err.Error())
				}
			}
			m.showRequest(found)
		})
	}()
}
