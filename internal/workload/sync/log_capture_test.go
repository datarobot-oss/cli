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

package sync

import (
	"bytes"
	"os"
	"testing"

	"github.com/datarobot/cli/internal/log"
	"github.com/stretchr/testify/require"
)

// captureWarnLog redirects os.Stderr to a pipe and reinitializes the log
// package's stderr logger, runs fn, and returns everything the logger wrote
// during fn. This is the seam for pinning phase warnings (the .wapiignore
// shadow warning, and later the --verify divergence notice) that are emitted
// only through log.Warn and therefore never reach an engine accessor.
//
// It mirrors the captureLog helper in internal/tools — the repo's existing
// log-capture pattern — rather than inventing a mechanism. The helper cannot
// be imported from another package's _test.go file, so it is copied here
// with the same semantics: the logger is built against the redirected
// os.Stderr, writes synchronously on each call, and StopStderr in a cleanup
// restores a nil logger so no later test writes through the stale pipe.
// Nothing in the log package changes; production behaviour is untouched.
//
// Tests using this helper must not call t.Parallel: the os.Stderr swap is
// process-global while fn runs.
func captureWarnLog(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	origStderr := os.Stderr
	os.Stderr = w

	log.StartStderr()

	fn()

	w.Close()

	os.Stderr = origStderr

	t.Cleanup(log.StopStderr)

	var buf bytes.Buffer

	_, err = buf.ReadFrom(r)
	require.NoError(t, err)

	r.Close()

	return buf.String()
}
