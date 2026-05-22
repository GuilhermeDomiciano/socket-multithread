package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type Anthropic struct {
	APIKey  string
	Model   string
	BaseURL string
}

func NewAnthropic(apiKey string) *Anthropic {
	return &Anthropic{APIKey: apiKey, Model: "claude-3-5-sonnet-20241022", BaseURL: "https://api.anthropic.com"}
}

func (a *Anthropic) Name() string             { return "anthropic" }
func (a *Anthropic) CostPer1kTokens() float64 { return 0.003 }

type anthroReq struct {
	Model     string      `json:"model"`
	Messages  []anthroMsg `json:"messages"`
	MaxTokens int         `json:"max_tokens"`
	Stream    bool        `json:"stream"`
	System    string      `json:"system,omitempty"`
}

type anthroMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthroEvent struct {
	Type  string `json:"type"`
	Delta *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta,omitempty"`
}

func (a *Anthropic) Stream(ctx context.Context, req Request, out chan<- Chunk) error {
	defer close(out)

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 1024
	}

	var systemContent string
	var msgs []anthroMsg
	for _, m := range req.Messages {
		if m.Role == "system" {
			if systemContent != "" {
				systemContent += "\n"
			}
			systemContent += m.Content
		} else {
			msgs = append(msgs, anthroMsg{Role: m.Role, Content: m.Content})
		}
	}

	body, err := json.Marshal(anthroReq{Model: a.Model, Messages: msgs, MaxTokens: maxTokens, Stream: true, System: systemContent})
	if err != nil {
		out <- Chunk{Provider: a.Name(), Err: fmt.Errorf("marshal request: %w", err)}
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		out <- Chunk{Provider: a.Name(), Err: err}
		return err
	}
	httpReq.Header.Set("x-api-key", a.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		out <- Chunk{Provider: a.Name(), Err: err}
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("anthropic: status %d", resp.StatusCode)
		out <- Chunk{Provider: a.Name(), Err: err}
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
		var event anthroEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			continue
		}
		switch event.Type {
		case "content_block_delta":
			if event.Delta != nil && event.Delta.Type == "text_delta" && event.Delta.Text != "" {
				out <- Chunk{Content: event.Delta.Text, Provider: a.Name()}
			}
		case "message_stop":
			out <- Chunk{Provider: a.Name(), Done: true}
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		out <- Chunk{Provider: a.Name(), Err: err}
		return err
	}
	out <- Chunk{Provider: a.Name(), Done: true}
	return nil
}
