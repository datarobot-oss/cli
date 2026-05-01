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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetMachineID_IsStable(t *testing.T) {
	id1 := getMachineID()
	id2 := getMachineID()

	assert.Equal(t, id1, id2, "machine ID should be stable across calls")
}

func TestGetMachineID_ReturnsNonEmpty(t *testing.T) {
	id := getMachineID()

	assert.NotEmpty(t, id, "expected a machine ID on this platform")
}

// machineid.ProtectedID returns a HMAC-SHA256 hex string: 32 bytes = 64 hex chars.
func TestGetMachineID_IsHexString(t *testing.T) {
	id := getMachineID()
	if id == "" {
		t.Skip("machine ID not available on this platform")
	}

	assert.Regexp(t, `^[0-9a-f]{64}$`, id, "machine ID should be 64-char lowercase hex")
}

func TestGetOrCreateDeviceID_IsStable(t *testing.T) {
	id1 := getOrCreateDeviceID()
	id2 := getOrCreateDeviceID()

	assert.NotEmpty(t, id1)
	assert.Equal(t, id1, id2, "device ID should be stable across calls")
}
