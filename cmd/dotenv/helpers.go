// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package dotenv

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/tui"
)

func generateRandomSecret(length int) (string, error) {
	bytes := make([]byte, length)

	_, err := rand.Read(bytes)
	if err != nil {
		return "", fmt.Errorf("Failed to generate random bytes: %w", err)
	}

	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}

// ensureInRepo checks if we're in a git repository, and returns the repo root path.
func ensureInRepo() (string, error) {
	repoRoot, err := repo.FindRepoRoot()
	if err != nil || repoRoot == "" {
		fmt.Println(tui.ErrorStyle.Render("Error:") + " Not inside a git repository")
		fmt.Println()
		fmt.Println("Run this command from within an application template git repository.")
		fmt.Println("To create a new template, run " + tui.BaseTextStyle.Render("`dr templates setup`") + ".")

		return "", errors.New("Not in git repository.")
	}

	return repoRoot, nil
}

// ensureInRepoWithDotenv checks if we're in a git repository and if .env file exists.
// It prints appropriate error messages and returns the dotenv file path if successful.
func ensureInRepoWithDotenv() (string, error) {
	repoRoot, err := ensureInRepo()
	if err != nil {
		return "", err
	}

	dotenv := filepath.Join(repoRoot, ".env")

	if _, err := os.Stat(dotenv); os.IsNotExist(err) {
		fmt.Printf("%s: .env file does not exist at %s\n", tui.ErrorStyle.Render("Error"), dotenv)
		fmt.Println()
		fmt.Println("Run " + tui.BaseTextStyle.Render("`dr dotenv setup`") + " to create one.")

		return "", errors.New(".env file does not exist.")
	}

	return dotenv, nil
}
