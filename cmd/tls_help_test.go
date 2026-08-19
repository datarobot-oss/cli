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

package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestFprintNoWindowsCertsHelp checks the guidance is actionable rather than merely
// restating the failure: it must name how to inspect the store, how to import a CA,
// and the --ca-cert escape hatch.
func TestFprintNoWindowsCertsHelp(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	fprintNoWindowsCertsHelp(&buf)

	out := buf.String()

	for _, want := range []string{
		"no Root or CA certificates",
		`Cert:\LocalMachine\Root`,
		`Cert:\CurrentUser\Root`,
		"certlm.msc",
		"Import-Certificate",
		"dr --ca-cert",
	} {
		assert.Contains(t, out, want)
	}

	assert.True(t, strings.HasSuffix(out, "\n"), "guidance must end with a newline")
}
