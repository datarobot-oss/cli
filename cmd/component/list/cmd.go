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

package list

import (
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/datarobot/cli/cmd/component/shared"
	"github.com/datarobot/cli/internal/appframework"
	"github.com/datarobot/cli/internal/tools"
	"github.com/spf13/cobra"
)

func PreRunE(_ *cobra.Command, _ []string) error {
	if err := tools.CheckPrerequisite("uv"); err != nil {
		return err
	}

	return nil
}

func RunE(_ *cobra.Command, _ []string) error {
	fw := shared.GetFrameworkPath()

	instances, err := appframework.ListInstalled(fw, ".")
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "Label\tModule\tAnswers\n")

	for _, inst := range instances {
		fmt.Fprintf(w, "%s\t%s\t%d\n", inst.Label, inst.Module, len(inst.Answers))
	}

	if err := w.Flush(); err != nil {
		return errors.New("flushing output")
	}

	return nil
}

func Cmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "📋 List installed components",
		PreRunE: PreRunE,
		RunE:    RunE,
	}
}
