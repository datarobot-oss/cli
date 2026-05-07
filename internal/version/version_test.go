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

package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersionDefaultValue(t *testing.T) {
	assert.Equal(t, "dev", Version, "Default Version should be 'dev'")
}

func TestAppNameVersionText(t *testing.T) {
	text := GetAppNameVersionText()

	assert.Contains(t, text, AppName, "Should contain AppName")
	assert.Contains(t, text, "version:", "Should contain 'version:' label")
	assert.Contains(t, text, Version, "Should contain the Version value")
}

func TestAppNameWithVersion(t *testing.T) {
	text := GetAppNameWithVersion()

	assert.Contains(t, text, AppName, "Should contain AppName")
	assert.Contains(t, text, "version", "Should contain 'version' label")
	assert.Contains(t, text, Version, "Should contain the Version value")
}

func TestAppNameFullVersionText(t *testing.T) {
	text := GetAppNameFullVersionText()

	assert.Contains(t, text, AppName, "Should contain AppName")
	assert.Contains(t, text, "version:", "Should contain 'version:' label")
	assert.Contains(t, text, "commit:", "Should contain 'commit:' label")
	assert.Contains(t, text, "built date:", "Should contain 'built date:' label")
	assert.Contains(t, text, "runtime:", "Should contain 'runtime:' label")
}

func TestInfoDataPopulated(t *testing.T) {
	assert.NotNil(t, Info, "Info should be populated")
	assert.Equal(t, Info.Version, Version, "Info.Version should match Version")
	assert.Equal(t, Info.Commit, GitCommit, "Info.Commit should match GitCommit")
	assert.Equal(t, Info.BuildDate, BuildDate, "Info.BuildDate should match BuildDate")
	assert.NotEmpty(t, Info.Runtime, "Info.Runtime should be populated")
}

func TestFullVersionPopulated(t *testing.T) {
	assert.NotEmpty(t, FullVersion, "FullVersion should be populated")
	assert.Contains(t, FullVersion, Version, "FullVersion should contain Version")
	assert.Contains(t, FullVersion, GitCommit, "FullVersion should contain GitCommit")
	assert.Contains(t, FullVersion, BuildDate, "FullVersion should contain BuildDate")
}
