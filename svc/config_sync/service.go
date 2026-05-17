package configsync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"

	"github.com/Shaik-Sirajuddin/memory/config"
	defaulthooks "github.com/Shaik-Sirajuddin/memory/config/hooks/default"
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent"
)

var (
	ErrMissingResolver = errors.New("configsync: missing config resolver")
	ErrMissingAgentID  = errors.New("configsync: missing agent id")
	ErrMissingHook     = errors.New("configsync: missing hook transformer")
)

// Agent describes an active agent that can receive synchronized hooks.
type Agent struct {
	ID          string
	Transformer codeagent.HookTransformer
}

// AgentRegistry lists active agents owned by the operator.
type AgentRegistry interface {
	ListActiveAgents(context.Context) ([]Agent, error)
}

// ConfigSyncService watches omni config and keeps each agent's hooks in sync.
type ConfigSyncService interface {
	Start(ctx context.Context) error
	Stop()
	RegisterAgent(agentID string, t codeagent.HookTransformer) error
	UnregisterAgent(agentID string)
	Transformer(agentID string) (codeagent.HookTransformer, bool)
}

type ServiceOptions struct {
	Resolver      config.OmniConfigResolver
	Agents        AgentRegistry
	Registry      HookParserRegistry
	BinaryPath    string
	WatchSettings bool
}

type service struct {
	resolver      config.OmniConfigResolver
	agents        AgentRegistry
	registry      HookParserRegistry
	binaryPath    string
	watchSettings bool

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
}

// NewService creates a config sync service.
func NewService(opts ServiceOptions) (ConfigSyncService, error) {
	if opts.Resolver == nil {
		return nil, ErrMissingResolver
	}
	reg := opts.Registry
	if reg == nil {
		reg = NewRegistry()
	}
	binaryPath := opts.BinaryPath
	if binaryPath == "" {
		path, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("configsync: resolve executable: %w", err)
		}
		binaryPath = path
	}
	return &service{
		resolver:      opts.Resolver,
		agents:        opts.Agents,
		registry:      reg,
		binaryPath:    binaryPath,
		watchSettings: opts.WatchSettings,
	}, nil
}

func (s *service) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.started = true
	s.mu.Unlock()

	if err := s.registerActiveAgents(runCtx); err != nil {
		s.Stop()
		return err
	}
	if err := s.syncRegisteredAgents(); err != nil {
		s.Stop()
		return err
	}
	if s.watchSettings {
		if err := s.resolver.WatchSettings(func(*config.OmniConfig) {
			_ = s.syncRegisteredAgents()
		}); err != nil {
			s.Stop()
			return err
		}
	}

	go func() {
		<-runCtx.Done()
		s.Stop()
	}()

	return nil
}

func (s *service) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	wasStarted := s.started
	s.started = false
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if wasStarted && s.watchSettings {
		s.resolver.Unwatch()
	}
}

func (s *service) RegisterAgent(agentID string, t codeagent.HookTransformer) error {
	if agentID == "" {
		return ErrMissingAgentID
	}
	if t == nil {
		return ErrMissingHook
	}
	s.registry.Register(agentID, t)
	return s.syncAgent(agentID, t)
}

func (s *service) UnregisterAgent(agentID string) {
	s.registry.Unregister(agentID)
}

func (s *service) Transformer(agentID string) (codeagent.HookTransformer, bool) {
	return s.registry.Get(agentID)
}

func (s *service) registerActiveAgents(ctx context.Context) error {
	if s.agents == nil {
		return nil
	}
	agents, err := s.agents.ListActiveAgents(ctx)
	if err != nil {
		return fmt.Errorf("configsync: list active agents: %w", err)
	}
	for _, agent := range agents {
		if agent.ID == "" || agent.Transformer == nil {
			continue
		}
		s.registry.Register(agent.ID, agent.Transformer)
	}
	return nil
}

func (s *service) syncRegisteredAgents() error {
	var errs []error
	for agentID, transformer := range s.registry.List() {
		if err := s.syncAgent(agentID, transformer); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *service) syncAgent(agentID string, t codeagent.HookTransformer) error {
	cfg, err := s.resolver.GetUserSettings()
	if err != nil {
		return fmt.Errorf("configsync: read user settings: %w", err)
	}
	for _, hook := range s.defaultHooks(agentID) {
		t.Add(hook.Name, hook.Entry)
	}
	for name, entry := range configuredHooks(cfg) {
		t.Add(name, entry)
	}
	return nil
}

func (s *service) defaultHooks(agentID string) []defaulthooks.DefaultHook {
	hooks := defaulthooks.All(s.binaryPath)
	for i := range hooks {
		hooks[i].Entry.Args = withAgentFlag(hooks[i].Entry.Args, agentID)
	}
	return hooks
}

func configuredHooks(cfg *config.OmniConfig) map[string]config.HookEntry {
	out := map[string]config.HookEntry{}
	if cfg == nil || cfg.Agent == nil {
		return out
	}
	for eventName, entries := range cfg.Agent.Hooks {
		for i, entry := range entries {
			name := fmt.Sprintf("omni.config.%s.%d", eventName, i)
			entry.Args = withEventFlag(entry.Args, eventName)
			out[name] = entry
		}
	}
	return out
}

func withAgentFlag(args []string, agentID string) []string {
	if agentID == "" || flagValue(args, "--agent") != "" {
		return slices.Clone(args)
	}
	out := slices.Clone(args)
	return append(out, "--agent", agentID)
}

func withEventFlag(args []string, eventName string) []string {
	if eventName == "" || flagValue(args, "--event") != "" {
		return slices.Clone(args)
	}
	out := slices.Clone(args)
	return append(out, "--event", eventName)
}

func flagValue(args []string, flag string) string {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
		if len(arg) > len(flag)+1 && arg[:len(flag)+1] == flag+"=" {
			return arg[len(flag)+1:]
		}
	}
	return ""
}
