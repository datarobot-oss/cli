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
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/suite"
)

func TestHostModelSuite(t *testing.T) {
	suite.Run(t, new(HostModelTestSuite))
}

type HostModelTestSuite struct {
	suite.Suite
}

// hostModelWrapper wraps HostModel to satisfy tea.Model interface
type hostModelWrapper struct {
	HostModel
}

func (w hostModelWrapper) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := w.HostModel.Update(msg)
	w.HostModel = model

	return w, cmd
}

func (suite *HostModelTestSuite) NewTestModel(m HostModel) *teatest.TestModel {
	wrapper := hostModelWrapper{HostModel: m}

	return teatest.NewTestModel(suite.T(), wrapper, teatest.WithInitialTermSize(300, 100))
}

func (suite *HostModelTestSuite) WaitFor(tm *teatest.TestModel, contains string) {
	teatest.WaitFor(
		suite.T(), tm.Output(),
		func(bts []byte) bool {
			return bytes.Contains(bts, []byte(contains))
		},
		teatest.WithCheckInterval(time.Millisecond*100),
		teatest.WithDuration(time.Second*3),
	)
}

func (suite *HostModelTestSuite) Quit(tm *teatest.TestModel) {
	err := tm.Quit()
	if err != nil {
		suite.T().Error(err)
	}
}

func (suite *HostModelTestSuite) TestHostModel_Init() {
	m := NewHostModel()

	suite.Equal(80, m.width)
	suite.False(m.showCustom)
	suite.NotNil(m.list)
	suite.NotNil(m.customInput)

	// Verify list has 4 items
	suite.Len(m.list.Items(), 4)
}

func (suite *HostModelTestSuite) TestHostModel_SelectUSCloud() {
	m := NewHostModel()

	var capturedURL string

	m.SuccessCmd = func(url string) tea.Cmd {
		return func() tea.Msg {
			capturedURL = url
			return nil
		}
	}

	tm := suite.NewTestModel(m)

	// Send window size to initialize the view
	tm.Send(tea.WindowSizeMsg{Width: 300, Height: 100})

	suite.WaitFor(tm, "US Cloud")

	// US Cloud is already selected by default (first item)
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	time.Sleep(100 * time.Millisecond) // Give time for the command to execute

	suite.Equal("https://app.datarobot.com", capturedURL)
	suite.Quit(tm)
}

func (suite *HostModelTestSuite) TestHostModel_SelectEUCloud() {
	m := NewHostModel()

	var capturedURL string

	m.SuccessCmd = func(url string) tea.Cmd {
		return func() tea.Msg {
			capturedURL = url
			return nil
		}
	}

	tm := suite.NewTestModel(m)

	// Send window size to initialize the view
	tm.Send(tea.WindowSizeMsg{Width: 300, Height: 100})

	suite.WaitFor(tm, "US Cloud")

	// Navigate to EU Cloud (second item)
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	time.Sleep(100 * time.Millisecond)

	suite.Equal("https://app.eu.datarobot.com", capturedURL)
	suite.Quit(tm)
}

func (suite *HostModelTestSuite) TestHostModel_SelectJapanCloud() {
	m := NewHostModel()

	var capturedURL string

	m.SuccessCmd = func(url string) tea.Cmd {
		return func() tea.Msg {
			capturedURL = url
			return nil
		}
	}

	tm := suite.NewTestModel(m)

	// Send window size to initialize the view
	tm.Send(tea.WindowSizeMsg{Width: 300, Height: 100})

	suite.WaitFor(tm, "US Cloud")

	// Navigate to Japan Cloud (third item)
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	time.Sleep(100 * time.Millisecond)

	suite.Equal("https://app.jp.datarobot.com", capturedURL)
	suite.Quit(tm)
}

func (suite *HostModelTestSuite) TestHostModel_NavigateToCustom() {
	m := NewHostModel()

	tm := suite.NewTestModel(m)

	// Send window size to initialize the view
	tm.Send(tea.WindowSizeMsg{Width: 300, Height: 100})

	suite.WaitFor(tm, "US Cloud")

	// Navigate to Custom/On-Prem (fourth item)
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	time.Sleep(100 * time.Millisecond)

	// Should switch to custom input screen
	suite.WaitFor(tm, "Custom DataRobot URL")

	suite.Quit(tm)
}

