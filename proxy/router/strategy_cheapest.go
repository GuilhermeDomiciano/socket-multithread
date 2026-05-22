package router

import (
	"context"
	"sort"

	"github.com/domiciano/llm-proxy/provider"
)

func estimateCost(p provider.Provider, req provider.Request) float64 {
	tokens := 0
	for _, m := range req.Messages {
		tokens += len(m.Content) / 4
	}
	return float64(tokens) * p.CostPer1kTokens() / 1000.0
}

// Cheapest sorts providers by estimated token cost and delegates to Fallback,
// so if the cheapest provider fails it tries the next cheapest automatically.
func Cheapest(ctx context.Context, providers []provider.Provider, req provider.Request) <-chan provider.Chunk {
	ranked := make([]provider.Provider, len(providers))
	copy(ranked, providers)
	sort.Slice(ranked, func(i, j int) bool {
		return estimateCost(ranked[i], req) < estimateCost(ranked[j], req)
	})
	return Fallback(ctx, ranked, req)
}
