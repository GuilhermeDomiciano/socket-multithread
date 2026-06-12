package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/domiciano/llm-proxy/event"
	"github.com/domiciano/llm-proxy/pipeline"
	"github.com/domiciano/llm-proxy/router"
)

// handleVizStream runs a dispatch with a telemetry sink and streams the
// resulting events to the browser as SSE. The chunk channel content is drained
// (and discarded) because the events already carry per-provider content.
func (s *Server) handleVizStream(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		http.Error(w, "query param q required", http.StatusBadRequest)
		return
	}
	strategy := router.Strategy(r.URL.Query().Get("strategy"))
	switch strategy {
	case "", "auto", router.StrategyFastest, router.StrategyCheapest, router.StrategyFallback, "benchmark":
	default:
		http.Error(w, "invalid strategy", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Connection", "keep-alive")

	sink := event.NewChanSink(64, time.Now(), r.Context().Done())

	go func() {
		if strategy == "benchmark" {
			if err := s.Gateway.Benchmark(r.Context(), q, sink); err != nil {
				if !errors.Is(err, pipeline.ErrBlocked) {
					sink.Emit(event.Event{Type: "error", Detail: err.Error()})
				}
				// Em ErrBlocked o evento "blocked" já foi emitido por Benchmark.
			}
			sink.Close()
			return
		}
		chunks, err := s.Gateway.Process(r.Context(), q, strategy, sink)
		if err != nil {
			if !errors.Is(err, pipeline.ErrBlocked) {
				sink.Emit(event.Event{Type: "error", Detail: err.Error()})
			}
			// On ErrBlocked the "blocked" event was already emitted by Process.
			sink.Close()
			return
		}
		for range chunks { // drain; content already emitted as events
		}
		sink.Close()
	}()

	for e := range sink.Events() {
		data, _ := json.Marshal(e) // Event fields are plain scalars; cannot fail
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

type sabotageReq struct {
	Provider string `json:"provider"`
	Mode     string `json:"mode"` // "fail" | "delay" | "clear"
	DelayMs  int    `json:"delay_ms,omitempty"`
}

func (s *Server) handleSabotage(w http.ResponseWriter, r *http.Request) {
	var req sabotageReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	sab, ok := s.Sabotage[req.Provider]
	if !ok {
		http.Error(w, "unknown provider", http.StatusNotFound)
		return
	}
	switch req.Mode {
	case "fail":
		sab.SetFail(true)
	case "delay":
		sab.SetDelay(time.Duration(req.DelayMs) * time.Millisecond)
	case "clear":
		sab.Clear()
	default:
		http.Error(w, "invalid mode", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
		"status":   "ok",
		"provider": req.Provider,
		"mode":     req.Mode,
	})
}
