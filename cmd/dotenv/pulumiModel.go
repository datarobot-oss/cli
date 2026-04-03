// Copyright 2025 DataRobot, Inc. and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dotenv

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/envbuilder"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/viper"
)

const (
	generatedPassphraseLength = 32
	pulumiConfigPassphraseKey = "pulumi_config_passphrase"
	pulumiDocsURL             = "https://www.pulumi.com/docs/iac/concepts/state-and-backends/"
	pulumiWhoamiTimeout       = 10 * time.Second
)

var pulumiArrow = lipgloss.NewStyle().Foreground(tui.DrPurple).SetString("→")

type pulumiLoginScreen int

const (
	pulumiLoginScreenBackendSelection pulumiLoginScreen = iota
	pulumiLoginScreenDIYURL
	pulumiLoginScreenLoggingIn
	pulumiLoginScreenPassphrasePrompt
)

// pulumiLoginModel handles the Pulumi login and passphrase setup flow.
// Login (backend selection → pulumi login) and passphrase setup are independent:
// login runs first (if needed), then the passphrase prompt appears (if needed).
type pulumiLoginModel struct {
	currentScreen       pulumiLoginScreen
	selectedOption      int
	options             []string
	diyInput            textinput.Model
	diyURL              string
	generatedPassphrase string
	err                 error
	loginOutput         string
	alreadyLoggedIn     bool // when true, skip backend selection and go straight to passphrase
	needsPassphrase     bool // when true, show passphrase prompt after login (or immediately if already logged in)
}

type (
	pulumiLoginCompleteMsg struct{}
	pulumiLoginErrorMsg    struct{ err error }
	pulumiLoginSuccessMsg  struct{ output string }
)

func newPulumiLoginModel(alreadyLoggedIn, needsPassphrase bool) pulumiLoginModel {
	ti := textinput.New()
	ti.Placeholder = "s3://my-pulumi-bucket or azblob://..."
	ti.Focus()
	ti.Width = 60

	initialScreen := pulumiLoginScreenBackendSelection

	if alreadyLoggedIn {
		initialScreen = pulumiLoginScreenPassphrasePrompt
	}

	return pulumiLoginModel{
		currentScreen:   initialScreen,
		selectedOption:  0,
		options:         []string{"Login locally", "Login to Pulumi Cloud", "DIY backend (S3, Azure Blob, etc.)"},
		diyInput:        ti,
		alreadyLoggedIn: alreadyLoggedIn,
		needsPassphrase: needsPassphrase,
	}
}

func (m pulumiLoginModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m pulumiLoginModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case pulumiLoginSuccessMsg:
		m.loginOutput = msg.output

		// Login done — show passphrase prompt if needed, otherwise complete
		if m.needsPassphrase {
			m.currentScreen = pulumiLoginScreenPassphrasePrompt

			return m, nil
		}

		return m, func() tea.Msg { return pulumiLoginCompleteMsg{} }

	case pulumiLoginErrorMsg:
		m.err = msg.err

		return m, tea.Quit
	}

	// Handle text input updates for DIY URL screen
	if m.currentScreen == pulumiLoginScreenDIYURL {
		var cmd tea.Cmd

		m.diyInput, cmd = m.diyInput.Update(msg)

		return m, cmd
	}

	return m, nil
}

func (m pulumiLoginModel) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.currentScreen {
	case pulumiLoginScreenBackendSelection:
		return m.handleBackendSelectionKey(msg)

	case pulumiLoginScreenDIYURL:
		return m.handleDIYURLKey(msg)

	case pulumiLoginScreenPassphrasePrompt:
		return m.handlePassphrasePromptKey(msg)

	case pulumiLoginScreenLoggingIn:
		// No key handling during login
		return m, nil
	}

	return m, nil
}

