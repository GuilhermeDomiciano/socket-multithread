package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/domiciano/llm-proxy/provider"
	"github.com/domiciano/llm-proxy/router"
	"github.com/domiciano/llm-proxy/server"
)

func TestVizStream_emits_events(t *testing.T) {
	r := &router.Router{
		Providers: []provider.Provider{&provider.MockProvider{MockName: "mock", Chunks: []string{"hi"}}},
		Strategy:  router.StrategyFastest,
	}
	mux := server.New(r, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/viz/stream?q=hello&strategy=fastest", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"type":"provider_start"`) {
		t.Errorf("missing provider_start in: %s", body)
	}
	if !strings.Contains(body, `"type":"won"`) {
		t.Errorf("missing won in: %s", body)
	}
	if !strings.Contains(body, "data: [DONE]") {
		t.Errorf("missing [DONE] in: %s", body)
	}
}

func TestVizStream_400_without_q(t *testing.T) {
	mux := server.New(newTestRouter([]string{"x"}), nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/viz/stream", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestVizStream_400_invalid_strategy(t *testing.T) {
	mux := server.New(newTestRouter([]string{"x"}), nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/viz/stream?q=hi&strategy=bogus", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestDashboard_served_at_root(t *testing.T) {
	mux := server.New(newTestRouter([]string{"x"}), nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Corrida Paralela") {
		t.Errorf("expected dashboard HTML at /, got status %d body-len %d", w.Code, w.Body.Len())
	}
}

func TestSabotage_404_unknown_provider(t *testing.T) {
	mux := server.New(newTestRouter([]string{"x"}), map[string]*provider.Sabotage{}, nil)
	body := `{"provider":"nope","mode":"fail"}`
	req := httptest.NewRequest(http.MethodPost, "/viz/sabotage", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSabotage_400_invalid_mode(t *testing.T) {
	sab := provider.NewSabotage(&provider.MockProvider{MockName: "openai", Chunks: []string{"x"}})
	mux := server.New(newTestRouter([]string{"x"}), map[string]*provider.Sabotage{"openai": sab}, nil)
	body := `{"provider":"openai","mode":"explode"}`
	req := httptest.NewRequest(http.MethodPost, "/viz/sabotage", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSabotage_200_sets_fail(t *testing.T) {
	sab := provider.NewSabotage(&provider.MockProvider{MockName: "openai", Chunks: []string{"x"}})
	mux := server.New(newTestRouter([]string{"x"}), map[string]*provider.Sabotage{"openai": sab}, nil)
	body := `{"provider":"openai","mode":"fail"}`
	req := httptest.NewRequest(http.MethodPost, "/viz/sabotage", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
