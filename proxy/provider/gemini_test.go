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

func TestGemini_streams_content_chunks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "key=") {
			t.Error("missing API key in query string")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, `data: {"candidates":[{"content":{"parts":[{"text":"Hi"}],"role":"model"}}]}`)
		fmt.Fprintln(w, `data: {"candidates":[{"content":{"parts":[{"text":" there"}],"role":"model"},"finishReason":"STOP"}]}`)
	}))
	defer srv.Close()

	p := provider.NewGemini("test-key", "gemini-2.5-flash")
	p.BaseURL = srv.URL

	out := make(chan provider.Chunk, 10)
	go p.Stream(context.Background(), provider.Request{
		Messages: []provider.Message{{Role: "user", Content: "hello"}},
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
	if strings.Join(contents, "") != "Hi there" {
		t.Fatalf("got %v", contents)
	}
}

func TestGemini_returns_error_on_non_200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "quota exceeded", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	p := provider.NewGemini("test-key", "gemini-2.5-flash")
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
	if !strings.Contains(last.Err.Error(), "quota exceeded") {
		t.Errorf("expected error to surface the response body, got: %v", last.Err)
	}
}

func TestGemini_name_unique_per_model(t *testing.T) {
	a := provider.NewGemini("k", "gemini-2.5-flash")
	b := provider.NewGemini("k", "gemini-2.5-pro")
	if a.Name() != "gemini:gemini-2.5-flash" {
		t.Errorf("expected gemini:gemini-2.5-flash, got %q", a.Name())
	}
	if a.Name() == b.Name() {
		t.Errorf("distinct models must have distinct Name(), both %q", a.Name())
	}
	if !(a.CostPer1kTokens() < b.CostPer1kTokens()) {
		t.Errorf("flash (%.5f) should cost less than pro (%.5f)", a.CostPer1kTokens(), b.CostPer1kTokens())
	}
}

func TestGemini_empty_model_defaults(t *testing.T) {
	p := provider.NewGemini("k", "")
	if p.Name() != "gemini:gemini-2.5-flash" {
		t.Errorf("empty model should default to gemini-2.5-flash, got %q", p.Name())
	}
}
