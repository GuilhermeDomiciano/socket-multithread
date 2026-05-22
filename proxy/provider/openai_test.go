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

	p := provider.NewOpenAI("test-key")
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

	p := provider.NewOpenAI("test-key")
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
