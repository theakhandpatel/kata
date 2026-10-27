package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// cmdDoctor checks the things that make the course fail in confusing ways.
//
// Every item here has produced a real, misleading error at some point: a Go version
// too old for the code, a read-only project directory, a missing pager, or a port
// that cannot be bound. One command that names them is cheaper than four surprises.
func cmdDoctor(args []string) error {
	root, err := findRoot()
	if err != nil {
		return err
	}

	type check struct {
		name   string
		level  string // ok, warn, fail
		detail string
	}
	var checks []check

	add := func(name, level, detail string) {
		checks = append(checks, check{name: name, level: level, detail: detail})
	}

	// Go version. The module declares a floor; the toolchain must meet it.
	if v, ok := goVersion(); ok {
		if atLeast(v, 1, 26) {
			add("go version", "ok", v)
		} else {
			add("go version", "fail", v+" is older than the 1.26 the module declares")
		}
	} else {
		add("go version", "fail", "the go command is not on PATH")
	}

	// Module.
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
		add("go.mod", "ok", root)
	} else {
		add("go.mod", "fail", "not found above the working directory")
	}

	// Writable scratch space. Grading builds a copy of the project here.
	scratch := filepath.Join(root, ".miyagi")
	if err := os.MkdirAll(scratch, 0o755); err == nil {
		probe := filepath.Join(scratch, "doctor-probe")
		if err := os.WriteFile(probe, []byte("ok"), 0o644); err == nil {
			_ = os.Remove(probe)
			add("scratch dir", "ok", scratch)
		} else {
			add("scratch dir", "fail", "cannot write to "+scratch)
		}
	} else {
		add("scratch dir", "fail", "cannot create "+scratch)
	}

	// A port for the scripted model server.
	if ln, err := net.Listen("tcp", "127.0.0.1:0"); err == nil {
		addr := ln.Addr().String()
		_ = ln.Close()
		add("loopback", "ok", "can bind "+addr)
	} else {
		add("loopback", "fail", "cannot bind a local port: "+err.Error())
	}

	// Terminal.
	if isTTY() {
		add("stdout", "ok", "a terminal")
	} else {
		add("stdout", "warn", "not a terminal; the TUI needs one, the other commands do not")
	}

	// Pager, used by `miyagi read`.
	if pager := os.Getenv("PAGER"); pager != "" {
		add("pager", "ok", pager)
	} else if _, err := exec.LookPath("less"); err == nil {
		add("pager", "ok", "less")
	} else {
		add("pager", "warn", "no PAGER and no less; pages print to stdout")
	}

	// Git, used for checkpoints and for diffing against the reference.
	if _, err := exec.LookPath("git"); err == nil {
		add("git", "ok", "on PATH")
	} else {
		add("git", "warn", "not on PATH; nothing in the course needs it, but you may want it")
	}

	// Chapters.
	written, total := 0, 0
	for _, c := range allChapters(root) {
		total++
		if c.written() {
			written++
		}
	}
	if written > 0 {
		add("chapters", "ok", fmt.Sprintf("%d of %d written", written, total))
	} else {
		add("chapters", "fail", "no chapters found under chapters/")
	}

	fmt.Println()
	failures := 0
	for _, c := range checks {
		mark := "ok  "
		switch c.level {
		case "warn":
			mark = "warn"
		case "fail":
			mark = "FAIL"
			failures++
		}
		fmt.Printf("  %-4s %-12s %s\n", mark, c.name, c.detail)
	}
	fmt.Println()

	if failures > 0 {
		return fmt.Errorf("%d doctor %s", failures, plural(failures, "check failed", "checks failed"))
	}
	fmt.Printf("  everything the course needs is in place.\n\n")
	return nil
}

func goVersion() (string, bool) {
	out, err := exec.Command("go", "version").CombinedOutput()
	if err != nil {
		// The runtime is the same Go the binary was built with, which is a useful
		// fallback when the command is not on PATH.
		return runtime.Version(), true
	}
	// "go version go1.26.9 linux/amd64"
	fields := strings.Fields(string(out))
	for _, f := range fields {
		if strings.HasPrefix(f, "go1.") {
			return f, true
		}
	}
	return strings.TrimSpace(string(out)), true
}

// atLeast compares a "go1.26.9" style version against a major/minor pair.
func atLeast(version string, wantMajor, wantMinor int) bool {
	v := strings.TrimPrefix(version, "go")
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	if major != wantMajor {
		return major > wantMajor
	}
	return minor >= wantMinor
}
