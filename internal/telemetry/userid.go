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
)

// AccountInfo represents the response from GET /api/v2/account/info/.
type AccountInfo struct {
	UID       string `json:"uid"`
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	TenantID  string `json:"tenantId"`
	OrgID     string `json:"orgId"`
}

type cachedUserID struct {
	UID              string `json:"uid"`
	Endpoint         string `json:"endpoint"`
	TokenFingerprint string `json:"token_fingerprint"`
}

// GetUserID fetches the DataRobot user uid from GET /api/v2/account/info/.
// It returns the uid string on success, or ("", error) on non-200 status,
// empty uid, or network failure.
func GetUserID(ctx context.Context) (string, error) {
	url, err := config.GetEndpointURL("/api/v2/account/info/")
	if err != nil {
		return "", err
	}

	var info AccountInfo

	//nolint:contextcheck // GetJSON does not yet accept context; ctx is reserved for future use
	if err := drapi.GetJSON(url, "", &info); err != nil {
		return "", err
	}

	if info.UID == "" {
		return "", errors.New("empty uid in account info response")
	}

	return info.UID, nil
}

func getOrCreateUserID(apiUserID string) string {
	if apiUserID != "" {
		persistUserID(apiUserID)

		return apiUserID
	}

	var cached cachedUserID

	if err := readJSONCacheFile(userIDFileName, &cached); err != nil {
		return ""
	}

	if cached.Endpoint != currentEndpoint() {
		return ""
	}

	if cached.TokenFingerprint != tokenFingerprint() {
		return ""
	}

	return cached.UID
}

func persistUserID(uid string) {
	cache := cachedUserID{
		UID:              uid,
		Endpoint:         currentEndpoint(),
		TokenFingerprint: tokenFingerprint(),
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
