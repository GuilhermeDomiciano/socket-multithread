package router_test

import (
	"context"
	"testing"

	"github.com/domiciano/llm-proxy/provider"
	"github.com/domiciano/llm-proxy/router"
)

func TestRouter_dispatches_fastest_strategy(t *testing.T) {
	p := &provider.MockProvider{MockName: "p", Chunks: []string{"hi"}}
	r := &router.Router{Providers: []provider.Provider{p}, Strategy: router.StrategyFastest}

	chunks, err := r.Dispatch(context.Background(), provider.Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got []string
	for c := range chunks {
		if !c.Done && c.Err == nil {
			got = append(got, c.Content)
		}
	}
	if len(got) != 1 || got[0] != "hi" {
		t.Fatalf("got %v", got)
	}
}

func TestRouter_returns_error_for_unknown_strategy(t *testing.T) {
	p := &provider.MockProvider{MockName: "p", Chunks: []string{"hi"}}
	r := &router.Router{Providers: []provider.Provider{p}, Strategy: "unknown"}

	_, err := r.Dispatch(context.Background(), provider.Request{})
	if err == nil {
		t.Error("expected error for unknown strategy")
	}
}

func TestRouter_returns_error_when_no_providers(t *testing.T) {
	r := &router.Router{Providers: nil, Strategy: router.StrategyFastest}
	_, err := r.Dispatch(context.Background(), provider.Request{})
	if err == nil {
		t.Error("expected error when providers list is empty")
	}
}
