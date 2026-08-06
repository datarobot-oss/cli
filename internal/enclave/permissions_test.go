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
