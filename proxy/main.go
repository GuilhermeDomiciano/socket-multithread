package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/domiciano/llm-proxy/guardrail"
	"github.com/domiciano/llm-proxy/pipeline"
	"github.com/domiciano/llm-proxy/provider"
	"github.com/domiciano/llm-proxy/router"
	"github.com/domiciano/llm-proxy/server"
)

func main() {
	rawProviders := buildProviders()
	if len(rawProviders) == 0 {
		log.Fatal("no providers configured — set at least one of: OPENAI_API_KEY, ANTHROPIC_API_KEY, GEMINI_API_KEY")
	}

	// Wrap each real provider in a Sabotage decorator so the live dashboard can
	// inject failures/latency at runtime. Inert by default (zero overhead).
	sabotage := make(map[string]*provider.Sabotage, len(rawProviders))
	providers := make([]provider.Provider, len(rawProviders))
	for i, p := range rawProviders {
		s := provider.NewSabotage(p)
		sabotage[p.Name()] = s
		providers[i] = s
	}

	strategy := router.Strategy(getEnv("PROXY_STRATEGY", "fastest"))
	r := &router.Router{Providers: providers, Strategy: strategy}

	port := getEnv("PROXY_PORT", "8080")
	timeoutMs, _ := strconv.Atoi(getEnv("PROXY_TIMEOUT_MS", "5000"))

	// Bound each provider call so a stuck/rate-limited backend (e.g. a Gemini
	// free-tier 429) fails fast instead of freezing a race or the benchmark.
	// Set once here, before serving, so the router reads it race-free.
	router.CallTimeout = time.Duration(timeoutMs) * time.Millisecond

	gateway := &pipeline.Gateway{
		Input:  guardrail.Chain{guardrail.NewInjectionGuard(), guardrail.NewPIIGuard()},
		Output: guardrail.NewPIIGuard(),
		Router: r,
	}

	srv := &http.Server{
		Addr:        ":" + port,
		Handler:     server.New(r, sabotage, gateway),
		ReadTimeout: time.Duration(timeoutMs) * time.Millisecond,
		// WriteTimeout is 0 (disabled) to allow long-running SSE streams.
	}

	log.Printf("llm-proxy starting on :%s  strategy=%s  providers=%d", port, strategy, len(providers))
	log.Fatal(srv.ListenAndServe())
}

// buildProviders turns the configured API keys + per-provider model lists into
// one racer per (provider, model). Each provider's models come from
// <PROVIDER>_MODELS (comma-separated); an empty list falls back to a sane default.
// PROXY_FALLBACK_ORDER still controls the provider group order. Duplicate racers
// (same Name()) are dropped so the sabotage map can't collide.
func buildProviders() []provider.Provider {
	var providers []provider.Provider
	seen := map[string]bool{}
	order := strings.Split(getEnv("PROXY_FALLBACK_ORDER", "openai,anthropic,gemini"), ",")
	for _, name := range order {
		switch strings.TrimSpace(name) {
		case "openai":
			if key := os.Getenv("OPENAI_API_KEY"); key != "" {
				for _, m := range parseModels(getEnv("OPENAI_MODELS", "gpt-4o")) {
					providers = addRacer(providers, seen, provider.NewOpenAI(key, m))
				}
			}
		case "anthropic":
			if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
				for _, m := range parseModels(getEnv("ANTHROPIC_MODELS", "claude-3-5-sonnet-20241022")) {
					providers = addRacer(providers, seen, provider.NewAnthropic(key, m))
				}
			}
		case "gemini":
			if key := os.Getenv("GEMINI_API_KEY"); key != "" {
				// GEMINI_MODEL (singular, legacy) seeds the default when GEMINI_MODELS is unset.
				def := getEnv("GEMINI_MODEL", "gemini-2.5-flash")
				for _, m := range parseModels(getEnv("GEMINI_MODELS", def)) {
					providers = addRacer(providers, seen, provider.NewGemini(key, m))
				}
			}
		}
	}
	return providers
}

// addRacer appends p unless a racer with the same Name() was already added.
func addRacer(providers []provider.Provider, seen map[string]bool, p provider.Provider) []provider.Provider {
	if seen[p.Name()] {
		return providers
	}
	seen[p.Name()] = true
	fmt.Println("provider loaded:", p.Name())
	return append(providers, p)
}

// parseModels splits a comma-separated model list, dropping blanks.
func parseModels(s string) []string {
	var out []string
	for _, m := range strings.Split(s, ",") {
		if m = strings.TrimSpace(m); m != "" {
			out = append(out, m)
		}
	}
	return out
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
