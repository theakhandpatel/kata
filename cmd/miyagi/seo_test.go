package main

import (
	"strings"
	"testing"
)

// The site is generated, so its search metadata is a function of the generator. These
// tests pin the parts a crawler reads, because a missing description or a dropped sitemap
// is invisible in a browser and only shows up weeks later as no traffic.

func TestBuildConfigHasSearchMetadata(t *testing.T) {
	cfg := buildConfig("Miyagi", ".")

	required := []struct{ what, want string }{
		{"sitemap", "sitemap:"},
		{"clean urls", "cleanUrls: true"},
		{"canonical host", "hostname: 'https://miyagi.akhand.cc'"},
		{"open graph title", "og:title"},
		{"open graph image", "og:image"},
		{"open graph description", "og:description"},
		{"twitter card", "twitter:card"},
		{"robots directive", "index, follow"},
		{"theme colour", "theme-color"},
		{"favicon", "favicon.ico"},
		{"logo in nav", "logo-128.png"},
	}
	for _, r := range required {
		if !strings.Contains(cfg, r.want) {
			t.Errorf("site config is missing %s (%q)", r.what, r.want)
		}
	}
}

func TestFirstSentenceIsUsableAsADescription(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "skips headings and fences",
			in:   "# Title\n\n```sh\n./agent -p hi\n```\n\nThe program answers a question.\n",
			want: "The program answers a question.",
		},
		{
			name: "takes one sentence from a paragraph",
			in:   "The program answers a question. It reads flags first.\n",
			want: "The program answers a question.",
		},
		{
			name: "strips inline code and bold",
			in:   "Edit `agent/provider_openai.go` and **nothing** else.\n",
			want: "Edit agent/provider_openai.go and nothing else.",
		},
		{
			name: "skips the metadata line and tables",
			in:   "**Part I · Wire** · 9 graded cases\n\n| a | b |\n\nA program that answers.\n",
			want: "A program that answers.",
		},
		{
			name: "empty when there is only structure",
			in:   "# Title\n\n```\ncode\n```\n",
			want: "",
		},
	}
	for _, c := range cases {
		if got := firstSentence(c.in); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestLongDescriptionsAreTrimmedAtAWordBoundary(t *testing.T) {
	long := strings.Repeat("word ", 60)
	got := trimToSentence(long)
	if len(got) > 156 {
		t.Errorf("description is %d bytes, want at most 156", len(got))
	}
	if strings.HasSuffix(got, " wor") {
		t.Errorf("description was cut mid-word: %q", got)
	}
	if !strings.HasSuffix(got, ".") {
		t.Errorf("trimmed description should end in a full stop: %q", got)
	}
}

func TestExistingFrontMatterIsNotOverwritten(t *testing.T) {
	in := []byte("---\ndescription: mine\n---\n\nBody.\n")
	if got := string(withDescription(in, "x.md")); got != string(in) {
		t.Errorf("front matter was rewritten:\n%s", got)
	}
}