func (m pulumiLoginModel) handleBackendSelectionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.selectedOption > 0 {
			m.selectedOption--
		}

	case "down", "j":
		if m.selectedOption < len(m.options)-1 {
			m.selectedOption++
		}

	case "enter":
		switch m.selectedOption {
		case 0: // Local
			m.currentScreen = pulumiLoginScreenLoggingIn

			return m, m.performLogin("local", "")

		case 1: // Cloud
			m.currentScreen = pulumiLoginScreenLoggingIn

			return m, m.performLogin("cloud", "")

		case 2: // DIY
			m.currentScreen = pulumiLoginScreenDIYURL
		}
	}

	return m, nil
}

func (m pulumiLoginModel) handleDIYURLKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.diyURL = strings.TrimSpace(m.diyInput.Value())

		if m.diyURL == "" {
			return m, nil
		}

		m.currentScreen = pulumiLoginScreenLoggingIn

		return m, m.performLogin("diy", m.diyURL)

	case "esc":
		m.currentScreen = pulumiLoginScreenBackendSelection

	default:
		var cmd tea.Cmd

		m.diyInput, cmd = m.diyInput.Update(msg)

		return m, cmd
	}

	return m, nil
}

func (m pulumiLoginModel) handlePassphrasePromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		return m.handlePassphraseAccepted()

	case "n", "N", "esc":
		return m, func() tea.Msg { return pulumiLoginCompleteMsg{} }
	}

	return m, nil
}

func (m pulumiLoginModel) handlePassphraseAccepted() (tea.Model, tea.Cmd) {
	passphrase, err := generateRandomSecret(generatedPassphraseLength)
	if err != nil {
		m.err = fmt.Errorf("failed to generate passphrase: %w", err)

		return m, nil
	}

	m.generatedPassphrase = passphrase

	if err := m.savePassphraseToConfig(); err != nil {
		return m, func() tea.Msg { return pulumiLoginErrorMsg{err} }
	}

	return m, func() tea.Msg { return pulumiLoginCompleteMsg{} }
}

func (m pulumiLoginModel) savePassphraseToConfig() error {
	if err := config.CreateConfigFileDirIfNotExists(); err != nil {
		return fmt.Errorf("failed to create config: %w", err)
	}

	viper.Set(pulumiConfigPassphraseKey, m.generatedPassphrase)

	if err := viper.WriteConfig(); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func (m pulumiLoginModel) performLogin(loginType, url string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd

		switch loginType {
		case "local":
			cmd = exec.Command("pulumi", "login", "--local")

		case "cloud":
			cmd = exec.Command("pulumi", "login")

		case "diy":
			cmd = exec.Command("pulumi", "login", url)

		default:
			return pulumiLoginErrorMsg{errors.New("unknown login type")}
		}

		output, err := cmd.CombinedOutput()
		if err != nil {
			return pulumiLoginErrorMsg{fmt.Errorf("pulumi login failed: %w\n%s", err, string(output))}
		}

		return pulumiLoginSuccessMsg{output: string(output)}
	}
}

