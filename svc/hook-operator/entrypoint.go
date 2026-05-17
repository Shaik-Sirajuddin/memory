package hookoperator

import (
	"context"
	"strings"

)

type entryPoint struct {
	cache    *entryCache
	exec     *executor
	enricher *enricher
}

func newEntryPoint(cache *entryCache, exec *executor, enricher *enricher) *entryPoint {
	return &entryPoint{cache: cache, exec: exec, enricher: enricher}
}

// Hook enriches the payload with omni context, reads hook entries from cache,
// executes them all in parallel, and returns the aggregated Result.
func (ep *entryPoint) Hook(payload HookPayload) (Result, error) {
	entries := ep.cache.get(payload.EventName)
	logger.Debug("hook: received", "event", payload.EventName, "entries", len(entries))

	if len(entries) == 0 {
		logger.Debug("hook: no entries configured, passing through", "event", payload.EventName)
		return Result{Continue: true}, nil
	}

	payload.Body = ep.enricher.enrich(payload.Body)

	results := ep.exec.runAll(context.Background(), payload, entries)
	result := aggregate(results)

	logger.Debug("hook: aggregated result", "event", payload.EventName, "continue", result.Continue, "suppress_output", result.SuppressOutput)
	return result, nil
}

// aggregate merges individual hook results into a single Result.
//
// Rules:
//   - Any continue=false blocks; the first stop_reason encountered wins.
//   - All system_messages are joined with a newline.
//   - SuppressOutput is true when any hook sets it to true.
//   - Errored hooks are treated as continue=true and do not block.
func aggregate(results []hookRunResult) Result {
	out := Result{Continue: true}

	var sysMessages []string

	for _, r := range results {
		if r.err != nil {
			// Treat errors as non-blocking; callers may log separately.
			continue
		}

		if !r.resp.Continue && out.Continue {
			out.Continue = false
			out.StopReason = r.resp.StopReason
		}

		if r.resp.SuppressOutput {
			out.SuppressOutput = true
		}

		if r.resp.SystemMessage != nil && *r.resp.SystemMessage != "" {
			sysMessages = append(sysMessages, *r.resp.SystemMessage)
		}
	}

	if len(sysMessages) > 0 {
		merged := strings.Join(sysMessages, "\n")
		out.SystemMessage = &merged
	}

	return out
}
