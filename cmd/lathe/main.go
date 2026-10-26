// Command lathe is the course runner.
//
// The learner should never have to type `go test`. This binary owns the
// vocabulary:
//
//	lathe                where am I, what is next, one action
//	lathe submit 01      compile triage, then grade
//	lathe read 01        the contract, in a pager
//	lathe read 01 overview   how it works, in general
//	lathe read 01 example    a full annotated implementation
//	lathe hint 01 usage  three rungs, no answer
//
// Grading is ordinary `go test` underneath, because the learner is building a
// harness and should not be shielded from the tool that grades them. What is
// hidden is the ceremony: build noise, package paths, flags, and the difference
// between "does not compile" and "wrong".
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "lathe:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return cmdStatus()
	}

	switch args[0] {
	case "submit", "run", "test", "s", "t":
		return cmdSubmit(args[1:])
	case "read", "open", "r":
		return cmdRead(args[1:])
	case "hint", "h":
		return cmdHint(args[1:])
	case "list", "ls":
		return cmdList()
	case "verify":
		return cmdVerify(args[1:])
	case "reset":
		return cmdReset(args[1:])
	case "progress", "p":
		return cmdProgress()
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown command %q (try `lathe help`)", args[0])
	}
}

func usage() {
	fmt.Print(`lathe -- build a coding-agent harness, one provable chapter at a time

  lathe                       where am I, what is next
  lathe submit [NN]           compile triage, then grade  (aliases: run, test)
  lathe submit NN --solution  grade the reference implementation instead
  lathe read [NN] [doc]       problem (default) | overview | example
  lathe hint NN <case-id>     the three-rung ladder for one case
  lathe verify                prove every chapter rejects its starter and accepts its reference
  lathe reset [NN]            restore harness/ to the start of a chapter
  lathe list                  the arc, with what is written and what is passed
  lathe progress              summary

You edit harness/. Nothing else. No ` + "`go test`" + ` required.
`)
}

// --- the arc ---

// planned is the whole arc, including chapters not yet written, so that
// `lathe list` communicates the shape of the course rather than only what
// happens to exist on disk. Keep in sync with README.md.
var planned = []struct {
	ID    string
	Title string
}{
	{"01", "Talk to the model"},
	{"02", "Tool calling"},
	{"03", "Tools that touch disk"},
	{"04", "The agent loop"},
	{"05", "Prompt assembly"},
	{"06", "Sessions"},
	{"07", "Context budget"},
	{"08", "Interrupt and steering"},
	{"09", "Skills"},
	{"10", "Subagent"},
	{"11", "MCP"},
}

func partOf(id string) string {
	n, _ := strconv.Atoi(id)
	switch {
	case n <= 2:
		return "I. Wire"
	case n <= 4:
		return "II. Act"
	case n <= 7:
		return "III. Remember"
	case n <= 9:
		return "IV. Extend"
	default:
		return "V. Compose"
	}
}

type chapter struct {
	ID    string
	Slug  string // "" when not written yet
	Title string
}

// allChapters merges the planned arc with what exists on disk.
func allChapters(root string) []chapter {
	onDisk := map[string]string{}
	if entries, err := os.ReadDir(filepath.Join(root, "chapters")); err == nil {
		for _, e := range entries {
			if !e.IsDir() || len(e.Name()) < 3 || e.Name()[2] != '-' {
				continue
			}
			onDisk[e.Name()[:2]] = e.Name()
		}
	}

	out := make([]chapter, 0, len(planned))
	for _, p := range planned {
		out = append(out, chapter{ID: p.ID, Slug: onDisk[p.ID], Title: p.Title})
	}
	return out
}

func (c chapter) written() bool { return c.Slug != "" }

func findChapter(root, arg string) (chapter, error) {
	all := allChapters(root)
	if arg == "" {
		arg = currentChapter(root).ID
	}
	if len(arg) == 1 {
		arg = "0" + arg
	}
	for _, c := range all {
		if c.ID == arg {
			return c, nil
		}
	}
	return chapter{}, fmt.Errorf("no chapter %q", arg)
}

// --- workspace ---

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

// --- progress ---

type progressEntry struct {
	Status string    `json:"status"` // passed | failed
	When   time.Time `json:"when"`
	Cases  string    `json:"cases,omitempty"`
}

type progressMap map[string]progressEntry

func progressPath(root string) string {
	return filepath.Join(root, ".lathe", "progress.json")
}

func loadProgress(root string) progressMap {
	b, err := os.ReadFile(progressPath(root))
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

func (p progressMap) passed(id string) bool {
	e, ok := p[id]
	return ok && e.Status == "passed"
}

// currentChapter is the first chapter that is written but not passed.
func currentChapter(root string) chapter {
	prog := loadProgress(root)
	var firstWritten *chapter
	for _, c := range allChapters(root) {
		if !c.written() {
			continue
		}
		if firstWritten == nil {
			cc := c
			firstWritten = &cc
		}
		if !prog.passed(c.ID) {
			return c
		}
	}
	if firstWritten != nil {
		return *firstWritten
	}
	return chapter{ID: "01", Title: "Talk to the model"}
}

func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func sortedIDs(m progressMap) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// plural is a small helper so messages read naturally at 1 and 0 cases.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func mustWritten(c chapter) error {
	if !c.written() {
		return fmt.Errorf("%s is not written yet — the course is being built in order; try `lathe list`", c.ID)
	}
	return nil
}
