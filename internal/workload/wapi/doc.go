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

// Package wapi manages the .datarobot/workload/ directory: the local,
// machine-managed sync state for a DataRobot workload project (conceptually
// .git/ for a workload). The package keeps the internal name; only the
// directory it manages is named for the product surface.
//
// It exposes plain functions that read and write the files inside it:
// config.json (identity + sync state), manifest.json (the BASE manifest from
// the last successful sync), .gitignore, and history.log (append-only JSONL
// with 1 MB rotation). It also drops a .drignore template at the project
// root on Initialize.
//
// This package is pure state management: no HTTP, no sync logic, no file
// locking. Consumers (the workload code CLI commands, the sync engine) layer
// those concerns on top.
//
// The one exception is the ignore file, which Initialize seeds. It reads no
// patterns; it asks the ignore package whether the project already has such a
// file and what to call a new one, so the name a project is given and the name
// a sync looks for cannot drift apart.
package wapi
