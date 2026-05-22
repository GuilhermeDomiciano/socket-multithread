package router_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/domiciano/llm-proxy/provider"
	"github.com/domiciano/llm-proxy/router"
)

func TestCheapest_dispatches_to_lowest_cost_provider(t *testing.T) {
	expensive := &provider.MockProvider{MockName: "expensive", MockCost: 5.0, Chunks: []string{"expensive"}}
	cheap := &provider.MockProvider{MockName: "cheap", MockCost: 0.1, Chunks: []string{"cheap"}}

	req := provider.Request{
		Messages: []provider.Message{{Role: "user", Content: "hello world"}},
	}
	out := router.Cheapest(context.Background(), []provider.Provider{expensive, cheap}, req)

	for c := range out {
		if c.Done || c.Err != nil {
			continue
		}
		if c.Provider != "cheap" {
			t.Errorf("expected chunk from 'cheap', got provider=%q", c.Provider)
		}
	}
}

func TestCheapest_falls_back_when_cheapest_fails(t *testing.T) {
	failing := &provider.MockProvider{MockName: "failing", MockCost: 0.01, FailWith: fmt.Errorf("down")}
	working := &provider.MockProvider{MockName: "working", MockCost: 1.0, Chunks: []string{"ok"}}

	req := provider.Request{
		Messages: []provider.Message{{Role: "user", Content: "hi"}},
	}
	out := router.Cheapest(context.Background(), []provider.Provider{failing, working}, req)

	var contents []string
	for c := range out {
		if !c.Done && c.Err == nil {
			contents = append(contents, c.Content)
		}
	}
	if len(contents) == 0 {
		t.Error("expected content from fallback provider")
	}
}
