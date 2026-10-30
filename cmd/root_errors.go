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

package cmd

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/spf13/cobra"
)

const (
	outputFormatFlag   = "--output-format"
	outputFormatEnvVar = "DATAROBOT_CLI_OUTPUT_FORMAT"
)

// executeRoot runs root with args and reports failures exactly once. JSON mode
// keeps its structured stderr envelope; text mode uses ReportError, including
// failures raised before a self-silencing child command can report them.
func executeRoot(ctx context.Context, root *cobra.Command, args []string) error {
	// Silence cobra in both formats, including errors before any hook runs.
	// JSON also suppresses usage text so stderr remains one JSON value.
	root.SilenceErrors = true

	jsonMode := jsonRequested(args)
	if jsonMode {
		root.SilenceUsage = true
	}

	root.SetArgs(args)

	err := root.ExecuteContext(ctx)
	if err == nil || errors.Is(err, cli.ErrSilent) {
		return err
	}

	// Config-only output formats become available during the pre-run hook.
	if jsonMode || outputformat.GetFormat(root) == outputformat.OutputFormatJSON {
		if printErr := outputformat.PrintJSONError(root.ErrOrStderr(), err); printErr != nil {
			log.Debug("Failed to write JSON error", "error", printErr)
		}
	} else {
		ReportError(root.ErrOrStderr(), err)
	}

	return err
}

// jsonRequested mirrors outputformat.GetFormat's precedence (flag, then env)
// on the raw arguments, since it runs before cobra has parsed them.
func jsonRequested(args []string) bool {
	format, found := lastOutputFormatArg(args)
	if !found {
		format = os.Getenv(outputFormatEnvVar)
	}

	return format == string(outputformat.OutputFormatJSON)
}

func lastOutputFormatArg(args []string) (string, bool) {
	var (
		format string
		found  bool
	)

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--" {
			break
		}

		if value, ok := strings.CutPrefix(arg, outputFormatFlag+"="); ok {
			format, found = value, true

			continue
		}

		if arg == outputFormatFlag && i+1 < len(args) {
			i++
			format, found = args[i], true
		}
	}

	return format, found
}
