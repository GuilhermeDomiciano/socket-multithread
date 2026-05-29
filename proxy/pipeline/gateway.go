package pipeline

import (
	"context"
	"errors"
	"fmt"

	"github.com/domiciano/llm-proxy/event"
	"github.com/domiciano/llm-proxy/guardrail"
	"github.com/domiciano/llm-proxy/intent"
	"github.com/domiciano/llm-proxy/provider"
	"github.com/domiciano/llm-proxy/router"
)

// ErrBlocked is returned (wrapped) when the input guard blocks the request.
var ErrBlocked = errors.New("request blocked by guardrail")

// Gateway runs the full smart-gateway pipeline around the parallel router.
type Gateway struct {
	Input  guardrail.Guard // e.g. Chain{InjectionGuard, PIIGuard}
	Output guardrail.Guard // e.g. PIIGuard
	Router *router.Router
}

// Process runs: input guard → intent (if strategy is "" or "auto") → router
// dispatch → output scrub. It returns a channel of sanitized chunks. On a
// guardrail block it returns an error wrapping ErrBlocked (and emits a
// "blocked" event); the provider is never called.
func (g *Gateway) Process(ctx context.Context, prompt string, strategyOverride router.Strategy, sink event.Sink) (<-chan provider.Chunk, error) {
	in := g.Input.Inspect(prompt)
	for _, f := range in.Findings {
		if f.Type == "injection" {
			continue
		}
		emit(sink, event.Event{Type: "guard_in", Detail: f.Type, Content: f.Placeholder})
	}
	if in.Blocked {
		emit(sink, event.Event{Type: "blocked", Detail: in.Reason})
		return nil, fmt.Errorf("%w: %s", ErrBlocked, in.Reason)
	}
	emit(sink, event.Event{Type: "masked_prompt", Content: in.Text})

	strategy := strategyOverride
	if strategy == "" || strategy == "auto" {
		it := intent.Classify(in.Text)
		emit(sink, event.Event{Type: "intent", Detail: it.Reason})
		strategy = it.Strategy
	}

	rtr := &router.Router{Providers: g.Router.Providers, Strategy: strategy}
	chunks, err := rtr.Dispatch(ctx, provider.Request{
		Messages: []provider.Message{{Role: "user", Content: in.Text}},
	}, sink)
	if err != nil {
		return nil, err
	}
	return scrub(ctx, chunks, g.Output, sink), nil
}
