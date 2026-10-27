// Reference implementation for chapter 09: the skill catalogue.
//
// Skills are discovered and advertised by name and description. Loading a body when
// one is invoked is chapter 10; this file keeps the advertisement side only.
package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrNoFrontmatter marks a SKILL.md whose header is missing. Such a file is
// skipped rather than fixed up: a skill with no name and no description cannot be
// advertised, so guessing would only hide the mistake.
var ErrNoFrontmatter = errors.New("missing frontmatter")

// Skill is one instruction file the model can invoke by name.
//
// The split between Description and Body is the whole point of the feature. The
// description travels in every request; the body travels only when the skill is
// used. A catalogue of fifty skills costs fifty lines, not fifty documents.
type Skill struct {
	Name        string
	Description string
	Body        string
	Path        string
}

// LoadSkills reads every `skills/<name>/SKILL.md` under root.
//
// A missing `skills` directory is not an error: most projects have none, and the
// agent has to work there.
func LoadSkills(root string) ([]Skill, error) {
	base := filepath.Join(root, "skills")
	entries, err := os.ReadDir(base)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var out []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(base, e.Name(), "SKILL.md")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		skill, err := ParseSkill(path, data)
		if err != nil {
			// A malformed file is skipped, not fatal: one bad skill must not stop
			// the agent from starting.
			continue
		}
		out = append(out, skill)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ParseSkill reads the frontmatter and body of one SKILL.md.
//
// The header is deliberately not YAML. It is `key: value` lines between `---`
// fences, which covers name and description and needs no dependency.
func ParseSkill(path string, data []byte) (Skill, error) {
	text := strings.TrimPrefix(string(data), "\ufeff")
	if !strings.HasPrefix(text, "---") {
		return Skill{}, ErrNoFrontmatter
	}
	rest := text[3:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return Skill{}, ErrNoFrontmatter
	}

	meta := map[string]string{}
	for _, line := range strings.Split(rest[:end], "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		meta[strings.ToLower(strings.TrimSpace(key))] = trimQuotes(strings.TrimSpace(value))
	}

	name := meta["name"]
	description := meta["description"]
	if name == "" || description == "" {
		return Skill{}, fmt.Errorf("%s: name and description are required", path)
	}

	body := strings.TrimLeft(rest[end+len("\n---"):], "\r\n")
	return Skill{Name: name, Description: description, Body: body, Path: path}, nil
}

func trimQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// Catalog holds the skills, advertises them, and expands invocations.
type Catalog struct {
	skills []Skill
	byName map[string]Skill
}

// NewCatalog indexes the skills by name. Later duplicates lose to earlier ones,
// because LoadSkills already sorts and the first definition wins is easier to
// explain than last-write-wins.
func NewCatalog(skills []Skill) *Catalog {
	c := &Catalog{skills: skills, byName: make(map[string]Skill, len(skills))}
	for _, s := range skills {
		if _, exists := c.byName[s.Name]; !exists {
			c.byName[s.Name] = s
		}
	}
	return c
}

// Len is the number of advertised skills.
func (c *Catalog) Len() int { return len(c.byName) }

// Contribute renders the catalogue for the system prompt.
//
// Names and descriptions only. Bodies are loaded when a skill is invoked, which is
// what keeps a large catalogue cheap.
func (c *Catalog) Contribute(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(c.skills) == 0 {
		return "", nil
	}

	var b strings.Builder
	b.WriteString("# Skills\n\n")
	b.WriteString("Invoke a skill by writing /name followed by its arguments.\n")
	for _, s := range c.skills {
		fmt.Fprintf(&b, "\n- %s: %s\n", s.Name, s.Description)
	}
	return b.String(), nil
}
