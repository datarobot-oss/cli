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
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// schemaPath is the contract consumers read. The tests below hold the Go types
// to it, so the two cannot drift apart.
var schemaPath = filepath.Join("..", "..", "docs", "schemas", "platform-describe.schema.json")

func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()

	schema, err := jsonschema.NewCompiler().Compile(schemaPath)
	require.NoError(t, err)

	return schema
}

// validateJSON checks a raw JSON document against the schema.
func validateJSON(t *testing.T, schema *jsonschema.Schema, raw []byte) error {
	t.Helper()

	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	require.NoError(t, err)

	return schema.Validate(instance)
}

func sampleReport() Report {
	enterprise := true

	return Report{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   "2026-10-06T20:00:00Z",
		Producer:      Producer{Name: "dr", Version: "1.2.3"},
		Server:        Server{Release: "11.12.0", APIVersion: "2.48", CanonicalURL: "https://dr.example.com"},
		Sections: map[string]Section{
			SectionInstall:      {Status: StatusOK, Data: Install{IsEnterprise: &enterprise, DefaultAppResourceBundle: "cpu.xlarge"}},
			SectionSeats:        {Status: StatusOK, Data: Seats{SeatLicenses: map[string]bool{}}},
			SectionEntitlements: {Status: StatusDegraded, Message: "ENABLE_X: HTTP 422", Data: map[string]bool{"ENABLE_Y": true}},
			SectionExecutionEnvironments: {Status: StatusOK, Data: ExecutionEnvironments{Items: []ExecutionEnvironment{
				{ID: "env-1", Name: "Python", UseCases: []string{"customApplication"}, HasSuccessfulVersion: true, BuildStatus: "success"},
			}}},
			SectionResourceBundles: {Status: StatusUnavailable, Message: "HTTP 403: forbidden"},
		},
	}
}

func marshal(t *testing.T, v any) []byte {
	t.Helper()

	raw, err := json.Marshal(v)
	require.NoError(t, err)

	return raw
}

func TestReport_MarshalsToTheSchema(t *testing.T) {
	require.NoError(t, validateJSON(t, compileSchema(t), marshal(t, sampleReport())))
}

func TestSchema_RejectsAnUnknownStatus(t *testing.T) {
	report := sampleReport()
	report.Sections[SectionSeats] = Section{Status: "weird", Data: Seats{SeatLicenses: map[string]bool{}}}

	assert.Error(t, validateJSON(t, compileSchema(t), marshal(t, report)))
}

func TestSchema_RejectsAMissingSchemaVersion(t *testing.T) {
	assert.Error(t, validateJSON(t, compileSchema(t), []byte(`{"server":{},"sections":{}}`)))
}

func TestSchema_RejectsAnUnavailableSectionWithoutAMessage(t *testing.T) {
	report := sampleReport()
	report.Sections[SectionSeats] = Section{Status: StatusUnavailable}

	assert.Error(t, validateJSON(t, compileSchema(t), marshal(t, report)))
}

// Readers ignore what they do not know, so a newer producer's additions must
// stay valid under an older copy of the schema.
func TestSchema_AcceptsUnknownFieldsAndSections(t *testing.T) {
	raw := []byte(`{
		"schemaVersion": 1,
		"server": {"release": "11.12.0", "somethingNew": true},
		"sections": {"seats": {"status": "ok", "data": {"seatLicenses": {}}}, "workloads": {"status": "ok"}}
	}`)

	require.NoError(t, validateJSON(t, compileSchema(t), raw))
}
