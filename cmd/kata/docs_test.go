package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"kata/internal/hints"
)

// The documentation and the grader must agree. They are the two halves of the course
// and nothing in the build compares them, so they drift silently until a learner
// notices. This test is that comparison.
//
// It exists because an independent learner run found a chapter README claiming six
// cases where the grader runs nine, and three case ids in the prose that had been
// renamed. Every assertion here is one of those defects.

func repoRootForTests(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..")
}

func writtenChapters(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repoRootForTests(t), "chapters"))
	if err != nil {
		t.Fatalf("read chapters: %v", err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && len(e.Name()) >= 3 && e.Name()[2] == '-' {
			out = append(out, e.Name())
		}
	}
	if len(out) == 0 {
		t.Fatal("no chapters found")
	}
	return out
}

var caseIDLine = regexp.MustCompile(`(?m)^\t+ID:\s+"([^"]+)"`)

func caseIDs(t *testing.T, slug string) []string {
	t.Helper()
	dir := filepath.Join(repoRootForTests(t), "chapters", slug, "checks")
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read checks for %s: %v", slug, err)
	}
	var out []string
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", f.Name(), err)
		}
		for _, m := range caseIDLine.FindAllStringSubmatch(string(body), -1) {
			out = append(out, m[1])
		}
	}
	if len(out) == 0 {
		t.Fatalf("no cases found in %s", slug)
	}
	return out
}

// Every chapter has the four documents and a checks file.
func TestChapterFilesExist(t *testing.T) {
	root := repoRootForTests(t)
	for _, slug := range writtenChapters(t) {
		for _, name := range []string{"README.md", "overview.md", "problem.md", "example.md"} {
			path := filepath.Join(root, "chapters", slug, name)
			info, err := os.Stat(path)
			if err != nil {
				t.Errorf("%s is missing %s", slug, name)
				continue
			}
			if info.Size() < 200 {
				t.Errorf("%s/%s is suspiciously small (%d bytes)", slug, name, info.Size())
			}
		}
	}
}

// A stated case count must equal the number of cases the checks declare.
func TestStatedCaseCountsMatch(t *testing.T) {
	root := repoRootForTests(t)
	gradedRe := regexp.MustCompile(`(\d+)\s+graded cases`)
	reportsRe := regexp.MustCompile(`reports[` + "`" + ` ]*(\d+)/(\d+)`)

	for _, slug := range writtenChapters(t) {
		want := len(caseIDs(t, slug))

		readme, err := os.ReadFile(filepath.Join(root, "chapters", slug, "README.md"))
		if err != nil {
			t.Fatalf("read README for %s: %v", slug, err)
		}
		for _, m := range gradedRe.FindAllStringSubmatch(string(readme), -1) {
			if n, _ := strconv.Atoi(m[1]); n != want {
				t.Errorf("%s/README.md says %d graded cases, checks declare %d", slug, n, want)
			}
		}

		problem, err := os.ReadFile(filepath.Join(root, "chapters", slug, "problem.md"))
		if err != nil {
			t.Fatalf("read problem for %s: %v", slug, err)
		}
		for _, m := range reportsRe.FindAllStringSubmatch(string(problem), -1) {
			if n, _ := strconv.Atoi(m[2]); n != want {
				t.Errorf("%s/problem.md says reports %s/%s, checks declare %d", slug, m[1], m[2], want)
			}
		}
	}
}

// Case ids named in prose must be cases that exist in that chapter.
//
// Only lines that talk about a case are checked, so command names and flags in prose
// do not trip it.
func TestDocCaseIDsExist(t *testing.T) {
	root := repoRootForTests(t)
	looksLikeID := regexp.MustCompile("^[a-z][a-z0-9]*(-[a-z0-9]+)+$")
	backticked := regexp.MustCompile("`([^`]+)`")
	mentionsCase := regexp.MustCompile(`(?i)\bcase\b|\bassert|\bcaught\b|\bfails?\b`)

	// Slugs that look like ids but are not. Keep this list short and explicit.
	notACase := map[string]bool{
		"read-only": true, "step-limit": true, "tool-timeout": true,
		"max-context": true, "max-steps": true, "tool-call": true,
		"system-message": true, "user-message": true, "context-window": true,
		"starter-state": true, "before-state": true, "after-state": true,
		"end-to-end": true, "key-value": true, "open-ended": true,
		"low-res": true, "long-running": true, "one-shot": true,
	}

	for _, slug := range writtenChapters(t) {
		chapter := slug[:2]
		known := map[string]bool{}
		for _, id := range hints.CasesFor(chapter) {
			known[id] = true
		}
		if len(known) == 0 {
			t.Errorf("chapter %s has no hint ladders", chapter)
			continue
		}

		for _, doc := range []string{"overview.md", "problem.md", "example.md", "README.md"} {
			body, err := os.ReadFile(filepath.Join(root, "chapters", slug, doc))
			if err != nil {
				continue
			}
			for _, line := range strings.Split(string(body), "\n") {
				if !mentionsCase.MatchString(line) {
					continue
				}
				for _, m := range backticked.FindAllStringSubmatch(line, -1) {
					token := m[1]
					if !looksLikeID.MatchString(token) || notACase[token] {
						continue
					}
					if !known[token] {
						t.Errorf("%s/%s names `%s`, which is not a case in chapter %s\n  line: %s",
							slug, doc, token, chapter, strings.TrimSpace(line))
					}
				}
			}
		}
	}
}