func (suite *HostModelTestSuite) TestHostModel_CustomURLInput() {
	m := NewHostModel()

	var capturedURL string

	m.SuccessCmd = func(url string) tea.Cmd {
		return func() tea.Msg {
			capturedURL = url
			return nil
		}
	}

	tm := suite.NewTestModel(m)

	// Send window size to initialize the view
	tm.Send(tea.WindowSizeMsg{Width: 300, Height: 100})

	suite.WaitFor(tm, "US Cloud")

	// Navigate to Custom/On-Prem and enter it
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	suite.WaitFor(tm, "Custom DataRobot URL")

	// Type custom URL
	customURL := "https://custom.datarobot.com"
	for _, ch := range customURL {
		tm.Send(tea.KeyMsg{
			Type:  tea.KeyRunes,
			Runes: []rune{ch},
		})
	}

	// Submit
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	time.Sleep(100 * time.Millisecond)

	suite.Equal(customURL, capturedURL)
	suite.Quit(tm)
}

func (suite *HostModelTestSuite) TestHostModel_CustomURLEscape() {
	m := NewHostModel()

	tm := suite.NewTestModel(m)

	// Send window size to initialize the view
	tm.Send(tea.WindowSizeMsg{Width: 300, Height: 100})

	suite.WaitFor(tm, "US Cloud")

	// Navigate to Custom/On-Prem and enter it
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	suite.WaitFor(tm, "Custom DataRobot URL")

	// Type some text
	for _, ch := range "https://test.com" {
		tm.Send(tea.KeyMsg{
			Type:  tea.KeyRunes,
			Runes: []rune{ch},
		})
	}

	// Press Esc to go back
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})

	// Should return to list view
	suite.WaitFor(tm, "US Cloud")

	suite.Quit(tm)
}

func (suite *HostModelTestSuite) TestHostModel_CustomURLEmptySubmit() {
	m := NewHostModel()

	var capturedURL string

	m.SuccessCmd = func(url string) tea.Cmd {
		return func() tea.Msg {
			capturedURL = url
			return nil
		}
	}

	tm := suite.NewTestModel(m)

	// Send window size to initialize the view
	tm.Send(tea.WindowSizeMsg{Width: 300, Height: 100})

	suite.WaitFor(tm, "US Cloud")

	// Navigate to Custom/On-Prem and enter it
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	suite.WaitFor(tm, "Custom DataRobot URL")

	// Press Enter without typing anything
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	time.Sleep(100 * time.Millisecond)

	// Should not have captured any URL
	suite.Empty(capturedURL)

	suite.Quit(tm)
}

func (suite *HostModelTestSuite) TestHostModel_WindowResize() {
	m := NewHostModel()

	// Send window resize message
	msg := tea.WindowSizeMsg{
		Width:  120,
		Height: 50,
	}

	updatedModel, _ := m.Update(msg)

	suite.Equal(120, updatedModel.width)
}

func (suite *HostModelTestSuite) TestHostModel_ListNavigation() {
	m := NewHostModel()

	tm := suite.NewTestModel(m)

	// Send window size to initialize the view
	tm.Send(tea.WindowSizeMsg{Width: 300, Height: 100})

	suite.WaitFor(tm, "US Cloud")

	// Navigate down
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	time.Sleep(50 * time.Millisecond)

	// Navigate up
	tm.Send(tea.KeyMsg{Type: tea.KeyUp})
	time.Sleep(50 * time.Millisecond)

	// Use j key (vim-style down)
	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune{'j'},
	})
	time.Sleep(50 * time.Millisecond)

	// Use k key (vim-style up)
	tm.Send(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune{'k'},
	})
	time.Sleep(50 * time.Millisecond)

	suite.Quit(tm)
}

func (suite *HostModelTestSuite) TestHostModel_ViewListMode() {
	m := NewHostModel()
	m.showCustom = false

	view := m.View()

	// View should contain list content
	suite.Contains(view, "US Cloud")
	suite.NotContains(view, "Custom DataRobot URL")
}

func (suite *HostModelTestSuite) TestHostModel_ViewCustomMode() {
	m := NewHostModel()
	m.showCustom = true

	view := m.View()

	// View should contain custom input content
	suite.Contains(view, "Custom DataRobot URL")
	suite.Contains(view, "Enter your DataRobot URL")
	suite.Contains(view, "Press Enter to continue or Esc to go back")
}

func (suite *HostModelTestSuite) TestHostModel_InitReturnsNil() {
	m := NewHostModel()

	cmd := m.Init()

	suite.Nil(cmd)
}
