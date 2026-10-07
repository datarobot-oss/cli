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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/datarobot/cli/internal/config"
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

func TestCreateCredential_PostsTheValueAndReturnsTheID(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v2/credentials/", r.URL.Path)

		var body map[string]string

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "my-app/OPENAI_API_KEY", body["name"])
		assert.Equal(t, CredentialTypeAPIToken, body["credentialType"])
		assert.Equal(t, "sk-live-abc123", body["apiToken"])

		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"credentialId":"66f1a2b3c4d5e6f7a8b9c0d1","name":"my-app/OPENAI_API_KEY"}`)
	}))

	cred, err := CreateCredential("my-app/OPENAI_API_KEY", "sk-live-abc123")
	require.NoError(t, err)
	assert.Equal(t, "66f1a2b3c4d5e6f7a8b9c0d1", cred.CredentialID)
}

// The update route takes the secret and nothing else. Sending the type back
// with it, the way the create call does, is answered by the platform with a
// 422 naming credentialType as "not allowed key", so this pins the one field
// that may travel: without it the rotation looks right against any mock and
// fails against every real instance.
func TestUpdateCredential_SendsOnlyTheSecret(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "/api/v2/credentials/66f1a2b3c4d5e6f7a8b9c0d1/", r.URL.Path)

		var body map[string]string

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, map[string]string{"apiToken": "fixture-key-rotated-d4d4"}, body)

		fmt.Fprint(w, `{"credentialId":"66f1a2b3c4d5e6f7a8b9c0d1","name":"my-app/OPENAI_API_KEY"}`)
	}))

	cred, err := UpdateCredential("66f1a2b3c4d5e6f7a8b9c0d1", "fixture-key-rotated-d4d4")
	require.NoError(t, err)

	// The id is what every manifest reference names, so a rotation that
	// changed it would leave the file pointing at the old value.
	assert.Equal(t, "66f1a2b3c4d5e6f7a8b9c0d1", cred.CredentialID)
}

// A credential id that is not there comes back as the HTTP error it is, so the
// caller can say which variable was not re-sent rather than fail the run.
func TestUpdateCredential_MissingIsAnHTTPError(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"credential not found"}`)
	}))

	_, err := UpdateCredential("66f1a2b3c4d5e6f7a8b9c0d1", "fixture-key-rotated-d4d4")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
}

// A name already in use is the caller's decision, not this function's:
// reusing whatever credential holds that name would deploy a value the user
// never supplied.
func TestCreateCredential_ConflictSurvivesAsAnHTTPError(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `{"detail":"credential name already exists"}`)
	}))

	_, err := CreateCredential("my-app/OPENAI_API_KEY", "sk-live-abc123")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusConflict, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "already exists")
}

// The debug log dumps outgoing request bodies, and this body is a secret by
// definition. Redaction is keyed on field names, so the guarantee only holds
// while the name this call sends is one of the names the redactor knows: a
// release carrying the POST but not that pairing writes the user's token, in
// the clear, into a log file in their home directory.
func TestCreateCredential_TheFieldItSendsIsOneTheDebugLogRedacts(t *testing.T) {
	var (
		sent   []byte
		readEr error
	)

	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent, readEr = io.ReadAll(r.Body)

		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"credentialId":"c1","name":"my-app/OPENAI_API_KEY"}`)
	}))

	_, err := CreateCredential("my-app/OPENAI_API_KEY", "sk-live-abc123")
	require.NoError(t, err)
	require.NoError(t, readEr)
	require.Contains(t, string(sent), "sk-live-abc123", "the platform still gets the real value")

	redacted := config.RedactSecretFields(string(sent))

	assert.NotContains(t, redacted, "sk-live-abc123", "and the debug log never does")
	assert.Contains(t, redacted, "REDACTED")
}

// The limit bounds the whole scan, not one page. This is a courtesy lookup
// that improves an error message, so a large tenant must not turn every
// refusal into a walk through every credential it has.
func TestFindCredentialNamed_StopsAtTheScanLimit(t *testing.T) {
	var (
		pages int
		base  string
	)

	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pages++

		// Always another page, and never the name being looked for: with no
		// bound this answers for as long as it is asked.
		fmt.Fprintf(w, `{"data":[{"credentialId":"c%d","name":"other"}],"next":%q}`,
			pages, base+"?page="+strconv.Itoa(pages+1))
	}))

	// Built from the configured endpoint, which serveAPI has just pointed at
	// the test server, rather than from the request the handler was sent.
	base, err := drapi.EndpointURL("/credentials/", url.Values{})
	require.NoError(t, err)

	found, err := FindCredentialNamed("wanted", 3)
	require.NoError(t, err)
	assert.Nil(t, found, "reaching the bound reads the same as not being there: no id to offer")
	assert.Equal(t, 3, pages, "one row per page, so the bound is three pages")
}

// drapi attaches the user's token to whatever URL it is given, so a next link
// naming another host would send the token there.
func TestFindCredentialNamed_RefusesANextOnAnotherHost(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w,
			`{"data":[{"credentialId":"c1","name":"other"}],"next":"https://elsewhere.example.com/api/v2/credentials/"}`)
	}))

	_, err := FindCredentialNamed("wanted", 200)
	require.Error(t, err)
}

// A name nothing holds is not an error: the caller asked whether one existed.
func TestFindCredentialNamed_AnswersNilWhenThereIsNone(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data":[],"next":""}`)
	}))

	found, err := FindCredentialNamed("wanted", 200)
	require.NoError(t, err)
	assert.Nil(t, found)
}