func (m pulumiLoginModel) View() string {
	var sb strings.Builder

	if m.err != nil {
		sb.WriteString(tui.ErrorStyle.Render("Pulumi Login Failed"))
		sb.WriteString("\n\n")
		sb.WriteString(m.err.Error() + "\n\n")
		sb.WriteString("🔧 How to fix this:\n")
		sb.WriteString("   1. Make sure Pulumi is installed: https://www.pulumi.com/docs/install/\n")
		sb.WriteString("   2. Check your Pulumi configuration and credentials\n")
		sb.WriteString("   3. If using Pulumi Cloud, verify your access token: " + tui.InfoStyle.Render("pulumi login") + "\n")
		sb.WriteString("   4. For local backends, ensure your storage path is accessible\n\n")
		sb.WriteString("For more information about Pulumi backends, see:\n")
		sb.WriteString(lipgloss.NewStyle().Foreground(tui.DrPurple).Render(pulumiDocsURL))
		sb.WriteString("\n")

		return sb.String()
	}

	switch m.currentScreen {
	case pulumiLoginScreenBackendSelection:
		sb.WriteString(tui.SubTitleStyle.Render("Pulumi State Backend Selection"))
		sb.WriteString("\n\n")
		sb.WriteString("Select where Pulumi should store your infrastructure state:\n\n")

		for i, option := range m.options {
			cursor := "  "

			if i == m.selectedOption {
				cursor = pulumiArrow.String() + " "
			}

			sb.WriteString(fmt.Sprintf("%s%s\n", cursor, option))
		}

		sb.WriteString("\n")
		sb.WriteString(tui.HintStyle.Render("↑/↓ to navigate • enter to select"))

	case pulumiLoginScreenDIYURL:
		sb.WriteString(tui.SubTitleStyle.Render("DIY Backend Configuration"))
		sb.WriteString("\n\n")
		sb.WriteString(fmt.Sprintf("For more information about backends, see:\n%s\n\n",
			lipgloss.NewStyle().Foreground(tui.DrPurple).Render(pulumiDocsURL)))
		sb.WriteString("Enter your backend URL:\n")
		sb.WriteString("Examples: s3://my-pulumi-bucket, azblob://..., gs://...\n\n")
		sb.WriteString(m.diyInput.View())
		sb.WriteString("\n\n")
		sb.WriteString(tui.HintStyle.Render("enter to continue • esc to go back"))

	case pulumiLoginScreenLoggingIn:
		sb.WriteString("Logging in to Pulumi...\n")

	case pulumiLoginScreenPassphrasePrompt:
		sb.WriteString(tui.SubTitleStyle.Render("Pulumi Configuration Passphrase"))
		sb.WriteString("\n\n")
		sb.WriteString("Would you like to set a default PULUMI_CONFIG_PASSPHRASE?\n")
		sb.WriteString("This will be used to encrypt secrets and stack variables.\n\n")
		sb.WriteString("We can auto-generate a strong passphrase and save it to your\n")
		sb.WriteString("DataRobot CLI config file (~/.config/datarobot/drconfig.yaml)\n\n")
		sb.WriteString(tui.BaseTextStyle.Render("enter or y to generate passphrase • n to skip"))
	}

	return sb.String()
}

// needsPulumiSetup returns true when the template requires Pulumi (has an active,
// non-hidden PULUMI_CONFIG_PASSPHRASE prompt), Pulumi is installed, and the user
// is either not logged in or has no passphrase configured.
func needsPulumiSetup(prompts []envbuilder.UserPrompt, loggedIn, passphraseSet bool) bool {
	if _, err := exec.LookPath("pulumi"); err != nil {
		return false
	}

	for _, p := range prompts {
		if p.Env == "PULUMI_CONFIG_PASSPHRASE" && p.Active && !p.Hidden {
			return !loggedIn || !passphraseSet
		}
	}

	return false
}

// isPulumiLoggedIn returns true if `pulumi whoami` succeeds (user is logged in).
func isPulumiLoggedIn() bool {
	ctx, cancel := context.WithTimeout(context.Background(), pulumiWhoamiTimeout)
	defer cancel()

	err := exec.CommandContext(ctx, "pulumi", "whoami", "--non-interactive").Run()

	return err == nil
}

// CheckPulumiSetup returns whether the Pulumi setup screen should be shown,
// whether the user is already logged in, and whether a passphrase needs to be
// configured. Call this before starting the dotenv TUI and set the corresponding
// Model fields.
func CheckPulumiSetup(dir string, variables []envbuilder.Variable) (needsSetup, alreadyLoggedIn, needsPassphrase bool) {
	prompts, err := envbuilder.GatherUserPrompts(dir, variables)
	if err != nil {
		return false, false, false
	}

	if _, err := exec.LookPath("pulumi"); err != nil {
		return false, false, false
	}

	loggedIn := isPulumiLoggedIn()
	passphraseSet := viper.GetString(pulumiConfigPassphraseKey) != ""

	return needsPulumiSetup(prompts, loggedIn, passphraseSet), loggedIn, !passphraseSet
}
