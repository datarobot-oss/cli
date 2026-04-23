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

package dotenv

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/envbuilder"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/tui"
)

type promptModel struct {
	prompt     envbuilder.UserPrompt
	input      textinput.Model
	list       list.Model
	Values     []string
	successCmd tea.Cmd
}

var (
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(tui.DrRed)
)

const (
	cursorStyle = '•'

	typeError            = "error"
	errMsgPrefix         = "😞 Unable to retrieve the list of available LLMs.\n\n"
	errMsgAuthFailed     = "🔐 Authentication failed. Please check your credentials and try again."
	errMsgNotFound       = "🔍 Requested resource not found. Please contact support for assistance."
	errMsgTimeout        = "⏳ Request timed out. Please check your network connection and try again."
	errMsgNoLLMs         = "🤷 No available LLMs found. Please contact support for assistance."
	errMsgContactSupport = "👥 Please try again or contact support if the issue persists."
)

type item envbuilder.PromptOption

func (i item) FilterValue() string {
	if i.Value != "" {
		return i.Value
	}

	return i.Name
}

type itemDelegate struct {
	multiple bool
}

func (d itemDelegate) Height() int                             { return 1 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}

	checkbox := ""

	if d.multiple {
		if i.Checked {
			checkbox = "[x] "
		} else {
			checkbox = "[ ] "
		}
	}

	str := fmt.Sprintf("%s%s", checkbox, i.Name)

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}

	fmt.Fprint(w, fn(str))
}

func newPromptModel(prompt envbuilder.UserPrompt, successCmd tea.Cmd) (promptModel, tea.Cmd) {
	if prompt.Type == "llmgw_catalog" {
		return newLLMListPrompt(prompt, successCmd)
	}

	if len(prompt.Options) == 0 {
		return newTextInputPrompt(prompt, successCmd)
	}

	return newListPrompt(prompt, successCmd)
}

func newTextInputPrompt(prompt envbuilder.UserPrompt, successCmd tea.Cmd) (promptModel, tea.Cmd) {
	ti := textinput.New()
	ti.SetValue(prompt.Value)

	// Mask the input if it's a secret
	if prompt.Type == envbuilder.PromptTypeSecret {
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = cursorStyle
	}

	cmd := ti.Focus()

	return promptModel{
		prompt:     prompt,
		input:      ti,
		successCmd: successCmd,
	}, cmd
}

func newListPrompt(prompt envbuilder.UserPrompt, successCmd tea.Cmd) (promptModel, tea.Cmd) {
	items := make([]list.Item, 0, len(prompt.Options)+1)

	if prompt.Optional {
		items = append(items, item{Blank: true, Name: "None (leave blank)"})
	}

	values := strings.Split(prompt.Value, ",")

	for _, option := range prompt.Options {
		if prompt.Multiple && slices.Index(values, option.Value) != -1 {
			option.Checked = true
		}

		items = append(items, item(option))
	}

	l := list.New(items, itemDelegate{prompt.Multiple}, 0, 15)

	if prompt.Value != "" && !prompt.Multiple {
		initialIndex := slices.IndexFunc(prompt.Options, func(po envbuilder.PromptOption) bool {
			return po.Value == prompt.Value
		})

		if prompt.Optional || initialIndex == -1 {
			initialIndex++
		}

		l.Select(initialIndex)
	}

	cmd := tea.WindowSize()

	return promptModel{
		prompt:     prompt,
		list:       l,
		successCmd: successCmd,
	}, cmd
}

func llmsToPromptOptions(llms []drapi.LLM) []envbuilder.PromptOption {
	options := make([]envbuilder.PromptOption, 0, len(llms))

	for _, llm := range llms {
		options = append(options, envbuilder.PromptOption{
			Blank:    false,
			Checked:  false,
			Name:     fmt.Sprintf("%s (%s)", llm.Name, llm.Provider),
			Value:    "datarobot/" + llm.Model,
			Requires: "",
		})
	}

	return options
}

