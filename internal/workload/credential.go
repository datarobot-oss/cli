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

// CredentialTypeAPIToken stores a single opaque token under the apiToken
// field, which is the shape every secret imported from a .env takes: one name,
// one value, and nothing about what the value is for.
const CredentialTypeAPIToken = "api_token"

// CreateCredential stores a secret and returns the credential holding it, so a
// manifest can reference it by id instead of carrying the value.
//
// This is the one call in the CLI that sends a secret. The value goes from
// wherever the caller read it straight to the platform and is never written to
// disk, which is the whole point: the reference that ends up in the committed
// manifest names a credential, and the credential is the only place the secret
// lives.
//
// A name already in use comes back as a 409 wrapped in *drapi.HTTPError.
// Callers decide what that means, because reusing a credential of the same
// name would silently deploy whatever value it already holds, which may not be
// the one the user just supplied.
func CreateCredential(name, value string) (*Credential, error) {
	url, err := config.GetEndpointURL("/api/v2/credentials/")
	if err != nil {
		return nil, err
	}

	body := map[string]string{
		"name":           name,
		"credentialType": CredentialTypeAPIToken,
		"apiToken":       value,
	}

	var cred Credential

	if err := drapi.PostJSON(url, "credential", body, &cred); err != nil {
		return nil, err
	}

	return &cred, nil
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
