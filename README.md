# canvas — CLI for Canvas LMS

Defaults to `https://onlinestudy.york.ac.uk` (override with `--base-url` or `CANVAS_BASE_URL`).

```bash
go build -o canvas .
./canvas auth login          # opens Chrome/Brave/Edge for SSO, then creates an API token
./canvas courses
./canvas modules COM00150M   # id, course code, or name fragment
./canvas todo --days 7
./canvas missing
./canvas api users/self/profile
```

## Auth

York sign-in is Canvas → Instructure Identity → York Shibboleth (SAML) → back to Canvas,
so the CLI doesn't replay it. `auth login` instead:

1. Opens its own native macOS window (WKWebView, via cgo) on `/login/saml`. It does not
   drive an external browser, so it works where browser remote debugging is disabled by policy.
   The window uses a non-persistent cookie store; nothing is left behind.
2. Polls the window's cookie store; whenever the Canvas session cookie (`canvas_session` /
   `_normandy_session`) changes, checks it against `GET /api/v1/users/self`.
3. Once accepted, closes the window and calls `POST /api/v1/users/self/tokens` to create a
   personal access token (90 days by default, see `--expires-days`).
4. Stores the token in the macOS Keychain (service `canvas-cli`). If Canvas refuses to
   create a token, it stores the session cookies instead (`--cookies-only` forces this).

`-v` logs cookie names and session checks. `auth login --token <tok>` stores a token you
created under Account → Settings (works on any OS). `auth logout` revokes the CLI-created
token and clears the Keychain entry.

Building needs cgo and the Xcode command line tools (`xcode-select --install`).
