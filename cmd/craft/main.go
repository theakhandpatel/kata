// Command craft is the course runner.
//
// It exists to make three things one keystroke away:
//
//	craft test 01     grade the chapter you are on
//	craft hint 01 X   get the next rung of the ladder, not the answer
//	craft progress    see the whole arc
//
// The grading itself is ordinary `go test`. That is deliberate: you are
// learning to build a harness, so the tooling that grades you should be a tool
// you already have to understand.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"agentcraft/internal/hints"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "craft:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}

	switch args[0] {
	case "list", "ls":
		return cmdList()
	case "test", "t":
		return cmdTest(args[1:])
	case "hint", "h":
		return cmdHint(args[1:])
	case "progress", "p":
		return cmdProgress()
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown command %q (try `craft help`)", args[0])
	}
}

func usage() {
	fmt.Print(`craft -- build a coding-agent harness, one provable chapter at a time

  craft list                  chapters and their status
  craft test [NN|all]         grade a chapter (default: all)
  craft test NN --solution    grade the reference implementation instead
  craft hint NN <case-id>     the three-tier hint ladder for one case
  craft progress              summary table

Chapters live in chapters/. You edit harness/. Nothing else.
`)
}

// --- chapters ---

type chapter struct {
	ID   string // "01"
	Slug string // "01-chat-with-the-model"
}

func findRoot() (string, error) {
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
			return "", fmt.Errorf("no go.mod found above %s", dir)
		}
		dir = parent
	}
}

func chapters(root string) ([]chapter, error) {
	entries, err := os.ReadDir(filepath.Join(root, "chapters"))
	if err != nil {
		return nil, fmt.Errorf("read chapters/: %w", err)
	}
	var out []chapter
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if len(name) < 3 || name[2] != '-' {
			continue
		}
		out = append(out, chapter{ID: name[:2], Slug: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func resolve(all []chapter, arg string) ([]chapter, error) {
	if arg == "" || arg == "all" {
		return all, nil
	}
	id := arg
	if len(id) == 1 {
		id = "0" + id
	}
	for _, c := range all {
		if c.ID == id {
			return []chapter{c}, nil
		}
	}
	return nil, fmt.Errorf("no chapter %q", arg)
}

// --- progress ---

type progressEntry struct {
	Status string    `json:"status"` // passed | failed
	When   time.Time `json:"when"`
	Cases  string    `json:"cases,omitempty"`
}

type progressMap map[string]progressEntry

func progressPath(root string) string {
	return filepath.Join(root, ".agentcraft", "progress.json")
}

func loadProgress(root string) progressMap {
	p := progressPath(root)
	b, err := os.ReadFile(p)
	if err != nil {
		return progressMap{}
	}
	var m progressMap
	if json.Unmarshal(b, &m) != nil {
		return progressMap{}
	}
	return m
}

func saveProgress(root string, m progressMap) error {
	p := progressPath(root)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0o644)
}

// --- test ---

func cmdTest(args []string) error {
	root, err := findRoot()
	if err != nil {
		return err
	}

	useSolution := false
	var target string
	for _, a := range args {
		switch a {
		case "--solution", "-s":
			useSolution = true
		default:
			target = a
		}
	}

	all, err := chapters(root)
	if err != nil {
		return err
	}
	selected, err := resolve(all, target)
	if err != nil {
		return err
	}

	prog := loadProgress(root)
	failures := 0

	for _, ch := range selected {
		passed, cases, err := runChapter(root, all, ch, useSolution)
		if err != nil {
			return err
		}
		if passed {
			fmt.Printf("  -> chapter %s passed (%s)\n\n", ch.ID, cases)
		} else {
			fmt.Printf("  -> chapter %s not passing yet (%s)\n\n", ch.ID, cases)
			failures++
		}
		if !useSolution {
			status := "failed"
			if passed {
				status = "passed"
			}
			prog[ch.ID] = progressEntry{Status: status, When: time.Now().UTC(), Cases: cases}
		}
	}

	if !useSolution {
		if err := saveProgress(root, prog); err != nil {
			return err
		}
	}

	if failures > 0 {
		return fmt.Errorf("%d chapter(s) not passing", failures)
	}
	return nil
}

// runChapter grades one chapter and returns whether it passed plus a "n/m"
// case summary.
func runChapter(root string, all []chapter, ch chapter, useSolution bool) (bool, string, error) {
	workDir := root
	if useSolution {
		tmp, cleanup, err := workspaceWithSolutions(root, all, ch.ID)
		if err != nil {
			return false, "", err
		}
		defer cleanup()
		workDir = tmp
		fmt.Printf("grading chapter %s against the reference solution\n\n", ch.ID)
	}

	cmd := exec.Command("go", "test", "./chapters/"+ch.Slug+"/checks/", "-count=1", "-v")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "NO_COLOR="+os.Getenv("NO_COLOR"))

	out, runErr := cmd.CombinedOutput()
	text, passed := filterTestOutput(string(out))
	fmt.Print(text)

	return passed, summarize(text), ignoreExit(runErr)
}

func summarize(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, "cases passed") {
			continue
		}
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "SUITE PASSED")
		line = strings.TrimPrefix(line, "SUITE FAILED")
		return strings.TrimSpace(line)
	}
	return "no summary"
}

