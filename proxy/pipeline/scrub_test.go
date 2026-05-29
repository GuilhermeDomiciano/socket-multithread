package pipeline

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/domiciano/llm-proxy/event"
	"github.com/domiciano/llm-proxy/guardrail"
	"github.com/domiciano/llm-proxy/provider"
)

type recSink struct {
	mu     sync.Mutex
	events []event.Event
}

func (r *recSink) Emit(e event.Event) {
	r.mu.Lock()
	r.events = append(r.events, e)
	r.mu.Unlock()
}

func (r *recSink) has(typ string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.events {
		if e.Type == typ {
			return true
		}
	}
	return false
}

func feed(chunks ...provider.Chunk) <-chan provider.Chunk {
	ch := make(chan provider.Chunk, len(chunks)+1)
	for _, c := range chunks {
		ch <- c
	}
	close(ch)
	return ch
}

func collect(ch <-chan provider.Chunk) string {
	var b strings.Builder
	for c := range ch {
		b.WriteString(c.Content)
	}
	return b.String()
}

func TestScrub_masks_pii_in_output(t *testing.T) {
	sink := &recSink{}
	in := feed(
		provider.Chunk{Content: "contato: joao@empresa.com pronto para ajudar voce hoje", Provider: "p"},
		provider.Chunk{Provider: "p", Done: true},
	)
	got := collect(scrub(context.Background(), in, guardrail.NewPIIGuard(), sink))
	if strings.Contains(got, "joao@empresa.com") {
		t.Errorf("output email not masked: %q", got)
	}
	if !sink.has("guard_out") {
		t.Error("expected a guard_out event")
	}
	if !sink.has("out_chunk") {
		t.Error("expected an out_chunk event carrying the sanitized answer")
	}
}

func TestScrub_catches_pii_split_across_chunks(t *testing.T) {
	sink := &recSink{}
	// email split across two chunks; carry buffer must reunite it.
	in := feed(
		provider.Chunk{Content: "escreva para joao@", Provider: "p"},
		provider.Chunk{Content: "empresa.com agora", Provider: "p"},
		provider.Chunk{Provider: "p", Done: true},
	)
	got := collect(scrub(context.Background(), in, guardrail.NewPIIGuard(), sink))
	if strings.Contains(got, "joao@empresa.com") {
		t.Errorf("boundary-split email leaked: %q", got)
	}
}

func TestScrub_passes_clean_stream_through(t *testing.T) {
	sink := &recSink{}
	in := feed(
		provider.Chunk{Content: "olá ", Provider: "p"},
		provider.Chunk{Content: "mundo", Provider: "p"},
		provider.Chunk{Provider: "p", Done: true},
	)
	got := collect(scrub(context.Background(), in, guardrail.NewPIIGuard(), sink))
	if got != "olá mundo" {
		t.Errorf("clean stream altered: %q", got)
	}
}
