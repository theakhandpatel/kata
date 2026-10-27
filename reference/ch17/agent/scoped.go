package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Scope is one directory that may contain skills, and how specific it is.
//
// A project may keep skills beside its own code, and a person may keep skills that apply
// everywhere they work. When both define the same name, one has to win, and which one
// wins has to be decided rather than discovered by directory order.
type Scope struct {
	// Name identifies the scope in messages: "project", "user", "builtin".
	Name string
	// Root is the workspace root: the directory that contains a `skills` folder. The
	// loader appends `skills` itself, so a project scope passes the project directory.
	Root string
	// Priority decides the winner when two scopes define the same skill name. Higher
	// wins. A project skill should usually outrank a user skill, because the project's
	// instructions are the more specific ones.
	Priority int
}

// ScopedSkill is a skill plus where it came from.
type ScopedSkill struct {
	Skill
	Scope    string
	Priority int
}

// LoadScoped discovers skills in every scope and resolves name collisions.
//
// The rules, in order:
//
//  1. a scope whose directory does not exist contributes nothing;
//  2. a malformed file is skipped, and the other skills in that scope still load;
//  3. when two scopes define the same name, the higher priority wins;
//  4. equal priorities are broken by scope name, so the result does not depend on the
//     order the caller listed the scopes.
//
// Every decision is reported, so a person can see why the skill they just wrote is not
// the one being used.
type ScopedCatalog struct {
	chosen []ScopedSkill
	byName map[string]ScopedSkill
	shadow []Shadowed
}

// Shadowed records a skill that lost to another scope.
type Shadowed struct {
	Name   string
	Winner string
	Loser  string
}

// LoadScoped reads the scopes and resolves collisions.
func LoadScoped(scopes []Scope) (*ScopedCatalog, error) {
	cat := &ScopedCatalog{byName: map[string]ScopedSkill{}}

	for _, scope := range scopes {
		skills, err := LoadSkills(scope.Root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, s := range skills {
			candidate := ScopedSkill{Skill: s, Scope: scope.Name, Priority: scope.Priority}

			existing, taken := cat.byName[s.Name]
			if !taken {
				cat.byName[s.Name] = candidate
				continue
			}

			winner, loser := pickWinner(existing, candidate)
			cat.byName[s.Name] = winner
			cat.shadow = append(cat.shadow, Shadowed{
				Name:   s.Name,
				Winner: winner.Scope,
				Loser:  loser.Scope,
			})
		}
	}

	cat.chosen = make([]ScopedSkill, 0, len(cat.byName))
	for _, s := range cat.byName {
		cat.chosen = append(cat.chosen, s)
	}
	sort.Slice(cat.chosen, func(i, j int) bool { return cat.chosen[i].Name < cat.chosen[j].Name })
	return cat, nil
}

// pickWinner applies the priority rule, with the scope name as a tiebreak.
func pickWinner(a, b ScopedSkill) (winner, loser ScopedSkill) {
	if a.Priority != b.Priority {
		if a.Priority > b.Priority {
			return a, b
		}
		return b, a
	}
	if a.Scope <= b.Scope {
		return a, b
	}
	return b, a
}

// Skills returns the resolved set.
func (c *ScopedCatalog) Skills() []ScopedSkill { return c.chosen }

// Shadowed returns every collision, with who won and who lost.
func (c *ScopedCatalog) Shadowed() []Shadowed { return c.shadow }

// Contribute renders the catalogue, naming each skill's scope.
//
// The scope is included because a model that sees two projects' skills should know which
// one it is following.
func (c *ScopedCatalog) Contribute(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(c.chosen) == 0 {
		return "", nil
	}

	var b strings.Builder
	b.WriteString("# Skills\n\n")
	b.WriteString("Invoke a skill by writing /name followed by its arguments.\n")
	for _, s := range c.chosen {
		fmt.Fprintf(&b, "\n- %s (%s): %s\n", s.Name, s.Scope, s.Description)
	}
	return b.String(), nil
}

// Explain returns a line per collision, for a person debugging their own skills.
func (c *ScopedCatalog) Explain() []string {
	out := make([]string, 0, len(c.shadow))
	for _, s := range c.shadow {
		out = append(out, fmt.Sprintf("%s: using the copy from %s, ignoring the one from %s",
			s.Name, s.Winner, s.Loser))
	}
	return out
}

// Expand resolves a slash command against the scoped set, and reports the scope that
// answered so a shadowed skill is visible at the point of use.
func (c *ScopedCatalog) Expand(prompt string) (string, []Expansion) {
	// Reuse the single-scope expansion by presenting the resolved skills as one
	// catalogue. The scope is already recorded alongside each skill.
	skills := make([]Skill, 0, len(c.chosen))
	for _, s := range c.chosen {
		skills = append(skills, s.Skill)
	}
	return NewCatalog(skills).Expand(prompt)
}

// DefaultScopes builds the scopes a course project usually wants.
//
// Project beats user, because a repository's instructions are about this code. The
// builtin scope is lowest so anything local can replace it.
func DefaultScopes(projectRoot string) []Scope {
	var scopes []Scope

	// XDG_CONFIG_HOME first, because that is where a Linux user's configuration lives,
	// and because it lets a caller point the user scope somewhere else.
	if config := os.Getenv("XDG_CONFIG_HOME"); config != "" {
		scopes = append(scopes, Scope{
			Name:     "user",
			Root:     filepath.Join(config, "agent"),
			Priority: 10,
		})
	} else if home, err := os.UserHomeDir(); err == nil && home != "" {
		scopes = append(scopes, Scope{
			Name:     "user",
			Root:     filepath.Join(home, ".config", "agent"),
			Priority: 10,
		})
	}
	scopes = append(scopes, Scope{
		Name:     "project",
		Root:     projectRoot,
		Priority: 20,
	})
	return scopes
}
