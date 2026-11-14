# Pico Auth: sync between computers

How the same Pico Auth vault is kept on several computers (for example a Mac at home and a
Windows PC at work). This is the full design discussed with the author on 2026-11-13 and 2026-11-14, with the
reasons behind each choice, so that work can continue from here without the original discussion.
Not everything here was agreed: the status of each point is the label it carries.

**State on 2026-11-14: the whole sync is written** (code in `cmd/picoauth`, section 11), in four blocks
(section 13); the code is pushed to `main`.
- Block 1, link the first computer: **tried by the author** with the real GitHub. The author's Mac is
  linked: its vault is in `genlupo/picoauth-vault`.
- Blocks 2 (add another computer), 3 (keep them aligned) and 4 (delete, unlink, linked devices): tested
  only against a fake GitHub in the tests. **Not tried for real**: the author will try them **all
  together** when the Windows PC of work is at hand (the test list is in section 13).
- On GitHub everything is ready: the GitHub App is registered (section 2.1), the repository is created
  and the app is installed on it (section 2.2).
- Export and restore were removed from the code (section 8).
- A **second sync through glupo.dev**, next to this one, is designed (not built) in
  [picoauth-glupo-sync.md](picoauth-glupo-sync.md).

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
| The author's own VPS | Dropped at first: a small server to write, secure and maintain; the work network may block it. These reasons were Claude's, not the author's (the author called the network one nonsense on 2026-11-14). **Reopened on 2026-11-14**: a sync through glupo.dev, next to GitHub, so that people who do not know GitHub can use it: [picoauth-glupo-sync.md](picoauth-glupo-sync.md). |
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
   Pico Auth names the app page when it can reach no repository (written).

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
   **Enter copies the code** to paste it on GitHub (asked by the author after the first try, when they
   had to select it with the mouse).
3. The user logs in to GitHub as usual (password and GitHub two-factor), types the code and clicks **Authorize**.
4. GitHub gives Pico Auth an access valid only on the repositories where the app is installed.
   Pico Auth keeps it in the system keychain (Keychain on Mac, Credential Manager on Windows).

- Fact (from GitHub, not a choice), written in `github.go`: GitHub App user access normally expires after about 8 hours and is
  renewed by Pico Auth with a refresh token, without the user. Only if the refresh token also expires
  (months) the browser login is asked again.

- **Proposed, and written.** Pico Auth finds the repository by itself, asking GitHub where the app is
  installed. If it is installed on more than one repository, it asks which one to use.

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

1. The user starts the link from the **Settings** page (5.1): key `s` in the account list → Settings →
   `1 Sync with GitHub`.
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
(Written in block 4 as **Reset this device**, in Settings: first called "Unlink this device", renamed on 2026-11-14 by the author because "unlink non significa delete", while the action deletes the vault and its accounts from the device. The description under it: "The vault stays on GitHub". The behaviour did not change. Added in the code, **proposed**: it also
deletes the file of this device from `computers/` on GitHub (5.1), so a device **offline cannot unlink**;
the author was not asked about this.)

The vault on GitHub and the other computers do not change. Afterwards the computer is like new: at the
next start Pico Auth shows the first-start menu (section 4). So a linked computer can also get rid of
its vault: by unlinking.

The case of a computer **no longer in the user's hands** (lost, stolen, already given back) is rare
and, as the author decided, **not handled for now**: there the four things stay on that computer.

**Removed from the open points (2026-11-14): "2 → From GitHub" on a repository with no vault.**
It looked like a case to design, but it is not a real one. A user who wants sync starts it from
Pico Auth on the computer that has the vault ("link to GitHub", this section): they create the
repository and install the app (2.2; if the app reaches no repository, Pico Auth says so and names the
app page), then Pico Auth logs in and uploads the vault at once. The
repository is born inside the link, so the user cannot "forget" to upload the vault. It can only happen
if the link breaks halfway (for example the connection drops before the upload): a plain error, where
the computer with the vault says the link did not finish and must be done again; on another computer
"From GitHub" just says that no vault was found.

### 5.1 The Settings page

**Decided (2026-11-14): the Settings page.** Pico Auth has no home page: it opens on the account list,
because it is almost always opened to copy a code. The keys of that list (copy, new, rename, delete) are
everyday actions on one account; linking to GitHub is a rare action on the whole vault and must not sit
next to them. So the list has **one key, `s`, that opens a separate page, Settings**, with the rare
administration operations, among them the link to GitHub. The name: "Vault" was proposed and rejected by
the author, because 99% of people do not know what a vault is; "Settings" is the name people look for
when they want to change something of the app.

