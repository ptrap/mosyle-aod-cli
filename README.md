# mosyle-aod

An unofficial command-line client for Mosyle Admin On Demand on **Apple Silicon Macs**. Request temporary administrator privileges using your existing Mosyle Self-Service session, with an optional wait for activation.

**Experimental:** the web integration is undocumented. A single Go-only HTTP request was accepted and local elevation was confirmed, without a GUI action or deeplink. Mosyle controls automatic revocation. A user without Mosyle management-portal access still needs independent validation. This project is not affiliated with or endorsed by Mosyle.

## Install

Install the experimental Apple Silicon release through [ptrap/homebrew-tap](https://github.com/ptrap/homebrew-tap):

```sh
brew install ptrap/tap/mosyle-aod
```

The formula installs a prebuilt Apple Silicon binary; users do not need a separate language runtime. Intel Macs are not supported. Alternatively, build from source:

```sh
go build -o mosyle-aod ./cmd/mosyle-aod
./mosyle-aod --help
```

## Usage

Run as your logged-in desktop user, **without sudo**. Mosyle Self-Service must already be installed and signed in; your organization's policy must permit user requests.

```sh
# Local status only. No network request or session access.
mosyle-aod status
mosyle-aod status --json

# Submit once; return when the server accepts the request.
mosyle-aod request --reason "Install development tools"

# Submit once; wait until local admin-group membership is observed.
mosyle-aod request --reason "Install development tools" --wait --timeout 3m

# Wait for an existing request; never submit another one.
mosyle-aod wait --timeout 180s
```

To continue a shell script after activation:

```sh
mosyle-aod request --reason "Maintenance" --wait && sudo your-command
```

`sudo` may still prompt for your password. This tool does not execute commands as root or change group membership itself.

`request` requires confirmation in the native macOS authentication dialog before reading the Mosyle session or contacting Mosyle. Use Touch ID or your Mac login password; the standard macOS policy can also permit Apple Watch confirmation. Passwords and biometric data are handled by macOS and are never read or stored by this tool. Every request creates a fresh authentication context. Cancellation, unavailable authentication, or a two-minute authentication timeout stops submission. Ctrl-C dismisses the pending prompt. There is no flag to skip authentication; unattended requests cannot proceed without confirmation.

`status` and `wait` do not prompt. `request` skips confirmation and submission if the user is already an admin. The policy's duration is read from Mosyle, not fixed to five minutes. A timeout or Ctrl-C does not cancel an accepted request: it may still activate later. Activation waiting defaults to three minutes. Requests are never automatically retried.

## Output and exit codes

All three subcommands accept `--json`, producing one final JSON object on stdout. Ordinary progress and errors go to stderr. Argument-parsing errors use stderr and exit code 2, including with `--json`.

```json
{"state":"active","admin":true,"accepted":false,"expires_at":null}
```

`accepted` means this invocation received acceptance from Mosyle, not that admin rights are active. `admin` reports local membership at the last check. `expires_at` stays null: membership does not reveal whether rights are permanent, their origin, or the remaining elevation time.

| Code | Meaning |
| --- | --- |
| 0 | Admin active, request accepted without waiting, or help/version succeeded |
| 1 | `status`: standard user |
| 2 | Invalid command or arguments |
| 3 | Mosyle session missing or expired |
| 4 | Request denied by policy or server |
| 5 | Activation wait timed out |
| 6 | Submission outcome unknown; inspect status before retrying |
| 7 | Network error before submission, or unsupported response |
| 8 | Unsupported environment or local failure |
| 9 | Local authentication failed, canceled, unavailable, or timed out; no request submitted |
| 130 | Interrupted |

## Session handling and compatibility

The client reads only Mosyle's own binary cookie file in the current OS user's home directory. Cookies and freshly obtained form tokens remain in memory, are sent only to `https://mybusiness.mosyle.com`, and are never included in application logs or saved by the client. HTTP redirects are not followed. No root helper, password storage, policy changes, or background renewal is installed.

Local confirmation is enforced by this CLI, not by the Mosyle server. Another client or modified binary with access to the same session cookies could submit directly. This feature does not establish a server-enforced authentication requirement.

The request flow validates the returned device identifier and checks the profile before submission. Only the observed `AllowUsers` policy flow is implemented. Mosyle School, approval-required profiles, alternate app layouts, and other session formats are not yet supported. The development account's readable cookie session also opened the management portal; do not assume ordinary employee sessions behave identically until tested.

If the session expires, sign in through Mosyle Self-Service. A missing request form can also mean the app's web interface has changed; the CLI fails instead of guessing new endpoints.

## Development

Go 1.27 or later and Apple Command Line Tools (or Xcode), with CGO enabled, are required to build the native LocalAuthentication bridge on macOS. Builds with `CGO_ENABLED=0` can run status/help but refuse requests because native authentication is unavailable. Release builds enable CGO explicitly. The only external module is `golang.org/x/net/html`, used to parse real HTML rather than relying on regular expressions.

```sh
go test -race ./...
go vet ./...
go test ./internal/aod -run '^$' -fuzz FuzzCookies -fuzztime 10s
scripts/release.sh v0.1.0 ptrap/mosyle-aod-cli
```

Tests use synthetic cookies and a local HTTP server; they do not submit live elevation requests. See [release instructions](docs/releasing.md).

License: MIT.
