// Command agent is the coding agent you are building.
//
// Contract, which later chapters keep:
//
//	agent -p "<prompt>"            the answer goes to stdout
//	agent -p "..." -events         one JSON event per line on stderr
//	agent -p "..." -steer-stdin    lines on stdin join the run at turn boundaries
//	HARNESS_DEBUG=1                token usage goes to stderr
//	exit 0 success, 1 error, 130 interrupted
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"kata/agent"
)

func main() {
	code, err := run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

func run() (int, error) {
	var (
		prompt      = flag.String("p", "", "the prompt to send (required)")
		model       = flag.String("m", "", "model name; defaults to $HARNESS_MODEL")
		timeout     = flag.Duration("timeout", 60*time.Second, "cancel the request after this long")
		toolTTL     = flag.Duration("tool-timeout", 10*time.Second, "cancel one tool call after this long")
		maxSteps    = flag.Int("max-steps", agent.DefaultMaxSteps, "stop after this many model requests")
		wantEvent   = flag.Bool("events", false, "write one JSON event per line to stderr")
		sessionPath = flag.String("session", "", "resume or create a session file")
		maxContext  = flag.Int("max-context", agent.DefaultMaxContext, "token budget per request; 0 disables pruning")
		steerStdin  = flag.Bool("steer-stdin", false, "treat lines on stdin as steering messages")
		mcpCommand  = flag.String("mcp", "", "command that starts an MCP server over stdio")
		retries     = flag.Int("retries", 3, "how many times to try a failed request")
		retryDelay  = flag.Duration("retry-delay", 500*time.Millisecond, "first wait between attempts")
		spillLimit  = flag.Int("spill-bytes", agent.DefaultSpillLimit, "move tool results larger than this to disk")
		maxTools    = flag.Int("max-tools", agent.DefaultMaxTools, "advertise at most this many tool schemas")
		maxTokens   = flag.Int("max-tokens", 0, "token limit for one reply; 0 leaves it unset")
		storePath   = flag.String("store", "", "session database; default <workspace>/.agent/sessions.db")
		resumeID    = flag.String("resume", "", "continue a session by id")
		continueLst = flag.Bool("continue", false, "continue the most recent session")
		listSess    = flag.Bool("sessions", false, "list stored sessions and exit")
		temperature = flag.Float64("temperature", 0, "sampling temperature; 0 leaves it unset")
	)
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	// Listing sessions is a read-only command. It needs no prompt and no provider.
	if *listSess {
		root, err := os.Getwd()
		if err != nil {
			return 1, err
		}
		path := *storePath
		if path == "" {
			path = filepath.Join(root, ".agent", "sessions.db")
		}
		sessions, err := agent.OpenStore(path)
		if err != nil {
			return 1, err
		}
		defer sessions.Close()

		listed, err := sessions.List(ctx)
		if err != nil {
			return 1, err
		}
		printSessions(listed)
		return 0, nil
	}

	if *prompt == "" {
		return 2, errors.New(`-p is required, for example: agent -p "list the files"`)
	}

	root, err := os.Getwd()
	if err != nil {
		return 1, err
	}

	var emit agent.Emitter
	if *wantEvent {
		emit = agent.JSONEmitter(os.Stderr)
	}

	client := agent.NewOpenAICompat(
		envOr("OPENAI_BASE_URL", "http://localhost:11434/v1"),
		os.Getenv("OPENAI_API_KEY"),
		firstNonEmpty(*model, os.Getenv("HARNESS_MODEL"), "gpt-4o-mini"),
	)
	client.Retry = agent.RetryPolicy{
		MaxAttempts: *retries,
		BaseDelay:   *retryDelay,
	}

	provider := agent.WithBudget(client, *maxContext, emit)

	// Skills: descriptions go in the system prompt, bodies wait until invoked.
	scoped, err := agent.LoadScoped(agent.DefaultScopes(root))
	if err != nil {
		return 1, err
	}
	catalog := scoped

	system, err := agent.BuildSystemPrompt(ctx, agent.BasePrompt,
		&agent.AgentsFile{Root: root},
		catalog,
	)
	if err != nil {
		return 1, err
	}

	// The child's registry is built first and does not contain the task tool, so a
	// subagent cannot start another one. The restriction is structural, not a rule
	// the child could ignore.
	baseTools := []agent.Tool{
		&agent.ReadTool{Root: root},
		&agent.WriteTool{Root: root},
		&agent.BashTool{Root: root, Timeout: *toolTTL},
	}
	// An MCP server is optional. If it cannot start, say so and carry on: an
	// unavailable extension must not stop the agent from answering.
	allTools := append([]agent.Tool{}, baseTools...)
	if *mcpCommand != "" {
		client, err := agent.StartMCP(ctx, *mcpCommand)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agent: mcp unavailable: %v\n", err)
		} else {
			defer client.Close()
			if err := client.Initialize(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "agent: mcp handshake failed: %v\n", err)
			} else if err := client.ListTools(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "agent: mcp tools/list failed: %v\n", err)
			} else {
				allTools = append(allTools, client.Tools()...)
			}
		}
	}

	// The child's registry is built from the built-in tools only, so a subagent
	// cannot recurse and does not inherit MCP tools it was not given.
	childTools := agent.NewRegistry(baseTools...)
	task := &agent.TaskTool{
		Provider: provider,
		Tools:    childTools,
		System:   system,
		MaxSteps: agent.DefaultSubagentSteps,
	}
	allTools = append(allTools, task)

	// Large results move to disk, and read_result fetches them back. The tool is added
	// to the registry so the model can use the handle it was given.
	store := &agent.SpillStore{Dir: filepath.Join(root, ".agent-results")}
	allTools = append(allTools, &agent.ReadResultTool{Store: store})

	// The registry advertises at most maxTools schemas. Anything past the ceiling is
	// reachable through load_tool, whose name is listed in the system prompt.
	full := agent.NewRegistry(allTools...)
	mounted := agent.NewMounted(full, *maxTools-1)
	loader := &agent.ToolLoader{Mounted: mounted}
	allTools = append(allTools, loader)
	full = agent.NewRegistry(allTools...)
	mounted = agent.NewMounted(full, *maxTools)
	mounted.Always(loader.Name())
	if list := mounted.ToolList(); list != "" {
		system = system + "\n" + list + "\n"
	}
	tools := agent.WithBudgetedRegistry(mounted, store, *spillLimit)

	// One store holds every conversation. The file-per-session recorder is still
	// available through -session, for a single throwaway transcript.
	storePathResolved := *storePath
	if storePathResolved == "" {
		storePathResolved = filepath.Join(root, ".agent", "sessions.db")
	}
	sessions, err := agent.OpenStore(storePathResolved)
	if err != nil {
		return 1, err
	}
	defer sessions.Close()

	sessionID := *resumeID
	if *continueLst && sessionID == "" {
		sessionID, err = sessions.Latest(ctx)
		if err != nil {
			return 1, err
		}
		if sessionID == "" {
			fmt.Fprintln(os.Stderr, "agent: no sessions to continue")
			return 1, nil
		}
	}

	// A single-file transcript, when asked for, alongside the store.
	var recorder agent.Recorder
	if *sessionPath != "" {
		rec, err := agent.OpenSession(*sessionPath)
		if err != nil {
			return 1, err
		}
		defer rec.Close()
		recorder = rec
	}

	prior := []agent.Message{}
	if sessionID != "" {
		// A mistyped id must not silently start a new conversation: the person thinks
		// they are rejoining a thread.
		ok, err := sessions.Exists(ctx, sessionID)
		if err != nil {
			return 1, err
		}
		if !ok {
			return 1, fmt.Errorf("no such session: %s", sessionID)
		}
		if prior, err = sessions.Messages(ctx, sessionID); err != nil {
			return 1, err
		}
	}

	history := make([]agent.Message, 0, len(prior)+2)
	history = append(history, agent.Message{Role: agent.RoleSystem, Content: system})
	history = append(history, prior...)
	// Expand slash commands before the prompt becomes history, so a resumed session
	// records what was actually sent rather than the shorthand.
	userText, _ := catalog.Expand(*prompt)

	history = append(history, agent.Message{Role: agent.RoleUser, Content: userText})

	opts := agent.LoopOptions{
		MaxSteps:    *maxSteps,
		Emit:        emit,
		MaxTokens:   *maxTokens,
		Temperature: *temperature,
	}
	if *steerStdin {
		steering := agent.NewStdinSteering(os.Stdin)
		steering.Start()
		opts.Steering = steering.Drain
	}

	res, err := agent.RunLoop(ctx, provider, tools, history, opts)

	// Persist before deciding, so an interrupted run still leaves a usable session.
	if saveErr := saveRun(ctx, sessions, sessionID, *prompt, res.History, len(prior)); saveErr != nil && err == nil {
		err = saveErr
	}
	if recorder != nil {
		for _, m := range res.History[len(prior)+1:] {
			if appendErr := recorder.Append(m); appendErr != nil && err == nil {
				err = appendErr
			}
		}
	}

	if err != nil {
		// Ctrl-C is a normal way to stop a long answer. Print what arrived, say so,
		// and use the shell's convention for an interrupted process.
		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			if res.Message.Content != "" {
				fmt.Println(res.Message.Content)
			}
			debugUsage(res.Usage)
			fmt.Fprintln(os.Stderr, "agent: aborted")
			return 130, nil
		}
		return 1, err
	}

	fmt.Println(res.Message.Content)
	debugUsage(res.Usage)
	return 0, nil
}

// saveRun writes the turns this run added, creating the session first when it is new.
func saveRun(ctx context.Context, store *agent.Store, id, prompt string, history []agent.Message, priorLen int) error {
	if id == "" {
		var err error
		id, err = store.Create(ctx, agent.TitleFromPrompt(prompt, 60))
		if err != nil {
			return err
		}
	}
	return store.Append(ctx, id, history[priorLen+1:])
}

// printSessions writes the table a person reads to pick a conversation.
func printSessions(sessions []agent.Session) {
	if len(sessions) == 0 {
		fmt.Println("no sessions yet")
		return
	}
	fmt.Printf("%-22s  %-40s  %4s  %s\n", "ID", "TITLE", "MSGS", "LAST USED")
	for _, s := range sessions {
		title := s.Title
		if len([]rune(title)) > 38 {
			title = string([]rune(title)[:38]) + "…"
		}
		fmt.Printf("%-22s  %-40s  %4d  %s\n", s.ID, title, s.Messages, s.Updated.Format("2006-01-02 15:04"))
	}
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
