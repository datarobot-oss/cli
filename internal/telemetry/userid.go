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

// hasValidUID returns true if the cache contains a non-empty UID.
// We want to preserve the cached UID for tracking purposes, even if we
// encounter a network error and can't fetch other identifiers.
func (c accountCache) hasValidUID() bool {
	return c.UID != ""
}

func (c accountCache) matchesCurrentConfig() bool {
	return c.Endpoint == config.GetBaseURL() && c.TokenFingerprint == tokenFingerprint()
}

func (c accountCache) toResult() accountInfoResult {
	return accountInfoResult{
		UID:            c.UID,
		OrganizationID: c.OrganizationID,
		TenantID:       c.TenantID,
	}
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

func retrieveAccountInfo(ctx context.Context) (accountInfoResult, error) {
	cached, hasMatchingCache := loadMatchingCache()
	if hasMatchingCache && cached.isComplete() {
		return cached.toResult(), nil
	}

	info, err := GetAccountInfo(ctx)
	if err != nil {
		log.Debugf("Failed to retrieve account info: %v", err)

		// Network error on partial cache: return cached UID to preserve tracking
		// (org/tenant fields remain empty if we couldn't fetch them)
		if hasMatchingCache && cached.hasValidUID() {
			return accountInfoResult{
				UID: cached.UID,
			}, nil
		}

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

// loadMatchingCache attempts to load the account cache from disk and returns it along with a boolean indicating
// whether the cache matches the current configuration.
func loadMatchingCache() (accountCache, bool) {
	var cached accountCache

	if err := readJSONCacheFile(userIDFileName, &cached); err != nil {
		return cached, false
	}

	return cached, cached.matchesCurrentConfig()
}

// derefOrEmpty returns the value of the string pointer, or an empty string if the pointer is nil.
func derefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

func persistAccountInfo(result accountInfoResult) {
	cache := accountCache{
		UID:              result.UID,
		Endpoint:         config.GetBaseURL(),
		TokenFingerprint: tokenFingerprint(),
		OrganizationID:   result.OrganizationID,
		TenantID:         result.TenantID,
	}

	writeJSONCacheFile(userIDFileName, cache)
}



func tokenFingerprint() string {
	token := viperx.GetString(config.DataRobotAPIKey)
	if token == "" {
		return ""
	}

	hash := sha256.Sum256([]byte(token))

	return hex.EncodeToString(hash[:])
}
