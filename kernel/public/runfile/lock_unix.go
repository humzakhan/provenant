//go:build !windows

package runfile

import (
	"os"
	"syscall"
)

// lockHandle holds the file descriptor used for the POSIX advisory lock.
// On Unix, the lock is held on the .part file itself.
type lockHandle struct {
	f *os.File
}

// acquireLock acquires an exclusive non-blocking advisory lock (flock LOCK_EX|LOCK_NB)
// on the provided file. If the lock is already held by another process,
// ErrConcurrentAppender is returned immediately — no blocking wait.
func acquireLock(f *os.File) (lockHandle, error) {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
			return lockHandle{}, ErrConcurrentAppender
		}
		return lockHandle{}, err
	}
	return lockHandle{f: f}, nil
}

// releaseLock releases the advisory lock held on lh.f.
func releaseLock(lh lockHandle) error {
	if lh.f == nil {
		return nil
	}
	return syscall.Flock(int(lh.f.Fd()), syscall.LOCK_UN)
}
