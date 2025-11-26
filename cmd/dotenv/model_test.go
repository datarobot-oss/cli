// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.
package dotenv

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/datarobot/cli/internal/envbuilder"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
)

var parakeetYamlContents = `
root:
  - env: PULUMI_CONFIG_PASSPHRASE
    type: string
    default: 123
    optional: true
    help: "The passphrase used to encrypt and decrypt the private key. This value is required if you're not using pulumi cloud."
  - env: DATAROBOT_DEFAULT_USE_CASE
    type: string
    default:
    optional: true
    help: "The default use case for this application. If not set, a new use case will be created automatically"
  - type: string
    default:
    optional: true
    multiple: true
    help: "The data source to use for this application."
    options:
      - name: "Google"
        requires: google_data_source
      - name: "Box"
        requires: box_data_source
  - env: INFRA_ENABLE_LLM
    type: string
    optional: true
    help: "Select the type of LLM integration to enable."
    options:
      - name: "LLM Gateway"
        value: "blueprint_with_llm_gateway.py"
      - name: "DataRobot Deployed LLM"
        value: "deployed_llm.py"
        requires: deployed_llm
      - name: "Registered Model with an LLM Blueprint"
        value: "registered_model.py"
        requires: registered_model
      - name: "External LLM"
        value: "blueprint_with_external_llm.py"
        requires: external_llm

google_data_source:
  - env: GOOGLE_CLIENT_ID
    type: string
    default:
    optional: false
    help: "The client ID for the Google data source."
  - env: GOOGLE_CLIENT_SECRET
    type: string
    default:
    optional: false
    help: "The client secret for the Google data source."

box_data_source:
  - env: BOX_CLIENT_ID
    type: string
    default:
    optional: false
    help: "The client ID for the Box data source."
  - env: BOX_CLIENT_SECRET
    type: string
    default:
    optional: false
    help: "The client secret for the Box data source."
`

func TestDotenvModelSuite(t *testing.T) {
	suite.Run(t, new(DotenvModelTestSuite))
}

type DotenvModelTestSuite struct {
	suite.Suite
	tempDir string
}

func (suite *DotenvModelTestSuite) SetupTest() {
	dir, _ := os.MkdirTemp("", "datarobot-config-test")
	suite.tempDir = dir

	datarobotDir := filepath.Join(dir, ".datarobot")

	err := os.MkdirAll(datarobotDir, os.ModePerm)
	if err != nil {
		suite.T().Errorf("Failed to create .datarobot directory: %v", err)
	}

	parakeetYamlName := filepath.Join(datarobotDir, "parakeet.yaml")

	parakeetYamlFile, err := os.OpenFile(parakeetYamlName, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		suite.T().Errorf("Failed to create test YAML file: %v", err)
	}

	defer parakeetYamlFile.Close()

	_, err = parakeetYamlFile.WriteString(parakeetYamlContents)
	if err != nil {
		suite.T().Errorf("Failed to write to test YAML file one: %v", err)
	}
}

func (suite *DotenvModelTestSuite) NewTestModel(m Model) *teatest.TestModel {
	return teatest.NewTestModel(suite.T(), m, teatest.WithInitialTermSize(300, 100))
}

func (suite *DotenvModelTestSuite) Send(tm *teatest.TestModel, keys ...string) {
	for _, key := range keys {
		tm.Send(tea.KeyMsg{
			Type:  tea.KeyRunes,
			Runes: []rune(key),
		})
	}
}

func (suite *DotenvModelTestSuite) WaitFor(tm *teatest.TestModel, contains string) {
	teatest.WaitFor(
		suite.T(), tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte(contains))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*3),
	)
}

func (suite *DotenvModelTestSuite) FinalModel(tm *teatest.TestModel) Model {
	err := tm.Quit()
	if err != nil {
		suite.T().Error(err)
	}

	finalModel := tm.FinalModel(suite.T())

	fm, ok := finalModel.(Model)
	if !ok {
		suite.T().Error("Final model is not of type Model")
	}

	return fm
}

