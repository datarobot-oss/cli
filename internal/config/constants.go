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

package config

const (
	DataRobotURL    = "endpoint"
	DataRobotAPIKey = "token"

	APIConsumerTrackingEnabled = "api-consumer-tracking-enabled"

	// SkipAuthKey is the viper key behind the --skip-auth persistent flag.
	//
	// It must match the flag name exactly: viper lowercases keys but does not treat
	// "-" and "_" as equivalent for lookups, so reading "skip_auth" silently missed
	// the bound flag and only ever saw DATAROBOT_CLI_SKIP_AUTH. Use this constant at
	// every bind and read site so the two cannot drift apart again.
	SkipAuthKey = "skip-auth"

	// DefaultLLMID is the config key for the user's default LLM ID. The value is
	// either an LLM Gateway model id or a DataRobot deployment id.
	DefaultLLMID = "default-llm-id"

	// TelemetryServerZone is the config key for the explicit Amplitude server
	// zone override ("US" or "EU"). When unset, the telemetry package infers
	// the zone from the configured DataRobot endpoint. Settable via the
	// --telemetry-server-zone flag, the DATAROBOT_CLI_TELEMETRY_SERVER_ZONE
	// env var, or the telemetry-server-zone config-file key. It is a universal
	// flag (forwarded to plugin subprocesses) so plugins that emit their own
	// telemetry can honor the user's data-residency preference.
	TelemetryServerZone = "telemetry-server-zone"

	// EnvPrefix is the canonical prefix for all DATAROBOT_CLI_* environment
	// variables. Use this constant instead of hard-coding the string literal.
	EnvPrefix = "DATAROBOT_CLI_"

	// UniversalAnnotationKey is the pflag annotation key used to mark a
	// persistent root flag for forwarding to plugin subprocesses as a
	// DATAROBOT_CLI_<suffix> env var. cmd/root.go writes it; internal/plugin reads it.
	UniversalAnnotationKey = "plugin-universal"
)
