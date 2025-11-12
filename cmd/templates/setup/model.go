// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/cmd/dotenv"
	"github.com/datarobot/cli/cmd/templates/clone"
	"github.com/datarobot/cli/cmd/templates/list"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/internal/state"
	"github.com/datarobot/cli/tui"
)

type screens int

const (
	welcomeScreen = screens(iota)
	hostScreen
	loginScreen
	listScreen
	cloneScreen
	dotenvScreen
	exitScreen
)

type Model struct {
	screen   screens
	template drapi.Template

	spinner          spinner.Model
	help             help.Model
	keys             keyMap
	isLoading        bool
	loadingMessage   string
	width            int
	hasAuthenticated bool // Track if we've already authenticated

	fromStartCommand     bool // true if invoked from dr start
	skipDotenvSetup      bool // true if dotenv setup was already completed
	dotenvSetupCompleted bool // tracks if dotenv was actually run (for state update)
	hostModel HostModel
	login     LoginModel
	list      list.Model
	clone     clone.Model
	dotenv    dotenv.Model
}

type keyMap struct {
	Enter key.Binding
	Quit  key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Enter, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Enter, k.Quit},
	}
}

type (
	getHostMsg          struct{}
	authKeyStartMsg     struct{}
	authKeySuccessMsg   struct{}
	templatesLoadedMsg  struct{ templatesList *drapi.TemplateList }
	templateSelectedMsg struct{}
	backToListMsg       struct{}
	templateClonedMsg   struct{}
	templateInDirMsg    struct {
		dotenvFile string
		template   drapi.Template
	}
	dotenvUpdatedMsg struct{}
	exitMsg          struct{}
)

func getHost() tea.Msg          { return getHostMsg{} }
func authSuccess() tea.Msg      { return authKeySuccessMsg{} }
func templateSelected() tea.Msg { return templateSelectedMsg{} }
func backToList() tea.Msg       { return backToListMsg{} }
func templateCloned() tea.Msg   { return templateClonedMsg{} }
func dotenvUpdated() tea.Msg    { return dotenvUpdatedMsg{} }
func exit() tea.Msg             { return exitMsg{} }

// matchTemplateByGitRemote attempts to match a template from the list based on the current git remote URL
func matchTemplateByGitRemote(templatesList *drapi.TemplateList) (drapi.Template, bool) {
	md := exec.Command("git", "config", "--get", "remote.origin.url")

	out, err := md.Output()
	if err != nil {
		log.Debug("Failed to get current git remote URL", "error", err)
		return drapi.Template{}, false
	}

	remoteURL := strings.TrimSpace(string(out))
	log.Debug("Current git remote URL: " + remoteURL)

	urlRepoRegex := ".com[:|/]([^.]*)"
	compiledRegex := regexp.MustCompile(urlRepoRegex)
	matches := compiledRegex.FindStringSubmatch(remoteURL)

	if len(matches) <= 1 {
		return drapi.Template{}, false
	}

	repoName := matches[1]
	log.Debug("Detected repo name: " + repoName)

	for _, t := range templatesList.Templates {
		tRepoMatches := compiledRegex.FindStringSubmatch(t.Repository.URL)
		if len(tRepoMatches) > 1 && tRepoMatches[1] == repoName {
			log.Debug("Found matching template: " + t.Name)
			return t, true
		}
	}

	return drapi.Template{}, false
}

// handleExistingRepo handles the case where we're already in a DataRobot repo
func handleExistingRepo(repoRoot string) tea.Msg {
	log.Debug("Already in a DataRobot repo at: " + repoRoot)

	templatesList, err := drapi.GetPublicTemplatesSorted()
	if err != nil {
		log.Warn("Failed to get templates, proceeding with dotenv setup anyway", "error", err)

		return templateInDirMsg{
			dotenvFile: filepath.Join(repoRoot, ".env"),
			template:   drapi.Template{},
		}
	}

	template, found := matchTemplateByGitRemote(templatesList)
	if found {
		return templateInDirMsg{
			dotenvFile: filepath.Join(repoRoot, ".env"),
			template:   template,
		}
	}

	log.Debug("Could not match git remote to a template, proceeding with dotenv setup")

	return templateInDirMsg{
		dotenvFile: filepath.Join(repoRoot, ".env"),
		template:   drapi.Template{},
	}
}

