# Live Web Visualization of the Parallel Proxy — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a live web dashboard that visualizes the proxy's parallel race (fan-out, first-response-wins, context cancellation), with live sabotage and an in-browser timeline — all additive, never altering the production path.

**Architecture:** A new `event` package defines `Event`/`Sink`/`ChanSink`. The router's strategy functions and `Dispatch` gain an optional `event.Sink` parameter and emit lifecycle events; when the sink is `nil`, behavior is byte-for-byte identical to today. A new SSE endpoint `/viz/stream` runs a dispatch with a `ChanSink` and streams events to the browser. A `provider.Sabotage` decorator wraps real providers and is toggled at runtime via `POST /viz/sabotage`. Static assets are embedded and served at `/`.

**Tech Stack:** Go 1.22 stdlib only (`net/http`, `embed`, `context`, `sync`, `encoding/json`), HTML + vanilla JS (no framework, no build).

**Spec:** `docs/superpowers/specs/2026-05-29-proxy-viz-design.md`

**Working directory for all commands:** `/Users/user/Documents/ulbra/sistemas-paralelos/proxy`
(Branch `feat/proxy-web-viz` is already checked out.)

---

## File Structure

**New files:**
- `event/event.go` — `Event`, `Sink`, `ChanSink`
- `event/event_test.go` — sink behavior tests
- `router/events_test.go` — emission tests (one place for all strategy emission assertions)
- `provider/sabotage.go` — `Sabotage` decorator + constructor + setters
- `provider/sabotage_test.go` — decorator tests
- `server/handler_viz.go` — `handleVizStream`, `handleSabotage`
- `server/static.go` — `//go:embed` + static handler
- `server/static/index.html` — dashboard markup + CSS
- `server/static/app.js` — EventSource client + rendering
- `server/handler_viz_test.go` — viz endpoint tests
- `examples/racecondition/main.go` — intentional data-race demo (didactic, not production)

**Modified files:**
- `router/router.go` — `Dispatch` gains `sink event.Sink`; emits `start`; adds `emit` helper
- `router/strategy_fastest.go` — `Fastest` gains `sink`; emits lifecycle events
- `router/strategy_fallback.go` — `Fallback` gains `sink`; emits lifecycle events
- `router/strategy_cheapest.go` — `Cheapest` gains `sink`; emits `decision`
- `router/strategy_fastest_test.go` — add `, nil` to 3 calls
- `router/strategy_fallback_test.go` — add `, nil` to 4 calls
- `router/strategy_cheapest_test.go` — add `, nil` to 2 calls
- `router/router_test.go` — add `, nil` to 4 `Dispatch` calls
- `server/handler_query.go` — `Dispatch` call adds `, nil`
- `server/handler_openai.go` — `Dispatch` call adds `, nil`
- `server/server.go` — `New` gains `sabotage` param; `Server` gains `Sabotage` field; register new routes
- `server/server_test.go` — add `, nil` to 4 `server.New` calls
- `main.go` — wrap providers in `Sabotage`, build registry, pass to `server.New`

---

## Task 1: `event` package

**Files:**
- Create: `event/event.go`
- Create: `event/event_test.go`

- [ ] **Step 1: Write the failing tests**

Create `event/event_test.go`:

```go
package event_test

import (
	"testing"
	"time"

	"github.com/domiciano/llm-proxy/event"
)

func TestChanSink_emits_in_order(t *testing.T) {
	done := make(chan struct{})
	s := event.NewChanSink(8, time.Now(), done)
	go func() {
		s.Emit(event.Event{Type: "a"})
		s.Emit(event.Event{Type: "b"})
		s.Close()
	}()
	var got []string
	for e := range s.Events() {
		got = append(got, e.Type)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("expected [a b], got %v", got)
	}
}

func TestChanSink_does_not_block_after_done(t *testing.T) {
	done := make(chan struct{})
	close(done)
	s := event.NewChanSink(0, time.Now(), done) // unbuffered + done closed
	finished := make(chan struct{})
	go func() {
		s.Emit(event.Event{Type: "x"}) // must take the done branch, not block
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("Emit blocked after done closed")
	}
}

func TestChanSink_stamps_relative_time(t *testing.T) {
	done := make(chan struct{})
	start := time.Now().Add(-50 * time.Millisecond)
	s := event.NewChanSink(1, start, done)
	s.Emit(event.Event{Type: "a"})
	s.Close()
	e := <-s.Events()
	if e.T < 40 {
		t.Fatalf("expected T >= ~50ms, got %d", e.T)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./event/ -v`
Expected: FAIL — `undefined: event.NewChanSink` / package has no Go files.

- [ ] **Step 3: Write the implementation**

Create `event/event.go`:

```go
// Package event carries optional, plug-in telemetry for the router.
// The production data path never depends on it: when a Sink is nil,
// no events are emitted and behavior is unchanged.
package event

import "time"

// Event is one observable moment in a dispatch (a provider started, won,
// was cancelled, produced a chunk, etc.). T is milliseconds since the race began.
type Event struct {
	Type     string `json:"type"`
	Provider string `json:"provider,omitempty"`
	T        int64  `json:"t"`
	Content  string `json:"content,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// Sink receives events. Implementations must be safe for concurrent Emit calls.
type Sink interface {
	Emit(Event)
}

// ChanSink is a Sink backed by a buffered channel. It stamps each event with
// the elapsed time since start and never blocks once done is closed (so a
// disconnected client cannot leak the producing goroutines).
type ChanSink struct {
	ch    chan Event
	done  <-chan struct{}
	start time.Time
}

func NewChanSink(buf int, start time.Time, done <-chan struct{}) *ChanSink {
	return &ChanSink{ch: make(chan Event, buf), done: done, start: start}
}

func (s *ChanSink) Emit(e Event) {
	e.T = time.Since(s.start).Milliseconds()
	select {
	case s.ch <- e:
	case <-s.done:
	}
}

// Events returns the read side of the channel.
func (s *ChanSink) Events() <-chan Event { return s.ch }