func ignoreExit(err error) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return nil
	}
	return err
}

// filterTestOutput drops the `go test` bookkeeping so the grader's report is
// the only thing on screen.
func filterTestOutput(s string) (string, bool) {
	var b strings.Builder
	passed := false
	sc := bufio.NewScanner(strings.NewReader(s))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "=== RUN"), strings.HasPrefix(line, "=== PAUSE"), strings.HasPrefix(line, "=== CONT"):
			continue
		case strings.HasPrefix(line, "--- PASS:"):
			continue
		case strings.Contains(line, "SUITE PASSED"):
			passed = true
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String(), passed
}

// --- solution workspace ---

// workspaceWithSolutions copies the repo to a temp dir and overlays the
// reference implementations for every chapter up to and including id. Solutions
// are cumulative: chapter 03's reference is chapter 02's plus its own files.
func workspaceWithSolutions(root string, all []chapter, id string) (string, func(), error) {
	tmp, err := os.MkdirTemp("", "agentcraft-sol-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }

	if err := copyTree(root, tmp); err != nil {
		cleanup()
		return "", nil, err
	}

	for _, ch := range all {
		if ch.ID > id {
			break
		}
		src := filepath.Join(root, "solutions", "ch"+ch.ID)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := copyTree(src, tmp); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("overlay ch%s: %w", ch.ID, err)
		}
	}
	return tmp, cleanup, nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		base := filepath.Base(path)
		if d.IsDir() {
			switch base {
			case ".git", ".agentcraft", "solutions", "node_modules":
				return fs.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyFile(path, filepath.Join(dst, rel))
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// --- list / progress / hint ---

func cmdList() error {
	root, err := findRoot()
	if err != nil {
		return err
	}
	all, err := chapters(root)
	if err != nil {
		return err
	}
	prog := loadProgress(root)

	fmt.Println("chapters")
	for _, c := range all {
		mark := " "
		note := ""
		if p, ok := prog[c.ID]; ok {
			if p.Status == "passed" {
				mark = "x"
			}
			note = "  " + p.Status
		}
		title := strings.TrimPrefix(c.Slug, c.ID+"-")
		fmt.Printf("  [%s] %s  %-42s%s\n", mark, c.ID, strings.ReplaceAll(title, "-", " "), note)
	}
	fmt.Println()
	return nil
}

func cmdProgress() error {
	root, err := findRoot()
	if err != nil {
		return err
	}
	all, err := chapters(root)
	if err != nil {
		return err
	}
	prog := loadProgress(root)

	done := 0
	for _, c := range all {
		status := "not started"
		if p, ok := prog[c.ID]; ok {
			status = p.Status
			if p.Status == "passed" {
				status = "passed  " + p.Cases
				done++
			}
		}
		fmt.Printf("  %s  %s\n", c.ID, status)
	}
	fmt.Printf("\n  %d/%d chapters passing\n\n", done, len(all))
	return nil
}

func cmdHint(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: craft hint <chapter> <case-id>")
	}
	ch := args[0]
	if len(ch) == 1 {
		ch = "0" + ch
	}
	id := args[1]

	tiers := hints.Get(ch, id)
	if len(tiers) == 0 {
		return fmt.Errorf("no hints for chapter %s case %q", ch, id)
	}

	fmt.Printf("\n  %s\n\n", id)
	labels := []string{
		"1. nudge      ",
		"2. specific   ",
		"3. the answer ",
	}
	for i, t := range tiers {
		label := fmt.Sprintf("%d. hint", i+1)
		if i < len(labels) {
			label = labels[i]
		}
		fmt.Printf("  %s %s\n\n", label, t)
	}
	if len(tiers) == 1 {
		fmt.Println("  (this case is short; the rest is in the chapter README)")
	}
	return nil
}
