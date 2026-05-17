package confhooks

import (
	"github.com/Shaik-Sirajuddin/memory/config"
	"github.com/Shaik-Sirajuddin/memory/connector/codeagent/hooks"
)

// HookSchema is the marker interface for per-event schema bundles.
// Callers use Go type assertion to get the concrete event type:
//
//	if s, ok := hook.Schemas.(*PreToolUseSchema); ok { ... }
type HookSchema interface {
	EventName() string
}

// Hook is a named hook registration at the omni layer.
// Schemas carries the event-specific schema so callers can type-assert
// to the concrete *EventSchema to access typed Input, ResponseSchema, and ResultSchema.
type Hook struct {
	Name    string
	Entry   config.HookEntry
	Schemas HookSchema
}

// EventBase fields present in every hook payload from any codeagent.
type EventBase struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	HookEventName  string `json:"hook_event_name"`
}

// Response is the canonical fields shared by every hook response.
type Response struct {
	Continue       bool    `json:"continue"`
	StopReason     *string `json:"stop_reason,omitempty"`
	SuppressOutput bool    `json:"suppress_output"`
	SystemMessage  *string `json:"system_message,omitempty"`
}

// HookResponseSchema is what omni sends back to the codeagent after processing a hook.
// The codeagent reads this and acts on it (block tool, abort session, modify context).
// Applies to response hooks: PreToolUse, PreSessionStart, PrePrompt.
type HookResponseSchema struct {
	EventName string
	Response  Response
}

// HookResultSchema is the parsed result from a hook command's stdout.
// Represents what the hook subprocess decided and wrote back.
// Applies to result hooks: PostToolUse, PostToolUseFailure, PostSessionStart, PostPrompt.
type HookResultSchema struct {
	EventName string
	Result    Response
}

// ── PreToolUse ────────────────────────────────────────────────────────────────

type PreToolUseInput struct {
	EventBase
	ToolName  string         `json:"tool_name"`
	ToolInput map[string]any `json:"tool_input"`
	ToolUseID string         `json:"tool_use_id"`
}

// PreToolUseResponseSchema — response hook.
// Continue=false blocks the tool call entirely.
// SystemMessage is injected into context before the block.
type PreToolUseResponseSchema struct{ Response }

// PreToolUseResultSchema — what the hook command writes to stdout.
type PreToolUseResultSchema struct{ Response }

type PreToolUseSchema struct {
	Input          PreToolUseInput
	ResponseSchema PreToolUseResponseSchema
	ResultSchema   PreToolUseResultSchema
}

func (s *PreToolUseSchema) EventName() string { return string(hooks.PreToolUse) }

// ── PostToolUse ───────────────────────────────────────────────────────────────

type PostToolUseInput struct {
	EventBase
	ToolName     string         `json:"tool_name"`
	ToolInput    map[string]any `json:"tool_input"`
	ToolUseID    string         `json:"tool_use_id"`
	ToolResponse any            `json:"tool_response"`
}

// PostToolUseResponseSchema — response hook.
// SuppressOutput=true hides tool result from agent context.
type PostToolUseResponseSchema struct{ Response }

// PostToolUseResultSchema — what the hook command writes to stdout.
type PostToolUseResultSchema struct{ Response }

type PostToolUseSchema struct {
	Input          PostToolUseInput
	ResponseSchema PostToolUseResponseSchema
	ResultSchema   PostToolUseResultSchema
}

func (s *PostToolUseSchema) EventName() string { return string(hooks.PostToolUse) }

// ── PostToolUseFailure ────────────────────────────────────────────────────────

type PostToolUseFailureInput struct {
	EventBase
	ToolName  string         `json:"tool_name"`
	ToolInput map[string]any `json:"tool_input"`
	ToolUseID string         `json:"tool_use_id"`
	Error     string         `json:"error"`
}

// PostToolUseFailureResponseSchema — response hook.
// Continue=false stops session after failure; StopReason names why.
type PostToolUseFailureResponseSchema struct{ Response }

// PostToolUseFailureResultSchema — what the hook command writes to stdout.
type PostToolUseFailureResultSchema struct{ Response }

type PostToolUseFailureSchema struct {
	Input          PostToolUseFailureInput
	ResponseSchema PostToolUseFailureResponseSchema
	ResultSchema   PostToolUseFailureResultSchema
}

func (s *PostToolUseFailureSchema) EventName() string { return string(hooks.PostToolUseFailure) }

// ── PreSessionStart ───────────────────────────────────────────────────────────

type PreSessionStartInput struct {
	EventBase
	Source string `json:"source"` // "startup" | "resume" | "clear" | "compact"
}

// PreSessionStartResponseSchema — response hook.
// Continue=false aborts session creation before it starts.
// SystemMessage is prepended as the first system turn.
type PreSessionStartResponseSchema struct{ Response }

// PreSessionStartResultSchema — what the hook command writes to stdout.
type PreSessionStartResultSchema struct{ Response }

type PreSessionStartSchema struct {
	Input          PreSessionStartInput
	ResponseSchema PreSessionStartResponseSchema
	ResultSchema   PreSessionStartResultSchema
}

func (s *PreSessionStartSchema) EventName() string { return string(hooks.PreSessionStart) }

// ── PostSessionStart ──────────────────────────────────────────────────────────

type PostSessionStartInput struct {
	EventBase
	// No extra fields — session is already live.
}

// PostSessionStartResponseSchema — response hook.
// SystemMessage appended after session starts (e.g. inject memory context).
type PostSessionStartResponseSchema struct{ Response }

// PostSessionStartResultSchema — what the hook command writes to stdout.
type PostSessionStartResultSchema struct{ Response }

type PostSessionStartSchema struct {
	Input          PostSessionStartInput
	ResponseSchema PostSessionStartResponseSchema
	ResultSchema   PostSessionStartResultSchema
}

func (s *PostSessionStartSchema) EventName() string { return string(hooks.PostSessionStart) }

// ── PrePrompt ─────────────────────────────────────────────────────────────────

type PrePromptInput struct {
	EventBase
	Prompt string `json:"prompt"` // raw user prompt before agent sees it
}

// PrePromptResponseSchema — response hook.
// Continue=false rejects the prompt before submission.
// SystemMessage is injected alongside the prompt as context.
type PrePromptResponseSchema struct{ Response }

// PrePromptResultSchema — what the hook command writes to stdout.
type PrePromptResultSchema struct{ Response }

type PrePromptSchema struct {
	Input          PrePromptInput
	ResponseSchema PrePromptResponseSchema
	ResultSchema   PrePromptResultSchema
}

func (s *PrePromptSchema) EventName() string { return string(hooks.PrePrompt) }

// ── PostPrompt ────────────────────────────────────────────────────────────────

type PostPromptInput struct {
	EventBase
	Prompt      string `json:"prompt"`   // original user prompt
	AgentAnswer string `json:"response"` // agent's final answer; named AgentAnswer to avoid clash with Response type
}

// PostPromptResponseSchema — response hook.
// Continue=false stops the session after this turn.
// SuppressOutput=true hides response from downstream consumers.
type PostPromptResponseSchema struct{ Response }

// PostPromptResultSchema — what the hook command writes to stdout.
type PostPromptResultSchema struct{ Response }

type PostPromptSchema struct {
	Input          PostPromptInput
	ResponseSchema PostPromptResponseSchema
	ResultSchema   PostPromptResultSchema
}

func (s *PostPromptSchema) EventName() string { return string(hooks.PostPrompt) }
