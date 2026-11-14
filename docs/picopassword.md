# Pico Password: preliminary analysis

A new picotools app, **Pico Password**: a place to keep passwords, "a kind of 1Password". Idea of the
author from 2026-11-13; this preliminary analysis was made with the author on 2026-11-14, step by step,
at a high level. Nothing is built yet.

Read first, for what is reused: [picoauth-sync.md](picoauth-sync.md) (the vault, the second key, the
approval of a new device, the GitHub sync) and [picoauth-glupo-sync.md](picoauth-glupo-sync.md) (the
sync through glupo.dev).

Each point is marked:
- **Decided**: agreed with the author.
- **Proposed**: suggested by Claude, not confirmed by the author (silence is not agreement).
- **Open**: not decided.

---

## 1. What it is

- **Decided** (the author's framing): "the equivalent of Pico Auth, but with passwords".
- **Decided** (Claude's description, which the author called "perfetto"):
  Pico Password is a safe place for your passwords. You open it and see the list of your logins; you
  find the one you need, copy the password and paste it into the site. When you sign up to a new site
  you add an item. The passwords are in an encrypted vault that opens only with your master password,
  the same vault on all your devices, always aligned: add a password on the Mac and you find it on the
  work PC. The sync goes through GitHub or glupo.dev, which see only encrypted data. In practice it is
  Pico Auth with passwords instead of codes: same simplicity, same security, same sync. No browser
  extension, no sharing with other people, nothing superfluous. For the author and the people they give
  it to (for example colleagues): who wants their passwords on several devices without a paid service.

## 2. Scope of the first version

- **Decided.** Start with the **base features only**: the list of credentials.
- **Decided.** **No browser integration** for now (no extension filling the login forms): only a vault
  where credentials are saved. You copy and paste.
- **Decided.** "Shared" means **synced between the user's own devices**, exactly as in Pico Auth.
  **Not shared with other people** (a vault in common with a colleague was explained: it would need a
  design of its own, since each person has their own master password; it is not wanted).
- A separate app, not passwords inside Pico Auth.

## 3. The data of an item

- **Decided.** The minimal fields are three:

  ```
  Name       Amazon
  User       mario@example.com
  Password   k9#Vq2!xLm…
  ```

  - **Name**: how you find the item in the list (it can contain the site, for example "amazon.it");
  - **User**: almost every site asks for it (email or user name);
  - **Password**: the reason the app exists.

  Without one of these three an item is useless.
- **Open: Site** (the author: "forse"). What it is for: the item is called "UniCredit" but the login
  page is not `unicredit.it`, it is an address you do not remember; Site keeps it there. When the
  address is obvious from the name, Site adds nothing. To decide later, when the author sees the app.
- **Decided: no Note** (free text, for example recovery codes).

## 4. Sync

- **Decided.** The same as Pico Auth: GitHub or glupo.dev, the vault encrypted, the second key, a new
  device approved by a linked one with the 6-digit code, Settings, Linked devices, Reset this device.
- **Decided.** On glupo.dev "exactly as for Pico Auth" (account with user name and invite code, the
  master password is also the account password, the server sees only encrypted data), at
  `https://vault.glupo.dev/picopassword` (the path prefix planned in picoauth-glupo-sync.md 9).

## 5. One glupo.dev account and one master password for both apps

The question came from how Pico Auth works: **the password of the glupo.dev account is the master
password of the vault**. With two apps there are two vaults, so either:
1. **one account, one master password** for Pico Auth and Pico Password, or
2. **two accounts, two master passwords**, one each.

**Decided: 1, one account and one master password.**

How it was decided. Claude listed three disadvantages of option 1; going through them one by one with
the author, they turned out smaller than first presented:
1. **The two apps are tied**, for example in a master password change. For the user it is one change
   (the author: "non vedo il problema"); the cost is only for who builds it: the change must reach both
   vaults, also when one app is not open or not installed on the device where the change is made. The
   author agreed it could be a problem, so it was looked at (below).
2. **Who learns the password has the one of both vaults.** Alone it is not enough: the copies on the
   server need the second key (each vault has its own), which an attacker gets only by having a device
   approved by one of the user's devices (the code would not match); on the user's unlocked computer
   the keychain holds everything anyway, with one account or two. What remains: with user name and
   password the attacker logs in to glupo.dev and could delete files, of both apps. The author: "relativo".
3. **Pico Password depends on Pico Auth.** It does not: Claude had assumed the account could be created
   only from Pico Auth, which the author never said. With sign-up possible from either app, who uses
   only Pico Password never sees Pico Auth.

What remains: who uses **both** apps has the same master password in both.

Advantages of option 1: one password to remember, one sign-up and one invite (simpler for colleagues).
Advantages of option 2, set aside: the apps fully independent, Pico Auth unchanged; TOTP codes and
passwords really separated by two passwords; simpler to build.

Either way, a new device is approved once per app (the second keys are different).

**Where the master password is kept** (asked by the author; facts of Pico Auth today): in the user's
head and in the keychain of each linked device (entry `master password`, service `picoauth`), which is
why the app opens by itself. Not on GitHub, not on glupo.dev (the server gets only `login_hash`,
Argon2id of the password, and keeps its SHA-256), not inside the vault file. So after a change every
device has the old password in its keychain: it notices (the login fails) and asks the new one once.

**Proposed** (the author asked whether it is solvable): each app updates its own vault at its first
opening after the change.
1. Mario changes the password from Pico Auth: Pico Auth encrypts its vault again with the new password
   and changes the account password on the server.
2. Days later Mario opens Pico Password on the PC: its login with the old password (in its keychain) is
   refused.
3. Pico Password asks "The master password changed: type the new one"; the login works.
4. Pico Password still has the old password in its keychain: it opens its vault, encrypts it with the
   new one and uploads it.
Edge case: between 1 and 4 Mario adds a new device to Pico Password; it knows only the new password,
while the vault is still encrypted with the old one. To handle, for example by asking the old password
in that case.

- **Open.** Whether a master password change is wanted at all (the author: not decided yet, "anche se
  forse sarebbe preferibile"). It does not exist in Pico Auth either (picoauth-sync.md 12).

## 6. Proposed by Claude, not decided

Claude's first high-level picture had more details. The author: "mi convince quasi tutto se non tutto",
but "sei già sceso troppo nei dettagli". So these are **Proposed**, to take up when the base is defined:
- an app `picopassword` in picotools, same window, keys and logic as Pico Auth: it opens on the list,
  `enter` copies;
- the vault, second key, approval and sync code, today inside `cmd/picoauth`, moved to a shared part used
  by both apps;
- **search**: passwords are many (100 or 200), unlike the few TOTP accounts; type "ama", find Amazon;
- a key to copy the password and one to copy the user;
- the password hidden on screen (`••••••`), shown only on request;
- a **password generator**: without the browser, you generate the password in Pico Password and paste it
  into the site at sign-up;
- the clipboard cleared after some seconds (a copied password otherwise stays there until something else
  is copied).

## 7. Still open

- **Site** field (section 3).
- **Master password change** (section 5).
- **Import from 1Password**: without it, the passwords one already has must be typed again by hand.
- **Lock**: Pico Auth opens by itself on a device; for passwords maybe it should lock after some
  inactivity, as 1Password does.
- **Sign-up from either app** (section 5, point 3): follows from the author's remark, to confirm.
- On the server, not discussed: one account for both apps means picoauth-server (or whatever serves
  `/picopassword`) shares the accounts between the two prefixes; how, to design. It also changes
  Pico Auth's glupo.dev account, which is in production with the author's test account `gennaro`.
- Everything in section 6.
