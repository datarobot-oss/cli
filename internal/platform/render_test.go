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

package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/datarobot/cli/internal/outputformat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func renderFixture(t *testing.T, dir string, format outputformat.OutputFormat) string {
	t.Helper()

	newFakeInstall(t, dir)

	report, err := Describe(context.Background())
	require.NoError(t, err)

	var out bytes.Buffer

	require.NoError(t, RenderTo(&out, format, *report))

	return out.String()
}

func TestRenderTo_JSONWrapsTheReportUnderPlatformBesideTheSchemaVersion(t *testing.T) {
	out := renderFixture(t, "sts-11.12.0", outputformat.OutputFormatJSON)

	var decoded map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &decoded))
	assert.EqualValues(t, SchemaVersion, decoded["schemaVersion"])
	assert.Contains(t, decoded["platform"], "sections")
	assert.NotContains(t, decoded, "sections", "the report sits under the platform key, not beside the version")
	require.NoError(t, validateJSON(t, compileSchema(t), []byte(out)))
}

func TestRenderTo_TextNamesTheInstallAndEverySection(t *testing.T) {
	out := renderFixture(t, "sts-11.12.0", outputformat.OutputFormatText)

	assert.Contains(t, out, "https://dr.example.com")
	assert.Contains(t, out, "11.12.0")

	for _, section := range []string{SectionInstall, SectionSeats, SectionEntitlements, SectionExecutionEnvironments, SectionResourceBundles} {
		assert.Contains(t, out, section)
	}
}

func TestRenderTo_TextSaysWhenNoSeatIsEnforced(t *testing.T) {
	out := renderFixture(t, "sts-11.12.0", outputformat.OutputFormatText)

	assert.Contains(t, out, "no seat licenses enforced")
}

func TestRenderTo_TextListsEnvironmentsForCustomApplicationsByID(t *testing.T) {
	out := renderFixture(t, "sts-11.12.0", outputformat.OutputFormatText)

	assert.Contains(t, out, "env-python ")
	assert.Contains(t, out, "env-python-unbuilt ")
	assert.NotContains(t, out, "env-model", "an environment without the customApplication use case is not listed")
}

func TestRenderTo_TextShowsWhyASectionIsUnavailable(t *testing.T) {
	report := sampleReport()

	var out bytes.Buffer

	require.NoError(t, RenderTo(&out, outputformat.OutputFormatText, report))

	assert.Contains(t, out.String(), "HTTP 403: forbidden")
	assert.Contains(t, out.String(), "ENABLE_X: HTTP 422")
}
