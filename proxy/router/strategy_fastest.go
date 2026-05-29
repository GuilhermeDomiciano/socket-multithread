package router

import (
	"context"
	"fmt"
	"sync"

	"github.com/domiciano/llm-proxy/event"
	"github.com/domiciano/llm-proxy/provider"
)

// Fastest fans out to all providers in parallel and returns chunks from the
// first provider to produce a non-error chunk. Losers are cancelled immediately.
func Fastest(ctx context.Context, providers []provider.Provider, req provider.Request, sink event.Sink) <-chan provider.Chunk {
	out := make(chan provider.Chunk, 64)

	go func() {
		defer close(out)

		// done is closed when the main goroutine exits, unblocking forwarder goroutines.
		done := make(chan struct{})
		defer close(done)

		type indexed struct {
			chunk provider.Chunk
			idx   int
		}
		merged := make(chan indexed, 64)

		cancels := make([]context.CancelFunc, len(providers))
		var wg sync.WaitGroup

		for i, p := range providers {
			i, p := i, p
			pCtx, cancel := context.WithCancel(ctx)
			cancels[i] = cancel
			ch := make(chan provider.Chunk, 64)

			// Stream goroutine: drives the provider. Stream closes ch on return.
			go func() {
				p.Stream(pCtx, req, ch) //nolint:errcheck
			}()

			// Forwarder goroutine: copies provider chunks into merged.
			wg.Add(1)
			go func() {
				defer wg.Done()
				for c := range ch {
					select {
					case merged <- indexed{c, i}:
					case <-done:
						return
					}
				}
			}()
		}

		// Close merged once all forwarders exit.
		go func() {
			wg.Wait()
			close(merged)
		}()

		winnerIdx := -1
		errCount := 0

		for ic := range merged {
			if winnerIdx == -1 {
				if ic.chunk.Err != nil {
					errCount++
					cancels[ic.idx]()
					if errCount == len(providers) {
						out <- provider.Chunk{Err: fmt.Errorf("all providers failed")}
						return
					}
					continue
				}
				// This provider wins — cancel all losers.
				winnerIdx = ic.idx
				defer cancels[winnerIdx]() // ensure winner's context is released when goroutine exits
				for i, cancel := range cancels {
					if i != winnerIdx {
						cancel()
					}
				}
			}
			if ic.idx == winnerIdx {
				out <- ic.chunk
				if ic.chunk.Done {
					return
				}
			}
		}

		if winnerIdx == -1 {
			out <- provider.Chunk{Err: fmt.Errorf("all providers failed")}
		}
	}()

	return out
}
