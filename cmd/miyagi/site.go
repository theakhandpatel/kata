package main

import (
	"fmt"
	"os"
	"io/fs"
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
	title := "Miyagi"
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
// Paths are preserved, so every relative link written for `miyagi read` and GitHub also
// resolves here. Only `README.md` is renamed, because VitePress wants `index.md` for a
// directory's landing page.
func prepareContent(root, srcDir string) (int, error) {
	pages := 0

	copyPage := func(source, target string) error {
		body, err := os.ReadFile(filepath.Join(root, source))
		if err != nil {
			return fmt.Errorf("read %s: %w", source, err)
		}
		body = withDescription(body, source)
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
		"docs/writing-style.md", "docs/designs.md", "docs/design-audit.md",
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

	// Images under docs/assets are copied so the same relative paths resolve on the
	// site as they do on GitHub. Chapter pages and the README reference them by that
	// path, so the two renderers must agree about where they live.
	if err := copyAssets(root, srcDir); err != nil {
		return pages, err
	}

	return pages, nil
}

// copyAssets mirrors docs/assets into the VitePress public directory.
//
// Files under `public` are served verbatim at the site root and are never passed through
// Rollup, which matters here: a relative image path in markdown is otherwise treated as a
// module import and fails to resolve. After this copy, `/docs/assets/logo.png` works on
// the site, and the same file is reached relatively on GitHub.
func copyAssets(root, srcDir string) error {
	src := filepath.Join(root, "docs", "assets")
	if _, err := os.Stat(src); err != nil {
		return nil
	}
	publicDir := filepath.Join(srcDir, "public", "docs", "assets")
	if err := os.MkdirAll(publicDir, 0o755); err != nil {
		return err
	}
	if err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		dest := filepath.Join(publicDir, d.Name())
		return os.WriteFile(dest, body, 0o644)
	}); err != nil {
		return err
	}

	// robots.txt points crawlers at the sitemap VitePress writes, so a new chapter is
	// discoverable without anyone submitting a URL by hand.
	robots := "User-agent: *\nAllow: /\n\nSitemap: https://miyagi.akhand.cc/sitemap.xml\n"
	return os.WriteFile(filepath.Join(srcDir, "public", "robots.txt"), []byte(robots), 0o644)
}

