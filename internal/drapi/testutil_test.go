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

package drapi

import (
	"testing"

	"github.com/datarobot/cli/internal/testutil"
)

// StubAPIToken resets the unexported token cache and delegates the
// config.GetAPITokenFunc swap to testutil.StubAPIToken. Tests outside this
// package (e.g. internal/telemetry) call testutil.StubAPIToken directly —
// they cannot access the unexported cache vars.
func StubAPIToken(t *testing.T, tok string) {
	t.Helper()

	token, errToken = tok, nil

	t.Cleanup(func() { token, errToken = "", nil })

	testutil.StubAPIToken(t, tok)
}
