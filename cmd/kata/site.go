package main

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// cmdSite renders the course as a static website.
//
// The generator is in Go, with no dependencies, because the course ships one binary
// and adding Node to read the documentation would be a strange price. The output is
// plain HTML and one stylesheet: open index.html from the filesystem and it works.
//
// The pages are the same markdown the chapters use. There is no second copy of the
// content, so the site cannot drift from what `kata read` shows.
func cmdSite(args []string) error {
	root, err := findRoot()
	if err != nil {
		return err
	}

	out := "site/dist"
	title := "Kata"
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
		}
	}
	if !filepath.IsAbs(out) {
		out = filepath.Join(root, out)
	}
	if err := os.RemoveAll(out); err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}

	pages, err := sitePages(root)
	if err != nil {
		return err
	}
	if len(pages) == 0 {
		return fmt.Errorf("nothing to render")
	}

	nav := siteNav(root)

	for i, p := range pages {
		source := filepath.Join(root, p.Source)
		body, err := os.ReadFile(source)
		if err != nil {
			return fmt.Errorf("read %s: %w", p.Source, err)
		}

		content := renderMarkdown(string(body))
		content = rewriteLinks(content)

		var prev, next *sitePage
		if i > 0 {
			prev = &pages[i-1]
		}
		if i+1 < len(pages) {
			next = &pages[i+1]
		}

		// Every link is relative to the page, so opening index.html from the
		// filesystem works without a web server.
		page := layout(layoutInput{
			SiteTitle: title,
			Title:     p.Title,
			Prefix:    strings.Repeat("../", strings.Count(p.Out, "/")),
			Nav:       nav,
			Current:   p.Out,
			Content:   content,
			SubNav:    p.SubNav,
			Prev:      prev,
			Next:      next,
		})

		dest := filepath.Join(out, p.Out)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, []byte(page), 0o644); err != nil {
			return err
		}
	}

	if err := os.WriteFile(filepath.Join(out, "style.css"), []byte(siteCSS), 0o644); err != nil {
		return err
	}

	fmt.Printf("\n  wrote %d pages and one stylesheet to %s\n", len(pages), out)
	fmt.Printf("  open %s\n\n", filepath.Join(out, "index.html"))
	return nil
}

type sitePage struct {
	Source string // path relative to the project root
	Out    string // path relative to the output directory
	Title  string
	SubNav []navLink // the four documents of a chapter, when the page belongs to one
}

type navLink struct {
	Title string
	Href  string
}

type navGroup struct {
	Title string
	Items []navLink
}

func sitePages(root string) ([]sitePage, error) {
	var pages []sitePage

	pages = append(pages, sitePage{Source: "README.md", Out: "index.html", Title: "Kata"})

	for _, ch := range allChapters(root) {
		if !ch.written() {
			continue
		}
		dir := "chapters/" + ch.Slug
		sub := []navLink{
			{Title: "Overview", Href: dir + "/overview.html"},
			{Title: "Problem", Href: dir + "/problem.html"},
			{Title: "Example", Href: dir + "/example.html"},
		}
		// The chapter index is the README, so its sub-nav points at the other three.
		index := sitePage{
			Source: dir + "/README.md",
			Out:    dir + "/index.html",
			Title:  ch.ID + " " + ch.Title,
			SubNav: sub,
		}
		pages = append(pages, index)

		for _, doc := range []string{"overview", "problem", "example"} {
			source := filepath.Join(dir, doc+".md")
			if _, err := os.Stat(filepath.Join(root, source)); err != nil {
				continue
			}
			pages = append(pages, sitePage{
				Source: source,
				Out:    dir + "/" + doc + ".html",
				Title:  ch.ID + " " + ch.Title + " · " + titleCase(doc),
				SubNav: sub,
			})
		}
	}

	for _, doc := range []struct{ source, out, title string }{
		{"docs/architecture.md", "docs/architecture.html", "Architecture"},
		{"docs/chapter-format.md", "docs/chapter-format.html", "Chapter format"},
		{"docs/writing-style.md", "docs/writing-style.html", "Writing style"},
		{"chapters/where-next.md", "chapters/where-next.html", "Where next"},
	} {
		if _, err := os.Stat(filepath.Join(root, doc.source)); err != nil {
			continue
		}
		pages = append(pages, sitePage{Source: doc.source, Out: doc.out, Title: doc.title})
	}

	return pages, nil
}

