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

package testutil

import (
	"context"
	"errors"
	"testing"

	"github.com/datarobot/cli/internal/config"
)

// StubAPIToken replaces config.GetAPITokenFunc for the duration of the test and
// restores it via t.Cleanup — no defer needed. Pass an empty string to simulate
// a missing/invalid token (the stub returns an error, matching real behaviour).
//
// Tests inside package drapi should use their own StubAPIToken wrapper
// (drapi/testutil_test.go) which also resets the unexported token cache.
func StubAPIToken(t *testing.T, tok string) {
	t.Helper()

	original := config.GetAPITokenFunc

	if tok == "" {
		config.GetAPITokenFunc = func(_ context.Context) (string, error) {
			return "", errors.New("empty token")
		}
	} else {
		config.GetAPITokenFunc = func(_ context.Context) (string, error) { return tok, nil }
	}

	t.Cleanup(func() { config.GetAPITokenFunc = original })
}
