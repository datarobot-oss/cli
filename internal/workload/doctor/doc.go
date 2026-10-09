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

// Package doctor implements the wapi-specific checks for
// `dr artifact code doctor`: read-only diagnostics over the project's sync
// state in <projectDir>/.datarobot/workload (legacy: .wapi/).
//
// Every check is a pure diagnostic: it performs zero local writes and zero
// network calls. Repairs live behind `--fix`/`--relink` in the command layer.
// The generic Check/Result/Runner framework these checks plug into lives in
// internal/doctor (imported here as core to avoid clashing with this
// package's name).
package doctor
