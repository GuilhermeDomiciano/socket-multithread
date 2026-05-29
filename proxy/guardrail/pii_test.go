package guardrail_test

import (
	"strings"
	"testing"

	"github.com/domiciano/llm-proxy/guardrail"
)

func TestScrubPII_masks_email_and_cpf(t *testing.T) {
	masked, finds := guardrail.ScrubPII("fale com joao@x.com cpf 123.456.789-00")
	if strings.Contains(masked, "joao@x.com") {
		t.Errorf("email not masked: %q", masked)
	}
	if !strings.Contains(masked, "[REDACTED_EMAIL_0]") {
		t.Errorf("missing email placeholder: %q", masked)
	}
	if !strings.Contains(masked, "[REDACTED_CPF_0]") {
		t.Errorf("missing cpf placeholder: %q", masked)
	}
	types := map[string]bool{}
	for _, f := range finds {
		types[f.Type] = true
	}
	if !types["email"] || !types["cpf"] {
		t.Errorf("expected email+cpf findings, got %v", finds)
	}
}

func TestScrubPII_masks_phone_and_card(t *testing.T) {
	masked, _ := guardrail.ScrubPII("cartao 1234 5678 9012 3456 tel (11) 98765-4321")
	if !strings.Contains(masked, "[REDACTED_CARD_0]") {
		t.Errorf("card not masked: %q", masked)
	}
	if !strings.Contains(masked, "[REDACTED_PHONE_0]") {
		t.Errorf("phone not masked: %q", masked)
	}
}

func TestScrubPII_numbers_multiple_occurrences(t *testing.T) {
	masked, _ := guardrail.ScrubPII("a@b.com e c@d.com")
	if !strings.Contains(masked, "[REDACTED_EMAIL_0]") || !strings.Contains(masked, "[REDACTED_EMAIL_1]") {
		t.Errorf("expected EMAIL_0 and EMAIL_1, got %q", masked)
	}
}

func TestScrubPII_clean_text_unchanged(t *testing.T) {
	masked, finds := guardrail.ScrubPII("olá, tudo bem?")
	if masked != "olá, tudo bem?" {
		t.Errorf("clean text changed: %q", masked)
	}
	if len(finds) != 0 {
		t.Errorf("expected no findings, got %v", finds)
	}
}

func TestPIIGuard_masks_and_never_blocks(t *testing.T) {
	r := guardrail.NewPIIGuard().Inspect("email a@b.com")
	if r.Blocked {
		t.Error("PIIGuard must never block")
	}
	if !strings.Contains(r.Text, "[REDACTED_EMAIL_0]") {
		t.Errorf("not masked: %q", r.Text)
	}
}
