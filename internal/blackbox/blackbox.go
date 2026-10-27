// Package blackbox builds and runs the learner's harness binary.
//
// The course grades a program, not a package. That choice decides a lot:
//
//   - a chapter is done when a user can see the behaviour in a terminal, not
//     when a function returns the right struct;
//   - the learner may structure the internals however they like, because nothing
//     reaches inside;
//   - anything the program prints to stdout is part of the contract, so the
//     output discipline is graded from chapter 01.
//
// Determinism comes from the environment, not from mocks. Each case starts a
// scripted server, points OPENAI_BASE_URL at it, and runs the binary.
package blackbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"agent/internal/fake"
)

// DefaultTimeout bounds one program run. A harness that blocks costs one case.
const DefaultTimeout = 25 * time.Second

var (
	buildOnce sync.Once
	builtPath string
	buildErr  error
)

// EntryPoint is where the learner's program lives. It is a variable so that a learner who
// restructures the tree can say where the entry point went, instead of being told their code
// does not compile.
//
//	KATA_ENTRY=./internal/agentcore/cmd/agent kata submit 01
var EntryPoint = "./agent/cmd/agent"

// packagePath is the importable package the harness lives in. Used only to give a better error
// when the entry point moved but the imports were not updated.
const packagePath = "agent/agent"

// Binary builds the harness once per test process and returns the path.
func Binary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		root, err := repoRoot()
		if err != nil {
			buildErr = err
			return
		}
		dir, err := os.MkdirTemp("", "kata-bin-*")
		if err != nil {
			buildErr = err
			return
		}
		out := filepath.Join(dir, "agent")

		entry := EntryPoint
		if v := os.Getenv("KATA_ENTRY"); v != "" {
			entry = v
		}

		cmd := exec.Command("go", "build", "-o", out, entry)
		cmd.Dir = root
		combined, err := cmd.CombinedOutput()
		if err != nil {
			buildErr = explainBuildFailure(root, entry, string(combined), err)
			return
		}
		builtPath = out
	})
	if buildErr != nil {
		t.Fatalf("%v", buildErr)
	}
	return builtPath
}

// explainBuildFailure turns the two confusing consequences of a restructure into sentences.
//
// Moving the entry point is allowed. Moving it and forgetting the import, or moving it without
// telling the grader, both produce compiler output that blames the wrong thing.
func explainBuildFailure(root, entry, output string, cause error) error {
	switch {
	case strings.Contains(output, "package "+packagePath+" is not in std"):
		return fmt.Errorf(
			"the program moved but its imports still say %q.\n"+
				"  Either keep the package at that path, or update the import to wherever you moved it,\n"+
				"  for example: import \"agent/internal/agentcore\" (the checks do not care).\n\n%s",
			packagePath, output)

	case strings.Contains(output, "no Go files"),
		strings.Contains(output, "cannot find main module"),
		strings.Contains(output, "no such directory"):
		return fmt.Errorf(
			"nothing to build at %q.\n"+
				"  If you restructured the tree, tell the grader where the entry point is:\n"+
				"      KATA_ENTRY=<path> kata submit NN\n"+
				"  The checks build and run your binary; they do not look inside the package.\n\n%s",
			entry, output)
	}
	return fmt.Errorf("build %s: %w\n%s", entry, cause, output)
}

var (
	mcpOnce sync.Once
	mcpPath string
	mcpErr  error
)

// MCPServer builds the scripted MCP server fixture once and returns its path.
func MCPServer(t *testing.T) string {
	t.Helper()
	mcpOnce.Do(func() {
		root, err := repoRoot()
		if err != nil {
			mcpErr = err
			return
		}
		dir := filepath.Join(root, ".kata", "bin")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			mcpErr = err
			return
		}
		out := filepath.Join(dir, "mcpserver")

		cmd := exec.Command("go", "build", "-o", out, "./internal/fake/mcpserver")
		cmd.Dir = root
		combined, err := cmd.CombinedOutput()
		if err != nil {
			mcpErr = fmt.Errorf("build ./internal/fake/mcpserver: %w\n%s", err, combined)
			return
		}
		mcpPath = out
	})
	if mcpErr != nil {
		t.Fatalf("%v", mcpErr)
	}
	return mcpPath
}

// syncBuffer is a bytes.Buffer with a mutex and a signal for the first write, so a caller
// can wait for output instead of sleeping for a guess.
type syncBuffer struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	got    bool
	ready  chan struct{}
	closed bool
}

