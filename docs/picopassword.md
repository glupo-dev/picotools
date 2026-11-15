# Pico Password: design and state

A new picotools app, **Pico Password**: a place to keep passwords, "a kind of 1Password". Idea of the
author from 2026-11-13; preliminary analysis with the author on 2026-11-14, step by step, at a high
level; screens, the local app and the sync designed and built on 2026-11-15.

**State at the end of 2026-11-15:**
- The **local app** is built (`cmd/picopassword`, section 9): list, item page, Edit/New, History,
  favourites, suggested passwords, Settings. No Site field, no search (sections 3, 4.7). The author tried
  it on a test vault of 120 dummy items (since deleted) and said it works.
- The **sync** with GitHub and glupo.dev is built (section 5), copied from Pico Auth, with its own server
  **picopassword-server**, which is **online** at `https://vault.glupo.dev/picopassword`.
- The author **created a real vault** on the Mac, linked it to glupo.dev as user **`gennaro`** (with an
  invite made with `picopassword invite` on the VPS) and added one real password copied from 1Password:
  the change reached the server.
- **Not tried yet:** a second device (join and approval), the GitHub sync (the GitHub App needs its
  private key and an installation first, section 5.1), being offline with a linked vault.
- Committed and pushed: picotools `e94a828` (on `main`), and the server in its own private repository
  `glupo-dev/picopassword-server` (`1835289`). After that only this document changed (not committed).

**Next steps** (none decided, in the order they came up): try a second device from glupo.dev; make the
private key of the GitHub App and try the GitHub sync; check the other apps after the `tab` change in
`internal/gui` (section 9); then the open points of section 8, one at a time, each as one proposal with
its screen.

How to work with the author on this app (from this chat): show the **screen** (a mockup) for every
proposal, not a description; **one version** at a time, not many; one step per message; do not move to
another topic before the current one is settled; ask before building; do not write Claude's proposals as
decisions.

Read first, for what is reused: [picoauth-sync.md](picoauth-sync.md) (the vault, the second key, the
approval of a new device, the GitHub sync) and [picoauth-glupo-sync.md](picoauth-glupo-sync.md) (the
sync through glupo.dev).

Each point is marked:
- **Decided**: agreed with the author.
- **Proposed**: suggested by Claude, not confirmed by the author (silence is not agreement).
- **Open**: not decided.
- **Built** says what exists in the code; **Dropped** an idea set aside, with why. A built detail with no
  mark (a message text, a minimum length) is Claude's, not confirmed by the author: treat it as Proposed.

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

## 4. The first version, screen by screen

Discussed on 2026-11-15. Claude proposed the screens on the model of Pico Auth, changing only what
passwords need; the author said "ok" and added the points marked below.

### 4.1 First start

- **Decided.** As in Pico Auth: `1 New vault` (asks the master password twice), or `2 Open an existing
  vault` → `1 From GitHub` / `2 From glupo.dev`.
- **Built**, both: `1 New vault` (at least 8 characters, typed twice) and `2 Open an existing vault`
  (section 5). The master password goes in the keychain (service `picopassword`), so the app opens by
  itself afterwards; when the keychain does not have it, the app asks it.

### 4.2 The list

The screen the app opens on, in the style of the tables list of Pico DB (`cmd/picodb/render.go`,
`renderTables`; the author showed a screenshot of it):

```
╭─────────────────╮
│  Pico Password  │
╰─────────────────╯

Passwords   10 items

›  1* Amazon            mario@example.com          ••••••••••••••••
   2  Apple ID          mario@icloud.com           ••••••••••••••
   3  Google            mario.rossi@gmail.com      ••••••••••••
   4  Google lavoro     m.rossi@azienda.it         ••••••••••••••••••
   5  Netflix           mario@example.com          ••••••••••
   6* PayPal            mario@example.com          ••••••••••••••••••••
   7  Poste Italiane    MRARSS75A01H501Z           ••••••••
   8  Spotify           mariorossi                 ••••••••••••
   9  UniCredit         12345678                   ••••••
  10  Wi-Fi casa        FRITZ!Box 7590             ••••••••••••••••••••••••

↑↓ move · ←→ page · enter copy password · u copy user · r reveal · o open · n new · f only favourites · s settings · esc quit
```

(The keys line as built: `o open` took the place of `e edit`, section 4.4.)

