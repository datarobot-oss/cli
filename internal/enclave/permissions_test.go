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

package enclave

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePermission(t *testing.T) {
	for _, in := range []string{"create", "CREATE", " Create "} {
		got, err := ParsePermission(in)
		require.NoError(t, err)
		assert.Equal(t, PermissionCreate, got)
	}
}

func TestParsePermission_Pin(t *testing.T) {
	for _, in := range []string{"pin", "PIN", " Pin "} {
		got, err := ParsePermission(in)
		require.NoError(t, err)
		assert.Equal(t, PermissionPin, got)
	}
}

func TestParsePermission_RejectsUnknown(t *testing.T) {
	_, err := ParsePermission("deploy")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create")
}

func TestResolveRecipientByID(t *testing.T) {
	r, err := ResolveRecipientByID("", "", "org-1")
	require.NoError(t, err)
	assert.Equal(t, RecipientOrg, r.Type)
	assert.Equal(t, "org-1", r.Value())

	r, err = ResolveRecipientByID("u-1", "", "")
	require.NoError(t, err)
	assert.Equal(t, RecipientUser, r.Type)

	r, err = ResolveRecipientByID("", "g-1", "")
	require.NoError(t, err)
	assert.Equal(t, RecipientGroup, r.Type)
}

func TestResolveRecipientByID_Errors(t *testing.T) {
	// The error must name only the flags these commands actually offer — no --user.
	_, err := ResolveRecipientByID("", "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--user-id")
	assert.NotContains(t, err.Error(), "--user,")

	_, err = ResolveRecipientByID("u-1", "", "org-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only one recipient")
}

func TestUpdateCreateAccess_RequiresAnID(t *testing.T) {
	// A username-only recipient cannot be used: the createAccess endpoint takes
	// an id. Fail before issuing the request rather than sending an empty id.
	err := GrantCreatePermission(Recipient{Type: RecipientUser, Username: "alice@corp.io"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--user-id")

	err = RevokeCreatePermission(Recipient{Type: RecipientUser, Username: "alice@corp.io"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--user-id")
}

func TestRenderEnclavePermissions_JSONRoundTrip(t *testing.T) {
	// The JSON shape is a contract for scripts; keep the keys stable.
	p := EnclavePermissions{
		EnclaveID:     "enc-1",
		SubjectUserID: "u-1",
		Permissions:   []string{"CAN_VIEW", "CAN_DEPLOY"},
		RBACEnabled:   true,
		ViaSysAdmin:   false,
	}

	data, err := json.Marshal(p)
	require.NoError(t, err)

	var back map[string]any

	require.NoError(t, json.Unmarshal(data, &back))
	assert.Equal(t, "enc-1", back["enclaveId"])
	assert.Equal(t, "u-1", back["subjectUserId"])
	assert.Equal(t, true, back["rbacEnabled"])
	assert.Equal(t, false, back["viaSysAdmin"])
	assert.Len(t, back["permissions"], 2)
}

func TestPinPermission_UsersAndGroupsOnly(t *testing.T) {
	// The server refuses organization recipients for pin, and the endpoint takes an
	// id; fail before sending the request, naming the flags that work.
	for _, r := range []Recipient{
		{Type: RecipientOrg, ID: "org-1"},
		{Type: RecipientUser, Username: "alice@corp.io"},
		{Type: RecipientGroup},
	} {
		err := GrantPinPermission(r)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "users and groups only")
		assert.Contains(t, err.Error(), "--user-id or --group")

		err = RevokePinPermission(r)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "users and groups only")
	}
}

func TestPinPermission_GroupRecipient(t *testing.T) {
	// A group is sent to pinAccess as shareRecipientType group, for grant and revoke.
	installSkipAuth(t)

	var bodies []createAccessUpdate

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "/api/v2/enclaves/pinAccess", r.URL.Path)

		var body createAccessUpdate

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))

		bodies = append(bodies, body)

		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	installEndpoint(t, srv.URL)

	group := Recipient{Type: RecipientGroup, ID: "656f0000000000000000abcd"}

	require.NoError(t, GrantPinPermission(group))
	require.NoError(t, RevokePinPermission(group))

	assert.Equal(t, []createAccessUpdate{
		{Operation: operationGrant, ShareRecipientType: RecipientGroup, ID: group.ID},
		{Operation: operationRevoke, ShareRecipientType: RecipientGroup, ID: group.ID},
	}, bodies)
}

