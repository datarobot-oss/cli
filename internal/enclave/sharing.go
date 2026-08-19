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
	"errors"
	"fmt"
	"strings"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
)

// sharedRolesSuffix is appended to an enclave's path to reach the sharedRoles
// sub-resource: PATCH /covalent/api/v2/outposts/{id}/sharedRoles.
const sharedRolesSuffix = "/sharedRoles"

// updateRolesOperation is the only operation the sharedRoles endpoint accepts.
const updateRolesOperation = "updateRoles"

// Server SharingRole values the CLI grants. NO_ROLE is the removal sentinel
// sent by revoke.
const (
	RoleOwner    = "OWNER"
	RoleUser     = "USER"
	RoleConsumer = "CONSUMER"
	roleNone     = "NO_ROLE"
)

// Recipient subject types (server SubjectType values, lowercase on the wire).
const (
	RecipientUser  = "user"
	RecipientGroup = "group"
	RecipientOrg   = "organization"
)

// sharedRole is one entry in a sharedRoles update: a recipient plus the role
// to set. Exactly one of Username (users only) or ID (users/groups/orgs) is
// populated, matching the server's oneof of GrantAccessControlWithUsername /
// GrantAccessControlWithId.
type sharedRole struct {
	ShareRecipientType string `json:"shareRecipientType"`
	Role               string `json:"role"`
	Username           string `json:"username,omitempty"`
	ID                 string `json:"id,omitempty"`
}

// sharedRolesUpdate is the PATCH body (server SharedRolesUpdateRequest).
type sharedRolesUpdate struct {
	Operation string       `json:"operation"`
	Roles     []sharedRole `json:"roles"`
}

// Recipient identifies who a grant/revoke targets: a subject type plus exactly
// one of a username (users only) or an id (users, groups, or organizations).
type Recipient struct {
	Type     string
	Username string
	ID       string
}

// Value returns the recipient's identifying string — its username when granted
// by username, otherwise its id.
func (r Recipient) Value() string {
	if r.Username != "" {
		return r.Username
	}

	return r.ID
}

// Describe renders "<type> <value>" for confirmation messages, e.g.
// "organization 656f0000000000000000abcd".
func (r Recipient) Describe() string {
	return r.Type + " " + r.Value()
}

// toSharedRole builds the wire entry for this recipient at the given role.
func (r Recipient) toSharedRole(role string) sharedRole {
	entry := sharedRole{ShareRecipientType: r.Type, Role: role}

	if r.Username != "" {
		entry.Username = r.Username
	} else {
		entry.ID = r.ID
	}

	return entry
}

// ParseRole maps a user-facing role name (owner|user|consumer, case-insensitive)
// to the server SharingRole value. Unknown names are rejected.
func ParseRole(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "owner":
		return RoleOwner, nil
	case "user":
		return RoleUser, nil
	case "consumer":
		return RoleConsumer, nil
	default:
		return "", fmt.Errorf("invalid role %q: use one of owner, user, consumer", value)
	}
}

// ResolveRecipient validates that exactly one recipient selector was provided
// and returns the corresponding Recipient. The flags mirror the command
// surface: --user (username), --user-id / --group / --org (id).
func ResolveRecipient(user, userID, group, org string) (Recipient, error) {
	var (
		chosen Recipient
		count  int
	)

	if user != "" {
		chosen = Recipient{Type: RecipientUser, Username: user}
		count++
	}

	if userID != "" {
		chosen = Recipient{Type: RecipientUser, ID: userID}
		count++
	}

	if group != "" {
		chosen = Recipient{Type: RecipientGroup, ID: group}
		count++
	}

	if org != "" {
		chosen = Recipient{Type: RecipientOrg, ID: org}
		count++
	}

	if count == 0 {
		return Recipient{}, errors.New(
			"a recipient is required: pass exactly one of --user, --user-id, --group, or --org")
	}

	if count > 1 {
		return Recipient{}, errors.New(
			"only one recipient may be given: pass exactly one of --user, --user-id, --group, or --org")
	}

	return chosen, nil
}

// RecipientTypeOf reports the recipient subject type implied by the given
// selector flags, for telemetry. It returns ("", false) when no selector or
// more than one is set, so callers get a best-effort label without duplicating
// ResolveRecipient's validation.
func RecipientTypeOf(user, userID, group, org string) (string, bool) {
	r, err := ResolveRecipient(user, userID, group, org)
	if err != nil {
		return "", false
	}

	return r.Type, true
}

// UpdateSharedRoles PATCHes the given role changes onto an enclave. The server
// replies 204 on success. Requires ENCLAVE_RBAC_ENABLED server-side; when
// disabled the endpoint is a 204 no-op.
func UpdateSharedRoles(enclaveID string, roles []sharedRole) error {
	url, err := config.GetEndpointURL(basePath + "/" + escapeID(enclaveID) + sharedRolesSuffix)
	if err != nil {
		return err
	}

	body := sharedRolesUpdate{Operation: updateRolesOperation, Roles: roles}

	return drapi.PatchJSON(url, "enclave", body, nil)
}

// GrantAccess grants role (a server SharingRole such as OWNER/USER/CONSUMER) to
// the recipient on the enclave.
func GrantAccess(enclaveID, role string, r Recipient) error {
	return UpdateSharedRoles(enclaveID, []sharedRole{r.toSharedRole(role)})
}

// RevokeAccess removes the recipient's access to the enclave by sending the
// NO_ROLE removal sentinel.
func RevokeAccess(enclaveID string, r Recipient) error {
	return UpdateSharedRoles(enclaveID, []sharedRole{r.toSharedRole(roleNone)})
}
