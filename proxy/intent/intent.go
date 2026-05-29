// Package intent classifies a prompt and picks a routing strategy by heuristic.
package intent

import (
	"fmt"
	"regexp"

	"github.com/domiciano/llm-proxy/router"
)

type Intent struct {
	Class    string          // "simple" | "complex"
	Strategy router.Strategy // "cheapest" | "fastest"
	Reason   string
}

var complexRe = regexp.MustCompile(`(?i)\b(explique|detalhe|analise|compare|passo a passo|c[óo]digo|implemente|disserte|hist[óo]ria|profundidade)\b`)
var simpleRe = regexp.MustCompile(`(?i)\b(oi|ol[áa]|traduza|resuma|defina|qual|quanto|bom dia|boa tarde|boa noite)\b`)

// Classify inspects the prompt: depth keywords → complex/fastest; transactional
// keywords → simple/cheapest; otherwise length (>200 chars) decides.
func Classify(prompt string) Intent {
	if m := complexRe.FindString(prompt); m != "" {
		return Intent{Class: "complex", Strategy: router.StrategyFastest,
			Reason: fmt.Sprintf("complex: keyword %q → fastest", m)}
	}
	if m := simpleRe.FindString(prompt); m != "" {
		return Intent{Class: "simple", Strategy: router.StrategyCheapest,
			Reason: fmt.Sprintf("simple: keyword %q → cheapest", m)}
	}
	if len(prompt) > 200 {
		return Intent{Class: "complex", Strategy: router.StrategyFastest,
			Reason: fmt.Sprintf("complex: length %d > 200 → fastest", len(prompt))}
	}
	return Intent{Class: "simple", Strategy: router.StrategyCheapest,
		Reason: "simple: short prompt → cheapest"}
}
