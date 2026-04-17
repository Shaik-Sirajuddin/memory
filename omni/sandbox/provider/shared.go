package provider

import (
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type ProvisionerState struct {
	mu        sync.Mutex
	sandboxes map[string]*Sandbox
	store     SandboxStore
}

func NewProvisionerState(store SandboxStore) ProvisionerState {
	return ProvisionerState{sandboxes: make(map[string]*Sandbox), store: store}
}

func (s *ProvisionerState) Create(kind ProvisionerKind, params CreateSandboxParams, fallback *Sandbox) (*Sandbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(params.ID) == "" {
		return nil, providerErrorf("id is required")
	}
	if _, exists := s.sandboxes[params.ID]; exists {
		return nil, providerErrorf("%s already exists", params.ID)
	}

	cfg := params.Config
	if cfg == nil && fallback != nil {
		cfg = fallback.Config
	}
	sbx := &Sandbox{
		Config: CloneConfig(cfg),
		State: &State{
			PID:    params.ID,
			Active: true,
		},
		Data: &Data{
			ID:          params.ID,
			Application: string(kind),
			CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		},
	}
	s.sandboxes[params.ID] = sbx
	if s.store != nil {
		if err := s.store.Create(sbx); err != nil {
			delete(s.sandboxes, params.ID)
			return nil, err
		}
	}
	return CloneSandbox(sbx), nil
}

func (s *ProvisionerState) Get(params *GetSandboxParams) (*Sandbox, error) {
	s.mu.Lock()
	for _, sbx := range s.sandboxes {
		if params != nil {
			if params.Active && (sbx.State == nil || !sbx.State.Active) {
				continue
			}
			if params.PID != nil && (sbx.State == nil || sbx.State.PID != *params.PID) {
				continue
			}
			if params.Name != nil && (sbx.Data == nil || sbx.Data.ID != *params.Name) {
				continue
			}
		}
		s.mu.Unlock()
		return CloneSandbox(sbx), nil
	}
	store := s.store
	s.mu.Unlock()
	if store != nil {
		sbx, err := store.Get(params)
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		if sbx != nil && sbx.Data != nil {
			s.sandboxes[sbx.Data.ID] = CloneSandbox(sbx)
		}
		s.mu.Unlock()
		return sbx, nil
	}
	return nil, NoProcessFound
}

func (s *ProvisionerState) List(params ListSandboxParams) ([]*Sandbox, error) {
	if s.store != nil {
		stored, err := s.store.List()
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		s.sandboxes = make(map[string]*Sandbox, len(stored))
		for _, sbx := range stored {
			if sbx != nil && sbx.Data != nil {
				s.sandboxes[sbx.Data.ID] = CloneSandbox(sbx)
			}
		}
		s.mu.Unlock()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]*Sandbox, 0, len(s.sandboxes))
	for _, sbx := range s.sandboxes {
		if params.Active && (sbx.State == nil || !sbx.State.Active) {
			continue
		}
		out = append(out, CloneSandbox(sbx))
	}
	return out, nil
}

func (s *ProvisionerState) SyncConfig(config *Config) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, sbx := range s.sandboxes {
		sbx.Config = CloneConfig(config)
		if s.store != nil {
			_ = s.store.Update(sbx)
		}
	}
}

func (s *ProvisionerState) SyncOne(id string, config *Config) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sbx, ok := s.sandboxes[id]
	if !ok {
		return
	}
	sbx.Config = CloneConfig(config)
	if s.store != nil {
		_ = s.store.Update(sbx)
	}
}

func CloneConfig(cfg *Config) *Config {
	if cfg == nil {
		return nil
	}
	out := &Config{}
	if cfg.WorkSpacePolicy != nil {
		out.WorkSpacePolicy = ClonePolicy(cfg.WorkSpacePolicy)
	}
	if cfg.AgentPolicy != nil {
		out.AgentPolicy = ClonePolicy(cfg.AgentPolicy)
	}
	return out
}

func ClonePolicy(p *Policy) *Policy {
	if p == nil {
		return nil
	}
	return &Policy{
		Dir:      p.Dir,
		FSPolicy: p.FSPolicy,
		Config: MountConfig{
			AccessDirs:  append([]string{}, p.Config.AccessDirs...),
			BlockedDirs: append([]string{}, p.Config.BlockedDirs...),
		},
	}
}

func CloneSandbox(sbx *Sandbox) *Sandbox {
	if sbx == nil {
		return nil
	}
	out := &Sandbox{
		Config: CloneConfig(sbx.Config),
	}
	if sbx.State != nil {
		out.State = &State{PID: sbx.State.PID, Active: sbx.State.Active}
	}
	if sbx.Data != nil {
		out.Data = &Data{ID: sbx.Data.ID, Application: sbx.Data.Application, CreatedAt: sbx.Data.CreatedAt}
	}
	return out
}

func SandboxAllowsWrite(sbx *Sandbox) bool {
	if sbx == nil || sbx.Config == nil || sbx.AgentPolicy == nil {
		return false
	}
	switch AgentFSPolicy(sbx.AgentPolicy.FSPolicy) {
	case AllPermissiveRead, Inherit:
		return true
	default:
		return false
	}
}

func SandboxAccessDirs(sbx *Sandbox) []string {
	var dirs []string
	if sbx == nil || sbx.Config == nil {
		return dirs
	}
	if sbx.WorkSpacePolicy != nil {
		dirs = append(dirs, sbx.WorkSpacePolicy.Config.AccessDirs...)
	}
	if sbx.AgentPolicy != nil {
		dirs = append(dirs, sbx.AgentPolicy.Config.AccessDirs...)
	}
	return UniqueCleaned(dirs)
}

func SandboxBlockedDirs(sbx *Sandbox) []string {
	var dirs []string
	if sbx == nil || sbx.Config == nil {
		return dirs
	}
	if sbx.WorkSpacePolicy != nil {
		dirs = append(dirs, sbx.WorkSpacePolicy.Config.BlockedDirs...)
	}
	if sbx.AgentPolicy != nil {
		dirs = append(dirs, sbx.AgentPolicy.Config.BlockedDirs...)
	}
	return UniqueCleaned(dirs)
}

func UniqueCleaned(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, dir := range in {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		cleanDir := filepath.Clean(dir)
		if _, ok := seen[cleanDir]; ok {
			continue
		}
		seen[cleanDir] = struct{}{}
		out = append(out, cleanDir)
	}
	return out
}

func providerErrorf(format string, args ...any) error {
	return errorsNew("sandbox: " + sprintf(format, args...))
}