func siteNav(root string) []navGroup {
	groups := []navGroup{{Title: "Start", Items: []navLink{{Title: "Home", Href: "index.html"}}}}

	for _, part := range []string{"I. Wire", "II. Act", "III. Remember", "IV. Extend", "V. Compose"} {
		var items []navLink
		for _, ch := range allChapters(root) {
			if !ch.written() || partOf(ch.ID) != part {
				continue
			}
			items = append(items, navLink{
				Title: ch.ID + " " + ch.Title,
				Href:  "chapters/" + ch.Slug + "/index.html",
			})
		}
		if len(items) > 0 {
			groups = append(groups, navGroup{Title: part, Items: items})
		}
	}

	groups = append(groups,
		navGroup{Title: "Reference", Items: []navLink{
			{Title: "Architecture", Href: "docs/architecture.html"},
			{Title: "Chapter format", Href: "docs/chapter-format.html"},
			{Title: "Writing style", Href: "docs/writing-style.html"},
		}},
		navGroup{Title: "After the course", Items: []navLink{
			{Title: "Where next", Href: "chapters/where-next.html"},
		}},
	)
	return groups
}

type layoutInput struct {
	SiteTitle string
	Title     string
	Prefix    string // "../" repeated for the page's depth
	Nav       []navGroup
	Current   string
	Content   string
	SubNav    []navLink
	Prev      *sitePage
	Next      *sitePage
}

func layout(in layoutInput) string {
	var b strings.Builder

	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n")
	b.WriteString("<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	if in.Title == in.SiteTitle {
		fmt.Fprintf(&b, "<title>%s</title>\n", html.EscapeString(in.Title))
	} else {
		fmt.Fprintf(&b, "<title>%s · %s</title>\n", html.EscapeString(in.Title), html.EscapeString(in.SiteTitle))
	}
	fmt.Fprintf(&b, "<link rel=\"stylesheet\" href=\"%sstyle.css\">\n", in.Prefix)
	// Mermaid renders diagrams in the browser. If it cannot load, the source stays
	// visible as a code block rather than leaving an empty box.
	b.WriteString("<script type=\"module\">\ntry {\n")
	b.WriteString("  const m = (await import('https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.esm.min.mjs')).default;\n")
	b.WriteString("  m.initialize({ startOnLoad: false, theme: 'neutral' });\n")
	b.WriteString("  await m.run({ nodes: Array.from(document.querySelectorAll('pre.mermaid')) });\n")
	b.WriteString("} catch (e) { /* offline: the diagram source stays readable */ }\n")
	b.WriteString("</script>\n")
	b.WriteString("</head>\n<body>\n")

	b.WriteString("<aside class=\"nav\">\n")
	fmt.Fprintf(&b, "<a class=\"brand\" href=\"%sindex.html\">%s</a>\n", in.Prefix, html.EscapeString(in.SiteTitle))
	for _, g := range in.Nav {
		b.WriteString("<div class=\"group\">\n")
		fmt.Fprintf(&b, "<div class=\"group-title\">%s</div>\n", html.EscapeString(g.Title))
		b.WriteString("<ul>\n")
		for _, item := range g.Items {
			class := ""
			if item.Href == in.Current {
				class = ` class="current"`
			}
			fmt.Fprintf(&b, "<li><a%s href=\"%s%s\">%s</a></li>\n",
				class, in.Prefix, item.Href, html.EscapeString(item.Title))
		}
		b.WriteString("</ul>\n</div>\n")
	}
	b.WriteString("</aside>\n")

	b.WriteString("<main>\n")
	if len(in.SubNav) > 0 {
		b.WriteString("<nav class=\"subnav\">\n")
		for _, l := range in.SubNav {
			class := ""
			if l.Href == in.Current {
				class = ` class="current"`
			}
			fmt.Fprintf(&b, "<a%s href=\"%s%s\">%s</a>\n", class, in.Prefix, l.Href, html.EscapeString(l.Title))
		}
		b.WriteString("</nav>\n")
	}

	b.WriteString(in.Content)

	b.WriteString("<nav class=\"pager\">\n")
	if in.Prev != nil {
		fmt.Fprintf(&b, "<a class=\"prev\" href=\"%s%s\">&larr; %s</a>\n",
			in.Prefix, in.Prev.Out, html.EscapeString(in.Prev.Title))
	} else {
		b.WriteString("<span></span>")
	}
	if in.Next != nil {
		fmt.Fprintf(&b, "<a class=\"next\" href=\"%s%s\">%s &rarr;</a>\n",
			in.Prefix, in.Next.Out, html.EscapeString(in.Next.Title))
	}
	b.WriteString("</nav>\n</main>\n</body>\n</html>\n")

	return b.String()
}

