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

// Package buildargs centralizes the positional argument shape shared by
// `dr artifact build get` and `dr artifact build logs`. Both accept either
// one positional (build-id, with the artifact-id read from .wapi/config.json)
// or two positionals (artifact-id, build-id). Keeping the dispatch in one
// place prevents the two leaves from drifting.

package buildargs

import (
	"fmt"

	"github.com/datarobot/cli/internal/workload"
)

// ResolveOptional maps cobra args to a single artifactID for commands that
// accept an optional artifact-id (0 or 1 positional).
//
//   - 0 args -> artifact-id is read from .wapi.
//   - 1 arg  -> args[0]=artifact-id (overrides .wapi).
//
// Returns an error if .wapi is required but missing.
func ResolveOptional(args []string) (string, error) {
	explicit := ""
	if len(args) == 1 {
		explicit = args[0]
	}

	id, _, err := workload.ResolveArtifactID(explicit)

	return id, err
}

// ResolvePositional maps cobra args to (artifactID, buildID).
//
//   - 1 arg  -> arg is the build-id; artifact-id is read from .wapi.
//   - 2 args -> args[0]=artifact-id, args[1]=build-id (overrides .wapi).
//
// Returns an error if the wrong arity is given or .wapi is required but
// missing.
func ResolvePositional(args []string) (string, string, error) {
	switch len(args) {
	case 1:
		artifactID, _, err := workload.ResolveArtifactID("")
		if err != nil {
			return "", "", err
		}

		return artifactID, args[0], nil
	case 2:
		artifactID, _, err := workload.ResolveArtifactID(args[0])
		if err != nil {
			return "", "", err
		}

		return artifactID, args[1], nil
	}

	return "", "", fmt.Errorf("expected 1 or 2 positional arguments, got %d", len(args))
}
