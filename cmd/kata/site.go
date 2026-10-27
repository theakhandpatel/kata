package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// cmdSite renders the course as a static website with VitePress.
//
// The generator does one thing: copy the course markdown into a VitePress project,
// write the sidebar from the chapter directories, and call VitePress. Rendering,
// search, anchors, syntax highlighting, prev/next navigation and the theme all belong
// to VitePress, so this file has no Markdown parser and no CSS.
//
// The project it writes is disposable. `site/src` is regenerated on every run, and
// only `site/dist` is kept for deployment.
func cmdSite(args []string) error {
	root, err := findRoot()
	if err != nil {
		return err
	}

	out := filepath.Join(root, "site", "dist")
	title := "Kata"
	skipInstall := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--out", "-o":
			if i+1 < len(args) {
				out = args[i+1]
				i++
			}
		case "--title", "-t":
			if i+1 < len(args) {
				title = args[i+1]
				i++
			}
		case "--no-install":
			skipInstall = true
		}
	}
	if !filepath.IsAbs(out) {
		out = filepath.Join(root, out)
	}

	siteDir := filepath.Join(root, "site")
	srcDir := filepath.Join(siteDir, "src")

	if err := os.RemoveAll(srcDir); err != nil {
		return err
	}
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		return err
	}

	pages, err := prepareContent(root, srcDir)
	if err != nil {
		return err
	}
	if pages == 0 {
		return fmt.Errorf("nothing to render")
	}

	if err := writeProject(srcDir, title, root); err != nil {
		return err
	}

	if !skipInstall {
		if err := ensureDependencies(siteDir); err != nil {
			return err
		}
	}

	if err := build(siteDir); err != nil {
		return err
	}

	built := filepath.Join(srcDir, ".vitepress", "dist")
	if err := os.RemoveAll(out); err != nil {
		return err
	}
	if err := copyTree(built, out); err != nil {
		return fmt.Errorf("copy build output: %w", err)
	}

	fmt.Printf("\n  wrote %d pages to %s\n", pages, out)
	fmt.Printf("  %s\n\n", filepath.Join(out, "index.html"))
	return nil
}

// prepareContent copies the course markdown into the VitePress source tree.
//
// Paths are preserved, so every relative link written for `kata read` and GitHub also
// resolves here. Only `README.md` is renamed, because VitePress wants `index.md` for a
// directory's landing page.
func prepareContent(root, srcDir string) (int, error) {
	pages := 0

	copyPage := func(source, target string) error {
		body, err := os.ReadFile(filepath.Join(root, source))
		if err != nil {
			return fmt.Errorf("read %s: %w", source, err)
		}
		dest := filepath.Join(srcDir, target)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, body, 0o644); err != nil {
			return err
		}
		pages++
		return nil
	}

	// The home page is the repository README.
	if err := copyPage("README.md", "index.md"); err != nil {
		return 0, err
	}

	for _, ch := range allChapters(root) {
		if !ch.written() {
			continue
		}
		dir := "chapters/" + ch.Slug
		for _, doc := range []struct{ source, target string }{
			{dir + "/README.md", dir + "/index.md"},
			{dir + "/overview.md", dir + "/overview.md"},
			{dir + "/problem.md", dir + "/problem.md"},
			{dir + "/example.md", dir + "/example.md"},
		} {
			if _, err := os.Stat(filepath.Join(root, doc.source)); err != nil {
				continue
			}
			if err := copyPage(doc.source, doc.target); err != nil {
				return pages, err
			}
		}
	}

	for _, doc := range []string{
		"docs/architecture.md", "docs/chapter-format.md",
		"docs/writing-style.md", "docs/designs.md",
		"docs/adr/0001-vitepress-for-the-site.md",
		"chapters/where-next.md",
	} {
		if _, err := os.Stat(filepath.Join(root, doc)); err != nil {
			continue
		}
		if err := copyPage(doc, doc); err != nil {
			return pages, err
		}
	}

	return pages, nil
}

func writeProject(srcDir, title, root string) error {
	pkg := `{
  "name": "kata-site",
  "private": true,
  "type": "module",
  "devDependencies": {
    "mermaid": "^11.14.0",
    "vitepress": "^1.6.4",
    "vitepress-plugin-mermaid": "^2.0.17"
  }
}
`
	if err := os.WriteFile(filepath.Join(filepath.Dir(srcDir), "package.json"), []byte(pkg), 0o644); err != nil {
		return err
	}

	config := buildConfig(title, root)
	dir := filepath.Join(srcDir, ".vitepress")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config.mts"), []byte(config), 0o644)
}

