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
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/suite"
)

var testEnvFile = `
PULUMI_CONFIG_PASSPHRASE=

# If empty, a new use case will be created
DATAROBOT_DEFAULT_USE_CASE=


# See README instructions for getting Google and Box OAuth Apps
GOOGLE_CLIENT_ID=
GOOGLE_CLIENT_SECRET=

BOX_CLIENT_ID=
BOX_CLIENT_SECRET=


# INFRA_ENABLE_LLM=
`

var testConfigFile = `
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

	file, err := os.OpenFile(filepath.Join(dir, ".env.template"), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		suite.T().Errorf("Failed to create test env file: %v", err)
	}

	defer file.Close()

	_, err = file.WriteString(testEnvFile)
	if err != nil {
		suite.T().Errorf("Failed to write to test env file: %v", err)
	}

	datarobotDir := filepath.Join(dir, ".datarobot")

	err = os.MkdirAll(datarobotDir, os.ModePerm)
	if err != nil {
		suite.T().Errorf("Failed to create .datarobot directory: %v", err)
	}

	configFile := filepath.Join(datarobotDir, "parakeet.yaml")

	file2, err := os.OpenFile(configFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		suite.T().Errorf("Failed to create test YAML file: %v", err)
	}

	defer file2.Close()

	_, err = file2.WriteString(testConfigFile)
	if err != nil {
		suite.T().Errorf("Failed to write to test YAML file one: %v", err)
	}
}

func (suite *DotenvModelTestSuite) TestDotenvModel_Happy_Path() {
	m := Model{
		screen:         wizardScreen,
		DotenvFile:     filepath.Join(suite.tempDir, ".env"),
		DotenvTemplate: filepath.Join(suite.tempDir, ".env.template"),
	}
	tm := teatest.NewTestModel(suite.T(), m, teatest.WithInitialTermSize(300, 100))

	// Set default pulumi passphrase to 123
	teatest.WaitFor(
		suite.T(), tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte("Default: 123"))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*3),
	)

	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("123"),
	})
	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("enter"),
	})

	// Accept default for use case
	teatest.WaitFor(
		suite.T(), tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte("The default use case for this application"))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*3),
	)

	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("enter"),
	})

	// Leave data source blank
	teatest.WaitFor(
		suite.T(), tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte("The data source to use for this application"))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*3),
	)

	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("enter"),
	})

	teatest.WaitFor(
		suite.T(), tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte("Select the type of LLM integration to enable."))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*3),
	)

	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("down"),
	})

	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("enter"),
	})

	err := tm.Quit()
	if err != nil {
		suite.T().Error(err)
	}

	expectedFilePath := filepath.Join(suite.tempDir, ".env")

	finalModel := tm.FinalModel(suite.T())

	fm, ok := finalModel.(Model)
	if !ok {
		suite.T().Error("Final model is not of type Model")
	}

	suite.FileExists(expectedFilePath, "Expected environment file to be created at default path")
	suite.Contains(fm.contents, "PULUMI_CONFIG_PASSPHRASE=123", "Expected env file to contain the entered passphrase")
	suite.Contains(fm.contents, "DATAROBOT_DEFAULT_USE_CASE=", "Expected env file to contain the default use case")
	suite.Contains(fm.contents, "INFRA_ENABLE_LLM=blueprint_with_llm_gateway.py", "Expected env file to contain the selected LLM option")
}

func (suite *DotenvModelTestSuite) TestDotenvModel_Branching_Path() {
	m := Model{
		screen:         wizardScreen,
		DotenvFile:     filepath.Join(suite.tempDir, ".env"),
		DotenvTemplate: filepath.Join(suite.tempDir, ".env.template"),
	}
	tm := teatest.NewTestModel(suite.T(), m, teatest.WithInitialTermSize(300, 100))

	// Set default pulumi passphrase to 123
	teatest.WaitFor(
		suite.T(), tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte("Default: 123"))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*3),
	)

	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("123"),
	})
	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("enter"),
	})

	// Accept default for use case
	teatest.WaitFor(
		suite.T(), tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte("The default use case for this application"))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*3),
	)

	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("enter"),
	})

	// Leave data source blank
	teatest.WaitFor(
		suite.T(), tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte("The data source to use for this application"))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*3),
	)

	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("down"),
	})

	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("enter"),
	})

	teatest.WaitFor(
		suite.T(), tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte("The client ID for the Google data source."))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*3),
	)

	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("parakeet_id"),
	})

	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("enter"),
	})

	teatest.WaitFor(
		suite.T(), tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte("The client secret for the Google data source."))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*3),
	)

	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("parakeet_secret"),
	})

	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("enter"),
	})

	teatest.WaitFor(
		suite.T(), tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte("LLM Gateway"))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*3),
	)

	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("down"),
	})

	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("enter"),
	})

	err := tm.Quit()
	if err != nil {
		suite.T().Error(err)
	}

	expectedFilePath := filepath.Join(suite.tempDir, ".env")

	finalModel := tm.FinalModel(suite.T())

	fm, ok := finalModel.(Model)
	if !ok {
		suite.T().Error("Final model is not of type Model")
	}

	suite.FileExists(expectedFilePath, "Expected environment file to be created at default path")
	suite.Contains(fm.contents, "PULUMI_CONFIG_PASSPHRASE=123", "Expected env file to contain the entered passphrase")
	suite.Contains(fm.contents, "DATAROBOT_DEFAULT_USE_CASE=", "Expected env file to contain the default use case")
	suite.Contains(fm.contents, "INFRA_ENABLE_LLM=blueprint_with_llm_gateway.py", "Expected env file to contain the selected LLM option")
	suite.Contains(fm.contents, "GOOGLE_CLIENT_ID=parakeet_id", "Expected env file to contain the entered Google client ID")
	suite.Contains(fm.contents, "GOOGLE_CLIENT_SECRET=parakeet_secret", "Expected env file to contain the entered Google client secret")
}
