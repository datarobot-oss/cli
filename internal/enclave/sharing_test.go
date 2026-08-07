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
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRole(t *testing.T) {
	cases := map[string]string{
		"owner":      RoleOwner,
		"USER":       RoleUser,
		" Consumer ": RoleConsumer,
	}

	for in, want := range cases {
		got, err := ParseRole(in)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}

	_, err := ParseRole("admin")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "owner, user, consumer")
}

func TestResolveRecipient(t *testing.T) {
	t.Run("user by username", func(t *testing.T) {
		r, err := ResolveRecipient("alice@corp.io", "", "", "")
		require.NoError(t, err)
		assert.Equal(t, Recipient{Type: RecipientUser, Username: "alice@corp.io"}, r)
		assert.Equal(t, "alice@corp.io", r.Value())
	})

	t.Run("user by id", func(t *testing.T) {
		r, err := ResolveRecipient("", "uid-1", "", "")
		require.NoError(t, err)
		assert.Equal(t, Recipient{Type: RecipientUser, ID: "uid-1"}, r)
		assert.Equal(t, "uid-1", r.Value())
	})

	t.Run("group", func(t *testing.T) {
		r, err := ResolveRecipient("", "", "grp-1", "")
		require.NoError(t, err)
		assert.Equal(t, RecipientGroup, r.Type)
	})

	t.Run("org", func(t *testing.T) {
		r, err := ResolveRecipient("", "", "", "org-1")
		require.NoError(t, err)
		assert.Equal(t, RecipientOrg, r.Type)
		assert.Equal(t, "organization org-1", r.Describe())
	})

	t.Run("none is an error", func(t *testing.T) {
		_, err := ResolveRecipient("", "", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "recipient is required")
	})

	t.Run("more than one is an error", func(t *testing.T) {
		_, err := ResolveRecipient("alice", "", "grp-1", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only one recipient")
	})
}

func TestRecipientTypeOf(t *testing.T) {
	typ, ok := RecipientTypeOf("", "", "", "org-1")
	assert.True(t, ok)
	assert.Equal(t, RecipientOrg, typ)

	_, ok = RecipientTypeOf("", "", "", "")
	assert.False(t, ok)

	_, ok = RecipientTypeOf("alice", "uid", "", "")
	assert.False(t, ok)
}

func TestGrantAccessSendsPatch(t *testing.T) {
	installSkipAuth(t)

	var (
		gotBody sharedRolesUpdate
		gotPath string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)

		gotPath = r.URL.Path

		raw, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(raw, &gotBody))

		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	installEndpoint(t, srv.URL)

	err := GrantAccess("abc-123", RoleUser, Recipient{Type: RecipientOrg, ID: "org-1"})
	require.NoError(t, err)

	assert.Equal(t, "/covalent/api/v2/outposts/abc-123/sharedRoles", gotPath)
	assert.Equal(t, "updateRoles", gotBody.Operation)
	require.Len(t, gotBody.Roles, 1)
	assert.Equal(t, sharedRole{ShareRecipientType: RecipientOrg, Role: RoleUser, ID: "org-1"}, gotBody.Roles[0])
}

func TestGrantAccessByUsernameOmitsID(t *testing.T) {
	installSkipAuth(t)

	var gotBody sharedRolesUpdate

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(raw, &gotBody))

		// Assert the raw body carries username and no id key for a username grant.
		assert.Contains(t, string(raw), `"username":"alice@corp.io"`)
		assert.NotContains(t, string(raw), `"id"`)

		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	installEndpoint(t, srv.URL)

	err := GrantAccess("abc-123", RoleOwner, Recipient{Type: RecipientUser, Username: "alice@corp.io"})
	require.NoError(t, err)

	require.Len(t, gotBody.Roles, 1)
	assert.Equal(t, "alice@corp.io", gotBody.Roles[0].Username)
	assert.Empty(t, gotBody.Roles[0].ID)
}

func TestRevokeAccessSendsNoRole(t *testing.T) {
	installSkipAuth(t)

	var gotBody sharedRolesUpdate

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)

		raw, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(raw, &gotBody))

		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	installEndpoint(t, srv.URL)

	err := RevokeAccess("abc-123", Recipient{Type: RecipientOrg, ID: "org-1"})
	require.NoError(t, err)

	require.Len(t, gotBody.Roles, 1)
	assert.Equal(t, roleNone, gotBody.Roles[0].Role)
	assert.Equal(t, "org-1", gotBody.Roles[0].ID)
}
