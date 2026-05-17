//go:build darwin

package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

func osWatch(ctx context.Context, path string, onChange func(), done chan struct{}) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	dirFd, err := unix.Open(dir, unix.O_RDONLY|unix.O_EVTONLY, 0)
	if err != nil {
		return fmt.Errorf("open dir %s: %w", dir, err)
	}

	kq, err := unix.Kqueue()
	if err != nil {
		unix.Close(dirFd) //nolint:errcheck
		return fmt.Errorf("kqueue: %w", err)
	}

	const fflags = unix.NOTE_WRITE | unix.NOTE_ATTRIB | unix.NOTE_RENAME | unix.NOTE_DELETE
	change := unix.Kevent_t{
		Filter: unix.EVFILT_VNODE,
		Flags:  unix.EV_ADD | unix.EV_ENABLE | unix.EV_CLEAR,
		Fflags: fflags,
	}
	unix.SetKevent(&change, dirFd, unix.EVFILT_VNODE, unix.EV_ADD|unix.EV_ENABLE|unix.EV_CLEAR)

	if _, err := unix.Kevent(kq, []unix.Kevent_t{change}, nil, nil); err != nil {
		unix.Close(kq)    //nolint:errcheck
		unix.Close(dirFd) //nolint:errcheck
		return fmt.Errorf("kevent register: %w", err)
	}

	base := filepath.Base(path)

	go func() {
		defer close(done)
		defer unix.Close(kq)    //nolint:errcheck
		defer unix.Close(dirFd) //nolint:errcheck

		events := make([]unix.Kevent_t, 4)
		timeout := unix.NsecToTimespec(int64(200 * time.Millisecond))

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			n, err := unix.Kevent(kq, nil, events, &timeout)
			if err != nil || n == 0 {
				continue
			}

			if _, statErr := os.Stat(filepath.Join(dir, base)); statErr == nil {
				select {
				case <-ctx.Done():
					return
				default:
					onChange()
				}
			}
		}
	}()

	return nil
}
