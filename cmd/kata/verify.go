package main

import (
	"fmt"
)

// cmdVerify proves the course is honest.
//
// For every written chapter it runs the same suite twice: once against
// every earlier reference overlay, which must pass every case, and once against the
// state before the chapter, which
// must fail at least one. A chapter whose starter passes grades nothing; a
// chapter whose reference fails is impossible to complete. Both are silent
// disasters that no amount of prose review catches, and both are cheap to detect
// offline.
//
// It also catches the class of bug that bit this project early: a negated
// assertion ("expect an error") passing against a blank stub.
func cmdVerify(args []string) error {
	root, err := findRoot()
	if err != nil {
		return err
	}

	var selected []chapter
	for _, c := range allChapters(root) {
		if c.written() {
			selected = append(selected, c)
		}
	}
	if len(selected) == 0 {
		return fmt.Errorf("no chapters written yet")
	}

	fmt.Printf("\n  verifying %d %s\n\n", len(selected), plural(len(selected), "chapter", "chapters"))

	type row struct {
		id     string
		refOK  bool
		refSum string
		stOK   bool
		stSum  string
	}
	var rows []row
	broken := 0

	for _, ch := range selected {
		refOK, refSum, err := runChapterKind(root, ch, "after")
		if err != nil {
			return err
		}
		stOK, stSum, err := runChapterKind(root, ch, "before")
		if err != nil {
			return err
		}

		r := row{id: ch.ID, refOK: refOK, refSum: refSum, stOK: stOK, stSum: stSum}
		rows = append(rows, r)

		if !refOK {
			broken++
			fmt.Printf("  %s chapter %s: the reference implementation FAILS its own checks (%s)\n\n", "BROKEN", ch.ID, refSum)
		}
		if stOK {
			broken++
			fmt.Printf("  %s chapter %s: the starter state PASSES the checks (%s)\n", "BROKEN", ch.ID, stSum)
			fmt.Printf("        that chapter grades nothing. A case is missing, or an assertion\n")
			fmt.Printf("        is satisfied by a stub (negated assertions are the usual culprit).\n\n")
		}
	}

	fmt.Printf("  %-4s %-28s %-28s\n", "", "reference (must pass)", "starter (must fail)")
	for _, r := range rows {
		fmt.Printf("  %-4s %-28s %-28s\n",
			r.id,
			verdict(r.refOK, r.refSum),
			verdict(!r.stOK, r.stSum))
	}
	fmt.Println()

	if broken > 0 {
		return fmt.Errorf("%d integrity %s", broken, plural(broken, "problem", "problems"))
	}
	fmt.Printf("  OK — every chapter rejects its own starter and accepts its reference.\n\n")
	return nil
}

func verdict(good bool, summary string) string {
	mark := "ok"
	if !good {
		mark = "WRONG"
	}
	return fmt.Sprintf("%-6s %s", mark, summary)
}
