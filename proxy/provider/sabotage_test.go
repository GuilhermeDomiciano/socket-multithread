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
