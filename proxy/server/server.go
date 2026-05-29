package server

import (
	"net/http"

	"github.com/domiciano/llm-proxy/provider"
	"github.com/domiciano/llm-proxy/router"
)

type Server struct {
	Router   *router.Router
	Sabotage map[string]*provider.Sabotage
}

// New builds the HTTP mux. sabotage may be nil (the /viz/sabotage endpoint will
// then report 404 for any provider); production endpoints are unaffected.
func New(r *router.Router, sabotage map[string]*provider.Sabotage) *http.ServeMux {
	s := &Server{Router: r, Sabotage: sabotage}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /query", s.handleQuery)
	mux.HandleFunc("POST /v1/chat/completions", s.handleOpenAICompat)
	mux.HandleFunc("GET /viz/stream", s.handleVizStream)
	mux.HandleFunc("POST /viz/sabotage", s.handleSabotage)
	mux.Handle("GET /", staticHandler())
	return mux
}
