// Command agent is the coding agent you are building.
//
// Contract, which later chapters keep:
//
//	agent -p "<prompt>"        the answer goes to stdout
//	AGENT_DEBUG=1            token usage goes to stderr
//	exit 0 on success, 1 on error
//
// stdout carries the answer and nothing else, so a caller can pipe it.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"agent/agent"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "agent: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		prompt  = flag.String("p", "", "the prompt to send (required)")
		model   = flag.String("m", "", "model name; defaults to $AGENT_MODEL")
		timeout = flag.Duration("timeout", 60*time.Second, "cancel the request after this long")
		toolTTL = flag.Duration("tool-timeout", 10*time.Second, "cancel one tool call after this long")
	)
	flag.Parse()

	if *prompt == "" {
		return errors.New(`-p is required, for example: agent -p "list the files"`)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	provider := agent.NewOpenAICompat(
		envOr("OPENAI_BASE_URL", "http://localhost:11434/v1"),
		os.Getenv("OPENAI_API_KEY"),
		firstNonEmpty(*model, os.Getenv("AGENT_MODEL"), "gpt-4o-mini"),
	)

	// Tools are resolved against the directory the program was started in, so
	// the workspace is whatever the user meant by running it here.
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	tools := agent.NewRegistry(
		&agent.ReadTool{Root: root},
		&agent.WriteTool{Root: root},
		&agent.BashTool{Root: root, Timeout: *toolTTL},
	)

	history := []agent.Message{{Role: agent.RoleUser, Content: *prompt}}

	msg, usage, err := provider.Chat(ctx, agent.Request{
		Messages: history,
		Tools:    tools.Defs(),
	})
	if err != nil {
		return err
	}
	history = append(history, msg)

	// One hop. If the model asked for tools, run them and give it one more
	// chance to answer. Chapter 04 replaces this with a loop, which is why the
	// limit is stated here rather than hidden.
	if len(msg.ToolCalls) > 0 {
		history = append(history, runTools(ctx, tools, msg.ToolCalls)...)

		msg, usage, err = provider.Chat(ctx, agent.Request{
			Messages: history,
			Tools:    tools.Defs(),
		})
		if err != nil {
			return err
		}
		history = append(history, msg)
	}

	fmt.Println(msg.Content)
	debugUsage(usage)
	return nil
}

// runTools executes every call in one assistant message and returns the results
// in the same order. Order matters: the next request pairs each result with its
// call by id, and providers reject a conversation where the pairing is broken.
func runTools(ctx context.Context, tools *agent.Registry, calls []agent.ToolCall) []agent.Message {
	out := make([]agent.Message, 0, len(calls))
	for _, call := range calls {
		res := tools.Run(ctx, call)
		out = append(out, agent.Message{
			Role:       agent.RoleTool,
			Content:    res.Content,
			ToolCallID: call.ID,
			Name:       call.Name,
		})
	}
	return out
}

func debugUsage(u agent.Usage) {
	if os.Getenv("AGENT_DEBUG") == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "usage prompt=%d completion=%d total=%d\n",
		u.PromptTokens, u.CompletionTokens, u.TotalTokens)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
