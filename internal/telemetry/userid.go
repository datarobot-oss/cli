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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
)

const userIDFileName = "user_id"

type cachedUserID struct {
	UID              string `json:"uid"`
	Endpoint         string `json:"endpoint"`
	TokenFingerprint string `json:"token_fingerprint"`
}

func getOrCreateUserID(apiUserID string) string {
	if apiUserID != "" {
		persistUserID(apiUserID)

		return apiUserID
	}

	configDir, err := config.GetConfigDir()
	if err != nil {
		return ""
	}

	cachePath := filepath.Join(configDir, userIDFileName)

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return ""
	}

	var cached cachedUserID

	if err := json.Unmarshal(data, &cached); err != nil {
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

	data, err := json.Marshal(cache)
	if err != nil {
		return
	}

	configDir, err := config.GetConfigDir()
	if err != nil {
		return
	}

	cachePath := filepath.Join(configDir, userIDFileName)

	if mkErr := os.MkdirAll(configDir, 0o700); mkErr != nil {
		return
	}

	_ = os.WriteFile(cachePath, data, 0o600)
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
