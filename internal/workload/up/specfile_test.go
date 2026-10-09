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
	"testing"

	"github.com/datarobot/cli/internal/workload/wizard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A spec file is for a first deploy. With a manifest in place it is refused
// rather than ignored, since the manifest is the source from then on.
func TestLoad_SpecFileIsRefusedOverAnExistingManifest(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, unboundImageManifest)

	_, err := load(dir, Options{NonInteractive: true, SpecFile: "spec.yaml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--spec-file is for a first deploy")
}

// With no manifest the file reaches the setup, which is where it is read.
func TestLoad_SpecFileReachesTheSetup(t *testing.T) {
	var seen string

	force(t, &runWizardFn, func(opts wizard.Options) (wizard.Result, error) {
		seen = opts.SpecFile

		return wizard.Result{}, assert.AnError
	})

	_, _ = load(t.TempDir(), Options{NonInteractive: true, SpecFile: "spec.yaml"})
	assert.Equal(t, "spec.yaml", seen)
}
