package provider

import (
	"context"
	"time"
)

// MockProvider simulates an LLM provider. Used in tests across packages.
type MockProvider struct {
	MockName string
	MockCost float64
	Delay    time.Duration // delay before each chunk
	Chunks   []string
	FailWith error // if set, sends error chunk immediately
}

func (m *MockProvider) Name() string             { return m.MockName }
func (m *MockProvider) CostPer1kTokens() float64 { return m.MockCost }

func (m *MockProvider) Stream(ctx context.Context, req Request, out chan<- Chunk) error {
	defer close(out)
	if m.FailWith != nil {
		select {
		case <-ctx.Done():
		case out <- Chunk{Provider: m.MockName, Err: m.FailWith}:
		}
		return m.FailWith
	}
	for _, c := range m.Chunks {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(m.Delay):
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- Chunk{Content: c, Provider: m.MockName}:
		}
	}
	select {
	case <-ctx.Done():
	case out <- Chunk{Provider: m.MockName, Done: true}:
	}
	return nil
}

// compile-time interface check
var _ Provider = (*MockProvider)(nil)
