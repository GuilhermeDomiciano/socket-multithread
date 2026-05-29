// Package event carries optional, plug-in telemetry for the router.
// The production data path never depends on it: when a Sink is nil,
// no events are emitted and behavior is unchanged.
package event

import "time"

// Event is one observable moment in a dispatch (a provider started, won,
// was cancelled, produced a chunk, etc.). T is milliseconds since the race began.
type Event struct {
	Type     string `json:"type"`
	Provider string `json:"provider,omitempty"`
	T        int64  `json:"t"`
	Content  string `json:"content,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// Sink receives events. Implementations must be safe for concurrent Emit calls.
type Sink interface {
	Emit(Event)
}

// ChanSink is a Sink backed by a buffered channel. It stamps each event with
// the elapsed time since start and never blocks once done is closed (so a
// disconnected client cannot leak the producing goroutines).
type ChanSink struct {
	ch    chan Event
	done  <-chan struct{}
	start time.Time
}

func NewChanSink(buf int, start time.Time, done <-chan struct{}) *ChanSink {
	return &ChanSink{ch: make(chan Event, buf), done: done, start: start}
}

func (s *ChanSink) Emit(e Event) {
	e.T = time.Since(s.start).Milliseconds()
	select {
	case s.ch <- e:
	case <-s.done:
	}
}

// Events returns the read side of the channel.
func (s *ChanSink) Events() <-chan Event { return s.ch }

// Close closes the channel. Call only after the producing dispatch has finished.
func (s *ChanSink) Close() { close(s.ch) }
