// Package grader is course infrastructure: the assertion engine that turns a
// failing programme into a specific sentence about what is wrong.
//
// Design rules, each of which exists because violating it produced a real
// misgrade:
//
//   - Every case carries a Why (the mechanism) and exactly three hint rungs.
//   - A case that panics or hangs fails *that case*, not the whole chapter.
//   - A suite with zero cases fails. Green-on-nothing is the worst bug a grader
//     can have.
//   - The machine-readable verdict is a JSON report, not a scraped stdout line.
package grader

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Case is one graded assertion.
type Case struct {
	ID    string
	Title string
	Why   string
	Hints [3]string
	// Timeout bounds the case. Zero means DefaultTimeout.
	Timeout time.Duration
	Run     func(t *testing.T) error
}

// DefaultTimeout bounds each case so a blocking harness fails one case with
// attribution instead of hanging the chapter until `go test -timeout` fires.
//
// It is deliberately generous. Most cases finish in well under a second on a laptop, but
// a shared CI runner under load is an order of magnitude slower, and a case that fails
// because the machine was busy teaches the reader nothing. Ten seconds caught that
// difference; thirty does not.
const DefaultTimeout = 30 * time.Second

// Mismatch is a failed comparison. Returning it lets the suite print the
// expected/actual pair without the case author formatting anything.
type Mismatch struct {
	Label string
	Want  any
	Got   any
}

func (m *Mismatch) Error() string {
	label := m.Label
	if label == "" {
		label = "value"
	}
	return fmt.Sprintf("%s: want %s, got %s", label, render(m.Want), render(m.Got))
}

// Eq compares two values and returns a *Mismatch describing the difference.
func Eq(want, got any) error { return Eqf("", want, got) }

// Eqf is Eq with a label naming the field under test.
func Eqf(label string, want, got any) error {
	if reflect.DeepEqual(want, got) {
		return nil
	}
	return &Mismatch{Label: label, Want: want, Got: got}
}

// EqJSON compares two values by their canonical JSON encoding. Use it for whole
// request bodies and anything the harness serialises.
func EqJSON(want, got any) error {
	w, err := json.Marshal(want)
	if err != nil {
		return fmt.Errorf("marshal expected: %w", err)
	}
	g, err := json.Marshal(got)
	if err != nil {
		return fmt.Errorf("marshal actual: %w", err)
	}
	if string(w) == string(g) {
		return nil
	}
	var wv, gv any
	_ = json.Unmarshal(w, &wv)
	_ = json.Unmarshal(g, &gv)
	return &Mismatch{Label: "json", Want: wv, Got: gv}
}

// Subset asserts that every key in want exists in got with an equal value, while
// tolerating additional keys. Use it when the harness may legitimately send
// optional fields the fixture does not care about.
func Subset(want, got map[string]any) error {
	for k, wv := range want {
		gv, ok := got[k]
		if !ok {
			return &Mismatch{Label: k, Want: wv, Got: "(absent)"}
		}
		wj, _ := json.Marshal(wv)
		gj, _ := json.Marshal(gv)
		if string(wj) != string(gj) {
			return &Mismatch{Label: k, Want: wv, Got: gv}
		}
	}
	return nil
}

// NoErr asserts that err is nil, unwrapping "not implemented" into a clearer
// message so a learner can tell "wrong" from "not started".
func NoErr(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "not implemented") {
		return fmt.Errorf("this chapter is not implemented yet: %w", err)
	}
	return err
}

// ErrIs asserts that err wraps or equals target.
func ErrIs(err, target error) error {
	if !errors.Is(err, target) {
		return &Mismatch{Label: "error", Want: target, Got: err}
	}
	return nil
}

// WantedErr asserts that err is non-nil.
//
// It also rejects the not-implemented sentinel: "negated" assertions (expect an
// error, expect a failure) would otherwise pass against a blank stub, which is
// the worst possible feedback -- green tests on an unbuilt chapter.
func WantedErr(err error) error {
	if err == nil {
		return errors.New("expected an error, got nil")
	}
	if strings.Contains(err.Error(), "not implemented") {
		return fmt.Errorf("this chapter is not implemented yet: %w", err)
	}
	return nil
}

// --- report ---

