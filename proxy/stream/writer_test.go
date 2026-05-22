package stream_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/domiciano/llm-proxy/provider"
	"github.com/domiciano/llm-proxy/stream"
)

func TestWrite_formats_content_as_SSE(t *testing.T) {
	chunks := make(chan provider.Chunk, 4)
	chunks <- provider.Chunk{Content: "Hello", Provider: "openai"}
	chunks <- provider.Chunk{Content: " world", Provider: "openai"}
	chunks <- provider.Chunk{Provider: "openai", Done: true}
	close(chunks)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/query", nil)
	stream.Write(w, r, chunks)

	body := w.Body.String()
	if !strings.Contains(body, `"content":"Hello"`) {
		t.Errorf("missing first chunk in: %s", body)
	}
	if !strings.Contains(body, `"content":" world"`) {
		t.Errorf("missing second chunk in: %s", body)
	}
	if !strings.Contains(body, "data: [DONE]") {
		t.Errorf("missing [DONE] in: %s", body)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("wrong Content-Type: %s", ct)
	}
}

func TestWrite_sends_error_event(t *testing.T) {
	chunks := make(chan provider.Chunk, 2)
	chunks <- provider.Chunk{Err: fmt.Errorf("provider down")}
	close(chunks)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/query", nil)
	stream.Write(w, r, chunks)

	body := w.Body.String()
	if !strings.Contains(body, `"error":"provider down"`) {
		t.Errorf("missing error event in: %s", body)
	}
}
