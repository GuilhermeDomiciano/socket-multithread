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

			emit(sink, event.Event{Type: "provider_start", Provider: p.Name()})

			go func() {
				p.Stream(pCtx, req, ch) //nolint:errcheck
			}()

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

		go func() {
			wg.Wait()
			close(merged)
		}()

		winnerIdx := -1
		errCount := 0

		for ic := range merged {
			pname := providers[ic.idx].Name()
			if winnerIdx == -1 {
				if ic.chunk.Err != nil {
					errCount++
					emit(sink, event.Event{Type: "failed", Provider: pname, Detail: ic.chunk.Err.Error()})
					cancels[ic.idx]()
					if errCount == len(providers) {
						emit(sink, event.Event{Type: "error", Detail: "all providers failed"})
						out <- provider.Chunk{Err: fmt.Errorf("all providers failed")}
						return
					}
					continue
				}
				// This provider wins — cancel all losers.
				winnerIdx = ic.idx
				emit(sink, event.Event{Type: "won", Provider: pname})
				defer cancels[winnerIdx]()
				for i, cancel := range cancels {
					if i != winnerIdx {
						cancel()
						emit(sink, event.Event{Type: "cancelled", Provider: providers[i].Name()})
					}
				}
			}
			if ic.chunk.Err != nil {
				// Loser error arriving after the winner was chosen — ignore for output.
				continue
			}
			// Emit chunk telemetry for EVERY provider (winners and stragglers) so the
			// dashboard can show each lane advancing — but only the winner's chunks are
			// forwarded to out below.
			if !ic.chunk.Done {
				emit(sink, event.Event{Type: "chunk", Provider: pname, Content: ic.chunk.Content})
			}
			if ic.idx == winnerIdx {
				out <- ic.chunk
				if ic.chunk.Done {
					emit(sink, event.Event{Type: "done", Provider: pname})
					return
				}
			}
		}

		if winnerIdx == -1 {
			emit(sink, event.Event{Type: "error", Detail: "all providers failed"})
			out <- provider.Chunk{Err: fmt.Errorf("all providers failed")}
		}
	}()

	return out
}
