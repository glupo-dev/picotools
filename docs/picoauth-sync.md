# Pico Auth: sync between computers

How the same Pico Auth vault is kept on several computers (for example a Mac at home and a
Windows PC at work). This is the full design discussed with the author on 2026-11-13, with the
reasons behind each choice, so that work can continue from here without the original discussion.
Not everything here was agreed: the status of each point is the label it carries.

**No code is written yet.** On GitHub everything is ready (2026-11-14): the GitHub App is registered
(section 2.1), the author's repository is created and the app is installed on it (section 2.2).
Today Pico Auth keeps the vault on one computer only
(`picoauth/vault.picoauth` in the app settings folder). Export and restore were removed from the code on
2026-11-14 (section 8).

Each point is marked:
- **Decided**: agreed with the author.
- **Proposed**: a detail suggested during the design that the author did not discuss (silence, not
  agreement): to confirm with the author when building.
- **Open**: not decided yet (listed at the end).

---

## 1. Goal and principles

- **Decided.** The goal is the same vault on several computers **at the same time**, always aligned
  (not only moving it now and then).
- **Decided.** Sync is **optional**. Without GitHub, Pico Auth works standalone on one computer, as today;
  without the backup file (removed, section 8) a standalone vault stays on its computer.
- **Decided.** The vault lives in a **private GitHub repository** of the user (for example `picoauth-vault`).
  The repository holds only the encrypted vault (plus the small files of section 6).
- **Decided.** Pico Auth **does not use git**. It talks to GitHub directly over HTTPS (the GitHub API).
- **Decided.** Each computer works on its **local copy** of the vault; the repository is only used
  to align the copies.

### Why GitHub, and the alternatives that were dropped

| Alternative | Why it was dropped |
|---|---|
| A synced cloud folder (Dropbox, OneDrive, iCloud, Google Drive) | It would be the simplest (Pico Auth only reads a local file), but the author has none of them on both computers and does not want them. |
| The author's own VPS | A small server to write, secure and maintain; the work network may block it. |
| git (clone + pull/push) | The author does not want Pico Auth to depend on git. |
| A GitHub token created by hand on each computer | Too contorted: create it on the site for each computer, copy it, paste it, create a new one when it expires. Replaced by the GitHub App login (section 2), where nothing is created or copied by hand. |

Any place that holds a file would work in principle (the vault is encrypted, so the place never
reads the accounts). GitHub was chosen because the author already uses it, access can be limited
to one repository, it detects two computers saving at the same time, and it is usually reachable
from work networks.

---

## 2. Access to GitHub: the GitHub App

A program that talks to a private repository without git must prove it acts for the user.
Instead of tokens created by hand, Pico Auth is a **GitHub App** and each computer logs in from the browser.

### 2.1 Register the app: once, by the author of Pico Auth

**Done on 2026-11-14.** The app is registered by the author, owned by the organization **glupo-dev**
(not the personal account genlupo), each field chosen by the author after the explanation below.

| | |
|---|---|
| Name | **Pico Auth** |
| Owner | `glupo-dev` (organization) |
| Public page | `https://github.com/apps/pico-auth` |
| Client ID | `Iv23liUsuaDQViHcGo3m` (goes into the Pico Auth code) |
| App ID | `5757134` (not used by Pico Auth) |

How to get there in the browser (signed in as genlupo): avatar top right → **Organizations** →
**Settings** next to glupo-dev → at the bottom of the left menu **Developer settings** → **GitHub Apps**
→ Pico Auth. (A new app is made from the same page with **New GitHub App**.)

Each field, what it is for and what was chosen:
- **GitHub App name**: Pico Auth. Names are unique on GitHub; the public page link comes from it.
- **Description**: "Keeps the Pico Auth vault in sync between your computers, through a private
  repository of yours." Shown to whoever installs the app.
