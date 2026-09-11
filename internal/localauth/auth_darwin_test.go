//go:build darwin && cgo

package localauth

import (
	"context"
	"errors"
	"os"
	"testing"
)

// This opt-in test exercises the real dialog without contacting Mosyle.
// Set MOSYLE_TEST_LOCAL_AUTH=success to authenticate, or cancel to test Cancel.
func TestInteractiveConfirmation(t *testing.T) {
	want := os.Getenv("MOSYLE_TEST_LOCAL_AUTH")
	if want == "" {
		t.Skip("set MOSYLE_TEST_LOCAL_AUTH=success or cancel to test the native dialog")
	}
	if want != "success" && want != "cancel" {
		t.Fatal("MOSYLE_TEST_LOCAL_AUTH must be success or cancel")
	}
	err := Confirm(context.Background())
	if want == "cancel" {
		if !errors.Is(err, ErrCanceled) {
			t.Fatalf("Confirm() = %v; want ErrCanceled", err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
}
