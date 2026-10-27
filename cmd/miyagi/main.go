// Command miyagi is the course runner.
//
// The learner should never have to type `go test`. This binary owns the
// vocabulary:
//
//	miyagi                where am I, what is next, one action
//	miyagi submit 01      compile triage, then grade
//	miyagi read 01        the contract, in a pager
//	miyagi read 01 overview   how it works, in general
//	miyagi read 01 example    a full annotated implementation
//	miyagi hint 01 usage  three rungs, no answer
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
	"strings"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "miyagi:", err)
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
	case "site":
		return cmdSite(args[1:])
	case "ui", "tui":
		return cmdUI(args[1:])
	case "watch", "w":
		return cmdWatch(args[1:])
	case "doctor", "doc":
		return cmdDoctor(args[1:])
	case "progress", "p":
		return cmdProgress()
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown command %q (try `miyagi help`)", args[0])
	}
}

func usage() {
	fmt.Print(`miyagi -- build a coding-agent harness, one provable chapter at a time

  miyagi                       where am I, what is next
  miyagi submit [NN]           compile triage, then grade  (aliases: run, test)
  miyagi submit NN --solution  grade the reference implementation instead
  miyagi read [NN] [doc]       problem (default) | overview | example
  miyagi hint NN <case-id>     the three-rung ladder for one case
  miyagi verify                prove every chapter rejects its starter and accepts its reference
  miyagi reset [NN]            restore agent/ to the start of a chapter
  miyagi read where-next       what to build after the course
  miyagi site                  render the course as a static website (site/dist)
  miyagi ui                    terminal interface: pick a chapter, grade it
  miyagi watch [NN]            re-grade on every save
  miyagi doctor                check the environment
  miyagi list                  the arc, with what is written and what is passed
  miyagi progress              summary

You edit agent/. Nothing else. No ` + "`go test`" + ` required.
`)
}

// --- the arc ---

// The arc is derived from the chapters directory, never listed by hand.
//
// A chapter directory is `NN-slug`, and its part comes from a `part` file inside it if
// one exists, otherwise from the part ranges below. Adding a chapter means adding a
// directory; nothing in this file needs editing.
//
// parts is ordered, and each entry owns every chapter number from its first value up to
// the next entry's first value. Appending a chapter inside an existing range needs no
// change here; opening a new part adds one line.
var parts = []struct {
	From  int
	Title string
}{
	{1, "I. Wire"},
	{3, "II. Act"},
	{6, "III. Remember"},
	{9, "IV. Extend"},
	{11, "V. Compose"},
	{14, "VI. Operate"},
	{19, "VII. Store"},
	{21, "VIII. Configure"},
}

// chapterTitle turns "03-running-a-tool" into "Running a tool".
func chapterTitle(slug string) string {
	rest := slug
	if len(slug) > 3 && slug[2] == '-' {
		rest = slug[3:]
	}
	// Sentence case for the slug, then a few words that keep their own spelling.
	lower := map[string]bool{
		"a": true, "an": true, "and": true, "as": true, "at": true, "by": true,
		"for": true, "in": true, "of": true, "on": true, "or": true, "the": true,
		"to": true, "with": true,
	}
	upper := map[string]string{"mcp": "MCP", "sse": "SSE", "json": "JSON", "agent": "agent"}

	words := strings.Split(strings.ReplaceAll(rest, "-", " "), " ")
	for i, w := range words {
		if w == "" {
			continue
		}
		if up, ok := upper[w]; ok {
			words[i] = up
			continue
		}
		if i > 0 && lower[w] {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	title := strings.Join(words, " ")
	// The first word is always capitalised, even when it is in the exception list.
	if title != "" {
		title = strings.ToUpper(title[:1]) + title[1:]
	}
	return title
}

// partOf returns the part a chapter belongs to, from its number.
func partOf(id string) string {
	n, err := strconv.Atoi(id)
	if err != nil {
		return ""
	}
	title := parts[0].Title
	for _, p := range parts {
		if n >= p.From {
			title = p.Title
		}
	}
	return title
}

type chapter struct {
	ID    string
	Slug  string // "" when not written yet
	Title string
}

// allChapters reads the arc from the chapters directory.
//
// Order comes from the directory names, which start with a two digit number, so a new
// chapter appears in the right place without this file knowing it exists.
func allChapters(root string) []chapter {
	entries, err := os.ReadDir(filepath.Join(root, "chapters"))
	if err != nil {
		return nil
	}
	var out []chapter
	for _, e := range entries {
		if !e.IsDir() || len(e.Name()) < 3 || e.Name()[2] != '-' {
			continue
		}
		slug := e.Name()
		title := chapterTitle(slug)
		if body, err := os.ReadFile(filepath.Join(root, "chapters", slug, "title")); err == nil {
			if custom := strings.TrimSpace(string(body)); custom != "" {
				title = custom
			}
		}
		out = append(out, chapter{ID: slug[:2], Slug: slug, Title: title})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
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
	return filepath.Join(root, ".miyagi", "progress.json")
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
		return fmt.Errorf("%s is not written yet — the course is being built in order; try `miyagi list`", c.ID)
	}
	return nil
}
