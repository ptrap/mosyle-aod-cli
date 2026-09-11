package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/ptrap/mosyle-aod-cli/internal/aod"
	"github.com/ptrap/mosyle-aod-cli/internal/localauth"
	"testing"
)

type fake struct {
	states        []bool
	requests      int
	requestErr    error
	confirmErr    error
	confirmations int
	afterConfirm  func()
}

func (f *fake) Admin(context.Context) (bool, error) {
	v := f.states[0]
	if len(f.states) > 1 {
		f.states = f.states[1:]
	}
	return v, nil
}
func (f *fake) Confirm(context.Context) error {
	f.confirmations++
	if f.afterConfirm != nil {
		f.afterConfirm()
	}
	return f.confirmErr
}
func (f *fake) Request(context.Context, string) (int, error) { f.requests++; return 10, f.requestErr }
func TestCommands(t *testing.T) {
	for _, tt := range []struct {
		name           string
		args           []string
		states         []bool
		code, requests int
		state          string
		err            error
	}{
		{"status standard", []string{"status", "--json"}, []bool{false}, 1, 0, "standard", nil},
		{"status admin", []string{"status", "--json"}, []bool{true}, 0, 0, "active", nil},
		{"request async", []string{"request", "--reason", "test", "--json"}, []bool{false}, 0, 1, "accepted", nil},
		{"request waiting", []string{"request", "--reason", "test", "--wait", "--json"}, []bool{false, true}, 0, 1, "active", nil},
		{"already admin", []string{"request", "--reason", "test", "--json"}, []bool{true}, 0, 0, "active", nil},
		{"activated during prepare", []string{"request", "--reason", "test", "--json"}, []bool{false}, 0, 1, "active", aod.ErrAlreadyActive},
		{"wait only", []string{"wait", "--json"}, []bool{false, true}, 0, 0, "active", nil},
		{"timeout", []string{"request", "--reason", "test", "--wait", "--timeout", "1ms", "--json"}, []bool{false}, 5, 1, "accepted", nil},
		{"unknown outcome", []string{"request", "--reason", "test", "--json"}, []bool{false}, 6, 1, "standard", aod.ErrUncertain},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := &fake{states: tt.states, requestErr: tt.err}
			var out, stderr bytes.Buffer
			code := Run(context.Background(), tt.args, &out, &stderr, "test", func(context.Context) (Backend, error) { return f, nil })
			var result Result
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal(err, out.String())
			}
			if code != tt.code || f.requests != tt.requests || result.State != tt.state {
				t.Fatalf("code=%d requests=%d result=%+v", code, f.requests, result)
			}
			if f.confirmations != tt.requests {
				t.Fatalf("confirmations=%d requests=%d", f.confirmations, tt.requests)
			}
			if result.ExpiresAt != nil {
				t.Fatal("must not invent expiry")
			}
			if tt.name == "timeout" && !result.Accepted {
				t.Fatal("timeout must retain acceptance")
			}
		})
	}
}
func TestInvalidArgumentsNeverAccessSession(t *testing.T) {
	for _, args := range [][]string{{"request"}, {"wait", "--timeout", "0s"}, {"status", "--reason", "x"}, {"request", "--reason", "x", "extra"}} {
		var out bytes.Buffer
		code := Run(context.Background(), args, &out, &out, "test", func(context.Context) (Backend, error) {
			t.Fatal("backend called")
			return nil, errors.New("unexpected")
		})
		if code != 2 {
			t.Fatal(code)
		}
	}
}
func TestHelpDoesNotNeedMac(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"version"}} {
		var out bytes.Buffer
		if Run(context.Background(), args, &out, &out, "test", func(context.Context) (Backend, error) { t.Fatal("backend called"); return nil, nil }) != 0 {
			t.Fatal(out.String())
		}
	}
}

func TestAuthenticationFailureNeverSubmits(t *testing.T) {
	for _, err := range []error{localauth.ErrFailed, localauth.ErrCanceled, localauth.ErrUnavailable, localauth.ErrTimeout} {
		t.Run(err.Error(), func(t *testing.T) {
			f := &fake{states: []bool{false}, confirmErr: err}
			var out, stderr bytes.Buffer
			code := Run(context.Background(), []string{"request", "--reason", "test", "--wait", "--json"}, &out, &stderr, "test", func(context.Context) (Backend, error) { return f, nil })
			var result Result
			if e := json.Unmarshal(out.Bytes(), &result); e != nil {
				t.Fatal(e)
			}
			if code != 9 || f.requests != 0 || f.confirmations != 1 || result.Accepted || result.Error != "authentication" {
				t.Fatalf("code=%d requests=%d confirmations=%d result=%+v", code, f.requests, f.confirmations, result)
			}
		})
	}
}

func TestCancellationAtConfirmationNeverSubmits(t *testing.T) {
	for _, during := range []bool{true, false} {
		ctx, cancel := context.WithCancel(context.Background())
		f := &fake{states: []bool{false}, afterConfirm: cancel}
		if during {
			f.confirmErr = context.Canceled
		}
		var out bytes.Buffer
		code := Run(ctx, []string{"request", "--reason", "test", "--json"}, &out, &out, "test", func(context.Context) (Backend, error) { return f, nil })
		cancel()
		if code != 130 || f.requests != 0 {
			t.Fatalf("code=%d requests=%d", code, f.requests)
		}
	}
}
