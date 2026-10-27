package main

import (
	"strings"
	"testing"
)

// The site is generated, so these tests cover the generation: which files are copied,
// and whether the sidebar knows about every chapter. Rendering belongs to VitePress and
// is not tested here.

func TestConfigListsEveryChapter(t *testing.T) {
	root := repoRootForTests(t)
	config := buildConfig("Miyagi", root)

	for _, ch := range allChapters(root) {
		if !ch.written() {
			continue
		}
		if !strings.Contains(config, `/chapters/`+ch.Slug+`/`) {
			t.Errorf("sidebar is missing chapter %s (%s)", ch.ID, ch.Slug)
		}
		if !strings.Contains(config, `"/chapters/`+ch.Slug+`/example"`) {
			t.Errorf("sidebar is missing the worked example for chapter %s", ch.ID)
		}
	}
}

func TestConfigNamesTheParts(t *testing.T) {
	root := repoRootForTests(t)
	config := buildConfig("Miyagi", root)
	for _, part := range []string{"I. Wire", "II. Act", "III. Remember", "IV. Extend", "V. Compose"} {
		if !strings.Contains(config, `"`+part+`"`) {
			t.Errorf("sidebar is missing part %q", part)
		}
	}
}

func TestConfigIsValidShape(t *testing.T) {
	config := buildConfig("Miyagi", repoRootForTests(t))
	for _, want := range []string{
		"withMermaid(defineConfig({",
		"search: { provider: 'local' }",
		"mermaid: {},",
		"export default",
	} {
		if !strings.Contains(config, want) {
			t.Errorf("config is missing %q", want)
		}
	}
	// Unbalanced braces would produce a file VitePress cannot parse.
	if strings.Count(config, "{") != strings.Count(config, "}") {
		t.Errorf("config braces are unbalanced")
	}
}
