package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	padding  = 2
	maxWidth = 80
)

var (
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170"))
	paginationStyle   = list.DefaultStyles().PaginationStyle.PaddingLeft(4)
	helpStyle         = list.DefaultStyles().HelpStyle.PaddingLeft(4).PaddingBottom(1)
	quitTextStyle     = lipgloss.NewStyle().Margin(1, 0, 2, 4)
	appStyle          = lipgloss.NewStyle().Padding(1, 2)
	titleStyle        = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFDF5")).
				Background(lipgloss.Color("#25A065")).
				Padding(0, 1)
	statusMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#04B575", Dark: "#04B575"}).
				Render
	descriptionStyle = lipgloss.NewStyle().
				PaddingLeft(4).
				Foreground(lipgloss.Color("#A49FA5"))
	// Table styles
	baseTableStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("240"))
)

type githubRepo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	HTMLURL     string `json:"html_url"`
}

type item struct {
	title       string
	description string
	url         string
}

func (i item) FilterValue() string { return "" }
func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.description }
func (i item) URL() string         { return i.url }

// Custom delegate for the project list items that shows descriptions
type projectDelegate struct{}

func (d projectDelegate) Height() int                             { return 2 } // Each item takes 2 lines (title + description)
func (d projectDelegate) Spacing() int                            { return 1 } // Add spacing between items
func (d projectDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d projectDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}

	// Title
	titleStr := fmt.Sprintf("%d. %s", index+1, i.title)

	// Render title (selected or not)
	if index == m.Index() {
		fmt.Fprintf(w, selectedItemStyle.Render("> "+titleStr))
	} else {
		fmt.Fprintf(w, itemStyle.Render(titleStr))
	}

	// Add a newline after the title
	fmt.Fprintf(w, "\n")

	// Render description on the second line
	desc := i.description
	if desc == "" {
		desc = "No description"
	}
	fmt.Fprintf(w, descriptionStyle.Render(desc))
}

type model struct {
	cursor      int
	sections    []string
	loading     bool
	progress    progress.Model
	page        string
	loaded      bool
	reposList   list.Model
	skillsTable table.Model
	errorMsg    string
}

func initialModel() model {
	// Create list with our custom delegate for projects
	projectList := list.New([]list.Item{}, projectDelegate{}, maxWidth, 20)
	projectList.SetShowTitle(true)
	projectList.Title = "GitHub Projects"
	projectList.Styles.Title = titleStyle
	projectList.SetShowPagination(false)   // Disable pagination
	projectList.SetShowStatusBar(false)    // Disable status bar
	projectList.SetFilteringEnabled(false) // Disable filtering

	// Initialize skills table
	skillsTable := initializeSkillsTable()

	return model{
		cursor:      0,
		sections:    []string{"About Me", "Education", "Experience", "Skills", "Projects", "Contact", "Exit"},
		progress:    progress.New(progress.WithDefaultGradient()),
		page:        "menu",
		loaded:      false,
		reposList:   projectList,
		skillsTable: skillsTable,
	}
}

