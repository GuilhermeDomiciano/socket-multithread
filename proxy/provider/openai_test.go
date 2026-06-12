package provider_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/domiciano/llm-proxy/provider"
)

func TestOpenAI_streams_content_chunks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("missing Authorization header")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}`)
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":" world"},"finish_reason":null}]}`)
		fmt.Fprintln(w, `data: [DONE]`)
	}))
	defer srv.Close()

	p := provider.NewOpenAI("test-key", "gpt-4o")
	p.BaseURL = srv.URL

	out := make(chan provider.Chunk, 10)
	go p.Stream(context.Background(), provider.Request{
		Messages: []provider.Message{{Role: "user", Content: "hi"}},
	}, out)

	var contents []string
	for c := range out {
		if c.Err != nil {
			t.Fatalf("unexpected error: %v", c.Err)
		}
		if !c.Done {
			contents = append(contents, c.Content)
		}
	}
	if strings.Join(contents, "") != "Hello world" {
		t.Fatalf("got %v", contents)
	}
}

func TestOpenAI_returns_error_on_non_200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limit", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	p := provider.NewOpenAI("test-key", "gpt-4o")
	p.BaseURL = srv.URL

	out := make(chan provider.Chunk, 5)
	go p.Stream(context.Background(), provider.Request{
		Messages: []provider.Message{{Role: "user", Content: "hi"}},
	}, out)

	var last provider.Chunk
	for c := range out {
		last = c
	}
	if last.Err == nil {
		t.Error("expected error chunk on HTTP 429")
	}
}

func TestOpenAI_name_unique_per_model(t *testing.T) {
	a := provider.NewOpenAI("k", "gpt-4o")
	b := provider.NewOpenAI("k", "gpt-4o-mini")
	if a.Name() != "openai:gpt-4o" {
		t.Errorf("expected openai:gpt-4o, got %q", a.Name())
	}
	if a.Name() == b.Name() {
		t.Errorf("distinct models must have distinct Name(), both %q", a.Name())
	}
	if !(b.CostPer1kTokens() < a.CostPer1kTokens()) {
		t.Errorf("gpt-4o-mini (%.4f) should cost less than gpt-4o (%.4f)", b.CostPer1kTokens(), a.CostPer1kTokens())
	}
}

func TestOpenAI_unknown_model_falls_back_to_default_cost(t *testing.T) {
	p := provider.NewOpenAI("k", "some-future-model")
	if p.CostPer1kTokens() != 0.005 {
		t.Errorf("unknown model should fall back to 0.005, got %.4f", p.CostPer1kTokens())
	}
	if p.Name() != "openai:some-future-model" {
		t.Errorf("got %q", p.Name())
	}
}
