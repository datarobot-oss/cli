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

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/cmd/dotenv"
	"github.com/datarobot/cli/cmd/templates/clone"
	"github.com/datarobot/cli/cmd/templates/list"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/internal/version"
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

	host   textinput.Model
	login  LoginModel
	list   list.Model
	clone  clone.Model
	dotenv dotenv.Model
}

type (
	getHostMsg          struct{}
	authKeyStartMsg     struct{}
	authKeySuccessMsg   struct{}
	templatesLoadedMsg  struct{ templatesList *drapi.TemplateList }
	templateSelectedMsg struct{}
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

func NewModel() Model {
	err := config.ReadConfigFile("")
	if err != nil {
		log.Error("Failed to read config file", "error", err)
	}

	return Model{
		screen:   welcomeScreen,
		template: drapi.Template{},

		host: textinput.New(),
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
		},
		dotenv: dotenv.Model{
			SuccessCmd: dotenvUpdated,
		},
	}
}

func (m Model) Init() tea.Cmd {
	return getTemplates()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint: cyclop
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "q":
			if m.screen != cloneScreen && m.screen != dotenvScreen {
				return m, tea.Quit
			}
		}
	case getHostMsg:
		m.screen = hostScreen
		focusCmd := m.host.Focus()

		return m, focusCmd
	case authKeyStartMsg:
		m.screen = loginScreen
		cmd := m.login.Init()

		return m, cmd
	case authKeySuccessMsg:
		m.screen = listScreen
		return m, getTemplates()
	case templatesLoadedMsg:
		m.screen = listScreen
		m.list.SetTemplates(msg.templatesList.Templates)

		return m, m.list.Init()
	case templateSelectedMsg:
		m.screen = cloneScreen
		m.template = m.list.Template
		m.clone.SetTemplate(m.template)

		return m, m.clone.Init()
	case templateClonedMsg:
		m.screen = dotenvScreen
		m.dotenv.DotenvFile = filepath.Join(m.clone.Dir, ".env")

		return m, m.dotenv.Init()

	case templateInDirMsg:
		m.screen = dotenvScreen
		m.list.Template = msg.template
		m.dotenv.DotenvFile = msg.dotenvFile

		return m, m.dotenv.Init()
	case dotenvUpdatedMsg:
		m.screen = exitScreen

		return m, tea.Sequence(tea.ExitAltScreen, exit)
	case exitMsg:
		return m, tea.Quit
	}

	var cmd tea.Cmd

	switch m.screen {
	case welcomeScreen:
	case hostScreen:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch keypress := msg.String(); keypress {
			case "enter":
				host := m.host.Value()
				m.host.SetValue("")
				m.host.Blur()

				return m, saveHost(host)
			}
		}

		m.host, cmd = m.host.Update(msg)

		return m, cmd
	case loginScreen:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch keypress := msg.String(); keypress {
			case "esc":
				m.login.server.Close()
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

func (m Model) View() string {
	var sb strings.Builder

	// Render header with logo
	sb.WriteString(tui.Header())
	sb.WriteString("\n\n")

	switch m.screen {
	case welcomeScreen:
		// Render welcome content
		welcome := tui.WelcomeStyle.Render("🎉 Welcome to " + version.AppName + " Setup Wizard!")
		sb.WriteString(welcome)
		sb.WriteString("\n\n")

		sb.WriteString(tui.BaseTextStyle.Render("This wizard helps you:"))
		sb.WriteString("\n")
		sb.WriteString(tui.BaseTextStyle.Render("  1️⃣ Choose an AI application template."))
		sb.WriteString("\n")
		sb.WriteString(tui.BaseTextStyle.Render("  2️⃣ Clone it to your computer."))
		sb.WriteString("\n")
		sb.WriteString(tui.BaseTextStyle.Render("  3️⃣ Configure your environment."))
		sb.WriteString("\n")
		sb.WriteString(tui.BaseTextStyle.Render("  4️⃣ Get you ready to build!"))
		sb.WriteString("\n\n")

		sb.WriteString(tui.BaseTextStyle.Render("⏱️ The process takes about 3-5 minutes."))
		sb.WriteString("\n")
		sb.WriteString(tui.BaseTextStyle.Render("🎯 You'll have a working AI app at the end"))
		sb.WriteString("\n\n")

		sb.WriteString(tui.BaseTextStyle.Render("Ready to get started? Press Enter to continue..."))
		sb.WriteString("\n\n")

		// Render footer with quit instructions
		sb.WriteString(tui.Footer())
	case hostScreen:
		sb.WriteString(tui.BaseTextStyle.Render("🌐 DataRobot URL Configuration"))
		sb.WriteString("\n\n")
		sb.WriteString(tui.BaseTextStyle.Render("Choose your DataRobot environment:"))
		sb.WriteString("\n\n")
		sb.WriteString("┌────────────────────────────────────────────────────────┐\n")
		sb.WriteString("│  [1] 🇺🇸 US Cloud        https://app.datarobot.com      │\n")
		sb.WriteString("│  [2] 🇪🇺 EU Cloud        https://app.eu.datarobot.com   │\n")
		sb.WriteString("│  [3] 🇯🇵 Japan Cloud     https://app.jp.datarobot.com   │\n")
		sb.WriteString("│  [4] 🏢 Custom/On-Prem   Enter your custom URL         │\n")
		sb.WriteString("└────────────────────────────────────────────────────────┘\n")
		sb.WriteString("\n")
		sb.WriteString(tui.BaseTextStyle.Render("🔗 Don't know which one? Check your DataRobot login page URL"))
		sb.WriteString("\n\n")

		sb.WriteString(m.host.View())
	case loginScreen:
		sb.WriteString(tui.BaseTextStyle.Render("🔐 DataRobot Authentication"))
		sb.WriteString("\n\n")
		sb.WriteString(tui.BaseTextStyle.Render("We'll now authenticate you with DataRobot using your browser."))
		sb.WriteString("\n\n")

		sb.WriteString(m.login.View())

		sb.WriteString("\n")
		sb.WriteString(tui.BaseTextStyle.Render("💡 Press Esc to change DataRobot URL"))
	case listScreen:
		sb.WriteString(m.list.View())
	case cloneScreen:
		sb.WriteString(m.clone.View())
	case dotenvScreen:
		sb.WriteString(m.dotenv.View())
	case exitScreen:
		sb.WriteString(tui.SubTitleStyle.Render(fmt.Sprintf("🎉 Template %s cloned and initialized.", m.template.Name)))
		sb.WriteString("\n")
		sb.WriteString(tui.BaseTextStyle.Render("To navigate to the project directory, use the following command:"))
		sb.WriteString("\n\n")
		sb.WriteString(tui.BaseTextStyle.Render("cd " + m.clone.Dir))
		sb.WriteString("\n")
	}

	return sb.String()
}
