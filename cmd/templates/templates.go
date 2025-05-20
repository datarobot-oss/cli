package templates

import (
	"encoding/json"
	"fmt"
	"github.com/datarobot/cli/cmd/auth"
	"log"
	"net/http"
	"os/exec"

	"github.com/spf13/cobra"
)

var datarobotEndpoint string = "https://staging.datarobot.com/api/v2"

type Template struct {
	Id          string `json:"id"`
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

	req, err := http.NewRequest("GET", datarobotEndpoint+"/applicationTemplates?limit=100", nil)
	req.Header.Add("Authorization", bearer)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()

	var templateList TemplateList

	json.NewDecoder(resp.Body).Decode(&templateList)

	for _, template := range templateList.Templates {
		fmt.Printf("ID: %s\tName: %s\n", template.Id, template.Name)
	}

	return nil
}

var listTemplatesCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available templates",
	Long:  `List all available templates in the DataRobot application.`,
	Run: func(cmd *cobra.Command, args []string) {
		ListTemplates()
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the status of the DataRobot application",
	Long:  `Check the status of the DataRobot application.`,
	Run: func(cmd *cobra.Command, args []string) {
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
	)
}
