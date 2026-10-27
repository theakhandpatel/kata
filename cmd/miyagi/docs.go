package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"agent/internal/hints"
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

	fmt.Printf("\n  miyagi · %s %s · %d passed\n\n",
		itoa(written), plural(written, "chapter", "chapters"), passed)

	if prog.passed(cur.ID) {
		fmt.Printf("  everything written so far passes.\n")
		fmt.Printf("  every chapter passes. See chapters/where-next.md for what to build next.\n\n")
		return nil
	}

	fmt.Printf("  you are on chapter %s — %s\n\n", cur.ID, cur.Title)
	fmt.Printf("    1. read   miyagi read %s\n", cur.ID)
	fmt.Printf("    2. build  edit agent/\n")
	fmt.Printf("    3. submit miyagi submit %s\n\n", cur.ID)
	fmt.Printf("  stuck?  miyagi hint %s <case-id>\n", cur.ID)
	if isTTY() {
		fmt.Printf("  prefer keys?  miyagi ui\n")
	}
	fmt.Printf("  lost?   miyagi list\n\n")
	return nil
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }

// --- read: the three documents ---

var docNames = []string{"chapter", "example"}

func cmdRead(args []string) error {
	root, err := findRoot()
	if err != nil {
		return err
	}
	chArg, doc := "", "chapter"
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

	name := "README.md"
	if doc == "example" {
		name = "example.md"
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
	states := ladder(root, prog)

	// The column is as wide as the longest title, so nothing is truncated and nothing
	// misaligns when a chapter is renamed.
	titleWidth := 0
	for _, c := range allChapters(root) {
		if n := len(c.Title); n > titleWidth {
			titleWidth = n
		}
	}

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
		note := ""
		mark := " "
		if !c.written() {
			note = "not written yet"
			mark = "·"
		} else {
			st := states[c.ID]
			mark = markString(st)
			switch st {
			case statePassed:
				note = prog[c.ID].Cases
			case stateFailed:
				note = "failed: " + prog[c.ID].Cases
			case stateLocked:
				note = "blocked"
			default:
				note = "ready"
			}
		}
		fmt.Printf("    [%s] %s  %-*s  %s\n", mark, c.ID, titleWidth, c.Title, note)
	}
	fmt.Println()
	fmt.Printf("  %d chapters. You edit agent/ only.\n\n", writtenCount(root))
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
		return fmt.Errorf("usage: miyagi hint <chapter> <case-id>")
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
			return fmt.Errorf("no hints for chapter %q — check `miyagi list` for chapter numbers", ch)
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
