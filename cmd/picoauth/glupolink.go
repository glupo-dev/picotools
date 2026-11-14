package main

import (
	"errors"
	"regexp"
	"strings"
)

// Link this vault to glupo.dev (docs/picoauth-glupo-sync.md, section 5): Settings › Sync with
// glupo.dev asks a user name and an invite code, the server checks them, then the account is
// created with login_hash, and the vault is uploaded encrypted with master password + a new
// second key, as with GitHub.

var usernameRe = regexp.MustCompile(`^[a-z0-9_-]{3,32}$`)

const glupoTitle = "Settings › Sync with glupo.dev"

func (m *model) startGlupoLink() { m.glupoForm("", "", "") }

// glupoForm asks user name and invite code; msg is the error of an earlier try, shown under it.
func (m *model) glupoForm(user, invite, msg string) {
	m.screen = screenSettings // esc goes back to Settings
	m.ask(glupoTitle, []question{
		{label: "User name", value: user},
		{label: "Invite code", value: invite},
	}, func(a []string) string {
		u, code := normalUsername(a[0]), strings.TrimSpace(a[1])
		if !usernameRe.MatchString(u) {
			return capitalize(errBadUsername.Error())
		}
		if code == "" {
			return "Type the invite code you were given"
		}
		m.glupoCheck(u, code)
		return ""
	})
	if msg != "" {
		m.form.err = msg
	}
}

// glupoCheck asks the server whether the invite is usable and the name free. If the name is
// taken or the invite used, it may be this user's own account from a link that stopped halfway
// (account created, vault not uploaded): then the login with the master password works, and the
// upload is done again without a new account.
func (m *model) glupoCheck(user, invite string) {
	v := m.newSyncView(glupoTitle, screenSettings, "Checking the invite code…")
	ctx, later, password := v.ctx, v.later, m.vault.password
	go func() {
		hash := loginHash(user, password)
		err := m.gl.check(ctx, user, invite)
		resume := false
		if errors.Is(err, errUsernameTaken) || errors.Is(err, errInviteNotValid) {
			if m.gl.login(ctx, user, hash) == nil {
				resume, err = true, nil
			}
		}
		later(func() {
			switch {
			case err == nil && resume:
				m.glupoUpload(v, user)
			case err == nil:
				m.glupoCreate(v, user, invite, hash)
			case errors.Is(err, errUsernameTaken), errors.Is(err, errInviteNotValid), errors.Is(err, errBadUsername):
				m.glupoForm(user, invite, capitalize(err.Error()))
			case offline(err):
				v.err = "No connection to glupo.dev: nothing was changed."
			default:
				v.err = capitalize(err.Error())
			}
		})
	}()
}

// glupoCreate creates the account; the server keeps a hash of login_hash and gives a token.
func (m *model) glupoCreate(v *syncView, user, invite string, hash []byte) {
	v.msg = "Creating the account " + user + "…"
	ctx, later := v.ctx, v.later
	go func() {
		err := m.gl.createAccount(ctx, user, invite, hash)
		later(func() {
			switch {
			case err == nil:
				m.glupoUpload(v, user)
			case errors.Is(err, errUsernameTaken), errors.Is(err, errInviteNotValid), errors.Is(err, errBadUsername):
				m.glupoForm(user, invite, capitalize(err.Error()))
			case offline(err):
				v.err = "No connection to glupo.dev: nothing was changed."
			default:
				v.err = capitalize(err.Error())
			}
		})
	}()
}

// glupoUpload uploads the vault with a new second key, as linkTo does for GitHub.
func (m *model) glupoUpload(v *syncView, user string) {
	repo := glupoPrefix + user
	m.gl.creds(user, m.vault.password)
	// The second key goes in the keychain before the upload (see linkTo).
	second, err := newSecond()
	if err == nil {
		err = saveSecond(second)
	}
	if err != nil {
		v.err = capitalize(err.Error())
		return
	}
	m.vault.setSecond(second)
	data, err := m.vault.encrypted()
	m.vault.setSecond(nil) // the local copy changes only after the upload
	if err != nil {
		v.err = capitalize(err.Error())
		return
	}
	v.msg = "Uploading the vault to glupo.dev…"
	ctx, later := v.ctx, v.later
	go func() {
		version, err := m.rs.putFile(ctx, repo, vaultFile, data, "", "")
		later(func() {
			if errors.Is(err, errConflict) {
				v.err = "The account " + user + " already holds a vault on glupo.dev: nothing was done. A vault goes only where there is no vault."
				return
			}
			m.linked(v, repo, version, second, data, err)
		})
	}()
}

// ---- Open an existing vault › From glupo.dev (docs/picoauth-glupo-sync.md, section 6) ----

const glupoJoinTitle = "Open an existing vault › From glupo.dev"

func (m *model) startGlupoJoin() { m.glupoJoinForm("", "") }

// glupoJoinForm asks user name and master password: on glupo.dev the master password is also the
// login, so it is typed before the approval (with GitHub it is asked after).
func (m *model) glupoJoinForm(user, msg string) {
	m.screen, m.startPage = screenStart, 1 // esc goes back to the second page
	m.ask(glupoJoinTitle, []question{
		{label: "User name", value: user},
		{label: "Master password", secret: true},
	}, func(a []string) string {
		u, pw := normalUsername(a[0]), a[1]
		if !usernameRe.MatchString(u) {
			return capitalize(errBadUsername.Error())
		}
		m.glupoJoin(u, pw)
		return ""
	})
	if msg != "" {
		m.form.err = msg
	}
}

// glupoJoin logs in with the master password, then goes on as with GitHub: the vault, a request,
// the approval of a linked device, the second key.
func (m *model) glupoJoin(user, password string) {
	v := m.newSyncView(glupoJoinTitle, screenStart, "Logging in to glupo.dev…")
	v.password = password
	ctx, later := v.ctx, v.later
	go func() {
		err := m.gl.login(ctx, user, loginHash(user, password))
		later(func() {
			switch {
			case err == nil:
				m.gl.creds(user, password)
				m.join(v, glupoPrefix+user)
			case errors.Is(err, errWrongLogin):
				m.glupoJoinForm(user, capitalize(errWrongLogin.Error()))
			case offline(err):
				v.err = "Cannot reach glupo.dev: try again later."
			default:
				v.err = capitalize(err.Error())
			}
		})
	}()
}
