package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"kata/internal/hints"
)

func timeNow() time.Time { return time.Now().UTC() }

// --- status: the default command ---

func cmdStatus() error {
	root, err := findRoot()
	if err != nil {
		return err
	}
	cur := currentChapter(root)
	prog := loadProgress(root)

	written := 0
	passed := 0
	for _, c := range allChapters(root) {
		if c.written() {
			written++
		}
		if prog.passed(c.ID) {
			passed++
		}
	}

	fmt.Printf("\n  kata · %s %s written · %d passed · %d planned\n\n",
		itoa(written), plural(written, "chapter", "chapters"), passed, len(planned))

	if prog.passed(cur.ID) {
		fmt.Printf("  everything written so far passes.\n")
		fmt.Printf("  next: more chapters are being written — see docs/chapter-format.md\n\n")
		return nil
	}

	fmt.Printf("  you are on chapter %s — %s\n\n", cur.ID, cur.Title)
	fmt.Printf("    1. read   kata read %s\n", cur.ID)
	fmt.Printf("    2. build  edit agent/\n")
	fmt.Printf("    3. submit kata submit %s\n\n", cur.ID)
	fmt.Printf("  stuck?  kata hint %s <case-id>\n", cur.ID)
	if isTTY() {
		fmt.Printf("  prefer keys?  kata ui\n")
	}
	fmt.Printf("  lost?   kata list\n\n")
	return nil
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }

// --- read: the three documents ---

var docNames = []string{"problem", "overview", "example", "readme"}

func cmdRead(args []string) error {
	root, err := findRoot()
	if err != nil {
		return err
	}
	chArg, doc := "", "problem"
	for _, a := range args {
		if isDocName(a) {
			doc = a
			continue
		}
		chArg = a
	}

	// The closing page is not a chapter, but it should be as easy to read.
	if chArg == "where-next" || chArg == "next" {
		body, err := os.ReadFile(filepath.Join(root, "chapters", "where-next.md"))
		if err != nil {
			return fmt.Errorf("no where-next page: %w", err)
		}
		return page(string(body))
	}

	ch, err := findChapter(root, chArg)
	if err != nil {
		return err
	}
	if err := mustWritten(ch); err != nil {
		return err
	}

	name := doc + ".md"
	if doc == "readme" {
		name = "README.md"
	}
	path := filepath.Join(root, "chapters", ch.Slug, name)
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("no %s for chapter %s (try: %s)", doc, ch.ID, strings.Join(docNames, ", "))
	}
	return page(string(body))
}

func isDocName(s string) bool {
	for _, d := range docNames {
		if s == d {
			return true
		}
	}
	return false
}

var mermaidFence = regexp.MustCompile("(?s)```mermaid\n.*?```")

// page prints markdown to the terminal, through $PAGER when there is one.
// Mermaid fences are replaced by a pointer, because a terminal cannot render
// them and a wall of graph syntax is worse than a note.
func page(md string) error {
	interactive := isTTY()
	if interactive {
		md = mermaidFence.ReplaceAllString(md, "  [diagram omitted — view this page rendered on GitHub, or read the ASCII sketch above]")
	}

	if !interactive {
		fmt.Print(md)
		return nil
	}

	pager := os.Getenv("PAGER")
	if pager == "" {
		if _, err := exec.LookPath("less"); err == nil {
			pager = "less -R"
		}
	}
	if pager == "" {
		fmt.Print(md)
		return nil
	}

	cmd := exec.Command("sh", "-c", pager)
	cmd.Stdin = strings.NewReader(md)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// --- list / progress ---

func cmdList() error {
	root, err := findRoot()
	if err != nil {
		return err
	}
	prog := loadProgress(root)

	fmt.Println()
	lastPart := ""
	for _, c := range allChapters(root) {
		if p := partOf(c.ID); p != lastPart {
			if lastPart != "" {
				fmt.Println()
			}
			fmt.Printf("  %s\n", p)
			lastPart = p
		}
		mark := " "
		note := ""
		switch {
		case !c.written():
			note = "not written yet"
			mark = "·"
		case prog.passed(c.ID):
			mark = "x"
			note = prog[c.ID].Cases
		default:
			note = "open"
		}
		fmt.Printf("    [%s] %s  %-26s %s\n", mark, c.ID, c.Title, note)
	}
	fmt.Println()
	fmt.Printf("  %d of %d chapters written. You edit agent/ only.\n\n", writtenCount(root), len(planned))
	return nil
}

func writtenCount(root string) int {
	n := 0
	for _, c := range allChapters(root) {
		if c.written() {
			n++
		}
	}
	return n
}

func cmdProgress() error {
	root, err := findRoot()
	if err != nil {
		return err
	}
	prog := loadProgress(root)
	done, total := 0, 0
	for _, c := range allChapters(root) {
		if !c.written() {
			continue
		}
		total++
		if prog.passed(c.ID) {
			done++
		}
	}
	fmt.Printf("\n  %d/%d written chapters passing\n\n", done, total)
	for _, id := range sortedIDs(prog) {
		e := prog[id]
		fmt.Printf("    %s  %-9s %s  (%s)\n", id, e.Status, e.Cases, e.When.Format("2006-01-02 15:04"))
	}
	fmt.Println()
	return nil
}

// --- hint ---

func cmdHint(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: kata hint <chapter> <case-id>")
	}
	ch := args[0]
	if len(ch) == 1 {
		ch = "0" + ch
	}
	id := args[1]

	tiers := hints.Get(ch, id)
	// A zero ladder means the id is unknown. Printing three empty rungs and exiting 0
	// makes a typo look like a case that has no hints.
	if tiers == ([3]string{}) {
		known := hints.CasesFor(ch)
		if len(known) == 0 {
			return fmt.Errorf("no hints for chapter %q — check `kata list` for chapter numbers", ch)
		}
		return fmt.Errorf("no case %q in chapter %s\n  known cases: %s",
			id, ch, strings.Join(known, ", "))
	}

	fmt.Printf("\n  %s\n\n", id)
	labels := []string{"1. nudge      ", "2. specific   ", "3. the answer "}
	for i, t := range tiers {
		label := fmt.Sprintf("%d. hint       ", i+1)
		if i < len(labels) {
			label = labels[i]
		}
		fmt.Printf("  %s %s\n\n", label, t)
	}
	return nil
}
