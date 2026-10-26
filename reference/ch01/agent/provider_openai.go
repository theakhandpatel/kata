// Reference implementation for chapter 01: text only.
//
// It speaks the streaming protocol, folds content deltas, reads usage, and fails
// correctly. Tool calls arrive in later chapters; this file deliberately ignores
// the `tools` field and the `tool_calls` deltas, because chapter 01 does not
// grade them.
package agent

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

// OpenAICompat talks to any server that implements the OpenAI
// /v1/chat/completions API: Ollama, llama.cpp, vLLM, LM Studio, OpenRouter,
// Groq, DeepSeek, and OpenAI itself.
type OpenAICompat struct {
	BaseURL string
	APIKey  string
	Model   string

	// HTTPClient is optional. A harness needs per-request control, so never
	// rely on http.DefaultClient implicitly.
	HTTPClient *http.Client
}

func NewOpenAICompat(baseURL, apiKey, model string) *OpenAICompat {
	return &OpenAICompat{BaseURL: baseURL, APIKey: apiKey, Model: model}
}

var _ Provider = (*OpenAICompat)(nil)

// Chat sends one request and returns the fully accumulated assistant message.
func (p *OpenAICompat) Chat(ctx context.Context, req Request) (Message, Usage, error) {
	model := req.Model
	if model == "" {
		model = p.Model
	}

	payload := wireRequest{
		Model:    model,
		Messages: toWireMessages(req.Messages),
		Stream:   true,
	}
	// Pointer fields keep "unset" distinct from "zero". Sending max_tokens: 0
	// tells a real provider to produce nothing.
	if req.MaxTokens > 0 {
		payload.MaxTokens = &req.MaxTokens
	}
	if req.Temperature > 0 {
		payload.Temperature = &req.Temperature
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return Message{}, Usage{}, fmt.Errorf("encode request: %w", err)
	}

	url := strings.TrimRight(p.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Message{}, Usage{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if p.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)
	}

	client := p.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		// Return the error untouched: it already wraps ctx.Err(), so callers can
		// test for cancellation with errors.Is.
		return Message{}, Usage{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Message{}, Usage{}, &HTTPError{Status: resp.StatusCode, Body: string(snippet)}
	}

	return readSSE(resp.Body)
}

// readSSE folds the stream into one message. Three things are easy to get
// wrong, and all three are graded.
func readSSE(r io.Reader) (Message, Usage, error) {
	var (
		content strings.Builder
		usage   Usage
	)

	scanner := bufio.NewScanner(r)
	// The default 64 KiB line limit is too small for a real chunk.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// SSE framing: separators, comments, and named fields other than data.
		if line == "" || strings.HasPrefix(line, ":") || !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}

		var chunk wireChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return Message{}, Usage{}, fmt.Errorf("decode stream chunk: %w", err)
		}

		if chunk.Usage != nil {
			usage = Usage{
				PromptTokens:     chunk.Usage.PromptTokens,
				CompletionTokens: chunk.Usage.CompletionTokens,
				TotalTokens:      chunk.Usage.TotalTokens,
			}
		}

		for _, choice := range chunk.Choices {
			// Append. Assigning keeps only the last fragment.
			content.WriteString(choice.Delta.Content)
		}
	}
	if err := scanner.Err(); err != nil {
		// Without this, a dropped connection looks like a short valid answer.
		return Message{}, Usage{}, fmt.Errorf("read stream: %w", err)
	}

	return Message{Role: RoleAssistant, Content: content.String()}, usage, nil
}

// --- wire types ---

type wireRequest struct {
	Model       string        `json:"model"`
	Messages    []wireMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
}

type wireMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type wireChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func toWireMessages(msgs []Message) []wireMessage {
	out := make([]wireMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, wireMessage{Role: m.Role, Content: m.Content})
	}
	return out
}
