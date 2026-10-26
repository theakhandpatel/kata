package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// cmdReset restores harness/ to the state at the start of a chapter.
//
// The state is derived, not stored: `assay reset 04` applies solutions/ch01
// through ch03 and drops the result into harness/. That is the same before-state
// `assay verify` grades, so what you reset to is exactly what the previous
// chapter proved.
//
// Only .go files are replaced. Notes and scratch files in harness/ survive.
func cmdReset(args []string) error {
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

	tmp, cleanup, err := workspaceFor(root, ch.ID, "before")
	if err != nil {
		return err
	}
	defer cleanup()

	target := filepath.Join(root, "harness")
	removed, err := removeGoFiles(target)
	if err != nil {
		return err
	}
	written, err := copyGoFiles(filepath.Join(tmp, "harness"), target)
	if err != nil {
		return err
	}

	fmt.Printf("\n  harness/ reset to the start of chapter %s\n", ch.ID)
	fmt.Printf("    removed %s, wrote %s\n\n",
		plural(removed, "1 file", fmt.Sprintf("%d files", removed)),
		plural(written, "1 file", fmt.Sprintf("%d files", written)))
	return nil
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func removeGoFiles(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func copyGoFiles(src, dst string) (int, error) {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return 0, err
	}
	n := 0
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if err := copyFile(path, filepath.Join(dst, rel)); err != nil {
			return err
		}
		n++
		return nil
	})
	return n, err
}