func (suite *DotenvModelTestSuite) TestDotenvModel_Happy_Path() {
	tm := suite.NewTestModel(Model{
		screen:     wizardScreen,
		DotenvFile: filepath.Join(suite.tempDir, ".env"),
	})

	// Set default pulumi passphrase to 123
	suite.WaitFor(tm, "Default: 123")
	suite.Send(tm, "backspace", "backspace", "backspace", "456", "enter")

	// Accept default for use case
	suite.WaitFor(tm, "The default use case for this application")
	suite.Send(tm, "case", "enter")

	// Leave data source blank
	suite.WaitFor(tm, "The data source to use for this application")
	suite.Send(tm, "enter")

	suite.WaitFor(tm, "Select the type of LLM integration to enable.")
	suite.Send(tm, "down")
	suite.WaitFor(tm, "> LLM Gateway")
	suite.Send(tm, "enter")

	// Wait for the file write to complete and return to list screen
	suite.WaitFor(tm, "Variables found in")

	// Exit list screen
	suite.Send(tm, "enter")

	fm := suite.FinalModel(tm)

	suite.Equal(filepath.Join(suite.tempDir, ".env"), fm.DotenvFile)

	actualContents, err := os.ReadFile(fm.DotenvFile)
	suite.Require().NoError(err, "Expected to read .env file")

	actualContentsStr := string(actualContents)

	suite.Contains(actualContentsStr, "PULUMI_CONFIG_PASSPHRASE=\"456\"\n", "Expected env file to contain the entered passphrase")
	suite.Contains(actualContentsStr, "DATAROBOT_DEFAULT_USE_CASE=\"case\"\n", "Expected env file to contain the default use case")
	suite.Contains(actualContentsStr, "INFRA_ENABLE_LLM=\"blueprint_with_llm_gateway.py\"\n", "Expected env file to contain the selected LLM option")

	os.Remove(fm.DotenvFile)
}

func (suite *DotenvModelTestSuite) TestDotenvModel_Branching_Path() {
	tm := suite.NewTestModel(Model{
		screen:     wizardScreen,
		DotenvFile: filepath.Join(suite.tempDir, ".env"),
	})

	// Set default pulumi passphrase to 123
	suite.WaitFor(tm, "Default: 123")
	suite.Send(tm, "backspace", "backspace", "backspace", "456", "enter")

	// Accept default for use case
	suite.WaitFor(tm, "The default use case for this application")
	suite.Send(tm, "case", "enter")

	// Set data source to google
	suite.WaitFor(tm, "The data source to use for this application")
	suite.Send(tm, "down")
	suite.WaitFor(tm, "> [ ] Google")
	suite.Send(tm, " ")
	suite.WaitFor(tm, "> [x] Google")
	suite.Send(tm, "enter")

	suite.WaitFor(tm, "The client ID for the Google data source.")
	suite.Send(tm, "google_parakeet_id", "enter")
	suite.WaitFor(tm, "The client secret for the Google data source.")
	suite.Send(tm, "google_parakeet_secret", "enter")

	suite.WaitFor(tm, "Select the type of LLM integration to enable.")
	suite.Send(tm, "down")
	suite.WaitFor(tm, "> LLM Gateway")
	suite.Send(tm, "enter")

	// Wait for the file write to complete and return to list screen
	suite.WaitFor(tm, "Variables found in")

	// Exit list screen
	suite.Send(tm, "enter")

	fm := suite.FinalModel(tm)

	suite.Equal(filepath.Join(suite.tempDir, ".env"), fm.DotenvFile)

	actualContents, err := os.ReadFile(fm.DotenvFile)
	suite.Require().NoError(err, "Expected to read .env file")

	actualContentsStr := string(actualContents)
	suite.Contains(actualContentsStr, "PULUMI_CONFIG_PASSPHRASE=\"456\"\n", "Expected env file to contain the entered passphrase")
	suite.Contains(actualContentsStr, "DATAROBOT_DEFAULT_USE_CASE=\"case\"\n", "Expected env file to contain the default use case")
	suite.Contains(actualContentsStr, "INFRA_ENABLE_LLM=\"blueprint_with_llm_gateway.py\"\n", "Expected env file to contain the selected LLM option")
	suite.Contains(actualContentsStr, "GOOGLE_CLIENT_ID=\"google_parakeet_id\"\n", "Expected env file to contain the entered Google client ID")
	suite.Contains(actualContentsStr, "# The client ID for the Google data source.", "Expected env file to have the 'help' entry from YAML as comment.")
	suite.Contains(actualContentsStr, "GOOGLE_CLIENT_SECRET=\"google_parakeet_secret\"\n", "Expected env file to contain the entered Google client secret")

	os.Remove(fm.DotenvFile)
}

