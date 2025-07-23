// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package templates

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"

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

func ListTemplates() error {
	key, err := auth.GetAPIKey()
	if err != nil {
		return err
	}

	bearer := "Bearer " + key

	// datarobotHost := "https://staging.datarobot.com/api/v2"
	// datarobotHost := "https://app.datarobot.com/api/v2"
	datarobotHost, err := auth.GetURL(false)
	if err != nil {
		return err
	}

	datarobotEndpoint := datarobotHost + "/api/v2/applicationTemplates/?limit=100"
	log.Info("Fetching templates from " + datarobotEndpoint)

	req, err := http.NewRequest(http.MethodGet, datarobotEndpoint, nil)
	if err != nil {
		return err
	}

	req.Header.Add("Authorization", bearer)

	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("Response status code is " + resp.Status)
	}

	var templateList TemplateList

	err = json.NewDecoder(resp.Body).Decode(&templateList)
	if err != nil {
		return err
	}

	for _, template := range templateList.Templates {
		fmt.Printf("ID: %s\tName: %s\n", template.ID, template.Name)
	}

	return nil
}

var listTemplatesCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available templates",
	Long:  `List all available templates in the DataRobot application.`,
	Run: func(_ *cobra.Command, _ []string) {
		err := ListTemplates()
		if err != nil {
			log.Fatal(err)
			return
		}
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the status of the DataRobot application",
	Long:  `Check the status of the DataRobot application.`,
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println("Checking the status of the DataRobot application...")
		gitcmd := exec.Command("git", "status")
		stdout, err := gitcmd.Output()
		if err != nil {
			log.Fatal(err.Error())
			return
		}

		// Print the output
		fmt.Println(string(stdout))
	},
}

func init() {
	TemplatesCmd.AddCommand(
		listTemplatesCmd,
		statusCmd,
		setupCmd,
	)
}
