// Command harness is the coding agent you are building.
//
// It is the only executable in the course. Each chapter adds one behaviour to
// this program, and each chapter's checks run the binary rather than calling
// your package directly. A chapter is done when the program does something a
// user can see from a terminal.
//
// Contract, which later chapters keep:
//
//	harness -p "<prompt>"        the answer goes to stdout
//	HARNESS_DEBUG=1              token usage goes to stderr
//	exit 0 on success, 1 on error
//
// stdout carries the answer and nothing else, so a caller can pipe it. Anything
// you add for your own eyes belongs on stderr.
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

	"assay/harness"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "harness: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		prompt  = flag.String("p", "", "the prompt to send (required)")
		model   = flag.String("m", "", "model name; defaults to $HARNESS_MODEL")
		timeout = flag.Duration("timeout", 60*time.Second, "cancel the request after this long")
	)
	flag.Parse()

	if *prompt == "" {
		return errors.New(`-p is required, for example: harness -p "list the files"`)
	}

	// Ctrl-C cancels the request, and so does the timeout. Both arrive through
	// the same context, which is why the provider must accept one.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	provider := harness.NewOpenAICompat(
		envOr("OPENAI_BASE_URL", "http://localhost:11434/v1"),
		os.Getenv("OPENAI_API_KEY"),
		firstNonEmpty(*model, os.Getenv("HARNESS_MODEL"), "gpt-4o-mini"),
	)

	msg, usage, err := provider.Chat(ctx, harness.Request{
		Messages: []harness.Message{{Role: harness.RoleUser, Content: *prompt}},
	})
	if err != nil {
		return err
	}

	fmt.Println(msg.Content)
	debugUsage(usage)
	return nil
}

// debugUsage keeps the token counters observable without putting them in the
// answer. Chapters 04 and 07 rely on this channel to check accounting.
func debugUsage(u harness.Usage) {
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
