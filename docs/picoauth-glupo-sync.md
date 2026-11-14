# Pico Auth: sync through glupo.dev

A second way to sync the Pico Auth vault: through an API on the author's own server (glupo.dev),
**next to** the sync through GitHub, which stays. Discussed with the author on 2026-11-14.
Read [picoauth-sync.md](picoauth-sync.md) first: the vault, the second key, the approval of a new
device with the 6-digit code, conflicts and the device list are designed there, and this document reuses
them without repeating them.

**State on 2026-11-14: design only, nothing is built.** The flow of the first device is defined
(section 5); the rest are details to settle when building (section 9).

Each point is marked:
- **Decided**: agreed with the author.
- **Proposed**: suggested by Claude during the discussion; the author did not object, or called it a
  detail. Silence is not agreement: to confirm with the author when building.
- **Open**: not decided.

---

## 1. Why: GitHub is too hard for other people

- The GitHub sync works and the author is happy with it. It stays.
- To use it, a user needs a GitHub account, must create a **private repository** and must
  **"install" the GitHub App** Pico Auth on it (picoauth-sync.md, 2.2). The author, a developer with
  30 years of experience, did it on 2026-11-14 only with Claude's help and said they would not know how to do
  it again alone; they know nobody who knows how to "install" an app on GitHub. Having GitHub and
  knowing what a private repository is already leaves out about 99% of people.
- The concrete case: **colleagues** of the author who would like their TOTP codes on their PC instead
  of the phone. The author could not explain the GitHub procedure to them.
- Without sync, Pico Auth is fragile: the backup file was removed (picoauth-sync.md, section 8), so
  if the only computer breaks, all the TOTP accounts are lost. So the sync is needed **even by who uses
  one computer only**, as its safety copy. ("Use it without sync" is not an answer; the author called
  that question out of place.)