// Close closes the channel. Call only after the producing dispatch has finished.
func (s *ChanSink) Close() { close(s.ch) }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./event/ -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add event/
git commit -m "feat(event): add Event, Sink, and ChanSink for plug-in router telemetry"
```

---

## Task 2: Thread `event.Sink` through the router (plumbing only)

This task changes signatures and updates every call site so the project still
builds and **all existing tests pass unchanged in behavior**. No `Emit` calls yet.

**Files:**
- Modify: `router/router.go`
- Modify: `router/strategy_fastest.go`
- Modify: `router/strategy_fallback.go`
- Modify: `router/strategy_cheapest.go`
- Modify: `router/strategy_fastest_test.go`
- Modify: `router/strategy_fallback_test.go`
- Modify: `router/strategy_cheapest_test.go`
- Modify: `router/router_test.go`
- Modify: `server/handler_query.go`
- Modify: `server/handler_openai.go`

- [ ] **Step 1: Add the `emit` helper and `sink` param to `Dispatch`**

In `router/router.go`, replace the import block and `Dispatch` with:

```go
package router

import (
	"context"
	"fmt"

	"github.com/domiciano/llm-proxy/event"
	"github.com/domiciano/llm-proxy/provider"
)

type Strategy string

const (
	StrategyFastest  Strategy = "fastest"
	StrategyCheapest Strategy = "cheapest"
	StrategyFallback Strategy = "fallback"
)

type Router struct {
	Providers []provider.Provider
	Strategy  Strategy
}

// emit is a nil-safe shortcut: when sink is nil, nothing happens and the
// production path is unaffected.
func emit(sink event.Sink, e event.Event) {
	if sink != nil {
		sink.Emit(e)
	}
}

func (r *Router) Dispatch(ctx context.Context, req provider.Request, sink event.Sink) (<-chan provider.Chunk, error) {
	if len(r.Providers) == 0 {
		return nil, fmt.Errorf("no providers configured")
	}
	switch r.Strategy {
	case StrategyFastest, "":
		return Fastest(ctx, r.Providers, req, sink), nil
	case StrategyCheapest:
		return Cheapest(ctx, r.Providers, req, sink), nil
	case StrategyFallback:
		return Fallback(ctx, r.Providers, req, sink), nil
	default:
		return nil, fmt.Errorf("unknown strategy: %q", r.Strategy)
	}
}
```

- [ ] **Step 2: Add `sink` param to the three strategy signatures**

In `router/strategy_fastest.go`, change the function signature line and add the `event` import:

```go
import (
	"context"
	"fmt"
	"sync"

	"github.com/domiciano/llm-proxy/event"
	"github.com/domiciano/llm-proxy/provider"
)

func Fastest(ctx context.Context, providers []provider.Provider, req provider.Request, sink event.Sink) <-chan provider.Chunk {
```

In `router/strategy_fallback.go`:

```go
import (
	"context"
	"errors"
	"fmt"

	"github.com/domiciano/llm-proxy/event"
	"github.com/domiciano/llm-proxy/provider"
)

func Fallback(ctx context.Context, providers []provider.Provider, req provider.Request, sink event.Sink) <-chan provider.Chunk {
```

In `router/strategy_cheapest.go`, change the signature and the delegating call:

```go
import (
	"context"
	"sort"

	"github.com/domiciano/llm-proxy/event"
	"github.com/domiciano/llm-proxy/provider"
)

func Cheapest(ctx context.Context, providers []provider.Provider, req provider.Request, sink event.Sink) <-chan provider.Chunk {
	ranked := make([]provider.Provider, len(providers))
	copy(ranked, providers)
	sort.Slice(ranked, func(i, j int) bool {
		return estimateCost(ranked[i], req) < estimateCost(ranked[j], req)
	})
	return Fallback(ctx, ranked, req, sink)
}
```

> Note: `event` is imported in all three now even though emission lands in Tasks 3–5. Go will error on an unused import. To avoid a broken intermediate state, the signatures above each reference `sink` only in `Cheapest` (passes it on). For `Fastest` and `Fallback` the `sink` parameter is currently unused — Go permits unused *function parameters*, so this compiles. The `event` import IS used (it appears in the parameter type `event.Sink`). No blank identifier needed.

- [ ] **Step 3: Update call sites in the two server handlers**

In `server/handler_query.go`, change the `Dispatch` call (around line 40):

```go
	chunks, err := rtr.Dispatch(ctx, provider.Request{
		Messages:    req.Messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}, nil)
```

In `server/handler_openai.go`, change the `Dispatch` call (around line 43):

```go
	chunks, err := s.Router.Dispatch(ctx, provider.Request{Messages: messages, MaxTokens: req.MaxTokens}, nil)
```

- [ ] **Step 4: Update call sites in the existing router tests**

Add `, nil` as the final argument to every strategy/Dispatch call:

- `router/strategy_fastest_test.go`: 3 calls to `router.Fastest(...)` →
  - `router.Fastest(context.Background(), []provider.Provider{slow, fast}, provider.Request{}, nil)`
  - `router.Fastest(ctx, []provider.Provider{slow, fast}, provider.Request{}, nil)`
  - `router.Fastest(context.Background(), []provider.Provider{p1, p2}, provider.Request{}, nil)`
- `router/strategy_fallback_test.go`: 4 calls to `router.Fallback(...)` — append `, nil` to each (lines with `failing, working`; `p1, p2`; `good`; `contentThenError, backup`).
- `router/strategy_cheapest_test.go`: 2 calls to `router.Cheapest(...)` — append `, nil` to each (`expensive, cheap`, `req`) and (`failing, working`, `req`).
- `router/router_test.go`: 4 calls to `r.Dispatch(context.Background(), provider.Request{})` → `r.Dispatch(context.Background(), provider.Request{}, nil)`.

- [ ] **Step 5: Build and run the full suite to verify green**

Run: `go build ./... && go test ./...`
Expected: PASS for all existing packages. No behavior changed; only signatures.

- [ ] **Step 6: Commit**

```bash
git add router/ server/handler_query.go server/handler_openai.go
git commit -m "refactor(router): thread optional event.Sink through Dispatch and strategies"
```

---

## Task 3: Emit lifecycle events from `Fastest` (the race)

**Files:**
- Modify: `router/strategy_fastest.go`
- Modify: `router/router.go` (emit `start`)
- Create: `router/events_test.go`

- [ ] **Step 1: Write the failing tests**

Create `router/events_test.go`:

```go
package router_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/domiciano/llm-proxy/event"
	"github.com/domiciano/llm-proxy/provider"
	"github.com/domiciano/llm-proxy/router"
)

// recSink records every emitted event for assertions.
type recSink struct {
	mu     sync.Mutex
	events []event.Event
}

func (r *recSink) Emit(e event.Event) {
	r.mu.Lock()
	r.events = append(r.events, e)
	r.mu.Unlock()
}

func (r *recSink) has(typ, prov string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.events {
		if e.Type == typ && (prov == "" || e.Provider == prov) {
			return true
		}
	}
	return false
}

func (r *recSink) typesList() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ts []string
	for _, e := range r.events {
		ts = append(ts, e.Type)
	}
	return ts
}

