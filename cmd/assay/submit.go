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
	"regexp"
	"strings"

	"assay/internal/grader"
)

// cmdSubmit is the only command a learner actually needs.
func cmdSubmit(args []string) error {
	root, err := findRoot()
	if err != nil {
		return err
	}

	useSolution := false
	target := ""
	for _, a := range args {
		switch a {
		case "--solution", "-s":
			useSolution = true
		default:
			target = a
		}
	}

	var selected []chapter
	if target == "" || target == "all" {
		for _, c := range allChapters(root) {
			if c.written() {
				selected = append(selected, c)
			}
		}
	} else {
		c, err := findChapter(root, target)
		if err != nil {
			return err
		}
		selected = []chapter{c}
	}
	if len(selected) == 0 {
		return errors.New("no chapters written yet")
	}
	for _, c := range selected {
		if err := mustWritten(c); err != nil {
			return err
		}
	}

	prog := loadProgress(root)
	failures := 0

	for _, ch := range selected {
		// Compile triage first. A build error is not a failed case, and saying so
		// is the single biggest UX win over raw `go test` output.
		if !useSolution {
			ok, report, err := triage(root)
			if err != nil {
				return err
			}
			if !ok {
				fmt.Printf("\n  chapter %s is not ready: the harness does not compile yet\n\n", ch.ID)
				fmt.Print(report)
				fmt.Printf("  no cases ran. That is not a failure — it is a build error.\n\n")
				if !prog.passed(ch.ID) {
					prog[ch.ID] = progressEntry{Status: "failed", When: timeNow(), Cases: "build error"}
					_ = saveProgress(root, prog)
				}
				return nil
			}
		}

		passed, cases, err := runChapter(root, ch, useSolution)
		if err != nil {
			return err
		}
		if passed {
			fmt.Printf("  %s chapter %s — %s\n\n", "PASS", ch.ID, cases)
		} else {
			fmt.Printf("  %s chapter %s — %s\n\n", "FAIL", ch.ID, cases)
			failures++
		}
		if !useSolution {
			status := "failed"
			if passed {
				status = "passed"
			}
			prog[ch.ID] = progressEntry{Status: status, When: timeNow(), Cases: cases}
		}
	}

	if !useSolution {
		if err := saveProgress(root, prog); err != nil {
			return err
		}
	}
	if failures > 0 {
		return fmt.Errorf("%d %s not passing", failures, plural(failures, "chapter is", "chapters are"))
	}
	if !useSolution {
		fmt.Printf("  next: assay read %s\n\n", nextChapterID(root, selected[len(selected)-1].ID))
	}
	return nil
}

func nextChapterID(root, after string) string {
	for _, c := range allChapters(root) {
		if c.ID > after && c.written() {
			return c.ID
		}
	}
	return after
}

// --- compile triage ---

var goErrRe = regexp.MustCompile(`^(?:\./)?([^\s:]+\.go):(\d+):(?:(\d+):)? (.*)$`)

// triage runs `go build ./...` and, on failure, returns a short, actionable
// report instead of the raw compiler dump. Returns false if the tree does not
// build. The report is returned rather than printed so the caller can put the
// headline above it.
func triage(root string) (bool, string, error) {
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err == nil {
		return true, "", nil
	}

	var b strings.Builder
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := goErrRe.FindStringSubmatch(line)
		if m == nil {
			// Not a located error (e.g. "no required module provides ...").
			if strings.Contains(line, "module") || strings.Contains(line, "cannot find") {
				fmt.Fprintf(&b, "    %s\n      → check go.mod / run `go mod tidy`\n", line)
			}
			continue
		}
		file, ln, col, msg := m[1], m[2], m[3], m[4]
		loc := file + ":" + ln
		if col != "" {
			loc += ":" + col
		}
		fmt.Fprintf(&b, "    %s\n      %s\n", loc, msg)
		if h := hintForBuildError(msg); h != "" {
			fmt.Fprintf(&b, "      → %s\n", h)
		}
		b.WriteString("\n")
	}
	return false, b.String(), nil
}

// hintForBuildError translates the half-dozen Go compiler messages that
// beginners actually hit into a sentence about the cause.
func hintForBuildError(msg string) string {
	switch {
	case strings.HasPrefix(msg, "undefined:"):
		return "that name is not declared here — usually a missing import, or a typo"
	case strings.Contains(msg, "imported and not used"):
		return "remove the import, or use the package"
	case strings.HasPrefix(msg, "declared and not used"):
		return "Go rejects unused variables; use it or replace it with _"
	case strings.Contains(msg, "missing return"):
		return "every code path in a function with results must return"
	case strings.Contains(msg, "cannot use"):
		return "type mismatch — the value's type does not match the parameter"
	case strings.Contains(msg, "assignment mismatch"):
		return "wrong number of values on one side of ="
	case strings.Contains(msg, "no required module provides"):
		return "missing dependency — run `go get <module>` then `go mod tidy`"
	case strings.Contains(msg, "expected declaration"), strings.Contains(msg, "expected ';'"):
		return "syntax error — look at the line above this one"
	case strings.Contains(msg, "cannot range over"):
		return "that value is not iterable; check what you are ranging over"
	case strings.Contains(msg, "too many return values"):
		return "the signature declares fewer results than you return"
	}
	return ""
}

