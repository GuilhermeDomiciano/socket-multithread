package router_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/domiciano/llm-proxy/provider"
	"github.com/domiciano/llm-proxy/router"
)

func TestSequentialAll_done_for_all_providers(t *testing.T) {
	p1 := &provider.MockProvider{MockName: "p1", Chunks: []string{"a"}}
	p2 := &provider.MockProvider{MockName: "p2", Chunks: []string{"b"}}
	sink := &recSink{}

	out := router.SequentialAll(context.Background(), []provider.Provider{p1, p2}, provider.Request{}, sink)
	for range out {
	}

	if !sink.has("done", "p1") || !sink.has("done", "p2") {
		t.Errorf("esperava done para ambos, got %v", sink.typesList())
	}
}

func TestSequentialAll_continues_past_failure(t *testing.T) {
	bad := &provider.MockProvider{MockName: "bad", FailWith: fmt.Errorf("down")}
	good := &provider.MockProvider{MockName: "good", Chunks: []string{"ok"}}
	sink := &recSink{}

	out := router.SequentialAll(context.Background(), []provider.Provider{bad, good}, provider.Request{}, sink)
	for range out {
	}

	if !sink.has("failed", "bad") {
		t.Errorf("esperava failed para bad, got %v", sink.typesList())
	}
	if !sink.has("done", "good") {
		t.Errorf("não pode parar no primeiro erro — esperava done para good, got %v", sink.typesList())
	}
}

func TestSequentialAll_call_timeout_bounds_stuck_provider(t *testing.T) {
	old := router.CallTimeout
	router.CallTimeout = 40 * time.Millisecond
	defer func() { router.CallTimeout = old }()

	stuck := &provider.MockProvider{MockName: "stuck", Delay: 5 * time.Second, Chunks: []string{"x"}}
	start := time.Now()
	out := router.SequentialAll(context.Background(), []provider.Provider{stuck}, provider.Request{}, nil)
	for range out {
	}
	if elapsed := time.Since(start); elapsed > 1*time.Second {
		t.Errorf("CallTimeout not applied — stuck provider froze for %v", elapsed)
	}
}
