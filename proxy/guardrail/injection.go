package guardrail

import "regexp"

var injectionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)ignore (as )?instruç(ões|oes) anteriores`),
	regexp.MustCompile(`(?i)ignore (all )?previous instructions`),
	regexp.MustCompile(`(?i)esqueça (as )?instruções`),
	regexp.MustCompile(`(?i)disregard (the )?(above|previous)`),
	regexp.MustCompile(`(?i)reveal your system prompt`),
	regexp.MustCompile(`(?i)mostre (o )?seu (system )?prompt`),
	regexp.MustCompile(`(?i)you are now`),
	regexp.MustCompile(`(?i)aja como`),
	regexp.MustCompile(`(?i)jailbreak`),
	regexp.MustCompile(`(?i)\bDAN\b`),
}

// InjectionGuard blocks prompts matching known injection patterns. It never
// alters the text.
type InjectionGuard struct{}

func NewInjectionGuard() *InjectionGuard { return &InjectionGuard{} }

func (g *InjectionGuard) Inspect(text string) Result {
	for _, re := range injectionPatterns {
		if re.MatchString(text) {
			return Result{
				Text:     text,
				Blocked:  true,
				Reason:   "prompt injection detected: " + re.String(),
				Findings: []Finding{{Type: "injection"}},
			}
		}
	}
	return Result{Text: text}
}
