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
)

// DefaultTimeout bounds one program run. A harness that blocks costs one case.
const DefaultTimeout = 25 * time.Second

var (
	buildOnce sync.Once
	builtPath string
	buildErr  error
)

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

		cmd := exec.Command("go", "build", "-o", out, "./agent/cmd/agent")
		cmd.Dir = root
		combined, err := cmd.CombinedOutput()
		if err != nil {
			buildErr = fmt.Errorf("build ./agent/cmd/agent: %w\n%s", err, combined)
			return
		}
		builtPath = out
	})
	if buildErr != nil {
		t.Fatalf("%v", buildErr)
	}
	return builtPath
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

// RunInterrupt starts the program, waits, sends SIGINT, then collects what it
// printed. This is how a person stops a runaway answer, so it is how the abort
// cases are graded.
func RunInterrupt(t *testing.T, dir string, env map[string]string, after time.Duration, args ...string) Result {
	t.Helper()
	bin := Binary(t)

	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), flatten(env)...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start harness: %v", err)
	}

	time.Sleep(after)
	_ = cmd.Process.Signal(os.Interrupt)

	err := cmd.Wait()
	res := finish(t, cmd, err, ctx, stdout.String(), stderr.String(), start)
	// A signal-terminated process reports -1 from ExitCode. Use the shell
	// convention of 128+signal so a test can assert 130 for SIGINT.
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

// StdoutLines is a convenience for asserting on the answer without worrying
// about a single trailing newline.
func StdoutLines(r Result) []string {
	trimmed := strings.TrimRight(r.Stdout, "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}
