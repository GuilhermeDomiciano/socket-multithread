package router

import (
	"context"
	"sync"

	"github.com/domiciano/llm-proxy/event"
	"github.com/domiciano/llm-proxy/provider"
)

// ParallelAll runs every provider concurrently and waits for ALL to finish
// (gather-all; no winner, no cancellation — contrast with Fastest). All content
// chunks are forwarded to out (interleaved). The wall-clock of draining out is
// ~the MAX of the providers' latencies. This is the parallel side of the benchmark.
func ParallelAll(ctx context.Context, providers []provider.Provider, req provider.Request, sink event.Sink) <-chan provider.Chunk {
	out := make(chan provider.Chunk, 64)

	go func() {
		defer close(out)
		var wg sync.WaitGroup

		for _, p := range providers {
			p := p
			cctx, cancel := callCtx(ctx)
			ch := make(chan provider.Chunk, 64)
			emit(sink, event.Event{Type: "provider_start", Provider: p.Name()})

			go func() {
				p.Stream(cctx, req, ch) //nolint:errcheck
			}()

			wg.Add(1)
			go func() {
				defer wg.Done()
				defer cancel() // release the per-call context once the stream drains
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
			}()
		}

		wg.Wait()
	}()

	return out
}