// --- grading ---

func runChapter(root string, ch chapter, useSolution bool) (bool, string, error) {
	if !useSolution {
		return runChapterKind(root, ch, "")
	}
	return runChapterKind(root, ch, "after")
}

// runChapterKind grades a chapter in a temp overlay of `kind` ("" = the learner's
// tree). Kinds are "solutions" and "stubs".
func runChapterKind(root string, ch chapter, kind string) (bool, string, error) {
	workDir := root
	if kind != "" {
		tmp, cleanup, err := workspaceFor(root, ch.ID, kind)
		if err != nil {
			return false, "", err
		}
		defer cleanup()
		workDir = tmp
		label := "the reference implementation"
		if kind == "before" {
			label = "the state before this chapter"
		}
		fmt.Printf("  grading chapter %s against %s\n\n", ch.ID, label)
	}

	repPath := filepath.Join(os.TempDir(), fmt.Sprintf("assay-report-%d.json", os.Getpid()))
	defer os.Remove(repPath)

	cmd := exec.Command("go", "test", "-tags=assay_checks", "-count=1", "-v",
		"./chapters/"+ch.Slug+"/checks/")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"NO_COLOR="+os.Getenv("NO_COLOR"),
		"CRAFT_REPORT="+repPath,
	)

	out, _ := cmd.CombinedOutput() // exit status is not the signal; the report is
	text, _ := filterTestOutput(string(out))
	fmt.Print(text)

	// Prefer the machine-readable report. Scraping stdout is only a fallback for
	// the case where the test binary never ran to completion (compile error,
	// panic in TestMain, timeout kill).
	if rep, ok := readReport(repPath); ok {
		passed := rep.Total > 0 && rep.Passed == rep.Total
		return passed, fmt.Sprintf("%d/%d cases passed", rep.Passed, rep.Total), nil
	}
	_, passed := filterTestOutput(string(out))
	return passed, "no report written (the checks did not run to completion)", nil
}

func readReport(path string) (grader.Report, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return grader.Report{}, false
	}
	var rep grader.Report
	if err := json.Unmarshal(b, &rep); err != nil {
		return grader.Report{}, false
	}
	return rep, true
}

// filterTestOutput drops `go test` bookkeeping so the grader's report is the
// only thing on screen.
func filterTestOutput(s string) (string, bool) {
	var b strings.Builder
	passed := false
	sc := bufio.NewScanner(strings.NewReader(s))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	skipNext := false
	for sc.Scan() {
		line := sc.Text()
		if skipNext {
			skipNext = false
			continue
		}
		switch {
		case strings.HasPrefix(line, "=== RUN"), strings.HasPrefix(line, "=== PAUSE"), strings.HasPrefix(line, "=== CONT"):
			continue
		case strings.HasPrefix(line, "--- PASS:"):
			continue
		case strings.HasPrefix(line, "--- FAIL:"):
			// keep the FAIL line, drop the trailing "exit status 1"
			b.WriteString(line)
			b.WriteString("\n")
			skipNext = false
			continue
		case line == "FAIL", strings.HasPrefix(line, "FAIL\t"), strings.HasPrefix(line, "ok  \t"):
			continue
		case strings.HasPrefix(line, "exit status "):
			continue
		case strings.Contains(line, "SUITE PASSED"):
			passed = true
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String(), passed
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
	return "no cases ran"
}

// --- reference workspace ---

// workspaceWithSolutions copies the repo to a temp dir and overlays the
// reference implementations for every written chapter up to and including id.
// Solutions are cumulative: chapter 03's reference is chapter 02's plus its own
// files. The learner's own tree is never touched, and --solution never records
// progress.
// workspaceFor copies the repo to a temp dir and overlays chapter solutions.
//
// mode "after"  -> solutions for chapters 01..NN  (chapter NN must pass)
// mode "before" -> solutions for chapters 01..NN-1 (chapter NN must fail)
//
// The "before" state is derived, never stored. One artifact per chapter, and no
// way for a starter directory to drift from the reference it was subtracted from.
func workspaceFor(root, id, mode string) (string, func(), error) {
	tmp, err := os.MkdirTemp("", "assay-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }

	if err := copyTree(root, tmp); err != nil {
		cleanup()
		return "", nil, err
	}

	// A chapter starts from the previous chapter's end state. Chapter 01 starts
	// from the untouched starter.
	if mode == "before" {
		if id == "01" {
			if err := copyTree(filepath.Join(root, "starter"), tmp); err != nil {
				cleanup()
				return "", nil, err
			}
			return tmp, cleanup, nil
		}
	}

	for _, c := range allChapters(root) {
		if c.ID > id || !c.written() {
			continue
		}
		if mode == "before" && c.ID == id {
			break
		}
		src := filepath.Join(root, "solutions", "ch"+c.ID)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := copyTree(src, tmp); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("overlay ch%s: %w", c.ID, err)
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
		if d.IsDir() {
			switch filepath.Base(path) {
			case ".git", ".assay", "solutions", "stubs", "node_modules":
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
