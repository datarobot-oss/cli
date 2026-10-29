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

package rewriteconfig

import (
	"errors"
	"fmt"
	"os"

	"github.com/datarobot/cli/internal/copier"
	"github.com/datarobot/cli/internal/repo"
	"github.com/spf13/cobra"
)

func PreRunE(_ *cobra.Command, _ []string) error {
	if !repo.IsInRepoRoot() {
		return errors.New("You must be in the repository root directory.")
	}

	return nil
}

func RunE(_ *cobra.Command, _ []string) error {
	baseURL := os.Getenv(copier.EnvApplicationTemplateGitBaseURL)
	if baseURL == "" {
		fmt.Printf("No %s environment variable set; nothing to rewrite.\n", copier.EnvApplicationTemplateGitBaseURL)

		return nil
	}

	answers, err := copier.AnswersFromPath(".", true)
	if err != nil {
		return fmt.Errorf("loading components: %w", err)
	}

	if len(answers) == 0 {
		fmt.Println("No component answers files found.")

		return nil
	}

	rewritten := 0

	for _, answer := range answers {
		if err := copier.RewriteSrcPathIfNeeded(answer.FileName); err != nil {
			return fmt.Errorf("rewriting %s: %w", answer.FileName, err)
		}

		rewritten++

		fmt.Printf("Rewritten: %s\n", answer.FileName)
	}

	fmt.Printf("Done. %d file(s) processed.\n", rewritten)

	return nil
}

func Cmd() *cobra.Command {
	return &cobra.Command{
		Use:           "rewrite-config",
		Short:         "✏️  Rewrite component answers files to use APPLICATION_TEMPLATE_GIT_BASE_URL",
		PreRunE:       PreRunE,
		RunE:          RunE,
		SilenceErrors: true,
	}
}
