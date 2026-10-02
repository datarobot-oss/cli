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
	"net/url"
	"strconv"
	"strings"

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

// CredentialList is one page of the credentials route.
type CredentialList struct {
	Data []Credential `json:"data"`
	Next string       `json:"next"`
}

// scanCredentials walks the credentials route page by page and returns every
// credential keep accepts. keep answers (take, stop): take collects the
// credential, stop ends the scan because the caller already has what it needs
// (a name lookup stops at its first match; a prefix collection never does).
//
// limit bounds the whole scan, not one page: a lookup that follows next links
// until a large tenant runs out would turn a courtesy into a long walk. A
// limit of 0 or less means walk to the end, which teardown wants so it does
// not leave a credential behind for being one page too far down. Every
// paginator in this package shares the same two safety checks it carries here:
// it refuses a next link that points at another host (drapi attaches the
// user's token to whatever URL it is given) and breaks on an empty page so a
// paginator that never advances cannot loop for ever.
func scanCredentials(limit int, keep func(Credential) (take, stop bool)) ([]Credential, error) {
	query := url.Values{}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}

	pageURL, err := drapi.EndpointURL("/credentials/", query)
	if err != nil {
		return nil, err
	}

	var found []Credential

	for scanned := 0; pageURL != "" && (limit <= 0 || scanned < limit); {
		var list CredentialList

		if err := drapi.GetJSON(pageURL, "credentials", &list); err != nil {
			return nil, err
		}

		if stop := collectMatches(list.Data, keep, &found); stop {
			return found, nil
		}

		scanned += len(list.Data)

		pageURL, err = nextCredentialPage(list)
		if err != nil {
			return nil, err
		}
	}

	return found, nil
}

// collectMatches appends the credentials keep accepts to found and reports
// whether keep asked to stop, which is how the first-match lookup ends its
// scan the moment it has an answer.
func collectMatches(page []Credential, keep func(Credential) (take, stop bool), found *[]Credential) bool {
	for i := range page {
		take, stop := keep(page[i])
		if take {
			*found = append(*found, page[i])
		}

		if stop {
			return true
		}
	}

	return false
}

// nextCredentialPage returns the URL of the page after list, or "" when the
// scan should stop: an empty next link, or a page that came back empty (which
// would otherwise let a paginator that never advances loop for ever). It
// refuses a next link on another host, because drapi attaches the user's token
// to whatever URL it is given.
func nextCredentialPage(list CredentialList) (string, error) {
	if list.Next == "" || len(list.Data) == 0 {
		return "", nil
	}

	if err := drapi.AssertNextOnSameHost(list.Next); err != nil {
		return "", err
	}

	return list.Next, nil
}

// FindCredentialNamed returns the credential called name, or nil when the
// organisation has none. Names are unique tenant-wide, so this is how a
// caller turns a name it expected to create into the id that already holds it.
//
// limit bounds the whole scan, not the page: this is a courtesy lookup that
// improves an error message, and following next links until a large tenant
// runs out would turn every refusal into a long walk. Reaching the bound
// answers nil, the same as a name that is genuinely not there, because to the
// caller the two mean the same thing: no id to offer.
func FindCredentialNamed(name string, limit int) (*Credential, error) {
	found, err := scanCredentials(limit, func(c Credential) (bool, bool) {
		match := c.Name == name

		// Stop on the first match: the name is unique, so there is no second.
		return match, match
	})
	if err != nil {
		return nil, err
	}

	if len(found) == 0 {
		return nil, nil
	}

	return &found[0], nil
}

// CredentialsWithPrefix returns every credential whose name starts with prefix,
// which is how teardown finds the "<workloadName>/<envName>" credentials the
// CLI minted for a workload. A limit of 0 or less walks the whole tenant,
// because a cleanup that stopped early would leave exactly the orphans it
// exists to remove.
func CredentialsWithPrefix(prefix string, limit int) ([]Credential, error) {
	return scanCredentials(limit, func(c Credential) (bool, bool) {
		return strings.HasPrefix(c.Name, prefix), false
	})
}

// CreateCredential stores a secret and returns the credential holding it, so a
// manifest can reference it by id instead of carrying the value.
//
// This is the one call in the CLI that sends a secret. The value goes from
// wherever the caller read it straight to the platform and never reaches the
// manifest, which is the whole point: the reference that ends up committed
// names a credential, and the credential is the only place the secret lives.
//
// Not reaching the manifest is this function's own guarantee. Not reaching
// disk at all takes one more thing, because the debug log dumps outgoing
// request bodies and this body is a secret by definition; that is what the
// redaction in config.RedactedReqInfo is for, and why it belongs in the same
// change as this call rather than a later one.
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

// UpdateCredential replaces the secret a credential holds, keeping its id so
// every manifest reference to it keeps working.
//
// This is the only way a rotated .env value can reach a workload. The API
// never hands a stored secret back, so nothing can tell whether the local
// value differs from the stored one: a caller cannot detect a rotation, only
// perform one, and must therefore be asked for it explicitly.
func UpdateCredential(credentialID, value string) (*Credential, error) {
	url, err := config.GetEndpointURL("/api/v2/credentials/" + escapeID(credentialID) + "/")
	if err != nil {
		return nil, err
	}

	// The type is set once, at creation, and the update route refuses it:
	// sending it back costs a 422 saying "credentialType is not allowed key",
	// which reads as a rejected value rather than as a field that may not be
	// repeated. Only the secret goes.
	body := map[string]string{"apiToken": value}

	var cred Credential

	if err := drapi.PatchJSON(url, "credential", body, &cred); err != nil {
		return nil, err
	}

	return &cred, nil
}

// DeleteCredential removes a stored credential by id. It is what teardown uses
// to clean up the credentials a workload owned, so a name is free to be reused.
//
// The two failures worth telling apart both arrive as *drapi.HTTPError: a 404
// for an id already gone (nothing to do), and a 409 for a credential still in
// use by a data connection or batch prediction job (the platform refuses, and
// the caller reports it rather than pretending it was removed).
func DeleteCredential(credentialID string) error {
	url, err := config.GetEndpointURL("/api/v2/credentials/" + escapeID(credentialID) + "/")
	if err != nil {
		return err
	}

	return drapi.DeleteJSON(url, "credential", nil, nil)
}
