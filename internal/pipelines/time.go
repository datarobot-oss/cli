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

package pipelines

import (
	"fmt"
	"strings"
	"time"
)

// Time wraps time.Time to accept the pipelines-api's naive UTC datetime strings
// (e.g. "2026-05-20T15:39:57.913317") in addition to standard RFC3339. Bare
// strings — those without a Z or ±HH:MM offset — are assumed to be UTC.
// All time.Time methods are promoted, so callers can use .UTC().Format(...)
// as normal.
type Time struct {
	time.Time
}

func (t *Time) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	if s == "" || s == "null" {
		t.Time = time.Time{}

		return nil
	}

	if parsed, err := time.Parse(time.RFC3339Nano, s); err == nil {
		t.Time = parsed

		return nil
	}

	// No timezone indicator — treat as UTC.
	if parsed, err := time.Parse(time.RFC3339Nano, s+"Z"); err == nil {
		t.Time = parsed

		return nil
	}

	return fmt.Errorf("pipelines: cannot parse time %q", s)
}