func TestFastest_emits_won_and_cancelled(t *testing.T) {
	slow := &provider.MockProvider{MockName: "slow", Delay: 80 * time.Millisecond, Chunks: []string{"s"}}
	fast := &provider.MockProvider{MockName: "fast", Delay: 5 * time.Millisecond, Chunks: []string{"f"}}
	sink := &recSink{}

	out := router.Fastest(context.Background(), []provider.Provider{slow, fast}, provider.Request{}, sink)
	for range out {
	}

	if !sink.has("provider_start", "fast") || !sink.has("provider_start", "slow") {
		t.Errorf("expected provider_start for both, got %v", sink.typesList())
	}
	if !sink.has("won", "fast") {
		t.Errorf("expected won for fast, got %v", sink.typesList())
	}
	if !sink.has("cancelled", "slow") {
		t.Errorf("expected cancelled for slow, got %v", sink.typesList())
	}
	if !sink.has("done", "fast") {
		t.Errorf("expected done for fast, got %v", sink.typesList())
	}
}

func TestFastest_emits_failed_and_error_when_all_fail(t *testing.T) {
	p1 := &provider.MockProvider{MockName: "p1", FailWith: fmt.Errorf("down")}
	p2 := &provider.MockProvider{MockName: "p2", FailWith: fmt.Errorf("down")}
	sink := &recSink{}

	out := router.Fastest(context.Background(), []provider.Provider{p1, p2}, provider.Request{}, sink)
	for range out {
	}

	if !sink.has("failed", "p1") || !sink.has("failed", "p2") {
		t.Errorf("expected failed for both, got %v", sink.typesList())
	}
	if !sink.has("error", "") {
		t.Errorf("expected error event, got %v", sink.typesList())
	}
}

