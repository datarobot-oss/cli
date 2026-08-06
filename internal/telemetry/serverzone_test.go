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

package telemetry

import (
	"testing"

	"github.com/amplitude/analytics-go/amplitude/types"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/stretchr/testify/assert"
)

func TestInferServerZone(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    types.ServerZone
	}{
		{name: "US default", baseURL: "https://app.datarobot.com", want: types.ServerZoneUS},
		{name: "EU subdomain", baseURL: "https://app.eu.datarobot.com", want: types.ServerZoneEU},
		{name: "EU suffix", baseURL: "https://mytenant.eu", want: types.ServerZoneEU},
		{name: "EU suffix with explicit port", baseURL: "https://mytenant.eu:8443", want: types.ServerZoneEU},
		{name: "EU subdomain with explicit port", baseURL: "https://app.eu.datarobot.com:443", want: types.ServerZoneEU},
		{name: "EU segment with trailing dot", baseURL: "https://app.eu.datarobot.com/api/v2", want: types.ServerZoneEU},
		{name: "europe is not EU", baseURL: "https://host.europe.datarobot.com", want: types.ServerZoneUS},
		{name: "empty defaults to US", baseURL: "", want: types.ServerZoneUS},
		{name: "uppercase host matched case-insensitively", baseURL: "https://APP.EU.DATAROBOT.COM", want: types.ServerZoneEU},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, inferServerZone(tc.baseURL))
		})
	}
}

func TestResolveServerZone_EmptyUsesInferred(t *testing.T) {
	viperx.Reset()
	t.Cleanup(viperx.Reset)

	viperx.Set(config.DataRobotURL, "https://app.eu.datarobot.com")

	assert.Equal(t, types.ServerZoneEU, resolveServerZone())
}

func TestResolveServerZone_OverrideWinsOverInference(t *testing.T) {
	viperx.Reset()
	t.Cleanup(viperx.Reset)

	// EU endpoint, explicit US override → US.
	viperx.Set(config.DataRobotURL, "https://app.eu.datarobot.com")
	viperx.Set(config.TelemetryServerZone, "US")

	assert.Equal(t, types.ServerZoneUS, resolveServerZone())

	// US endpoint, explicit EU override → EU.
	viperx.Set(config.DataRobotURL, "https://app.datarobot.com")
	viperx.Set(config.TelemetryServerZone, "EU")

	assert.Equal(t, types.ServerZoneEU, resolveServerZone())
}

func TestResolveServerZone_CaseInsensitive(t *testing.T) {
	viperx.Reset()
	t.Cleanup(viperx.Reset)

	viperx.Set(config.DataRobotURL, "https://app.datarobot.com")

	for _, val := range []string{"eu", "Eu", "eU"} {
		viperx.Set(config.TelemetryServerZone, val)

		assert.Equal(t, types.ServerZoneEU, resolveServerZone(),
			"override %q should resolve to EU", val)
	}
}

func TestResolveServerZone_InvalidFallsBackToInferred(t *testing.T) {
	viperx.Reset()
	t.Cleanup(viperx.Reset)

	// EU endpoint with an unsupported value (APAC has no Amplitude DC) → warn
	// and fall back to the inferred EU zone, not a hard-coded US.
	viperx.Set(config.DataRobotURL, "https://app.eu.datarobot.com")
	viperx.Set(config.TelemetryServerZone, "APAC")

	assert.Equal(t, types.ServerZoneEU, resolveServerZone())
}

func TestResolveServerZone_WhitespaceOnlyTreatedAsUnset(t *testing.T) {
	viperx.Reset()
	t.Cleanup(viperx.Reset)

	viperx.Set(config.DataRobotURL, "https://app.datarobot.com")
	viperx.Set(config.TelemetryServerZone, "   ")

	assert.Equal(t, types.ServerZoneUS, resolveServerZone())
}
