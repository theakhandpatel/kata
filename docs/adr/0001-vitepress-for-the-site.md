# 1. VitePress for the course website

**Status:** accepted, 2026-10-27

## Context

The course prose is Markdown in `chapters/`, and `kata read` shows the same files in a
terminal. The website needs to render that prose without becoming a second source of
truth.

The first version was a generator written in this repository: a Markdown parser, a
layout, a stylesheet, and a diagram pass, about 450 lines. It worked, and it was the
largest maintenance liability in the project. It supported a subset of Markdown, had no
search or heading anchors, and silently emitted broken cross-references.

Two off-the-shelf options were considered: VitePress (Node, used by the harness-
engineering course that informed this one) and Hugo (a single Go binary, no Node).

## Decision

VitePress, with `vitepress-plugin-mermaid`.

`kata site` no longer renders anything. It copies the chapter Markdown into a generated
VitePress project, writes the sidebar from the chapter directories, and shells out to
VitePress. The generated project (`site/src`) and the build output (`site/dist`) are not
committed; Netlify runs `go run ./cmd/kata site` and publishes the result.

## Why not Hugo

Hugo would remove the Node dependency and build in milliseconds. It would also mean:

- adopting a theme and its structure, where VitePress's default theme is already the
  documentation shape this course wants;
- writing a Mermaid shortcode or accepting a client-side include, where the VitePress
  plugin works today with no code;
- rebuilding the generator's front-matter config, and repeating this migration;
- losing local search and the outline, which are configuration in VitePress and theme
  features in Hugo.

None of that buys anything the course needs. Node is required to *publish*, and Netlify
provides it; taking the course still needs only Go.

## Consequences

- The repository contains no Markdown renderer, no theme CSS, and no layout code.
- Search, dark mode, heading anchors, nested navigation, syntax highlighting and
  diagrams come from the tool. Upgrades are a version bump.
- VitePress fails the build on a dead internal link. That already caught two links the
  hand-written generator had shipped.
- Publishing requires Node and npm. `kata site` installs the dependencies on first run.
- Reversing this is contained: the generator only copies files and writes a sidebar, so
  moving to Hugo means replacing `cmd/kata/site.go` and nothing else.
