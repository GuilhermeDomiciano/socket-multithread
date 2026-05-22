package stream

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/domiciano/llm-proxy/provider"
)

type payload struct {
	Content  string `json:"content,omitempty"`
	Provider string `json:"provider,omitempty"`
	Error    string `json:"error,omitempty"`
}

// Write consumes chunks from the channel and writes SSE events to w.
// Returns when the channel is closed, a Done/Err chunk is received, or the client disconnects.
func Write(w http.ResponseWriter, r *http.Request, chunks <-chan provider.Chunk) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
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
				data, _ := json.Marshal(payload{Error: chunk.Err.Error()}) // payload contains only strings; cannot fail
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
				return
			}
			if chunk.Done {
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}
			data, _ := json.Marshal(payload{Content: chunk.Content, Provider: chunk.Provider}) // payload contains only strings; cannot fail
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
