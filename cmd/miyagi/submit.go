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

	"agent/internal/grader"
)

// errBuildFailed distinguishes "the program does not compile" from "a case failed". Both
// exit non-zero, and the message says which.
var errBuildFailed = errors.New("the program does not compile")

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

	all := allChapters(root)
	prog := loadProgress(root)

	var selected []chapter
	switch {
	case target == "all" || target == "every":
		// Everything, in order, without stopping. For an author who wants the whole picture.
		for _, c := range all {
			if c.written() {
				selected = append(selected, c)
			}
		}
	case target == "":
		// The ladder: from the frontier forward, stopping at the first chapter that fails.
		if c, ok := frontier(root, prog); ok {
			start := -1
			for i, x := range all {
				if x.ID == c.ID {
					start = i
				}
			}
			for _, x := range all[start:] {
				if x.written() {
					selected = append(selected, x)
				}
			}
		}
	default:
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

	// Grading a locked chapter is allowed but pointless: say why instead of printing a wall
	// of failures that belong to an earlier chapter.
	if target != "" && target != "all" && target != "every" {
		if why := explainState(root, prog, selected[0].ID); why != "" {
			fmt.Printf("  note: %s\n\n", why)
		}
	}
	if target != "" && target != "all" && target != "every" {
		prog = loadProgress(root)
	}

	failures := 0

	// Grading a chapter whose predecessor is unfinished usually means a wall of failures
	// that are really the predecessor's job. Say so once, and say it as a likelihood
	// rather than a rule: chapters are graded independently, and a later one can pass on
	// its own.

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
				fmt.Printf("  no cases ran, because the program does not compile.\n")
				fmt.Printf("  that is not a wrong answer, and it still counts as not passing.\n\n")
				if !prog.passed(ch.ID) {
					prog[ch.ID] = progressEntry{Status: "failed", When: timeNow(), Cases: "build error"}
					_ = saveProgress(root, prog)
				}
				// A build error must fail the command. A script that runs `miyagi submit` in CI
				// reads the exit code, and a zero here would report a broken tree as healthy.
				return errBuildFailed
			}
		}

		passed, cases, err := runChapter(root, ch, useSolution)
		if err != nil {
			return err
		}
		if passed {
			fmt.Printf("  %s chapter %s — %s\n\n", "PASS", ch.ID, cases)
			_ = saveProgressLater(root, prog, ch.ID, "passed", cases)
		} else {
			fmt.Printf("  %s chapter %s — %s\n\n", "FAIL", ch.ID, cases)
			failures++
			_ = saveProgressLater(root, prog, ch.ID, "failed", cases)

			// The ladder stops here unless the caller asked for everything. Everything after
			// this chapter assumes it, so its failures would be noise.
			if target != "all" && target != "every" {
				if next := nextChapterID(root, ch.ID); next != ch.ID {
					fmt.Printf("  the ladder stops here: chapter %s comes next, and it assumes %s.\n", next, ch.ID)
					fmt.Printf("  %s\n", "`miyagi submit all` grades everything anyway.")
					fmt.Printf("  %s\n\n", "`miyagi read "+ch.ID+"` reprints the chapter.")
				}
				break
			}
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
		fmt.Printf("  next: miyagi read %s\n\n", nextChapterID(root, selected[len(selected)-1].ID))
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
	// The tree can compile while the entry point the checks build is somewhere else. That
	// happens whenever a learner restructures, and it produces nine failures that all blame
	// the wrong thing, so it is caught here first.
	entry := os.Getenv("MIYAGI_ENTRY")
	if entry == "" {
		entry = "./agent/cmd/agent"
	}
	probe := exec.Command("go", "build", "-o", os.DevNull, entry)
	probe.Dir = root
	if out, err := probe.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		return false, fmt.Sprintf(
			"    nothing to build at %s\n"+
				"      %s\n"+
				"      → if you restructured the tree, tell the grader where the entry point is:\n"+
				"          MIYAGI_ENTRY=<path> miyagi submit NN\n"+
				"        the checks build and run your binary; they never look inside the package.\n\n",
			entry, firstLine(msg)), nil
	}

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