// CaseResult is one line of the machine-readable verdict.
type CaseResult struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Passed     bool   `json:"passed"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"durationMs"`
}

// Report is written to $CRAFT_REPORT so the CLI never scrapes stdout.
type Report struct {
	Chapter string       `json:"chapter"`
	Title   string       `json:"title"`
	Passed  int          `json:"passed"`
	Total   int          `json:"total"`
	Cases   []CaseResult `json:"cases"`
}

// Run executes every case, prints a per-case report, and returns the verdict. It
// never fails t: the caller decides what a failure means. That split exists
// because a grader whose own tests cannot assert "this suite should fail" is a
// grader nobody can test.
func Run(t *testing.T, id, title string, cases []Case) Report {
	t.Helper()

	fmt.Printf("%s\n\n", bold("chapter "+id+" -- "+title))

	rep := Report{Chapter: id, Title: title, Total: len(cases)}
	if len(cases) == 0 {
		writeReport(rep)
		return rep
	}

	for i, c := range cases {
		num := fmt.Sprintf("%02d", i+1)
		started := time.Now()
		err := runBounded(t, c)
		res := CaseResult{
			ID: c.ID, Title: c.Title, Passed: err == nil,
			DurationMs: time.Since(started).Milliseconds(),
		}
		if err == nil {
			fmt.Printf("  %s %s %s\n", green("PASS"), dim(num), c.Title)
			rep.Passed++
		} else {
			res.Error = err.Error()
			fmt.Printf("  %s %s %s\n", red("FAIL"), dim(num), c.Title)
			printFailure(id, c, err)
		}
		rep.Cases = append(rep.Cases, res)
	}

	writeReport(rep)
	return rep
}

// Suite runs the chapter and fails the test when anything is wrong, including a
// suite with no cases. Green-on-nothing is the worst bug a grader can have.
func Suite(t *testing.T, id, title string, cases []Case) {
	t.Helper()

	if len(cases) == 0 {
		fmt.Printf("%s\n\n", bold("chapter "+id+" -- "+title))
		fmt.Printf("%s  a suite with no cases is a bug, not a pass\n\n", red("SUITE INVALID"))
		writeReport(Report{Chapter: id, Title: title})
		t.Fatal("grader: zero cases")
		return
	}

	rep := Run(t, id, title, cases)

	fmt.Printf("\n%s\n", divider())
	if rep.Passed == rep.Total {
		fmt.Printf("%s  %d/%d cases passed\n\n", green("SUITE PASSED"), rep.Passed, rep.Total)
		return
	}
	fmt.Printf("%s  %d/%d cases passed, %d failed\n\n",
		red("SUITE FAILED"), rep.Passed, rep.Total, rep.Total-rep.Passed)

	for _, c := range rep.Cases {
		if !c.Passed {
			fmt.Printf("  - %s\n", c.ID)
		}
	}
	fmt.Printf("\n  %s\n\n", dim("run `miyagi hint "+id+" <case-id>` for the full hint ladder"))
	t.Fail()
}

func writeReport(rep Report) {
	path := os.Getenv("CRAFT_REPORT")
	if path == "" {
		return
	}
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, append(b, '\n'), 0o644)
}

// runBounded runs a case with a panic barrier and a timeout, so a harness that
// hangs or panics costs one case and names it.
func runBounded(t *testing.T, c Case) error {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("panicked: %v", r)
			}
		}()
		done <- c.Run(t)
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("timed out after %s (the harness blocked; check for an unclosed reader or a missing context)", timeout)
	}
}

