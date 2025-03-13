package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/stopwatch"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	padding  = 2
	maxWidth = 9999
)

var (
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170"))
	paginationStyle   = list.DefaultStyles().PaginationStyle.PaddingLeft(4)
	helpStyle         = list.DefaultStyles().HelpStyle.PaddingLeft(4).PaddingBottom(1)
	quitTextStyle     = lipgloss.NewStyle().Margin(1, 0, 2, 4)
	appStyle          = lipgloss.NewStyle().Padding(1, 2).Align(lipgloss.Center)
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
	width       int
	height      int
	showSplash  bool
	splashTimer int
	stopwatch   stopwatch.Model
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
		width:       maxWidth,
		height:      24,
		showSplash:  true,
		splashTimer: 20,
		stopwatch:   stopwatch.NewWithInterval(time.Second),
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
	return tea.Batch(
		tickCmd(),
		m.stopwatch.Init(),
	)
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
	// Mise à jour du stopwatch
	var swCmd tea.Cmd
	m.stopwatch, swCmd = m.stopwatch.Update(msg)

	// Si on est sur l'écran d'intro
	if m.showSplash {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			// Une touche a été pressée, on passe à l'app
			m.showSplash = false
			return m, nil

		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			return m, nil

		case tickMsg:
			// Décrémenter le timer du splash
			m.splashTimer--
			if m.splashTimer <= 0 {
				m.showSplash = false
				return m, nil
			}
			return m, tickCmd()
		}
		return m, tea.Batch(swCmd)
	}

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
		m.width = msg.Width
		m.height = msg.Height

		m.progress.Width = msg.Width - padding*2 - 4
		if m.progress.Width > maxWidth {
			m.progress.Width = maxWidth
		}

		// Mettre à jour la hauteur et largeur
		if m.page == "Projects" {
			h, v := appStyle.GetFrameSize()
			m.reposList.SetSize(msg.Width-h, msg.Height-v-10)
		}
		if m.page == "Skills" {
			m.skillsTable.SetWidth(msg.Width - 20)
			m.skillsTable.SetHeight(msg.Height - 15)
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
		return m, tea.Batch(cmd, swCmd)
	}

	// Update table if we're on the Skills page
	if m.page == "Skills" {
		var cmd tea.Cmd
		m.skillsTable, cmd = m.skillsTable.Update(msg)
		return m, cmd
	}

	// Utiliser le même type de message (tickMsg) que celui déjà défini
	clockCmd := tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})

	// Combiner avec les autres commandes
	return m, tea.Batch(swCmd, clockCmd)
}

