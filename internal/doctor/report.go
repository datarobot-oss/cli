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

package doctor

// Report is the complete outcome of one doctor run: the run's context plus
// every check result in execution order. Reporters consume this struct; the
// owning command layer fills in ProjectDir and ArtifactID.
type Report struct {
	// ProjectDir is the resolved absolute path of the diagnosed project.
	ProjectDir string

	// ArtifactID is the linked artifact id, or nil when unlinked. Reporters
	// render nil as JSON null / "not linked"; an empty string is normalized
	// to unlinked (empty ≈ nil).
	ArtifactID *string

	// Checks holds the results in runner order.
	Checks []Result

	// Actions is nil for read-only runs (the JSON "actions" key is omitted).
	// For repair runs it points at the per-repair outcomes; it is a pointer so
	// a repair run with zero actions still renders "actions": [].
	Actions *[]Action
}

// NewReport builds a Report from a runner's results. The returned report is a
// read-only run (Actions nil); repair runs set Actions themselves.
func NewReport(projectDir string, artifactID *string, checks []Result) Report {
	return Report{
		ProjectDir: projectDir,
		ArtifactID: artifactID,
		Checks:     checks,
	}
}

// Counts tallies the report's checks by status.
func (r Report) Counts() Counts {
	return CountResults(r.Checks)
}

// OverallStatus derives the lowercase top-level verdict: "fail" if any check
// FAILed, else "warn" if any WARNed, else "ok" (SKIP-only counts as ok).
func (r Report) OverallStatus() string {
	return OverallStatus(r.Checks)
}

// ExitCode is 1 if any check FAILed, else 0 (OK/WARN/SKIP are allowed).
func (r Report) ExitCode() int {
	if r.Counts().FAIL > 0 {
		return 1
	}

	return 0
}

// linkedArtifact returns the artifact id for display, or "" when unlinked
// (empty ≈ nil normalization).
func (r Report) linkedArtifact() string {
	if r.ArtifactID == nil || *r.ArtifactID == "" {
		return ""
	}

	return *r.ArtifactID
}

// artifactIDForJSON returns the artifact id pointer to serialize: nil
// (→ JSON null) when unlinked or when the id is an empty string.
func (r Report) artifactIDForJSON() *string {
	if r.linkedArtifact() == "" {
		return nil
	}

	id := *r.ArtifactID

	return &id
}
