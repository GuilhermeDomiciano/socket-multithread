package guardrail

// Chain applies guards in order, threading the (possibly masked) text and
// accumulating findings. It stops at the first guard that blocks.
type Chain []Guard

func (c Chain) Inspect(text string) Result {
	out := Result{Text: text}
	for _, g := range c {
		r := g.Inspect(out.Text)
		out.Text = r.Text
		out.Findings = append(out.Findings, r.Findings...)
		if r.Blocked {
			out.Blocked = true
			out.Reason = r.Reason
			return out
		}
	}
	return out
}
