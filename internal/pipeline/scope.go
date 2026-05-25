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

// scope.go contains the shared draft/locked scope resolution used by the
// input and run CLI commands. The pipelines API exposes two URL shapes
// for these resources:
//
//	draft  -> /pipelines/{id}/<sub>
//	locked -> /pipelines/{id}/versions/{ver}/<sub>
//
// The CLI surfaces the choice with optional --scope and --version flags;
// ResolveScope encapsulates the precedence rules so every verb behaves
// consistently. PipelinePath then turns the resolved scope into the right
// URL fragment.

package pipeline

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/datarobot/cli/internal/config"
)

// Scope identifies whether a request targets the mutable draft of a pipeline
// or a locked, frozen version.
type Scope string

const (
	ScopeDraft  Scope = "draft"
	ScopeLocked Scope = "locked"
)

// ResolveScope applies the CLI precedence rules:
//
//   - empty scope, no version  -> draft
//   - empty scope, version=N   -> locked (auto-promoted from --version)
//   - scope=draft  + version   -> error (draft has no versions)
//   - scope=locked + no version -> error (locked requires --version)
//   - scope=draft  + no version -> draft
//   - scope=locked + version=N -> locked
//
// `version` is a pointer because 0 is a real value sent by the user; nil
// means the flag was not provided.
func ResolveScope(scope string, version *int) (Scope, *int, error) {
	normalized := strings.ToLower(strings.TrimSpace(scope))

	switch normalized {
	case "":
		if version == nil {
			return ScopeDraft, nil, nil
		}

		return ScopeLocked, version, nil
	case string(ScopeDraft):
		if version != nil {
			return "", nil, errors.New("--scope=draft cannot be combined with --version")
		}

		return ScopeDraft, nil, nil
	case string(ScopeLocked):
		if version == nil {
			return "", nil, errors.New("--scope=locked requires --version=<n>")
		}

		return ScopeLocked, version, nil
	default:
		return "", nil, fmt.Errorf("invalid --scope: %q (supported: draft, locked)", scope)
	}
}

// PipelinePath builds an API path for a sub-resource hanging off a single
// pipeline. The leading "/api/v2" prefix is added by config.GetEndpointURL,
// so this returns just the segment beginning at "/pipelines".
//
// suffix should NOT start with a slash (e.g. "inputs", "inputs/abc").
func PipelinePath(pipelineID string, scope Scope, version *int, suffix string) (string, error) {
	if pipelineID == "" {
		return "", errors.New("pipeline id is required")
	}

	suffix = strings.TrimPrefix(suffix, "/")

	base := "/api/v2/pipelines/" + pipelineID

	if scope == ScopeLocked {
		if version == nil {
			return "", errors.New("locked scope requires a version")
		}

		base += "/versions/" + strconv.Itoa(*version)
	}

	if suffix == "" {
		return base, nil
	}

	return base + "/" + suffix, nil
}

// EndpointFor combines PipelinePath with config.GetEndpointURL so callers
// can write a single line. It returns a fully-qualified URL ready to be
// passed to drapi.GetJSON / doJSON / doDelete.
func EndpointFor(pipelineID string, scope Scope, version *int, suffix string) (string, error) {
	path, err := PipelinePath(pipelineID, scope, version, suffix)
	if err != nil {
		return "", err
	}

	return config.GetEndpointURL(path)
}
