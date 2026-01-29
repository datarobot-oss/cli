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

package setup

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/assets"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/misc/open"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/viper"
)

type LoginModel struct {
	loginMessage string
	server       *http.Server
	APIKeyChan   chan string
	err          error
	GetHostCmd   tea.Cmd
	SuccessCmd   tea.Cmd
}

type errMsg struct{ error } //nolint: errname

type startedMsg struct {
	server  *http.Server
	message string
}

func startServer(apiKeyChan chan string, datarobotHost string) tea.Cmd {
	return func() tea.Msg {
		addr := "localhost:51164"

		mux := http.NewServeMux()
		server := &http.Server{
			Addr:    addr,
			Handler: mux,
		}

		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.URL.Query().Get("key")

			// Response to browser
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_ = assets.Write(w, "templates/success.html")

			apiKeyChan <- apiKey // send the key to the main goroutine
		})

		listen, err := net.Listen("tcp", addr)
		if err != nil {
			// close previous auth server if address already in use
			_, err = http.Get("http://" + addr)
			if err != nil {
				return errMsg{err}
			}

			listen, err = net.Listen("tcp", addr)
			if err != nil {
				return errMsg{err}
			}
		}

		// Start the server in a goroutine
		go func() {
			err := server.Serve(listen)
			if !errors.Is(err, http.ErrServerClosed) {
				log.Errorf("Server error: %v\n", err)
			}
		}()

		authURL := datarobotHost + "/account/developer-tools?cliRedirect=true"

		// Style the URL
		urlStyle := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#6124DF", Dark: "#9D7EDF"}).
			Underline(true).
			Bold(true)

		// Create styled frame for the auth URL - use dynamic width based on URL length
		// Add padding for borders and internal padding (2 + 2 + 4 for border chars)
		urlWidth := len(authURL) + 6
		urlFrameStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "#6124DF", Dark: "#9D7EDF"}).
			Padding(1, 2).
			Width(urlWidth)

		styledURL := urlStyle.Render(authURL)
		urlBox := urlFrameStyle.Render(styledURL)

		hint := tui.BaseTextStyle.
			Faint(true).
			Render("💡 If your browser didn't open automatically, click or copy the link above")

		message := lipgloss.JoinVertical(
			lipgloss.Left,
			"",
			urlBox,
			"",
			hint,
			"",
		)

		open.Open(authURL)

		return startedMsg{
			server:  server,
			message: message,
		}
	}
}

func (lm LoginModel) waitForAPIKey() tea.Cmd {
	return func() tea.Msg {
		// Wait for the key from the handler
		apiKey := <-lm.APIKeyChan

		// Now shut down the server after key is received
		if err := lm.server.Shutdown(context.Background()); err != nil {
			return errMsg{fmt.Errorf("Error during shutdown: %v", err)}
		}

		// empty apiKey means we need to interrupt current auth flow
		if apiKey == "" {
			return errMsg{errors.New("Interrupt request received.")}
		}

		viper.Set(config.DataRobotAPIKey, apiKey)

		err := auth.WriteConfigFileSilent()
		if err != nil {
			return errMsg{fmt.Errorf("Error during writing config file: %v", err)}
		}

		return lm.SuccessCmd()
	}
}

func (lm LoginModel) Init() tea.Cmd {
	datarobotHost := config.GetBaseURL()
	if datarobotHost == "" {
		return lm.GetHostCmd
	}

	return startServer(lm.APIKeyChan, datarobotHost)
}

func (lm LoginModel) Update(msg tea.Msg) (LoginModel, tea.Cmd) {
	switch msg := msg.(type) {
	case startedMsg:
		lm.loginMessage = msg.message
		lm.server = msg.server

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
		sb.WriteString(fmt.Sprintf("something went wrong: %s", lm.err))
		sb.WriteString("\n\n")
	}

	return sb.String()
}
