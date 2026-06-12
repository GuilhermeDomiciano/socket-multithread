package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Gemini struct {
	APIKey  string
	Model   string
	BaseURL string
}

// geminiPrices is USD per 1k INPUT tokens for the "cheapest" strategy. Rates
// current as of 2026-06 (see ai.google.dev/gemini-api/docs/pricing). Flash and
// Flash-Lite still have a free tier; Pro is paid-only. Unknown models fall back
// to the gemini-2.5-flash rate. (Gemini 2.0 was shut down 2026-06-01.)
var geminiPrices = map[string]float64{
	"gemini-3.5-flash":       0.0015,
	"gemini-3.1-pro-preview": 0.002,
	"gemini-3-flash-preview": 0.0005,
	"gemini-3.1-flash-lite":  0.00025,
	"gemini-2.5-pro":         0.00125,
	"gemini-2.5-flash":       0.0003,
	"gemini-2.5-flash-lite":  0.0001,
}

// NewGemini builds a Gemini racer for a specific model. An empty model defaults
// to gemini-2.5-flash (fast, free-tier eligible).
func NewGemini(apiKey, model string) *Gemini {
	if model == "" {
		model = "gemini-2.5-flash"
	}
	return &Gemini{APIKey: apiKey, Model: model, BaseURL: "https://generativelanguage.googleapis.com"}
}

func (g *Gemini) Name() string { return "gemini:" + g.Model }
func (g *Gemini) CostPer1kTokens() float64 {
	if c, ok := geminiPrices[g.Model]; ok {
		return c
	}
	return 0.0003
}

type geminiReq struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
	Role  string       `json:"role"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiResp struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
}

func (g *Gemini) Stream(ctx context.Context, req Request, out chan<- Chunk) error {
	defer close(out)

	contents := make([]geminiContent, len(req.Messages))
	for i, m := range req.Messages {
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		contents[i] = geminiContent{Parts: []geminiPart{{Text: m.Content}}, Role: role}
	}
	body, err := json.Marshal(geminiReq{Contents: contents})
	if err != nil {
		out <- Chunk{Provider: g.Name(), Err: fmt.Errorf("marshal request: %w", err)}
		return err
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse&key=%s", g.BaseURL, g.Model, g.APIKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		out <- Chunk{Provider: g.Name(), Err: err}
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		out <- Chunk{Provider: g.Name(), Err: err}
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Surface Google's actual error message (quota, API not enabled,
		// invalid key, model not found...) — otherwise "status 429" is opaque.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		detail := strings.TrimSpace(string(snippet))
		err := fmt.Errorf("gemini: status %d: %s", resp.StatusCode, detail)
		out <- Chunk{Provider: g.Name(), Err: err}
		return err
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var gr geminiResp
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &gr); err != nil {
			continue
		}
		for _, cand := range gr.Candidates {
			for _, part := range cand.Content.Parts {
				if part.Text != "" {
					out <- Chunk{Content: part.Text, Provider: g.Name()}
				}
			}
			if cand.FinishReason == "STOP" {
				out <- Chunk{Provider: g.Name(), Done: true}
				return nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		out <- Chunk{Provider: g.Name(), Err: err}
		return err
	}
	out <- Chunk{Provider: g.Name(), Done: true}
	return nil
}
