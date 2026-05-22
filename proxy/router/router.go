package router

import (
	"context"
	"fmt"

	"github.com/domiciano/llm-proxy/provider"
)

type Strategy string

const (
	StrategyFastest  Strategy = "fastest"
	StrategyCheapest Strategy = "cheapest"
	StrategyFallback Strategy = "fallback"
)

type Router struct {
	Providers []provider.Provider
	Strategy  Strategy
}

func (r *Router) Dispatch(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	if len(r.Providers) == 0 {
		return nil, fmt.Errorf("no providers configured")
	}
	switch r.Strategy {
	case StrategyFastest, "":
		return Fastest(ctx, r.Providers, req), nil
	case StrategyCheapest:
		return Cheapest(ctx, r.Providers, req), nil
	case StrategyFallback:
		return Fallback(ctx, r.Providers, req), nil
	default:
		return nil, fmt.Errorf("unknown strategy: %q", r.Strategy)
	}
}
