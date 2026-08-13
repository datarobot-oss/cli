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

package manifest

// Manifest-level and credential field names, defined once so a schema rename
// is a one-line change. The validation ledger declares the deeper spec field
// names it walks in its own file.
const (
	keyArtifact        = "artifact"
	keyArtifactID      = "artifactId"
	keyDRCredentialID  = "drCredentialId"
	keyEnvironmentVars = "environmentVars"
	keyImportance      = "importance"
	keyKey             = "key"
	keyName            = "name"
	keyRuntime         = "runtime"
	keySource          = "source"
	keyValue           = "value"
	keyWorkloadID      = "workloadId"
)

// Spec field names no validation rule reads, which is why the ledger does not
// declare them. Both halves of the package still write them: the renderer
// builds them out of a draft, and binding edits them in place on a live spec.
// That is two files per rename, which is the situation this one exists to
// prevent, so they are named here rather than inline in either.
const (
	keyA2AEnabled     = "a2aEnabled"
	keyCPU            = "cpu"
	keyLivenessProbe  = "livenessProbe"
	keyPath           = "path"
	keyReadinessProbe = "readinessProbe"
	keyStartupProbe   = "startupProbe"
	keyType           = "type"
)
