package hints

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Every case in the table must have all three rungs. A missing rung is a broken
// ladder, and the grader would silently print a shorter ladder to a stuck
// learner.
func TestTableRungsComplete(t *testing.T) {
	for ch, cases := range Table {
		for id, rungs := range cases {
			for i, r := range rungs {
				if strings.TrimSpace(r) == "" {
					t.Errorf("chapter %s case %s: rung %d is empty", ch, id, i+1)
				}
			}
		}
	}
}

// Every case the checks declare must have a ladder, and every ladder must belong
// to a declared case. Both directions matter: an orphan hint is dead content, and
// a hintless case strands the learner.
func TestHintsMatchChecks(t *testing.T) {
	declared := declaredCases(t)

	for ch, cases := range declared {
		table, ok := Table[ch]
		if !ok {
			t.Errorf("chapter %s has checks but no hints", ch)
			continue
		}
		for id := range cases {
			if _, ok := table[id]; !ok {
				t.Errorf("chapter %s case %s has checks but no hint ladder", ch, id)
			}
		}
	}

	for ch, cases := range Table {
		for id := range cases {
			if _, ok := declared[ch][id]; !ok {
				t.Errorf("chapter %s hint %q belongs to no declared case (typo?)", ch, id)
			}
		}
	}
}

var caseIDRe = regexp.MustCompile(`ID:\s*"([^"]+)"`)

// declaredCases scrapes chapter check files for `ID: "..."` literals. This is
// deliberately crude: it is a consistency linter, not a parser.
func declaredCases(t *testing.T) map[string]map[string]bool {
	t.Helper()
	out := map[string]map[string]bool{}

	root := "../../chapters"
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read chapters: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() || len(e.Name()) < 3 {
			continue
		}
		ch := e.Name()[:2]
		checks := root + "/" + e.Name() + "/checks"
		files, err := os.ReadDir(checks)
		if err != nil {
			continue
		}
		out[ch] = map[string]bool{}
		for _, f := range files {
			if !strings.HasSuffix(f.Name(), "_test.go") {
				continue
			}
			b, err := os.ReadFile(checks + "/" + f.Name())
			if err != nil {
				continue
			}
			for _, m := range caseIDRe.FindAllStringSubmatch(string(b), -1) {
				out[ch][m[1]] = true
			}
		}
	}
	return out
}
