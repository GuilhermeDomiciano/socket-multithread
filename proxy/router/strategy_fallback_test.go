package router_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/domiciano/llm-proxy/provider"
	"github.com/domiciano/llm-proxy/router"
)

func TestFallback_skips_failing_provider_and_uses_next(t *testing.T) {
	failing := &provider.MockProvider{MockName: "failing", FailWith: fmt.Errorf("down")}
	working := &provider.MockProvider{MockName: "working", Chunks: []string{"ok"}}

	out := router.Fallback(context.Background(), []provider.Provider{failing, working}, provider.Request{})

	var contents []string
	for c := range out {
		if c.Err != nil {
			t.Fatalf("unexpected error: %v", c.Err)
		}
		if !c.Done {
			contents = append(contents, c.Content)
		}
	}
	if len(contents) != 1 || contents[0] != "ok" {
		t.Fatalf("got %v", contents)
	}
}

func TestFallback_returns_error_when_all_providers_fail(t *testing.T) {
	p1 := &provider.MockProvider{MockName: "p1", FailWith: fmt.Errorf("p1 down")}
	p2 := &provider.MockProvider{MockName: "p2", FailWith: fmt.Errorf("p2 down")}

	out := router.Fallback(context.Background(), []provider.Provider{p1, p2}, provider.Request{})

	var last provider.Chunk
	for c := range out {
		last = c
	}
	if last.Err == nil {
		t.Error("expected error chunk when all providers fail")
	}
}

func TestFallback_returns_chunks_from_first_working_provider(t *testing.T) {
	good := &provider.MockProvider{MockName: "good", Chunks: []string{"a", "b"}}

	out := router.Fallback(context.Background(), []provider.Provider{good}, provider.Request{})

	var contents []string
	for c := range out {
		if !c.Done && c.Err == nil {
			contents = append(contents, c.Content)
		}
	}
	if len(contents) != 2 {
		t.Fatalf("expected 2 chunks, got %v", contents)
	}
}
