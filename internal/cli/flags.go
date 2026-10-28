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

// flags.go defines shared flag-name constants used across command definitions.
// Centralising flag names here prevents magic strings from drifting out of sync
// between the registration site, viperx bindings, and shared helpers like
// IsNonInteractive.
package cli

// YesFlagName is the canonical name used for non-interactive confirmation flags.
// Commands that accept "--yes"/"-y" should derive their flag registration from
// this constant so shared helpers (telemetry, env bindings, etc.) can consume it.
const YesFlagName = "yes"
