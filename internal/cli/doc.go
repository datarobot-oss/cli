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

// Package cli provides shared primitives used across the DataRobot CLI command
// layer. It sits between the cobra command definitions (cmd/) and the internal
// domain packages (internal/*), holding cross-cutting concerns that individual
// commands need but that do not belong in any single domain package.
//
// Current contents:
//
//   - CommandAdder: a cobra.Command wrapper that filters feature-gated
//     subcommands at registration time (command.go).
//   - YesFlagName: the canonical "--yes"/"-y" flag name constant (flags.go).
//   - IsNonInteractive: merges the three sources of automation signal —
//     DATAROBOT_CLI_NON_INTERACTIVE env var, --yes flag, and --force-interactive
//     override — into a single boolean (runtime.go).
//   - ErrSilent: a sentinel error for commands that have already printed their
//     own user-facing message and want cobra to stay quiet (command.go).
package cli
