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

// Package manifest owns the committed .datarobot.yaml workload manifest: the
// repository-root file that is the platform's workload-create spec verbatim,
// plus the two CLI-managed conveniences layered on top. workloadId binds the
// file to a workload and is stripped before every API call; the
// dr-credential:<credential-id>/<key> value shorthand compiles to the API's
// credential-backed environment variable object (the object form is accepted
// in the file as well).
//
// There are no typed spec structs here on purpose: the workload-api schema
// moves faster than any struct would, so unknown blocks pass through to the
// API untouched and platform additions need no CLI release. The package
// parses the file once into a yaml.Node tree and derives everything from it:
// Compile lowers the tree to the JSON create payload. A stacked follow-up
// adds Validate, walking the same tree so every finding carries its manifest
// line, and WriteWorkloadID, the package's only write: a single-key edit
// that keeps the user's comments, unknown keys and their order intact.
// config never rewrites an existing manifest; after the first write the
// file is the interface.
//
// Non-scope: no HTTP. Compile returns CredentialRefs so callers can verify
// each referenced credential against the store (one GET per credential)
// before anything mutates; the lookup itself lives with the workload API
// clients. No wizard UI and no sync state either: the similarly named
// wapi.Manifest is the .datarobot/workload/manifest.json sync index, a
// different file entirely.
package manifest
