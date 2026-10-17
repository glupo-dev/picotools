# gltools

Small, fast, keyboard-only desktop tools: the quick parts of big apps.

Big apps like SSMS, DBeaver or Postman are great, but they take seconds to open and show
a hundred features when you need one. gltools takes the opposite road: start from those apps,
keep only the few things done every day, and make them open in a second.

- **Keyboard only.** Every screen is a numbered list: type a number or use ↑/↓ and enter, esc goes back.
- **No setup, no manual.** Open it and use it. It assumes you already know the domain
  (what a table or a header is): it does not teach it.
- **Local.** Nothing goes to an online service you did not call yourself.
  Keys and connection strings are kept in the system keychain, not in plain files.
- **Mac and Windows**, one code base.

## The apps

### GL Tools
The small things you would otherwise search the web for.

- **Security:** random strings and passwords, hashes, SSH key pair (Ed25519), decode a JWT
- **Network:** my IP address, ping, check a website, DNS lookup, IP info, port check
- **JSON/XML:** validate, view with colors, structure and skeleton of a document,
  JSON examples from an OpenAPI spec, XML example from an XSD schema

### GL API
Call the APIs you use, with their keys kept safe.
Providers (base URL + key) and their endpoints are saved once; variables like `{city}`
remember their last values. A response can be copied as a one-line `curl` command,
with `<API_KEY>` in place of the key. See [docs/glapi.md](docs/glapi.md).

### GL DB
A read-only browser for **SQL Server** and **PostgreSQL**.
Saved connections → database → tables with row counts → a table: columns, keys and indexes,
its `CREATE TABLE`, and all its rows, 50 per page.
Rows can be exported to CSV, `INSERT`, or `DROP` + `CREATE` + `INSERT`.
It only reads: it has no SQL prompt, and the scripts it writes are text for you, never run.

### GL Edit
A simple text editor driven by the keyboard, in the spirit of vi but without its modes:
esc opens a command line with `w` save, `e` open, `q` quit, `wq`, `cd`, `mkdir`, `pwd`.

### GL Shell
The system shell (zsh on Mac, PowerShell on Windows) in its own window.

## Where things are saved

Only GL API and GL DB save something:

- Saved items (providers, endpoints, connections): one JSON file per app in the standard app folder
  (`~/Library/Application Support/` on Mac, `%AppData%` on Windows).
- Secrets (API keys, connection strings): the system keychain
  (Keychain on Mac, Credential Manager on Windows).

## Build

Written in Go with [Fyne](https://fyne.io). `build.sh` builds every app for Mac and Windows into `dist/`.
It runs on a Mac and needs Go, the Xcode command line tools and mingw-w64
(`brew install mingw-w64`) for the Windows builds.

```bash
./build.sh
```

`third_party/terminal` is a modified copy of [fyne-io/terminal](https://github.com/fyne-io/terminal)
(BSD-3-Clause), used by GL Shell. See [third_party/README.md](third_party/README.md).

## How it is made

The code is written with Claude Opus (Anthropic), under the direction and review of a developer
with almost 30 years of experience. Every feature starts from a real, repeated need, and most
ideas are turned down: the goal is to stay small.

## License

MIT, see [LICENSE](LICENSE). Take it and do what you like with it. Contributions are not accepted.
