package main

import (
	"fmt"
	"strings"
)

// The course is graded as a ladder, not as a checklist.
//
// Chapter NN assumes every chapter before it. Grading a later one while an earlier one is red
// produces failures that are really the earlier chapter's job, and it hides the one piece of
// information a learner needs: where the frontier is. So grading runs in order and stops at
// the first chapter that does not pass.
//
// The suites themselves are independent: every check file is black box, so a broken chapter
// still compiles and a later chapter is still gradeable. The ladder is a choice about
// feedback, not a technical constraint. `--all` grades everything anyway, for an author who
// wants the whole picture.

// state is where a chapter sits in the ladder.
type state int

const (
	stateOpen   state = iota // not attempted, and every prerequisite passed
	stateLocked              // not attempted, and a prerequisite has not passed
	stateFailed              // attempted and failing
	statePassed              // attempted and passing
)

func (s state) String() string {
	switch s {
	case statePassed:
		return "passed"
	case stateFailed:
		return "failed"
	case stateLocked:
		return "blocked"
	default:
		return "ready"
	}
}

// ladder computes each written chapter's state in order.
//
// A chapter is locked when any earlier written chapter has not passed. Once a chapter is
// graded and fails, everything after it is locked, because the course builds upwards.
func ladder(root string, prog progressMap) map[string]state {
	out := map[string]state{}
	blocked := false

	for _, c := range allChapters(root) {
		if !c.written() {
			continue
		}
		entry, attempted := prog[c.ID]

		switch {
		case attempted && entry.Status == "passed":
			out[c.ID] = statePassed
		case attempted:
			out[c.ID] = stateFailed
			blocked = true
		case blocked:
			out[c.ID] = stateLocked
		default:
			out[c.ID] = stateOpen
		}

		// Anything that is not passed blocks the chapters after it. A chapter that has never
		// been attempted is the frontier, and only the one at the frontier is ready.
		if out[c.ID] != statePassed {
			blocked = true
		}
	}
	return out
}

// frontier returns the first chapter that is ready to be graded: not passed, not locked.
func frontier(root string, prog progressMap) (chapter, bool) {
	states := ladder(root, prog)
	for _, c := range allChapters(root) {
		if !c.written() {
			continue
		}
		if states[c.ID] == stateOpen {
			return c, true
		}
	}
	// Everything passed, or the first unfinished thing is failed. Either way, report the
	// first chapter that is not passed.
	for _, c := range allChapters(root) {
		if c.written() && states[c.ID] != statePassed {
			return c, true
		}
	}
	return chapter{}, false
}

// markString renders the ladder mark for `miyagi list`.
func markString(s state) string {
	switch s {
	case statePassed:
		return "x"
	case stateFailed:
		return "!"
	case stateLocked:
		return "-"
	default:
		return " "
	}
}

// explainState is the one-line reason a chapter cannot be graded yet.
func explainState(root string, prog progressMap, id string) string {
	states := ladder(root, prog)
	switch states[id] {
	case stateLocked, stateFailed:
		// Name the earliest chapter that is not passed.
		for _, c := range allChapters(root) {
			if !c.written() {
				continue
			}
			if c.ID == id {
				break
			}
			if states[c.ID] != statePassed {
				if states[c.ID] == stateFailed {
					return fmt.Sprintf("chapter %s has not passed, so chapter %s is waiting on it", c.ID, id)
				}
				return fmt.Sprintf("chapter %s has not been graded, so chapter %s is waiting on it", c.ID, id)
			}
		}
	}
	return ""
}

// describeLadder is the paragraph `miyagi status` prints so the rule is never a surprise.
func describeLadder() string {
	return strings.TrimSpace(`
Chapters are graded in order. The first one that does not pass becomes your frontier, and
everything after it waits, because each chapter uses what the one before it built.
`)
}