// The scan stops at the first match: names are unique tenant-wide, so there is
// no second, and a next link past the hit must not be followed. The second page
// answers 500 if it is ever fetched, so a refactor that read past the match
// would turn this green test red rather than pass by returning the same id.
func TestFindCredentialNamed_StopsAtTheFirstMatch(t *testing.T) {
	var (
		pages int
		base  string
	)

	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pages++

		if pages > 1 {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"message":"the scan should have stopped at the first match"}`)

			return
		}

		fmt.Fprintf(w, `{"data":[{"credentialId":"c1","name":"wanted"}],"next":%q}`, base+"?page=2")
	}))

	base, err := drapi.EndpointURL("/credentials/", url.Values{})
	require.NoError(t, err)

	found, err := FindCredentialNamed("wanted", 200)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "c1", found.CredentialID)
	assert.Equal(t, 1, pages, "a match on page one must not fetch page two")
}

// Teardown collects every "<workloadName>/" credential, and it has to follow
// next to do it: the orphans it exists to remove could sit on any page.
func TestCredentialsWithPrefix_CollectsMatchesAcrossPages(t *testing.T) {
	var base string

	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			fmt.Fprint(w, `{"data":[{"credentialId":"c3","name":"my-app/DATAROBOT_API_TOKEN"},{"credentialId":"c4","name":"other/KEY"}],"next":""}`)

			return
		}

		fmt.Fprintf(w,
			`{"data":[{"credentialId":"c1","name":"my-app/LLM_API_KEY"},{"credentialId":"c2","name":"my-app-staging/LLM_API_KEY"}],"next":%q}`,
			base+"?page=2")
	}))

	var err error

	base, err = drapi.EndpointURL("/credentials/", url.Values{})
	require.NoError(t, err)

	creds, err := CredentialsWithPrefix("my-app/", 0)
	require.NoError(t, err)

	// c2 starts with "my-app" but not "my-app/", so the prefix boundary keeps
	// one workload from deleting a differently-named workload's credentials.
	require.Len(t, creds, 2)
	assert.Equal(t, "my-app/LLM_API_KEY", creds[0].Name)
	assert.Equal(t, "my-app/DATAROBOT_API_TOKEN", creds[1].Name)
}

// The same-host guard the name lookup carries has to hold for the prefix scan
// too: drapi would send the token to whatever host a next link named.
func TestCredentialsWithPrefix_RefusesANextOnAnotherHost(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w,
			`{"data":[{"credentialId":"c1","name":"my-app/A"}],"next":"https://elsewhere.example.com/api/v2/credentials/"}`)
	}))

	_, err := CredentialsWithPrefix("my-app/", 0)
	require.Error(t, err)
}

// Nothing matching the prefix is an empty result, not an error: a workload
// with no secrets owns no credentials, and its delete cleans up nothing.
func TestCredentialsWithPrefix_EmptyWhenNoneMatch(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data":[{"credentialId":"c1","name":"other/KEY"}],"next":""}`)
	}))

	creds, err := CredentialsWithPrefix("my-app/", 0)
	require.NoError(t, err)
	assert.Empty(t, creds)
}

// With no limit the walk ends only when a page is empty or its Next clears. A
// server whose Next cycles on the same host with non-empty pages would otherwise
// loop for ever and hang the delete; maxCredentialPages turns that into an error
// instead of a hang.
func TestCredentialsWithPrefix_StopsAfterThePageCap(t *testing.T) {
	var (
		pages int
		base  string
	)

	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pages++

		// Always another non-empty page on the same host: a cursor that never ends.
		fmt.Fprintf(w, `{"data":[{"credentialId":"c%d","name":"my-app/A"}],"next":%q}`,
			pages, base+"?page="+strconv.Itoa(pages+1))
	}))

	base, err := drapi.EndpointURL("/credentials/", url.Values{})
	require.NoError(t, err)

	_, err = CredentialsWithPrefix("my-app/", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not finish listing")
	assert.Equal(t, maxCredentialPages, pages, "the walk stops at the cap, not one page past it")
}

func TestDeleteCredential_DeletesByID(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/v2/credentials/66f1a2b3c4d5e6f7a8b9c0d1/", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))

	require.NoError(t, DeleteCredential("66f1a2b3c4d5e6f7a8b9c0d1"))
}

// A pasted id with a slash in it must stay inside the credentials route rather
// than walk out to some other resource, the same guard GetCredential has.
func TestDeleteCredential_EscapesID(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/credentials/a%2Fb/", r.URL.EscapedPath())
		w.WriteHeader(http.StatusNoContent)
	}))

	require.NoError(t, DeleteCredential("a/b"))
}

// A 404 is an id already gone, and it surfaces as the HTTP error it is so the
// caller can tell it apart from a credential it actually removed.
func TestDeleteCredential_MissingIsAnHTTPError(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	err := DeleteCredential("does-not-exist")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
}

// A credential still in use by a data connection or batch job comes back 409;
// the caller reports it rather than pretending the value is gone.
func TestDeleteCredential_InUseSurvivesAsAConflict(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `{"message":"Credentials are in use by one or more data connections or batch prediction jobs."}`)
	}))

	err := DeleteCredential("in-use")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusConflict, httpErr.StatusCode)
}