func newSyncBuffer() *syncBuffer {
	return &syncBuffer{ready: make(chan struct{})}
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	n, err := b.buf.Write(p)
	if !b.got {
		b.got = true
		if !b.closed {
			close(b.ready)
			b.closed = true
		}
	}
	return n, err
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// waitForData blocks until something has been written, or the budget runs out.
//
// Running out is not a failure: the caller signals the process either way, and the case
// decides what the result means.
func (b *syncBuffer) waitForData(budget time.Duration) {
	b.mu.Lock()
	got := b.got
	b.mu.Unlock()
	if got {
		return
	}
	select {
	case <-b.ready:
	case <-time.After(budget):
	}
}

// Result is what a program run produced.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}

// TimedOut reports whether the run was killed by the harness timeout rather
// than exiting on its own.
func (r Result) TimedOut() bool { return r.ExitCode == -1 }

// Run executes the binary with extra environment variables and arguments.
func Run(t *testing.T, env map[string]string, args ...string) Result {
	t.Helper()
	return RunTimeout(t, DefaultTimeout, env, args...)
}

// RunIn is Run with a working directory. The workspace a tool sees is the
// directory the program was started in, so this is what makes file tools
// testable without touching your own checkout.
func RunIn(t *testing.T, dir string, env map[string]string, args ...string) Result {
	t.Helper()
	return run(t, dir, DefaultTimeout, env, args...)
}

// RunWithInput is Run with text on stdin. Steering reads it while the run is in
// progress, so the input is available from the first turn boundary onward.
func RunWithInput(t *testing.T, dir string, env map[string]string, stdin string, args ...string) Result {
	t.Helper()
	bin := Binary(t)

	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), flatten(env)...)
	cmd.Stdin = strings.NewReader(stdin)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	return finish(t, cmd, err, ctx, stdout.String(), stderr.String(), start)
}

// RunInterruptUntil starts the program, waits until the scripted server has flushed at least
// n bytes, sends SIGINT, and collects what the program printed.
//
// Waiting for bytes rather than for a duration is the difference between a fact and a hope.
// A fixed delay races process startup on a slow machine; bytes arriving means the answer is
// genuinely mid-stream, which is exactly the moment a person presses Ctrl-C.
func RunInterruptUntil(t *testing.T, dir string, env map[string]string, srv *fake.Server, n int, args ...string) Result {
	t.Helper()
	bin := Binary(t)

	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), flatten(env)...)

	out := newSyncBuffer()
	errOut := newSyncBuffer()
	cmd.Stdout = out
	cmd.Stderr = errOut

	start := time.Now()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start harness: %v", err)
	}

	if !srv.WaitForBytes(n, DefaultTimeout) {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("the server never wrote %d bytes; the program did not reach the stream", n)
	}
	_ = cmd.Process.Signal(os.Interrupt)

	err := cmd.Wait()
	res := finish(t, cmd, err, ctx, out.String(), errOut.String(), start)
	if cmd.ProcessState != nil {
		if ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			res.ExitCode = 128 + int(ws.Signal())
		}
	}
	return res
}

// RunTimeout is Run with an explicit wall-clock bound.
func RunTimeout(t *testing.T, limit time.Duration, env map[string]string, args ...string) Result {
	t.Helper()
	return run(t, "", limit, env, args...)
}

func run(t *testing.T, dir string, limit time.Duration, env map[string]string, args ...string) Result {
	t.Helper()
	bin := Binary(t)

	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), flatten(env)...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	return finish(t, cmd, err, ctx, stdout.String(), stderr.String(), start)
}

// finish turns a process outcome into a Result.
func finish(t *testing.T, cmd *exec.Cmd, err error, ctx context.Context,
	stdout, stderr string, start time.Time) Result {
	t.Helper()
	res := Result{Stdout: stdout, Stderr: stderr, Duration: time.Since(start)}

	if ctx.Err() == context.DeadlineExceeded {
		res.ExitCode = -1
		return res
	}
	if err != nil {
		var exit *exec.ExitError
		if asExitError(err, &exit) {
			res.ExitCode = exit.ExitCode()
			return res
		}
		t.Fatalf("run harness: %v", err)
	}
	return res
}

func asExitError(err error, target **exec.ExitError) bool {
	e, ok := err.(*exec.ExitError)
	if ok {
		*target = e
	}
	return ok
}

func flatten(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod above %s", dir)
		}
		dir = parent
	}
}
