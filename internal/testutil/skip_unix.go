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

//go:build !windows

package testutil

import (
	"os"
	"testing"
)

// SkipIfRoot skips the test when it runs with effective UID 0. Root bypasses
// file and directory permission bits, so a test whose fault is injected via
// chmod (e.g. a read-only directory that must refuse a write or a remove) is
// inert as root: the fault never fires and the test fails on its own setup
// instead of on the behaviour under test. Container CI images commonly run
// as root, which is where this bites.
func SkipIfRoot(t *testing.T) {
	t.Helper()

	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission bits")
	}
}
