//go:build !linux && !darwin

package config

import (
	"context"
	"os"
	"time"
)

func osWatch(ctx context.Context, path string, onChange func(), done chan struct{}) error {
	go func() {
		defer close(done)
		var lastMod time.Time
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				info, err := os.Stat(path)
				if err != nil {
					continue
				}
				if info.ModTime().After(lastMod) {
					lastMod = info.ModTime()
					onChange()
				}
			}
		}
	}()
	return nil
}
