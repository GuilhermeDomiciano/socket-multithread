package provider

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Sabotage wraps a real Provider and can, on command, force it to fail or add
// latency before streaming. It is used by the live visualization to demonstrate
// resilience and re-routing. With no sabotage set, it is a transparent passthrough
// (zero overhead in production).
type Sabotage struct {
	Inner      Provider
	mu         sync.RWMutex
	forceFail  bool
	extraDelay time.Duration
}

func NewSabotage(inner Provider) *Sabotage { return &Sabotage{Inner: inner} }

func (s *Sabotage) Name() string             { return s.Inner.Name() }
func (s *Sabotage) CostPer1kTokens() float64 { return s.Inner.CostPer1kTokens() }

func (s *Sabotage) Stream(ctx context.Context, req Request, out chan<- Chunk) error {
	s.mu.RLock()
	fail, delay := s.forceFail, s.extraDelay
	s.mu.RUnlock()

	if fail {
		defer close(out)
		err := errors.New("sabotaged: " + s.Inner.Name() + " forced down")
		select {
		case <-ctx.Done():
		case out <- Chunk{Provider: s.Inner.Name(), Err: err}:
		}
		return err
	}

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			close(out)
			return ctx.Err()
		}
	}

	// Passthrough: the inner provider owns and closes out.
	return s.Inner.Stream(ctx, req, out)
}

func (s *Sabotage) SetFail(v bool) {
	s.mu.Lock()
	s.forceFail = v
	s.mu.Unlock()
}

func (s *Sabotage) SetDelay(d time.Duration) {
	s.mu.Lock()
	s.extraDelay = d
	s.mu.Unlock()
}

func (s *Sabotage) Clear() {
	s.mu.Lock()
	s.forceFail = false
	s.extraDelay = 0
	s.mu.Unlock()
}

// compile-time interface check
var _ Provider = (*Sabotage)(nil)
