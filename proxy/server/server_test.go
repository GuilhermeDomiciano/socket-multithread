package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/domiciano/llm-proxy/provider"
	"github.com/domiciano/llm-proxy/router"
	"github.com/domiciano/llm-proxy/server"
)

func newTestRouter(chunks []string) *router.Router {
	return &router.Router{
		Providers: []provider.Provider{
			&provider.MockProvider{MockName: "mock", Chunks: chunks},
		},
		Strategy: router.StrategyFastest,
	}
}

func TestHandleQuery_returns_SSE_stream(t *testing.T) {
	mux := server.New(newTestRouter([]string{"hello", " world"}), nil)
	body := `{"messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/query", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", ct)
	}
	resp := w.Body.String()
	if !strings.Contains(resp, `"content":"hello"`) {
		t.Errorf("missing first chunk in: %s", resp)
	}
	if !strings.Contains(resp, "data: [DONE]") {
		t.Errorf("missing [DONE] in: %s", resp)
	}
}

func TestHandleQuery_returns_400_on_empty_messages(t *testing.T) {
	mux := server.New(newTestRouter([]string{}), nil)
	body := `{"messages":[]}`
	req := httptest.NewRequest(http.MethodPost, "/query", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleOpenAICompat_streaming(t *testing.T) {
	mux := server.New(newTestRouter([]string{"Hi"}), nil)
	body := `{"messages":[{"role":"user","content":"hello"}],"stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", ct)
	}
	resp := w.Body.String()
	if !strings.Contains(resp, `"content":"Hi"`) {
		t.Errorf("missing content in: %s", resp)
	}
}

func TestHandleOpenAICompat_non_streaming(t *testing.T) {
	mux := server.New(newTestRouter([]string{"Hello"}), nil)
	body := `{"messages":[{"role":"user","content":"hi"}],"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("could not decode response: %v", err)
	}
	choices, _ := result["choices"].([]interface{})
	if len(choices) == 0 {
		t.Fatal("expected choices in response")
	}
}
