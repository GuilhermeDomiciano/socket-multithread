package router_test

import (
	"context"
	"testing"
	"time"

	"github.com/domiciano/llm-proxy/provider"
	"github.com/domiciano/llm-proxy/router"
)

func TestParallelAll_done_for_all_no_cancel(t *testing.T) {
	p1 := &provider.MockProvider{MockName: "p1", Delay: 10 * time.Millisecond, Chunks: []string{"a"}}
	p2 := &provider.MockProvider{MockName: "p2", Delay: 30 * time.Millisecond, Chunks: []string{"b"}}
	sink := &recSink{}

	out := router.ParallelAll(context.Background(), []provider.Provider{p1, p2}, provider.Request{}, sink)
	for range out {
	}

	// Nenhum perdedor é cancelado: ambos completam.
	if !sink.has("done", "p1") || !sink.has("done", "p2") {
		t.Errorf("esperava done para ambos (sem cancelamento), got %v", sink.typesList())
	}
}

func TestParallelAll_overlaps_latencies(t *testing.T) {
	p1 := &provider.MockProvider{MockName: "p1", Delay: 60 * time.Millisecond, Chunks: []string{"a"}}
	p2 := &provider.MockProvider{MockName: "p2", Delay: 60 * time.Millisecond, Chunks: []string{"b"}}

	start := time.Now()
	out := router.ParallelAll(context.Background(), []provider.Provider{p1, p2}, provider.Request{}, nil)
	for range out {
	}
	// Paralelo ≈ máximo (~60ms), não soma (~120ms).
	if elapsed := time.Since(start); elapsed > 110*time.Millisecond {
		t.Errorf("parallel-all deveria sobrepor (~60ms), levou %v", elapsed)
	}
}
