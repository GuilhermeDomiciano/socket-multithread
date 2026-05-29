package guardrail

import (
	"fmt"
	"regexp"
)

type piiPattern struct {
	typ   string
	re    *regexp.Regexp
	label string
}

// Order matters: longer/more specific patterns first so a looser pattern
// (phone) does not consume part of a card number.
var piiPatterns = []piiPattern{
	{"credit_card", regexp.MustCompile(`\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}`), "CARD"},
	{"cpf", regexp.MustCompile(`\d{3}\.?\d{3}\.?\d{3}-?\d{2}`), "CPF"},
	{"phone", regexp.MustCompile(`(?:\+?55\s?)?\(?\d{2}\)?\s?9?\d{4}-?\d{4}`), "PHONE"},
	{"email", regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.-]+`), "EMAIL"},
}

// ScrubPII masks every PII occurrence with [REDACTED_<LABEL>_<n>] and returns
// the masked text plus one Finding per type that matched.
func ScrubPII(text string) (string, []Finding) {
	var findings []Finding
	for _, p := range piiPatterns {
		count := 0
		first := ""
		text = p.re.ReplaceAllStringFunc(text, func(string) string {
			ph := fmt.Sprintf("[REDACTED_%s_%d]", p.label, count)
			if count == 0 {
				first = ph
			}
			count++
			return ph
		})
		if count > 0 {
			findings = append(findings, Finding{Type: p.typ, Placeholder: first, Count: count})
		}
	}
	return text, findings
}

// PIIGuard masks PII; it never blocks.
type PIIGuard struct{}

func NewPIIGuard() *PIIGuard { return &PIIGuard{} }

func (g *PIIGuard) Inspect(text string) Result {
	masked, findings := ScrubPII(text)
	return Result{Text: masked, Findings: findings}
}