func printFailure(id string, c Case, err error) {
	if c.Why != "" {
		fmt.Print(wrapped("        ", dim("why:")+" ", c.Why))
	}

	var m *Mismatch
	if errors.As(err, &m) {
		want, got := render(m.Want), render(m.Got)
		if m.Label != "" {
			fmt.Printf("        %s %s\n", dim("field:"), m.Label)
		}
		if line, ok := firstDiffLine(want, got); ok {
			fmt.Printf("        %s\n          %s\n", dim("expected:"), line.want)
			fmt.Printf("        %s\n          %s\n", dim("actual:  "), line.got)
			if line.n > 1 {
				fmt.Printf("        %s first of %d differing lines\n", dim("note:"), line.n)
			}
		} else if strings.Contains(want, "\n") || strings.Contains(got, "\n") {
			fmt.Printf("        %s\n%s", dim("expected:"), indent(want, "          "))
			fmt.Printf("        %s\n%s", dim("actual:"), indent(got, "          "))
		} else {
			fmt.Printf("        %s %s\n", dim("expected:"), want)
			fmt.Printf("        %s %s\n", dim("actual:  "), got)
		}
	} else {
		fmt.Printf("        %s %s\n", dim("error:"), err.Error())
	}

	rungs := 0
	for _, h := range c.Hints {
		if h != "" {
			rungs++
		}
	}
	if rungs > 0 && c.Hints[0] != "" {
		fmt.Print(wrapped("        ", dim("hint:")+" ", c.Hints[0]))
	}
	if rungs > 1 {
		fmt.Printf("        %s miyagi hint %s %s\n", dim("more:"), id, c.ID)
	}
	fmt.Println()
}

type diffLine struct {
	want, got string
	n         int
}

// firstDiffLine finds the first line that differs, and counts how many differ.
// A learner needs the divergence, not two twenty-line dumps.
func firstDiffLine(want, got string) (diffLine, bool) {
	wl := strings.Split(strings.TrimRight(want, "\n"), "\n")
	gl := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(wl) == 1 && len(gl) == 1 {
		return diffLine{}, false
	}
	n := len(wl)
	if len(gl) > n {
		n = len(gl)
	}
	differing := 0
	first := -1
	for i := 0; i < n; i++ {
		var a, b string
		if i < len(wl) {
			a = wl[i]
		}
		if i < len(gl) {
			b = gl[i]
		}
		if a != b {
			differing++
			if first < 0 {
				first = i
			}
		}
	}
	if first < 0 {
		return diffLine{}, false
	}
	a, b := "", ""
	if first < len(wl) {
		a = wl[first]
	}
	if first < len(gl) {
		b = gl[first]
	}
	return diffLine{want: fmt.Sprintf("line %d: %s", first+1, strings.TrimSpace(a)),
		got: fmt.Sprintf("line %d: %s", first+1, strings.TrimSpace(b)), n: differing}, true
}

func render(v any) string {
	switch x := v.(type) {
	case nil:
		return "nil"
	case string:
		if x == "" {
			return `"" (empty)`
		}
		return fmt.Sprintf("%q", x)
	case error:
		if x == nil {
			return "nil"
		}
		return x.Error()
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map, reflect.Slice, reflect.Struct, reflect.Ptr, reflect.Interface:
		if b, err := json.MarshalIndent(v, "", "  "); err == nil {
			return string(b)
		}
		return fmt.Sprintf("%T%+v", v, v)
	}
	return fmt.Sprintf("%v (%T)", v, v)
}

func indent(s, pad string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(pad)
		b.WriteString(l)
		b.WriteString("\n")
	}
	return b.String()
}

func divider() string {
	return dim(strings.Repeat("-", 60))
}

// --- tiny colour layer (no dependencies, respects NO_COLOR and pipes) ---

var colour = detectColour()

func detectColour() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func wrap(code, s string) string {
	if !colour {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func green(s string) string { return wrap("32", s) }
func red(s string) string   { return wrap("31", s) }
func dim(s string) string   { return wrap("2", s) }
func bold(s string) string  { return wrap("1", s) }

func wrapped(lead, label, text string) string {
	const width = 78
	pad := lead + strings.Repeat(" ", len(stripANSI(label)))
	words := strings.Fields(text)
	if len(words) == 0 {
		return lead + label + "\n"
	}

	var b strings.Builder
	b.WriteString(lead)
	b.WriteString(label)
	col := len(lead) + len(stripANSI(label))
	for i, w := range words {
		if i > 0 {
			if col+1+len(w) > width {
				b.WriteString("\n")
				b.WriteString(pad)
				col = len(pad)
			} else {
				b.WriteString(" ")
				col++
			}
		}
		b.WriteString(w)
		col += len(w)
	}
	b.WriteString("\n")
	return b.String()
}

func stripANSI(s string) string {
	if !colour {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
