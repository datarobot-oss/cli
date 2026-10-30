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

package up

import (
	"bytes"
	"testing"

	"github.com/datarobot/cli/internal/usecase"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	fileUC  = "68b0aa11bb22cc33dd44ee55"
	boundWL = "6ae326a88ad1d49b09eaae94"
)

// A workload on an Enclave with no Use Case cannot be restarted or replaced
// (MISSING_USE_CASE), and the Workload API takes useCaseId only on create, so
// up links an existing workload itself.
func TestLinkUseCase(t *testing.T) {
	running := Live{Live: manifest.Live{WorkloadID: boundWL}, State: StateRunning}
	missing := Live{Live: manifest.Live{WorkloadID: boundWL}, State: StateMissing}

	for name, c := range map[string]struct {
		file     string
		live     Live
		linked   *workload.UseCase
		wantLink bool
		wantNote string
	}{
		"unlinked is linked":       {file: fileUC, live: running, wantLink: true, wantNote: "Linked workload"},
		"already linked":           {file: fileUC, live: running, linked: &workload.UseCase{ID: fileUC}},
		"linked elsewhere is kept": {file: fileUC, live: running, linked: &workload.UseCase{ID: "68b0ffffffffffffffffffff"}, wantNote: "stays linked"},
		"file names none":          {live: running},
		"create sends it instead":  {file: fileUC, live: missing},
	} {
		t.Run(name, func(t *testing.T) {
			content := "name: my-app\nartifactId: 68a1\n"
			if c.file != "" {
				content += "useCaseId: " + c.file + "\n"
			}

			parsed, err := manifest.Parse([]byte(content), "")
			require.NoError(t, err)

			var links []string

			force(t, &workloadUseCaseFn, func(string) (*workload.UseCase, error) { return c.linked, nil })
			force(t, &linkUseCaseFn, func(id usecase.ID, wl string) error {
				links = append(links, string(id)+"/"+wl)

				return nil
			})

			var stderr bytes.Buffer

			require.NoError(t, linkUseCase(Loaded{Manifest: parsed}, c.live, Options{Stderr: &stderr}))

			if c.wantLink {
				assert.Equal(t, []string{fileUC + "/" + boundWL}, links)
			} else {
				assert.Empty(t, links)
			}

			if c.wantNote == "" {
				assert.Empty(t, stderr.String())
			} else {
				assert.Contains(t, stderr.String(), c.wantNote)
			}
		})
	}
}
