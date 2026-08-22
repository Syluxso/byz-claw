package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Syluxso/byzclaw/internal/ports"
)

// OpenAICompat talks to OpenAI-compatible chat completions + tools (xAI, OpenAI, Ollama).
type OpenAICompat struct {
	BaseURL string
	APIKey  string
	Model   string
	HTTP    *http.Client
}

func NewOpenAICompat(baseURL, apiKey, model string) *OpenAICompat {
	return &OpenAICompat{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Model:   model,
		HTTP:    &http.Client{Timeout: 120 * time.Second},
	}
}

type oaReq struct {
	Model    string          `json:"model"`
	Messages []oaMessage     `json:"messages"`
	Tools    []oaTool        `json:"tools,omitempty"`
}

type oaMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []oaToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type oaTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type oaToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type oaResp struct {
	Choices []struct {
		Message oaMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *OpenAICompat) Complete(ctx context.Context, req ports.CompletionRequest) (ports.CompletionResponse, error) {
	model := c.Model
	if req.Model != "" {
		model = req.Model
	}
	body := oaReq{Model: model, Messages: toOAMessages(req.Messages)}
	for _, t := range req.Tools {
		schema := t.Schema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		var tool oaTool
		tool.Type = "function"
		tool.Function.Name = t.Name
		tool.Function.Description = t.Description
		tool.Function.Parameters = schema
		body.Tools = append(body.Tools, tool)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return ports.CompletionResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return ports.CompletionResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return ports.CompletionResponse{}, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return ports.CompletionResponse{}, err
	}
	var parsed oaResp
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return ports.CompletionResponse{}, fmt.Errorf("decode model response: %w (%s)", err, truncate(string(respBody), 200))
	}
	if parsed.Error != nil {
		return ports.CompletionResponse{}, fmt.Errorf("model error: %s", parsed.Error.Message)
	}
	if resp.StatusCode >= 300 {
		return ports.CompletionResponse{}, fmt.Errorf("model HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 300))
	}
	if len(parsed.Choices) == 0 {
		return ports.CompletionResponse{}, fmt.Errorf("model returned no choices")
	}
	msg := parsed.Choices[0].Message
	out := ports.CompletionResponse{
		Content: msg.Content,
		Usage: ports.TokenUsage{
			Input:  parsed.Usage.PromptTokens,
			Output: parsed.Usage.CompletionTokens,
		},
	}
	for _, tc := range msg.ToolCalls {
		args := json.RawMessage(tc.Function.Arguments)
		if !json.Valid(args) {
			args = json.RawMessage(`{}`)
		}
		out.ToolCalls = append(out.ToolCalls, ports.ToolCall{
			ID:   tc.ID,
			Name: tc.Function.Name,
			Args: args,
		})
	}
	return out, nil
}

func toOAMessages(msgs []ports.Message) []oaMessage {
	out := make([]oaMessage, 0, len(msgs))
	for _, m := range msgs {
		om := oaMessage{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			args := string(tc.Args)
			if args == "" {
				args = "{}"
			}
			om.ToolCalls = append(om.ToolCalls, oaToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{Name: tc.Name, Arguments: args},
			})
		}
		out = append(out, om)
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
