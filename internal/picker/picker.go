package picker

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Item is one selectable row.
type Item struct {
	Label string
	Detail string
	Value string
}

type model struct {
	items  []Item
	cursor int
	choice *Item
	quit   bool
	width  int
}

var (
	titleStyle  = lipgloss.NewStyle().Bold(true)
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// Run shows an interactive list; returns the chosen Value or "" if cancelled.
func Run(title string, items []Item) (string, error) {
	if len(items) == 0 {
		return "", fmt.Errorf("nothing to pick")
	}
	m := model{items: items}
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	out, ok := final.(model)
	if !ok || out.choice == nil {
		return "", nil
	}
	return out.choice.Value, nil
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.quit = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "enter":
			c := m.items[m.cursor]
			m.choice = &c
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() string {
	if m.quit && m.choice == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("orma recall"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("enter select · esc cancel · j/k move"))
	b.WriteString("\n\n")
	for i, it := range m.items {
		cursor := "  "
		line := it.Label
		if i == m.cursor {
			cursor = cursorStyle.Render("> ")
			line = cursorStyle.Render(it.Label)
		}
		b.WriteString(cursor)
		b.WriteString(line)
		b.WriteByte('\n')
		if it.Detail != "" {
			b.WriteString("    ")
			b.WriteString(dimStyle.Render(truncate(it.Detail, 72)))
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
