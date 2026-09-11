//go:build darwin && cgo

package localauth

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework LocalAuthentication
#include <stdlib.h>
#include "auth.h"
*/
import "C"

import (
	"context"
	"time"
	"unsafe"
)

// Confirm requires fresh user authentication and bounds the lifetime of the UI.
// Cancellation invalidates the native context, dismissing any pending prompt.
func Confirm(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	reason := C.CString("confirm a request for temporary administrator privileges")
	defer C.free(unsafe.Pointer(reason))
	handle := C.aod_auth_begin(reason)
	defer C.aod_auth_release(handle)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(2 * time.Minute)
	defer timeout.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch C.aod_auth_poll(handle) {
		case C.AOD_AUTH_SUCCESS:
			return nil
		case C.AOD_AUTH_CANCELED:
			return ErrCanceled
		case C.AOD_AUTH_UNAVAILABLE:
			return ErrUnavailable
		case C.AOD_AUTH_FAILED:
			return ErrFailed
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return ErrTimeout
		case <-ticker.C:
		}
	}
}
