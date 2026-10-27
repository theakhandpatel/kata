// Command agent is the coding agent you are building.
//
// Contract, which later chapters keep:
//
//	agent -p "<prompt>"          the answer goes to stdout
//	agent -p "..." -events       one JSON event per line on stderr
//	HARNESS_DEBUG=1              token usage goes to stderr
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

	"kata/agent"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "agent: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		prompt    = flag.String("p", "", "the prompt to send (required)")
		model     = flag.String("m", "", "model name; defaults to $HARNESS_MODEL")
		timeout   = flag.Duration("timeout", 60*time.Second, "cancel the request after this long")
		toolTTL   = flag.Duration("tool-timeout", 10*time.Second, "cancel one tool call after this long")
		maxSteps  = flag.Int("max-steps", agent.DefaultMaxSteps, "stop after this many model requests")
		wantEvent = flag.Bool("events", false, "write one JSON event per line to stderr")
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
		firstNonEmpty(*model, os.Getenv("HARNESS_MODEL"), "gpt-4o-mini"),
	)

	root, err := os.Getwd()
	if err != nil {
		return err
	}
	tools := agent.NewRegistry(
		&agent.ReadTool{Root: root},
		&agent.WriteTool{Root: root},
		&agent.BashTool{Root: root, Timeout: *toolTTL},
	)

	var emit agent.Emitter
	if *wantEvent {
		emit = agent.JSONEmitter(os.Stderr)
	}

	res, err := agent.RunLoop(ctx, provider, tools,
		[]agent.Message{{Role: agent.RoleUser, Content: *prompt}},
		agent.LoopOptions{MaxSteps: *maxSteps, Emit: emit},
	)
	if err != nil {
		return err
	}

	fmt.Println(res.Message.Content)
	debugUsage(res.Usage)
	return nil
}

func debugUsage(u agent.Usage) {
	if os.Getenv("HARNESS_DEBUG") == "" {
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
