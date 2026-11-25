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

package outputformat

import (
	"strings"

	"github.com/spf13/cobra"
)

const flagName = "output-format"

// FromArgs returns the value of the last `--output-format` in args, in either
// the `--output-format json` or `--output-format=json` form. found is false
// when the flag is absent. The value is returned as written and is not
// validated: a caller that only asks "is it json?" has no use for an error.
// Scanning stops at a bare `--`, after which everything is a positional
// argument rather than a flag.
//
// It exists for commands that set DisableFlagParsing and hand their args to
// another program (plugins), so cobra never parses `--output-format` for them.
// Only the long flag is recognised: the deprecated `--format` and `-o` aliases
// exist on `dr self version` alone, and on such a command line `-o` belongs to
// the program being run and may mean something else entirely.
//
// This could possibly be refactored to parse args with a pflag.FlagSet that
// tolerates unknown flags (ParseErrorsAllowlist.UnknownFlags) and to reuse
// OutputFormat.Set for validation. The explicit loop is kept for now; before
// switching, check how pflag treats an unknown flag that takes a value.
func FromArgs(args []string) (format OutputFormat, found bool) {
	const long = "--" + flagName

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == "--":
			return format, found
		case arg == long && i+1 < len(args):
			format, found = OutputFormat(args[i+1]), true
			i++
		case strings.HasPrefix(arg, long+"="):
			format, found = OutputFormat(strings.TrimPrefix(arg, long+"=")), true
		}
	}

	return format, found
}

// GetFormatFromArgs is GetFormat for a command whose flags cobra does not
// parse. An explicit `--output-format` in args wins, as it does for every other
// command; otherwise the format is whatever GetFormat resolves, which covers a
// root flag placed before the command name (`dr --output-format json <plugin>`)
// and DATAROBOT_CLI_OUTPUT_FORMAT.
func GetFormatFromArgs(cmd *cobra.Command, args []string) OutputFormat {
	if format, found := FromArgs(args); found {
		return format
	}

	return GetFormat(cmd)
}
