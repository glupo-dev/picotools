# GL API

A small app to call the APIs I use, with their keys kept safe.
Same style as GL Tools: a console window driven only by the keyboard. No buttons, no forms.
It is not Postman and not a chat: only what real needs ask for.

## Structure

- New app **GL API** in `cmd/glapi`, with a generic icon, built by `build.sh` like the others.
- Theme and window come from `internal/gui`.
- The JSON/XML viewer of GL Tools ("View JSON/XML") moves to `internal/gui`, so GL Tools and GL API share it. GL Tools stays the same.

## Data

### Provider

One provider = one key. Two keys for the same service (e.g. test and production) are two providers.

- name (e.g. `meteo-test`)
- base URL (e.g. `https://api.meteo.com/v1`)
- key, optional: where it goes and its name: a header (e.g. `X-API-Key`), `Authorization: Bearer <key>`,
  or a query parameter (e.g. `apikey`)
- fixed headers, optional: a list of `name: value` sent with every request (e.g. `anthropic-version: 2023-06-01`)

### Endpoint

Each provider has its endpoints.

- name (e.g. `city forecast`)
- method and path (e.g. `GET /forecast?city={city}`)
- body, optional: pasted as text (also on more lines)

Variables:

- `{name}` is required: the request does not start while it is empty.
- `{name?}` is optional: when empty, in the query string the whole parameter is left out
  (`/forecast?city={city}&lang={lang?}` becomes `/forecast?city=Napoli`); in the path or the body it becomes empty.
- Values put in the body are escaped for JSON (quotes, line breaks).
- The last 10 values of each variable are remembered; the field starts with the last one and ↑/↓ go through the others.
  So variables must not hold secrets.

`Content-Type: application/json` is added by the app when there is a body.

### Where things are saved

- **Everything except the keys** goes in one JSON file in the standard app folder:
  - Mac: `~/Library/Application Support/glapi/`
  - Windows: `%AppData%\glapi\`
  - Not encrypted (it holds no secrets), readable only by the user, saved after every change.
    Everything is managed from the app: the file is never edited by hand.
- **Keys** go in the system keychain (Keychain on Mac, Credential Manager on Windows), through `go-keyring`.

Example of the file:

```json
{
  "providers": [
    {
      "name": "meteo-test",
      "url": "https://api.meteo.com/v1",
      "keyIn": "header",
      "keyName": "X-API-Key",
      "endpoints": [
        {
          "name": "city forecast",
          "request": "GET /forecast?city={city}&lang={lang?}",
          "values": { "city": ["Napoli", "Roma"] }
        }
      ]
    },
    {
      "name": "anthropic",
      "url": "https://api.anthropic.com/v1",
      "keyIn": "header",
      "keyName": "x-api-key",
      "headers": [{ "name": "anthropic-version", "value": "2023-06-01" }],
      "endpoints": [
        {
          "name": "ask",
          "request": "POST /messages",
          "body": "{\"model\": \"claude-sonnet-5-5\", \"max_tokens\": 1024, \"messages\": [{\"role\": \"user\", \"content\": \"{prompt}\"}]}"
        }
      ]
    }
  ],
  "saveDir": "/Users/me/Downloads"
}
```

`keyIn` is `header`, `bearer`, `query` or empty (no key). `values` are the last values typed for each variable.

## Screens

Every list uses the same keys: ↑/↓ move · enter open/run · number quick pick · `n` new · `m` edit · `d` delete (asks first) · esc back.

### Providers (start)

```
GL API

  1  meteo-test              api.meteo.com · key · 3 endpoints
  2  meteo-prod              api.meteo.com · key · 3 endpoints
  3  my-backend              localhost:8080 · key · 7 endpoints
  4  jsonplaceholder         jsonplaceholder.typicode.com · no key · 2 endpoints

  n  new provider

  ↑/↓ move · enter open · number quick pick · m edit · d delete · esc quit
```

### New provider

Questions one after the other:

```
New provider
Name: anthropic
Base URL: https://api.anthropic.com/v1
Key goes in (1 header · 2 Authorization: Bearer · 3 query · 4 none): 1
Header name: x-api-key
Key: ••••••••••••
Fixed headers (name: value, empty line to finish):
  anthropic-version: 2023-06-01

Saved.
```

### Edit provider

```
GL API › anthropic › edit

  1  Name           anthropic
  2  Base URL       https://api.anthropic.com/v1
  3  Key            header x-api-key · ••••••••
  4  Headers        anthropic-version: 2023-06-01

  ↑/↓ move · enter change · esc back
```

Headers open their own list: `n` new (`name: value`), enter change, `d` delete.

### Endpoints of a provider

```
GL API › meteo-test

  1  GET   /forecast?city={city}         city forecast
  2  GET   /current?lat={lat}&lon={lon}  current weather
  3  POST  /alerts                       create alert

  n  new endpoint

  ↑/↓ move · enter run · m edit · d delete · esc back
```

### Running an endpoint

```
meteo-test › city forecast
city: Napoli█        ↑/↓ earlier values (3)
lang (optional): █
```

While waiting a visible indicator is shown; esc cancels. Timeout: 60 seconds.

### Response

```
200 OK · 143 ms · 1.2 KB
{ ...colored JSON in the viewer... }
```

`h` response headers · `c` copy the body · `s` save · `u` copy as curl · esc back.

### Copy as curl

`u` opens the request just sent as a curl command, for each shell:

```
1  Mac / Linux          curl 'https://…' -H 'X-API-Key: <API_KEY>' -d '{"a":1}'
2  Windows cmd          curl.exe "https://…" -H "X-API-Key: <API_KEY>" -d "{\"a\":1}"
3  Windows PowerShell   curl.exe 'https://…' -H 'X-API-Key: <API_KEY>' -d '{"a":1}'
```

enter or the number copies the whole command, always on one line, with the variables filled in.
The key is never written: `<API_KEY>` stands in its place, so the command can be sent to anyone;
whoever knows the key puts it there. A JSON body on more lines is compacted to one.

Responses that are not text (images, PDF) show only status and size, with `s` to save them as they are.

### Save

```
meteo-test › city forecast › save
File: ~/Downloads/meteo-test-city-forecast-20261017-1530.json█
enter save · esc cancel
```

- Name: `provider-endpoint-YYYYMMDD-HHMM.ext`, spaces turned into `-`, characters not allowed in file names left out;
  a second save in the same minute adds `-2`. The name can be changed before enter.
- Extension from the response Content-Type: `.json`, `.xml`, `.html`, otherwise `.txt`.
- Folder: Downloads the first time, then the last one used.
- An existing file is never overwritten.
- JSON and XML are saved formatted, as shown on screen.

## Not now

Added only when a real need comes up:

- free request (any URL, no provider)
- headers on a single endpoint
- an editor for the body
- `{{key}}` inside the body
- anything chat-like (conversation, model list, tokens, streaming)
