# Pico Auth: sync through glupo.dev

A second way to sync the Pico Auth vault: through an API on the author's own server (glupo.dev),
**next to** the sync through GitHub, which stays. Designed and built with the author on 2026-11-14.
Read [picoauth-sync.md](picoauth-sync.md) first: the vault, the second key, the approval of a new
device with the 6-digit code, conflicts, the device list and "Reset this device" are designed there,
and this document reuses them without repeating them.

**State on 2026-11-14:**
- **Block 1 (link the first device) is in production** at `https://vault.glupo.dev/picoauth`, on the
  author's VPS. The author's Mac is linked to it as `gennaro` (a test vault).
- **Block 2 (keep aligned)** works through the code shared with GitHub (upload at every change,
  10-second check, conflicts, device list): tried by the author on one device only (a change, the device
  list, the message without server).
- **Block 3 (add another device) is written** (2026-11-14, section 6): tested with two devices against
  the fake server and against the real server running on the Mac; **not tried by the author**, who has
  no second device at hand now. **Next step: the author tries it** (a second Pico Auth on the Mac, or the
  work PC), which also tries block 2 with two devices.
- Code: `cmd/picoauth` in picotools; the server is the separate project **picoauth-server**
  (`~/Projects/glupo-dev/picoauth-server`, private repository `glupo-dev/picoauth-server`). The code of
  both is pushed.

Block numbers in this document are its own (1 link, 2 keep aligned, 3 add a device); picoauth-sync.md
section 13 numbers the GitHub blocks differently (2 add a device, 3 keep aligned).

Each point is marked:
- **Decided**: agreed with the author.
- **Proposed**: suggested by Claude; the author did not object, or called it a detail. Silence is not
  agreement: to confirm with the author.
- **Open**: not decided.

---

## 1. Why: GitHub is too hard for other people

- The GitHub sync works and the author is happy with it. It stays.
- To use it, a user needs a GitHub account, must create a **private repository** and must
  **"install" the GitHub App** Pico Auth on it (picoauth-sync.md, 2.2). The author, a developer with
  30 years of experience, did it on 2026-11-14 only with Claude's help and said they would not know how to
  do it again alone; they know nobody who knows how to "install" an app on GitHub. Having GitHub and
  knowing what a private repository is already leaves out about 99% of people.
- The concrete case: **colleagues** of the author who would like their TOTP codes on their PC instead
  of the phone. The author could not explain the GitHub procedure to them.
- Without sync, Pico Auth is fragile: the backup file was removed (picoauth-sync.md, section 8), so
  if the only computer breaks, all the TOTP accounts are lost. So the sync is needed **even by who uses
  one computer only**, as its safety copy. ("Use it without sync" is not an answer; the author called
  that question out of place.)
