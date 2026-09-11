package localauth

import (
	"context"
	"errors"
	"testing"
)

func TestCanceledContextDoesNotStartAuthentication(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Confirm(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Confirm() = %v; want context.Canceled", err)
	}
}
