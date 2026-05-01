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
	"github.com/datarobot/cli/internal/log"
	"github.com/denisbrodbeck/machineid"
)

// getMachineID returns a stable OS-provided machine identifier, hashed
// for privacy. Returns an empty string if the identifier cannot be obtained.
func getMachineID() string {
	id, err := machineid.ProtectedID("dr")
	if err != nil {
		// Errors at this point will cause telemetry to be less effective
		// or to fail outright, depending on if we have user-id.
		log.Warn("Failed to get machine ID:", err)
		return ""
	}

	return id
}