// handleGitRepoWithoutDataRobotCLI handles the case where we're in a git repo but .datarobot/cli doesn't exist yet
func handleGitRepoWithoutDataRobotCLI(templatesList *drapi.TemplateList) tea.Msg {
	template, found := matchTemplateByGitRemote(templatesList)
	if !found {
		return templatesLoadedMsg{templatesList}
	}

	cwd, err := os.Getwd()
	if err != nil {
		log.Error("Failed to get current working directory", "error", err)
		return templatesLoadedMsg{templatesList}
	}

	return templateInDirMsg{
		dotenvFile: filepath.Join(cwd, ".env"),
		template:   template,
	}
}

func getTemplates() tea.Cmd {
	return func() tea.Msg {
		datarobotHost := config.GetBaseURL()
		if datarobotHost == "" {
			return getHostMsg{}
		}

		// Check if we're already in a DataRobot repo by looking for .datarobot/cli folder
		repoRoot, err := repo.FindRepoRoot()
		if err == nil && repoRoot != "" {
			return handleExistingRepo(repoRoot)
		}

		// Not in a DataRobot repo, show the template gallery
		templatesList, err := drapi.GetPublicTemplatesSorted()
		if err != nil {
			return authKeyStartMsg{}
		}

		// Check if we're in a git repo that matches a template URL (for cases where .datarobot/cli doesn't exist yet)
		return handleGitRepoWithoutDataRobotCLI(templatesList)
	}
}

func saveHost(host string) tea.Cmd {
	return func() tea.Msg {
		_ = config.SaveURLToConfig(host)

		return authKeyStartMsg{}
	}
}

