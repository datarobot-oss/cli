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

// payload.go contains the shared helper for resolving an input payload
// from either a positional argument or the --from-file flag, then parsing
// it as JSON.

package inpututil

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// ResolvePayload mirrors the create/update flag pattern from
// `dr pipelines create`: a JSON file path can be supplied either as a
// positional argument or via --from-file=<path>; exactly one of the two
// must be provided. The contents of the file must be a JSON object so it
// fits the `{payload: object}` body the API expects.
func ResolvePayload(args []string, fromFile string) (map[string]any, error) {
	path, err := resolveFilePath(args, fromFile)
	if err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var payload map[string]any

	err = json.Unmarshal(raw, &payload)
	if err != nil {
		return nil, fmt.Errorf("parse %s as JSON object: %w", path, err)
	}

	return payload, nil
}

// resolveFilePath returns the file path supplied either positionally or
// via --from-file. Exactly one of the two must be provided.
func resolveFilePath(args []string, fromFile string) (string, error) {
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
		return "", errors.New("a JSON payload file is required (positional argument or --from-file)")
	}
}