func (m model) View() string {
	// Si on doit afficher l'écran d'intro
	if m.showSplash {
		mainStyle := lipgloss.NewStyle().
			Width(m.width).
			Height(m.height).
			Align(lipgloss.Center).
			AlignVertical(lipgloss.Center)

		// Création d'un conteneur principal bien centré
		containerStyle := lipgloss.NewStyle().
			Width(80). // Largeur fixe plus adaptée à l'art ASCII
			Align(lipgloss.Center).
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#8A2BE2")).
			Padding(1, 2) // Padding vertical et horizontal

		// Style pour l'art ASCII avec alignement précis
		nameStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#4287f5")).
			Align(lipgloss.Center).
			Width(76) // Légèrement plus petit que le conteneur

		// Style pour le texte de présentation
		titleStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#ff66c4")).
			Align(lipgloss.Center).
			Width(76). // Même largeur que l'art ASCII
			MarginTop(1)

		// Appliquer les styles
		artBox := nameStyle.Render(nameArt)
		title := titleStyle.Render("Milan Hommet - Fullstack Developer")

		// Combinaison de l'art et du titre dans un conteneur
		content := artBox + "\n" + title
		containerContent := containerStyle.Render(content)

		return mainStyle.Render(containerContent)
	}

	// Style pour le contenu principal, mais avec une hauteur réduite pour laisser
	// de la place pour l'en-tête et le pied de page
	mainStyle := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height - 4). // Réduire la hauteur pour laisser de la place
		Align(lipgloss.Center).
		AlignVertical(lipgloss.Center)

	innerStyle := lipgloss.NewStyle().
		Width(m.width - 4).
		Align(lipgloss.Center)

	// Créer un conteneur avec une bordure visible pour l'en-tête
	headerContainerStyle := lipgloss.NewStyle().
		Width(m.width).
		BorderBottom(true).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#8A2BE2"))

	// Style pour le nom à gauche
	nameStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#4287f5")).
		Bold(true).
		PaddingLeft(4).
		Width(30)

	// Style pour le titre à droite
	jobStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#ff66c4")).
		Bold(true).
		PaddingRight(4).
		Align(lipgloss.Right).
		Width(30)

	// Structure simplifiée pour l'en-tête
	leftContent := nameStyle.Render("Milan Hommet")
	rightContent := jobStyle.Render("Fullstack Developer")

	// Joindre les deux parties avec un espace au milieu
	header := headerContainerStyle.Render(
		lipgloss.JoinHorizontal(
			lipgloss.Center,
			leftContent,
			lipgloss.NewStyle().
				Width(m.width-lipgloss.Width(leftContent)-lipgloss.Width(rightContent)).
				Render(""),
			rightContent,
		),
	)

	// Style pour le pied de page
	footerStyle := lipgloss.NewStyle().
		Width(m.width).
		BorderTop(true).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#8A2BE2"))

	// Contenu du pied de page
	clockText := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		PaddingLeft(4).
		Width(20).
		Render("🕒 " + time.Now().Format("15:04:05"))

	timerText := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		Align(lipgloss.Right).
		PaddingRight(4).
		Width(20).
		Render("⏱ " + m.stopwatch.View())

	footer := footerStyle.Render(
		lipgloss.JoinHorizontal(
			lipgloss.Center,
			clockText,
			lipgloss.NewStyle().
				Width(m.width-lipgloss.Width(clockText)-lipgloss.Width(timerText)).
				Render(""),
			timerText,
		),
	)

	// Contenu principal (comme avant)
	var inner string

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Align(lipgloss.Center).
		Width(m.width - 4)

	inner += titleStyle.Render("Welcome to my portfolio") + "\n\n"

	var content string
	if m.errorMsg != "" {
		content = fmt.Sprintf("Error: %s\n", m.errorMsg)
	} else if m.page == "menu" {
		menuStyle := lipgloss.NewStyle().Align(lipgloss.Center).Width(m.width - 4)
		menuContent := ""
		for i, section := range m.sections {
			if i == m.cursor {
				menuContent += " > " + section + "\n"
			} else {
				menuContent += "   " + section + "\n"
			}
		}
		content = menuStyle.Render(menuContent)

		// Centrer également les contrôles
		controlsStyle := lipgloss.NewStyle().Align(lipgloss.Center).Width(m.width - 4)
		content += "\n" + controlsStyle.Render("Controls: ↑/k up • ↓/j down • b back • q quit • ENTER select")
	} else if m.loading {
		loadingStyle := lipgloss.NewStyle().Align(lipgloss.Center).Width(m.width - 4)
		content = loadingStyle.Render("Loading...") + "\n\n"
		content += m.progress.View() + "\n"
	} else {
		content = m.getPageContent()

		// Centrer les contrôles en bas
		controlsStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Align(lipgloss.Center).Width(m.width - 4)
		content += "\n\n" + controlsStyle.Render("Controls: ↑/k up • ↓/j down • b back • q quit")
	}

	inner += content

	// Assembler toutes les parties
	return header + "\n" + mainStyle.Render(innerStyle.Render(inner)) + "\n" + footer
}

func (m model) getPageContent() string {
	contentStyle := lipgloss.NewStyle().
		Align(lipgloss.Center).
		Width(m.width - 10)

	switch m.page {
	case "About Me":
		return contentStyle.Render("\nAbout Me:\n" +
			"I'm a software developer based in France, specializing in software and mobile development but I'm also interested in game development.\n" +
			"I'm currently pursuing an MBA in development and management. I like to learn new languages and frameworks in my free time.\n" +
			"I have a work-study contract at Téïcée as a backend developer.\n")
	case "Education":
		return contentStyle.Render("\nEducation:\n" +
			"2023 - 2025 : Master degree - Fullstack developer\n" +
			"2022 - 2023 : Bachelor degree - Web developer\n" +
			"2020 - 2022 : BTEC Higher National Diploma - web and software development\n")
	case "Experience":
		return contentStyle.Render("\nExperience:\n" +
			"2022 - today : Fullstack Developer at Téïcée\n")
	case "Skills":
		// Pour les tables, on doit appliquer un style spécial
		tableContent := "\nSkills:\n\n" + baseTableStyle.Render(m.skillsTable.View())
		return contentStyle.Render(tableContent)
	case "Projects":
		content := m.reposList.View()
		if m.loaded {
			content += "\n\nPress Enter to open the selected project in your browser."
		}
		return content // La liste gère son propre rendu
	case "Contact":
		return contentStyle.Render("\nContact:\n" +
			"Email: milan.hommet@protonmail.com\n" +
			"LinkedIn: https://www.linkedin.com/in/milan-hommet-840414315/\n")
	}
	return contentStyle.Render("\nContent for " + m.page)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*50, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func clockTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func main() {
	p := tea.NewProgram(
		initialModel(),
		tea.WithAltScreen(),       // Utilise l'écran alternatif (plein écran)
		tea.WithMouseCellMotion(), // Supporte les événements de souris
	)
	if err := p.Start(); err != nil {
		fmt.Println("Oh no!", err)
		os.Exit(1)
	}
}
