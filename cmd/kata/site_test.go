package main

import (
	"strings"
	"testing"
)

// Tests for the site renderer. The generated pages are the documentation a reader
// sees, so a rendering bug is a documentation bug that only shows up in a browser.

func TestRenderHeadingsAndParagraphs(t *testing.T) {
	got := renderMarkdown("# Title\n\nA paragraph with `code` and **bold**.\n")
	for _, want := range []string{
		"<h1>Title</h1>",
		"<p>A paragraph with <code>code</code> and <strong>bold</strong>.</p>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderTable(t *testing.T) {
	got := renderMarkdown("| Tool | Use |\n|---|---|\n| read | files |\n")
	for _, want := range []string{"<table>", "<th>Tool</th>", "<td>read</td>"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderMermaidStaysASourceBlock(t *testing.T) {
	got := renderMarkdown("```mermaid\nflowchart LR\n  a --> b\n```\n")
	if !strings.Contains(got, `<pre class="mermaid">`) {
		t.Fatalf("mermaid block not marked:\n%s", got)
	}
	// The source must survive for the offline case, when the renderer cannot load.
	if !strings.Contains(got, "flowchart LR") {
		t.Fatalf("mermaid source lost:\n%s", got)
	}
}

func TestRenderEscapesCode(t *testing.T) {
	got := renderMarkdown("```go\nif a < b && c > d {}\n```\n")
	if !strings.Contains(got, "a &lt; b &amp;&amp; c &gt; d") {
		t.Fatalf("code not escaped:\n%s", got)
	}
}

func TestRewriteLinks(t *testing.T) {
	cases := map[string]string{
		`<a href="overview.md">`:               `<a href="overview.html">`,
		`<a href="../where-next.md">`:          `<a href="../where-next.html">`,
		`<a href="chapters/11-mcp/README.md">`: `<a href="chapters/11-mcp/index.html">`,
		`<a href="https://example.com/x.md">`:  `<a href="https://example.com/x.html">`,
	}
	for in, want := range cases {
		if got := rewriteLinks(in); got != want {
			t.Fatalf("rewriteLinks(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRelativePrefixByDepth(t *testing.T) {
	// Root pages link to the stylesheet beside them; nested pages climb out.
	if got := strings.Repeat("../", strings.Count("index.html", "/")); got != "" {
		t.Fatalf("root prefix = %q", got)
	}
	if got := strings.Repeat("../", strings.Count("chapters/04-the-agent-loop/problem.html", "/")); got != "../../" {
		t.Fatalf("chapter prefix = %q", got)
	}
}
