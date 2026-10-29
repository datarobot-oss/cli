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

// Package jobargs holds what the job commands share: reading a request body
// from a spec file and flags, and asking before an irreversible delete.
package jobargs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/datarobot/cli/cmd/helpers"
	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Field is one request-body field a flag can set.
type Field struct {
	Flag string // the command-line flag
	Key  string // the camelCase API key it sets
}

// Body reads the JSON or YAML spec file at path, when one is given, then lays
// the set flags over it. A flag may fill a field the spec leaves out, but not
// contradict one it sets: which of the two was meant cannot be told.
func Body(cmd *cobra.Command, path string, fields []Field) (map[string]any, error) {
	body := map[string]any{}

	if path != "" {
		raw, err := workload.ReadSpecFile(path)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(raw, &body); err != nil || body == nil {
			return nil, fmt.Errorf("spec file %s must hold a JSON object", path)
		}
	}

	for _, field := range fields {
		if !cmd.Flags().Changed(field.Flag) {
			continue
		}

		value, _ := cmd.Flags().GetString(field.Flag)

		if existing, ok := body[field.Key]; ok && existing != value {
			return nil, fmt.Errorf("--%s %q contradicts %s %v in the spec file; set it in one place", field.Flag, value, field.Key, existing)
		}

		body[field.Key] = value
	}

	return body, nil
}

// RuntimeFromFile reads a runtime block from a JSON or YAML file.
func RuntimeFromFile(path string) (map[string]any, error) {
	raw, err := workload.ReadSpecFile(path)
	if err != nil {
		return nil, err
	}

	var runtime map[string]any

	if err := json.Unmarshal(raw, &runtime); err != nil || runtime == nil {
		return nil, fmt.Errorf("runtime file %s must hold a JSON object", path)
	}

	return runtime, nil
}

// AddYesFlag registers --yes for a command that asks before acting.
func AddYesFlag(cmd *cobra.Command, usage string) {
	cmd.Flags().BoolP(cli.YesFlagName, "y", false, usage)
}

// Confirm asks before an irreversible action on a job named on the command
// line. --yes or a declared non-interactive run answers it; otherwise it asks
// on a terminal and refuses where it cannot. A declined prompt is (false, nil).
func Confirm(cmd *cobra.Command, question string) (bool, error) {
	yes, _ := cmd.Flags().GetBool(cli.YesFlagName)
	if yes || cli.IsNonInteractive(cmd) {
		return true, nil
	}

	remedy := "pass --yes (or set " + reader.NonInteractiveEnv + "=1) to run without a prompt"

	// The question would land in the JSON document something is parsing.
	if outputformat.GetFormat(cmd) == outputformat.OutputFormatJSON {
		return false, errors.New("--output-format json cannot ask for confirmation: " + remedy)
	}

	if !canAsk(cmd) {
		return false, errors.New("confirmation required: " + remedy)
	}

	confirmed, err := helpers.Confirm(cmd.ErrOrStderr(), cmd.InOrStdin(), question)
	if err != nil || confirmed {
		return confirmed, err
	}

	fmt.Fprintln(cmd.ErrOrStderr(), tui.DimStyle.Render("Aborted."))

	return false, nil
}

func canAsk(cmd *cobra.Command) bool {
	if !reader.IsStdinTerminal() {
		return false
	}

	f, ok := cmd.ErrOrStderr().(*os.File)
	if !ok {
		// A test buffer: the question can be captured, so it can be asked.
		return true
	}

	return term.IsTerminal(int(f.Fd()))
}
