package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type Gemini struct {
	APIKey  string
	Model   string
	BaseURL string
}

func NewGemini(apiKey string) *Gemini {
	// gemini-2.0-flash has no free-tier quota on some projects (limit: 0).
	// Default to gemini-2.5-flash, overridable via GEMINI_MODEL.
	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = "gemini-2.5-flash"
	}
	return &Gemini{APIKey: apiKey, Model: model, BaseURL: "https://generativelanguage.googleapis.com"}
}

func (g *Gemini) Name() string             { return "gemini" }
func (g *Gemini) CostPer1kTokens() float64 { return 0.00015 }

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