func TestDispatch_emits_start(t *testing.T) {
	m := &provider.MockProvider{MockName: "m", Chunks: []string{"hi"}}
	r := &router.Router{Providers: []provider.Provider{m}, Strategy: router.StrategyFastest}
	sink := &recSink{}

	out, err := r.Dispatch(context.Background(), provider.Request{}, sink)
	if err != nil {
		t.Fatal(err)
	}
	for range out {
	}
	if !sink.has("start", "") {
		t.Errorf("expected start event, got %v", sink.typesList())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./router/ -run 'TestFastest_emits|TestDispatch_emits' -v`
Expected: FAIL — events not emitted yet (no `won`/`cancelled`/`start`).

- [ ] **Step 3: Emit `start` in `Dispatch`**

In `router/router.go`, replace the `switch` in `Dispatch` with:

```go
	switch r.Strategy {
	case StrategyFastest, "":
		emit(sink, event.Event{Type: "start", Detail: "fastest"})
		return Fastest(ctx, r.Providers, req, sink), nil
	case StrategyCheapest:
		emit(sink, event.Event{Type: "start", Detail: "cheapest"})
		return Cheapest(ctx, r.Providers, req, sink), nil
	case StrategyFallback:
		emit(sink, event.Event{Type: "start", Detail: "fallback"})
		return Fallback(ctx, r.Providers, req, sink), nil
	default:
		return nil, fmt.Errorf("unknown strategy: %q", r.Strategy)
	}
```

- [ ] **Step 4: Emit lifecycle events in `Fastest`**

Replace the entire body of `router/strategy_fastest.go` with (keeps the existing concurrency structure, adds `emit(...)` calls; output to `out` is unchanged):

```go
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
```

- [ ] **Step 5: Run the router suite to verify pass (new + existing)**

Run: `go test ./router/ -v`
Expected: PASS — new emission tests AND the four original fastest/dispatch tests still pass (output behavior unchanged).

- [ ] **Step 6: Commit**

```bash
git add router/strategy_fastest.go router/router.go router/events_test.go
git commit -m "feat(router): emit start/provider_start/won/cancelled/chunk/done from Fastest"
```

---

## Task 4: Emit lifecycle events from `Fallback`

**Files:**
- Modify: `router/strategy_fallback.go`
- Modify: `router/events_test.go` (add one test)

- [ ] **Step 1: Add the failing test**

Append to `router/events_test.go`:

```go
func TestFallback_emits_failed_then_done(t *testing.T) {
	bad := &provider.MockProvider{MockName: "bad", FailWith: fmt.Errorf("down")}
	good := &provider.MockProvider{MockName: "good", Chunks: []string{"ok"}}
	sink := &recSink{}

	out := router.Fallback(context.Background(), []provider.Provider{bad, good}, provider.Request{}, sink)
	for range out {
	}

	if !sink.has("provider_start", "bad") || !sink.has("provider_start", "good") {
		t.Errorf("expected provider_start for both, got %v", sink.typesList())
	}
	if !sink.has("failed", "bad") {
		t.Errorf("expected failed for bad, got %v", sink.typesList())
	}
	if !sink.has("done", "good") {
		t.Errorf("expected done for good, got %v", sink.typesList())
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./router/ -run TestFallback_emits -v`
Expected: FAIL — no `provider_start`/`failed`/`done` emitted yet.

- [ ] **Step 3: Emit events in `Fallback`**

Replace the entire body of `router/strategy_fallback.go` with:

```go
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
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./router/ -v`
Expected: PASS (all router tests, including the four original fallback tests).

- [ ] **Step 5: Commit**

```bash
git add router/strategy_fallback.go router/events_test.go
git commit -m "feat(router): emit provider_start/chunk/failed/done from Fallback"
```

---

## Task 5: Emit `decision` from `Cheapest`

**Files:**
- Modify: `router/strategy_cheapest.go`
- Modify: `router/events_test.go` (add one test)

- [ ] **Step 1: Add the failing test**

Append to `router/events_test.go`:

```go
func TestCheapest_emits_decision(t *testing.T) {
	cheap := &provider.MockProvider{MockName: "cheap", MockCost: 0.001, Chunks: []string{"x"}}
	pricey := &provider.MockProvider{MockName: "pricey", MockCost: 0.05, Chunks: []string{"y"}}
	sink := &recSink{}
	req := provider.Request{Messages: []provider.Message{{Role: "user", Content: "hello world"}}}

	out := router.Cheapest(context.Background(), []provider.Provider{pricey, cheap}, req, sink)
	for range out {
	}

	if !sink.has("decision", "") {
		t.Errorf("expected decision event, got %v", sink.typesList())
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./router/ -run TestCheapest_emits -v`
Expected: FAIL — no `decision` event.

- [ ] **Step 3: Emit `decision` in `Cheapest`**

Replace the entire body of `router/strategy_cheapest.go` with:

```go
package router

import (
	"context"
	"fmt"
	"sort"

	"github.com/domiciano/llm-proxy/event"
	"github.com/domiciano/llm-proxy/provider"
)

func estimateCost(p provider.Provider, req provider.Request) float64 {
	tokens := 0
	for _, m := range req.Messages {
		tokens += len(m.Content) / 4
	}
	return float64(tokens) * p.CostPer1kTokens() / 1000.0
}

// Cheapest sorts providers by estimated token cost and delegates to Fallback,
// so if the cheapest provider fails it tries the next cheapest automatically.
func Cheapest(ctx context.Context, providers []provider.Provider, req provider.Request, sink event.Sink) <-chan provider.Chunk {
	ranked := make([]provider.Provider, len(providers))
	copy(ranked, providers)
	sort.Slice(ranked, func(i, j int) bool {
		return estimateCost(ranked[i], req) < estimateCost(ranked[j], req)
	})

	if sink != nil {
		detail := "order by cost: "
		for i, p := range ranked {
			if i > 0 {
				detail += ", "
			}
			detail += fmt.Sprintf("%s=$%.6f", p.Name(), estimateCost(p, req))
		}
		emit(sink, event.Event{Type: "decision", Detail: detail})
	}

	return Fallback(ctx, ranked, req, sink)
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./router/ -v`
Expected: PASS (all router tests, including the two original cheapest tests).

- [ ] **Step 5: Commit**

```bash
git add router/strategy_cheapest.go router/events_test.go
git commit -m "feat(router): emit cost decision from Cheapest"
```

---

## Task 6: `provider.Sabotage` decorator

**Files:**
- Create: `provider/sabotage.go`
- Create: `provider/sabotage_test.go`

- [ ] **Step 1: Write the failing tests**

Create `provider/sabotage_test.go`:

```go
package provider_test

import (
	"context"
	"testing"
	"time"

	"github.com/domiciano/llm-proxy/provider"
)

func drain(p provider.Provider, ctx context.Context) []provider.Chunk {
	out := make(chan provider.Chunk, 16)
	go func() { p.Stream(ctx, provider.Request{}, out) }() //nolint:errcheck
	var got []provider.Chunk
	for c := range out {
		got = append(got, c)
	}
	return got
}

func TestSabotage_passthrough_by_default(t *testing.T) {
	inner := &provider.MockProvider{MockName: "x", Chunks: []string{"a", "b"}}
	s := provider.NewSabotage(inner)
	var content string
	for _, c := range drain(s, context.Background()) {
		content += c.Content
	}
	if content != "ab" {
		t.Errorf("expected passthrough 'ab', got %q", content)
	}
}

func TestSabotage_fail_short_circuits(t *testing.T) {
	inner := &provider.MockProvider{MockName: "x", Chunks: []string{"a"}}
	s := provider.NewSabotage(inner)
	s.SetFail(true)
	got := drain(s, context.Background())
	if len(got) != 1 || got[0].Err == nil {
		t.Fatalf("expected single error chunk, got %v", got)
	}
}

func TestSabotage_delay_respects_cancelled_context(t *testing.T) {
	inner := &provider.MockProvider{MockName: "x", Chunks: []string{"a"}}
	s := provider.NewSabotage(inner)
	s.SetDelay(5 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	start := time.Now()
	drain(s, ctx)
	if time.Since(start) > time.Second {
		t.Errorf("delay did not abort on cancelled context")
	}
}

func TestSabotage_clear_restores_passthrough(t *testing.T) {
	inner := &provider.MockProvider{MockName: "x", Chunks: []string{"a"}}
	s := provider.NewSabotage(inner)
	s.SetFail(true)
	s.Clear()
	for _, c := range drain(s, context.Background()) {
		if c.Err != nil {
			t.Fatalf("expected no error after Clear, got %v", c.Err)
		}
	}
}

func TestSabotage_satisfies_provider_interface(t *testing.T) {
	var _ provider.Provider = provider.NewSabotage(&provider.MockProvider{MockName: "x"})
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./provider/ -run TestSabotage -v`
Expected: FAIL — `undefined: provider.NewSabotage`.

- [ ] **Step 3: Write the implementation**

Create `provider/sabotage.go`:

```go
package provider

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Sabotage wraps a real Provider and can, on command, force it to fail or add
// latency before streaming. It is used by the live visualization to demonstrate
// resilience and re-routing. With no sabotage set, it is a transparent passthrough
// (zero overhead in production).
type Sabotage struct {
	Inner      Provider
	mu         sync.RWMutex
	forceFail  bool
	extraDelay time.Duration
}

func NewSabotage(inner Provider) *Sabotage { return &Sabotage{Inner: inner} }

func (s *Sabotage) Name() string             { return s.Inner.Name() }
func (s *Sabotage) CostPer1kTokens() float64 { return s.Inner.CostPer1kTokens() }

func (s *Sabotage) Stream(ctx context.Context, req Request, out chan<- Chunk) error {
	s.mu.RLock()
	fail, delay := s.forceFail, s.extraDelay
	s.mu.RUnlock()

	if fail {
		defer close(out)
		err := errors.New("sabotaged: " + s.Inner.Name() + " forced down")
		select {
		case <-ctx.Done():
		case out <- Chunk{Provider: s.Inner.Name(), Err: err}:
		}
		return err
	}

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			close(out)
			return ctx.Err()
		}
	}

	// Passthrough: the inner provider owns and closes out.
	return s.Inner.Stream(ctx, req, out)
}

func (s *Sabotage) SetFail(v bool) {
	s.mu.Lock()
	s.forceFail = v
	s.mu.Unlock()
}

func (s *Sabotage) SetDelay(d time.Duration) {
	s.mu.Lock()
	s.extraDelay = d
	s.mu.Unlock()
}

func (s *Sabotage) Clear() {
	s.mu.Lock()
	s.forceFail = false
	s.extraDelay = 0
	s.mu.Unlock()
}

// compile-time interface check
var _ Provider = (*Sabotage)(nil)
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./provider/ -run TestSabotage -v`
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add provider/sabotage.go provider/sabotage_test.go
git commit -m "feat(provider): add runtime-controllable Sabotage decorator"
```

---

## Task 7: Update `server.New` signature and `Server` struct

This change keeps the project building before the viz handlers exist. New routes are NOT registered yet (their handlers arrive in Tasks 8–10).

**Files:**
- Modify: `server/server.go`
- Modify: `server/server_test.go`
- Modify: `main.go`

- [ ] **Step 1: Update `Server` and `New`**

Replace `server/server.go` with:

```go
package server

import (
	"net/http"

	"github.com/domiciano/llm-proxy/provider"
	"github.com/domiciano/llm-proxy/router"
)

type Server struct {
	Router   *router.Router
	Sabotage map[string]*provider.Sabotage
}

// New builds the HTTP mux. sabotage may be nil (the /viz/sabotage endpoint will
// then report 404 for any provider); production endpoints are unaffected.
func New(r *router.Router, sabotage map[string]*provider.Sabotage) *http.ServeMux {
	s := &Server{Router: r, Sabotage: sabotage}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /query", s.handleQuery)
	mux.HandleFunc("POST /v1/chat/completions", s.handleOpenAICompat)
	return mux
}
```

- [ ] **Step 2: Update existing `server_test.go` call sites**

In `server/server_test.go`, add `, nil` to all four `server.New(...)` calls:

- `server.New(newTestRouter([]string{"hello", " world"}), nil)`
- `server.New(newTestRouter([]string{}), nil)`
- `server.New(newTestRouter([]string{"Hi"}), nil)`
- `server.New(newTestRouter([]string{"Hello"}), nil)`

- [ ] **Step 3: Update `main.go` call site (temporary nil; registry added in Task 11)**

In `main.go`, change the server construction:

```go
	srv := &http.Server{
		Addr:        ":" + port,
		Handler:     server.New(r, nil),
		ReadTimeout: time.Duration(timeoutMs) * time.Millisecond,
		// WriteTimeout is 0 (disabled) to allow long-running SSE streams.
	}
```

- [ ] **Step 4: Build and test**

Run: `go build ./... && go test ./...`
Expected: PASS (production endpoints unchanged).

- [ ] **Step 5: Commit**

```bash
git add server/server.go server/server_test.go main.go
git commit -m "refactor(server): New accepts a sabotage registry; add Server.Sabotage field"
```

---

## Task 8: `/viz/stream` SSE handler

**Files:**
- Create: `server/handler_viz.go`
- Modify: `server/server.go` (register route)
- Create: `server/handler_viz_test.go`

- [ ] **Step 1: Write the failing tests**

Create `server/handler_viz_test.go`:

```go
package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/domiciano/llm-proxy/provider"
	"github.com/domiciano/llm-proxy/router"
	"github.com/domiciano/llm-proxy/server"
)

func TestVizStream_emits_events(t *testing.T) {
	r := &router.Router{
		Providers: []provider.Provider{&provider.MockProvider{MockName: "mock", Chunks: []string{"hi"}}},
		Strategy:  router.StrategyFastest,
	}
	mux := server.New(r, nil)
	req := httptest.NewRequest(http.MethodGet, "/viz/stream?q=hello&strategy=fastest", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"type":"provider_start"`) {
		t.Errorf("missing provider_start in: %s", body)
	}
	if !strings.Contains(body, `"type":"won"`) {
		t.Errorf("missing won in: %s", body)
	}
	if !strings.Contains(body, "data: [DONE]") {
		t.Errorf("missing [DONE] in: %s", body)
	}
}

func TestVizStream_400_without_q(t *testing.T) {
	mux := server.New(newTestRouter([]string{"x"}), nil)
	req := httptest.NewRequest(http.MethodGet, "/viz/stream", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestVizStream_400_invalid_strategy(t *testing.T) {
	mux := server.New(newTestRouter([]string{"x"}), nil)
	req := httptest.NewRequest(http.MethodGet, "/viz/stream?q=hi&strategy=bogus", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./server/ -run TestVizStream -v`
Expected: FAIL — route not registered (404, no event-stream content type).

- [ ] **Step 3: Write the handler**

Create `server/handler_viz.go`:

```go
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/domiciano/llm-proxy/event"
	"github.com/domiciano/llm-proxy/provider"
	"github.com/domiciano/llm-proxy/router"
)

// handleVizStream runs a dispatch with a telemetry sink and streams the
// resulting events to the browser as SSE. The chunk channel content is drained
// (and discarded) because the events already carry per-provider content.
func (s *Server) handleVizStream(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		http.Error(w, "query param q required", http.StatusBadRequest)
		return
	}
	strategy := router.Strategy(r.URL.Query().Get("strategy"))
	switch strategy {
	case "", router.StrategyFastest, router.StrategyCheapest, router.StrategyFallback:
	default:
		http.Error(w, "invalid strategy", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Connection", "keep-alive")

	rtr := &router.Router{Providers: s.Router.Providers, Strategy: strategy}
	sink := event.NewChanSink(64, time.Now(), r.Context().Done())

	go func() {
		chunks, err := rtr.Dispatch(r.Context(), provider.Request{
			Messages: []provider.Message{{Role: "user", Content: q}},
		}, sink)
		if err != nil {
			sink.Emit(event.Event{Type: "error", Detail: err.Error()})
			sink.Close()
			return
		}
		for range chunks { // drain; content already emitted as events
		}
		sink.Close()
	}()

	for e := range sink.Events() {
		data, _ := json.Marshal(e) // Event fields are plain scalars; cannot fail
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}
```

- [ ] **Step 4: Register the route**

In `server/server.go`, add inside `New` after the `/v1/chat/completions` line:

```go
	mux.HandleFunc("GET /viz/stream", s.handleVizStream)
```

- [ ] **Step 5: Run to verify pass**

Run: `go test ./server/ -run TestVizStream -v`
Expected: PASS (3 tests).

- [ ] **Step 6: Commit**

```bash
git add server/handler_viz.go server/handler_viz_test.go server/server.go
git commit -m "feat(server): add /viz/stream SSE endpoint emitting race events"
```

---

## Task 9: `/viz/sabotage` control handler

**Files:**
- Modify: `server/handler_viz.go` (add handler + request type)
- Modify: `server/server.go` (register route)
- Modify: `server/handler_viz_test.go` (add tests)

- [ ] **Step 1: Add the failing tests**

Append to `server/handler_viz_test.go`:

```go
func TestSabotage_404_unknown_provider(t *testing.T) {
	mux := server.New(newTestRouter([]string{"x"}), map[string]*provider.Sabotage{})
	body := `{"provider":"nope","mode":"fail"}`
	req := httptest.NewRequest(http.MethodPost, "/viz/sabotage", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSabotage_400_invalid_mode(t *testing.T) {
	sab := provider.NewSabotage(&provider.MockProvider{MockName: "openai", Chunks: []string{"x"}})
	mux := server.New(newTestRouter([]string{"x"}), map[string]*provider.Sabotage{"openai": sab})
	body := `{"provider":"openai","mode":"explode"}`
	req := httptest.NewRequest(http.MethodPost, "/viz/sabotage", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSabotage_200_sets_fail(t *testing.T) {
	sab := provider.NewSabotage(&provider.MockProvider{MockName: "openai", Chunks: []string{"x"}})
	mux := server.New(newTestRouter([]string{"x"}), map[string]*provider.Sabotage{"openai": sab})
	body := `{"provider":"openai","mode":"fail"}`
	req := httptest.NewRequest(http.MethodPost, "/viz/sabotage", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./server/ -run TestSabotage_ -v`
Expected: FAIL — route not registered.

- [ ] **Step 3: Add the handler**

Append to `server/handler_viz.go` (the `time` and `json`/`http` imports are already present):

```go
type sabotageReq struct {
	Provider string `json:"provider"`
	Mode     string `json:"mode"` // "fail" | "delay" | "clear"
	DelayMs  int    `json:"delay_ms,omitempty"`
}

func (s *Server) handleSabotage(w http.ResponseWriter, r *http.Request) {
	var req sabotageReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	sab, ok := s.Sabotage[req.Provider]
	if !ok {
		http.Error(w, "unknown provider", http.StatusNotFound)
		return
	}
	switch req.Mode {
	case "fail":
		sab.SetFail(true)
	case "delay":
		sab.SetDelay(time.Duration(req.DelayMs) * time.Millisecond)
	case "clear":
		sab.Clear()
	default:
		http.Error(w, "invalid mode", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
		"status":   "ok",
		"provider": req.Provider,
		"mode":     req.Mode,
	})
}
```

- [ ] **Step 4: Register the route**

In `server/server.go`, add inside `New`:

```go
	mux.HandleFunc("POST /viz/sabotage", s.handleSabotage)
```

- [ ] **Step 5: Run to verify pass**

Run: `go test ./server/ -v`
Expected: PASS (all server tests).

- [ ] **Step 6: Commit**

```bash
git add server/handler_viz.go server/handler_viz_test.go server/server.go
git commit -m "feat(server): add /viz/sabotage runtime control endpoint"
```

---

## Task 10: Static dashboard (embedded) at `/`

**Files:**
- Create: `server/static/index.html`
- Create: `server/static/app.js`
- Create: `server/static.go`
- Modify: `server/server.go` (register `GET /`)

- [ ] **Step 1: Create the HTML**

Create `server/static/index.html`:

```html
<!DOCTYPE html>
<html lang="pt-br">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>LLM Proxy — Corrida Paralela</title>
<style>
  :root { color-scheme: dark; }
  body { margin:0; font-family: system-ui, sans-serif; background:#0f0f12; color:#eee; padding:24px; }
  h1 { font-size:18px; margin:0 0 16px; }
  .bar-top { display:flex; gap:8px; align-items:center; margin-bottom:18px; flex-wrap:wrap; }
  #q { flex:1; min-width:240px; padding:8px 10px; border-radius:6px; border:1px solid #444; background:#1a1a1a; color:#eee; }
  .pill { font-size:12px; padding:5px 12px; border-radius:14px; border:1px solid #555; cursor:pointer; user-select:none; }
  .pill.on { background:#10a37f; color:#fff; border-color:#10a37f; }
  #run { padding:8px 16px; border-radius:6px; border:0; background:#10a37f; color:#fff; font-weight:700; cursor:pointer; }
  .lane { display:flex; align-items:center; gap:10px; margin:10px 0; }
  .lane .pname { width:90px; font-size:13px; font-weight:700; }
  .track { position:relative; flex:1; height:28px; background:#ffffff14; border-radius:6px; overflow:hidden; }
  .fill { position:absolute; left:0; top:0; bottom:0; width:0; border-radius:6px; transition:width .12s linear; background:#666; }
  .lane.won .fill { background:#10a37f; }
  .lane.cancelled .fill { background:#555; opacity:.35; }
  .lane.failed .fill { background:#c0392b; }
  .badge { font-size:11px; padding:2px 8px; border-radius:10px; background:#333; color:#9bd; min-width:80px; text-align:center; }
  .sab button { font-size:12px; padding:3px 8px; border-radius:5px; border:1px solid #555; background:#222; color:#ddd; cursor:pointer; }
  #timeline { margin-top:18px; padding:10px 12px; background:#ffffff10; border-radius:6px; font-size:12px; line-height:1.7; min-height:60px; }
  #timeline b { color:#9bd; }
  .label { font-size:11px; text-transform:uppercase; letter-spacing:.05em; color:#888; }
</style>
</head>
<body>
  <h1>LLM Proxy — Corrida Paralela (fan-out · first-response-wins · context cancel)</h1>

  <div class="bar-top">
    <input id="q" placeholder="Pergunte algo..." value="Explique goroutines em uma frase">
    <span class="pill on" data-strategy="fastest">fastest</span>
    <span class="pill" data-strategy="cheapest">cheapest</span>
    <span class="pill" data-strategy="fallback">fallback</span>
    <button id="run">Run</button>
  </div>

  <div id="lanes"></div>

  <div class="label">timeline</div>
  <div id="timeline"></div>

  <script src="/app.js"></script>
</body>
</html>
```

- [ ] **Step 2: Create the JS client**

Create `server/static/app.js`:

```javascript
let strategy = "fastest";
const lanes = {}; // provider name -> { el, fill, badge, chunks }

document.querySelectorAll(".pill").forEach(p => {
  p.addEventListener("click", () => {
    document.querySelectorAll(".pill").forEach(x => x.classList.remove("on"));
    p.classList.add("on");
    strategy = p.dataset.strategy;
  });
});

document.getElementById("run").addEventListener("click", run);

// esc escapes server-provided strings before they touch innerHTML.
// Inputs here are self-controlled (provider names, our own error text), but
// escaping keeps the timeline XSS-safe regardless.
function esc(s) {
  return String(s == null ? "" : s)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

function run() {
  // reset
  document.getElementById("lanes").innerHTML = "";
  document.getElementById("timeline").innerHTML = "";
  for (const k in lanes) delete lanes[k];

  const q = encodeURIComponent(document.getElementById("q").value);
  const es = new EventSource(`/viz/stream?q=${q}&strategy=${strategy}`);

  es.onmessage = (msg) => {
    if (msg.data === "[DONE]") { es.close(); return; }
    const e = JSON.parse(msg.data);
    handle(e);
  };
  es.onerror = () => es.close();
}

function ensureLane(name) {
  if (lanes[name]) return lanes[name];
  const wrap = document.createElement("div");
  wrap.className = "lane";
  wrap.innerHTML = `
    <span class="pname">${esc(name)}</span>
    <div class="track"><div class="fill"></div></div>
    <span class="badge">running</span>
    <span class="sab">
      <button data-mode="fail">💥</button>
      <button data-mode="delay">⏱ +5s</button>
    </span>`;
  document.getElementById("lanes").appendChild(wrap);
  const lane = {
    el: wrap,
    fill: wrap.querySelector(".fill"),
    badge: wrap.querySelector(".badge"),
    chunks: 0,
  };
  wrap.querySelectorAll(".sab button").forEach(b => {
    b.addEventListener("click", () => sabotage(name, b.dataset.mode));
  });
  lanes[name] = lane;
  return lane;
}

function tl(text) {
  const t = document.getElementById("timeline");
  t.innerHTML += text + "<br>";
}

function handle(e) {
  switch (e.type) {
    case "start":
      tl(`<b>t=${e.t}ms</b> · start (${esc(e.detail)})`);
      break;
    case "provider_start": {
      ensureLane(e.provider);
      tl(`t=${e.t}ms · ${esc(e.provider)} iniciou`);
      break;
    }
    case "chunk": {
      const lane = ensureLane(e.provider);
      lane.chunks++;
      const w = Math.min(90, lane.chunks * 12);
      lane.fill.style.width = w + "%";
      break;
    }
    case "won": {
      const lane = ensureLane(e.provider);
      lane.el.classList.add("won");
      lane.fill.style.width = "100%";
      lane.badge.textContent = "WON";
      tl(`<b>t=${e.t}ms</b> · ${esc(e.provider)} venceu`);
      break;
    }
    case "cancelled": {
      const lane = ensureLane(e.provider);
      lane.el.classList.add("cancelled");
      lane.badge.textContent = "cancelled ❌";
      tl(`t=${e.t}ms · ${esc(e.provider)} cancelado (ctx)`);
      break;
    }
    case "failed": {
      const lane = ensureLane(e.provider);
      lane.el.classList.add("failed");
      lane.badge.textContent = "failed";
      tl(`t=${e.t}ms · ${esc(e.provider)} falhou: ${esc(e.detail || "")}`);
      break;
    }
    case "decision":
      tl(`t=${e.t}ms · decisão: ${esc(e.detail)}`);
      break;
    case "done": {
      const lane = ensureLane(e.provider);
      lane.fill.style.width = "100%";
      if (!lane.el.classList.contains("won")) lane.el.classList.add("won");
      tl(`<b>t=${e.t}ms</b> · ${esc(e.provider)} concluiu`);
      break;
    }
    case "error":
      tl(`<b>t=${e.t}ms</b> · ERRO: ${esc(e.detail)}`);
      break;
  }
}

function sabotage(provider, mode) {
  fetch("/viz/sabotage", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ provider, mode, delay_ms: 5000 }),
  });
}
```

> Note: the `error` case label text contains the literal string shown to the user; keep it as written in your editor as `ERRO` (ASCII). If any non-ASCII slipped in, replace with `ERRO`.

- [ ] **Step 3: Create the embed + handler**

Create `server/static.go`:

```go
package server

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var staticFiles embed.FS

// staticHandler serves the embedded dashboard (index.html, app.js) at the root.
func staticHandler() http.Handler {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	return http.FileServerFS(sub)
}
```

- [ ] **Step 4: Register the root route**

In `server/server.go`, add inside `New` (after the viz routes):

```go
	mux.Handle("GET /", staticHandler())
```

- [ ] **Step 5: Build and test**

Run: `go build ./... && go test ./...`
Expected: PASS. (Static files are served but not unit-tested; the build proves the embed compiles.)

- [ ] **Step 6: Manual smoke check (optional but recommended)**

Run: `go vet ./...`
Expected: no output.

- [ ] **Step 7: Commit**

```bash
git add server/static.go server/static/
git commit -m "feat(server): embed and serve the live dashboard at /"
```

---

## Task 11: Wire sabotage registry in `main.go`

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Wrap providers and build the registry**

In `main.go`, replace the body of `main()` from the `strategy :=` line down to the `srv :=` block with:

```go
	rawProviders := buildProviders()
	if len(rawProviders) == 0 {
		log.Fatal("no providers configured — set at least one of: OPENAI_API_KEY, ANTHROPIC_API_KEY, GEMINI_API_KEY")
	}

	// Wrap each real provider in a Sabotage decorator so the live dashboard can
	// inject failures/latency at runtime. Inert by default (zero overhead).
	sabotage := make(map[string]*provider.Sabotage, len(rawProviders))
	providers := make([]provider.Provider, len(rawProviders))
	for i, p := range rawProviders {
		s := provider.NewSabotage(p)
		sabotage[p.Name()] = s
		providers[i] = s
	}

	strategy := router.Strategy(getEnv("PROXY_STRATEGY", "fastest"))
	r := &router.Router{Providers: providers, Strategy: strategy}

	port := getEnv("PROXY_PORT", "8080")
	timeoutMs, _ := strconv.Atoi(getEnv("PROXY_TIMEOUT_MS", "5000"))

	srv := &http.Server{
		Addr:        ":" + port,
		Handler:     server.New(r, sabotage),
		ReadTimeout: time.Duration(timeoutMs) * time.Millisecond,
		// WriteTimeout is 0 (disabled) to allow long-running SSE streams.
	}
```

> The first two lines replace the old `providers := buildProviders()` + length check. Ensure the old guard block is removed so it is not duplicated.

- [ ] **Step 2: Build and test**

Run: `go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add main.go
git commit -m "feat: wrap providers in Sabotage and wire the registry into the server"
```

---

## Task 12: Intentional data-race demo (didactic) + final verification

This is a standalone example for the presentation's "why correct concurrency matters" segment. It is NOT part of the production binary and lives under `examples/`.

**Files:**
- Create: `examples/racecondition/main.go`

- [ ] **Step 1: Create the racy example**

Create `examples/racecondition/main.go`:

```go
//go:build ignore

// Demonstração didática (NÃO faz parte do proxy de produção).
// Roda com:  go run -race examples/racecondition/main.go
// O -race detector aponta o data race no map compartilhado sem proteção.
package main

import (
	"fmt"
	"sync"
)

func main() {
	counts := map[string]int{} // map compartilhado, SEM mutex — propositalmente errado
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			counts["chunks"]++ // concurrent map writes -> DATA RACE
		}()
	}
	wg.Wait()
	fmt.Println("counts:", counts)
}
```

> The `//go:build ignore` tag keeps this file out of `go build ./...` and `go test ./...`, so it never affects the production binary. It is run explicitly with `go run -race`.

- [ ] **Step 2: Verify the demo triggers the race detector**

Run: `go run -race examples/racecondition/main.go`
Expected: prints `WARNING: DATA RACE` with a stack trace (and a non-deterministic count). This is the intended teaching output.

- [ ] **Step 3: Verify the production build is unaffected and fully green**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: build succeeds, all tests PASS, vet clean.

- [ ] **Step 4: Format check**

Run: `gofmt -l .`
Expected: no output (all files formatted). If any file is listed, run `gofmt -w <file>` and re-check.

- [ ] **Step 5: Commit**

```bash
git add examples/
git commit -m "docs(examples): add intentional data-race demo for the presentation"
```

---

## Done — Demo Runbook (for the presentation)

1. `OPENAI_API_KEY=... ANTHROPIC_API_KEY=... GEMINI_API_KEY=... go run .`
2. Open `http://localhost:8080/`.
3. Type a prompt, keep `fastest`, click **Run** → the three lanes race; the winner fills green, losers go `cancelled ❌`; the timeline narrates the milliseconds.
4. Click **💥** on the leading provider, **Run** again → it fails (red) and another provider wins live (resilience).
5. Switch to `fallback` / `cheapest` to show sequential vs cost-based behavior.
6. In a terminal: `go run -race examples/racecondition/main.go` → show the `DATA RACE` warning, then explain how the proxy's channel/`select` design avoids exactly this.

---

## Self-Review Notes (author)

- **Spec coverage:** event model (T1), router emission incl. all 3 strategies (T3–T5), `/viz/stream` (T8), `/viz/sabotage` + decorator (T6, T9), static embed at `/` (T10), `main.go` wiring (T11), error handling (400/404/invalid mode/all-fail `error` event — T3,T8,T9), tests per package (each task), race-condition didactic segment (T12), `Sink == nil` ⇒ unchanged production (verified by existing tests passing in T2/T7). All spec sections mapped.
- **Type consistency:** `event.Sink` / `event.Event{Type,Provider,T,Content,Detail}` / `NewChanSink(buf, start, done)` / `Events()` / `Close()` used identically across T1, T3–T5, T8. `provider.NewSabotage` / `SetFail` / `SetDelay` / `Clear` consistent across T6, T9, T11. `server.New(r, sabotage)` consistent across T7–T11.
- **No placeholders:** every code step contains full code; commands have expected output.