// firstLine returns the first non-empty line of compiler output, for a one-line summary.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			return strings.TrimSpace(line)
		}
	}
	return s
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

// gradeResult is one chapter's verdict, with the text a terminal would show.
//
// The text is returned rather than printed so that the TUI and the watcher can use
// the same grading path without scrabbling over shared stdout.
type gradeResult struct {
	Passed bool
	Cases  string
	Text   string
	Report grader.Report
}

// runChapterKind grades a chapter and prints what happened. Kinds are "after" (the
// reference implementation), "before" (the state before this chapter) and "" (the
// learner's tree).
func runChapterKind(root string, ch chapter, kind string) (bool, string, error) {
	if kind != "" {
		label := "the reference implementation"
		if kind == "before" {
			label = "the state before this chapter"
		}
		fmt.Printf("  grading chapter %s against %s\n\n", ch.ID, label)
	}

	res, err := gradeChapter(root, ch, kind)
	if err != nil {
		return false, "", err
	}
	fmt.Print(res.Text)
	return res.Passed, res.Cases, nil
}

// gradeChapter grades one chapter without printing anything.
func gradeChapter(root string, ch chapter, kind string) (gradeResult, error) {
	workDir := root
	if kind != "" {
		tmp, cleanup, err := workspaceFor(root, ch.ID, kind)
		if err != nil {
			return gradeResult{}, err
		}
		defer cleanup()
		workDir = tmp
	}

	repPath := filepath.Join(os.TempDir(), fmt.Sprintf("miyagi-report-%d.json", os.Getpid()))
	defer os.Remove(repPath)

	cmd := exec.Command("go", "test", "-tags=miyagi_checks", "-count=1", "-v",
		"./chapters/"+ch.Slug+"/checks/")
	cmd.Dir = workDir
	// Test temp dirs go beside the workspace for the same reason.
	testTmp := filepath.Join(workDir, ".miyagi", "tmp")
	_ = os.MkdirAll(testTmp, 0o755)

	cmd.Env = append(os.Environ(),
		"NO_COLOR="+os.Getenv("NO_COLOR"),
		"CRAFT_REPORT="+repPath,
		"TMPDIR="+testTmp,
	)

	out, _ := cmd.CombinedOutput() // exit status is not the signal; the report is
	text, _ := filterTestOutput(string(out))

	// Prefer the machine-readable report. Scraping stdout is only a fallback for
	// the case where the test binary never ran to completion (compile error,
	// panic in TestMain, timeout kill).
	if rep, ok := readReport(repPath); ok {
		return gradeResult{
			Passed: rep.Total > 0 && rep.Passed == rep.Total,
			Cases:  fmt.Sprintf("%d/%d cases passed", rep.Passed, rep.Total),
			Text:   text,
			Report: rep,
		}, nil
	}
	return gradeResult{
		Cases: "no report written (the checks did not run to completion)",
		Text:  text,
	}, nil
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

// saveProgressLater records one chapter's verdict in the in-memory map. The map is written
// to disk once, at the end of the run.
func saveProgressLater(root string, prog progressMap, id, status, cases string) error {
	prog[id] = progressEntry{Status: status, When: timeNow(), Cases: cases}
	return nil
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
	// Grade inside the project, not in the system temp dir. A copy plus a compiled
	// binary is about 12 MB per run, and 22 runs of `miyagi verify` will fill a small
	// tmpfs.
	workRoot := filepath.Join(root, ".miyagi", "work")
	if err := os.MkdirAll(workRoot, 0o755); err != nil {
		return "", nil, err
	}
	tmp, err := os.MkdirTemp(workRoot, "grade-*")
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
		src := filepath.Join(root, "reference", "ch"+c.ID)
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
			case ".git", ".miyagi", "reference", "stubs", "node_modules":
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