// --- markdown ---

var (
	headingRe   = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	ulItemRe    = regexp.MustCompile(`^(\s*)-\s+(.*)$`)
	olItemRe    = regexp.MustCompile(`^(\s*)(\d+)\.\s+(.*)$`)
	tableSepRe  = regexp.MustCompile(`^\|?[\s:-]*-[\s:|-]*\|?$`)
	codeFenceRe = regexp.MustCompile("^```(.*)$")
	hrRe        = regexp.MustCompile(`^(-{3,}|\*{3,})\s*$`)
)

// renderMarkdown turns one document into HTML.
//
// It supports the subset the course uses: headings, paragraphs, lists, tables,
// fenced code, blockquotes, rules, and inline code, bold, italic and links. That
// subset is small on purpose; a full implementation would be a dependency.
func renderMarkdown(src string) string {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")

	var (
		out     strings.Builder
		i       int
		inList  bool
		listTag string
	)

	closeList := func() {
		if inList {
			fmt.Fprintf(&out, "</%s>\n", listTag)
			inList = false
		}
	}

	for i < len(lines) {
		line := lines[i]

		// Fenced code.
		if m := codeFenceRe.FindStringSubmatch(line); m != nil {
			closeList()
			lang := strings.TrimSpace(m[1])
			i++
			var body []string
			for i < len(lines) && !strings.HasPrefix(lines[i], "```") {
				body = append(body, lines[i])
				i++
			}
			i++ // closing fence
			code := html.EscapeString(strings.Join(body, "\n"))
			if lang == "mermaid" {
				fmt.Fprintf(&out, "<pre class=\"mermaid\">%s</pre>\n", code)
			} else if lang == "" {
				fmt.Fprintf(&out, "<pre><code>%s</code></pre>\n", code)
			} else {
				fmt.Fprintf(&out, "<pre><code class=\"language-%s\">%s</code></pre>\n",
					html.EscapeString(lang), code)
			}
			continue
		}

		// Headings.
		if m := headingRe.FindStringSubmatch(line); m != nil {
			closeList()
			level := len(m[1])
			fmt.Fprintf(&out, "<h%d>%s</h%d>\n", level, inline(m[2]), level)
			i++
			continue
		}

		// Horizontal rule.
		if hrRe.MatchString(line) {
			closeList()
			out.WriteString("<hr>\n")
			i++
			continue
		}

		// Table: a header row followed by a separator row.
		if strings.HasPrefix(strings.TrimSpace(line), "|") && i+1 < len(lines) && tableSepRe.MatchString(strings.TrimSpace(lines[i+1])) {
			closeList()
			header := splitRow(line)
			i += 2
			out.WriteString("<table>\n<thead><tr>")
			for _, c := range header {
				fmt.Fprintf(&out, "<th>%s</th>", inline(c))
			}
			out.WriteString("</tr></thead>\n<tbody>\n")
			for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
				out.WriteString("<tr>")
				for _, c := range splitRow(lines[i]) {
					fmt.Fprintf(&out, "<td>%s</td>", inline(c))
				}
				out.WriteString("</tr>\n")
				i++
			}
			out.WriteString("</tbody>\n</table>\n")
			continue
		}

		// Blockquote.
		if strings.HasPrefix(strings.TrimSpace(line), ">") {
			closeList()
			var body []string
			for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), ">") {
				body = append(body, strings.TrimPrefix(strings.TrimSpace(lines[i]), ">"))
				i++
			}
			fmt.Fprintf(&out, "<blockquote><p>%s</p></blockquote>\n", inline(strings.TrimSpace(strings.Join(body, " "))))
			continue
		}

		// Lists, flat or one level deep.
		if m := ulItemRe.FindStringSubmatch(line); m != nil {
			if !inList || listTag != "ul" {
				closeList()
				out.WriteString("<ul>\n")
				inList, listTag = true, "ul"
			}
			class := ""
			if len(m[1]) >= 2 {
				class = ` class="nested"`
			}
			fmt.Fprintf(&out, "<li%s>%s</li>\n", class, inline(m[2]))
			i++
			continue
		}
		if m := olItemRe.FindStringSubmatch(line); m != nil {
			if !inList || listTag != "ol" {
				closeList()
				out.WriteString("<ol>\n")
				inList, listTag = true, "ol"
			}
			fmt.Fprintf(&out, "<li>%s</li>\n", inline(m[3]))
			i++
			continue
		}

		// Blank line ends a list and separates paragraphs.
		if strings.TrimSpace(line) == "" {
			closeList()
			i++
			continue
		}

		// Paragraph.
		closeList()
		var para []string
		for i < len(lines) && strings.TrimSpace(lines[i]) != "" && !startsBlock(lines[i]) {
			para = append(para, strings.TrimSpace(lines[i]))
			i++
		}
		if len(para) > 0 {
			fmt.Fprintf(&out, "<p>%s</p>\n", inline(strings.Join(para, " ")))
		}
	}
	closeList()
	return out.String()
}

