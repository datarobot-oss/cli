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
	"os/exec"
	"strings"
)

func osVersion() string {
	out, err := exec.Command("cmd", "/c", "ver").Output()
	if err != nil {
		return ""
	}

	// "ver" output: "Microsoft Windows [Version 10.0.22621.1234]"
	s := strings.TrimSpace(string(out))
	start := strings.Index(s, "[Version ")

	if start == -1 {
		return ""
	}

	s = s[start+len("[Version "):]
	end := strings.Index(s, "]")

	if end == -1 {
		return ""
	}

	return s[:end]
}
