package intent_test

import (
	"strings"
	"testing"

	"github.com/domiciano/llm-proxy/intent"
	"github.com/domiciano/llm-proxy/router"
)

func TestClassify_complex_keyword_goes_fastest(t *testing.T) {
	it := intent.Classify("Explique em detalhes como funciona o GC do Go")
	if it.Class != "complex" || it.Strategy != router.StrategyFastest {
		t.Errorf("expected complex/fastest, got %+v", it)
	}
	if it.Reason == "" {
		t.Error("expected a non-empty reason")
	}
}

func TestClassify_simple_keyword_goes_cheapest(t *testing.T) {
	it := intent.Classify("traduza 'cat' para o português")
	if it.Class != "simple" || it.Strategy != router.StrategyCheapest {
		t.Errorf("expected simple/cheapest, got %+v", it)
	}
}

func TestClassify_long_prompt_without_keyword_is_complex(t *testing.T) {
	long := strings.Repeat("palavra ", 40) // > 200 chars, no keyword
	it := intent.Classify(long)
	if it.Strategy != router.StrategyFastest {
		t.Errorf("expected long prompt to be fastest, got %+v", it)
	}
}

func TestClassify_short_unknown_is_cheapest(t *testing.T) {
	it := intent.Classify("blarg flemp")
	if it.Strategy != router.StrategyCheapest {
		t.Errorf("expected short unknown to be cheapest, got %+v", it)
	}
}