func NewModel(fromStartCommand bool) Model {
	err := config.ReadConfigFile("")
	if err != nil {
		log.Error("Failed to read config file", "error", err)
	}

	// Check if dotenv setup was already completed
	skipDotenv := state.HasCompletedDotenvSetup()
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = tui.InfoStyle

	h := help.New()
	h.ShowAll = false

	keys := keyMap{
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "next"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}

	return Model{
		screen:   welcomeScreen,
		template: drapi.Template{},

		spinner:          s,
		help:             h,
		keys:             keys,
		isLoading:        true,
		loadingMessage:   "Checking authentication and loading templates...",
		width:            80,
		hasAuthenticated: false,

		hostModel: NewHostModel(),
		login: LoginModel{
			APIKeyChan: make(chan string, 1),
			GetHostCmd: getHost,
			SuccessCmd: authSuccess,
		},
		list: list.Model{
			SuccessCmd: templateSelected,
		},
		clone: clone.Model{
			SuccessCmd: templateCloned,
			BackCmd:    backToList,
		},
		dotenv: dotenv.Model{
			SuccessCmd: dotenvUpdated,
		},

		fromStartCommand: fromStartCommand,
		skipDotenvSetup:  skipDotenv,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, getTemplates())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint: cyclop
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.help.Width = msg.Width
	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "q":
			if m.screen != cloneScreen && m.screen != dotenvScreen {
				return m, tea.Quit
			}
		}
	case spinner.TickMsg:
		var cmd tea.Cmd

		m.spinner, cmd = m.spinner.Update(msg)

		return m, cmd
	case getHostMsg:
		m.screen = hostScreen
		m.isLoading = false
		m.loadingMessage = ""
		m.hostModel.SuccessCmd = saveHost

		return m, m.hostModel.Init()
	case authKeyStartMsg:
		// Prevent double authentication
		if m.hasAuthenticated {
			return m, getTemplates()
		}

		m.isLoading = true
		m.loadingMessage = "Authenticating with DataRobot..."
		m.screen = loginScreen
		cmd := m.login.Init()

		return m, cmd
	case authKeySuccessMsg:
		m.hasAuthenticated = true
		m.isLoading = true
		m.loadingMessage = "Loading templates..."
		m.screen = listScreen

		return m, getTemplates()
	case templatesLoadedMsg:
		m.isLoading = false
		m.loadingMessage = ""
		m.screen = listScreen
		m.list.SetTemplates(msg.templatesList.Templates)

		return m, m.list.Init()
	case templateSelectedMsg:
		m.screen = cloneScreen
		m.template = m.list.Template
		m.clone.SetTemplate(m.template)

		return m, m.clone.Init()
	case backToListMsg:
		m.screen = listScreen

		return m, m.list.Init()
	case templateClonedMsg:
		// Skip dotenv if it was already completed
		if m.skipDotenvSetup {
			m.screen = exitScreen

			return m, tea.Sequence(tea.ExitAltScreen, exit)
		}

		m.isLoading = false
		m.loadingMessage = ""
		m.screen = dotenvScreen
		m.dotenv.DotenvFile = filepath.Join(m.clone.Dir, ".env")
		m.dotenvSetupCompleted = true

		return m, m.dotenv.Init()

	case templateInDirMsg:
		// Skip dotenv if it was already completed
		if m.skipDotenvSetup {
			m.screen = exitScreen

			return m, tea.Sequence(tea.ExitAltScreen, exit)
		}

		m.screen = dotenvScreen
		m.list.Template = msg.template
		m.dotenv.DotenvFile = msg.dotenvFile
		m.dotenvSetupCompleted = true

		return m, m.dotenv.Init()
	case dotenvUpdatedMsg:
		m.screen = exitScreen

		// Update state if dotenv setup was completed
		if m.dotenvSetupCompleted {
			_ = state.UpdateAfterDotenvSetup()
		}

		return m, tea.Sequence(tea.ExitAltScreen, exit)
	case exitMsg:
		return m, tea.Quit
	}

	var cmd tea.Cmd

	switch m.screen {
	case welcomeScreen:
		// No interaction needed - loading starts automatically
	case hostScreen:
		m.hostModel, cmd = m.hostModel.Update(msg)

		return m, cmd
	case loginScreen:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch keypress := msg.String(); keypress {
			case "esc":
				m.login.server.Close()
				// Reset authentication flag when user goes back to change URL
				m.hasAuthenticated = false

				return m, getHost
			}
		}

		m.login, cmd = m.login.Update(msg)

		return m, cmd
	case listScreen:
		m.list, cmd = m.list.Update(msg)

		return m, cmd
	case cloneScreen:
		m.clone, cmd = m.clone.Update(msg)

		// Show loading status when cloning starts
		if m.clone.IsCloning() && !m.isLoading {
			m.isLoading = true
			m.loadingMessage = "Cloning template to your computer..."
		}

		return m, cmd
	case dotenvScreen:
		dotenvModel, cmd := m.dotenv.Update(msg)
		// Type assertion to appease compiler
		m.dotenv = dotenvModel.(dotenv.Model)

		return m, cmd
	case exitScreen:
	}

	return m, nil
}

