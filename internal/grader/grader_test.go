package grader

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// Tests for the grader itself. A bug here misgrades every learner without any
// visible symptom, so each failure mode the grader claims to handle has a test.
//
// The tests call Run, not Suite, because a failing Suite marks the calling test
// failed. That is right for a real chapter and useless for asserting that a
// suite should fail.

func TestRunPassesWhenCasesPass(t *testing.T) {
	rep := Run(t, "99", "synthetic", []Case{
		{ID: "a", Title: "trivially true", Run: func(*testing.T) error { return nil }},
	})
	if rep.Passed != 1 || rep.Total != 1 {
		t.Fatalf("report = %d/%d, want 1/1", rep.Passed, rep.Total)
	}
}

func TestRunCountsFailures(t *testing.T) {
	rep := Run(t, "99", "synthetic", []Case{
		{ID: "a", Title: "true", Run: func(*testing.T) error { return nil }},
		{ID: "b", Title: "false", Run: func(*testing.T) error {
			return &Mismatch{Label: "x", Want: 1, Got: 2}
		}},
	})
	if rep.Passed != 1 || rep.Total != 2 {
		t.Fatalf("report = %d/%d, want 1/2", rep.Passed, rep.Total)
	}
	if rep.Cases[1].Error == "" {
		t.Fatal("failing case has no recorded error")
	}
}

func TestRunReportsZeroCases(t *testing.T) {
	rep := Run(t, "99", "synthetic", nil)
	if rep.Total != 0 {
		t.Fatalf("total = %d, want 0", rep.Total)
	}
}

func TestRunRecoversPanic(t *testing.T) {
	rep := Run(t, "99", "synthetic", []Case{
		{ID: "boom", Title: "panics", Run: func(*testing.T) error { panic("synthetic") }},
	})
	if rep.Passed != 0 {
		t.Fatal("a panicking case counted as passed")
	}
	if !strings.Contains(rep.Cases[0].Error, "panicked") {
		t.Fatalf("panic not recorded: %q", rep.Cases[0].Error)
	}
}

func TestRunBoundsEachCase(t *testing.T) {
	start := time.Now()
	rep := Run(t, "99", "synthetic", []Case{
		{ID: "hang", Title: "blocks", Timeout: 50 * time.Millisecond,
			Run: func(*testing.T) error { time.Sleep(2 * time.Second); return nil }},
	})
	if rep.Passed != 0 {
		t.Fatal("a hanging case counted as passed")
	}
	if !strings.Contains(rep.Cases[0].Error, "timed out") {
		t.Fatalf("timeout not recorded: %q", rep.Cases[0].Error)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("timeout did not bound the case: took %s", elapsed)
	}
}

func TestWantedErrRejectsNotImplemented(t *testing.T) {
	// The historical bug: "expect an error" is satisfied by a stub's sentinel,
	// so an unstarted chapter reports green.
	if err := WantedErr(errors.New("harness: not implemented yet")); err == nil {
		t.Fatal("WantedErr accepted a not-implemented error")
	}
	if err := WantedErr(nil); err == nil {
		t.Fatal("WantedErr accepted nil")
	}
	if err := WantedErr(errors.New("boom")); err != nil {
		t.Fatalf("WantedErr rejected a real error: %v", err)
	}
}

func TestEqJSONIgnoresMapOrder(t *testing.T) {
	a := map[string]any{"a": 1, "b": []any{1, 2}}
	b := map[string]any{"b": []any{1, 2}, "a": 1}
	if err := EqJSON(a, b); err != nil {
		t.Fatalf("equivalent JSON compared unequal: %v", err)
	}
	if err := EqJSON(a, map[string]any{"a": 1}); err == nil {
		t.Fatal("different JSON compared equal")
	}
}

func TestSubsetToleratesExtras(t *testing.T) {
	got := map[string]any{"model": "m", "stream": true, "extra": 1}
	if err := Subset(map[string]any{"model": "m", "stream": true}, got); err != nil {
		t.Fatalf("subset rejected extras: %v", err)
	}
	if err := Subset(map[string]any{"model": "m"}, map[string]any{"model": "n"}); err == nil {
		t.Fatal("subset accepted a wrong value")
	}
	if err := Subset(map[string]any{"model": "m"}, map[string]any{}); err == nil {
		t.Fatal("subset accepted a missing key")
	}
}

func TestFirstDiffLine(t *testing.T) {
	if _, ok := firstDiffLine(`"same"`, `"same"`); ok {
		t.Fatal("identical single-line strings reported a diff")
	}
	d, ok := firstDiffLine("a\nb\nc", "a\nX\nc")
	if !ok || d.n != 1 {
		t.Fatalf("expected one differing line, got %+v ok=%v", d, ok)
	}
}
