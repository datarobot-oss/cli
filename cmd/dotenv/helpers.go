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
		fmt.Println(tui.ErrorStyle.Render("Oops! ") + "This command needs to run inside your AI application folder.")
		fmt.Println()
		fmt.Println("📁 What this means:")
		fmt.Println("   You need to be in a folder that contains your AI application code.")
		fmt.Println()
		fmt.Println("🔧 How to fix this:")
		fmt.Println("   1. If you haven't created an app yet: run " + tui.InfoStyle.Render("dr templates setup"))
		fmt.Println("   2. If you have an app: navigate to its folder using " + tui.InfoStyle.Render("cd your-app-name"))
		fmt.Println("   3. Then try this command again")

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
		fmt.Printf("%s: Your app is missing its configuration file (.env)\n", tui.ErrorStyle.Render("Missing Config"))
		fmt.Println()
		fmt.Println("📄 What this means:")
		fmt.Println("   Your AI application needs a '.env' file to store settings like API keys.")
		fmt.Println()
		fmt.Println("🔧 How to fix this:")
		fmt.Println("   Run " + tui.InfoStyle.Render("dr dotenv setup") + " to create the configuration file.")
		fmt.Println("   This will guide you through setting up all required settings.")

		return "", errors.New("'.env' file does not exist.")
	}

	return dotenv, nil
}
