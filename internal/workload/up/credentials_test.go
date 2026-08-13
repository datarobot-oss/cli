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

package up

import (
	"errors"
	"net/http"
	"testing"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubCredentials swaps the lookup for one that answers from ids, recording
// what it was asked for.
func stubCredentials(t *testing.T, present map[string]bool, failWith error) *[]string {
	t.Helper()

	asked := make([]string, 0, 4)

	force(t, &getCredentialFn, func(id string) (*workload.Credential, error) {
		asked = append(asked, id)

		if failWith != nil {
			return nil, failWith
		}

		if !present[id] {
			return nil, &drapi.HTTPError{StatusCode: http.StatusNotFound}
		}

		return &workload.Credential{CredentialID: id, Name: "cred-" + id}, nil
	})

	return &asked
}

func TestVerifyCredentials_NoRefsAsksNothing(t *testing.T) {
	asked := stubCredentials(t, nil, nil)

	require.NoError(t, verifyCredentials(nil))
	assert.Empty(t, *asked, "a manifest with no references must not touch the network")
}

func TestVerifyCredentials_PresentPasses(t *testing.T) {
	asked := stubCredentials(t, map[string]bool{"cred-1": true}, nil)

	err := verifyCredentials([]manifest.CredentialRef{
		{CredentialID: "cred-1", Key: "apiToken", EnvName: "OPENAI_API_KEY", Line: 12},
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"cred-1"}, *asked)
}

// One GET per credential, not per reference: several variables commonly read
// different keys of the same stored credential, and a deploy should not pay
// for the same lookup twice.
func TestVerifyCredentials_DeduplicatesByID(t *testing.T) {
	asked := stubCredentials(t, map[string]bool{"cred-1": true}, nil)

	err := verifyCredentials([]manifest.CredentialRef{
		{CredentialID: "cred-1", Key: "apiToken", EnvName: "TOKEN_A", Line: 12},
		{CredentialID: "cred-1", Key: "password", EnvName: "TOKEN_B", Line: 15},
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"cred-1"}, *asked)
}

// The missing-id error has to name the variable and the line, not only the
// id: the id is a hex string the reader would otherwise have to go hunting
// for in the file.
func TestVerifyCredentials_MissingNamesVariableAndLine(t *testing.T) {
	stubCredentials(t, map[string]bool{}, nil)

	err := verifyCredentials([]manifest.CredentialRef{
		{CredentialID: "cred-gone", Key: "apiToken", EnvName: "OPENAI_API_KEY", Line: 12},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "OPENAI_API_KEY")
	assert.Contains(t, err.Error(), "cred-gone")
	assert.Contains(t, err.Error(), "line 12")
}

// The wizard writes the placeholder in full rather than commenting the entry
// out, precisely so the deploy stops here instead of starting a container
// without its secret. It never reaches the network.
func TestVerifyCredentials_PlaceholderIsRefusedWithoutALookup(t *testing.T) {
	asked := stubCredentials(t, nil, nil)

	err := verifyCredentials([]manifest.CredentialRef{
		{CredentialID: manifest.CredentialPlaceholder, Key: "apiToken", EnvName: "OPENAI_API_KEY", Line: 12},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), manifest.CredentialPlaceholder)
	assert.Contains(t, err.Error(), "OPENAI_API_KEY")
	assert.Empty(t, *asked, "a placeholder is known to be unusable without asking the platform")
}

// Every problem is reported at once. The refs come from a file the user is
// about to edit, and learning about the second bad id only after fixing the
// first is two trips through a build pipeline for one edit.
func TestVerifyCredentials_ReportsEveryProblem(t *testing.T) {
	stubCredentials(t, map[string]bool{"cred-ok": true}, nil)

	err := verifyCredentials([]manifest.CredentialRef{
		{CredentialID: manifest.CredentialPlaceholder, EnvName: "FIRST", Line: 10},
		{CredentialID: "cred-ok", EnvName: "SECOND", Line: 13},
		{CredentialID: "cred-gone", EnvName: "THIRD", Line: 16},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "FIRST")
	assert.Contains(t, err.Error(), "THIRD")
	assert.NotContains(t, err.Error(), "SECOND", "the reference that resolved is not a problem")
}

// A transport failure means the references were not checked, which is not the
// same as their being wrong. Reporting a credential as missing because the
// platform was briefly unreachable would send the user editing a correct file.
func TestVerifyCredentials_TransportFailureIsNotAMissingCredential(t *testing.T) {
	stubCredentials(t, nil, &drapi.HTTPError{StatusCode: http.StatusBadGateway})

	err := verifyCredentials([]manifest.CredentialRef{
		{CredentialID: "cred-1", EnvName: "OPENAI_API_KEY", Line: 12},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot verify")
	assert.NotContains(t, err.Error(), "does not exist")

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr, "the underlying failure has to survive for the caller to inspect")
	assert.Equal(t, http.StatusBadGateway, httpErr.StatusCode)
}

func TestVerifyCredentials_NonHTTPErrorPropagates(t *testing.T) {
	stubCredentials(t, nil, errors.New("no endpoint configured"))

	err := verifyCredentials([]manifest.CredentialRef{{CredentialID: "cred-1", EnvName: "X", Line: 1}})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no endpoint configured")
}