func (suite *DotenvModelTestSuite) TestDotenvModel_Both_Path() {
	tm := suite.NewTestModel(Model{
		screen:     wizardScreen,
		DotenvFile: filepath.Join(suite.tempDir, ".env"),
	})

	// Set default pulumi passphrase to 123
	suite.WaitFor(tm, "Default: 123")
	suite.Send(tm, "backspace", "backspace", "backspace", "456", "enter")

	// Accept default for use case
	suite.WaitFor(tm, "The default use case for this application")
	suite.Send(tm, "case", "enter")

	// Set data source to google and box
	suite.WaitFor(tm, "The data source to use for this application")
	suite.Send(tm, "down")
	suite.WaitFor(tm, "> [ ] Google")
	suite.Send(tm, " ")
	suite.WaitFor(tm, "> [x] Google")

	suite.Send(tm, "down")
	suite.WaitFor(tm, "> [ ] Box")
	suite.Send(tm, " ")
	suite.WaitFor(tm, "> [x] Box")
	suite.Send(tm, "enter")

	suite.WaitFor(tm, "The client ID for the Google data source.")
	suite.Send(tm, "google_parakeet_id", "enter")
	suite.WaitFor(tm, "The client secret for the Google data source.")
	suite.Send(tm, "google_parakeet_secret", "enter")

	suite.WaitFor(tm, "The client ID for the Box data source.")
	suite.Send(tm, "box_parakeet_id", "enter")
	suite.WaitFor(tm, "The client secret for the Box data source.")
	suite.Send(tm, "box_parakeet_secret", "enter")

	suite.WaitFor(tm, "Select the type of LLM integration to enable.")
	suite.Send(tm, "down")
	suite.WaitFor(tm, "> LLM Gateway")
	suite.Send(tm, "enter")

	// Wait for the file write to complete and return to list screen
	suite.WaitFor(tm, "Variables found in")

	// Exit list screen
	suite.Send(tm, "enter")

	fm := suite.FinalModel(tm)

	suite.Equal(filepath.Join(suite.tempDir, ".env"), fm.DotenvFile)

	actualContents, err := os.ReadFile(fm.DotenvFile)
	suite.Require().NoError(err, "Expected to read .env file")

	actualContentsStr := string(actualContents)
	suite.Contains(actualContentsStr, "PULUMI_CONFIG_PASSPHRASE=\"456\"\n", "Expected env file to contain the entered passphrase")
	suite.Contains(actualContentsStr, "DATAROBOT_DEFAULT_USE_CASE=\"case\"\n", "Expected env file to contain the default use case")
	suite.Contains(actualContentsStr, "INFRA_ENABLE_LLM=\"blueprint_with_llm_gateway.py\"\n", "Expected env file to contain the selected LLM option")
	suite.Contains(actualContentsStr, "GOOGLE_CLIENT_ID=\"google_parakeet_id\"\n", "Expected env file to contain the entered Google client ID")
	suite.Contains(actualContentsStr, "# The client ID for the Google data source.", "Expected env file to have the 'help' entry from YAML as comment.")
	suite.Contains(actualContentsStr, "GOOGLE_CLIENT_SECRET=\"google_parakeet_secret\"\n", "Expected env file to contain the entered Google client secret")
	suite.Contains(actualContentsStr, "BOX_CLIENT_ID=\"box_parakeet_id\"\n", "Expected env file to contain the entered Box client ID")
	suite.Contains(actualContentsStr, "BOX_CLIENT_SECRET=\"box_parakeet_secret\"\n", "Expected env file to contain the entered Box client secret")

	os.Remove(fm.DotenvFile)
}

