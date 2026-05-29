package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/domiciano/llm-proxy/provider"
	"github.com/domiciano/llm-proxy/router"
	"github.com/domiciano/llm-proxy/server"
)

func main() {
	providers := buildProviders()
	if len(providers) == 0 {
		log.Fatal("no providers configured — set at least one of: OPENAI_API_KEY, ANTHROPIC_API_KEY, GEMINI_API_KEY")
	}

	strategy := router.Strategy(getEnv("PROXY_STRATEGY", "fastest"))
	r := &router.Router{Providers: providers, Strategy: strategy}

	port := getEnv("PROXY_PORT", "8080")
	timeoutMs, _ := strconv.Atoi(getEnv("PROXY_TIMEOUT_MS", "5000"))

	srv := &http.Server{
		Addr:        ":" + port,
		Handler:     server.New(r, nil),
		ReadTimeout: time.Duration(timeoutMs) * time.Millisecond,
		// WriteTimeout is 0 (disabled) to allow long-running SSE streams.
	}

	log.Printf("llm-proxy starting on :%s  strategy=%s  providers=%d", port, strategy, len(providers))
	log.Fatal(srv.ListenAndServe())
}

func buildProviders() []provider.Provider {
	var providers []provider.Provider
	order := strings.Split(getEnv("PROXY_FALLBACK_ORDER", "openai,anthropic,gemini"), ",")
	for _, name := range order {
		switch strings.TrimSpace(name) {
		case "openai":
			if key := os.Getenv("OPENAI_API_KEY"); key != "" {
				providers = append(providers, provider.NewOpenAI(key))
				fmt.Println("provider loaded: openai")
			}
		case "anthropic":
			if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
				providers = append(providers, provider.NewAnthropic(key))
				fmt.Println("provider loaded: anthropic")
			}
		case "gemini":
			if key := os.Getenv("GEMINI_API_KEY"); key != "" {
				providers = append(providers, provider.NewGemini(key))
				fmt.Println("provider loaded: gemini")
			}
		}
	}
	return providers
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
