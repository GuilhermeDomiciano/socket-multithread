package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/domiciano/llm-proxy/event"
	"github.com/domiciano/llm-proxy/provider"
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
	case "", router.StrategyFastest, router.StrategyCheapest, router.StrategyFallback:
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

	rtr := &router.Router{Providers: s.Router.Providers, Strategy: strategy}
	sink := event.NewChanSink(64, time.Now(), r.Context().Done())

	go func() {
		chunks, err := rtr.Dispatch(r.Context(), provider.Request{
			Messages: []provider.Message{{Role: "user", Content: q}},
		}, sink)
		if err != nil {
			sink.Emit(event.Event{Type: "error", Detail: err.Error()})
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
