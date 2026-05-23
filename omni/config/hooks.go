package config

// AddHook adds a HookEntry for the given event name.
// Returns false without writing if an identical entry (same URL or same Command) already exists.
func (r *DefaultOmniConfigResolver) AddHook(eventName string, entry HookEntry) (bool, error) {
	cfg, err := r.GetUserSettings()
	if err != nil {
		return false, err
	}

	if cfg.Agent == nil {
		cfg.Agent = &Settings{}
	}
	if cfg.Agent.Hooks == nil {
		cfg.Agent.Hooks = make(map[string][]HookEntry)
	}

	for _, existing := range cfg.Agent.Hooks[eventName] {
		if hookEntriesEqual(existing, entry) {
			return false, nil
		}
	}

	cfg.Agent.Hooks[eventName] = append(cfg.Agent.Hooks[eventName], entry)
	return true, r.SaveUserSettings(cfg)
}

// AddHooks adds multiple HookEntries across one or more event names.
// Each entry is skipped if an identical one already exists for that event.
// Returns the count of entries actually added.
func (r *DefaultOmniConfigResolver) AddHooks(hooks map[string][]HookEntry) (int, error) {
	cfg, err := r.GetUserSettings()
	if err != nil {
		return 0, err
	}

	if cfg.Agent == nil {
		cfg.Agent = &Settings{}
	}
	if cfg.Agent.Hooks == nil {
		cfg.Agent.Hooks = make(map[string][]HookEntry)
	}

	added := 0
	for eventName, entries := range hooks {
		for _, entry := range entries {
			duplicate := false
			for _, existing := range cfg.Agent.Hooks[eventName] {
				if hookEntriesEqual(existing, entry) {
					duplicate = true
					break
				}
			}
			if !duplicate {
				cfg.Agent.Hooks[eventName] = append(cfg.Agent.Hooks[eventName], entry)
				added++
			}
		}
	}

	if added == 0 {
		return 0, nil
	}
	return added, r.SaveUserSettings(cfg)
}

// RemoveHook removes all entries matching the given HookEntry from the event.
// Returns true if at least one entry was removed.
func (r *DefaultOmniConfigResolver) RemoveHook(eventName string, entry HookEntry) (bool, error) {
	cfg, err := r.GetUserSettings()
	if err != nil {
		return false, err
	}

	if cfg.Agent == nil || cfg.Agent.Hooks == nil {
		return false, nil
	}

	existing := cfg.Agent.Hooks[eventName]
	filtered := existing[:0]
	for _, e := range existing {
		if !hookEntriesEqual(e, entry) {
			filtered = append(filtered, e)
		}
	}

	if len(filtered) == len(existing) {
		return false, nil
	}

	cfg.Agent.Hooks[eventName] = filtered
	return true, r.SaveUserSettings(cfg)
}

// hookEntriesEqual returns true if two HookEntry values refer to the same hook.
// Matches on URL (for webhook hooks) or Command (for subprocess hooks).
func hookEntriesEqual(a, b HookEntry) bool {
	if a.Url != nil && b.Url != nil {
		return *a.Url == *b.Url
	}
	if a.Command != nil && b.Command != nil {
		return *a.Command == *b.Command
	}
	return false
}
