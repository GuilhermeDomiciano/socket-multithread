// Package guardrail inspects text for PII (masked) and prompt injection (blocked).
package guardrail

// Finding describes one detected item. For PII, Placeholder holds the first
// replacement token and Count the number of occurrences. For injection,
// Type is "injection" and Placeholder is empty.
type Finding struct {
	Type        string
	Placeholder string
	Count       int
}

// Result is the outcome of inspecting text.
type Result struct {
	Text     string // possibly masked
	Findings []Finding
	Blocked  bool
	Reason   string
}

// Guard inspects text and returns a Result.
type Guard interface {
	Inspect(text string) Result
}
