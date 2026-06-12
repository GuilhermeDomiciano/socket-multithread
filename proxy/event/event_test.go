package event_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/domiciano/llm-proxy/event"
)

func TestEvent_phase_omitempty(t *testing.T) {
	b, _ := json.Marshal(event.Event{Type: "chunk"})
	if strings.Contains(string(b), "phase") {
		t.Errorf("phase deve sumir quando vazio: %s", b)
	}
	b2, _ := json.Marshal(event.Event{Type: "chunk", Phase: "seq"})
	if !strings.Contains(string(b2), `"phase":"seq"`) {
		t.Errorf("phase deveria aparecer: %s", b2)
	}
}

func TestChanSink_emits_in_order(t *testing.T) {
	done := make(chan struct{})
	s := event.NewChanSink(8, time.Now(), done)
	go func() {
		s.Emit(event.Event{Type: "a"})
		s.Emit(event.Event{Type: "b"})
		s.Close()
	}()
	var got []string
	for e := range s.Events() {
		got = append(got, e.Type)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("expected [a b], got %v", got)
	}
}

func TestChanSink_does_not_block_after_done(t *testing.T) {
	done := make(chan struct{})
	close(done)
	s := event.NewChanSink(0, time.Now(), done) // unbuffered + done closed
	finished := make(chan struct{})
	go func() {
		s.Emit(event.Event{Type: "x"}) // must take the done branch, not block
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("Emit blocked after done closed")
	}
}

func TestChanSink_stamps_relative_time(t *testing.T) {
	done := make(chan struct{})
	start := time.Now().Add(-50 * time.Millisecond)
	s := event.NewChanSink(1, start, done)
	s.Emit(event.Event{Type: "a"})
	s.Close()
	e := <-s.Events()
	if e.T < 40 {
		t.Fatalf("expected T >= ~50ms, got %d", e.T)
	}
}