- **Homepage URL**: `https://picoauth.glupo.dev`. It is the "website" link on the app's public page and
  on the login screen: it tells a user who does not know the app what it is and who makes it, before
  giving it access. Pico Auth never uses it. **To do when other users arrive (section 9)**: put a
  presentation page there (what Pico Auth is, who makes it, how to use it, not technical). Until then
  the address shows whatever the `*.glupo.dev` server answers for an unknown name. The field can be
  changed at any time.
- **Enable Device Flow**: on. It is the login of 2.3 (code typed on `github.com/login/device`), made
  for programs on a computer.
- **Redirect URI**, **Allow wildcard matching**, **Request user authorization (OAuth) during
  installation**: empty / off. They are the login with redirect, made for websites.
- **Expire user authorization tokens**: on. The access expires after about 8 hours and Pico Auth renews
  it with the refresh token; an access stolen from a computer lasts little.
- **Setup URL**, **Redirect on update**: empty / off. They send the user to a page of the app after
  installing; Pico Auth has nothing to set up there.
- **Webhook**: off (Active unchecked). A webhook is GitHub sending messages to a server of the app;
  Pico Auth has no server and reads the repository by itself (section 7).
- **Permissions**: only **Repository → Contents: Read and write**, plus **Metadata: Read-only**, which
  GitHub adds to every app (mandatory). Everything else "No access". The fewer the permissions, the less
  a stolen access can do: only the files of that repository.
- **Subscribe to events**: none (they are the webhook messages).
- **Where can this GitHub App be installed**: **Any account**. With "Only on this account" it could be
  installed only on glupo-dev: the author's vault could not be in a personal repository
  (for example `genlupo/picoauth-vault`) and other users could not use it. Any account shows nothing of
  one user to the others.

Credentials (left menu → Credentials):
- **Key pairs**: GitHub requires a private key before the app can be installed, so one was generated
  (a `.pem` file). The private key lets the app act **as itself** on every repository where it is
  installed: it is for apps running on a server. **Pico Auth does not use it**: it always acts for the
  user, through the Device Flow login. The `.pem` file is a secret: keep it off the disk (in 1Password)
  or delete it; a new one can be made with **New key**. The SHA256 string shown is only the fingerprint
  that tells keys apart.
- **Client secrets**: none, keep it at 0. It is a password of the app for the website login; Device Flow
  does not need it, and a program given to others cannot keep a secret anyway.

- The **Client ID** is not a secret and gives no access by itself: it only tells GitHub which app asks
  for the login. It is free.

The public page `github.com/apps/pico-auth` shows the name, the author (glupo-dev), the description,
the homepage link and the permissions. It shows nothing about who installed it or their repositories.

### 2.2 Create the repository and install the app: once, by each user (decided)

1. Create a **private** repository, for example `picoauth-vault`.
2. On the app page click **Install** → **Only select repositories** → that repository.
   "Install" here means giving the app access to that repository: nothing is installed on a computer.
   Pico Auth can show or open the link of the app page.

**Done by the author on 2026-11-14:**
- Repository **`genlupo/picoauth-vault`**, in the personal account genlupo (the author's choice: the
  vault holds personal codes, it is not part of the project; it is also how another user will do it).
  **Private**, with a README only (a README does not count as a vault, section 5), no .gitignore,
  no license. Created from github.com: **+** top right → **New repository**.
- App **installed** on genlupo with **Only select repositories** → `picoauth-vault`.
  "All repositories" would let Pico Auth read and write every repository of the account (and future
  ones): an access stolen from a computer could touch all of them. With one repository it can touch
  only the encrypted vault, and Pico Auth can find the repository by itself (2.3).
  Done from the app settings (glupo-dev → Developer settings → GitHub Apps → Pico Auth → **Install App**
  → Install next to genlupo); another user does it from the public page `github.com/apps/pico-auth`.
- The installation page, to find it again: avatar → **Settings** → **Applications** (under
  Integrations) → **Configure** next to Pico Auth. From there: change the repositories the app sees;
  **Suspend** (blocks the app's access, can be undone); **Uninstall** (removes the app: no computer can
  read or write the vault on GitHub any more).

