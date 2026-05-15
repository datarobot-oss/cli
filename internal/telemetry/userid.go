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

package telemetry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/log"
)

// AccountInfo represents the response from GET /api/v2/account/info/.
type AccountInfo struct {
	UID       string  `json:"uid"`
	Email     string  `json:"email"`
	FirstName string  `json:"firstName"`
	LastName  string  `json:"lastName"`
	TenantID  *string `json:"tenantId"`
	OrgID     string  `json:"orgId"`
}

type accountCache struct {
	UID              string `json:"uid"`
	Endpoint         string `json:"endpoint"`
	TokenFingerprint string `json:"token_fingerprint"`
	OrganizationID   string `json:"organization_id"`
	TenantID         string `json:"tenant_id"`
}

// isComplete returns true when the cache contains all account fields
// that are expected to be present. Old cache files (from CLI versions
// before organization_id/tenant_id were added) have OrganizationID == ""
// and are treated as partial, triggering a re-fetch.
//
// tenant_id may legitimately be null/empty from the API for legacy users
// or system accounts, so we do not include it in the completeness check.
func (c accountCache) isComplete() bool {
	return c.UID != "" && c.OrganizationID != ""
}

type accountInfoResult struct {
	UID            string
	OrganizationID string
	TenantID       string
}

// GetAccountInfo fetches the DataRobot account info from GET /api/v2/account/info/.
// It returns the full AccountInfo on success, or (*AccountInfo, error) on non-200 status,
// empty uid, or network failure.
func GetAccountInfo(_ context.Context) (*AccountInfo, error) {
	url, err := config.GetEndpointURL("/api/v2/account/info/")
	if err != nil {
		return nil, err
	}

	var info AccountInfo

	//nolint:contextcheck // GetJSON does not yet accept context; ctx is reserved for future use
	if err := drapi.GetJSON(url, "", &info); err != nil {
		return nil, err
	}

	if info.UID == "" {
		return nil, errors.New("empty uid in account info response")
	}

	return &info, nil
}

// GetUserID fetches the DataRobot user uid from GET /api/v2/account/info/.
// It returns the uid string on success, or ("", error) on non-200 status,
// empty uid, or network failure.
func GetUserID(ctx context.Context) (string, error) {
	info, err := GetAccountInfo(ctx)
	if err != nil {
		return "", err
	}

	return info.UID, nil
}

func retrieveAccountInfo(ctx context.Context) (accountInfoResult, error) {
	// Check cache first to avoid making an API call
	var cached accountCache

	if err := readJSONCacheFile(userIDFileName, &cached); err == nil {
		if cached.Endpoint == currentEndpoint() && cached.TokenFingerprint == tokenFingerprint() {
			if cached.isComplete() {
				return accountInfoResult{
					UID:            cached.UID,
					OrganizationID: cached.OrganizationID,
					TenantID:       cached.TenantID,
				}, nil
			}

			// Partial cache: endpoint/token match but missing org/tenant.
			// Fall through to re-fetch from API and upgrade the cache in place.
		}
	}

	// Cache miss or partial; try to fetch from API
	info, err := GetAccountInfo(ctx)
	if err != nil {
		log.Debugf("Failed to retrieve account info: %v", err)

		return accountInfoResult{}, err
	}

	result := accountInfoResult{
		UID:            info.UID,
		OrganizationID: info.OrgID,
		TenantID:       derefOrEmpty(info.TenantID),
	}

	persistAccountInfo(result)

	return result, nil
}

func derefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

func persistAccountInfo(result accountInfoResult) {
	cache := accountCache{
		UID:              result.UID,
		Endpoint:         currentEndpoint(),
		TokenFingerprint: tokenFingerprint(),
		OrganizationID:   result.OrganizationID,
		TenantID:         result.TenantID,
	}

	writeJSONCacheFile(userIDFileName, cache)
}

func currentEndpoint() string {
	if endpoint := viperx.GetString(config.DataRobotURL); endpoint != "" {
		if baseURL, err := config.SchemeHostOnly(endpoint); err == nil {
			return baseURL
		}
	}

	return ""
}

func tokenFingerprint() string {
	token := viperx.GetString(config.DataRobotAPIKey)
	if token == "" {
		return ""
	}

	hash := sha256.Sum256([]byte(token))

	return hex.EncodeToString(hash[:])
}
