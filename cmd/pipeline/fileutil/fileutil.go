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

package fileutil

import "errors"

// ResolveFilePath returns the file path supplied either as a positional
// argument or via --from-file. Exactly one of the two must be provided.
func ResolveFilePath(args []string, fromFile string) (string, error) {
	positional := ""
	if len(args) > 0 {
		positional = args[0]
	}

	switch {
	case positional != "" && fromFile != "":
		return "", errors.New("specify the file either as a positional argument or via --from-file, not both")
	case positional != "":
		return positional, nil
	case fromFile != "":
		return fromFile, nil
	default:
		return "", errors.New("a file path is required (positional argument or --from-file)")
	}
}