func startsBlock(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "```") || headingRe.MatchString(t) ||
		strings.HasPrefix(t, "|") || strings.HasPrefix(t, ">") ||
		ulItemRe.MatchString(line) || olItemRe.MatchString(line) || hrRe.MatchString(t)
}

func splitRow(line string) []string {
	t := strings.TrimSpace(line)
	t = strings.TrimPrefix(t, "|")
	t = strings.TrimSuffix(t, "|")
	parts := strings.Split(t, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

var (
	codeSpanRe = regexp.MustCompile("`([^`]+)`")
	linkRe     = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	boldRe     = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	italicRe   = regexp.MustCompile(`\*([^*]+)\*`)
)

// inline renders the inline markup, escaping everything else.
//
// Code spans are lifted out first, so backticks protect their contents from the
// emphasis and link passes.
func inline(s string) string {
	var spans []string
	s = codeSpanRe.ReplaceAllStringFunc(s, func(m string) string {
		spans = append(spans, m[1:len(m)-1])
		return fmt.Sprintf("\x00%d\x00", len(spans)-1)
	})

	s = html.EscapeString(s)
	s = boldRe.ReplaceAllString(s, "<strong>$1</strong>")
	s = italicRe.ReplaceAllString(s, "<em>$1</em>")
	s = linkRe.ReplaceAllString(s, `<a href="$2">$1</a>`)

	for i, span := range spans {
		s = strings.ReplaceAll(s, fmt.Sprintf("\x00%d\x00", i), "<code>"+html.EscapeString(span)+"</code>")
	}
	return s
}

var mdLinkRe = regexp.MustCompile(`href="([^"]+)\.md"`)

// rewriteLinks points internal links at the generated pages.
func rewriteLinks(s string) string {
	return mdLinkRe.ReplaceAllStringFunc(s, func(m string) string {
		href := m[len(`href="`) : len(m)-len(`.md"`)]
		if strings.Contains(href, "README") {
			return `href="` + strings.ReplaceAll(href, "README", "index") + `.html"`
		}
		return `href="` + href + `.html"`
	})
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

const siteCSS = `:root {
  --bg: #ffffff;
  --fg: #1b1f23;
  --muted: #57606a;
  --line: #d8dee4;
  --soft: #f6f8fa;
  --accent: #0b5cad;
  --code-bg: #f6f8fa;
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #0f1115;
    --fg: #e6edf3;
    --muted: #9aa4ae;
    --line: #2a2f36;
    --soft: #161b22;
    --accent: #6cb6ff;
    --code-bg: #161b22;
  }
}
* { box-sizing: border-box; }
html { background: var(--bg); }
body {
  margin: 0;
  color: var(--fg);
  background: var(--bg);
  font: 16px/1.65 ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, sans-serif;
  display: grid;
  grid-template-columns: 17rem minmax(0, 1fr);
}
.nav {
  border-right: 1px solid var(--line);
  padding: 1.5rem 1rem;
  height: 100vh;
  position: sticky;
  top: 0;
  overflow-y: auto;
  font-size: 0.9rem;
}
.brand {
  display: block;
  font-weight: 700;
  font-size: 1.15rem;
  color: var(--fg);
  text-decoration: none;
  margin-bottom: 1.5rem;
}
.group { margin-bottom: 1.25rem; }
.group-title {
  color: var(--muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
  font-size: 0.72rem;
  margin-bottom: 0.35rem;
}
.nav ul { list-style: none; margin: 0; padding: 0; }
.nav li { margin: 0.1rem 0; }
.nav a {
  color: var(--fg);
  text-decoration: none;
  display: block;
  padding: 0.15rem 0.4rem;
  border-radius: 4px;
}
.nav a:hover { background: var(--soft); }
.nav a.current { background: var(--soft); color: var(--accent); font-weight: 600; }
main { padding: 2.5rem 2rem 5rem; max-width: 46rem; width: 100%; }
h1, h2, h3, h4 { line-height: 1.25; margin: 2rem 0 0.6rem; }
h1 { font-size: 1.85rem; margin-top: 0; }
h2 { font-size: 1.35rem; border-top: 1px solid var(--line); padding-top: 1.2rem; }
h3 { font-size: 1.08rem; }
p, li { max-width: 68ch; }
a { color: var(--accent); }
code {
  background: var(--code-bg);
  padding: 0.12em 0.35em;
  border-radius: 4px;
  font-size: 0.88em;
  font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, monospace;
}
pre {
  background: var(--code-bg);
  border: 1px solid var(--line);
  border-radius: 6px;
  padding: 0.9rem 1rem;
  overflow-x: auto;
}
pre code { background: none; padding: 0; font-size: 0.86rem; }
pre.mermaid { background: var(--soft); text-align: center; }
blockquote {
  margin: 1.2rem 0;
  padding: 0.2rem 1rem;
  border-left: 3px solid var(--line);
  color: var(--muted);
}
table { border-collapse: collapse; margin: 1.2rem 0; width: 100%; font-size: 0.92rem; }
th, td { border: 1px solid var(--line); padding: 0.4rem 0.6rem; text-align: left; vertical-align: top; }
th { background: var(--soft); }
hr { border: 0; border-top: 1px solid var(--line); margin: 2rem 0; }
ul, ol { padding-left: 1.3rem; }
li.nested { margin-left: 1.2rem; }
.subnav {
  display: flex;
  gap: 0.4rem;
  margin-bottom: 1.5rem;
  font-size: 0.88rem;
}
.subnav a {
  padding: 0.25rem 0.6rem;
  border: 1px solid var(--line);
  border-radius: 999px;
  text-decoration: none;
  color: var(--muted);
}
.subnav a.current { border-color: var(--accent); color: var(--accent); }
.pager {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
  margin-top: 3rem;
  padding-top: 1.2rem;
  border-top: 1px solid var(--line);
  font-size: 0.9rem;
}
@media (max-width: 60rem) {
  body { grid-template-columns: 1fr; }
  .nav { position: static; height: auto; border-right: 0; border-bottom: 1px solid var(--line); }
  main { padding: 1.5rem 1.1rem 4rem; }
}
`
