# mosyle-aod

An unofficial command-line client for Mosyle Admin On Demand on **Apple Silicon Macs**. Request temporary administrator privileges using device-based Mosyle authentication, with an optional wait for activation and a saved Self-Service session fallback.

**Experimental:** the integration is undocumented. A fresh native session was used to request elevation on an enrolled Mac; local admin rights activated after about 13 seconds. The app was already running during that test, so activation with Self-Service closed remains unverified. Mosyle controls automatic revocation. This project is not affiliated with or endorsed by Mosyle.

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

Run as your logged-in desktop user, **without sudo**. Mosyle Self-Service must be installed and configured for your enrolled Mac; your organization's policy must permit user requests. On the supported device-authentication flow, no Self-Service launch or email/password login is needed to acquire a session. Other configurations require a valid saved Self-Service session.

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

The client first attempts native device authentication. It reads the installed Self-Service executable's bundled application constants, the configured account URL from the current user's Mosyle preferences, and the local device identifier. It calls `device_info`, requires a matching Mac with `NO_AUTH` and no password requirement, then exchanges the assigned-user token for a fresh in-memory Self-Service session.

Native extraction has no version or binary-hash pin. It tries the known ARM64 string locations after checking the executable format, segment bounds, and string layout. Updates that preserve those locations can work immediately; moved or incompatible constants may require a future extractor update. If extraction, native login, or native session preparation fails, the client tries Mosyle's saved binary cookies once. Both paths validate the device and policy before submission. Cancellation stops the flow, and fallback never occurs after an elevation submission.

Tokens and cookies are sent only to `https://mybusiness.mosyle.com`, remain in memory, and are never included in application logs or saved by the client. HTTP redirects are not followed. No app launch, root helper, password storage, policy changes, or background renewal is installed.

Local confirmation is enforced by this CLI, not by the Mosyle server. Another client or modified binary with access to the same device-authentication inputs or session cookies could submit directly. This feature does not establish a server-enforced authentication requirement.

The request flow validates the returned device identifier and checks the profile before submission. Only the observed `AllowUsers` policy flow is implemented. Mosyle School, approval-required profiles, alternate app layouts, and other session formats are not yet supported. The development account's readable cookie session also opened the management portal; do not assume ordinary employee sessions behave identically until tested.

Supported native authentication obtains a fresh session on each request. If native authentication is unsupported and the saved session expires, sign in through Mosyle Self-Service. A missing request form can also mean the app's web interface has changed; the CLI fails instead of guessing new endpoints.

## Development

Go 1.27 or later and Apple Command Line Tools (or Xcode), with CGO enabled, are required to build the native LocalAuthentication bridge on macOS. Builds with `CGO_ENABLED=0` can run status/help but refuse requests because native authentication is unavailable. Release builds enable CGO explicitly. The only external module is `golang.org/x/net/html`, used to parse real HTML rather than relying on regular expressions.

```sh
go test -race ./...
go vet ./...
go test ./internal/aod -run '^$' -fuzz FuzzCookies -fuzztime 10s
scripts/release.sh v0.1.0 ptrap/mosyle-aod-cli
```

Tests use synthetic authentication responses and cookies with a local HTTP server; they do not submit live elevation requests. An optional installed-app check validates native login and policy without submitting elevation:

```sh
MOSYLE_AOD_NATIVE_LIVE_TEST=1 go test ./internal/aod -run '^TestNativeInstalledPreparation$' -v -count=1
```

See [release instructions](docs/releasing.md).

License: MIT.
