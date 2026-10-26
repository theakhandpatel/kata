// Package grader is course infrastructure: the assertion engine that turns a
// failing programme into a specific sentence about what is wrong.
//
// Design rule: never fail with a bare "expected != actual". Every case carries
// a Why (the mechanism you got wrong) and a three-tier hint ladder (nudge,
// specific, worked example), because the difference between a useful exercise
// and a frustrating one is entirely in the error message.
package grader

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Case is one graded assertion.
type Case struct {
	ID    string
	Title string
	Why   string
	Hints []string
	Run   func(t *testing.T) error
}

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

// WantedErr asserts that err is non-nil, and returns a mismatch if it is nil.
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

// Suite runs every case, prints a per-case report to stdout, and fails the test
// if any case failed. Cases do not stop each other: you get the whole picture in
// one run, which is the entire point of the exercise.
func Suite(t *testing.T, id, title string, cases []Case) {
	t.Helper()

	fmt.Printf("%s\n\n", bold("chapter "+id+" -- "+title))

	passed := 0
	var failed []*Case

	for i, c := range cases {
		err := runCase(t, c)
		num := fmt.Sprintf("%02d", i+1)
		if err == nil {
			passed++
			fmt.Printf("  %s %s %s\n", green("PASS"), dim(num), c.Title)
			continue
		}
		failed = append(failed, &c)
		fmt.Printf("  %s %s %s\n", red("FAIL"), dim(num), c.Title)
		printFailure(id, c, err)
	}

	fmt.Printf("\n%s\n", divider())
	if len(failed) == 0 {
		fmt.Printf("%s  %d/%d cases passed\n\n", green("SUITE PASSED"), passed, len(cases))
		return
	}
	fmt.Printf("%s  %d/%d cases passed, %d failed\n\n",
		red("SUITE FAILED"), passed, len(cases), len(failed))

	for _, c := range failed {
		fmt.Printf("  - %s\n", c.ID)
	}
	fmt.Printf("\n  %s\n", dim("run `craft hint "+id+" <case-id>` for the full hint ladder"))
	fmt.Println()
	t.Fail()
}

func runCase(t *testing.T, c Case) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panicked: %v", r)
		}
	}()
	return c.Run(t)
}

func printFailure(id string, c Case, err error) {
	if c.Why != "" {
		fmt.Print(wrapped("        ", dim("why:")+" ", c.Why))
	}

	var m *Mismatch
	if errors.As(err, &m) {
		want := render(m.Want)
		got := render(m.Got)
		if m.Label != "" {
			fmt.Printf("        %s %s\n", dim("field:"), m.Label)
		}
		if strings.Contains(want, "\n") || strings.Contains(got, "\n") {
			fmt.Printf("        %s\n%s", dim("expected:"), indent(want, "          "))
			fmt.Printf("        %s\n%s", dim("actual:"), indent(got, "          "))
		} else {
			fmt.Printf("        %s %s\n", dim("expected:"), want)
			fmt.Printf("        %s %s\n", dim("actual:  "), got)
		}
	} else {
		fmt.Printf("        %s %s\n", dim("error:"), err.Error())
	}

	if len(c.Hints) > 0 {
		fmt.Print(wrapped("        ", dim("hint:")+" ", c.Hints[0]))
	}
	if len(c.Hints) > 1 {
		fmt.Printf("        %s craft hint %s %s\n", dim("more:"), id, c.ID)
	}
	fmt.Println()
}

// wrapped prints text with a hanging indent so long explanations stay readable
// in a terminal instead of scrolling off the right edge.
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
	case reflect.Slice, reflect.Map, reflect.Struct, reflect.Ptr, reflect.Interface:
		return fmt.Sprintf("%s%+v", typePrefix(v), v)
	}
	return fmt.Sprintf("%v (%T)", v, v)
}

func typePrefix(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("(%T) ", v)
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
