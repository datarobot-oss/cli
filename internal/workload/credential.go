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

package workload

import (
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
)

// Credential is the projection of a stored DataRobot credential this CLI
// needs: just enough to confirm one exists (and, if ever useful later,
// report its type) before referencing it from an environmentVars entry.
type Credential struct {
	CredentialID   string `json:"credentialId"`
	Name           string `json:"name"`
	CredentialType string `json:"credentialType"`
}

// GetCredential fetches a single stored credential by id. The server
// replies 404 (surfaced as *drapi.HTTPError) when the id does not exist --
// callers use that to validate a dr-credential:<id>/<key> reference before
// writing it into an artifact spec, where a bad id would otherwise go
// unnoticed until the workload actually tries to run.
func GetCredential(credentialID string) (*Credential, error) {
	url, err := config.GetEndpointURL("/api/v2/credentials/" + escapeID(credentialID) + "/")
	if err != nil {
		return nil, err
	}

	var cred Credential

	if err := drapi.GetJSON(url, "credential", &cred); err != nil {
		return nil, err
	}

	return &cred, nil
}
