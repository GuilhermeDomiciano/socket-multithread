package router

import (
	"context"
	"errors"
	"fmt"

	"github.com/domiciano/llm-proxy/event"
	"github.com/domiciano/llm-proxy/provider"
)

// Fallback tries providers sequentially. It advances to the next provider only
// if the current one sends an error chunk before producing any content.
// If a provider starts streaming content and then errors, that error is forwarded
// to the client — a partial stream cannot be retried transparently.
func Fallback(ctx context.Context, providers []provider.Provider, req provider.Request, sink event.Sink) <-chan provider.Chunk {
	out := make(chan provider.Chunk, 64)

	go func() {
		defer close(out)
		var errs []error
		for _, p := range providers {
			pCtx, cancel := context.WithCancel(ctx)
			ch := make(chan provider.Chunk, 64)
			emit(sink, event.Event{Type: "provider_start", Provider: p.Name()})
			go func() {
				p.Stream(pCtx, req, ch) //nolint:errcheck
			}()

			sentContent := false
			providerFailed := false

			for c := range ch {
				if c.Err != nil {
					if sentContent {
						// Already committed — forward the error and stop.
						out <- c
						cancel()
						return
					}
					providerFailed = true
					errs = append(errs, c.Err)
					emit(sink, event.Event{Type: "failed", Provider: p.Name(), Detail: c.Err.Error()})
					cancel()
					break
				}
				if !c.Done {
					emit(sink, event.Event{Type: "chunk", Provider: p.Name(), Content: c.Content})
				}
				out <- c
				if !c.Done {
					sentContent = true
				}
				if c.Done {
					emit(sink, event.Event{Type: "done", Provider: p.Name()})
					cancel()
					return
				}
			}
			cancel()
			if !providerFailed {
				// Channel closed without Done and without error — treat as failure.
				continue
			}
		}
		emit(sink, event.Event{Type: "error", Detail: "all providers failed"})
		out <- provider.Chunk{Err: fmt.Errorf("all providers failed: %w", errors.Join(errs...))}
	}()

	return out
}
