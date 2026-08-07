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
	"fmt"
	"net/url"
	"strings"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
)

// createAccessPath is the collection-level permission sub-resource:
// PATCH /covalent/api/v2/outposts/createAccess. Unlike sharedRoles it targets no
// single enclave — it governs who may create them.
const createAccessPath = "/createAccess"

// Operations accepted by the createAccess endpoint.
const (
	operationGrant  = "grant"
	operationRevoke = "revoke"
)

// PermissionCreate is the user-facing name of the collection-level permission
// that allows registering new enclaves (server: CAN_CREATE).
const PermissionCreate = "create"

// createAccessUpdate is the PATCH body (server EnclaveCreateAccessRequest).
// The endpoint identifies recipients by id only — there is no username form, so
// users must be named with their DataRobot user id.
type createAccessUpdate struct {
	Operation          string `json:"operation"`
	ShareRecipientType string `json:"shareRecipientType"`
	ID                 string `json:"id"`
}

// ParsePermission maps a user-facing permission name to its canonical form.
// Only "create" exists today; the flag is kept so further collection-level
// permissions can be added without changing the command surface.
func ParsePermission(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case PermissionCreate:
		return PermissionCreate, nil
	default:
		return "", fmt.Errorf("invalid permission %q: the only supported permission is create", value)
	}
}

// ResolveRecipientByID validates that exactly one id-based recipient selector
// was provided. The createAccess endpoint takes an id, so these commands offer
// no --user (username) form — hence a distinct error message from
// ResolveRecipient, naming only the flags that actually exist here.
func ResolveRecipientByID(userID, group, org string) (Recipient, error) {
	var (
		chosen Recipient
		count  int
	)

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
		return Recipient{}, fmt.Errorf(
			"a recipient is required: pass exactly one of --user-id, --group, or --org")
	}

	if count > 1 {
		return Recipient{}, fmt.Errorf(
			"only one recipient may be given: pass exactly one of --user-id, --group, or --org")
	}

	return chosen, nil
}

// RecipientTypeByIDOf reports the recipient subject type implied by the id-based
// selector flags, for telemetry. It returns ("", false) when no selector or more
// than one is set, so callers get a best-effort label without duplicating
// ResolveRecipientByID's validation.
func RecipientTypeByIDOf(userID, group, org string) (string, bool) {
	r, err := ResolveRecipientByID(userID, group, org)
	if err != nil {
		return "", false
	}

	return r.Type, true
}

// updateCreateAccess PATCHes a grant or revoke of the create permission for the
// given recipient. The server replies 204 on success. Requires
// ENCLAVE_RBAC_ENABLED server-side; when disabled the endpoint is a 204 no-op.
func updateCreateAccess(operation string, r Recipient) error {
	if r.ID == "" {
		return fmt.Errorf(
			"the enclave create permission is granted by id: use --user-id, --group, or --org")
	}

	// Named endpoint, not url: the net/url package is imported in this file.
	endpoint, err := config.GetEndpointURL(basePath + createAccessPath)
	if err != nil {
		return err
	}

	body := createAccessUpdate{
		Operation:          operation,
		ShareRecipientType: r.Type,
		ID:                 r.ID,
	}

	return drapi.PatchJSON(endpoint, "enclave", body, nil)
}

// GrantCreatePermission allows the recipient to create (register) enclaves.
func GrantCreatePermission(r Recipient) error {
	return updateCreateAccess(operationGrant, r)
}

// RevokeCreatePermission withdraws the recipient's ability to create enclaves.
func RevokeCreatePermission(r Recipient) error {
	return updateCreateAccess(operationRevoke, r)
}

// permissionsSuffix is the effective-permissions sub-resource on an enclave:
// GET /covalent/api/v2/outposts/{id}/permissions.
const permissionsSuffix = "/permissions"

// EnclavePermissions is the effective permission set a subject holds on an
// enclave, plus how that set was reached. RBACEnabled=false or ViaSysAdmin=true
// both yield the full set without any grant, which is otherwise
// indistinguishable from genuinely being an owner.
type EnclavePermissions struct {
	EnclaveID     string   `json:"enclaveId"`
	SubjectUserID string   `json:"subjectUserId"`
	Permissions   []string `json:"permissions"`
	RBACEnabled   bool     `json:"rbacEnabled"`
	ViaSysAdmin   bool     `json:"viaSysAdmin"`
}

// GetEnclavePermissions fetches the effective permissions on an enclave. An empty
// userID means the calling user; naming another user requires a system
// administrator server-side.
func GetEnclavePermissions(enclaveID, userID string) (*EnclavePermissions, error) {
	path := basePath + "/" + escapeID(enclaveID) + permissionsSuffix
	if userID != "" {
		path += "?userId=" + url.QueryEscape(userID)
	}

	endpoint, err := config.GetEndpointURL(path)
	if err != nil {
		return nil, err
	}

	var permissions EnclavePermissions

	if err := drapi.GetJSON(endpoint, "enclave permissions", &permissions); err != nil {
		return nil, err
	}

	return &permissions, nil
}
