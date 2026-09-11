package aod

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWait(t *testing.T) {
	calls := 0
	ok, e := Wait(context.Background(), time.Second, time.Millisecond, func(context.Context) (bool, error) { calls++; return calls == 3, nil })
	if !ok || e != nil || calls != 3 {
		t.Fatalf("ok=%v e=%v calls=%d", ok, e, calls)
	}
	_, e = Wait(context.Background(), 5*time.Millisecond, time.Millisecond, func(context.Context) (bool, error) { return false, nil })
	if !errors.Is(e, ErrTimeout) {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e = Wait(ctx, time.Second, time.Millisecond, func(context.Context) (bool, error) { t.Fatal("check after cancellation"); return false, nil })
	if !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
