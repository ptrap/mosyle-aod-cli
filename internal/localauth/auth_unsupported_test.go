//go:build !darwin || !cgo

package localauth

import (
	"context"
	"errors"
	"testing"
)

func TestMissingNativeBridgeFailsClosed(t *testing.T) {
	if err := Confirm(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Confirm() = %v; want ErrUnavailable", err)
	}
}
