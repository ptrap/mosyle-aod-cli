package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ptrap/mosyle-aod-cli/internal/aod"
)

type Backend interface {
	Admin(context.Context) (bool, error)
	Request(context.Context, string) (int, error)
}
type Factory func(context.Context) (Backend, error)
type Result struct {
	State     string     `json:"state"`
	Admin     bool       `json:"admin"`
	Accepted  bool       `json:"accepted"`
	Minutes   int        `json:"duration_minutes,omitempty"`
	ExpiresAt *time.Time `json:"expires_at"`
	Error     string     `json:"error,omitempty"`
	Message   string     `json:"message,omitempty"`
}

const help = `mosyle-aod: unofficial Mosyle Admin On Demand CLI

Usage:
  mosyle-aod status [--json]
  mosyle-aod request --reason "Why" [--wait] [--timeout 180s] [--json]
  mosyle-aod wait [--timeout 180s] [--json]
  mosyle-aod version

request returns after acceptance unless --wait is given. wait never submits a request.
status exits 0 for admin, 1 for standard user. Expiry is not inferred from membership.
`

func Run(ctx context.Context, args []string, out, errOut io.Writer, version string, factory Factory) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(out, help)
		return 0
	}
	if args[0] == "version" || args[0] == "--version" {
		if len(args) != 1 {
			fmt.Fprintln(errOut, "version takes no arguments")
			return 2
		}
		fmt.Fprintln(out, "mosyle-aod "+version)
		return 0
	}
	command := args[0]
	if command != "status" && command != "request" && command != "wait" {
		fmt.Fprintln(errOut, "unknown command; use --help")
		return 2
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(errOut)
	jsonMode := fs.Bool("json", false, "output a single JSON result")
	var reason string
	var wait bool
	timeout := 180 * time.Second
	if command == "request" {
		fs.StringVar(&reason, "reason", "", "justification recorded by Mosyle")
		fs.BoolVar(&wait, "wait", false, "wait until local admin rights are active")
	}
	if command != "status" {
		fs.DurationVar(&timeout, "timeout", 180*time.Second, "maximum activation wait, e.g. 180s or 3m")
	}
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(errOut, "unexpected positional arguments")
		return 2
	}
	if timeout <= 0 || timeout > time.Hour {
		fmt.Fprintln(errOut, "timeout must be positive and at most 1h")
		return 2
	}
	if command == "request" && strings.TrimSpace(reason) == "" {
		fmt.Fprintln(errOut, "request requires --reason")
		return 2
	}
	result := Result{}
	finish := func(code int, e error) int {
		if e != nil {
			result.Error = errorKind(e)
			result.Message = e.Error()
			if result.State == "" {
				result.State = "error"
			}
		}
		if *jsonMode {
			if json.NewEncoder(out).Encode(result) != nil {
				return 8
			}
		} else if e != nil {
			fmt.Fprintln(errOut, result.Message)
		} else {
			switch result.State {
			case "active":
				fmt.Fprintln(out, "Admin rights are active.")
			case "standard":
				fmt.Fprintln(out, "Standard user.")
			case "accepted":
				fmt.Fprintf(out, "Mosyle accepted the request (%d minutes); activation is not yet confirmed.\n", result.Minutes)
			}
		}
		return code
	}
	backend, e := factory(ctx)
	if e != nil {
		return finish(errorCode(e), e)
	}
	active, e := backend.Admin(ctx)
	if e != nil {
		return finish(errorCode(e), e)
	}
	result.Admin = active
	if active {
		result.State = "active"
		return finish(0, nil)
	}
	result.State = "standard"
	if command == "status" {
		return finish(1, nil)
	}
	if command == "request" {
		minutes, e := backend.Request(ctx, reason)
		if errors.Is(e, aod.ErrAlreadyActive) {
			result.State = "active"
			result.Admin = true
			return finish(0, nil)
		}
		if e != nil {
			return finish(errorCode(e), e)
		}
		result.Accepted = true
		result.Minutes = minutes
		result.State = "accepted"
		if !wait {
			return finish(0, nil)
		}
	}
	if !*jsonMode {
		fmt.Fprintln(errOut, "Waiting for Mosyle activation...")
	}
	active, e = aod.Wait(ctx, timeout, 2*time.Second, backend.Admin)
	result.Admin = active
	if e != nil {
		return finish(errorCode(e), e)
	}
	result.State = "active"
	return finish(0, nil)
}
func errorKind(e error) string {
	switch {
	case errors.Is(e, aod.ErrSession):
		return "session"
	case errors.Is(e, aod.ErrDenied):
		return "denied"
	case errors.Is(e, aod.ErrTimeout):
		return "timeout"
	case errors.Is(e, aod.ErrUncertain):
		return "unknown_outcome"
	case errors.Is(e, aod.ErrNetwork):
		return "network"
	case errors.Is(e, aod.ErrProtocol):
		return "unsupported_response"
	case errors.Is(e, aod.ErrPlatform), errors.Is(e, aod.ErrUser):
		return "environment"
	case errors.Is(e, context.Canceled):
		return "interrupted"
	default:
		return "local_error"
	}
}
func errorCode(e error) int {
	switch errorKind(e) {
	case "session":
		return 3
	case "denied":
		return 4
	case "timeout":
		return 5
	case "unknown_outcome":
		return 6
	case "network", "unsupported_response":
		return 7
	case "interrupted":
		return 130
	default:
		return 8
	}
}
