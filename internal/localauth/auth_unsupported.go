//go:build !darwin || !cgo

package localauth

import "context"

// Builds without the native bridge fail closed rather than skipping confirmation.
func Confirm(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrUnavailable
}
