// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.
package setup

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/datarobot/cli/internal/config"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v3"
)

type TestModel struct {
	login LoginModel
}

type testAuthSuccessMsg struct{}

func testAuthSuccess() tea.Msg {
	return testAuthSuccessMsg{}
}

func testModel() TestModel {
	return TestModel{
		login: LoginModel{
			APIKeyChan: make(chan string, 1),
			SuccessCmd: testAuthSuccess,
		},
	}
}

func (m TestModel) Init() tea.Cmd {
	return m.login.Init()
}

func (m TestModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.login, cmd = m.login.Update(msg)
	return m, cmd
}

func (m TestModel) View() string {
	return m.login.View()
}

func TestLoginModelSuite(t *testing.T) {
	suite.Run(t, new(LoginModelTestSuite))
}

type LoginModelTestSuite struct {
	suite.Suite
	tempDir    string
	configFile string
}

func (suite *LoginModelTestSuite) SetupTest() {
	dir, _ := os.MkdirTemp("", "datarobot-config-test")
	suite.tempDir = dir
	suite.T().Setenv("HOME", suite.tempDir)
	suite.configFile = filepath.Join(suite.tempDir, ".config/datarobot/drconfig.yaml")
	err := config.ReadConfigFile("")
	if err != nil {
		suite.T().Errorf("Failed to read config file: %v", err)
	}
}

func (suite *LoginModelTestSuite) AfterTest(suiteName, testName string) {
	suite.T().Logf("AfterTest: %s - %s finished", suiteName, testName)
	os.RemoveAll(suite.tempDir) // Clean up the temporary directory after each test
	dir, _ := os.MkdirTemp("", "datarobot-config-test")
	suite.tempDir = dir
	suite.T().Setenv("HOME", suite.tempDir)
	suite.configFile = filepath.Join(suite.tempDir, ".config/datarobot/drconfig.yaml")

}

func (suite *LoginModelTestSuite) TestLoginModel_Init_Press_1() {

	m := testModel()
	tm := teatest.NewTestModel(suite.T(), m, teatest.WithInitialTermSize(300, 100))

	teatest.WaitFor(
		suite.T(), tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte("https://app.datarobot.com"))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*3),
	)

	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("1"),
	})

	teatest.WaitFor(
		suite.T(), tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte("Please visit this link to connect your DataRobot credentials to the CLI"))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*10),
	)

	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("ctrl+c"),
	})

	err := tm.Quit()

	if err != nil {
		suite.T().Error(err)
	}

	expectedFilePath := filepath.Join(suite.tempDir, ".config/datarobot/drconfig.yaml")
	suite.FileExists(expectedFilePath, "Expected config file to be created at default path")
	yamlFile, _ := os.ReadFile(expectedFilePath)

	yamlData := make(map[string]string)

	err = yaml.Unmarshal(yamlFile, &yamlData)
	suite.Equal("https://app.datarobot.com/api/v2", yamlData["endpoint"], "Expected config file to have the selected host")

}

func (suite *LoginModelTestSuite) TestLoginModel_Init_Press_2() {

	m := testModel()
	tm := teatest.NewTestModel(suite.T(), m, teatest.WithInitialTermSize(300, 100))

	teatest.WaitFor(
		suite.T(), tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte("https://app.eu.datarobot.com"))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*3),
	)

	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("2"),
	})

	teatest.WaitFor(
		suite.T(), tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte("Please visit this link to connect your DataRobot credentials to the CLI"))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*10),
	)

	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("ctrl+c"),
	})

	err := tm.Quit()

	if err != nil {
		suite.T().Error(err)
	}

	expectedFilePath := filepath.Join(suite.tempDir, ".config/datarobot/drconfig.yaml")
	suite.FileExists(expectedFilePath, "Expected config file to be created at default path")
	yamlFile, _ := os.ReadFile(expectedFilePath)

	yamlData := make(map[string]string)

	err = yaml.Unmarshal(yamlFile, &yamlData)
	suite.Equal("https://app.eu.datarobot.com/api/v2", yamlData["endpoint"], "Expected config file to have the selected host")

}
