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

func TestFallback_forwards_error_after_partial_content(t *testing.T) {
	// Provider sends one content chunk then an error.
	// Fallback must forward the error and NOT try the next provider.
	backup := &provider.MockProvider{MockName: "backup", Chunks: []string{"backup content"}}

	// Create a provider that sends one content chunk then an error chunk.
	contentThenError := &contentThenErrorProvider{name: "partial"}

	out := router.Fallback(context.Background(), []provider.Provider{contentThenError, backup}, provider.Request{})

	var gotContent bool
	var gotError bool
	for c := range out {
		if !c.Done && c.Err == nil && c.Content != "" {
			gotContent = true
		}
		if c.Err != nil {
			gotError = true
			if c.Provider == "backup" {
				t.Error("fallback should not have tried backup provider after partial content")
			}
		}
	}
	if !gotContent {
		t.Error("expected to receive the partial content chunk")
	}
	if !gotError {
		t.Error("expected to receive the error chunk after partial content")
	}
	_ = backup
}

// contentThenErrorProvider sends one content chunk then one error chunk.
type contentThenErrorProvider struct {
	name string
}

func (p *contentThenErrorProvider) Name() string             { return p.name }
func (p *contentThenErrorProvider) CostPer1kTokens() float64 { return 0.001 }
func (p *contentThenErrorProvider) Stream(ctx context.Context, req provider.Request, out chan<- provider.Chunk) error {
	defer close(out)
	out <- provider.Chunk{Content: "partial", Provider: p.name}
	out <- provider.Chunk{Err: fmt.Errorf("mid-stream failure"), Provider: p.name}
	return fmt.Errorf("mid-stream failure")
}
