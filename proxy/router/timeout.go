package router

import (
	"context"
	"time"
)

// CallTimeout bounds each individual provider Stream call. Zero (the default,
// used by tests) means no bound. main sets it from PROXY_TIMEOUT_MS once at
// startup, before any request is served, so it is effectively immutable while
// serving. With it set, a stuck or rate-limited provider fails fast (💥) instead
// of freezing a fan-out race or — worst case — the benchmark's gather-all phases,
// which wait for every provider to finish.
var CallTimeout time.Duration

// callCtx derives a per-call context cancelled by the parent, by the returned
// cancel func, or by CallTimeout — whichever comes first. Callers MUST call the
// returned cancel to release resources.
func callCtx(parent context.Context) (context.Context, context.CancelFunc) {
	if CallTimeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, CallTimeout)
}
