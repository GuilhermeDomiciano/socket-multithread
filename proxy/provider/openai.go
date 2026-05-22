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

type OpenAI struct {
	APIKey  string
	Model   string
	BaseURL string
}

func NewOpenAI(apiKey string) *OpenAI {
	return &OpenAI{APIKey: apiKey, Model: "gpt-4o", BaseURL: "https://api.openai.com"}
}

func (o *OpenAI) Name() string             { return "openai" }
func (o *OpenAI) CostPer1kTokens() float64 { return 0.005 }

type openAIReq struct {
	Model     string   `json:"model"`
	Messages  []oaiMsg `json:"messages"`
	Stream    bool     `json:"stream"`
	MaxTokens int      `json:"max_tokens,omitempty"`
}

type oaiMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type oaiChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

func (o *OpenAI) Stream(ctx context.Context, req Request, out chan<- Chunk) error {
	defer close(out)

	msgs := make([]oaiMsg, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = oaiMsg{Role: m.Role, Content: m.Content}
	}
	body, err := json.Marshal(openAIReq{Model: o.Model, Messages: msgs, Stream: true, MaxTokens: req.MaxTokens})
	if err != nil {
		out <- Chunk{Provider: o.Name(), Err: fmt.Errorf("marshal request: %w", err)}
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		out <- Chunk{Provider: o.Name(), Err: err}
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+o.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		out <- Chunk{Provider: o.Name(), Err: err}
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("openai: status %d", resp.StatusCode)
		out <- Chunk{Provider: o.Name(), Err: err}
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
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			out <- Chunk{Provider: o.Name(), Done: true}
			return nil
		}
		var c oaiChunk
		if err := json.Unmarshal([]byte(data), &c); err != nil || len(c.Choices) == 0 {
			continue
		}
		if content := c.Choices[0].Delta.Content; content != "" {
			out <- Chunk{Content: content, Provider: o.Name()}
		}
	}
	if err := scanner.Err(); err != nil {
		out <- Chunk{Provider: o.Name(), Err: err}
		return err
	}
	out <- Chunk{Provider: o.Name(), Done: true}
	return nil
}
