package server

import (
	"net/http"

	"github.com/domiciano/llm-proxy/guardrail"
	"github.com/domiciano/llm-proxy/pipeline"
	"github.com/domiciano/llm-proxy/provider"
	"github.com/domiciano/llm-proxy/router"
)

type Server struct {
	Router   *router.Router
	Sabotage map[string]*provider.Sabotage
	Gateway  *pipeline.Gateway
}

// defaultGateway builds the standard smart-gateway pipeline for a router.
func defaultGateway(r *router.Router) *pipeline.Gateway {
	return &pipeline.Gateway{
		Input:  guardrail.Chain{guardrail.NewInjectionGuard(), guardrail.NewPIIGuard()},
		Output: guardrail.NewPIIGuard(),
		Router: r,
	}
}

// New builds the HTTP mux. sabotage may be nil. gateway may be nil (a default
// pipeline is then constructed for r). Production endpoints are unaffected.
func New(r *router.Router, sabotage map[string]*provider.Sabotage, gateway *pipeline.Gateway) *http.ServeMux {
	if gateway == nil {
		gateway = defaultGateway(r)
	}
	s := &Server{Router: r, Sabotage: sabotage, Gateway: gateway}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /query", s.handleQuery)
	mux.HandleFunc("POST /v1/chat/completions", s.handleOpenAICompat)
	mux.HandleFunc("GET /viz/stream", s.handleVizStream)
	mux.HandleFunc("POST /viz/sabotage", s.handleSabotage)
	mux.Handle("GET /", staticHandler())
	return mux
}