// A chapter mentioned by the docs as a command must exist.
func TestDocumentedCommandsPointAtWrittenChapters(t *testing.T) {
	root := repoRootForTests(t)
	submitRe := regexp.MustCompile("kata submit ([0-9]{2})|kata read ([0-9]{2})")

	written := map[string]bool{}
	for _, slug := range writtenChapters(t) {
		written[slug[:2]] = true
	}

	for _, slug := range writtenChapters(t) {
		for _, doc := range []string{"README.md", "problem.md", "overview.md", "example.md"} {
			body, err := os.ReadFile(filepath.Join(root, "chapters", slug, doc))
			if err != nil {
				continue
			}
			for _, m := range submitRe.FindAllStringSubmatch(string(body), -1) {
				id := m[1]
				if id == "" {
					id = m[2]
				}
				if !written[id] {
					t.Errorf("%s/%s tells the reader to use chapter %s, which is not written", slug, doc, id)
				}
			}
		}
	}
}

// The two documents that must agree about how many chapters exist.
func TestRootDocsAgreeOnTheArc(t *testing.T) {
	root := repoRootForTests(t)
	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	n := len(writtenChapters(t))

	claims := regexp.MustCompile(`course of (\w+) chapters`)
	if m := claims.FindStringSubmatch(string(readme)); m != nil {
		words := map[string]int{"one": 1, "two": 2, "three": 3, "four": 4, "five": 5,
			"six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10, "eleven": 11, "twelve": 12}
		if got, ok := words[m[1]]; ok && got != n {
			t.Errorf("README says %q chapters, %d chapters exist", m[1], n)
		}
	}
}

// Prose checks. These are the tells the writing style forbids, limited to patterns
// that are always wrong here rather than matters of taste.
func TestProseHasNoAITells(t *testing.T) {
	root := repoRootForTests(t)
	banned := []struct {
		pattern string
		why     string
	}{
		{"—", "no em dashes; use a comma, a colon, or a new sentence"},
		{"crucial", "AI vocabulary"},
		{"delve", "AI vocabulary"},
		{"leverage", "AI vocabulary"},
		{"seamless", "AI vocabulary"},
		{"robust", "AI vocabulary"},
		{"showcase", "AI vocabulary"},
		{"underscore", "AI vocabulary"},
		{"comprehensive", "AI vocabulary"},
		{"is not just ", "state the point instead of the construction"},
		{"seam", "say 'interface' or describe the line in plain words"},
		{"delta log", "say 'the pieces of the reply'"},
		{"monotonic", "say 'it only goes up'"},
		{"idempotent", "say 'safe to repeat'"},
		{"deterministic", "say 'repeatable'"},
		{"orthogonal", "say 'separate'"},
		{"coalesce", "say 'merge'"},
		{"not only ", "state the point instead of the construction"},
		{"It's worth noting", "filler"},
		{"In order to", "use 'to'"},
	}

	files := []string{"README.md", "docs/architecture.md", "docs/designs.md", "docs/chapter-format.md"}
	entries, err := os.ReadDir(filepath.Join(root, "chapters"))
	if err != nil {
		t.Fatalf("read chapters: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() || len(e.Name()) < 3 {
			continue
		}
		for _, doc := range []string{"README.md", "overview.md", "problem.md", "example.md"} {
			files = append(files, filepath.Join("chapters", e.Name(), doc))
		}
	}

	for _, rel := range files {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			continue
		}
		for _, b := range banned {
			if strings.Contains(string(body), b.pattern) {
				line := lineContaining(string(body), b.pattern)
				t.Errorf("%s: %q appears (%s)\n  line: %s", rel, b.pattern, b.why, line)
			}
		}
	}
}

func lineContaining(body, needle string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, needle) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}
