package main

import "testing"

func TestParseModels_splits_and_trims(t *testing.T) {
	got := parseModels("gpt-4o, gpt-4o-mini ,, ")
	if len(got) != 2 || got[0] != "gpt-4o" || got[1] != "gpt-4o-mini" {
		t.Fatalf("expected [gpt-4o gpt-4o-mini], got %v", got)
	}
}

func TestParseModels_empty_yields_none(t *testing.T) {
	if got := parseModels("  ,  ,"); len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}