### 2.3 Link a computer: browser login (decided)

1. In Pico Auth the user chooses to link GitHub.
2. Pico Auth shows a short code (like `ABCD-1234`) and opens `github.com/login/device` in the browser.
3. The user logs in to GitHub as usual (password and GitHub two-factor), types the code and clicks **Authorize**.
4. GitHub gives Pico Auth an access valid only on the repositories where the app is installed.
   Pico Auth keeps it in the system keychain (Keychain on Mac, Credential Manager on Windows).

- Fact (from GitHub, not a choice), to handle when building: GitHub App user access normally expires after about 8 hours and is
  renewed by Pico Auth with a refresh token, without the user. Only if the refresh token also expires
  (months) the browser login is asked again.

- **Proposed.** Pico Auth finds the repository by itself, asking GitHub where the app is installed.
  If it is installed on more than one repository, it asks which one to use.

---

## 3. Security: master password + second key

The question was a kind of two-factor to open the vault file. A one-time code (TOTP) cannot protect
a file: nobody checks the code, whoever has the right key opens it. Only secrets that go into the
encryption key count. A typed "secret key" was **rejected** by the author (it is a second password).
The design follows 1Password, where the second secret moves between devices by itself.

- **Decided.** The vault on GitHub opens only with the **master password + a second key**.
- **Decided.** The second key is random (not a password), created by Pico Auth, kept in the keychain
  of each linked computer, and **never on GitHub**. The user never sees or types it.
- **Decided.** A new computer gets the second key when **an already linked computer approves it**
  (section 6), after comparing a 6-digit code on both screens.
- Explanation (not a choice): on a linked computer both secrets are in its keychain, so there the protection is the computer's
  login, as today. The second key protects **the copy on GitHub**: whoever takes that file cannot open it,
  even knowing or guessing the master password.

What an attacker gets:

| The attacker has | Result |
|---|---|
| The user's GitHub account (password + GitHub two-factor) | Can download the file: encrypted, useless without master password and second key. A request for a new computer made by him shows a code that does not match any screen of the user, who refuses it. |
| The GitHub access stolen from one computer | The same: only that repository, only the encrypted file. That access works without GitHub two-factor, so GitHub is only where the file is, not a second security layer. |
| One unlocked linked computer | Everything, as today with a standalone vault. |

Things to know:
- **Proposed.** A strong master password is still needed (for example generated by 1Password):
  advice to the user, not a check made by the app.
- Fact (consequence of GitHub, not a choice): every upload is a commit, the repository keeps **all old versions** of the vault, all encrypted.
  If the master password or the second key is ever changed, old versions still open with the old
  password and the old second key. This cannot be avoided with GitHub.

---

## 4. First start of Pico Auth on a computer

Shown only the first time, when the computer has no vault yet; then Pico Auth opens on the list of accounts.
The old first screen (`1 New vault / 2 Restore a backup`) was found confusing: "restore" in particular.

- **Decided** (the author proposed this structure): two choices, then the sources on a second page.

```
1  New vault
2  Open an existing vault
```

  and choosing 2:

```
1  From GitHub
```

- **Decided (2026-11-14).** The second page stays even with GitHub **alone** on it (the backup file was
  removed, section 8): the first menu must stay the same, and a source can be added there later if
  the backup ever comes back.

- **Open.** The labels are to be improved.

---

## 5. Flow: link the first computer (it already has a local vault)

The author's case: the Mac already has a vault with its accounts, and the repository is new and empty.

Before: the repository exists and the app is installed on it (2.2).

1. The user starts the link (**open**: from where, which key and screen; a key `s` in the account list was suggested).
2. Browser login (2.3).
3. Pico Auth finds the repository and checks that it holds **no vault** (a README or other files do not count).
4. It creates the second key and keeps it in the keychain.
5. It encrypts the vault with master password + second key and uploads it.

