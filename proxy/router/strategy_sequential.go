package router

import (
	"context"

	"github.com/domiciano/llm-proxy/event"
	"github.com/domiciano/llm-proxy/provider"
)

// SequentialAll runs every provider one after another to completion (gather-all).
// Unlike Fallback it does NOT stop at the first success: a failing provider just
// contributes its time-to-fail and we move on. All content chunks are forwarded
// to out. The wall-clock of draining out is ~the SUM of the providers' latencies.
func SequentialAll(ctx context.Context, providers []provider.Provider, req provider.Request, sink event.Sink) <-chan provider.Chunk {
	out := make(chan provider.Chunk, 64)

	go func() {
		defer close(out)
		for _, p := range providers {
			ch := make(chan provider.Chunk, 64)
			emit(sink, event.Event{Type: "provider_start", Provider: p.Name()})

			go func() {
				p.Stream(ctx, req, ch) //nolint:errcheck
			}()

			for c := range ch {
				if c.Err != nil {
					emit(sink, event.Event{Type: "failed", Provider: p.Name(), Detail: c.Err.Error()})
					continue
				}
				if c.Done {
					emit(sink, event.Event{Type: "done", Provider: p.Name()})
				} else {
					emit(sink, event.Event{Type: "chunk", Provider: p.Name(), Content: c.Content})
				}
				out <- c
			}
		}
	}()

	return out
}
