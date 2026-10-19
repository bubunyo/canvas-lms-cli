# canvas-cli

A command-line client for [Canvas LMS](https://www.instructure.com/canvas): list courses,
modules, upcoming work and missing submissions, and pull module content (pages,
assignments, files) to disk as markdown.

Defaults to `https://onlinestudy.york.ac.uk`; point it at any Canvas instance with
`--base-url` or `CANVAS_BASE_URL`.

## Install

### Script (macOS, Linux, Git Bash on Windows)

```bash
curl -fsSL https://raw.githubusercontent.com/bubunyo/canvas-lms-cli/main/install.sh | sh
```

Downloads the latest release for your OS/architecture, verifies its SHA-256 checksum,
and installs `canvas-cli` to `~/.local/bin`. Options:

```bash
curl -fsSL https://raw.githubusercontent.com/bubunyo/canvas-lms-cli/main/install.sh | CANVAS_CLI_VERSION=v0.1.0 INSTALL_DIR=/usr/local/bin sh
```

### Manual download

Grab the archive for your platform from
[Releases](https://github.com/bubunyo/canvas-lms-cli/releases), check it against
`checksums.txt`, extract, and put `canvas-cli` (or `canvas-cli.exe`) on your `PATH`.

| Platform | Asset |
|---|---|
| macOS, Apple Silicon | `canvas-cli_darwin_arm64.tar.gz` |
| macOS, Intel | `canvas-cli_darwin_amd64.tar.gz` |
| Linux x86-64 / ARM64 | `canvas-cli_linux_amd64.tar.gz` / `canvas-cli_linux_arm64.tar.gz` |
| Windows x86-64 / ARM64 | `canvas-cli_windows_amd64.zip` / `canvas-cli_windows_arm64.zip` |

On Windows (PowerShell), for example:

```powershell
Invoke-WebRequest https://github.com/bubunyo/canvas-lms-cli/releases/latest/download/canvas-cli_windows_amd64.zip -OutFile canvas-cli.zip
Expand-Archive canvas-cli.zip -DestinationPath "$env:LOCALAPPDATA\canvas-cli"
# then add %LOCALAPPDATA%\canvas-cli to your PATH
```

The binaries are not code-signed. If macOS blocks a binary you downloaded with a
browser, run `xattr -d com.apple.quarantine canvas-cli` (the install script does this for you).

### With Go

```bash
go install github.com/bubunyo/canvas-lms-cli/cmd/canvas-cli@latest
```

The macOS build needs cgo and the Xcode command line tools (`xcode-select --install`)
for the login window. From a checkout, `make install` or `./install.sh --source` does the same.

## Usage

```bash
canvas-cli auth login                 # sign in (see Authentication below)
canvas-cli auth status
canvas-cli courses [--all]
canvas-cli modules COM00150M          # course id, course code, or name fragment
canvas-cli todo --days 7              # upcoming planner items
canvas-cli missing                    # past-due, unsubmitted assignments
canvas-cli pull COM00150M "week 1"    # save module content as markdown
canvas-cli pull COM00150M -o ~/Documents/york   # every module
canvas-cli api users/self/profile     # raw GET against any API path
```

Every command accepts `--json` for raw output.

### Pulling content

`pull` writes `<out>/<COURSE CODE>/<NN - module>/`, with one markdown file per page,
assignment, quiz or discussion prompt (not other students' replies), downloaded file
attachments, and a `README.md` index. Images and documents linked from pages are saved
under `files/` and the links rewritten to the local copies. Modules are matched by id or
name fragment; with none given, all modules are pulled. Re-running skips files already
downloaded; `--no-files` skips downloads entirely.

## Authentication

York's sign-in is Canvas → Instructure Identity → York Shibboleth (SAML, MFA) → Canvas,
which can't be replayed by a script, so on macOS `auth login`:

1. Opens a native login window (WebKit, via cgo) on `/login/saml`. It doesn't drive an
   external browser, so it works where browser remote debugging is disabled by policy,
   and it keeps no cookies afterwards.
2. Watches the window's cookies; when the Canvas session cookie changes it checks it
   against `GET /api/v1/users/self`, and closes the window once it's accepted.
3. Tries `POST /api/v1/users/self/tokens` to create a personal access token
   (`--expires-days`, default 90) and stores it in the OS keychain.
4. If the institution doesn't allow students to create tokens (York doesn't), it stores
   the session cookies in the keychain instead. Run `auth login` again when they expire.

On Linux and Windows, or anywhere you have a token from Account → Settings →
New Access Token:

```bash
canvas-cli auth login --token -      # paste the token on stdin
```

`auth login -v` logs cookie names and session checks. `auth logout` revokes a
CLI-created token and clears the keychain entry.

## Development

```bash
make build      # ./canvas-cli
make test
make install    # go install with version from git describe
```

Releases are built by [`.github/workflows/release.yml`](.github/workflows/release.yml)
when a `v*` tag is pushed: tests run on macOS, macOS binaries are built with cgo on a
macOS runner (arm64 + amd64), Linux and Windows binaries are cross-compiled without cgo
(amd64 + arm64), and everything is published to a GitHub release with `checksums.txt`.

```bash
git tag v0.1.0 && git push origin v0.1.0
```