func (suite *DotenvModelTestSuite) Test__loadPromptsFindsEnvValues() {
	suite.T().Setenv("DATAROBOT_DEFAULT_USE_CASE", "existing_use_case")
	suite.T().Setenv("PULUMI_CONFIG_PASSPHRASE", "existing_passphrase")
	tm := suite.NewTestModel(Model{
		screen:     wizardScreen,
		DotenvFile: filepath.Join(suite.tempDir, ".env"),
	})

	suite.WaitFor(tm, "Default: 123")

	err := tm.Quit()
	if err != nil {
		suite.T().Error(err)
	}

	m := tm.FinalModel(suite.T())

	usecaseIndex := slices.IndexFunc(m.(Model).prompts, func(p envbuilder.UserPrompt) bool {
		return p.Env == "DATAROBOT_DEFAULT_USE_CASE"
	})
	usecaseValue := m.(Model).prompts[usecaseIndex].Value
	suite.Equal("existing_use_case", usecaseValue, "Expected existing use case to be detected")

	pulumiIndex := slices.IndexFunc(m.(Model).prompts, func(p envbuilder.UserPrompt) bool {
		return p.Env == "PULUMI_CONFIG_PASSPHRASE"
	})
	pulumiValue := m.(Model).prompts[pulumiIndex].Value
	suite.Equal("existing_passphrase", pulumiValue, "Expected existing passphrase to be detected")
}

func (suite *DotenvModelTestSuite) Test__externalEditorCmd() {
	// The function under test is Model.externalEditorCmd()
	// and is dependent ONLY on the global viper config.
	m := Model{
		DotenvFile: "/path/to/dotenv/file",
	}

	// Test VISUAL takes precedence
	suite.T().Setenv("VISUAL", "nano")
	suite.T().Setenv("EDITOR", "vim")
	// Bind the env vars to viper
	_ = viper.BindEnv("external-editor", "VISUAL", "EDITOR")

	cmd := m.externalEditorCmd()
	suite.Contains(cmd.Path, "nano", "Expected VISUAL to take precedence")
	suite.Equal([]string{"nano", m.DotenvFile}, cmd.Args, "Expected correct arguments")

	// Test EDITOR fallback
	suite.T().Setenv("VISUAL", "")
	// Bind the env vars to viper
	_ = viper.BindEnv("external-editor", "VISUAL", "EDITOR")

	cmd = m.externalEditorCmd()
	suite.Contains(cmd.Path, "vim", "Expected EDITOR as fallback")

	// Test when neither is set
	suite.T().Setenv("EDITOR", "")
	// Bind the env vars to viper
	_ = viper.BindEnv("external-editor", "VISUAL", "EDITOR")

	cmd = m.externalEditorCmd()
	suite.Contains(cmd.Path, "", "Expected empty editor when none is set")

	// Test default value
	viper.SetDefault("external-editor", "vi")
	// Bind the env vars to viper; this should not override the default
	_ = viper.BindEnv("external-editor", "VISUAL", "EDITOR")

	cmd = m.externalEditorCmd()
	suite.Contains(cmd.Path, "vi", "Expected vi as default fallback")
}