func TestPinPermission_GroupOnUsersOnlyServerHint(t *testing.T) {
	// A server that predates group recipients refuses them with a 422 saying pin is
	// for users only. Say what that means, and keep the HTTP error reachable.
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"detail":[{"type":"value_error","loc":["body","shareRecipientType"],` +
			`"msg":"Value error, the pin permission can be granted to users only, got group"}]}`))
	}))
	defer srv.Close()

	installEndpoint(t, srv.URL)

	group := Recipient{Type: RecipientGroup, ID: "656f0000000000000000abcd"}

	for name, err := range map[string]error{
		"grant":  GrantPinPermission(group),
		"revoke": RevokePinPermission(group),
	} {
		require.ErrorIs(t, err, errPinGroupUnsupported, name)

		var httpErr *drapi.HTTPError

		require.ErrorAs(t, err, &httpErr, name)
		assert.Equal(t, http.StatusUnprocessableEntity, httpErr.StatusCode, name)
	}
}

func TestPinPermission_UnknownGroupHasNoHint(t *testing.T) {
	// A 422 about the group itself is the server's answer to this request, not a
	// missing feature, so it passes through unchanged.
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"detail":"the recipient group id does not name a known group"}`))
	}))
	defer srv.Close()

	installEndpoint(t, srv.URL)

	err := GrantPinPermission(Recipient{Type: RecipientGroup, ID: "656f0000000000000000abcd"})
	require.Error(t, err)
	require.NotErrorIs(t, err, errPinGroupUnsupported)
	assert.Contains(t, err.Error(), "known group")
}

func TestCollectionAccessPath(t *testing.T) {
	// Pin has its own endpoint; it is not a field on createAccess.
	assert.Equal(t, "/api/v2/enclaves/createAccess", collectionAccessPath(PermissionCreate))
	assert.Equal(t, "/api/v2/enclaves/pinAccess", collectionAccessPath(PermissionPin))
}

func TestPinPermission_OlderServerHint(t *testing.T) {
	// A server without /enclaves/pinAccess answers FastAPI's default 404. Say
	// so, and keep the HTTP error reachable for callers that branch on the status.
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail":"Not Found"}`))
	}))
	defer srv.Close()

	installEndpoint(t, srv.URL)

	errs := map[string]error{
		"grant":  GrantPinPermission(Recipient{Type: RecipientUser, ID: "u-1"}),
		"revoke": RevokePinPermission(Recipient{Type: RecipientUser, ID: "u-1"}),
	}
	_, errs["list"] = ListCollectionAccess(PermissionPin)

	for name, err := range errs {
		require.Error(t, err, name)
		require.ErrorIs(t, err, errPinUnsupported, name)
		assert.Contains(t, err.Error(), "pinAccess", name)

		var httpErr *drapi.HTTPError

		require.ErrorAs(t, err, &httpErr, name)
		assert.Equal(t, http.StatusNotFound, httpErr.StatusCode, name)
	}
}

func TestPinPermission_EndpointNotFoundHasNoHint(t *testing.T) {
	// A 404 that the pin endpoint itself answered (its own detail) is about the
	// request, not a missing route, so it passes through without the hint.
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail":"Enclave access control request failed (fetch account info)"}`))
	}))
	defer srv.Close()

	installEndpoint(t, srv.URL)

	err := GrantPinPermission(Recipient{Type: RecipientUser, ID: "u-1"})
	require.Error(t, err)
	require.NotErrorIs(t, err, errPinUnsupported)
}

func TestPinList_EndpointNotFoundHasNoHint(t *testing.T) {
	// The list goes through drapi.Get, which must carry the body too: a 404 the
	// pin endpoint answered itself is not a missing route.
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail":"Enclave access control request failed (fetch account info)"}`))
	}))
	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := ListCollectionAccess(PermissionPin)
	require.Error(t, err)
	require.NotErrorIs(t, err, errPinUnsupported)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
}

func TestRouteMissing(t *testing.T) {
	assert.True(t, routeMissing([]byte(`{"detail":"Not Found"}`)))
	assert.True(t, routeMissing(nil))
	assert.True(t, routeMissing([]byte(`<html>404 Not Found</html>`)))
	assert.True(t, routeMissing([]byte(`{"message":"no route"}`)))
	assert.False(t, routeMissing([]byte(`{"detail":"user not found"}`)))
	assert.False(t, routeMissing([]byte(`{"detail":[{"msg":"bad id"}]}`)))
}

func TestCreatePermission_NotFoundHasNoPinHint(t *testing.T) {
	// The hint is for pin only; a 404 on createAccess passes through unchanged.
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	installEndpoint(t, srv.URL)

	err := GrantCreatePermission(Recipient{Type: RecipientUser, ID: "u-1"})
	require.Error(t, err)
	require.NotErrorIs(t, err, errPinUnsupported)
}