// Initialize the skills table
func initializeSkillsTable() table.Model {
	columns := []table.Column{
		{Title: "Category", Width: 25},
		{Title: "Skills", Width: 50},
	}

	rows := []table.Row{
		{"Programming Languages", "Python, JavaScript, TypeScript, Dart, PHP"},
		{"Mobile Development", "Flutter, React Native"},
		{"Software Development", "Electron"},
		{"Web Development", "React, Symfony, VueJS, NextJS, NodeJS"},
		{"Databases", "MySQL, MongoDB, Microsoft SQL Server"},
		{"Game Engine", "Unity"},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	return t
}

type tickMsg time.Time
type repoMsg struct{ repos []githubRepo }
type errMsg struct{ err error }
type openURLMsg string

func (m model) Init() tea.Cmd {
	return tickCmd()
}

func fetchGitHubRepos() tea.Cmd {
	return func() tea.Msg {
		resp, err := http.Get("https://api.github.com/users/mhommet/repos?sort=updated&per_page=100")
		if err != nil {
			return errMsg{err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return errMsg{err: fmt.Errorf("failed to fetch repositories: %s", resp.Status)}
		}

		var repos []githubRepo
		if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
			return errMsg{err: err}
		}

		return repoMsg{repos: repos}
	}
}

// Helper function to open URLs in the default browser
func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", etc.
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

// Command to open a URL in browser
func openURL(url string) tea.Cmd {
	return func() tea.Msg {
		err := openBrowser(url)
		if err != nil {
			return errMsg{err: err}
		}
		return openURLMsg(url)
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "up", "k":
			if m.page == "menu" {
				if m.cursor > 0 {
					m.cursor--
				}
			} else if m.page == "Projects" {
				m.reposList.CursorUp()
			} else if m.page == "Skills" {
				m.skillsTable.MoveUp(1) // Pass the number of rows to move up
			}
		case "down", "j":
			if m.page == "menu" {
				if m.cursor < len(m.sections)-1 {
					m.cursor++
				}
			} else if m.page == "Projects" {
				m.reposList.CursorDown()
			} else if m.page == "Skills" {
				m.skillsTable.MoveDown(1) // Pass the number of rows to move down
			}
		case "enter":
			if m.page == "menu" {
				if m.sections[m.cursor] == "Exit" {
					return m, tea.Quit
				}
				m.loading = true
				m.page = m.sections[m.cursor]
				m.loaded = false
				m.progress = progress.New(progress.WithDefaultGradient())

				if m.page == "Projects" {
					return m, fetchGitHubRepos()
				}

				return m, tickCmd()
			} else if m.page == "Projects" && m.loaded {
				// When Enter is pressed on a repo, open it in browser
				if len(m.reposList.Items()) > 0 {
					selected := m.reposList.SelectedItem().(item)
					if selected.url != "" {
						return m, openURL(selected.url)
					}
				}
			}
		case "b":
			if m.page != "menu" {
				m.page = "menu"
				m.cursor = 0
				m.loaded = false
			}
		}
	case tea.WindowSizeMsg:
		m.progress.Width = msg.Width - padding*2 - 4
		if m.progress.Width > maxWidth {
			m.progress.Width = maxWidth
		}
		// Update the list height when window size changes
		if m.page == "Projects" {
			h, v := appStyle.GetFrameSize()
			m.reposList.SetSize(msg.Width-h, msg.Height-v)
		}
		return m, nil
	case tickMsg:
		if m.progress.Percent() == 1.0 {
			m.loading = false
			m.loaded = true
			return m, nil
		}
		cmd := m.progress.IncrPercent(0.25)
		return m, tea.Batch(tickCmd(), cmd)
	case repoMsg:
		var items []list.Item
		for _, repo := range msg.repos {
			items = append(items, item{
				title:       repo.Name,
				description: repo.Description,
				url:         repo.HTMLURL,
			})
		}
		m.reposList.SetItems(items)
		m.loading = false
		m.loaded = true

		// Add a status message to inform the user
		statusCmd := m.reposList.NewStatusMessage(statusMessageStyle("Press Enter to open selected project in browser"))
		return m, statusCmd
	case errMsg:
		m.errorMsg = msg.err.Error()
		m.loading = false
		m.loaded = true
		return m, nil
	case openURLMsg:
		// We could show a status message here if needed
		return m, nil
	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd
	}

	// Update list if we're on the Projects page
	if m.page == "Projects" {
		var cmd tea.Cmd
		m.reposList, cmd = m.reposList.Update(msg)
		return m, cmd
	}

	// Update table if we're on the Skills page
	if m.page == "Skills" {
		var cmd tea.Cmd
		m.skillsTable, cmd = m.skillsTable.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m model) View() string {
	var s string
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	s += titleStyle.Render("Welcome to my portfolio - Milan Hommet") + "\n\n"

	if m.errorMsg != "" {
		s += fmt.Sprintf("Error: %s\n", m.errorMsg)
	} else if m.page == "menu" {
		for i, section := range m.sections {
			if i == m.cursor {
				s += " > " + section + "\n"
			} else {
				s += "   " + section + "\n"
			}
		}
		s += "\nControls: ↑/k up • ↓/j down • b back • q quit • ENTER select\n"
	} else if m.loading {
		pad := strings.Repeat(" ", padding)
		s += "Loading...\n\n"
		s += pad + m.progress.View() + "\n"
	} else {
		s += m.getPageContent()

		// Add control hints at the bottom
		controlsStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
		s += "\n\n" + controlsStyle.Render("Controls: ↑/k up • ↓/j down • b back • q quit")
	}

	return appStyle.Render(s)
}

func (m model) getPageContent() string {
	switch m.page {
	case "About Me":
		return "\nAbout Me:\n" +
			"I'm a software developer based in France, specializing in software and mobile development but I'm also interested in game development.\n" +
			"I'm currently pursuing an MBA in development and management. I like to learn new languages and frameworks in my free time.\n" +
			"I have a work-study contract at Téïcée as a backend developer.\n"
	case "Education":
		return "\nEducation:\n" +
			"2023 - 2025 : Master degree - Fullstack developer\n" +
			"2022 - 2023 : Bachelor degree - Web developer\n" +
			"2020 - 2022 : BTEC Higher National Diploma - web and software development\n"
	case "Experience":
		return "\nExperience:\n" +
			"2022 - today : Fullstack Developer at Téïcée\n"
	case "Skills":
		return "\nSkills:\n\n" + baseTableStyle.Render(m.skillsTable.View())
	case "Projects":
		content := m.reposList.View()
		if m.loaded {
			content += "\n\nPress Enter to open the selected project in your browser."
		}
		return content
	case "Contact":
		return "\nContact:\n" +
			"Email: milan.hommet@protonmail.com\n" +
			"LinkedIn: https://www.linkedin.com/in/milan-hommet-840414315/\n"
	}
	return "\nContent for " + m.page
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*50, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func main() {
	p := tea.NewProgram(initialModel())
	if err := p.Start(); err != nil {
		fmt.Println("Oh no!", err)
		os.Exit(1)
	}
}
