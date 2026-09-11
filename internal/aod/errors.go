package aod

import "errors"

var (
	ErrAlreadyActive = errors.New("admin rights became active before submission")
	ErrSession       = errors.New("Mosyle session unavailable or expired; sign in to Self-Service")
	ErrProtocol      = errors.New("unexpected Mosyle response; this app version or session may not be supported")
	ErrDenied        = errors.New("Mosyle does not allow this request; check your organization's policy")
	ErrNetwork       = errors.New("could not contact Mosyle")
	ErrUncertain     = errors.New("request outcome is unknown; check status or Self-Service before retrying")
	ErrTimeout       = errors.New("admin rights not observed before timeout; an accepted request may still activate later")
	ErrPlatform      = errors.New("this command requires macOS on Apple Silicon")
	ErrUser          = errors.New("run as the logged-in desktop user, without sudo")
)
