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
	screen      screens
	template    drapi.Template
	exitMessage string

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

func getTemplates() tea.Cmd {
	return func() tea.Msg {
		datarobotHost := config.GetBaseURL()
		if datarobotHost == "" {
			return getHostMsg{}
		}

		templatesList, err := drapi.GetTemplates()
		if err != nil {
			return authKeyStartMsg{}
		}

		// We need to detect if we're already in a template repo to allow users to rerun setup on their
		// Current Template
		// We do this by checking if the URL of any of the templates matches the current git remote URL
		// If it does, we set that template as selected in the list
		md := exec.Command("git", "config", "--get", "remote.origin.url")
		out, err := md.Output()

		if err == nil { //nolint: nestif
			remoteURL := strings.TrimSpace(string(out))
			log.Debug("Current git remote URL: " + remoteURL)

			urlRepoRegex := ".com[:|/]([^.]*)"
			compiledRegex := regexp.MustCompile(urlRepoRegex)
			matches := compiledRegex.FindStringSubmatch(remoteURL)

			if len(matches) > 1 {
				repoName := matches[1]
				log.Debug("Detected repo name: " + repoName)

				for _, t := range templatesList.Templates {
					tRepoMatches := compiledRegex.FindStringSubmatch(t.Repository.URL)
					if len(tRepoMatches) > 1 && tRepoMatches[1] == repoName {
						log.Debug("Found matching template: " + t.Name)

						cwd, err := os.Getwd()
						if err != nil {
							log.Error("Failed to get current working directory", "error", err)
							break
						}

						return templateInDirMsg{
							dotenvFile: filepath.Join(cwd, ".env"),
							template:   t,
						}
					}
				}
			}
		} else {
			log.Debug("Failed to get current git remote URL. Assuming we're not in a repo and continuing.", "error", err)
		}

		return templatesLoadedMsg{templatesList}
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
		case "ctrl+c":
			return m, tea.Quit
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
		m.exitMessage = fmt.Sprintf("Template '%s' cloned and initialized in '%s' directory.\n\n",
			m.template.Name, m.clone.Dir,
		)

		return m, tea.Sequence(tea.ExitAltScreen, exit)
	case exitMsg:
		return m, tea.Quit
	}

	var cmd tea.Cmd

	var cmds []tea.Cmd

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
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
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
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case listScreen:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch keypress := msg.String(); keypress {
			case "esc":
				// return m, getHost
				return m, nil
			}
		}

		m.list, cmd = m.list.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case cloneScreen:
		m.clone, cmd = m.clone.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case dotenvScreen:
		dotenvModel, cmd := m.dotenv.Update(msg)
		// Type assertion to appease compiler
		m.dotenv = dotenvModel.(dotenv.Model)

		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case exitScreen:
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	var sb strings.Builder

	// Render header with logo
	sb.WriteString(tui.Header())
	sb.WriteString("\n\n")

	switch m.screen {
	case welcomeScreen:
		// Render welcome content
		welcome := tui.WelcomeStyle.Render("Welcome to " + version.AppName)
		sb.WriteString(welcome)
		sb.WriteString("\n\n")

		sb.WriteString(tui.BaseTextStyle.Render("This wizard will help you set up a new DataRobot application template."))
		sb.WriteString("\n\n")

		// Render footer with quit instructions
		sb.WriteString(tui.Footer())
	case hostScreen:
		sb.WriteString(tui.BaseTextStyle.Render("This wizard will help you set up a new DataRobot application template."))
		sb.WriteString("\n\n")
		sb.WriteString("Please specify your DataRobot URL, or enter the numbers 1 - 3 If you are using that multi tenant cloud offering\n")
		sb.WriteString("Please enter 1 if you're using https://app.datarobot.com\n")
		sb.WriteString("Please enter 2 if you're using https://app.eu.datarobot.com\n")
		sb.WriteString("Please enter 3 if you're using https://app.jp.datarobot.com\n")
		sb.WriteString("Otherwise, please enter the URL you use\n\n")

		sb.WriteString(m.host.View())
	case loginScreen:
		sb.WriteString(tui.BaseTextStyle.Render("This wizard will help you set up a new DataRobot application template."))
		sb.WriteString("\n\n")

		sb.WriteString(m.login.View())

		sb.WriteString(tui.BaseTextStyle.Render("Press Esc to change DataRobot URL"))
	case listScreen:
		sb.WriteString(m.list.View())
	case cloneScreen:
		sb.WriteString(m.clone.View())
	case dotenvScreen:
		sb.WriteString(m.dotenv.View())
	case exitScreen:
		sb.WriteString(m.exitMessage)
	}

	return sb.String()
}
