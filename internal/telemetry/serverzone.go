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
	"net/url"
	"strings"

	"github.com/amplitude/analytics-go/amplitude/types"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
)

// euHostPatterns lists the host substrings that identify a DataRobot instance
// in the EU data-residency zone. The Amplitude SDK has no APAC data center, so
// only US and EU are meaningful here. To extend inference (e.g. a new EU host
// pattern), append to this slice — no other changes are required.
//
// A host is treated as EU when, after lowercasing, it either contains ".eu."
// or ends with ".eu". The trailing-dot variant avoids false positives on hosts
// such as "host.europe.datarobot.com".
var euHostPatterns = []string{".eu."}

// inferServerZone derives the Amplitude ServerZone from a DataRobot base URL.
// EU-matching hosts return ServerZoneEU; everything else (including the empty
// string) defaults to ServerZoneUS.
func inferServerZone(baseURL string) types.ServerZone {
	host := strings.ToLower(baseURL)

	// Match against the hostname (port-stripped) rather than the full URL so
	// that an EU endpoint with an explicit port (e.g. "https://mytenant.eu:8443")
	// still satisfies the ".eu" suffix rule. Fall back to the lowercased full
	// string if parsing fails or no host is present.
	if u, err := url.Parse(baseURL); err == nil && u.Hostname() != "" {
		host = strings.ToLower(u.Hostname())
	}

	if strings.HasSuffix(host, ".eu") {
		return types.ServerZoneEU
	}

	for _, pattern := range euHostPatterns {
		if strings.Contains(host, pattern) {
			return types.ServerZoneEU
		}
	}

	return types.ServerZoneUS
}

// resolveServerZone determines the Amplitude ServerZone to use at client
// initialization. An explicit override (flag, env var, or config file) takes
// precedence over the value inferred from the configured DataRobot endpoint.
//
// Precedence follows viper's own ordering (flag > env > config). An empty or
// whitespace-only override is treated as "unset" and falls back to inference.
// An invalid value (not "US" or "EU", case-insensitive) logs a warning to the
// debug log and falls back to the inferred zone rather than failing CLI
// execution — telemetry initialization must never block or error visibly.
func resolveServerZone() types.ServerZone {
	inferred := inferServerZone(config.GetBaseURL())

	raw := strings.TrimSpace(viperx.GetString(config.TelemetryServerZone))
	if raw == "" {
		return inferred
	}

	switch strings.ToUpper(raw) {
	case "EU":
		return types.ServerZoneEU

	case "US":
		return types.ServerZoneUS

	default:
		log.Warnf(
			"invalid value %q for telemetry-server-zone (expected US or EU); falling back to inferred zone %s",
			raw, inferred,
		)

		return inferred
	}
}