func (m Model) View() string { //nolint: cyclop
	var sb strings.Builder

	// Render header with logo
	sb.WriteString(tui.Header())
	sb.WriteString("\n\n")

	switch m.screen {
	case welcomeScreen:
		// Consolidated styling
		contentWidth := 60

		title := tui.WelcomeStyle.
			Width(contentWidth).
			Align(lipgloss.Left).
			MarginBottom(1).
			Render("🎉 Welcome to DataRobot CLI Setup Wizard!")

		subtitle := tui.BaseTextStyle.
			Width(contentWidth).
			Render("This wizard helps you:")

		// Create styled frame for steps
		stepStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "#6124DF", Dark: "#9D7EDF"}).
			Padding(1, 2).
			Width(contentWidth)

		stepsContent := strings.Join([]string{
			"1️⃣  Choose an AI application template",
			"2️⃣  Clone it to your computer",
			"3️⃣  Configure your environment",
			"4️⃣  Get you ready to build!",
		}, "\n")

		steps := stepStyle.Render(stepsContent)

		info := tui.InfoStyle.
			Width(contentWidth).
			MarginTop(1).
			Render(strings.Join([]string{
				"⏱️  Takes about 3-5 minutes",
				"🎯 You'll have a working AI app at the end",
			}, "\n"))

		content := lipgloss.JoinVertical(
			lipgloss.Left,
			title,
			"",
			subtitle,
			steps,
			"",
			info,
		)

		sb.WriteString(content)
		sb.WriteString("\n\n")

	case hostScreen:
		sb.WriteString(m.hostModel.View())

	case loginScreen:
		title := tui.BaseTextStyle.
			Bold(true).
			Render("🔐 Connect Your DataRobot Account")

		subtitle := tui.BaseTextStyle.
			Render("Opening your browser to securely authenticate...")

		content := lipgloss.JoinVertical(
			lipgloss.Left,
			title,
			"",
			subtitle,
			m.login.View(),
			"",
			tui.BaseTextStyle.Faint(true).Render("💡 Press Esc to change DataRobot URL"),
		)

		sb.WriteString(content)
	case listScreen:
		sb.WriteString(m.list.View())
	case cloneScreen:
		sb.WriteString(m.clone.View())
	case dotenvScreen:
		sb.WriteString(m.dotenv.View())
	case exitScreen:
		sb.WriteString(tui.SubTitleStyle.Render(fmt.Sprintf("🎉 Template %s cloned and initialized.", m.template.Name)))
		sb.WriteString("\n")

		if m.fromStartCommand {
			sb.WriteString(tui.BaseTextStyle.Render("You can now start running your AI application!"))
			sb.WriteString("\n\n")
			sb.WriteString(tui.BaseTextStyle.Render("• Use "))
			sb.WriteString(tui.InfoStyle.Render("dr task run"))
			sb.WriteString(tui.BaseTextStyle.Render(" to see the key commands to deploy the app"))
			sb.WriteString("\n")
			sb.WriteString(tui.BaseTextStyle.Render("• Use "))
			sb.WriteString(tui.InfoStyle.Render("dr task list"))
			sb.WriteString(tui.BaseTextStyle.Render(" to see all the additional commands"))
			sb.WriteString("\n")
		} else {
			sb.WriteString(tui.BaseTextStyle.Render("To navigate to the project directory, use the following command:"))
			sb.WriteString("\n\n")
			sb.WriteString(tui.BaseTextStyle.Render("cd " + m.clone.Dir))
			sb.WriteString("\n\n")
			sb.WriteString(tui.BaseTextStyle.Render("afterward get started with: "))
			sb.WriteString(tui.InfoStyle.Render("dr start"))
			sb.WriteString("\n")
		}
	}

	// Always show status bar at the bottom
	sb.WriteString("\n")

	if m.isLoading {
		sb.WriteString(tui.RenderStatusBar(m.width, m.spinner, m.loadingMessage, m.isLoading))
	} else if m.screen == welcomeScreen {
		// Show idle status bar only on welcome screen
		sb.WriteString(tui.RenderStatusBar(m.width, m.spinner, "Ready to start your AI journey", false))
	} else if m.screen == hostScreen {
		// Show status bar on host selection screen (waiting for input, no spinner)
		sb.WriteString(tui.RenderStatusBar(m.width, m.spinner, "Waiting for environment host selection", false))
	} else if m.screen == listScreen {
		// Show status bar on template selection screen
		sb.WriteString(tui.RenderStatusBar(m.width, m.spinner, "Choose your template to get started", false))
	}

	return sb.String()
}