func writeProject(srcDir, title, root string) error {
	pkg := `{
  "name": "miyagi-site",
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
	if err := os.WriteFile(filepath.Join(dir, "config.mts"), []byte(config), 0o644); err != nil {
		return err
	}

	// The theme directory holds the stylesheet and the logo. VitePress picks up
	// theme/index.ts by convention when a theme directory exists.
	themeDir := filepath.Join(dir, "theme")
	if err := os.MkdirAll(themeDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(themeDir, "index.ts"), []byte(themeEntry), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(themeDir, "custom.css"), []byte(themeCSS), 0o644)
}

// themeEntry pulls in the default theme and then our stylesheet, which is the whole
// extent of the customisation: colours, the logo lockup, and a wider content column for
// pages that carry wide tables.
const themeEntry = `import DefaultTheme from 'vitepress/theme'
import './custom.css'

export default DefaultTheme
`

// themeCSS is deliberately small. It sets a warm palette drawn from the logo, tightens
// the hero, and stops the prose column from stretching to an unreadable width on a
// large monitor. Everything else is VitePress defaults, because a course site should
// look like documentation, not like a landing page.
const themeCSS = `/**
 * Miyagi site theme.
 *
 * One accent colour, taken from the logo, plus a readable measure. The default
 * VitePress theme is already good documentation design, so this file changes as
 * little as possible.
 */

:root {
  --vp-c-brand-1: #9a4b28;
  --vp-c-brand-2: #b85c33;
  --vp-c-brand-3: #d4754a;
  --vp-c-brand-soft: rgba(154, 75, 40, 0.14);

  /* A warm off-white rather than pure white, which is easier to read for long prose. */
  --vp-c-bg: #fdfbf9;
  --vp-c-bg-alt: #f6f1ec;
  --vp-c-bg-soft: #f3ece5;

  --vp-font-family-base: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI',
    Roboto, 'Helvetica Neue', sans-serif;
  --vp-font-family-mono: 'JetBrains Mono', ui-monospace, 'SF Mono', Menlo, monospace;
}

.dark {
  --vp-c-brand-1: #e08a5c;
  --vp-c-brand-2: #d4754a;
  --vp-c-brand-3: #c2603a;
  --vp-c-brand-soft: rgba(224, 138, 92, 0.16);

  --vp-c-bg: #16120f;
  --vp-c-bg-alt: #1d1815;
  --vp-c-bg-soft: #241d18;
}

/* The hero: logo and title side by side, tagline underneath. */
.VPHero .name,
.VPHero .text {
  letter-spacing: -0.02em;
}

.VPHero .tagline {
  max-width: 34rem;
  margin-inline: auto;
}

/* Long-form pages carry wide tables and code; give them room, but stop before the
   line becomes hard to track. */
@media (min-width: 960px) {
  .VPDoc .content-container {
    max-width: 46rem;
  }
}

/* Chapter pages open with a metadata line in bold, which reads as a caption rather
   than a heading. */
.vp-doc > p:first-of-type strong:first-child {
  font-weight: 600;
}

/* Inline code should not shout. */
.vp-doc :not(pre) > code {
  font-size: 0.86em;
}

/* The course leans on tables; give them a little more breathing room. */
.vp-doc table {
  display: table;
  width: 100%;
}

.vp-doc tr:nth-child(2n) {
  background: var(--vp-c-bg-soft);
}
`

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
	// The logo and favicon are copied into the site source from docs/assets, so the
	// repository and the published site use one source of art.
	//
	// `cleanUrls` gives every page a path without `.html`, which is what search engines
	// and shared links should see.
	b.WriteString("  cleanUrls: true,\n")
	b.WriteString("  sitemap: { hostname: 'https://miyagi.akhand.cc' },\n")
	b.WriteString("  head: [\n")
	b.WriteString("    ['link', { rel: 'icon', href: '/docs/assets/favicon.ico', sizes: 'any' }],\n")
	b.WriteString("    ['link', { rel: 'apple-touch-icon', href: '/docs/assets/logo-128.png' }],\n")
	b.WriteString("    ['meta', { name: 'theme-color', content: '#9a4b28' }],\n")
	b.WriteString("    ['meta', { name: 'author', content: 'Akhand Patel' }],\n")
	b.WriteString("    ['meta', { name: 'robots', content: 'index, follow' }],\n")
	// The share card. Absolute URLs are required by every crawler.
	b.WriteString("    ['meta', { property: 'og:type', content: 'website' }],\n")
	b.WriteString("    ['meta', { property: 'og:site_name', content: 'Miyagi' }],\n")
	b.WriteString("    ['meta', { property: 'og:title', content: 'Miyagi: build a coding-agent harness in Go' }],\n")
	b.WriteString("    ['meta', { property: 'og:description', content: 'Twenty-one graded chapters that take you from one HTTP request to a working agent. Offline, deterministic, no API key.' }],\n")
	b.WriteString("    ['meta', { property: 'og:image', content: 'https://miyagi.akhand.cc/docs/assets/logo.png' }],\n")
	b.WriteString("    ['meta', { property: 'og:url', content: 'https://miyagi.akhand.cc/' }],\n")
	b.WriteString("    ['meta', { name: 'twitter:card', content: 'summary_large_image' }],\n")
	b.WriteString("    ['meta', { name: 'twitter:title', content: 'Miyagi: build a coding-agent harness in Go' }],\n")
	b.WriteString("    ['meta', { name: 'twitter:description', content: 'Twenty-one graded chapters that take you from one HTTP request to a working agent. Offline, deterministic, no API key.' }],\n")
	b.WriteString("    ['meta', { name: 'twitter:image', content: 'https://miyagi.akhand.cc/docs/assets/logo.png' }],\n")
	b.WriteString("  ],\n")
	b.WriteString("  themeConfig: {\n")
	b.WriteString("    logo: { src: '/docs/assets/logo-128.png', alt: 'Miyagi', width: 32, height: 32 },\n")
	b.WriteString("    search: { provider: 'local' },\n")
	b.WriteString("    outline: { level: [2, 3] },\n")
	b.WriteString("    nav: [\n")
	b.WriteString("      { text: 'Home', link: '/' },\n")
	if first := firstChapterLink(root); first != "" {
		fmt.Fprintf(&b, "      { text: 'Chapters', link: %s },\n", strconv.Quote(first))
	}
	b.WriteString("      { text: 'Designs', link: '/docs/designs' },\n")
	b.WriteString("      { text: 'GitHub', link: 'https://github.com/theakhandpatel/miyagi' },\n")
	b.WriteString("    ],\n")
	b.WriteString("    sidebar: [\n")
	b.WriteString("      { text: 'Start', items: [{ text: 'What Miyagi is', link: '/' }] },\n")

	for _, part := range partTitles() {
		var items []string
		for _, ch := range allChapters(root) {
			if !ch.written() || partOf(ch.ID) != part {
				continue
			}
			base := "/chapters/" + ch.Slug + "/"
			items = append(items, fmt.Sprintf(
				"        { text: %s, link: %s, collapsed: false, items: [\n"+
					"          { text: 'The chapter', link: %s },\n"+
					"          { text: 'Worked example', link: %s },\n"+
					"        ] },",
				strconv.Quote(ch.ID+" "+ch.Title), strconv.Quote(base),
				strconv.Quote(base), strconv.Quote(base+"example")))
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
        { text: 'Design audit', link: '/docs/design-audit' },
        { text: 'Why VitePress', link: '/docs/adr/0001-vitepress-for-the-site' },
      ] },
`)
	b.WriteString("      { text: 'After the course', items: [{ text: 'Where next', link: '/chapters/where-next' }] },\n")
	b.WriteString("    ],\n")
	b.WriteString("    // No editLink: the generated site renames README.md to index.md, so a link\n")
	b.WriteString("    // derived from the page path points at a file the repository does not have.\n")
	b.WriteString("    socialLinks: [{ icon: 'github', link: 'https://github.com/theakhandpatel/miyagi' }],\n")
	b.WriteString("  },\n")
	b.WriteString("  mermaid: {},\n")
	b.WriteString("}))\n")

	return b.String()
}

