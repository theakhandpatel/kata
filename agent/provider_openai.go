package agent

import (
	"context"
	"fmt"
	"net/http"
)

// OpenAICompat talks to any server that implements the OpenAI
// /v1/chat/completions API: Ollama, llama.cpp, vLLM, LM Studio, OpenRouter,
// Groq, DeepSeek, and OpenAI itself.
//
// BaseURL is the part before /chat/completions, e.g. "http://localhost:11434/v1".
type OpenAICompat struct {
	BaseURL string
	APIKey  string
	Model   string

	// HTTPClient is optional. If nil, use a sensible default. Never use
	// http.DefaultClient implicitly in a harness: you need per-request timeouts
	// and test injection.
	HTTPClient *http.Client
}

// NewOpenAICompat is provided as scaffolding. You will need it to compile
// chapter 01's checks, but the interesting work is in Chat.
func NewOpenAICompat(baseURL, apiKey, model string) *OpenAICompat {
	return &OpenAICompat{BaseURL: baseURL, APIKey: apiKey, Model: model}
}

// Chat sends one request and returns the fully accumulated assistant message.
//
// Chapter 01 goal: implement this. The server streams Server-Sent Events; you
// must reassemble the deltas into one Message plus a Usage. See
// chapters/01-chat-with-the-model/README.md.
func (p *OpenAICompat) Chat(ctx context.Context, req Request) (Message, Usage, error) {
	return Message{}, Usage{}, fmt.Errorf("%w: OpenAICompat.Chat", ErrNotImplemented)
}

// Ensure the stub satisfies the seam.
var _ Provider = (*OpenAICompat)(nil)
