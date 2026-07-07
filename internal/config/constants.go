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

	// EnvPrefix is the canonical prefix for all DATAROBOT_CLI_* environment
	// variables. Use this constant instead of hard-coding the string literal.
	EnvPrefix = "DATAROBOT_CLI_"

	// UniversalAnnotationKey is the pflag annotation key used to mark a
	// persistent root flag for forwarding to plugin subprocesses as a
	// DATAROBOT_CLI_<suffix> env var. cmd/root.go writes it; internal/plugin reads it.
	UniversalAnnotationKey = "plugin-universal"
)
