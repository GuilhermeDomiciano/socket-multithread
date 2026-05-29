package router_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/domiciano/llm-proxy/provider"
	"github.com/domiciano/llm-proxy/router"
)

func TestFastest_returns_chunks_from_fastest_provider(t *testing.T) {
	slow := &provider.MockProvider{MockName: "slow", Delay: 80 * time.Millisecond, Chunks: []string{"slow"}}
	fast := &provider.MockProvider{MockName: "fast", Delay: 5 * time.Millisecond, Chunks: []string{"fast"}}

	out := router.Fastest(context.Background(), []provider.Provider{slow, fast}, provider.Request{}, nil)

	for c := range out {
		if c.Done || c.Err != nil {
			continue
		}
		if c.Provider != "fast" {
			t.Errorf("expected chunk from 'fast', got provider=%q", c.Provider)
		}
	}
}

func TestFastest_completes_quickly_when_one_provider_is_fast(t *testing.T) {
	slow := &provider.MockProvider{MockName: "slow", Delay: 300 * time.Millisecond, Chunks: []string{"s1", "s2"}}
	fast := &provider.MockProvider{MockName: "fast", Delay: 5 * time.Millisecond, Chunks: []string{"f1"}}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	out := router.Fastest(ctx, []provider.Provider{slow, fast}, provider.Request{}, nil)
	for range out {
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Errorf("fastest took %v — slow provider not cancelled in time", elapsed)
	}
}

func TestFastest_returns_error_when_all_providers_fail(t *testing.T) {
	p1 := &provider.MockProvider{MockName: "p1", FailWith: fmt.Errorf("p1 down")}
	p2 := &provider.MockProvider{MockName: "p2", FailWith: fmt.Errorf("p2 down")}

	out := router.Fastest(context.Background(), []provider.Provider{p1, p2}, provider.Request{}, nil)

	var last provider.Chunk
	for c := range out {
		last = c
	}
	if last.Err == nil {
		t.Error("expected error chunk when all providers fail")
	}
}