// buildConfig writes the VitePress configuration, including the sidebar.
//
// The sidebar is the only part of the site that knows about the course, and it is
// derived from the chapter directories, so adding a chapter never means editing a
// config by hand.
func buildConfig(title, root string) string {
	var b strings.Builder

	b.WriteString("import { defineConfig } from 'vitepress'\n")
	b.WriteString("import { withMermaid } from 'vitepress-plugin-mermaid'\n\n")
	b.WriteString("export default withMermaid(defineConfig({\n")
	fmt.Fprintf(&b, "  title: %s,\n", strconv.Quote(title))
	b.WriteString("  description: 'Build a coding-agent harness in Go, graded offline by a local verifier.',\n")
	b.WriteString("  lang: 'en',\n")
	b.WriteString("  themeConfig: {\n")
	b.WriteString("    search: { provider: 'local' },\n")
	b.WriteString("    outline: { level: [2, 3] },\n")
	b.WriteString("    nav: [\n")
	b.WriteString("      { text: 'Home', link: '/' },\n")
	b.WriteString("      { text: 'Chapters', link: '/chapters/01-chat-with-the-model/' },\n")
	b.WriteString("      { text: 'Designs', link: '/docs/designs' },\n")
	b.WriteString("      { text: 'GitHub', link: 'https://github.com/theakhandpatel/kata' },\n")
	b.WriteString("    ],\n")
	b.WriteString("    sidebar: [\n")
	b.WriteString("      { text: 'Start', items: [{ text: 'What Kata is', link: '/' }] },\n")

	for _, part := range []string{"I. Wire", "II. Act", "III. Remember", "IV. Extend", "V. Compose"} {
		var items []string
		for _, ch := range allChapters(root) {
			if !ch.written() || partOf(ch.ID) != part {
				continue
			}
			base := "/chapters/" + ch.Slug + "/"
			items = append(items, fmt.Sprintf(
				"        { text: %s, link: %s, collapsed: false, items: [\n"+
					"          { text: 'Overview', link: %s },\n"+
					"          { text: 'Problem', link: %s },\n"+
					"          { text: 'Example', link: %s },\n"+
					"        ] },",
				strconv.Quote(ch.ID+" "+ch.Title), strconv.Quote(base),
				strconv.Quote(base+"overview"), strconv.Quote(base+"problem"), strconv.Quote(base+"example")))
		}
		if len(items) == 0 {
			continue
		}
		fmt.Fprintf(&b, "      { text: %s, collapsed: false, items: [\n%s\n      ] },\n",
			strconv.Quote(part), strings.Join(items, "\n"))
	}

	b.WriteString(`      { text: 'Reference', collapsed: false, items: [
        { text: 'Architecture', link: '/docs/architecture' },
        { text: 'Chapter format', link: '/docs/chapter-format' },
        { text: 'Writing style', link: '/docs/writing-style' },
        { text: 'Designs', link: '/docs/designs' },
        { text: 'Why VitePress', link: '/docs/adr/0001-vitepress-for-the-site' },
      ] },
`)
	b.WriteString("      { text: 'After the course', items: [{ text: 'Where next', link: '/chapters/where-next' }] },\n")
	b.WriteString("    ],\n")
	b.WriteString("    editLink: { pattern: 'https://github.com/theakhandpatel/kata/edit/main/:path' },\n")
	b.WriteString("    socialLinks: [{ icon: 'github', link: 'https://github.com/theakhandpatel/kata' }],\n")
	b.WriteString("  },\n")
	b.WriteString("  mermaid: {},\n")
	b.WriteString("}))\n")

	return b.String()
}

func ensureDependencies(siteDir string) error {
	if _, err := os.Stat(filepath.Join(siteDir, "node_modules", "vitepress")); err == nil {
		return nil
	}
	fmt.Println("  installing site dependencies (first run only)")
	cmd := exec.Command("npm", "install", "--no-audit", "--no-fund")
	cmd.Dir = siteDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("npm install: %w", err)
	}
	return nil
}

func build(siteDir string) error {
	cmd := exec.Command("npx", "vitepress", "build", "src")
	cmd.Dir = siteDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("vitepress build: %w", err)
	}
	return nil
}
