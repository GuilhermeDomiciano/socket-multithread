package server

import (
	"net/http"

	"github.com/domiciano/llm-proxy/router"
)

type Server struct {
	Router *router.Router
}

func New(r *router.Router) *http.ServeMux {
	s := &Server{Router: r}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /query", s.handleQuery)
	mux.HandleFunc("POST /v1/chat/completions", s.handleOpenAICompat)
	return mux
}
