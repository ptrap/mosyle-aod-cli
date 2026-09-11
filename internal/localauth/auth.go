// Package localauth asks macOS to authenticate the desktop user without exposing
// credentials to the CLI. Each confirmation uses a fresh authentication context.
package localauth

import "errors"

var (
	ErrFailed      = errors.New("macOS authentication failed; no request submitted")
	ErrCanceled    = errors.New("macOS authentication canceled; no request submitted")
	ErrUnavailable = errors.New("macOS authentication unavailable; no request submitted")
	ErrTimeout     = errors.New("macOS authentication timed out; no request submitted")
)
