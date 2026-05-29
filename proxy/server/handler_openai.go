package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/domiciano/llm-proxy/provider"
)

type oaiCompatReq struct {
	Messages  []oaiCompatMsg `json:"messages"`
	Stream    bool           `json:"stream"`
	MaxTokens int            `json:"max_tokens,omitempty"`
}

type oaiCompatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (s *Server) handleOpenAICompat(w http.ResponseWriter, r *http.Request) {
	var req oaiCompatReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Messages) == 0 {
		http.Error(w, "messages required", http.StatusBadRequest)
		return
	}

	messages := make([]provider.Message, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = provider.Message{Role: m.Role, Content: m.Content}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	chunks, err := s.Router.Dispatch(ctx, provider.Request{Messages: messages, MaxTokens: req.MaxTokens}, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if req.Stream {
		writeOAIStream(w, r, chunks)
	} else {
		writeOAIComplete(w, chunks)
	}
}

func writeOAIStream(w http.ResponseWriter, r *http.Request, chunks <-chan provider.Chunk) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for {
		select {
		case <-r.Context().Done():
			return
		case chunk, ok := <-chunks:
			if !ok {
				return
			}
			if chunk.Err != nil {
				data, _ := json.Marshal(map[string]string{"error": chunk.Err.Error()}) // payload contains only strings; cannot fail
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
				return
			}
			if chunk.Done {
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}
			event := map[string]interface{}{
				"model": chunk.Provider,
				"choices": []map[string]interface{}{
					{"delta": map[string]string{"content": chunk.Content}},
				},
			}
			data, _ := json.Marshal(event) // payload contains only strings; cannot fail
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func writeOAIComplete(w http.ResponseWriter, chunks <-chan provider.Chunk) {
	var content, model string
	var providerErr error
	for chunk := range chunks {
		if chunk.Err != nil {
			providerErr = chunk.Err
			break
		}
		if !chunk.Done {
			content += chunk.Content
			if model == "" {
				model = chunk.Provider
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if providerErr != nil {
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"error": providerErr.Error()}) //nolint:errcheck
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
		"model": model,
		"choices": []map[string]interface{}{
			{
				"message":       map[string]string{"role": "assistant", "content": content},
				"finish_reason": "stop",
			},
		},
	})
}
