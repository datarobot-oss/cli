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
	"fmt"
	"net/http"
	"testing"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCredential_Decodes(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v2/credentials/66f1a2b3c4d5e6f7a8b9c0d1/", r.URL.Path)
		fmt.Fprint(w, `{"credentialId":"66f1a2b3c4d5e6f7a8b9c0d1","name":"my-app/OPENAI_API_KEY","credentialType":"api_token"}`)
	}))

	cred, err := GetCredential("66f1a2b3c4d5e6f7a8b9c0d1")
	require.NoError(t, err)
	require.NotNil(t, cred)
	assert.Equal(t, "66f1a2b3c4d5e6f7a8b9c0d1", cred.CredentialID)
	assert.Equal(t, "my-app/OPENAI_API_KEY", cred.Name)
	assert.Equal(t, "api_token", cred.CredentialType)
}

// TestGetCredential_MissingIsAnError is the whole point of the call: a
// reference to a credential that does not exist has to fail here, at plan
// time, rather than when the container tries to start.
func TestGetCredential_MissingIsAnError(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := GetCredential("does-not-exist")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
}

// TestGetCredential_EscapesID keeps a pasted id with a slash in it from
// walking out of the credentials route and hitting some other resource.
func TestGetCredential_EscapesID(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/credentials/a%2Fb/", r.URL.EscapedPath())
		fmt.Fprint(w, `{"credentialId":"x"}`)
	}))

	_, err := GetCredential("a/b")
	require.NoError(t, err)
}
