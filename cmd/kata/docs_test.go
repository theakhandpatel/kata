package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"agent/internal/hints"
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
		for _, name := range []string{"README.md", "example.md"} {
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

		for _, m := range reportsRe.FindAllStringSubmatch(string(readme), -1) {
			if n, _ := strconv.Atoi(m[2]); n != want {
				t.Errorf("%s/README.md says reports %s/%s, checks declare %d", slug, m[1], m[2], want)
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

		for _, doc := range []string{"README.md", "example.md"} {
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
		for _, doc := range []string{"README.md", "example.md"} {
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

	// A word-number for every count the README might state. The earlier version stopped
	// at "twelve", so it silently passed "twenty-one" while four chapters were added.
	words := map[string]int{
		"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6,
		"seven": 7, "eight": 8, "nine": 9, "ten": 10, "eleven": 11, "twelve": 12,
		"thirteen": 13, "fourteen": 14, "fifteen": 15, "sixteen": 16, "seventeen": 17,
		"eighteen": 18, "nineteen": 19, "twenty": 20, "twenty-one": 21, "twenty-two": 22,
	}
	claims := regexp.MustCompile(`(\w+(?:-\w+)?) chapters`)
	for _, m := range claims.FindAllStringSubmatch(string(readme), -1) {
		if got, ok := words[m[1]]; ok && got != n {
			t.Errorf("README says %q chapters, %d chapters exist", m[1], n)
		}
	}

	// The parts count is stated in prose and is not derivable from a filename, so it is
	// counted from the chapter pages themselves.
	parts := map[string]bool{}
	entries, err := os.ReadDir(filepath.Join(root, "chapters"))
	if err != nil {
		t.Fatalf("read chapters: %v", err)
	}
	partRe := regexp.MustCompile(`Part ([IVX]+)`)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, "chapters", e.Name(), "README.md"))
		if err != nil {
			continue
		}
		if m := partRe.FindSubmatch(body); m != nil {
			parts[string(m[1])] = true
		}
	}
	partClaims := regexp.MustCompile(`(\w+) parts`)
	for _, m := range partClaims.FindAllStringSubmatch(string(readme), -1) {
		if got, ok := words[m[1]]; ok && got != len(parts) {
			t.Errorf("README says %q parts, the chapters use %d", m[1], len(parts))
		}
	}

	// A clone instruction that cannot be followed is worse than none. The README
	// shipped a literal `<this repo>` placeholder.
	if strings.Contains(string(readme), "<this repo>") {
		t.Error("README still has a <this repo> placeholder in the clone command")
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

	files := []string{
		"README.md",
		"docs/architecture.md",
		"docs/designs.md",
		"docs/chapter-format.md",
		// writing-style.md is excluded on purpose: it has to name the words it bans.
		"chapters/where-next.md",
	}
	entries, err := os.ReadDir(filepath.Join(root, "chapters"))
	if err != nil {
		t.Fatalf("read chapters: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() || len(e.Name()) < 3 {
			continue
		}
		for _, doc := range []string{"README.md", "example.md"} {
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

// Every chapter page has the same shape: open with the thing you are building, close with
// what finished looks like, and keep the four named sections in between. Chapters drift when
// they are written at different times, and a reader notices the inconsistency even when they
// cannot name it.
func TestChapterPagesHaveTheSameShape(t *testing.T) {
	root := repoRootForTests(t)
	required := []string{"What you are building", "What you need to build", "How you are checked", "If you get stuck"}

	for _, slug := range writtenChapters(t) {
		body, err := os.ReadFile(filepath.Join(root, "chapters", slug, "README.md"))
		if err != nil {
			continue
		}

		// Headings outside fenced blocks only, or an example file's contents count.
		var heads []string
		inFence := false
		for _, line := range strings.Split(string(body), "\n") {
			if strings.HasPrefix(line, "```") {
				inFence = !inFence
				continue
			}
			if !inFence && strings.HasPrefix(line, "## ") {
				heads = append(heads, strings.TrimSpace(strings.TrimPrefix(line, "## ")))
			}
		}
		if len(heads) == 0 {
			t.Errorf("%s has no sections", slug)
			continue
		}

		if heads[0] != "What you are building" {
			t.Errorf("%s opens with %q; it should open with 'What you are building'", slug, heads[0])
		}
		if heads[len(heads)-1] != "Done when" {
			t.Errorf("%s ends with %q; it should end with 'Done when'", slug, heads[len(heads)-1])
		}
		for _, want := range required {
			found := false
			for _, h := range heads {
				if h == want {
					found = true
				}
			}
			if !found {
				t.Errorf("%s has no %q section", slug, want)
			}
		}
	}
}

// The example page's heading must carry its own chapter number. Ten of them drifted after
// the chapter splits, including one that claimed to be chapter 10 twice.
func TestExampleHeadingsMatchTheirChapter(t *testing.T) {
	root := repoRootForTests(t)
	for _, slug := range writtenChapters(t) {
		body, err := os.ReadFile(filepath.Join(root, "chapters", slug, "example.md"))
		if err != nil {
			continue
		}
		first := strings.SplitN(string(body), "\n", 2)[0]
		want := "# " + slug[:2] + " Example"
		if first != want {
			t.Errorf("%s/example.md starts with %q; want %q", slug, first, want)
		}
	}
}

// Each check file's leading comment names its own chapter. Ten of them drifted after the
// chapter splits, including two that claimed chapter 10.
func TestCheckCommentsNameTheirChapter(t *testing.T) {
	root := repoRootForTests(t)
	for _, slug := range writtenChapters(t) {
		dir := filepath.Join(root, "chapters", slug, "checks")
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if !strings.HasSuffix(f.Name(), "_test.go") {
				continue
			}
			body, err := os.ReadFile(filepath.Join(dir, f.Name()))
			if err != nil {
				continue
			}
			m := regexp.MustCompile(`Chapter (\d\d)`).FindStringSubmatch(string(body))
			if m == nil {
				continue
			}
			if m[1] != slug[:2] {
				t.Errorf("%s/%s names Chapter %s; it is chapter %s", slug, f.Name(), m[1], slug[:2])
			}
		}
	}
}

// Every case id must appear in its chapter's page. A case with no claim is a test the
// reader was not told about, and three shipped like that when cases were added late.
func TestEveryCaseHasAClaim(t *testing.T) {
	root := repoRootForTests(t)
	for _, slug := range writtenChapters(t) {
		body, err := os.ReadFile(filepath.Join(root, "chapters", slug, "README.md"))
		if err != nil {
			continue
		}
		page := string(body)
		for _, id := range caseIDs(t, slug) {
			if !strings.Contains(page, id) {
				t.Errorf("%s: case %q has no claim in README.md", slug, id)
			}
		}
	}
}
