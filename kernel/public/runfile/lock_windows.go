//go:build windows

package runfile

import "os"

// lockHandle is a stub on Windows. File locking is not implemented.
type lockHandle struct{}

// acquireLock always returns ErrUnsupportedPlatform on Windows.
func acquireLock(_ *os.File) (lockHandle, error) {
	return lockHandle{}, ErrUnsupportedPlatform
}

// releaseLock is a no-op on Windows.
func releaseLock(_ lockHandle) error {
	return nil
}