- So with GitHub only, Pico Auth stays an app for the author alone.
- **Decided.** The author wants the glupo.dev sync **next to** the GitHub one ("lo voglio fare
  assolutamente affianco a sync con github").
- **Decided.** Pico Auth is **not sold**: free, for the author and people they choose.

## 2. The model

- **Decided** (the author's framing): an app (Pico Auth) that must sync one file, and a server that keeps
  it. The server cannot read it: the vault is encrypted, for the server it is only bytes.
- Besides the vault, the same small files as with GitHub pass through the server: request and answer
  while a device is added, one file per linked device (picoauth-sync.md, 5.1 and 6).
- **Decided.** The user (example: "Mario") has an account on glupo.dev; the server lets each account read
  and write only its own files.
- **Decided.** Pico Auth talks to the API at `https://vault.glupo.dev/picoauth` (section 9 for the name).

## 3. Accounts, password, invite

- **Decided: no SSO** ("dimentica il sso"). Sign-up and login happen **inside Pico Auth**, which talks
  directly to the API: no browser, no separate login system. (A browser login on the author's SSO
  `sso.glupo.dev`, like the GitHub Device Flow, was proposed and dropped.)
- **Decided: one password.** The **master password** of the vault is also the password of the account.
  The server never receives it: Pico Auth sends `login_hash` computed from it (section 5, step 3), from
  which the server cannot get the password nor the vault key. Why: two passwords (login and vault) are
  hard to accept for users. Bitwarden uses the same scheme. Price: changing the master password also
  changes the login (there is no password change yet, picoauth-sync.md 12).
- **Decided: an account with a user name**, unique on the server, not secret; it enters the salt of
  `login_hash`.
- **Decided ("ok vanno bene", 2026-11-14): no email**, only the user name. Without email the server
  keeps almost no personal data. A forgotten account password cannot be reset by email, but it is the
  master password, and a forgotten master password loses the vault anyway. (Earlier the author: "potrei
  anche farne a meno, probabilmente".)
- **Decided: an invite code.** Only people the author chooses can sign up, so nobody on the internet uses
  the server as free storage. The author creates the code with a command on the VPS and gives it to the
  colleague; it is valid **once**; after sign-up it is never needed again (not on new devices either).
- Fact, told to the author: whoever gets the server's database (or the author) could try to guess the
  master password offline from the stored hash of `login_hash`; Argon2id makes each try slow, and even a
  guessed password does not open the vault without `second_key`. So "the server cannot get the password"
  holds only for a strong master password; the vault stays safe anyway.
- **Dropped: no account at all** (a vault reachable with a secret passed through the approval). It
  simplifies the legal side but complicates the rest: a new device cannot find its vault without typing
  an identifier, abandoned vaults cannot be claimed or deleted, anyone could fill the disk. The author:
  "anche senza email si complicherebbe la storia".

## 4. Before the sync: the vault always exists

- Pico Auth always has a master password before any sync: at first start `1 New vault` asks it (twice)
  and creates the vault; only then the account list and Settings exist.
- **Decided.** After creating the vault, Mario can either add accounts first and link later (or never),
  **case A**, or link the empty vault at once and add accounts after, **case B**. Same result. In case B
  each added account needs a connection (a linked vault blocks changes offline, picoauth-sync.md, 7);
  in case A accounts can be added offline because the vault is not linked yet.
- First start → `2 Open an existing vault` → `2 From glupo.dev` is for a user who already has an
  account and a vault on glupo.dev: a new device (section 6).

## 5. Flow: link the first device (sign-up), as built

**Decided** (explained step by step and confirmed by the author, who considers it a specification).
The technical details inside the steps are **Proposed** (the author called them details).

Case A: Mario has a local vault with 3 accounts and an invite code.

1. **Settings.** Account list → `s` → Settings → `2 Sync with glupo.dev` (Settings of a vault not
   linked: `1 Sync with GitHub`, `2 Sync with glupo.dev`, `3 Delete this vault`).
2. **User name and invite**, on Mario's computer: a form with two fields, user name (`mario`, lowered by
   Pico Auth, checked against `a-z 0-9 - _`, 3 to 32) and invite code (`K7QM-3XPA`; case and dash do not
   matter). The password is not asked: it is the master password, already in the keychain.
   Pico Auth sends **only** user name and invite code (`POST /check`) and asks "are they valid?"; the
   server checks that the code exists and is unused and that the name is free, **saves nothing** and
   answers. On "Invite code not valid or already used" or "This user name is taken" the form comes back
   with the message and the values. Why the early check (**Decided**, the author: "forse è più
   sensato"): Mario learns of a mistake while the form is in front of them. The real check is repeated at
   step 4.
3. **Preparation, in memory**:
   - `login_hash = Argon2id(master_password, salt = "picoauth-login:" + username)`, with costs fixed
     for ever (3 passes, 64 MB, 4 threads, 32 bytes). The fixed prefix makes it different from the vault
     key; the user name makes it different for two users with the same password, and lets a new device
     compute the same value from user name + password alone.
   - `second_key` = 32 random bytes, generated **on Mario's computer**, once for the whole vault. The
     server never generates, receives or sees it.
   - The vault encrypted with `vault_key = HKDF-SHA256(Argon2id(master_password, vault_salt),
     second_key)` and AES-256-GCM, as with GitHub. Opening it needs **both** the master password and
     `second_key`.
4. **Account on the server.** `POST /accounts` with `username`, `invite_code`, `login_hash` (base64).
   In **one transaction** the server checks again invite and name, saves the account with `SHA-256` of
   `login_hash`, marks the invite used, makes a `token` (32 random bytes), keeps only `SHA-256(token)` and
   returns the token. If the check fails it saves nothing; the computer is unchanged.
5. **Upload**, in this order:
   1. keychain: the token (entry `glupo.dev access`, service `picoauth`) and `second_key` (before the
      upload, so a vault on the server always has its key somewhere, as with GitHub);
   2. `PUT /files/vault.picoauth` with `Authorization: Bearer <token>` and no version (= only if
      missing); the server returns a `version` (the role of GitHub's sha);
   3. on disk: the local vault replaced by the encrypted one (same bytes as on the server) and
      `sync.json` with `"repo": "glupo.dev/mario"` and the version; the local vault changes only after
      the server accepted the upload;
   4. the file of this device, `computers/<id>.json` (name, system, since, last).
   **A link that stopped halfway** (account created, upload failed) is finished by doing it again: the
   check fails (invite used, or name taken), so Pico Auth tries the login with user name + master
   password; if it works, it is this user's account and the vault is uploaded (a new `second_key` is
   made at each try). If the account already holds a vault: "The account … already holds a vault on
   glupo.dev: nothing was done."
6. **Done.** "This vault is now on glupo.dev, in the account mario"; Settings shows "Synced with
   glupo.dev: mario", `1 Reset this device` ("The vault stays on glupo.dev.") and `2 Linked devices`.
   Everyday use as with GitHub: every change uploaded at once, check every 10 seconds, requests of new
   devices, the device list.

Case B: the same, with the empty vault; then each account added with `n` is uploaded at once.

For Mario: choose a password (at vault creation), then a user name and an invite code. No GitHub, no
repository, no browser.

Behaviour once linked (**Proposed**, as built):
- When the server refuses the token (for example revoked by the author), the device logs in again **by
  itself** with its user name and master password, without asking. Only if that fails: "glupo.dev
  refused the login of this device: nothing was changed".
- Without the server, a change gives "Cannot reach glupo.dev: the change was not saved. Try again
  later." (text **chosen by the author**; the same with GitHub. Dropped: "No connection…: nothing was
  changed. The codes still work." and "No (active) connection…: could not save changes."). The codes
  keep working.
- **Reset this device** (picoauth-sync.md 5) deletes the device's file on the server, **revokes the
  device's token** (`POST /logout`), then deletes everything here; the vault stays on glupo.dev. As with
  GitHub, it reads the device list first and, on the last device, warns "This is the only linked device:
  after this, nobody can open the vault on glupo.dev any more". It needs the server: offline it says
  "No connection to glupo.dev: nothing was changed. A device is reset only online." (text by Claude,
  not reviewed by the author)
- Other texts written by Claude, not reviewed by the author, and not consistent with each other yet:
  without the server, the link says "No connection to glupo.dev: nothing was changed." and adding a
  device "Cannot reach glupo.dev: try again later." (`glupolink.go`); the reset text above. Only the
  change text "Cannot reach glupo.dev: the change was not saved. Try again later." is the author's.

## 6. Flow: add another device (block 3), as built

**Proposed** (the author called it "dettagli" and said to go ahead with Claude's two proposals below,
2026-11-14; not tried by the author yet):
1. First start → `2 Open an existing vault` → `2 From glupo.dev` (second page: `1 From GitHub`,
   `2 From glupo.dev`; the page was kept with GitHub alone exactly so a source can be added,
   picoauth-sync.md 4).
2. A form: user name and **master password** (`glupolink.go`, `glupoJoinForm`). The password is asked
   **before** the approval, because it is the login (with GitHub it is asked after the second key
   arrives). Pico Auth computes `login_hash` and calls `POST /login`; the token goes in the keychain.
   A wrong user name or password brings the form back, user name kept, with **one message for both**,
   "Wrong user name or master password" (the server does not say which, on purpose). Without the
   server: "Cannot reach glupo.dev: try again later."
3. The rest is the GitHub code of `join.go`, through `remotes`: the vault must exist ("The account …
   holds no vault on glupo.dev: link first the device that has it (Settings › Sync with glupo.dev)");
   request with an X25519 public key, 6-digit code compared on both screens, `y` + master password on a
   linked device, answer with `second_key` encrypted, request and answer deleted. The server only
   relays a public key and an encrypted blob.
4. The master password typed at step 2 opens the vault at once, without asking again, and goes in the
   keychain; "This device is now linked to glupo.dev, account mario".
5. If the request is refused, expires (15 minutes) or the user presses esc, the new device's token is
   revoked on the server and removed from the keychain (`leftJoin`).

Fixed on the way, for GitHub too: esc while waiting did not withdraw the request (the delete was sent
with the view's context, already cancelled by esc); it now uses the background context.

A user who forgot the master password finds out at step 2: the login fails. Nothing else to do: the
vault cannot be opened without it anyway.

## 7. Concerns and legal side

Concerns told to the author (facts and advice, not decisions):
1. Other people's data become the author's responsibility: if the server stops, changes are blocked
   (codes keep working, from the local copy).
2. Backups of the server are the author's job (GitHub keeps every version by itself). Losing the server
   does not lose the accounts: every linked device has the whole vault.
3. Accounts: sign-up, abuse; the invite code covers abuse.
4. Trust: for a security app "a person's server" convinces less than GitHub; the presentation page
   (`picoauth.glupo.dev`, picoauth-sync.md 2.1 and 9) should explain the encryption.
5. Technical load: ~360 calls an hour per open device, fine for a few users.

Dropped: "the work network may block glupo.dev" — the author called it nonsense; it is a doubt, not a
fact, and the same doubt holds for `api.github.com`.

Legal (Claude is not a lawyer; a list to verify, for a free service to a few colleagues; server in the EU,
GDPR applies even if free):
- A short **privacy notice**: controller and contact, data kept (user name, encrypted vault, device names
  and dates, IP addresses in logs if any), purpose and basis (running the service the user asked for),
  how long (as long as the account exists), processor (Netcup), the user's rights.
- A **data processing agreement** (AVV) with Netcup, usually signed from the customer panel.
- Account deletion possible (by the user or on request): no flow yet (section 10).
- Few data, protected: HTTPS, short logs; the encrypted vault helps a lot.
- Breach: duty to notify the authority within 72 hours.
- Useful, not mandatory: short terms of use (free, no guarantee, lost master password = vault lost);
  **Impressum** probably needed if the author lives in or targets Germany: to check.
- IP addresses: the server sees them, it keeps them only if something logs them (Caddy logs a site only
  with a `log` directive, the `vault.glupo.dev` block has none; picoauth-server does not log IPs; Netcup
  may have network records). The author's point: the same as anyone visiting one of the author's pages.

## 8. Server choices (2026-11-14)

**Decided by the author:**
- The server is a **separate project**, in **Rust** ("il server non sa cosa sia go"), like the author's
  other services: **`picoauth-server`**, folder `~/Projects/glupo-dev/picoauth-server`, private
  repository `glupo-dev/picoauth-server` (private like idp-rs and dashboard-rs). Name chosen by the author
  over `picoauth-rs` and `picoauth-api-rs`; it matches `schule-server`. Deployed like the others
  (`~/Projects/glupo-dev/netcup/CLAUDE.md`): no Docker, a binary + systemd service behind Caddy.
  (Go inside picotools was proposed and dropped.)
- Data in **PostgreSQL** (already on the VPS for idp-rs), not files: account creation and "write only if
  the version is unchanged" must be all-or-nothing (a transaction, an `UPDATE … WHERE version`).
- Invites: **a command on the server**, like the invites of idp-rs; a web page only for the author was
  left out for now.

**Accepted by the author** ("ok vanno bene"), Claude's proposals for the details:
1. No email, only `username`.
2. `username`: lower case, 3 to 32 characters `a-z 0-9 - _`; Pico Auth lowers it before computing
   `login_hash`.
3. `login_hash`: Argon2id with costs fixed in Pico Auth's code, the same for ever; the server keeps a hash
   of it, not the value.
4. Token: does not expire; revoked by "Reset this device" on that device or by the author with a command.
5. Login attempts: at most 10 a minute per `username` (kept in memory: a restart of the server clears
   the counters).
6. API shape: generic files with version (`GET`, `PUT`, `DELETE`, list) on the same paths as GitHub
   (`vault.picoauth`, `requests/<id>.json`, `answers/<id>.json`, `computers/<id>.json`), so the shared
   code of Pico Auth does not change; plus endpoints for check, account, login and logout.
7. Invite code: 8 characters (`K7QM-3XPA`), valid once, no expiry.

## 9. The address: vault.glupo.dev/picoauth

**Decided by the author**, step by step:
- First `https://api.picoauth.glupo.dev` (a name of its own per service; `api.glupo.dev/picoauth` was
  proposed and dropped).
- At the deploy it turned out that the **Actalis certificate `*.glupo.dev`**, paid by the author and used
  for sso and schule (the author's choice, not Let's Encrypt), covers **one level only**: not
  `api.picoauth.glupo.dev`. Alternatives: Let's Encrypt for that name alone, or a second Actalis
  certificate. The author chose **a one-level name**. Proposed and rejected ("una peggio dell'altra"):
  `picoauth-api`, `picoauth-sync`, `pas`; then `totp`, `sync`, `pico`. Chosen: **`vault`**.
- A future **Pico Password** (an idea of the author, set aside) will have its vaults and APIs too: each
  app gets a **path prefix** on the same name, `/picoauth/...` now, `/picopassword/...` one day (the
  author: "nulla ci vieta di usare sempre vault.glupo.dev con degli endpoint diversi"). Caddy strips the
  prefix, so the server itself serves `/check`, `/files/…`.
- **No `v1/`** in the paths (explained and agreed): few users, and `/picoauth/v2` can be added if ever
  needed.
- `picoauth.glupo.dev` stays the presentation page (picoauth-sync.md 2.1).

The endpoints, as Pico Auth calls them (the server's own list is at the top of `src/web.rs`):
- without token: `POST /picoauth/check`, `POST /picoauth/accounts`, `POST /picoauth/login`;
- with `Authorization: Bearer <token>`: `POST /picoauth/logout`, `GET|PUT|DELETE
  /picoauth/files/{path}` (`DELETE …?version=`), `GET /picoauth/list?dir=`;
- `GET /picoauth/health` answers `ok`.
JSON; file content in base64; errors `{"error": "<code>"}` (`invalid_username`, `invite_not_valid`,
`username_taken`, `wrong_login`, `too_many_attempts`, `unauthorized`, `conflict`, `not_found`,
`invalid_path`).

## 10. Still open

- **The author tries block 3** (section 6), and with it block 2 with two devices, when the work PC is
  at hand. Without a second computer: a second Pico Auth on the Mac with its own data folder and keychain
  entries; offered on 2026-11-14, the author answered "not now" (do not build it unless asked). Docker was
  asked about and is not practical for a desktop app with a keychain.
- What happens on the server when the last device resets: account and vault kept or deleted (the
  "orphan vault", also open for GitHub, picoauth-sync.md 8). Today they stay, and there is no way out
  in the app: a new link to that account is refused ("already holds a vault") and "From glupo.dev"
  would wait for an approval for ever. By hand, the author can delete that account's rows in PostgreSQL
  on the VPS (`files`, then `tokens`, `accounts`); no command for it yet.
- **Moving a vault between GitHub and glupo.dev.** "Reset, then link to the other" does not work: reset
  deletes the local vault and the keychain secrets. A path is still to design.
- Account deletion: no flow, no command yet.
- To do before giving it to colleagues (section 7): privacy notice, AVV with Netcup, Impressum to check,
  server backups (PostgreSQL `picoauth`), the presentation page.
- Changing the master password (it changes `login_hash` too).
- **`picoauth revoke <username>` does not lock a device out**, found while checking this document, not
  discussed with the author: it deletes the user's tokens, but every linked device logs in again by itself
  with the master password in its keychain (section 5), so it only forces a new login. Cutting a device
  off would need something else (for example refusing that device's login): to discuss.
- Not discussed: `/check` and `/accounts` tell whether a user name exists (`username_taken`), and only
  the login is rate limited; with invites and few users this was judged acceptable by Claude, not by the
  author.
- Two linked devices approving the same request at the same time: the second answer is refused by the
  server (a write without version on an existing file is a conflict); not tested, same as on GitHub.
- Labels of the new menus and messages: written by Claude, the author saw the Settings ones.
- The GitHub test of picoauth-sync.md 13 (Windows PC) cannot be done as written any more: the Mac is no
  longer linked to GitHub (reset on 2026-11-14 to try glupo.dev, its vault was a test vault). The
  encrypted `vault.picoauth` left in `genlupo/picoauth-vault` cannot be opened by anyone; keep it, delete
  the file or the repository: the author's choice.

## 11. What was built (2026-11-14)

### picoauth-server (`~/Projects/glupo-dev/picoauth-server`)

- Rust, axum + sqlx like idp-rs, one crate. `src/web.rs` (the API, listed at the top), `src/db.rs`
  (queries), `src/names.rs` (rules: user names, invite codes, paths, random tokens and versions),
  `src/ratelimit.rs`, `src/main.rs` (commands). Tables in `migrations/`: `accounts` (username,
  `login_sha`), `invites` (code, used_at, used_by), `tokens` (SHA-256 only, last_used), `files`
  (account, path, content, random version).
- Commands: `serve`, `invite`, `invites`, `revoke <username>`. Settings from the environment or `.env`:
  `DATABASE_URL`, `PICOAUTH_BIND_ADDR` (default `127.0.0.1:8686`; 8585 is taken by dashboard-rs on the
  Mac, idp-rs uses 8484, schule-server 8787).
- Tests: `cargo test` with `DATABASE_URL` set (sqlx makes a fresh database per test); unit tests for the
  rules. On the Mac the local database is `picoauth` (used for tests and the first local try).
- `deploy/`: `package.sh` (on the Mac: static Linux binary with cargo-zigbuild/musl, and
  `dist/picoauth-server.tar.gz`), `install.sh` (on the VPS, idempotent), `picoauth-server.service`,
  `picoauth.env`, `picoauth` (the commands wrapper).

### Pico Auth (`cmd/picoauth`)

- `glupo.go`: the client of picoauth-server; `glupoServer = "https://vault.glupo.dev/picoauth"`
  (`PICOAUTH_SERVER` overrides it, only for tests against a local server); `loginHash`; the token in
  the keychain; the automatic login again; `remotes`, which sends every file operation to GitHub or to
  glupo.dev by the "repository" of the link (`glupo.dev/<username>` → glupo.dev; a GitHub owner cannot
  contain a dot), so `save.go`, `join.go`, `computers.go`, `forget.go` work for both unchanged.
  (Claude had proposed a "storage interface"; `remotes` is how it was done.)
- `glupolink.go`: Settings › Sync with glupo.dev (section 5) and first start › Open an existing vault ›
  From glupo.dev (section 6: `glupoJoinForm`, `glupoJoin`, then `join` in `join.go`; `leftJoin` there
  forgets the token of a device that did not get in).
- Messages name the service of the link (`where`).
- Tests: `glupo_test.go` (a fake server in memory: link, wrong invite, change, token revoked, offline,
  reset, link again after a failed upload; add a device, refuse it, no vault yet, esc withdraws),
  `glupo_real_test.go` (link, change, a second device approved, reset, against a real server; skipped
  unless `PICOAUTH_SERVER`, `PICOAUTH_TEST_INVITE`, `PICOAUTH_TEST_USER` are set; passed against the
  server running on the Mac). `go test -race ./cmd/picoauth`; `./build.sh` builds every app into `dist/`.

### On the VPS `glupo` (done with the author, step by step)

1. **DNS** at OVHcloud (Web Cloud › Domain names › glupo.dev › DNS zone): `vault` A `62.83.43.122` and
   AAAA `2a0a:4cc0:60:3377:74d3:dbff:fefc:faf`, TTL 300.
2. **PostgreSQL**: system user `picoauth` (nologin, no home), role `picoauth`, database `picoauth` owned
   by it, peer access through the socket: `DATABASE_URL=postgres://picoauth@%2Frun%2Fpostgresql/picoauth`
   (sqlx refuses an empty host: the socket folder goes in the host, `/` as `%2F`).
3. **Server**: package copied with `scp … glupo:~/`, unpacked, `sudo sh picoauth-server/install.sh`:
   binary `/usr/local/bin/picoauth-server` (the previous one as `.prev`), wrapper `/usr/local/bin/picoauth`,
   settings `/etc/picoauth/picoauth.env` (root:picoauth 0640, written only the first time), service
   `picoauth-server` (systemd, user `picoauth`, hardened), on `127.0.0.1:8686`.
4. **Caddy**: block `vault.glupo.dev` at the end of `/etc/caddy/Caddyfile` (copy before:
   `Caddyfile.prima-di-vault`), with the Actalis certificate files of sso (`tls
   /etc/caddy/certs/sso.glupo.dev.fullchain.pem /etc/caddy/certs/sso.glupo.dev.key.pem`, so **renewing
   that certificate before 2027-05-15 covers vault too**), `handle_path /picoauth/*` → `reverse_proxy
   127.0.0.1:8686`, anything else 404.
5. Checked from the Mac: `/picoauth/health` ok in IPv4 and IPv6, certificate `*.glupo.dev` Actalis,
   http → https; the author's Mac linked as `gennaro`.

Author's commands on the VPS: `picoauth invite`, `picoauth invites`, `picoauth revoke <username>` (the
wrapper runs the binary as user `picoauth` from `/`, with `/etc/picoauth/picoauth.env`; `sudo` asks the
password of gennaro). Logs: `sudo journalctl -u picoauth-server`. Update: `deploy/package.sh` on the Mac,
`scp dist/picoauth-server.tar.gz glupo:~/`, then on the VPS `tar -xzf picoauth-server.tar.gz && sudo sh
picoauth-server/install.sh`.

Mistakes met at the deploy, already fixed in the project: the socket URL form (above), and the wrapper
starting in the home folder, which user `picoauth` cannot read (`.env` lookup): it now starts from `/`.
The VPS infrastructure notes (`~/Projects/glupo-dev/netcup/CLAUDE.md`) do not list `vault.glupo.dev`
yet: offered to the author, not done.
