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

// Package enclave talks to the public Outposts API under /covalent/api/v2/outposts.
// The platform calls these resources "outposts" on the wire; the CLI and the
// RBAC layer surface them as "enclaves", so the command tree and this package
// use the enclave name while the request paths stay /outposts.
package enclave

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
)

// Enclave statuses as serialized by the server (OutpostStatus StrEnum). CREATED
// is a transient pre-registration state the public API never returns on its own.
const (
	StatusCreated    = "CREATED"
	StatusRegistered = "REGISTERED"
	StatusActive     = "ACTIVE"
	StatusDrain      = "DRAIN"
	StatusDown       = "DOWN"
	StatusMaint      = "MAINT"
)

// basePath is the collection route. The server route is "/outposts" (no
// trailing slash); requests must match it exactly to avoid a 307 redirect.
const basePath = "/covalent/api/v2/outposts"

// Enclave is the projection of an outpost record returned by GET /outposts and
// GET /outposts/{id}. Those responses are serialized snake_case (no alias
// generator on the server schema), hence the dispatch_url tag.
type Enclave struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DispatchURL string `json:"dispatch_url"`
	Status      string `json:"status"`
}

// InstallationSecret is one K8s Secret's literal contents the installer must
// materialize on the enclave cluster. Returned only by register and never
// persisted server-side, so the register command output is the sole place it
// ever appears.
type InstallationSecret struct {
	Data map[string]string `json:"data"`
}

// RegistrationResult is the POST /outposts response. Unlike the GET schema it
// is serialized camelCase (the server schema sets explicit aliases).
type RegistrationResult struct {
	EnclaveID           string                        `json:"outpostId"`
	Name                string                        `json:"name"`
	Status              string                        `json:"status"`
	InstallationSecrets map[string]InstallationSecret `json:"installationSecrets"`
}

type registerRequest struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
}

// escapeID percent-encodes a user-supplied enclave id so it always stays a
// single URL path segment.
func escapeID(id string) string {
	return url.PathEscape(id)
}

// RegisterEnclave registers a new enclave (outpost) and returns the created
// record together with its one-shot installation secrets. The server replies
// 201 with the full registration document inline. A name already registered
// for the tenant yields a 409.
func RegisterEnclave(name string, labels map[string]string) (*RegistrationResult, error) {
	url, err := config.GetEndpointURL(basePath)
	if err != nil {
		return nil, err
	}

	var result RegistrationResult

	if err := drapi.PostJSON(url, "enclave", registerRequest{Name: name, Labels: labels}, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetEnclave fetches a single enclave by id.
func GetEnclave(enclaveID string) (*Enclave, error) {
	url, err := config.GetEndpointURL(basePath + "/" + escapeID(enclaveID))
	if err != nil {
		return nil, err
	}

	var enclave Enclave

	if err := drapi.GetJSON(url, "enclave", &enclave); err != nil {
		return nil, err
	}

	return &enclave, nil
}

// ListEnclaves returns every enclave visible to the caller. The endpoint
// responds with a bare JSON array (not a paginated envelope).
func ListEnclaves() ([]Enclave, error) {
	url, err := config.GetEndpointURL(basePath)
	if err != nil {
		return nil, err
	}

	var enclaves []Enclave

	if err := drapi.GetJSON(url, "enclaves", &enclaves); err != nil {
		return nil, err
	}

	return enclaves, nil
}

// DeactivateEnclave marks an enclave inactive and returns the updated record.
// The endpoint takes no request body and replies 200 with the enclave.
func DeactivateEnclave(enclaveID string) (*Enclave, error) {
	url, err := config.GetEndpointURL(basePath + "/" + escapeID(enclaveID) + "/deactivate")
	if err != nil {
		return nil, err
	}

	var enclave Enclave

	// No body param server-side; send an empty object so the request carries
	// valid JSON matching the application/json content type.
	if err := drapi.PostJSON(url, "enclave", struct{}{}, &enclave); err != nil {
		return nil, err
	}

	return &enclave, nil
}

// DeleteEnclave deletes an enclave. The server replies 204 on success and 404
// when no enclave with the id exists.
func DeleteEnclave(enclaveID string) error {
	url, err := config.GetEndpointURL(basePath + "/" + escapeID(enclaveID))
	if err != nil {
		return err
	}

	return drapi.DeleteJSON(url, "enclave", nil, nil)
}

// ParseLabels turns repeated key=value flag values into a labels map, rejecting
// malformed entries and empty keys.
func ParseLabels(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}

	labels := make(map[string]string, len(values))

	for _, v := range values {
		key, value, found := strings.Cut(v, "=")
		if !found || key == "" {
			return nil, fmt.Errorf("invalid label %q: expected key=value", v)
		}

		labels[key] = value
	}

	return labels, nil
}
