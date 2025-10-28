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
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/cmd/auth"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Run(args []string) error {
	templateID, dir, err := validateArgs(args)
	if err != nil {
		return err
	}

	template, err := drapi.GetTemplate(templateID)
	if err != nil {
		return err
	}

	repoURL := template.Repository.URL
	fmt.Printf("ID: %s\nName: %s\nRepository URL: %s\n", template.ID, template.Name, repoURL)

	dirStyled := lipgloss.NewStyle().Bold(true).Render(dir)

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		return fmt.Errorf("directory %s already exists", dirStyled)
	}

	fmt.Printf("\nCloning into %s directory...\n", dirStyled)

	updatedDir := cleanDirPath(dir)

	out, err := gitClone(repoURL, updatedDir)
	if err != nil {
		return err
	}

	fmt.Println(out)

	fmt.Println(tui.SubTitleStyle.Render(fmt.Sprintf("🎉 Template %s cloned.", template.Name)))
	fmt.Println(tui.BaseTextStyle.Render("To navigate to the project directory, use the following command:"))
	fmt.Println()
	fmt.Println(tui.BaseTextStyle.Render("cd " + updatedDir))

	return nil
}

func validateArgs(args []string) (string, string, error) {
	if len(args) == 0 {
		return "", "", errors.New("template ID required")
	}

	templateID := args[0]

	template, err := drapi.GetTemplate(templateID)
	if err != nil {
		return "", "", err
	}

	dir := ""
	if len(args) > 1 {
		dir = args[1]
	} else {
		dir = template.DefaultDir()
	}

	return templateID, dir, nil
}

func gitClone(repoURL, dir string) (string, error) {
	cmd := exec.Command("git", "clone", repoURL, dir)

	stdout, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	return string(stdout), nil
}

func gitOrigin(dir string, isAbsolute bool) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")

	path, err := os.Getwd()
	if err != nil {
		return ""
	}

	if isAbsolute {
		cmd.Dir = dir
	} else {
		cmd.Dir = filepath.Join(path, dir)
	}

	stdout, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(stdout))
}

func gitPull(dir string) (string, error) {
	cmd := exec.Command("git", "pull")

	path, err := os.Getwd()
	if err != nil {
		return "", err
	}

	cmd.Dir = filepath.Join(path, dir)

	stdout, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	return string(stdout), nil
}

var Cmd = &cobra.Command{
	Use:   "clone",
	Short: "Clone application template",
	Long:  `Clone application template into user provided directory.`,
	PreRunE: func(_ *cobra.Command, _ []string) error {
		return auth.EnsureAuthenticatedE()
	},
	Run: func(_ *cobra.Command, args []string) {
		err := Run(args)
		if err != nil {
			log.Fatal(err)
			return
		}
	},
}
