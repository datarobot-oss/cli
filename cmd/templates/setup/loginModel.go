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
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/spf13/viper"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/cmd/auth"
)

func waitForAPIKey(apiKeyChan chan string, server *http.Server, successCmd func(string) tea.Cmd) tea.Cmd {
	return func() tea.Msg {
		// Wait for the key from the handler
		apiKey := <-apiKeyChan

		// fmt.Println("Successfully consumed API Key from API Request")
		// Now shut down the server after key is received
		if err := server.Shutdown(context.Background()); err != nil {
			return errMsg{fmt.Errorf("error during shutdown: %v", err)}
		}

		viper.Set(auth.DataRobotAPIKey, apiKey)
		auth.WriteConfigFileSilent()

		return successCmd(apiKey)
	}
}

type LoginModel struct {
	loginMessage string
	apiKeyChan   chan string
	apiKey       string
	err          error
	successCmd   func(string) tea.Cmd
}

// type responseMsg string
type startedMsg struct {
	server  *http.Server
	message string
}

type errMsg struct{ error }

func (e errMsg) Error() string { return e.error.Error() }

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

			fmt.Fprint(w, "Successfully processed API key, you may close this window.")

			apiKeyChan <- apiKey // send the key to the main goroutine
		})

		listen, err := net.Listen("tcp", addr)
		if err != nil {
			return errMsg{err}
		}

		// Start the server in a goroutine
		go func() {
			err := server.Serve(listen)
			if err != http.ErrServerClosed {
				// log.Errorf("Server error: %v\n", err)
				log.Printf("Server error: %v\n", err)
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

func (lm LoginModel) Init() tea.Cmd {
	datarobotHost, err := auth.GetURL(false)
	if err != nil {
		return func() tea.Msg {
			return errMsg{err}
		}
	}

	return startServer(lm.apiKeyChan, datarobotHost)
}

func (lm LoginModel) Update(msg tea.Msg) (LoginModel, tea.Cmd) {
	switch msg := msg.(type) {
	case startedMsg:
		lm.loginMessage = msg.message
		return lm, waitForAPIKey(lm.apiKeyChan, msg.server, lm.successCmd)

	case errMsg:
		lm.err = msg
		return lm, nil

	default:
		return lm, nil
	}
}

func (lm LoginModel) View() string {
	var sb strings.Builder

	if lm.apiKey != "" {
		sb.WriteString(fmt.Sprintf("api key: %s", lm.apiKey))
		sb.WriteString("\n\n")
	} else if lm.loginMessage != "" {
		sb.WriteString(lm.loginMessage)
	} else if lm.err != nil {
		sb.WriteString(fmt.Sprintf("something went wrong: %s", lm.err))
		sb.WriteString("\n\n")
	} else {
		sb.WriteString("else\n\n")
	}

	return sb.String()
}