**Decided (2026-11-14): content of Settings.** `esc` goes back to the account list, which is the home of
Pico Auth. Delete and unlink go in Settings too (decided 2026-11-14, with the labels below); one of
the two is shown, depending on the link:

Not linked (left) and linked (right); a blank line separates the items, each with a short grey
description under it (the author asked for the blank line):

```
Settings                                Settings

  1  Sync with GitHub                   Synced with GitHub: genlupo/picoauth-vault
  2  Delete this vault
                                          1  Reset this device
                                          2  Linked devices
```

Both ask to type `delete` and the master password (the rules in this section). Written in block 4: both remove
from this computer the vault file, `sync.json`, and from the keychain the master password, the second
key and the GitHub access; Pico Auth then shows the first-start menu. The vault on GitHub is not
touched.

**Decided (2026-11-14): Linked devices** (labels "Linked devices" and "Unlink this device", chosen by the author over "computers"; "Unlink this device" became "Reset this device", see section 5).
**Decided (2026-11-14): "device" everywhere.** Every text of Pico Auth says "device", never "computer":
screens, messages and the commit messages on GitHub (for example "Pico Auth: opened on a device").
Written. Not changed, because they are not texts the user reads in the app: the names in the code, the
folder `computers/` in the repository and the field `computer` of `sync.json` (renaming them would break
the vault already linked on the author's Mac). This document still says "computer" in its explanations. Settings shows the list of the computers linked to the
vault (name, system, since when), view only, like the devices page of Netflix but **without removing a
computer from another one**: each computer is unlinked only from itself. Why the list: the author knows
which computers have the vault (a name they do not know is an alarm), and Pico Auth knows when the
**last** computer is being unlinked, which was the reason: an "is this the only computer?" warning asks
the user something they may not know. Removing another computer was explained and left out: without
a server, a computer already has its copy and the second key; for one still in the user's hands it
would be possible, for a stolen one it would not help (the lost computer, above in section 5).

Written (2026-11-14), the details **proposed**: each linked computer writes `computers/<id>.json`
(name, system, date) after the link or the approval; a computer linked before the list existed writes
it at its next start. The name is the one the system gives (`os.Hostname`, without `.local`): on a Mac
usually the Local hostname of Sharing, on Windows the device name. Unlink reads the list first: if this is
the only device, the confirmation says "This is the only linked device: after this, nobody can open
the vault on GitHub any more" (a warning, not a block). Unlink deletes the file of this computer, so it
**needs GitHub**, like every change of a linked vault: without a connection nothing is changed.

**Decided (2026-11-14) by the author: "since" and "last".** Each device in the list shows
`since 14 Nov 2026   last today 16:52`: **since** is the day it was linked, **last** the last time
Pico Auth was **opened** on it, even only to copy a code, with no change. Each device writes the time in
its `computers/<id>.json` once at each opening (written: at the first check after opening, about a second
after the account list appears; the check runs only with the account list on screen, so an opening that
never shows the list writes nothing) (each write is a commit; at every 10-second check it
would be 360 commits an hour). A limit of once an hour was suggested and dropped by the author: one
commit per opening is little. If Pico Auth stays open for days, "last" is when it was opened. A
"last sync" (last change sent or received) was written first and replaced: the author wants to know
when a device used the program.

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
4. It creates its own key pair (written: the private part stays **only in memory**, so closing Pico Auth
   or pressing esc withdraws the request) and writes in the repository
   a **request**: a small file with its public key (**proposed**: for example `requests/<id>.json`).
5. It shows a **6-digit code** computed from that public key, for example `482 913`,
   and waits, checking the repository every 10 seconds (section 7). If no linked computer is open, it
   keeps waiting; a linked computer that opens later deletes requests older than 15 minutes, and the
   new computer then shows "Request refused or expired".

### On an already linked computer (any one, not only the Mac)
6. When Pico Auth opens (or at its next check while open, **only while the account list is on screen**,
   not in the middle of a form or in Settings) it finds the request and shows:
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
- **Decided.** When it opens, and from time to time while it is open (**every 10 seconds**, confirmed by
  the author on 2026-11-14 after asking if it was too often: it is one light call, about 360 an hour
  while open, against the 5,000 an hour GitHub allows),
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
  Confirmed again by the author on 2026-11-14: "no internet, no changes" is the price of a synced vault,
  changes are rare, and the **codes always work** (also with no internet or GitHub down). Note told to
  the author: the block happens whenever **GitHub** cannot be reached, so if the work network blocks
  `api.github.com` changes would never work on the work PC: the try of block 2 on that PC will show it.

**Decided** (explained and accepted). Conflicts, when two computers save at almost the same time:
1. Pico Auth uploads saying to GitHub "I replace the version I read".
2. If another computer uploaded a newer version meanwhile, **GitHub refuses**.
3. Pico Auth downloads the newer vault, **applies again the single operation** (add, rename or delete)
   on it, and uploads.

This works because changes are small and separate. The only ambiguous case is the same account changed
on two computers (for example renamed on one and deleted on the other). First **proposed** "the last
operation wins"; the code **does differently: the delete wins** (the rename says "the account was deleted
on another computer"), because accounts have no id and a deleted account cannot come back. Told to the
author on 2026-11-14 ("tell me if you prefer the other"), no answer: still **proposed**.

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
- **Open, never discussed:** a vault left on GitHub with no linked device (all unlinked or all lost) is
  an orphan: "From GitHub" would wait for ever (no one can approve), and the repository blocks a new
  link (it already holds a vault). Today the only way out is by hand on github.com: delete
  `vault.picoauth` (and `computers/`) from the repository, or use a new empty repository.

---

## 9. Other users

- **Decided** (this whole section was explained to the author and accepted). Pico Auth can be given to
  others (for example "Mario"), each with their own vault.
- Mario needs only a GitHub account: he creates his own private repository and installs the same
  public app on it (2.2). He registers nothing. Then he links his computers with his own login.
- GitHub gives Mario's Pico Auth access only to his repository: he does not see the author's, the author
  does not see his. Each has their own master password and second key.
- Who does not want GitHub uses Pico Auth standalone (on one computer only, section 8). Note
  (2026-11-14): with the backup removed, a standalone vault is lost if its computer breaks, so in practice
  the sync is also the safety copy; this is why a sync through glupo.dev is planned (below).
- **To do before giving it to others**: the presentation page at `https://picoauth.glupo.dev`, the
  Homepage URL of the app (see 2.1).
- **2026-11-14, in practice:** the GitHub setup (private repository, "install" the GitHub App) proved too
  hard even for the author, who could not redo it alone; for other people, such as colleagues, the
  author decided to add a sync through glupo.dev next to GitHub: [picoauth-glupo-sync.md](picoauth-glupo-sync.md).

---

## 10. Rejected: do not propose again

Sync and storage:
- Cloud-drive folders (Dropbox, OneDrive, iCloud, Google Drive): "piuttosto GitHub con tutte le sue
  contorsioni". The author confirmed on 2026-11-14 that this phrase was about these services only, not
  about the VPS (the VPS is now the planned second sync, [picoauth-glupo-sync.md](picoauth-glupo-sync.md)).
- git (clone, pull, push): Pico Auth does not use git.
- GitHub tokens created by hand on each computer: too contorted (replaced by the GitHub App login).
- A typed "secret key" as second factor: it is a second password.
- Merging a local vault with a vault already on GitHub, or asking the user whether to replace one of
  the two: a local vault goes only to a repository with no vault, "non si può fare, punto".

Settings and linked devices:
- The sync as a key in the account list next to copy/new/rename/delete: "sync non può stare lì" (5.1).
- "Vault" as the name of the Settings page: 99% of people do not know the word.
- A warning that **asks** "is this the only computer?" (the user may not know). Kept instead: the warning
  "This is the only linked device…" that Pico Auth shows when it **knows** it from the devices list (5.1).
- Removing (unlinking) a device from another one, as on Netflix: left out (5.1).
- "last sync" in the list (last change sent or received): replaced by "last" = last opening.
- Writing "last" at most once an hour: once per opening is fine.
- The **IP address** of the last use: the local IP says nothing, the public IP needs an outside service
  at each opening, against the README promise "Nothing goes to an online service you did not call
  yourself". The author: "lascia stare".

First start:
- No first-start menu at all, with restore and GitHub as keys of the account list
  (`n` / `r` / `g`, or restore inside the Export screen `e`): the same menu with letters, and the empty
  list cannot reach `e` anyway.
- The old first screen `1 New vault / 2 Restore a backup`: "restore" alone, on the first screen, is
  not understood.

---

## 11. The code

All in `cmd/picoauth` (Go, Fyne GUI through `picotools/internal/gui`):
- `vault.go`: the vault file. A JSON envelope: Argon2id parameters and salt, nonce, accounts encrypted
  with AES-256-GCM. Version 1: key from the master password only (a vault not linked). Version 2: key =
  HKDF-SHA256 of the Argon2id key with the second key. Keychain (`go-keyring`, service `picoauth`):
  entries `master password`, `second key`; `writeFile` writes through a temporary file.
- `github.go`: the GitHub client. Device Flow login with Client ID `Iv23liUsuaDQViHcGo3m`, the access in
  the keychain (entry `github access`) renewed with the refresh token (a lock, since calls run in several
  goroutines), the repositories where the app is installed, list a folder, read / write / delete a file
  with its sha (a write refused because the file changed gives `errConflict`).
- `sync.go`: the Settings page, the shared connect step (login + find the repository), the link of
  section 5, the new login when the access is refused (`relogin`), and `sync.json` next to the vault
  (repository, sha of the last version, id of this device).
- `pair.go`: requests and answers of section 6 (X25519, HKDF, AES-GCM), the 6-digit code.
- `join.go`: the new computer of section 6 (From GitHub, request, wait, master password) and, on a linked
  computer, the approval screen (`y` + master password, `n` refuses).
- `save.go`: section 7. Every change goes through `apply` (local only, or uploaded first when linked),
  conflicts, offline, and `checkRemote`, the 10-second check (newer vault, requests, device file).
- `computers.go`: the devices list of 5.1 (`computers/<id>.json`, since and last).
- `forget.go`: delete this vault and reset this device (unlink).
- `model.go` / `render.go`: screens and keys. First start `screenStart` with two pages (section 4).
  Keys of the list: `n` new, `m` rename, `d` delete, `s` settings, `enter`/`1-9` copy, `↑/↓`, `esc`/`q`.
- `totp.go`, `form.go`, `main.go`: codes and `otpauth://`, question forms, window and 1-second tick.
- Tests: `totp_test.go`, `vault_test.go`, `model_test.go`, `sync_test.go` (all the sync against a fake
  GitHub, no network; run with `go test -race ./cmd/picoauth`). `./build.sh` builds every app into `dist/`.

On the computer, in the app settings folder (`~/Library/Application Support/picoauth/` on Mac,
`%AppData%\picoauth\` on Windows): `vault.picoauth` (the local copy; on a linked computer the same bytes
as on GitHub) and `sync.json` (only when linked).

In the repository: `vault.picoauth` (root), `requests/<id>.json`, `answers/<id>.json` (only while a
computer is being added), `computers/<id>.json` (one per linked device). Every write is a commit made as
the user (for example "Pico Auth: vault changed", "Pico Auth: opened on a device").

---

## 12. Still open

- The **labels** of the first-start menu (section 4): written as `1 New vault`, `2 Open an existing
  vault`, then `1 From GitHub`, provisional.
- The conflict case "renamed here, deleted elsewhere" (section 7): the delete wins in the code, to confirm.
- The **presentation page** at `https://picoauth.glupo.dev` before giving Pico Auth to others (2.1, 9).
- **Changing the master password**: there is no such function; with sync the other computers would
  keep the old password in their keychain and must ask the new one. Never discussed.
- The **orphan vault** (section 8): never discussed; the only way out today is by hand.
- **Unlink needs GitHub** (section 5, 5.1): proposed and written, the author was not asked.

Technical points never discussed with the author (to decide when building, do not present as decided):
- **Proposed and written in block 3** (to confirm with the author):
  - A linked computer reads the **root of the repository** at start and every 10 seconds, only while
    the account list is on screen: one call gives the version (sha) of the vault and whether there are
    requests. A different sha means a newer vault: it is downloaded and becomes the local copy.
  - A change is uploaded replacing the sha read last; the local copy changes only when GitHub accepts.
    While it uploads, "Saving on GitHub…" is shown and other changes wait. After a conflict the change
    is applied again up to 5 times. An account is found again by its contents (name, user, secret,
    settings), since accounts have no id: renamed here and deleted on another computer gives "the
    account was deleted on another computer" (the delete wins, not "the last operation wins" written
    in section 7); deleted on both is fine.
  - Without GitHub (no connection, GitHub down): "No connection to GitHub: nothing was changed. The
    codes still work." The checks fail silently and try again.
  - If GitHub no longer accepts the access (the refresh too), a change opens the browser login again,
    then is saved.
- **Proposed and written in block 1** (to confirm with the author): the vault key is Argon2id on the
  master password, then HKDF-SHA256 with the second key (32 random bytes). The **local copy** of a
  linked computer is the **same file** as the one on GitHub (same key, same bytes). Envelope version 2
  means "needs the second key"; version 1 is a vault not linked. The vault is `vault.picoauth` at the
  root of the repository. The second key goes in the keychain **before** the upload, so a vault on
  GitHub always has its key somewhere; the local copy becomes version 2 only after the upload worked.
- **Proposed and written for the linked devices** (5.1): name from the network name of the computer
  (Mac: System Settings → General → Sharing → Local hostname, without `.local`; Windows: Settings →
  System → About → Device name), written once, when the device file is created (after the link or the
approval, or at the next start for a device linked before the list existed). Offered to the author: let the user type a
  name at the link (a Windows name like `DESKTOP-4F7K2QX` says little); no answer yet.
- **Proposed and written in block 2** (to confirm with the author): the request is
  `requests/<id>.json` (an X25519 public key and the time), the answer `answers/<id>.json` (the second
  key encrypted with AES-256-GCM, key from X25519 between a one-time key of the linked computer and the
  request, then HKDF-SHA256). The 6-digit code is the first 4 bytes of SHA-256 of the public key, modulo
  one million, shown as `482 913`. A linked computer checks the requests when the account list is on
  screen, at start and every 10 seconds; it deletes requests older than 15 minutes. The new computer
  keeps its private key only in memory: closing it or pressing esc withdraws the request. The new
  computer deletes request and answer once it has the second key.
- To check with the real GitHub: whether the access is renewed **without a client secret** (there is
  none, section 2.1). If GitHub refuses, Pico Auth asks the browser login again.
- Two linked computers approving the same request at the same time (both write an answer: the second
  write is refused by GitHub; not tested).
---

## 13. Building it in blocks

**Decided (2026-11-14).** The sync is written in blocks; each block ends with something the author can
try in the app:
1. **Link the first computer** (section 5): `s` → Settings → `1 Sync with GitHub`,
   browser login, the vault uploaded encrypted. **Written and tried by the author on 2026-11-14**
   with the real GitHub: the vault is in `genlupo/picoauth-vault`. If the app can reach
   more than one repository it asks which one (the proposal of 2.3).
   Not in block 1: changes made after the link are saved only on this computer, and go to GitHub
   from block 3.
2. **Add another computer** (section 6): first-start menu, From GitHub, approval with the 6-digit code.
   **Written 2026-11-14, to try** (it needs a second computer, for example the Windows PC). The labels
   of the first-start menu are provisional (section 12).
3. **Keep them aligned** (section 7): upload at every change, check every 10 seconds, offline changes
   blocked, conflicts. **Written 2026-11-14, to try.** On the Mac alone the author can try the upload
   (a commit "Pico Auth: vault changed" at every change) and the block without connection; changes
   arriving from another computer need the PC.
4. **Delete and unlink** (section 5). **Written 2026-11-14, to try.** Then the list of linked devices
   (5.1), written 2026-11-14.

**Next step: the author tries blocks 2, 3 and 4 together** with the Windows PC
(`dist/windows/picoauth.exe`), the Mac open on the account list:
1. PC: first start → `2 Open an existing vault` → `1 From GitHub` → browser login (enter copies the code).
2. PC shows a 6-digit code; Mac, within about 10 seconds, shows the request: same code → `y` + master
   password. PC asks the master password once and shows the accounts. (Also try `n`: the PC shows
   "Request refused or expired".)
3. A change on one (add, rename, delete) appears on the other within about 10 seconds.
4. With Wi-Fi off a change is refused with "No connection to GitHub", the codes still work.
5. Settings → `2 Linked devices` shows both, with since and last.
6. Unlink only **after** both are linked, and unlink the **PC**, not the Mac: the Mac is today the only
   linked device, unlinking it would leave nobody able to open the vault on GitHub. To use the PC again
   afterwards: "From GitHub" and the approval from the Mac again.
Also to watch: whether the work network lets the PC reach `api.github.com`, and whether GitHub renews
the access after about 8 hours without a client secret (if not, Pico Auth asks the browser login again).
