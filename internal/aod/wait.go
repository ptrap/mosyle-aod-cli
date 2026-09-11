package aod

import (
	"context"
	"time"
)

// Wait only observes local membership. It never submits or renews a request.
func Wait(ctx context.Context, timeout, interval time.Duration, check func(context.Context) (bool, error)) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		if err := ctx.Err(); err != nil {
			if err == context.DeadlineExceeded {
				return false, ErrTimeout
			}
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			return false, err
		}
		active, err := check(ctx)
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return false, ErrTimeout
			}
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			return false, err
		}
		if active {
			return true, nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			if ctx.Err() == context.DeadlineExceeded {
				return false, ErrTimeout
			}
			return false, ctx.Err()
		case <-timer.C:
		}
	}
}
