package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// cmdWatch re-grades a chapter whenever its code changes.
//
// The loop is deliberate: a watcher is a program that runs forever, so it prints one
// line per attempt and nothing else. If you want the detail, run `kata submit`.
func cmdWatch(args []string) error {
	root, err := findRoot()
	if err != nil {
		return err
	}
	ch, err := findChapter(root, firstArg(args))
	if err != nil {
		return err
	}
	if err := mustWritten(ch); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("\n  watching chapter %s — %s\n", ch.ID, ch.Title)
	fmt.Printf("  %s\n\n", dim("edits to agent/ or the chapter's checks trigger a re-grade; Ctrl-C stops"))

	last := ""
	attempt := 0

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\n  stopped")
			return nil
		case <-time.After(400 * time.Millisecond):
		}

		snapshot := watchSnapshot(root, ch)
		if snapshot == last {
			continue
		}
		last = snapshot

		// A short settle delay keeps a burst of editor writes to one run.
		select {
		case <-ctx.Done():
			fmt.Println("\n  stopped")
			return nil
		case <-time.After(250 * time.Millisecond):
		}

		attempt++
		started := time.Now()
		res, err := gradeChapter(root, ch, "")
		if err != nil {
			fmt.Printf("  %s  %s\n", time.Now().Format("15:04:05"), err)
			continue
		}

		if res.Passed {
			fmt.Printf("  %s  %s  %s  %s\n",
				time.Now().Format("15:04:05"),
				green("pass"),
				res.Cases,
				dim(time.Since(started).Round(time.Millisecond).String()))
			continue
		}

		fmt.Printf("  %s  %s  %s  %s\n",
			time.Now().Format("15:04:05"),
			red("fail"),
			res.Cases,
			dim(time.Since(started).Round(time.Millisecond).String()))

		failed := failingCases(res.Report)
		if len(failed) > 0 {
			fmt.Printf("            %s\n", strings.Join(failed, ", "))
			fmt.Printf("            %s\n", dim("kata hint "+ch.ID+" "+failed[0]))
		}
	}
}

// watchSnapshot fingerprints everything a re-grade would read: the learner's agent
// and the chapter's checks.
func watchSnapshot(root string, ch chapter) string {
	var b strings.Builder

	for _, dir := range []string{
		filepath.Join(root, "agent"),
		filepath.Join(root, "chapters", ch.Slug, "checks"),
	} {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			fmt.Fprintf(&b, "%s:%d:%d;", path, info.Size(), info.ModTime().UnixNano())
			return nil
		})
	}
	return b.String()
}

// Small local wrappers so the watcher can colour without depending on the grader's
// private helpers.
func green(s string) string { return "\x1b[32m" + s + "\x1b[0m" }
func red(s string) string   { return "\x1b[31m" + s + "\x1b[0m" }
func dim(s string) string   { return "\x1b[2m" + s + "\x1b[0m" }
