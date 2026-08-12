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

// Credential is the slice of a stored DataRobot credential this CLI needs:
// enough to confirm one exists and to name it back to the user. The secret
// itself is never returned by the API and never wanted here.
type Credential struct {
	CredentialID   string `json:"credentialId"`
	Name           string `json:"name"`
	CredentialType string `json:"credentialType"`
}

// GetCredential fetches a stored credential by id. A missing id comes back as
// a 404 wrapped in *drapi.HTTPError.
//
// This is what verifies a dr-credential:<id>/<key> reference before it is
// written into an artifact spec. Without the check a mistyped id survives
// every local validation and only surfaces when the container fails to
// start, long after the build has been paid for.
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
