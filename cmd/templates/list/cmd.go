// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package list

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/cmd/auth"
	"github.com/spf13/cobra"
)

type Template struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsGlobal    bool   `json:"isGlobal"`
	IsPremium   bool   `json:"isPremium"`
}

type TemplateList struct {
	Templates  []Template `json:"data"`
	Count      int        `json:"count"`
	TotalCount int        `json:"totalCount"`
	Next       string     `json:"next"`
	Previous   string     `json:"previous"`
}

func getTemplates() (*TemplateList, error) {
	key, err := auth.GetAPIKey()
	if err != nil {
		return nil, err
	}

	bearer := "Bearer " + key

	// datarobotHost := "https://staging.datarobot.com/api/v2"
	// datarobotHost := "https://app.datarobot.com/api/v2"
	datarobotHost, err := auth.GetURL(false)
	if err != nil {
		return nil, err
	}

	datarobotEndpoint := datarobotHost + "/api/v2/applicationTemplates/?limit=100"
	log.Info("Fetching templates from " + datarobotEndpoint)

	req, err := http.NewRequest(http.MethodGet, datarobotEndpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", bearer)

	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Response status code is " + resp.Status)
	}

	var templateList TemplateList

	err = json.NewDecoder(resp.Body).Decode(&templateList)
	if err != nil {
		return nil, err
	}

	return &templateList, nil
}

func Run() error {
	templateList, err := getTemplates()
	if err != nil {
		return err
	}

	for _, template := range templateList.Templates {
		fmt.Printf("ID: %s\tName: %s\n", template.ID, template.Name)
	}

	return nil
}

var Cmd = &cobra.Command{
	Use:   "list",
	Short: "List all available templates",
	Long:  `List all available templates in the DataRobot application.`,
	Run: func(_ *cobra.Command, _ []string) {
		err := Run()
		if err != nil {
			log.Fatal(err)
			return
		}
	},
}

func RunTea() error {
	templateList, _ := getTemplates()
	m := NewModel(templateList.Templates)
	p := tea.NewProgram(m, tea.WithAltScreen())

	_, err := p.Run()
	return err
}

var TeaCmd = &cobra.Command{
	Use:   "list_tea",
	Short: "List all available templates",
	Long:  `List all available templates in the DataRobot application.`,
	Run: func(_ *cobra.Command, _ []string) {
		err := RunTea()
		if err != nil {
			log.Fatal(err)
			return
		}
	},
}
