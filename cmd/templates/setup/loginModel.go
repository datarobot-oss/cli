// Copyright 2026 DataRobot, Inc. and its affiliates.
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

package setup

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
)

type LoginModel struct {
	loginMessage string
	flow         *auth.BrowserFlow
	APIKeyChan   chan string
	err          error
	GetHostCmd   tea.Cmd
	SuccessCmd   tea.Cmd
}

type errMsg struct{ error } //nolint: errname

type startedMsg struct {
	flow    *auth.BrowserFlow
	message string
}

// startLogin binds the callback listener, opens the browser, and renders either
// the "browser is opening" hint or, when the browser could not be launched, the
// link the user has to follow by hand.
func startLogin(datarobotHost string) tea.Cmd {
	return func() tea.Msg {
		flow, err := auth.NewBrowserFlow(datarobotHost)
		if err != nil {
			return errMsg{err}
		}

		openErr := flow.OpenBrowser()
		if openErr != nil {
			log.Debugf("Could not open the browser automatically: %v", openErr)
		}

		return startedMsg{
			flow:    flow,
			message: auth.RenderBrowserPrompt(flow.AuthURL(), auth.BrowserStateFor(openErr)),
		}
	}
}

// waitForAPIKey blocks on the callback and persists the key once it arrives.
func (lm LoginModel) waitForAPIKey() tea.Cmd {
	return func() tea.Msg {
		defer func() {
			if closeErr := lm.flow.Close(); closeErr != nil {
				log.Debugf("%v", closeErr)
			}
		}()

		apiKey, err := lm.flow.Wait(context.Background())
		if err != nil {
			return errMsg{err}
		}

		viperx.Set(config.DataRobotAPIKey, apiKey)

		if err := auth.WriteConfigFileSilent(); err != nil {
			return errMsg{fmt.Errorf("Error during writing config file: %w", err)}
		}

		return lm.SuccessCmd()
	}
}

func (lm LoginModel) Init() tea.Cmd {
	datarobotHost := config.GetBaseURL()
	if datarobotHost == "" {
		return lm.GetHostCmd
	}

	return startLogin(datarobotHost)
}

func (lm LoginModel) Update(msg tea.Msg) (LoginModel, tea.Cmd) {
	switch msg := msg.(type) {
	case startedMsg:
		lm.loginMessage = msg.message
		lm.flow = msg.flow

		return lm, lm.waitForAPIKey()

	case errMsg:
		lm.err = msg

		return lm, nil

	default:
		return lm, nil
	}
}

func (lm LoginModel) View() string {
	var sb strings.Builder

	if lm.loginMessage != "" {
		sb.WriteString(lm.loginMessage)
	} else if lm.err != nil {
		fmt.Fprintf(&sb, "something went wrong: %s", lm.err)
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// Close releases the callback listener. It is safe to call before the flow has
// started, which happens when the user presses Esc to change the DataRobot URL
// while the login screen is still initialising.
func (lm LoginModel) Close() {
	if lm.flow == nil {
		return
	}

	if err := lm.flow.Close(); err != nil {
		log.Debugf("%v", err)
	}
}
