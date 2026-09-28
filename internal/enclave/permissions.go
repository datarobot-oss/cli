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
	"net/url"
	"strings"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
)

// createAccessPath is the collection-level permission sub-resource:
// PATCH /api/v2/enclaves/createAccess. Unlike sharedRoles it targets no
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

// PermissionPin is the user-facing name of the collection-level permission that
// allows pinning a workload to one chosen enclave, overriding the scheduler's
// placement (server: CAN_OVERRIDE_WORKLOAD_PLACEMENT). Pinning is separate from
// deploy access: the pinned enclave must still be one the workload's use case
// allows and the user can deploy to. The create permission implies pin.
const PermissionPin = "pin"

// createAccessUpdate is the PATCH body (server EnclaveCreateAccessRequest).
// The endpoint identifies recipients by id only — there is no username form, so
// users must be named with their DataRobot user id. Permission is omitted for
// "create" so the request stays compatible with servers that predate "pin".
type createAccessUpdate struct {
	Operation          string `json:"operation"`
	ShareRecipientType string `json:"shareRecipientType"`
	ID                 string `json:"id"`
	Permission         string `json:"permission,omitempty"`
}

// ParsePermission maps a user-facing permission name to its canonical form.
func ParsePermission(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case PermissionCreate:
		return PermissionCreate, nil
	case PermissionPin:
		return PermissionPin, nil
	default:
		return "", fmt.Errorf(
			"invalid permission %q: the supported permissions are create and pin", value)
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
		return Recipient{}, errors.New(
			"a recipient is required: pass exactly one of --user-id, --group, or --org")
	}

	if count > 1 {
		return Recipient{}, errors.New(
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

// updateCollectionAccess PATCHes a grant or revoke of a collection-level
// permission for the given recipient. The server replies 204 on success.
// Requires ENCLAVE_RBAC_ENABLED server-side; when disabled the endpoint is a
// 204 no-op.
func updateCollectionAccess(operation, permission string, r Recipient) error {
	if r.ID == "" {
		return errors.New(
			"collection-level enclave permissions are granted by id: use --user-id, --group, or --org")
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
	// "create" is the server default; omitting it keeps the request compatible
	// with servers that predate the pin permission.
	if permission != PermissionCreate {
		body.Permission = permission
	}

	return drapi.PatchJSON(endpoint, "enclave", body, nil)
}

// GrantCollectionPermission grants the named collection-level permission
// ("create" or "pin") to the recipient.
func GrantCollectionPermission(permission string, r Recipient) error {
	return updateCollectionAccess(operationGrant, permission, r)
}

// RevokeCollectionPermission withdraws the named collection-level permission
// from the recipient. The server refuses to revoke pin from a recipient who
// holds create, since create implies pin — revoke create instead.
func RevokeCollectionPermission(permission string, r Recipient) error {
	return updateCollectionAccess(operationRevoke, permission, r)
}

// permissionsSuffix is the effective-permissions sub-resource on an enclave:
// GET /api/v2/enclaves/{id}/permissions.
const permissionsSuffix = "/permissions"

// EnclavePermissions is the effective permission set a subject holds on an
// enclave, plus how that set was reached. RBACEnabled=false or ViaSysAdmin=true
// both yield the full set without any grant, which is otherwise
// indistinguishable from genuinely being an owner.
type EnclavePermissions struct {
	EnclaveID     string   `json:"enclaveId"`
	SubjectUserID string   `json:"subjectUserId"`
	Permissions   []string `json:"permissions"`
	// GrantedPermissions is what the subject was actually granted. It differs from
	// Permissions when the answer came from a bypass, which is the only way to tell
	// whether a share landed.
	GrantedPermissions []string `json:"grantedPermissions"`
	RBACEnabled        bool     `json:"rbacEnabled"`
	ViaSysAdmin        bool     `json:"viaSysAdmin"`
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

// SharedRole is one recipient of an enclave and the role they hold on it.
type SharedRole struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	ShareRecipientType string `json:"shareRecipientType"`
	Role               string `json:"role"`
}

// ListSharedRoles returns who holds which role on the enclave. An empty result
// means nothing has been shared yet (or the server has RBAC disabled).
func ListSharedRoles(enclaveID string) ([]SharedRole, error) {
	endpoint, err := config.GetEndpointURL(basePath + "/" + escapeID(enclaveID) + sharedRolesSuffix)
	if err != nil {
		return nil, err
	}

	var roles []SharedRole

	if err := drapi.GetJSON(endpoint, "enclave shared roles", &roles); err != nil {
		return nil, err
	}

	return roles, nil
}

// CollectionPermissions is what a subject may do that is not tied to a single
// enclave (today: create one), plus how that answer was reached.
type CollectionPermissions struct {
	SubjectUserID      string   `json:"subjectUserId"`
	Permissions        []string `json:"permissions"`
	GrantedPermissions []string `json:"grantedPermissions"`
	RBACEnabled        bool     `json:"rbacEnabled"`
	ViaSysAdmin        bool     `json:"viaSysAdmin"`
}

// CreateAccessHolder is a recipient holding a collection-level enclave
// permission.
type CreateAccessHolder struct {
	ShareRecipientType string   `json:"shareRecipientType"`
	ID                 string   `json:"id"`
	Permissions        []string `json:"permissions"`
}

// GetCollectionPermissions reports whether a subject may create enclaves. An empty
// userID means the calling user; naming another requires a system administrator.
func GetCollectionPermissions(userID string) (*CollectionPermissions, error) {
	path := basePath + permissionsSuffix
	if userID != "" {
		path += "?userId=" + url.QueryEscape(userID)
	}

	endpoint, err := config.GetEndpointURL(path)
	if err != nil {
		return nil, err
	}

	var permissions CollectionPermissions

	if err := drapi.GetJSON(endpoint, "enclave permissions", &permissions); err != nil {
		return nil, err
	}

	return &permissions, nil
}

// ListCollectionAccess returns who holds the named collection-level permission
// ("create" or "pin"). System administrators only. The query parameter is
// omitted for "create", the server default, so the request stays compatible
// with servers that predate the pin permission.
func ListCollectionAccess(permission string) ([]CreateAccessHolder, error) {
	path := basePath + createAccessPath
	if permission != PermissionCreate {
		path += "?permission=" + url.QueryEscape(permission)
	}

	endpoint, err := config.GetEndpointURL(path)
	if err != nil {
		return nil, err
	}

	var holders []CreateAccessHolder

	if err := drapi.GetJSON(endpoint, "enclave create access", &holders); err != nil {
		return nil, err
	}

	return holders, nil
}
