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
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v3"
)

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

	viper.Reset()
}

func (suite *LoginModelTestSuite) TestLoginModel_Init_Press_1() {
	m := NewModel()
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
	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("enter"),
	})
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

	_ = yaml.Unmarshal(yamlFile, &yamlData)
	suite.Equal("https://app.datarobot.com/api/v2", yamlData["endpoint"], "Expected config file to have the selected host")
}

func (suite *LoginModelTestSuite) TestLoginModel_Init_Press_2() {
	m := NewModel()
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
	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("enter"),
	})
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

	_ = yaml.Unmarshal(yamlFile, &yamlData)
	suite.Equal("https://app.eu.datarobot.com/api/v2", yamlData["endpoint"], "Expected config file to have the selected host")
}

func (suite *LoginModelTestSuite) TestLoginModel_Init_Press_3() {
	m := NewModel()
	tm := teatest.NewTestModel(suite.T(), m, teatest.WithInitialTermSize(300, 100))

	teatest.WaitFor(
		suite.T(), tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte("https://app.jp.datarobot.com"))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*3),
	)

	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("3"),
	})
	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("enter"),
	})
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

	_ = yaml.Unmarshal(yamlFile, &yamlData)
	suite.Equal("https://app.jp.datarobot.com/api/v2", yamlData["endpoint"], "Expected config file to have the selected host")
}

func (suite *LoginModelTestSuite) TestLoginModel_Init_Custom_URL() {
	m := NewModel()
	tm := teatest.NewTestModel(suite.T(), m, teatest.WithInitialTermSize(300, 100))

	teatest.WaitFor(
		suite.T(), tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte("https://app.jp.datarobot.com"))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*3),
	)

	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("https://app.parakeet.datarobot.com"),
	})
	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("enter"),
	})
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

	_ = yaml.Unmarshal(yamlFile, &yamlData)
	suite.Equal("https://app.parakeet.datarobot.com/api/v2", yamlData["endpoint"], "Expected config file to have the selected host")
}

func (suite *LoginModelTestSuite) TestLoginModel_Init_Non_URL() {
	m := NewModel()
	tm := teatest.NewTestModel(suite.T(), m, teatest.WithInitialTermSize(300, 100))

	teatest.WaitFor(
		suite.T(), tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte("https://app.jp.datarobot.com"))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*3),
	)

	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("squak-squak"),
	})
	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("enter"),
	})
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

	_ = yaml.Unmarshal(yamlFile, &yamlData)
	suite.Equal("", yamlData["endpoint"], "Expected config file to have the selected host")
}