- **Decided** (the author): the **logo** in its box at the top, as in Pico DB ("non è una riga ma una
  specie di logo"), then a blank line and the title of the screen.
- **Decided.** The same colors as Pico DB: logo green; title white with the count in grey (as
  "16 tables"); the **selected row** with a dark green background across the whole width, `›` and the
  name in green (the author had asked for the selected row to be more highlighted than in Pico Auth);
  names white; user and dots grey; the keys line grey.
- **Decided.** Each row shows number, name, user and the password hidden (section 4.6 for how).
- **Decided** (the author): **numbers shown, as in Pico DB, only as labels**: green, aligned to the
  right. Typing a digit does nothing, and the keys line says nothing about them. Why: numbers as keys
  "confondono solamente, anche con 3 elementi" (in Pico DB, a digit opens an entry up to 9, and in
  longer lists the digits move the cursor and enter opens).
- **Proposed:** the title "Passwords"; the items in **alphabetical order** by name.

### 4.3 Moving in the list

- **Decided.** `↑/↓` one row; the list wraps (from the last `↓` goes to the first, from the first `↑` to
  the last), as in Pico DB.
- **Decided.** `←/→` one **page** (Claude's suggestion, the author: "può essere una buona idea"). `→`
  as PgDn: to the last row of the page, or when already there to the last row of the next page; `←` the
  same upwards with the first row. They stop at the end and at the start (as `page` in
  `cmd/picodb/model.go`). Why the side arrows: next to `↑/↓`, in the same place on every keyboard,
  understood alone (one row, one page), and free: Pico Auth does not use them at all, and Pico
  Password needs them nowhere else (Pico DB uses them on one screen only, with another meaning).
- **Dropped:** **PgUp/PgDn** (the author: hard to find, in different places on different keyboards);
  **`n`/`p`** as in Pico DB (`n` is "new item").
- **Decided.** **Rows per page: as many as fit in the window, at most 50**, as Pico DB (`listRoom` in
  `cmd/picodb/render.go`: the window height less 9 lines, 2 more while a message is shown, at most
  `pageSize` = 50; built the same in `cmd/picopassword/render.go`, so a message takes 2 rows of the list
  while it is shown, as in Pico DB). The author tried
  that Pico DB shows 50 rows on several PCs; **49 is not acceptable**, so Pico Password must use exactly
  the lines of Pico DB around the list (the code of both: `rows - 9`, `- 2` more while a message is
  shown).
- **Decided** (Claude's suggestion, to keep the 50 rows): the **keys on one line**, without shortening
  (about 115 characters; the tables line of Pico DB is about 105 and is shown whole). Not chosen: dropping
  the blank line under the title (it makes the list readable).

### 4.4 Keys

**Decided rule: in the list you only read; every change is made on the page of an item (section 4.8)
or in New (section 4.9).** The list works always, also without connection; changes need the connection
(below).

- **Decided.** `enter` copies the password of the selected row; `u` copies the user. A message under the
  list, for example "Password copied: Google", in the 2 lines Pico DB keeps for messages.
- **Decided.** `n` new item (section 4.9).
- **Decided** (the author chose the word: "open" rather than "details"): `o` **opens the page of the
  item** (section 4.8). First the plan was `e edit`, with delete and favourite inside Edit (the author:
  "delete toglilo, va in edit"); designing that screen it became a page of the item with Edit as one of
  its actions.
- **Decided.** No `d` in the list: delete is on the item page, so an item cannot be deleted by mistake
  from the list.
- **Decided.** `f` only favourites (section 4.5); `r` reveal (section 4.6); `s` Settings (section 4.11).
- **Decided** (the author, 2026-11-15): **`esc` in the list asks before quitting.** The author often
  presses `esc` there by mistake, to go "back" towards nothing. The first `esc` only shows "Press esc
  again to quit" in the message lines; a second `esc` right after quits; any other key clears the
  message and does its usual job (`↓` moves), and the next `esc` asks again. Only in the list: on every
  other page `esc` goes back one page. Why two `esc` and not a question page (Claude's suggestion, the
  author: "ok solo sulla homepage"): nothing new to read, and a real quit costs one key more. Where there
  is nothing to go back to, `esc` still quits at once: the first start (no vault yet), the master
  password asked at opening, the error page.
- **Decided** (the author asked, Claude agreed): **changes disabled without connection** to the remote
  vault. When the last 10-second check could not reach the server, the keys that change the vault (`n`,
  and `e`, `f`, `d` on the item page) are grey and pressing them does nothing; a message says so
  (the text is Claude's, built, **Proposed**: "No connection to glupo.dev: you can copy, not change",
  with GitHub instead of glupo.dev for a vault on GitHub);
  at the next check that works
  they come back by themselves. Why: better to say it at once than to let the user fill a form and
  refuse the change at the end (as Pico Auth does today). The message at saving ("the change was not
  saved") stays, since the connection can drop between two checks, while a form is open. A vault not
  linked (local only) has no server: there everything always works. **Built** (section 9): on the item
  page Edit,
  favourite and Delete are grey; in the list the keys line stays as it is, since it is all grey already
  (**Proposed**).

### 4.5 Favourites

- **Decided** (the author asked "vogliamo fare anche i preferiti?"; Claude: yes, because there is no
  search, and the passwords used every day are few, for example 5 of 150). As in Pico DB, so whoever
  knows one app knows the other: a favourite item has a `*` after its number; `f` shows **only the
  favourites**, `f` again the whole list.
- **Decided.** An item is made a favourite (and back) **on its page** with `f` (section 4.8), as a table
  of Pico DB is from its page.
- **Decided** (the author; Claude agreed): **favourites are in the vault**, the same on every device,
  already there on a new device. Consequence: marking a favourite is a change, uploaded at once,
  impossible without connection; favourites change rarely, so it is not a problem. Not chosen: per
  device (for example the work passwords as favourites at work, the personal ones at home).
- **Proposed** (as Pico DB), **built**: switching between all and favourites keeps the cursor on the same
  item when it is still in the list; the title shows "3 favourites of 10 items"; the keys line says
  `f only favourites` or `f all items`. In the list of favourites only, the `*` is not shown (they all
  are). With no favourite: "No favourites yet: open an item with o and press f to add it."
- A new item is not a favourite: after New the list goes back to all the items, to show it.

### 4.6 How the password is shown

- **Decided** (the author): **one dot per character** by default. Claude had advised always 8 dots, so
  that someone looking at the screen does not learn the length (1Password does so). The author: the
  length helps, and 99% of the time the owner is alone in front of the program; no real danger in
  showing it. Example of the help: a password of 6 characters is an old one, to change.
- **Decided: reveal**, key `r` (the author: "assolutamente sì"): shows in clear the password of the
  selected row; `r` again hides it. Why: a password that cannot be pasted must be typed by hand on
  another device, for example a TV (watch the screen, type with the remote, 30 seconds or more).
  - It hides by itself when the selection moves or the list is left, and after a time: **Proposed** 30
    seconds (built so).
  - **Decided** (the author, trying the app): `r` works also **on the item page** (section 4.8), with the
    same rules.
  - **Dropped:** show while the key is held, hide on release (the author's first idea). Holding the key
    for the whole time blocks a hand while the other uses the remote. Also, the window of picotools
    would need to see the key release: not verified.
- **First characters in clear.** Why (the author): sometimes 1 or 2 characters are enough to tell which
  password it is, for passwords invented by a person (variants like `Mario1975!`, `Mario1976!`), not the
  generated ones. Risk, as told to the author: with a trusted colleague seeing 1 or 2 characters,
  practically none; a little more with a recorded meeting or a screenshot of a shared screen, still
  very small (2 characters of a long generated password help nobody; of a weak one a little). So it
  became a choice in Settings.
- **Decided: Settings › Show passwords as** (the author's idea, 2026-11-15). The Settings page as built
  is in section 4.11; `1 Show passwords as` opens:

  ```
  Show passwords as

    1  8 dots
       ••••••••   (always 8, whatever the length)

  ▶ 2  One dot per character
       ••••••••••••   (a 12-character password)

    3  First 2 characters
       Ma••••••••••   (the first 2 in clear, then a dot per character)

    4  First and last character
       M••••••••••!   (first and last in clear, a dot per character in between)

  enter choose   esc back
  ```

  - Option 4 added by the author. Told: it shows 2 characters like option 3, but does not tell apart
    `Mario1975!` and `Mario1976!` (both `M••••••••!`); it helps when the variants change at the start or
    the end.
  - Each option with its example in grey under it; `▶` marks the current choice; `1-4`, or `↑/↓` and
    `enter`; `esc` back without changes (**Proposed**: the layout and keys).
  - **Decided** (Claude's advice, asked by the author): the choice is **per device**, a small preferences
    file next to the vault, not in the vault and not synced. Why: the risk depends on the place (home
    alone, the work PC shared in meetings); no sync, no conflicts; the vault holds only the items, as in
    Pico Auth. Price: a new device starts from the default.
  - **Decided.** The default is **One dot per character**.
  - The position of the item in Settings (`1`) is **Proposed**.

### 4.7 Search: not in the first version

- **Decided** (the author, 2026-11-15): **no search** in the first version, for now. The author then
  said "ti dico come faremo" and moved to the list of Pico DB (sections 4.2, 4.3); how to search was not
  said yet, so it stays **open**.
- **Decided: two requirements the author called "imprescindibili"**, which any search must respect:
  find an item **with the arrows** and **copy the password with one key** (`enter`), with no step in
  between; the other keys of the list (`u`, `r`, `o`, `n`, `f`, `s`) stay.
- **Decided: the pico rule**, stated by the author on the way: **no more than 3 lines of instructions for
  the whole program**; an interface that needs explaining is wrong ("se non lo capisco io che sono lo
  sviluppatore devi completamente cambiare logica").
- Why it failed: the commands are single letters, and letters are also what one types to search. Every
  design shown went around that conflict, and each needed explaining. **Dropped**, all proposed by
  Claude:
  1. `/` opens a search field (as in `less`, `vim`, Gmail; on an Italian keyboard `/` is Shift+7); while
     searching letters are text, `enter` copies and closes, `esc` closes. Two modes: the author did not
     understand when a letter is a command and when it is search.
  2. The same, with `enter` ending the typing and leaving the list filtered, then the usual keys.
  3. Type-to-search always, every command with Ctrl (as Spotlight and 1Password); the author does not
     want key combinations.
  4. A search box always visible ("Type to search"), `enter` **opens** the item and a numbered menu of
     actions (Copy password, Copy user, …) does the rest: copying the password took two steps.
  5. The search field as a row above the list, reached with `↑` from the first item ("the keys go where
     the `▶` is"): needed a manual.
  6. Type to search, `enter` copies, `→` opens a menu with everything else.
- Found on the way: searching **only in the name**, not in the user (the users are almost all the same
  email, so with the user "a" kept 9 of 10 items, with the name only 6); from the first character, the
  list filtered at every character, any position in the name, case ignored. **Decided** by the author
  ("sì, solo nel nome"), for whenever a search comes back.

### 4.8 The page of an item

Opened with `o` from the list. Designed with the author on 2026-11-15 ("prima riga direi edit, …
seconda riga add or remove to favourites, delete ovviamente con conferma"), **built**:

```
╭─────────────────╮
│  Pico Password  │
╰─────────────────╯

Amazon

  User        mario@example.com
  Password    ••••••••••••
  Favourite   no

› e  Edit
  f  Add to favourites
  h  History
  d  Delete

↑↓ move · enter choose · r reveal · e edit · f favourite · h history · d delete · esc back
```

- **Decided.** The fields on top, the actions below, each with its letter; an action is chosen with its
  letter, or with `↑/↓` and `enter` (the selected one highlighted as in the list).
- **Decided.** `e` **Edit** (section 4.9).
- **Decided.** `f` **Add to favourites** / **Remove from favourites** (the label follows the state):
  saved at once, message "Added to favourites" / "Removed from favourites".
- **Decided** (the author: "oppure le vecchie password su una voce specifica history"): `h` **History**
  (section 4.10).
- **Decided.** `d` **Delete**, with a confirmation (below).
- **Decided** (the author, trying the app): `r` reveals the password here too (section 4.6).
- `esc` back to the list, with the cursor on the item.

**Delete.** It took long: Claude showed too many versions, and messages the author found unclear
("delete significa delete"; "la cosa che non va è che mi hai presentato 50 versioni"). The author picked
the yes/no one, as Pico Auth, and set the rules:

```
Delete Amazon?

› █

type yes and enter to delete · anything else no
```

- **Decided.** It deletes only with **`yes` and `enter`** (case ignored). Not `y` alone (the author:
  "per errore potrei cancellare un item").
- **Decided** (the author): **anything else and `enter` is the same as `esc`**: back to the item page,
  nothing deleted. `Backspace` corrects.
- After the delete: back to the list, message "Amazon deleted".

### 4.9 Edit and New

The same page for both, built. It went through several designs, all with the author on 2026-11-15:

```
Edit Amazon

  Name       Amazon Prime
  User       mario@example.com
  Password   ••••••••••
› Repeat     ••••••••••█
  Save

↑↓ move · enter next · esc back
```

- **Decided.** **One page** with Name, User, Password and a **Save** row. `↑/↓` move between the rows
  (they wrap). On a field `enter` goes to the row below; **only `enter` on Save saves** (the author:
  "devo scendere fino ad una quarta riga che conferma"). `esc`, from any row, goes back without saving
  anything (keys line `esc back`, the author's words, not "cancel all").
- **Decided.** On Name and User the value is already written and is edited as text: the cursor at the
  end, `←/→` move it, typing inserts, `Backspace` deletes one character, paste inserts at the cursor
  (the author had noticed that in Pico Auth, if you do not keep a field, it is deleted and must be
  typed again; Pico Auth gets the same fix later, "ma non adesso").
- **Decided** (the author): **a password typed by hand is never shown**, only its length in dots, one
  per character, the old one and the new one (this page does not follow Show passwords as, which is for
  the list, the item page and History; built so, not discussed). So the old one cannot be corrected one character at a time: on the Password row any
  character typed, a paste or `Backspace` **replaces it whole** (the author: "perfetto"). `↑/↓`, `enter`,
  `esc` leave it as it is: passing over the Password row changes nothing.
- **Decided** (the author, firmly: "repeat ci deve sempre essere"): **Repeat**, the password typed twice.
  In Edit the Repeat row appears only when the password is changed, and stays even if the new one is
  then emptied; in New it is always there. On Save,
  if they differ: "The two passwords are different", the cursor on Repeat, emptied.
- **Decided** (the author): on the **Password** row, **`←` suggests a strong password and `→` a stronger
  one** (section 4.12). A suggested password **is shown in clear**, and Repeat is filled with it, also in
clear; each
  press makes a new one. Typing or `Backspace` after a suggestion starts again by hand (dots, Repeat to
  type).
- On Save: a name, a user and a changed password cannot be empty; a changed password goes into the History (section 4.10);
  the items are sorted again by name. Edit goes back to the item page ("Amazon saved"); New goes back to
  the list with the new item selected ("Amazon added"); `esc` in New goes back to the list.

**Dropped on the way** (all Claude's, or tried and found wrong):
1. The questions **one at a time**, as in Pico Auth (Name, then User, then Password). To the author each
   question looked like a page: on the Password `esc` closed everything and the changes to Name and User
   were lost ("ricomincio daccapo").
2. `esc` going back **one question**: to cancel everything, `esc` three times ("macchinoso").
3. `enter` saving from any field: the author wants the Save row.
4. Each field changed **on its own** from the item page: "è lo stesso solo una pagina più su".
5. The password **in clear** while the cursor is on it, and Repeat dropped: the old password must not be
   seen, and Repeat stays.
6. `tab` to show/hide the password (decided first for New): not needed, the password is never shown.
7. The suggestions on `↓/↑` (decided first, built in the questions form): the arrows are needed to move
   between the rows.

### 4.10 History

- **Decided** (the author): the **old passwords** of an item, with the date they were replaced, on their
  own page (`h` on the item page); **at most 10**, the newest first, the oldest dropped. Only a change
  of the password in Edit adds one (also when that change meets a change of another device, 5.2 case 4).

```
Amazon › History

› 15 Nov 2026   ••••••••••
  02 Mar 2025   ••••••••

↑↓ move · enter copy password · r reveal · esc back
```

- **Proposed**, built: `enter` copies the old password ("Old password copied"); `r` reveals it as in the
  list (30 seconds, moving hides it); with none: "No old passwords: they appear here when the password
  is changed in Edit."

### 4.11 Settings, as built

A vault not synced:

```
Settings

  1  Show passwords as:  One dot per character
     How the list hides the passwords, on this device only.

  2  Suggested passwords:  strong 20 · stronger 30
     How long and with which characters the passwords suggested with ← and → are.

  3  Sync with GitHub
     Put this vault in a private GitHub repository of yours, to have it on your other devices.

  4  Sync with glupo.dev
     Put this vault on glupo.dev with an invite code, to have it on your other devices.

  5  Delete this vault
     Deletes the vault and its passwords from this device, for ever.

number choose · esc back
```

A synced vault: the line "Synced with glupo.dev: gennaro" (or "Synced with GitHub: owner/repo") on top,
then `1` and `2` as above, `3 Linked devices`, `4 Reset this device` ("The vault stays on glupo.dev").

- `1` Show passwords as (section 4.6); `2` Suggested passwords (section 4.12; also a choice of this
  device only, though its description does not say so); `3`/`4` the sync (section 5; the entries do
  what Pico Auth's do, the order is different: Pico Auth has Reset `1` and Linked devices `2`); `5` Delete this vault, as Pico Auth: type `delete`, then the master password; it
  removes the vault file, the link, the preferences and the keychain entries, and goes back to the first
  start. Reset this device does the same for a synced device, after removing it from Linked devices; the
  vault stays on the server.
- The descriptions under the entries are the app's texts as built: `1` says "the list", though the
  choice also applies to the item page and History; `2` does not say "on this device only". To fix if
  the author wants.
- **Proposed**: the order of the entries, and the numbers as keys as in Pico Auth (asked to the author
  whether to remove them, as in the list: no answer yet).

### 4.12 Suggested passwords (the generator)

Decided with the author on 2026-11-15. What it is for: without the browser, the password for a new site
is made in Pico Password and pasted into the site.

- **Decided.** Two kinds, **strong** and **stronger**, from the Password row of Edit and New: `←` strong,
  `→` stronger (section 4.9). The keys line says "← suggest strong · → suggest stronger", without lengths
  (the author: "togli 20 e 30, fai suggest strong e stronger").
- **Decided** (the author): what strong and stronger are is chosen in **Settings › 2 Suggested
  passwords**, and they must differ **in more than the length**: each has length, digits and symbols.
  Letters, upper and lower case, are always in. Defaults (Claude's, the author: "esatto"):

  ```
  Suggested passwords

                     Strong     Stronger
    Length           20         30
    Digits           yes        yes
    Symbols          no         yes

    Strong     ←   kR7vQ2mxT9pL4wZe8hNa
    Stronger   →   Hq3!nW8zR#5tYb2kM9@pLx4&Vc7sJe

  1 strong · 2 stronger · esc back
  ```

  Strong without symbols because some sites refuse them. The examples change when a value changes. `1`
  or `2` asks the three values of that kind one after the other (Length 8 to 64, Digits and Symbols
  `yes`/`no`).
- **Proposed**, built: the symbols are `!@#$%&*?-_=+`; every kind of character chosen appears at least
  once; random from the system (Go `crypto/rand`).
- **Proposed**, built, **to confirm**: the choices are **per device** (in the preferences file, as Show
  passwords as). The alternative is in the vault, the same on every device; asked to the author, no
  answer yet.

## 5. Sync

### 5.1 How

- **Decided** (the author, 2026-11-15): **two completely separate apps**, "quasi sconosciute tra loro":
  Pico Password has its own server, its own users, tokens and invites, its own GitHub App. They share
  only the logic (the sync, the invites, the approval), and the code is **copied almost identical** from
  Pico Auth, not shared (Claude's advice: Pico Auth, in use, is not touched; the price is that a fix
  goes in two places). This replaces the decision of section 6 (one account for both apps).
- **Decided.** The same as Pico Auth: GitHub or glupo.dev, the vault encrypted, the second key, a new
  device approved by a linked one with the 6-digit code, Settings, Linked devices, Reset this device.
- **Decided.** On glupo.dev "exactly as for Pico Auth" (account with user name and invite code, the
  master password is also the account password, the server sees only encrypted data), at
  `https://vault.glupo.dev/picopassword` (the path prefix planned in picoauth-glupo-sync.md 9).
- **Built:** the GitHub App **Pico Password** (`github.com/apps/pico-password`, created by the author on
  glupo-dev with the same choices as Pico Auth: Device Flow on, tokens that expire, no webhook, only
  Contents read and write, any account; Client ID `Iv23lianw0vEJ0ZqhqzT`). The private key GitHub asks
  before the first installation is still to make.
- **Built:** **picopassword-server** in `~/Projects/glupo-dev/picopassword-server`, a copy of
  picoauth-server with its own names: database `picopassword`, port **8687**, service
  `picopassword-server`, command `picopassword` (invite, invites, revoke), settings
  `/etc/picopassword/picopassword.env`; the vault file `vault.picopassword`. Its repository is
  **`glupo-dev/picopassword-server`, private** on GitHub (created by Claude at the author's request).
  **On the VPS** since 2026-11-15, installed by the author (the author chose
  to do it, with Claude's commands): system user, role and database `picopassword` (peer access), the
  package from `deploy/package.sh` with `install.sh`, and in the `vault.glupo.dev` block of Caddy
  `handle_path /picopassword/*` → `reverse_proxy 127.0.0.1:8687`, before `handle` (copy of before:
  `/etc/caddy/Caddyfile.prima-di-picopassword`). Checked from the Mac: `/picopassword/health` ok in IPv4
  and IPv6, `/picoauth/health` still ok. Details in the VPS notes (`~/Projects/glupo-dev/netcup/CLAUDE.md`).
- The `login_hash` is salted with `picopassword-login:` and the user name, so the same user name and
  password on the two servers never give the same hash.
- Author's commands on the VPS: `picopassword invite` (a new invite code), `picopassword invites`,
  `picopassword revoke <username>`; logs `sudo journalctl -u picopassword-server`. Update: on the Mac
  `deploy/package.sh` in picopassword-server, `scp dist/picopassword-server.tar.gz glupo:~/`, on the VPS
  `tar -xzf picopassword-server.tar.gz && sudo sh picopassword-server/install.sh`. At start the log has a
  harmless warning about `.pgpass` (the user `picopassword` has no home).

### 5.2 Changes and conflicts

Explained to the author on 2026-11-15 (facts of how Pico Auth works, and their consequences here).

**How the server notices two changes at the same time: a version number.** The vault on the server is
at version 7; Mac and PC both downloaded it. The Mac uploads saying "I replace version 7": the server has
7, accepts, the vault becomes 8. The PC uploads a moment later, also "I replace version 7": the server
has 8, refuses. The PC downloads 8 (with the Mac's change), applies its own change again on it, uploads
"I replace version 8": accepted, 9, with both changes. So no change is lost silently. GitHub does it with
the sha of the file, glupo.dev with its `version` field (picoauth-sync.md 7).

The cases, from the most common:
1. One change at a time: none. The other devices see it within about 10 seconds.
2. No connection: the change is refused (section 4.4); nothing pending.
3. Two devices change two different items at the same time: none, both kept (as above).
4. **Two devices change the same item at the same time.** Pico Auth finds an item again **by its
   contents** (name, user, secret), because items have no id. In Pico Password changing the password is
   the most common change: the PC would look for "Amazon with the old password", not find it, and say
   "the item was deleted on another device", which is false. **Proposed** (consequence, not discussed
   further), **built**: **every item has a hidden id**, so it is found again even when changed. Then:
   - the **same field** changed on both (Mac sets the password `AAA`, PC `BBB`): **the last one wins**
     (`BBB`); nothing breaks, all devices end aligned. As built, when the last one is a changed password,
     the one it replaced (`AAA`) goes into its History, so it is not lost. Never confirmed by the author
     (only explained), as in Pico Auth;
   - **different fields** (Mac the password, PC the favourite): the PC applies again only its own change,
     both stay.
5. A change on one device, a delete on the other: **the delete wins**, as in Pico Auth (decided by the
   author, 2026-11-15, Claude's advice: the code stays the same, and it is a near impossible case). The
   device that made the change says "The item was deleted on another device" and its pages of that item
   close; the new password is still on its screen until then, nowhere else.
6. For some seconds a device has the old version (change on the Mac, copy at once from the PC): rare;
   trying again after 10 seconds is enough.

The author: "un caso assolutamente limite, nessuno cambia qualche voce del suo vault contemporaneamente
su più dispositivi", asked to understand it anyway.

**Dropped** (ideas of the author, "sono solo idee"; Claude advised against, the author agreed):
- **Stop the same vault being opened on two devices at once.** Possible only in part: glupo.dev could
  keep a lock renewed every 10 seconds, GitHub has no reliable way; and every device has its local copy,
  which opens without connection, so opening cannot really be stopped. Annoying: Pico Password left open
  at work keeps it closed at home; a device switched off keeps the lock until it expires.
- **Disable `e`/`n` while another device has the vault open.** The device list knows when a device was
  last opened, not whether it is open now: every open device would have to say "I'm here" every few
  seconds (fine on glupo.dev, a commit every few seconds on GitHub). Same problem: open at work blocks
  changes at home.
- **Disable `e`/`n` on the other devices while one is in Edit or New.** More reasonable (a short block),
  but: the others see it only at their next check, so two devices opening Edit within 10 seconds both get
  in, and the conflicts must be handled anyway; a form left open, or a device switched off, keeps the
  signal until it expires (for example 5 minutes); more work on the server and for GitHub.

## 6. One glupo.dev account and one master password for both apps (replaced)

**Replaced on 2026-11-15** by section 5.1: two separate apps, so **two accounts and two master
passwords**, one for each app (they may be equal if the user wants). What follows is how the first
decision was taken, kept for the record.

The question came from how Pico Auth works: **the password of the glupo.dev account is the master
password of the vault**. With two apps there are two vaults, so either:
1. **one account, one master password** for Pico Auth and Pico Password, or
2. **two accounts, two master passwords**, one each.

**Decided then, replaced since: 1, one account and one master password.**

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

**Proposed, now moot** (two separate apps, nothing to keep aligned between them): each app updates its
own vault at its first opening after the change.
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

- **Still open** (section 8): whether a master password change is wanted at all (the author: not
  decided yet, "anche se forse sarebbe preferibile"). It does not exist in Pico Auth either
  (picoauth-sync.md 12).

## 7. Proposed by Claude, not decided

Claude's first high-level picture had more details. The author: "mi convince quasi tutto se non tutto",
but "sei già sceso troppo nei dettagli". What became a decision went into sections 4 and 5. Left:
- the clipboard cleared after some seconds (a copied password otherwise stays there until something else
  is copied). Pico Auth already clears a copied code after 30 seconds; Pico Password does not. Told to
  the author only as a note ("the copied password stays in the clipboard"), never discussed.
- **Dropped:** moving the sync code of Pico Auth to a part shared by both apps: the author chose two
  separate apps with the code copied (section 5.1).

## 8. Still open

- **Site** field (section 3).
- **Search** (section 4.7): how, the author's to say.
- The title "Passwords" and the alphabetical order (section 4.2): built so, not confirmed.
- **Suggested passwords per device or in the vault** (section 4.12).
- **The numbers in Settings** (section 4.11).
- **Master password change**: not in Pico Auth either.
- **Clipboard cleared after a copy** (section 7): not discussed yet; for a password manager it matters.
- **Import from 1Password** (raised by Claude, not discussed): without it, the passwords one already has
  must be typed again by hand (the author copied a first real one by hand).
- **Lock**: Pico Auth opens by itself on a device; for passwords maybe it should lock after some
  inactivity, as 1Password does.
- **Later, decided:** the fields of the Pico Auth forms editable as here (section 4.9).
- **To check:** the other apps (Pico DB, Pico Edit, …) after the `tab` change in `internal/gui`
  (section 9): not tried in a real window.
- Everything in section 7.

## 9. Implementation notes

Built on 2026-11-15 in `cmd/picopassword`, on the model of `cmd/picoauth` and the lists of `cmd/picodb`:

- `vault.go`: the vault file `~/Library/Application Support/picopassword/vault.picopassword` (the
  config folder of the system), a JSON envelope (`format` "picopassword", `version` 1, or 2 when synced,
  with the second key) with the items
  encrypted with **AES-256-GCM** under a key made by **Argon2id** (3 passes, 64 MiB, 4 threads, random
  salt) from the master password. Written to a temporary file with mode 0600, then renamed. The master
  password is in the keychain (go-keyring, service `picopassword`, account `master password`).
- An item: `ID` (random hex, hidden, for the sync conflicts of section 5.2), `Name`, `User`, `Password`,
  `Favourite`, `History` (at most 10 `{Password, Replaced}`).
- `settings.go`: the preferences of the device in `prefs.json` next to the vault (`show_passwords`,
  `strong`, `stronger`); Settings. `generate.go`: the suggested passwords. `sync.json` next to the vault
  is the link (as Pico Auth).
- `model.go` (the list, the keys), `item.go` (item page, History, Delete), `edit.go` (Edit and New),
  `form.go` (the questions one at a time, still used for the master password and Settings), `render.go`.
- `model_test.go`: the flows by keys, with the keychain mocked (`keyring.MockInit`); `device_test.go`
  the helpers of the sync tests. `go test -race ./cmd/picopassword` (all pass); the real-server test runs
  only with `PICOPASSWORD_SERVER`, `PICOPASSWORD_TEST_INVITE`, `PICOPASSWORD_TEST_USER` (passed against
  the server on the Mac).
- `internal/gui`, for every app: Fyne used `tab` to move the focus and the apps never received it, so
  the console now keeps the keyboard focus and accepts `tab` (`KeyTab`). Pico Password no longer uses
  `tab`; the change stays, and the other apps were not tried after it.
- `build.sh` builds it with the others (`dist/mac/Pico Password.app`, `dist/windows/picopassword.exe`).
- **The sync** (2026-11-15), copied from `cmd/picoauth` with the names changed: `github.go`, `glupo.go`,
  `glupolink.go`, `join.go`, `pair.go`, `computers.go`, `forget.go`, `sync.go`, `save.go`, and their
  tests `sync_test.go`, `glupo_test.go`, `glupo_real_test.go` (against a real server, only when asked).
  What differs from Pico Auth:
  - `vault.go` is Pico Auth's (version 2 with the second key, HKDF info `picopassword vault`), with the
    items; the keychain service is `picopassword`;
  - every change (New, Edit, favourite, Delete) goes through `apply` with a function that finds the
    item **by its id** (`findItem`), so after a conflict it is applied again on the newer vault;
  - the keys that change the vault are refused while a change is being saved, or when the last check
    could not reach the server (`offline`, set by the 10-second check and by a failed upload, cleared by
    the next check that works): the message "No connection to GitHub: you can copy, not change", and on
    the item page Edit, favourite and Delete are grey (**Proposed**: the keys line of the list stays as
    it is, since it is all grey already);
  - an item deleted on another device closes its open pages at the next second;
  - Settings: `1` Show passwords as, `2` Suggested passwords, then not synced `3` Sync with GitHub, `4`
    Sync with glupo.dev, `5` Delete this vault; synced `3` Linked devices, `4` Reset this device
    (section 4.11); Reset and Delete also remove `prefs.json`;
  - the first start has `2 Open an existing vault` › From GitHub / From glupo.dev, as Pico Auth;
  - not copied: clearing the clipboard after 30 seconds (section 7);
  - the glupo.dev form fields are editable (prefilled values are kept and edited), unlike Pico Auth,
    where typing replaces them.
- **The test vault** of the author's machine (120 dummy items, a random master password kept only in the
  keychain) was deleted on 2026-11-15 with its keychain entry: the next start is New vault.
