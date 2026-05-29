package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/domiciano/llm-proxy/pipeline"
	"github.com/domiciano/llm-proxy/provider"
	"github.com/domiciano/llm-proxy/router"
	"github.com/domiciano/llm-proxy/stream"
)

type queryReq struct {
	Messages    []provider.Message `json:"messages"`
	Strategy    router.Strategy    `json:"strategy,omitempty"`
	MaxTokens   int                `json:"max_tokens,omitempty"`
	Temperature float64            `json:"temperature,omitempty"`
}

// lastUserContent returns the content of the last message with role "user"
// (falling back to the last message). The gateway operates on this prompt.
func lastUserContent(msgs []provider.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return msgs[i].Content
		}
	}
	if len(msgs) > 0 {
		return msgs[len(msgs)-1].Content
	}
	return ""
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	var req queryReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Messages) == 0 {
		http.Error(w, "messages required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	chunks, err := s.Gateway.Process(ctx, lastUserContent(req.Messages), req.Strategy, nil)
	if errors.Is(err, pipeline.ErrBlocked) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
			"error":  "blocked by guardrail",
			"reason": err.Error(),
		})
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	stream.Write(w, r, chunks)
}
