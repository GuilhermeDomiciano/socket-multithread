package provider_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/domiciano/llm-proxy/provider"
)

func TestMockProvider_streams_all_chunks(t *testing.T) {
	p := &provider.MockProvider{
		MockName: "mock",
		Chunks:   []string{"hello", " world"},
	}
	out := make(chan provider.Chunk, 10)
	go p.Stream(context.Background(), provider.Request{}, out)

	var got []string
	for c := range out {
		if c.Err != nil {
			t.Fatalf("unexpected error: %v", c.Err)
		}
		if !c.Done {
			got = append(got, c.Content)
		}
	}
	if len(got) != 2 || got[0] != "hello" || got[1] != " world" {
		t.Fatalf("got %v", got)
	}
}

func TestMockProvider_sends_error_chunk(t *testing.T) {
	p := &provider.MockProvider{
		MockName: "mock",
		FailWith: fmt.Errorf("boom"),
	}
	out := make(chan provider.Chunk, 10)
	go p.Stream(context.Background(), provider.Request{}, out)

	var chunks []provider.Chunk
	for c := range out {
		chunks = append(chunks, c)
	}
	if len(chunks) != 1 || chunks[0].Err == nil {
		t.Fatalf("expected one error chunk, got %v", chunks)
	}
}

func TestMockProvider_respects_context_cancel(t *testing.T) {
	p := &provider.MockProvider{
		MockName: "mock",
		Delay:    100 * time.Millisecond,
		Chunks:   []string{"a", "b", "c", "d", "e"},
	}
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan provider.Chunk, 10)
	go p.Stream(ctx, provider.Request{}, out)

	cancel()

	var count int
	for c := range out {
		if !c.Done && c.Err == nil {
			count++
		}
	}
	if count == 5 {
		t.Fatal("expected context cancel to stop provider before all 5 chunks")
	}
}