func newLLMListPrompt(prompt envbuilder.UserPrompt, successCmd tea.Cmd) (promptModel, tea.Cmd) {
	llms, err := drapi.GetLLMs()
	if err != nil {
		log.Errorf("Error retrieving LLMs: %s", err.Error())

		var (
			netErr     net.Error
			httpErr    *drapi.HTTPError
			statusCode int
		)

		helpMsg := errMsgPrefix

		// Check if the error is a network timeout or an HTTP error to provide more specific feedback
		// Treat net.Error.Timeout() as an HTTP 408 Request Timeout for user-friendly messaging
		if errors.As(err, &netErr) && netErr.Timeout() {
			statusCode = http.StatusRequestTimeout
		} else if errors.As(err, &httpErr) {
			statusCode = httpErr.StatusCode
		}

		switch statusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			helpMsg += errMsgAuthFailed
		case http.StatusNotFound:
			helpMsg += errMsgNotFound
		case http.StatusRequestTimeout, http.StatusGatewayTimeout:
			helpMsg += errMsgTimeout
		default:
			helpMsg += err.Error() + "\n\n" + errMsgContactSupport
		}

		errPrompt := prompt
		errPrompt.Type = typeError

		errPrompt.Help = helpMsg

		return promptModel{prompt: errPrompt, successCmd: successCmd}, nil
	}

	if len(llms.LLMs) == 0 {
		log.Warn("No active LLMs found in the catalog")

		errPrompt := prompt
		errPrompt.Type = typeError
		errPrompt.Help = errMsgNoLLMs

		return promptModel{prompt: errPrompt, successCmd: successCmd}, nil
	}

	prompt.Options = append(prompt.Options, llmsToPromptOptions(llms.LLMs)...)

	return newListPrompt(prompt, successCmd)
}

func (pm promptModel) GetValues() []string {
	if len(pm.prompt.Options) == 0 {
		return []string{strings.TrimSpace(pm.input.Value())}
	}

	items := pm.list.Items()
	current := items[pm.list.GlobalIndex()].(item)

	if pm.prompt.Multiple {
		values := make([]string, 0, len(items))

		for i := range items {
			if itm := items[i].(item); itm.Checked {
				values = append(values, itm.FilterValue())
			}
		}

		return values
	}

	if current.Blank {
		return nil
	}

	return []string{current.FilterValue()}
}

func (pm promptModel) Update(msg tea.Msg) (promptModel, tea.Cmd) {
	if len(pm.prompt.Options) > 0 {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch keypress := msg.String(); keypress {
			case " ":
				// toggle checkbox, don't submit
				return pm.toggleCurrent()
			case "enter":
				// submit if valid
				return pm.submitList()
			}
		}

		var cmd tea.Cmd

		pm.list, cmd = pm.list.Update(msg)

		return pm, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "enter":
			return pm.submitInput()
		}
	}

	var cmd tea.Cmd

	pm.input, cmd = pm.input.Update(msg)

	return pm, cmd
}

func (pm promptModel) toggleCurrent() (promptModel, tea.Cmd) {
	items := pm.list.Items()
	currentItem := items[pm.list.Index()].(item)

	if !pm.prompt.Multiple {
		return pm, nil
	}

	if currentItem.Blank {
		for i := range items {
			itm := items[i].(item)
			itm.Checked = false
			items[i] = itm
		}
	} else {
		currentItem.Checked = !currentItem.Checked
		items[pm.list.Index()] = currentItem
	}

	cmd := pm.list.SetItems(items)

	return pm, cmd
}

func (pm promptModel) submitList() (promptModel, tea.Cmd) {
	pm.Values = pm.GetValues()

	if pm.prompt.Optional || len(pm.Values) > 0 {
		return pm, pm.successCmd
	}

	return pm, nil
}

func (pm promptModel) submitInput() (promptModel, tea.Cmd) {
	pm.Values = pm.GetValues()

	if pm.prompt.Optional || len(pm.Values[0]) > 0 {
		return pm, pm.successCmd
	}

	return pm, nil
}

func (pm promptModel) View() string {
	var sb strings.Builder

	sb.Write([]byte(tui.SubTitleStyle.Render(fmt.Sprintf("Variable: %v", pm.prompt.Env))))
	sb.WriteString("\n\n")

	if pm.prompt.Type.String() == typeError {
		sb.WriteString(tui.ErrorStyle.Render(pm.prompt.Help))
	} else {
		sb.WriteString(tui.BaseTextStyle.Render(pm.prompt.Help))
	}

	sb.WriteString("\n")

	if pm.prompt.Default != "" {
		sb.WriteString(tui.BaseTextStyle.Render(fmt.Sprintf("Default: %v", pm.prompt.Default)))
		sb.WriteString("\n\n")
	}

	if len(pm.prompt.Options) > 0 {
		sb.WriteString(pm.list.View())
		sb.WriteString("\n  ")

		if pm.prompt.Multiple {
			sb.WriteString(tui.DimStyle.Render("space to toggle • enter to answer • "))
		}
	} else if pm.prompt.Type.String() == typeError {
		sb.WriteString("\n\n")
	} else {
		sb.WriteString(pm.input.View())
		sb.WriteString("\n\n")
	}

	sb.WriteString(tui.DimStyle.Render("ctrl-p back to previous"))

	return sb.String()
}