// firstChapterLink points the top navigation at whichever chapter is first on disk.
// partTitles returns the part headings in order, from the same table the chapter list
// uses, so a new part appears on the site without editing this file.
func partTitles() []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, p.Title)
	}
	return out
}

func firstChapterLink(root string) string {
	all := allChapters(root)
	if len(all) == 0 {
		return ""
	}
	return "/chapters/" + all[0].Slug + "/"
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

// withDescription gives every page its own meta description.
//
// VitePress uses one site-wide description for the `description` tag unless a page
// declares its own front matter. A course with 21 similarly named chapters then presents
// search engines with 21 identical snippets, which is the difference between "22 results
// that all look the same" and a result a reader can choose between. The chapter's own
// opening sentence is already written for a human, so it is used verbatim, trimmed to a
// length crawlers display.
//
// A page that already has front matter is left alone: the author said what they wanted.
func withDescription(body []byte, source string) []byte {
	text := string(body)
	if strings.HasPrefix(text, "---\n") {
		return body
	}

	desc := firstSentence(text)
	if desc == "" {
		return body
	}

	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "description: %s\n", strconv.Quote(desc))
	b.WriteString("---\n\n")
	b.WriteString(text)
	return []byte(b.String())
}

// firstSentence finds the first real paragraph that is not a heading, code fence, table,
// image or HTML block, and returns its first sentence, capped at about 155 characters
// because that is all a search result shows.
func firstSentence(text string) string {
	lines := strings.Split(text, "\n")
	inFence := false
	var para []string

	flush := func() string {
		if len(para) == 0 {
			return ""
		}
		joined := strings.Join(para, " ")
		para = nil
		// Cut at the first sentence end.
		for i, r := range joined {
			if (r == '.' || r == '!' || r == '?') && i+1 < len(joined) {
				if joined[i+1] == ' ' {
					return trimToSentence(joined[:i+1])
				}
			}
		}
		return trimToSentence(joined)
	}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		switch {
		case line == "":
			if s := flush(); s != "" {
				return s
			}
			continue
		case strings.HasPrefix(line, "#"),
			strings.HasPrefix(line, "|"),
			strings.HasPrefix(line, ">"),
			strings.HasPrefix(line, "<"),
			strings.HasPrefix(line, "!["),
			strings.HasPrefix(line, "-"),
			strings.HasPrefix(line, "*"),
			strings.HasPrefix(line, "1."),
			strings.HasPrefix(line, "**Part"):
			continue
		}
		// Strip inline markup a crawler would otherwise show literally.
		line = strings.ReplaceAll(line, "`", "")
		line = strings.ReplaceAll(line, "**", "")
		para = append(para, line)
	}
	return flush()
}

// trimToSentence caps a description without cutting mid-word.
func trimToSentence(s string) string {
	const max = 155
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	if i := strings.LastIndexByte(cut, ' '); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:") + "."
}
