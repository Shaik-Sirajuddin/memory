package config

import (
	"context"
	"fmt"
)

// ConfigWatcher watches the omni config file for changes.
type ConfigWatcher interface {
	// WatchSettings registers onChange; replaces any prior watcher.
	WatchSettings(onChange func(*OmniConfig)) error
	// Unwatch stops the active watcher and releases resources.
	Unwatch()
}

// WatchSettings starts an OS-native file watcher on the omni config file.
// Calling WatchSettings again replaces the existing watcher.
func (r *DefaultOmniConfigResolver) WatchSettings(onChange func(*OmniConfig)) error {
	path, err := r.UserConfigPath()
	if err != nil {
		return fmt.Errorf("config: watch: resolve path: %w", err)
	}

	r.watchMu.Lock()
	r.stopWatchLocked()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	r.watchCancel = cancel
	r.watchDone = done
	r.watchMu.Unlock()

	notify := func() {
		cfg, err := r.GetUserSettings()
		if err != nil {
			return
		}
		onChange(cfg)
	}

	if err := osWatch(ctx, path, notify, done); err != nil {
		cancel()
		return fmt.Errorf("config: watch: %w", err)
	}

	return nil
}

// Unwatch stops the active watcher goroutine and waits for it to exit.
func (r *DefaultOmniConfigResolver) Unwatch() {
	r.watchMu.Lock()
	r.stopWatchLocked()
	r.watchMu.Unlock()
}

// stopWatchLocked cancels and waits for any running watcher. Caller must hold watchMu.
func (r *DefaultOmniConfigResolver) stopWatchLocked() {
	if r.watchCancel != nil {
		r.watchCancel()
		r.watchCancel = nil
	}
	if r.watchDone != nil {
		<-r.watchDone
		r.watchDone = nil
	}
}
