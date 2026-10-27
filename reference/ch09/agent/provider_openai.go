// Reference implementation for chapter 01.
//
// Read this only after your own attempt fails. It replaces
// agent/provider_openai.go wholesale when you run `kata submit --solution 01`.
//
// The shape to notice: this file knows about the wire format and nothing else.
// No loop, no tools, no session. Those come in later chapters, and they will
// call Chat without caring how it works.
package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// OpenAICompat talks to any server that implements the OpenAI
// /v1/chat/completions API: Ollama, llama.cpp, vLLM, LM Studio, OpenRouter,
// Groq, DeepSeek, and OpenAI itself.
type OpenAICompat struct {
	BaseURL string
	APIKey  string
	Model   string

	HTTPClient *http.Client
}

func NewOpenAICompat(baseURL, apiKey, model string) *OpenAICompat {
	return &OpenAICompat{BaseURL: baseURL, APIKey: apiKey, Model: model}
}

var _ Provider = (*OpenAICompat)(nil)

// Chat sends one request and returns the fully accumulated assistant message.
//
// It is a thin wrapper: collecting the stream is a policy, not the primitive.
// Callers that want to see text as it arrives use Stream instead.
func (p *OpenAICompat) Chat(ctx context.Context, req Request) (Message, Usage, error) {
	return p.Stream(ctx, req, nil)
}

// Stream sends one request and calls onText for every content delta, in order.
//
// onText may be nil. It is called from the reading goroutine, so it must not
// block for long, and it must not assume the message is complete.
func (p *OpenAICompat) Stream(ctx context.Context, req Request, onText func(string)) (Message, Usage, error) {
	model := req.Model
	if model == "" {
		model = p.Model
	}

	payload := wireRequest{
		Model:    model,
		Messages: toWireMessages(req.Messages),
		Stream:   true,
	}
	if req.MaxTokens > 0 {
		payload.MaxTokens = &req.MaxTokens
	}
	if req.Temperature > 0 {
		payload.Temperature = &req.Temperature
	}
	if len(req.Tools) > 0 {
		payload.Tools = toWireTools(req.Tools)
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
		return Message{}, Usage{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Message{}, Usage{}, &HTTPError{Status: resp.StatusCode, Body: string(snippet)}
	}

	return readSSE(ctx, resp.Body, onText)
}

// readSSE reassembles a streamed completion. Three things are easy to get
// wrong, and all three are graded:
//
//   - text deltas append, they do not replace
//   - tool-call fragments are keyed by index, not by id (later fragments omit
//     the id entirely)
//   - the scanner's error is a real error
//
// readSSE joins the stream into one message.
//
// It takes the context because cancellation has to be noticed while reading, not after the
// stream ends. A person who presses Ctrl-C has read part of the answer already, and the
// request has to stop there rather than finishing the reply they asked to stop.
func readSSE(ctx context.Context, r io.Reader, onText func(string)) (Message, Usage, error) {
	var (
		content strings.Builder
		usage   Usage
		calls   = map[int]*callAccumulator{}
	)

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return Message{Role: RoleAssistant, Content: content.String()}, usage, err
		}

		line := scanner.Text()

		// SSE framing: comments, and named fields other than data.
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}

		var chunk wireChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			// Keep what arrived: a caller that aborts still wants the text it saw.
			return Message{Role: RoleAssistant, Content: content.String()}, usage,
				fmt.Errorf("decode stream chunk: %w", err)
		}

		if chunk.Usage != nil {
			usage = Usage{
				PromptTokens:     chunk.Usage.PromptTokens,
				CompletionTokens: chunk.Usage.CompletionTokens,
				TotalTokens:      chunk.Usage.TotalTokens,
			}
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" && onText != nil {
				onText(choice.Delta.Content)
			}
			content.WriteString(choice.Delta.Content)

			for _, tc := range choice.Delta.ToolCalls {
				acc := calls[tc.Index]
				if acc == nil {
					acc = &callAccumulator{}
					calls[tc.Index] = acc
				}
				if tc.ID != "" {
					acc.id = tc.ID
				}
				if tc.Function.Name != "" {
					acc.name = tc.Function.Name
				}
				acc.args.WriteString(tc.Function.Arguments)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		// An interrupted stream is not an empty answer. Return the partial message
		// alongside the error so the caller can print what the user already saw.
		return Message{Role: RoleAssistant, Content: content.String()}, usage,
			fmt.Errorf("read stream: %w", err)
	}

	msg := Message{Role: RoleAssistant, Content: content.String()}
	if len(calls) > 0 {
		indexes := make([]int, 0, len(calls))
		for i := range calls {
			indexes = append(indexes, i)
		}
		sort.Ints(indexes)
		for _, i := range indexes {
			acc := calls[i]
			msg.ToolCalls = append(msg.ToolCalls, ToolCall{
				ID:        acc.id,
				Name:      acc.name,
				Arguments: acc.args.String(),
			})
		}
	}

	return msg, usage, nil
}

type callAccumulator struct {
	id   string
	name string
	args strings.Builder
}

// --- wire types ---

type wireRequest struct {
	Model       string        `json:"model"`
	Messages    []wireMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	Tools       []wireTool    `json:"tools,omitempty"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name,omitempty"`
}

type wireToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type wireTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

type wireChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
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
		wm := wireMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
		for _, tc := range m.ToolCalls {
			var w wireToolCall
			w.ID = tc.ID
			w.Type = "function"
			w.Function.Name = tc.Name
			w.Function.Arguments = tc.Arguments
			wm.ToolCalls = append(wm.ToolCalls, w)
		}
		out = append(out, wm)
	}
	return out
}

func toWireTools(defs []ToolDef) []wireTool {
	out := make([]wireTool, 0, len(defs))
	for _, d := range defs {
		var w wireTool
		w.Type = "function"
		w.Function.Name = d.Name
		w.Function.Description = d.Description
		w.Function.Parameters = d.Parameters
		out = append(out, w)
	}
	return out
}
