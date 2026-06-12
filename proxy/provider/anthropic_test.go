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

func TestAnthropic_streams_content_chunks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") == "" {
			t.Error("missing x-api-key header")
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Error("missing anthropic-version header")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello"}}`)
		fmt.Fprintln(w, `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":" Claude"}}`)
		fmt.Fprintln(w, `data: {"type":"message_stop"}`)
	}))
	defer srv.Close()

	p := provider.NewAnthropic("test-key", "claude-3-5-sonnet-20241022")
	p.BaseURL = srv.URL

	out := make(chan provider.Chunk, 10)
	go p.Stream(context.Background(), provider.Request{
		Messages:  []provider.Message{{Role: "user", Content: "hi"}},
		MaxTokens: 100,
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
	if strings.Join(contents, "") != "Hello Claude" {
		t.Fatalf("got %v", contents)
	}
}

func TestAnthropic_returns_error_on_non_200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "overloaded", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	p := provider.NewAnthropic("test-key", "claude-3-5-sonnet-20241022")
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
		t.Error("expected error chunk on HTTP 503")
	}
}
