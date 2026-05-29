package pipeline

import (
	"context"

	"github.com/domiciano/llm-proxy/event"
	"github.com/domiciano/llm-proxy/guardrail"
	"github.com/domiciano/llm-proxy/provider"
)

// scrubCarry is the number of bytes held back so PII split across chunk
// boundaries is reunited before masking. Must exceed the longest PII token.
const scrubCarry = 48

// emit is a nil-safe sink helper (production callers pass nil).
func emit(sink event.Sink, e event.Event) {
	if sink != nil {
		sink.Emit(e)
	}
}

// scrub consumes chunks, masks PII in the content as it streams (using guard),
// emits guard_out events per finding, and forwards sanitized chunks. The
// terminal Done/Err chunk is forwarded after the remaining buffer is flushed.
func scrub(ctx context.Context, in <-chan provider.Chunk, guard guardrail.Guard, sink event.Sink) <-chan provider.Chunk {
	out := make(chan provider.Chunk, 64)
	go func() {
		defer close(out)
		var buf string
		send := func(c provider.Chunk) {
			select {
			case out <- c:
			case <-ctx.Done():
			}
		}
		flush := func(s, prov string) {
			if s == "" {
				return
			}
			r := guard.Inspect(s)
			for _, f := range r.Findings {
				emit(sink, event.Event{Type: "guard_out", Detail: f.Type, Content: f.Placeholder})
			}
			send(provider.Chunk{Content: r.Text, Provider: prov})
		}
		for {
			select {
			case <-ctx.Done():
				return
			case c, ok := <-in:
				if !ok {
					flush(buf, "")
					return
				}
				if c.Err != nil || c.Done {
					flush(buf, c.Provider)
					buf = ""
					send(c)
					// Do NOT return here: keep reading until `in` is closed.
					// Upstream (router) emits its terminal "done"/"error" event
					// to the sink AFTER sending this chunk and only closes `in`
					// last (deferred). Waiting for the close guarantees all
					// upstream sink emits happen-before the consumer's Close().
					continue
				}
				buf += c.Content
				if len(buf) > scrubCarry {
					commit := buf[:len(buf)-scrubCarry]
					buf = buf[len(buf)-scrubCarry:]
					flush(commit, c.Provider)
				}
			}
		}
	}()
	return out
}
