package provider

import "context"

type Message struct {
	Role    string
	Content string
}

type Request struct {
	Messages    []Message
	MaxTokens   int
	Temperature float64
}

type Chunk struct {
	Content  string
	Provider string
	Err      error
	Done     bool
}

// Provider is implemented by every LLM backend.
// Stream writes chunks to out and closes out when done (success, error, or ctx cancel).
type Provider interface {
	Name() string
	CostPer1kTokens() float64
	Stream(ctx context.Context, req Request, out chan<- Chunk) error
}