- So with GitHub only, Pico Auth stays an app for the author alone.
- **Decided.** The author wants to build the glupo.dev sync, **next to** the GitHub one ("lo voglio
  fare assolutamente affianco a sync con github").
- **Decided.** Pico Auth is **not sold**: free, for the author and people they choose.

## 2. The model

- **Decided** (the author's high-level framing): an app (Pico Auth) that must sync one file, and a server
  that keeps it. The server cannot read it: the vault is encrypted, for the server it is only bytes.
- Besides the vault, small files pass through the server as today with GitHub: request and answer
  while a device is added, one file per linked device (picoauth-sync.md, 5.1 and 6).
- **Decided.** The user (example: "Mario") has an account on glupo.dev; glupo.dev knows which requests
  are Mario's and lets Mario read and write only that vault.
- **Decided.** Pico Auth talks to an API on glupo.dev (for example under `api.glupo.dev`).

What exists on glupo.dev, **from the notes of other projects, not confirmed by the author in this chat**
(the infrastructure is documented in `~/Projects/glupo-dev/netcup/CLAUDE.md`): a Netcup VPS `glupo`
with Caddy (HTTPS), systemd, PostgreSQL 18, an SSO at `sso.glupo.dev`, the name `api.glupo.dev`.

## 3. Accounts, password, invite

- **Decided: no SSO** ("dimentica il sso"). Sign-up and login happen **inside Pico Auth**, which
  talks directly to the API: no browser, no separate login system. The API keeps the accounts itself.
  (A browser login on the SSO, like the GitHub Device Flow, was proposed and dropped.)
- **Decided: one password.** The **master password** of the vault is also the password of the account.
  The server never receives it: Pico Auth sends a `login_hash` computed from it (5, step 3), from which
  the server cannot get the password nor the vault key. Why: two passwords (one for the login, one for
  the vault) are hard to accept for users. The same scheme is used by Bitwarden.
  Price: changing the master password also changes the login (there is no password change yet, see
  picoauth-sync.md, 12).
- **Decided: an account with a user name.** The user name identifies the account and enters the salt of
  `login_hash`; it is not secret, unique on the server.
- **Proposed: no email.** The author: "potrei anche farne a meno, probabilmente". Without email the
  server keeps almost no personal data; a forgotten password cannot be reset by email, but a forgotten
  master password loses the vault anyway, and the account password is the master password. The choice
  "with or without email" was left to the author and is still to confirm (section 9); the flows below
  use no email.
- Fact, told to the author: whoever gets the server's database (or the author) could try to guess the
  master password offline from the stored hash of `login_hash`; Argon2id makes each try slow, and even a
  guessed password does not open the vault without `second_key`. So "the server cannot get the
  password" holds only for a strong master password; the vault stays safe anyway.
- **Decided: an invite code.** Only people the author chooses can sign up, so nobody on the internet uses
  the server as free storage. The author creates the code and gives it to the colleague; it is valid
  **once**; after sign-up it is never needed again (not on new devices either). How the author creates
  codes: Open (section 9).
- **Dropped: no account at all** (a vault reachable with a secret passed through the approval). It
  simplifies the legal side but complicates the rest: a new device cannot find its vault without typing
  an identifier, abandoned vaults cannot be claimed or deleted, anyone could fill the disk. The author
  agreed that "anche senza email si complicherebbe la storia" and the account stayed.

## 4. Before the sync: the vault always exists

- Pico Auth always has a master password before any sync: at first start `1 New vault` asks it (twice)
  and creates the vault; only then the account list and Settings exist.
- **Decided.** After creating the vault, Mario can either add accounts first and link later (or never),
  **case A**, or link the empty vault at once and add accounts after, **case B**. Same result. In case B
  each added account needs a connection (a linked vault blocks changes offline, picoauth-sync.md, 7);
  in case A accounts can be added offline because the vault is not linked yet.
- `2 Open an existing vault` → `From glupo.dev` (second page, section 7) is for a user who already has an account and a vault on
  glupo.dev (a new device, section 6).

## 5. Flow: link the first device (sign-up)

**Decided** (explained step by step and confirmed by the author, who considers it a specification).
The technical details inside the steps are **Proposed** (the author called them details).

Case A: Mario has a local vault with 3 accounts and an invite code.

1. **Settings.** Account list → `s` → Settings → `Sync with glupo.dev`.
2. **User name and invite**, on Mario's computer. A form with two fields: user name (`mario`) and
   invite code (`K7QM-3XPA`). The password is not asked: it is the master password, already in the
   keychain. Pico Auth sends **only** user name and invite code to the API and asks "are they valid?".
   The server checks that the code exists and is unused and that the name is free, **saves nothing**
   and answers. Messages (proposed): "Invite code not valid or already used", "This user name is taken",
   "No connection to glupo.dev"; Mario stays on the form.
   Why the early check (**Decided**, the author: "forse è più sensato"): Mario learns of a mistake while
   the form is in front of Mario. The real check is repeated at step 4.
3. **Preparation, only in memory** (nothing on disk or in the keychain yet, so a failure leaves
   nothing half done):
   - `login_hash = Argon2id(master_password, salt = "picoauth-login:" + username)`. The fixed prefix
     makes it different from the vault key; the user name makes it different for two users with the same
     password, and lets a new device compute the same value from user name + password alone.
   - `second_key` = 32 random bytes, generated **on Mario's computer**, once for the whole vault.
     The server never generates, receives or sees it.
   - The vault (with its 3 accounts) encrypted with
     `vault_key = HKDF-SHA256(Argon2id(master_password, vault_salt), second_key)` and AES-256-GCM, as
     today with GitHub (`vault_salt` is random, in the vault envelope). Opening it needs **both** the
     master password and `second_key`.
4. **Account on the server.** `POST /picoauth/accounts` (proposed) with `username`, `invite_code`,
   `login_hash`. In **one transaction** the server checks again invite and name, saves the account with
   a hash of `login_hash` (not `login_hash` itself), marks the invite used, generates a `token`
   (32 random bytes), keeps only `SHA-256(token)` and returns the token. If the check fails it saves
   nothing and answers an error; the computer is unchanged.
5. **Upload**, in this order:
   1. keychain: `token` and `second_key` (the second key before the upload, so a vault on the server
      always has its key somewhere, as with GitHub);
   2. `PUT /picoauth/vault` with `Authorization: Bearer <token>`: the encrypted vault; the server
      returns a `version` (the role of GitHub's sha);
   3. the file of this device (name, system, since, last);
   4. on disk: the local vault replaced by the encrypted one (same bytes as on the server), and
      `sync.json` (server, user name, version, device id). The local vault changes only after the
      server accepted the upload.
   If the upload fails halfway, the account exists with no vault: Pico Auth says the link did not
   finish; the next Sync does not create the account again (name taken) but logs in with
   `username` + `login_hash`, gets a new token and uploads.
6. **Done.** Settings shows "Synced with glupo.dev: mario". Everyday use as with GitHub: every change
   uploaded at once, check every 10 seconds, requests of new devices.

Case B: the same, with the empty vault; then each account added with `n` is uploaded at once.

For Mario: choose a password (at vault creation), then a user name and an invite code. No GitHub,
no repository, no browser.

## 6. Flow: add another device

**Proposed** (sketched, the author called the rest "dettagli"):
1. First start → `2 Open an existing vault` → `From glupo.dev`.
2. User name + master password. Pico Auth computes `login_hash`, logs in, receives a `token`
   (keychain), downloads the vault and cannot open it (no `second_key`).
3. The approval **exactly as today** (picoauth-sync.md, section 6): X25519 key pair (private part only
   in memory), request with the public key, 6-digit code `SHA-256(public_key)` first 4 bytes mod
   1,000,000; a linked device shows the same code, Mario compares, `y` + master password; the linked
   device encrypts `second_key` (ephemeral X25519, ECDH, HKDF, AES-256-GCM) and writes the answer; the new
   device decrypts it, keeps `second_key` in the keychain, deletes request and answer. The server only
   relays a public key and an encrypted blob.
4. The master password typed at step 2 opens the vault and is kept in the keychain.

## 7. In Pico Auth: two services

**Proposed** (Claude's suggestion, no answer from the author):
- The shared parts (second key, approval, conflicts, device list) only need: read, write, list and
  delete a file with its version, and refuse a write when the version changed. So: a small "storage"
  interface in `cmd/picoauth` with two implementations, `github.go` (exists) and a new glupo.dev client;
  the API on glupo.dev offers the same operations. Order: first the interface with GitHub behind it and
  the tests passing as today, then the API.
- First-start second page: `1 From GitHub` / `2 From glupo.dev` (the page was kept with GitHub alone
  exactly so that a source can be added, picoauth-sync.md, 4). Settings: `Sync with GitHub` /
  `Sync with glupo.dev`.
- A vault is linked to **one** service at a time. **Open: moving from one service to the other.**
  "Unlink, then link to the other" does not work as built: unlink deletes the local vault and the
  keychain secrets (picoauth-sync.md, 5), so nothing local is left to link. A path is still to design.
- Two services to test at every change of the sync.

## 8. Concerns and legal side

Concerns told to the author (facts and advice, not decisions):
1. Other people's data become the author's responsibility: if the server stops, changes are blocked
   (codes keep working, from the local copy).
2. Backups of the server are the author's job (GitHub keeps every version by itself). Losing the server
   does not lose the accounts: every linked device has the whole vault.
3. Accounts: sign-up, abuse; the invite code covers abuse.
4. Privacy and legal (below).
5. Trust: for a security app "a person's server" convinces less than GitHub; the presentation page
   (`picoauth.glupo.dev`, picoauth-sync.md 2.1 and 9) should explain the encryption.
6. Technical part, the easiest: versioned writes, ~360 calls an hour per open device (fine for a few
   users), only the storage client changes in Pico Auth.

Dropped: "the work network may block glupo.dev" — the author called it nonsense; it is a doubt, not a
fact, and the same doubt holds for `api.github.com`. Check: open `https://glupo.dev` in the browser of
the work PC.

Legal (Claude is not a lawyer, a list to verify, for a free service to a few colleagues; server in
Germany, GDPR applies even if free):
- A short **privacy notice**: controller and contact, data kept (account, encrypted vault, device names
  and dates, IP addresses in logs if any), purpose and basis (running the service the user asked for),
  how long (as long as the account exists), processor (Netcup), the user's rights.
- A **data processing agreement** (AVV) with Netcup, usually signed from the customer panel.
- Account deletion possible (by the user or on request).
- Few data, protected: HTTPS, short logs; the encrypted vault helps a lot.
- Breach: duty to notify the authority within 72 hours.
- Useful, not mandatory: short terms of use (free, no guarantee, lost master password = vault lost);
  **Impressum** probably needed if the author lives in or targets Germany: to check.
- IP addresses: the server sees them, it keeps them only if something logs them (Caddy logs a site only
  with a `log` directive; the API only if it writes them; Netcup may have network records). The
  author's point: it is the same as anyone visiting one of the author's pages. Without email the server keeps
  almost nothing that tells who the user is; the device name (for example "MacBook-di-Mario") is the only
  hint.

## 9. Still open

The author called all of these "dettagli", to settle when building; Claude prepares proposals, the
author confirms:
- How the author creates invite codes (a command on the VPS, a page only for the author, other).
- What happens on the server when the last device unlinks: account and vault kept or deleted (the
  "orphan vault", also open for GitHub, picoauth-sync.md 8).
- Unlink: revoke the device's token on the server (so unlink needs a connection, as with GitHub,
  where this is also still to confirm).
- Moving a vault between GitHub and glupo.dev (section 7).
- Unspecified details of the API: how the server hashes `login_hash` (algorithm); which calls write the
  device, request and answer files and whether the names `computers/`, `requests/`, `answers/` stay;
  how `sync.json` records which service it belongs to; what a new device shows when the account exists
  but has no vault (the upload stopped halfway, section 5 step 5).
- Email or not (section 3).
- Token lifetime: never expires with revocation at unlink, or expires and renews.
- Argon2id parameters of `login_hash`: must be identical on every device for ever: fixed in Pico Auth's
  code, or given by the server before login (as Bitwarden does).
- User name rules (case, allowed characters): it enters the salt, so `Mario` and `mario` differ.
- Limit on login attempts (online guessing).
- API shape: generic "file with version" endpoints (closest to GitHub, shared code untouched) or
  dedicated endpoints (`/vault`, `/requests`, `/devices`). The endpoint names in section 5 are examples.
- Server side: language (Go like Pico Auth, or Rust like the author's other services `idp-rs`,
  `dashboard-rs`), which repository, vault in PostgreSQL or on disk.
- Labels of the new menus and messages, and where `Sync with glupo.dev` goes in Settings (today:
  `1 Sync with GitHub` / `2 Delete this vault`). The host name (`api.glupo.dev`) is an example.
- Retry of a failed link: reuse the `second_key` already in the keychain or make a new one; what happens
  if only the device-file write fails.
- New device: the master password is typed before the approval (with GitHub it is asked after); a user
  who forgot it (the keychain always opens the app) finds out only there.
- Account deletion: no flow yet.
- To do before giving it to colleagues (section 8): privacy notice, AVV with Netcup, Impressum to check,
  server backups, the presentation page.
- Changing the master password (it changes `login_hash` too).

Next step: none fixed. The GitHub test with the Windows PC (picoauth-sync.md, 13) is still to do, and it
also tests the parts shared with glupo.dev.
