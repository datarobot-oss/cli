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

package start

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/viper"
)

const (
	generatedPassphraseLength = 32
	pulumiConfigPassphraseKey = "pulumi_config_passphrase"
	pulumiDocsURL             = "https://www.pulumi.com/docs/iac/concepts/state-and-backends/"
)

type pulumiLoginScreen int

const (
	pulumiLoginScreenBackendSelection pulumiLoginScreen = iota
	pulumiLoginScreenDIYURL
	pulumiLoginScreenPassphrasePrompt
	pulumiLoginScreenLoggingIn
)

// pulumiLoginModel handles the Pulumi login flow
type pulumiLoginModel struct {
	currentScreen       pulumiLoginScreen
	selectedOption      int
	options             []string
	diyInput            textinput.Model
	diyURL              string
	wantsPassphrase     bool
	generatedPassphrase string
	err                 error
	loginOutput         string
}

type (
	pulumiLoginCompleteMsg struct{}
	pulumiLoginErrorMsg    struct{ err error }
	pulumiLoginSuccessMsg  struct{ output string }
)

func newPulumiLoginModel() pulumiLoginModel {
	ti := textinput.New()
	ti.Placeholder = "s3://my-pulumi-bucket or azblob://..."
	ti.Focus()
	ti.Width = 60

	return pulumiLoginModel{
		currentScreen:  pulumiLoginScreenBackendSelection,
		selectedOption: 0,
		options:        []string{"Login locally", "Login to Pulumi Cloud", "DIY backend (S3, Azure Blob, etc.)"},
		diyInput:       ti,
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
		return m, func() tea.Msg { return pulumiLoginCompleteMsg{} }

	case pulumiLoginErrorMsg:
		m.err = msg.err
		return m, nil
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
		// User selected an option
		switch m.selectedOption {
		case 0: // Local
			m.currentScreen = pulumiLoginScreenPassphrasePrompt
		case 1: // Cloud
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

		m.currentScreen = pulumiLoginScreenPassphrasePrompt
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
	case "y", "Y":
		m.wantsPassphrase = true
		// Generate passphrase
		passphrase, err := generateRandomPassphrase(generatedPassphraseLength)
		if err != nil {
			m.err = fmt.Errorf("failed to generate passphrase: %w", err)
			return m, nil
		}

		m.generatedPassphrase = passphrase

		// Perform login
		switch m.selectedOption {
		case 0: // Local
			return m, m.performLogin("local", "")
		case 2: // DIY
			return m, m.performLogin("diy", m.diyURL)
		}
	case "n", "N":
		m.wantsPassphrase = false
		// Perform login without passphrase
		switch m.selectedOption {
		case 0: // Local
			return m, m.performLogin("local", "")
		case 2: // DIY
			return m, m.performLogin("diy", m.diyURL)
		}
	case "esc":
		// Go back
		if m.selectedOption == 2 { // DIY
			m.currentScreen = pulumiLoginScreenDIYURL
		} else {
			m.currentScreen = pulumiLoginScreenBackendSelection
		}
	}

	return m, nil
}

func (m pulumiLoginModel) savePassphraseToConfig() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".config", "datarobot")
	if err := os.MkdirAll(configDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
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

		// Set passphrase in config if requested
		if m.wantsPassphrase && m.generatedPassphrase != "" {
			if err := m.savePassphraseToConfig(); err != nil {
				return pulumiLoginErrorMsg{err}
			}
		}

		// Determine which pulumi login command to run
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

		// Run the command
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
		sb.WriteString(tui.ErrorStyle.Render("Error: ") + m.err.Error() + "\n")
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
				cursor = arrow.String() + " "
			}

			sb.WriteString(fmt.Sprintf("%s%s\n", cursor, option))
		}

		sb.WriteString("\n")
		sb.WriteString(tui.DimStyle.Render("↑/↓ to navigate • enter to select"))

	case pulumiLoginScreenDIYURL:
		sb.WriteString(tui.SubTitleStyle.Render("DIY Backend Configuration"))
		sb.WriteString("\n\n")
		sb.WriteString(fmt.Sprintf("For more information about backends, see:\n%s\n\n",
			lipgloss.NewStyle().Foreground(tui.DrPurple).Render(pulumiDocsURL)))
		sb.WriteString("Enter your backend URL:\n")
		sb.WriteString("Examples: s3://my-pulumi-bucket, azblob://..., gs://...\n\n")
		sb.WriteString(m.diyInput.View())
		sb.WriteString("\n\n")
		sb.WriteString(tui.DimStyle.Render("enter to continue • esc to go back"))

	case pulumiLoginScreenPassphrasePrompt:
		sb.WriteString(tui.SubTitleStyle.Render("Pulumi Configuration Passphrase"))
		sb.WriteString("\n\n")
		sb.WriteString("Would you like to set a default PULUMI_CONFIG_PASSPHRASE?\n")
		sb.WriteString("This will be used to encrypt secrets and stack variables.\n\n")
		sb.WriteString("We can auto-generate a strong passphrase and save it to your\n")
		sb.WriteString("DataRobot CLI config file (~/.config/datarobot/drconfig.yaml)\n\n")
		sb.WriteString(tui.DimStyle.Render("y to generate passphrase • n to skip • esc to go back"))

	case pulumiLoginScreenLoggingIn:
		sb.WriteString("Logging in to Pulumi...\n")
	}

	return sb.String()
}

func generateRandomPassphrase(length int) (string, error) {
	bytes := make([]byte, length)

	_, err := rand.Read(bytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}

// checkPulumiState checks if user is logged into Pulumi and guides them through login if needed
func checkPulumiState(m *Model) tea.Msg {
	// Check if pulumi is installed
	_, err := exec.LookPath("pulumi")
	if err != nil {
		// Pulumi not installed, skip this step
		return stepCompleteMsg{}
	}

	// Check if user is logged in using 'pulumi whoami'
	cmd := exec.Command("pulumi", "whoami")

	err = cmd.Run()
	if err == nil {
		// User is already logged in
		return stepCompleteMsg{}
	}

	// User is not logged in, need to guide them through login
	// Signal to the main model to enter Pulumi login sub-model
	return stepCompleteMsg{
		needPulumiLogin: true,
	}
}
