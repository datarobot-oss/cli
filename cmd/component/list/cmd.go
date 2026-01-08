// Copyright 2025 DataRobot, Inc. and its affiliates.
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

package list

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/datarobot/cli/internal/copier"
	"github.com/spf13/cobra"
)

func RunE(_ *cobra.Command, _ []string) error {
	answers, err := copier.AnswersFromPath(".", false)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "Component name\tAnswers file\tRepository\n")

	for _, answer := range answers {
		fmt.Fprintf(w, "%s\t%s\t%s\n", answer.ComponentDetails.Name, answer.FileName, answer.Repo)
	}

	w.Flush()

	return nil
}

func Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed components.",
		RunE:  RunE,
	}
}
