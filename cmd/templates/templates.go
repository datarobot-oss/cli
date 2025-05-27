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
	"fmt"
	"log"
	"net/http"
	"os/exec"

	"github.com/datarobot/cli/cmd/auth"

	"github.com/spf13/cobra"
)

var datarobotEndpoint = "https://staging.datarobot.com/api/v2"

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

	bearer := "Bearer " + key

	if err != nil {
		return nil
	}

	req, err := http.NewRequest(http.MethodGet, datarobotEndpoint+"/applicationTemplates?limit=100", nil)
	if err != nil {
		log.Fatal(err) // TODO: handler errors properly
	}

	req.Header.Add("Authorization", bearer)

	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err) // TODO: just return the error
	}

	defer resp.Body.Close()

	var templateList TemplateList

	err = json.NewDecoder(resp.Body).Decode(&templateList)
	if err != nil {
		log.Fatal(err) // TODO: just return the error
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
		_ = ListTemplates() // TODO: handle errors properly
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
			fmt.Println(err.Error())
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
