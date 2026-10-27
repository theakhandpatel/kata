package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"agent/internal/grader"
)

// cmdUI opens a terminal interface for the course.
//
// The screen is a chapter list and a panel, and the only action that matters is
// "grade this chapter". Everything it shows comes from the same functions the plain
// commands use, so the interface cannot disagree with `kata list` or `kata submit`.
func cmdUI(args []string) error {
	// A full-screen interface needs a terminal on both ends. Without one, Bubble Tea waits
	// for events that never come, and a script or a pipe hangs forever with no message.
	// Say so and exit instead.
	if !isTTY() || !stdinIsTTY() {
		return fmt.Errorf("the terminal interface needs a terminal on stdin and stdout; " +
			"for a script use `kata submit`, `kata list` or `kata progress`")
	}

	root, err := findRoot()
	if err != nil {
		return err
	}

	chapters := allChapters(root)
	if len(chapters) == 0 {
		return fmt.Errorf("no chapters written yet")
	}

	// Start on the first chapter that has not passed.
	cursor := 0
	prog := loadProgress(root)
	for i, c := range chapters {
		if c.written() && !prog.passed(c.ID) {
			cursor = i
			break
		}
	}

	model := uiModel{
		root:     root,
		chapters: chapters,
		progress: prog,
		cursor:   cursor,
	}

	program := tea.NewProgram(model, tea.WithAltScreen())
	_, err = program.Run()
	return err
}

type uiModel struct {
	root     string
	chapters []chapter
	progress progressMap
	cursor   int

	running  bool
	status   string
	result   *gradeResult
	resultID string

	width  int
	height int
}

type gradeDoneMsg struct {
	id     string
	result gradeResult
	err    error
}

func (m uiModel) Init() tea.Cmd { return nil }

func (m uiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.result = nil
			}

		case "down", "j":
			if m.cursor < len(m.chapters)-1 {
				m.cursor++
				m.result = nil
			}

		case "enter":
			if m.running {
				return m, nil
			}
			ch := m.chapters[m.cursor]
			if !ch.written() {
				m.status = fmt.Sprintf("chapter %s is not written yet", ch.ID)
				return m, nil
			}
			m.running = true
			m.status = fmt.Sprintf("grading chapter %s", ch.ID)
			m.result = nil
			return m, m.gradeCmd(ch)

		case "r":
			m.progress = loadProgress(m.root)
			m.status = "reloaded progress"

		case "g":
			m.cursor = 0
			m.result = nil
		}

	case gradeDoneMsg:
		m.running = false
		if msg.err != nil {
			m.status = "grading failed: " + msg.err.Error()
			return m, nil
		}
		res := msg.result
		m.result = &res
		m.resultID = msg.id

		// Record the verdict so the plain commands agree with what is on screen.
		st := "failed"
		if res.Passed {
			st = "passed"
		}
		m.progress[msg.id] = progressEntry{Status: st, When: timeNow(), Cases: res.Cases}
		_ = saveProgress(m.root, m.progress)

		if res.Passed {
			m.status = "chapter " + msg.id + " passed"
		} else {
			m.status = "chapter " + msg.id + " is not passing yet"
		}
	}

	return m, nil
}

func (m uiModel) gradeCmd(ch chapter) tea.Cmd {
	root := m.root
	return func() tea.Msg {
		res, err := gradeChapter(root, ch, "")
		return gradeDoneMsg{id: ch.ID, result: res, err: err}
	}
}

var (
	uiTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	uiMuted   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	uiOK      = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	uiBad     = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	uiCursor  = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	uiPart    = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true)
	uiPanel   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	uiHelp    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	uiHeading = lipgloss.NewStyle().Bold(true)
)

func (m uiModel) View() string {
	left := m.listView()
	right := m.panelView()

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
	help := uiHelp.Render("↑/↓ move · enter grade · r reload · g first · q quit")
	return "\n" + body + "\n\n  " + help + "\n"
}

func (m uiModel) listView() string {
	var b strings.Builder
	b.WriteString(uiTitle.Render("Kata") + "\n\n")

	lastPart := ""
	for i, ch := range m.chapters {
		part := partOf(ch.ID)
		if part != lastPart {
			b.WriteString("\n" + uiPart.Render(strings.ToUpper(part)) + "\n")
			lastPart = part
		}

		mark, style := "○", uiMuted
		switch {
		case !ch.written():
			mark = "·"
		case m.progress.passed(ch.ID):
			mark, style = "✓", uiOK
		case m.progress[ch.ID].Status == "failed":
			mark, style = "✗", uiBad
		default:
			mark = " "
		}

		line := fmt.Sprintf("%s %s %-26s", mark, ch.ID, truncate(ch.Title, 24))
		if m.progress[ch.ID].Cases != "" {
			line += " " + m.progress[ch.ID].Cases
		}
		if i == m.cursor {
			b.WriteString(uiCursor.Render("▸ "+line) + "\n")
			continue
		}
		b.WriteString(style.Render(line) + "\n")
	}
	return lipgloss.NewStyle().Width(44).Render(b.String())
}

func (m uiModel) panelView() string {
	ch := m.chapters[m.cursor]

	var b strings.Builder
	b.WriteString(uiHeading.Render(ch.ID+" "+ch.Title) + "\n")
	b.WriteString(uiMuted.Render(partOf(ch.ID)) + "\n\n")

	if !ch.written() {
		b.WriteString("This chapter is not written yet.\n")
		b.WriteString(uiMuted.Render("The course is built in order; the arc above shows what is coming.") + "\n")
		return uiPanel.Width(52).Render(b.String())
	}

	if entry, ok := m.progress[ch.ID]; ok {
		state := entry.Status
		style := uiBad
		if state == "passed" {
			style = uiOK
		}
		b.WriteString(style.Render(state) + "  " + uiMuted.Render(entry.Cases) + "\n\n")
	} else {
		b.WriteString(uiMuted.Render("not graded yet") + "\n\n")
	}

	if m.running {
		b.WriteString("grading…\n")
	}

	if m.result != nil && m.resultID == ch.ID {
		if m.result.Passed {
			b.WriteString(uiOK.Render(m.result.Cases) + "\n")
		} else {
			b.WriteString(uiBad.Render(m.result.Cases) + "\n\n")
			failed := failingCases(m.result.Report)
			if len(failed) > 0 {
				b.WriteString(uiMuted.Render("failing") + "\n")
				for _, id := range failed {
					b.WriteString("  · " + id + "\n")
				}
				b.WriteString("\n" + uiMuted.Render("kata hint "+ch.ID+" "+failed[0]) + "\n")
			}
		}
	}

	if m.status != "" {
		b.WriteString("\n" + uiMuted.Render(m.status))
	}

	return uiPanel.Width(52).Render(b.String())
}

func failingCases(rep grader.Report) []string {
	var out []string
	for _, c := range rep.Cases {
		if !c.Passed {
			out = append(out, c.ID)
		}
	}
	return out
}

// stdinIsTTY reports whether standard input is a terminal. A person's keyboard is; a pipe
// or /dev/null is not.
func stdinIsTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
