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
	"context"
	"errors"
	"testing"
)

// StubAPIToken seeds the package-level token cache and stubs GetAPITokenFunc for
// the duration of the test. Cleanup is registered via t.Cleanup — no defer needed.
// Pass an empty string to simulate a missing/invalid token (GetAPITokenFunc will
// return an error, matching real behaviour when no credentials exist).
//
// Note: this is a same-package test helper with access to unexported cache vars.
// Tests outside package drapi (e.g. internal/telemetry) use their own equivalent
// that only swaps GetAPITokenFunc — they cannot reach the cache vars directly.
func StubAPIToken(t *testing.T, tok string) {
	t.Helper()

	token, errToken = tok, nil

	t.Cleanup(func() { token, errToken = "", nil })

	origFunc := GetAPITokenFunc

	if tok == "" {
		GetAPITokenFunc = func(_ context.Context) (string, error) {
			return "", errors.New("empty token")
		}
	} else {
		GetAPITokenFunc = func(_ context.Context) (string, error) { return tok, nil }
	}

	t.Cleanup(func() { GetAPITokenFunc = origFunc })
}
