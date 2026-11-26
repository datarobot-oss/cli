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

// Package filearg resolves a recipe file given either positionally or with
// --from-file, the two equivalent forms `dr pipeline create` also accepts.
package filearg

import (
	"errors"

	"github.com/spf13/cobra"
)

// FlagName is the alternative to the positional file argument.
const FlagName = "from-file"

// Add registers --from-file on cmd.
func Add(cmd *cobra.Command, dest *string) {
	cmd.Flags().StringVar(dest, FlagName, "", "Recipe file (JSON or YAML); alternative to the positional argument")
}

// Resolve returns the file named positionally (positional) or by --from-file
// (flag), refusing both at once. required says whether one must be given.
func Resolve(positional []string, flag string, required bool) (string, error) {
	if len(positional) > 0 && flag != "" {
		return "", errors.New("give the recipe file either as an argument or with --from-file, not both")
	}

	if len(positional) > 0 {
		return positional[0], nil
	}

	if flag == "" && required {
		return "", errors.New("a recipe file is required: pass it as an argument or with --from-file")
	}

	return flag, nil
}