After: the vault is on the Mac and on GitHub. On the Mac nothing changes in everyday use.
The Mac is the first linked computer and can approve the others.

**Decided rule: a local vault is linked only to a repository with no vault.**
If the repository already holds a vault, Pico Auth stops with a clear message and does nothing.
Two vaults are never merged. (A merge was discussed and dropped: git cannot merge an encrypted file,
the two vaults have different passwords and keys, and the user's intention would be ambiguous.)

Who has a local vault and wants to use the one already on GitHub has two ways:
- link the local vault to a **new empty repository**, or
- **delete the local vault**, then use 2 → From GitHub (section 6).

**Decided.** A local vault can be deleted **only if it is not linked to GitHub**,
confirming by typing `delete` and the **master password of that vault**.

**Decided (2026-11-14): unlink a computer.** For example the last day at a job, with the work PC still
in front of the user. A linked computer holds the **local copy** of the vault and, in its keychain, the
**master password**, the **second key** and the **GitHub access**. "Unlink this computer":
1. asks confirmation by typing `delete` and the master password (as deleting a local vault above);
2. deletes the local copy of the vault;
3. deletes master password, second key and GitHub access from the keychain.

The vault on GitHub and the other computers do not change. Afterwards the computer is like new: at the
next start Pico Auth shows the first-start menu (section 4). So a linked computer can also get rid of
its vault: by unlinking.

The case of a computer **no longer in the user's hands** (lost, stolen, already given back) is rare
and, as the author decided, **not handled for now**: there the four things stay on that computer.

**Removed from the open points (2026-11-14): "2 → From GitHub" on a repository with no vault.**
It looked like a case to design, but it is not a real one. A user who wants sync starts it from
Pico Auth on the computer that has the vault ("link to GitHub", this section): Pico Auth guides them to
create the repository and install the app (2.2), then logs in and uploads the vault at once. The
repository is born inside the link, so the user cannot "forget" to upload the vault. It can only happen
if the link breaks halfway (for example the connection drops before the upload): a plain error, where
the computer with the vault says the link did not finish and must be done again; on another computer
"From GitHub" just says that no vault was found.

---

## 6. Flow: add another computer

**Decided**: the flow below, steps 1 to 11, as explained to the author step by step and accepted
(approval from a linked computer, 6-digit code compared on both screens, second key passed through the
repository, the new computer asks the master password once).
**Proposed**: the technical details inside the steps (key pair, file names, timings, deleting request
and answer).

For example the PC at work, when the vault is already on GitHub (put there by the Mac).
The two computers never talk to each other: everything passes through files in the repository,
which each Pico Auth reads when it opens and from time to time while it is open.

### On the new computer
1. First start → **2 Open an existing vault → From GitHub**.
2. Browser login (2.3), with the user's GitHub account.
3. Pico Auth downloads the vault but **cannot open it**: it has no second key.
4. It creates its own key pair (the private part stays in its keychain) and writes in the repository
   a **request**: a small file with its public key (**proposed**: for example `requests/<id>.json`).
5. It shows a **6-digit code** computed from that public key, for example `482 913`,
   and waits, checking the repository (**proposed**: about every 10 seconds, as in section 7).

### On an already linked computer (any one, not only the Mac)
6. When Pico Auth opens (or at its next check while open) it finds the request and shows:
   "A new computer asks for access, code 482 913. Approve?"
7. The user checks that the code is **the same** as on the new computer's screen, presses `y` and
   types the **master password** (**decided 2026-11-14**).
   The code is there because whoever gets into the GitHub account could write a fake request:
   its code would be different from the one on the user's screen, and the user refuses it.
   The master password is there because a linked computer opens by itself: without it, whoever sits at
   it while it is unlocked could approve a request with `y` alone and give the second key away. It is
   typed only when a computer is added (rarely), and the everyday use (opening, copying codes) stays
   without password. The author's words: "non siamo alla NASA", but one password per device is no drama.

   **Refusing (decided 2026-11-14)**, with `n`: because the code is different (a fake request) or
   because the user changed their mind. The linked computer **deletes the request** from the
   repository, so it does not come back. The new computer, at its next check, finds its request gone,
   shows "Request refused or expired" and goes back to the first-start menu; if it really was the
   user's computer, they do "From GitHub" again from the start.
8. Pico Auth encrypts the second key with the new computer's public key and writes the answer
   in the repository. Only the new computer can read it.

### Back on the new computer
9. It finds the answer, decrypts the second key with its private key and keeps it in its keychain.
10. It deletes request and answer from the repository (**proposed**).
11. It asks the **master password once**, opens the vault and keeps the password in its keychain.
    From now on it opens by itself, like the Mac.

---

## 7. Everyday use and conflicts

- **Decided.** Pico Auth always works on the **local copy**: the codes are computed from it, also
  without internet.
- **Decided.** When it opens, and from time to time while it is open (**proposed**: about every 10 seconds),
  it reads the repository: a newer vault (downloaded into the local copy) and requests from new computers
  (shown for approval). When Pico Auth is closed it reads nothing (explained to the author, no objection).
- **Decided.** Every change (add, rename, delete an account) is saved locally and **uploaded at once**.
  The other computers see it at their next opening or check.
- **Decided (2026-11-14).** **Without internet, changes are blocked**: Pico Auth says there is no
  connection and changes nothing. The codes keep working offline (they come from the local copy).
  Why (the alternative was "save the change and upload it later"): adding an account happens while
  turning on two-factor on a website, so online anyway; rename and delete are rare and can wait.
  Every change is on GitHub as soon as it is made, so the other computers never fall behind and no
  pending changes pile up to be reconciled after hours or days (where "renamed on one, deleted on the
  other" would happen more often). No queue of changes to keep on disk. If editing offline is ever
  really needed, "upload later" can be added without throwing anything away.
  Consequence: the change to retry after a conflict (below) is kept only in memory during the upload.

**Decided** (explained and accepted). Conflicts, when two computers save at almost the same time:
1. Pico Auth uploads saying to GitHub "I replace the version I read".
2. If another computer uploaded a newer version meanwhile, **GitHub refuses**.
3. Pico Auth downloads the newer vault, **applies again the single operation** (add, rename or delete)
   on it, and uploads.

This works because changes are small and separate. The only ambiguous case is the same account changed
on two computers (for example renamed on one and deleted on the other): **proposed**, the last operation
wins (said to the author, not discussed).

Two computers already linked share the same vault, password and second key: a conflict is solved by
repeating one operation. This is different from section 5, where two independent vaults would meet.

---

## 8. Export and backup: removed

**Decided (2026-11-14): the whole Export is removed** from Pico Auth: the encrypted backup file, the
plain `otpauth://` list, "restore a backup" at the first start and "From a backup file" in the new
first-start menu (section 4). It may be added again if a real need appears.

Why. The author: "o sei uno che usa in locale o sei uno che usa GitHub", and nobody makes exports
regularly ("i password manager servono proprio a questo"). The uses were listed one by one and judged:
1. A standalone user moving to a new computer (one-time, so no need to remember it): realistic, but
   standalone users do not interest the author; they can link GitHub to move.
2. A computer breaks without sync: the backup helps only if recent, and nobody keeps it recent.
3. With sync, all linked computers lost together (the backup was the "emergency kit"): a backup made
   maybe a year earlier gives back only part of the accounts; the author: nobody uses a USB stick any more.
4. The vault file damaged by a bug: rare, and with sync there is the copy on GitHub and on the other
   computer.
5. The `otpauth://` list to leave Pico Auth for another app (the way out that Authy lacks): judged
   unrealistic by the author.
6. The list to have the same codes on a phone too: the two copies would not stay aligned.

Consequences (facts, said to the author):
- With sync, the safety is having **at least two linked computers**. If **all** of them are lost, the
  vault on GitHub cannot be opened (the second key was only on them): the accounts are lost and must be
  set up again site by site. Rare case, accepted.
- A standalone vault cannot be moved to another computer, except by linking it to GitHub.

---

## 9. Other users

- **Decided** (this whole section was explained to the author and accepted). Pico Auth can be given to
  others (for example "Mario"), each with their own vault.
- Mario needs only a GitHub account: he creates his own private repository and installs the same
  public app on it (2.2). He registers nothing. Then he links his computers with his own login.
- GitHub gives Mario's Pico Auth access only to his repository: he does not see the author's, the author
  does not see his. Each has their own master password and second key.
- Who does not want GitHub uses Pico Auth standalone (on one computer only, section 8).
- **To do before giving it to others**: the presentation page at `https://picoauth.glupo.dev`, the
  Homepage URL of the app (see 2.1).

---

## 10. Rejected: do not propose again

Sync and storage:
- Cloud-drive folders (Dropbox, OneDrive, iCloud, Google Drive) and the author's VPS: "piuttosto GitHub
  con tutte le sue contorsioni".
- git (clone, pull, push): Pico Auth does not use git.
- GitHub tokens created by hand on each computer: too contorted (replaced by the GitHub App login).
- A typed "secret key" as second factor: it is a second password.
- Merging a local vault with a vault already on GitHub, or asking the user whether to replace one of
  the two: a local vault goes only to a repository with no vault, "non si può fare, punto".

First start:
- No first-start menu at all, with restore and GitHub as keys of the account list
  (`n` / `r` / `g`, or restore inside the Export screen `e`): the same menu with letters, and the empty
  list cannot reach `e` anyway.
- The old first screen `1 New vault / 2 Restore a backup`: "restore" alone, on the first screen, is
  not understood.

---

## 11. Code today (what sync will change)

All in `cmd/picoauth`:
- `vault.go`: the vault file. A JSON envelope with the Argon2id parameters and salt, a nonce, and the
  accounts encrypted with AES-256-GCM; the key comes from the master password only. The master password
  is kept in the keychain (`go-keyring`, service `picoauth`). Sync needs the key to come from master
  password + second key.
- `model.go` / `render.go`: the screens. The first start is `screenStart` / `renderStart`
  (today only `1 New vault`, to become the menu of section 4). The account list is `screenList`.
- `totp.go`: the codes and the `otpauth://` parsing. Not touched by sync.
- `form.go`: the question forms (copied from Pico API). `main.go`: window, clipboard, the 1-second tick.
- Keys of the list today: `n` new, `m` rename, `d` delete (only when there are accounts),
  `enter`/`1-9` copy a code, `↑/↓` move, `esc`/`q` quit.
- Tests: `totp_test.go`, `vault_test.go`, `model_test.go`. `./build.sh` builds every app into `dist/`.

---

## 12. Still open

- The **key and screen** to link an existing local vault to GitHub (section 5).
- The **labels** of the first-start menu (section 4).
- The **key and screen to delete** a local vault and to **unlink** a computer (the rules are decided in
  section 5, not the place).

Technical points never discussed with the author (to decide when building, do not present as decided):
- How the master password and the second key are combined into the vault key (for example Argon2id on
  the password, then HKDF with the second key), and whether the **local copy** on a linked computer uses
  the same key as the copy on GitHub (this affects sections 5 and 7 and the delete rule).
  The file envelope version goes from 1 to 2.
- File names in the repository: the vault (for example `vault.picoauth`), requests and answers
  (for example `requests/<id>.json`, `answers/<id>.json`); how the 6-digit code is computed from the
  public key (for example a hash of it).
- **Changing the master password**: today there is no such function; with sync the other computers
  would keep the old password in their keychain and must ask the new one.
- Old or unanswered requests: when they are deleted. Two linked computers approving the same request
  at the same time. What the new computer does if no linked computer is open (wait, timeout).