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
