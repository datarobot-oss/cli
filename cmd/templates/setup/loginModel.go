// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package setup

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/cmd/auth"
	"github.com/datarobot/cli/internal/assets"
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
			return errMsg{err}
		}

		// Start the server in a goroutine
		go func() {
			err := server.Serve(listen)
			if !errors.Is(err, http.ErrServerClosed) {
				log.Errorf("Server error: %v\n", err)
			}
		}()

		var msg strings.Builder

		msg.WriteString("\n\nPlease visit this link to connect your DataRobot credentials to the CLI\n")
		msg.WriteString("(If you're prompted to log in, you may need to re-enter this URL):\n")
		msg.WriteString(datarobotHost)
		msg.WriteString("/account/developer-tools?cliRedirect=true\n\n")

		return startedMsg{
			server:  server,
			message: msg.String(),
		}
	}
}

func (lm LoginModel) waitForAPIKey() tea.Cmd {
	return func() tea.Msg {
		// Wait for the key from the handler
		apiKey := <-lm.APIKeyChan
		viper.Set(auth.DataRobotAPIKey, apiKey)
		auth.WriteConfigFileSilent()

		// Now shut down the server after key is received
		if err := lm.server.Shutdown(context.Background()); err != nil {
			return errMsg{fmt.Errorf("error during shutdown: %v", err)}
		}

		return lm.SuccessCmd()
	}
}

func (lm LoginModel) Init() tea.Cmd {
	datarobotHost, _ := auth.GetBaseURL()
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
