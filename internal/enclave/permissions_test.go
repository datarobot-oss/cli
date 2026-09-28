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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePermission(t *testing.T) {
	for _, in := range []string{"create", "CREATE", " Create "} {
		got, err := ParsePermission(in)
		require.NoError(t, err)
		assert.Equal(t, PermissionCreate, got)
	}

	for _, in := range []string{"pin", "PIN", " Pin "} {
		got, err := ParsePermission(in)
		require.NoError(t, err)
		assert.Equal(t, PermissionPin, got)
	}
}

func TestParsePermission_RejectsUnknown(t *testing.T) {
	// deploy is a per-enclave permission (dr enclave access), not a
	// collection-level one; the error should name what IS supported.
	_, err := ParsePermission("deploy")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create")
	assert.Contains(t, err.Error(), "pin")
}

func TestCreateAccessUpdate_PermissionOnTheWire(t *testing.T) {
	// "create" is the server default and must be OMITTED so requests keep
	// working against servers that predate the pin permission; "pin" must be
	// carried explicitly.
	create, err := json.Marshal(createAccessUpdate{
		Operation: "grant", ShareRecipientType: RecipientUser, ID: "u-1",
	})
	require.NoError(t, err)
	assert.NotContains(t, string(create), "permission")

	pin, err := json.Marshal(createAccessUpdate{
		Operation: "grant", ShareRecipientType: RecipientUser, ID: "u-1",
		Permission: PermissionPin,
	})
	require.NoError(t, err)
	assert.Contains(t, string(pin), `"permission":"pin"`)
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

func TestUpdateCollectionAccess_RequiresAnID(t *testing.T) {
	// A username-only recipient cannot be used: the createAccess endpoint takes
	// an id. Fail before issuing the request rather than sending an empty id.
	for _, permission := range []string{PermissionCreate, PermissionPin} {
		err := GrantCollectionPermission(
			permission, Recipient{Type: RecipientUser, Username: "alice@corp.io"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--user-id")

		err = RevokeCollectionPermission(
			permission, Recipient{Type: RecipientUser, Username: "alice@corp.io"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--user-id")
	}
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
