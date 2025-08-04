// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package clone

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/spf13/cobra"
)

func Run(args []string) error {
	if len(args) == 0 {
		return errors.New("template ID required")
	}

	templateId := args[0]

	template, err := drapi.GetTemplate(templateId)
	if err != nil {
		return err
	}

	repoUrl := template.Repository.URL
	fmt.Printf("ID: %s\nName: %s\nRepository URL: %s\n", template.ID, template.Name, repoUrl)

	dir := ""
	if len(args) > 1 {
		dir = args[1]
	} else {
		split := strings.Split(template.Repository.URL, "/")
		if len(split) > 0 {
			dir = split[len(split)-1]
		}
	}

	dirStyled := lipgloss.NewStyle().Bold(true).Render(dir)

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		return errors.New(fmt.Sprintf("Directory %s already exists", dirStyled))
	}

	fmt.Printf("\nCloning into %s directory...\n", dirStyled)

	err = gitClone(repoUrl, dir)
	if err != nil {
		return err
	}

	return nil
}

func gitClone(repoUrl, dir string) error {
	cmd := exec.Command("git", "clone", repoUrl, dir)
	stdout, err := cmd.Output()
	if err != nil {
		return err
	}

	fmt.Println(string(stdout))

	return nil
}

var Cmd = &cobra.Command{
	Use:   "clone",
	Short: "Clone application template",
	Long:  `Clone application template into user provided directory.`,
	Run: func(_ *cobra.Command, args []string) {
		err := Run(args)
		if err != nil {
			log.Fatal(err)
			return
		}
	},
}
