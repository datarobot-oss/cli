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

// packages.go contains the shared helper for resolving a list of pip
// package specs from the --package flag. The flag is repeatable and also
// accepts comma-separated values so users can write either:
//
//   --package numpy --package pandas==2.0
//   --package numpy,pandas==2.0
//
// Empty entries are dropped and surrounding whitespace is trimmed.

package envutil

import (
	"errors"
	"strings"
)

// NormalizePackages takes the raw slice from a cobra StringSliceVar and
// returns a cleaned list. It returns an error when the resulting list is
// empty so callers can surface a friendly validation message.
func NormalizePackages(raw []string) ([]string, error) {
	out := make([]string, 0, len(raw))

	for _, entry := range raw {
		for _, item := range strings.Split(entry, ",") {
			trimmed := strings.TrimSpace(item)
			if trimmed != "" {
				out = append(out, trimmed)
			}
		}
	}

	if len(out) == 0 {
		return nil, errors.New("at least one package is required (use --package)")
	}

	return out, nil
}
