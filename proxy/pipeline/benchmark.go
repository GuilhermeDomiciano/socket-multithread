package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/domiciano/llm-proxy/event"
	"github.com/domiciano/llm-proxy/provider"
	"github.com/domiciano/llm-proxy/router"
)

// phaseSink stamps every event with a benchmark phase ("seq"|"par") and forwards
// it, so the gather-all router funcs stay phase-agnostic.
type phaseSink struct {
	inner event.Sink
	phase string
}

func (p phaseSink) Emit(e event.Event) {
	e.Phase = p.phase
	if p.inner != nil {
		p.inner.Emit(e)
	}
}

type speedupPayload struct {
	SeqMs  int64   `json:"seq_ms"`
	ParMs  int64   `json:"par_ms"`
	Factor float64 `json:"factor"`
}

// Benchmark runs the same prompt gather-all sequentially then in parallel, times
// both, and emits a "speedup" event. It honors the input guard (PII mask +
// injection block) but skips intent and output scrub — the number is the point.
func (g *Gateway) Benchmark(ctx context.Context, prompt string, sink event.Sink) error {
	in := g.Input.Inspect(prompt)
	for _, f := range in.Findings {
		if f.Type == "injection" {
			continue
		}
		emit(sink, event.Event{Type: "guard_in", Detail: f.Type, Content: f.Placeholder})
	}
	if in.Blocked {
		emit(sink, event.Event{Type: "blocked", Detail: in.Reason})
		return fmt.Errorf("%w: %s", ErrBlocked, in.Reason)
	}
	emit(sink, event.Event{Type: "masked_prompt", Content: in.Text})

	req := provider.Request{Messages: []provider.Message{{Role: "user", Content: in.Text}}}
	providers := g.Router.Providers

	t0 := time.Now()
	for range router.SequentialAll(ctx, providers, req, phaseSink{inner: sink, phase: "seq"}) {
	}
	seqMs := time.Since(t0).Milliseconds()

	t1 := time.Now()
	for range router.ParallelAll(ctx, providers, req, phaseSink{inner: sink, phase: "par"}) {
	}
	parMs := time.Since(t1).Milliseconds()

	factor := 0.0
	if parMs > 0 {
		factor = math.Round(float64(seqMs)/float64(parMs)*100) / 100
	}
	payload, _ := json.Marshal(speedupPayload{SeqMs: seqMs, ParMs: parMs, Factor: factor})
	detail := fmt.Sprintf("Sequencial %.2fs · Paralelo %.2fs · %.1f× mais rápido",
		float64(seqMs)/1000, float64(parMs)/1000, factor)
	emit(sink, event.Event{Type: "speedup", Content: string(payload), Detail: detail})
	return nil
}
