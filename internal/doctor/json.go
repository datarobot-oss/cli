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

import (
	"encoding/json"
	"io"
)

// jsonCheck is the pinned per-check JSON shape: uppercase status, id, summary,
// optional remedy and details, and the fixable flag.
type jsonCheck struct {
	ID      string            `json:"id"`
	Status  Status            `json:"status"`
	Summary string            `json:"summary"`
	Remedy  string            `json:"remedy,omitempty"`
	Details map[string]string `json:"details,omitempty"`
	Fixable bool              `json:"fixable"`
}

// jsonReport is the pinned top-level JSON shape: absolute projectDir,
// artifactId (null when unlinked), lowercase status, checks in runner order,
// per-status summary counts, and — for repair runs only — an actions array.
type jsonReport struct {
	ProjectDir string      `json:"projectDir"`
	ArtifactID *string     `json:"artifactId"`
	Status     string      `json:"status"`
	Checks     []jsonCheck `json:"checks"`
	Summary    Counts      `json:"summary"`
	Actions    *[]Action   `json:"actions,omitempty"`
}

// WriteJSON renders a report as a single pure-JSON object (indented, with a
// trailing newline). HTML escaping is disabled so remedy strings like
// "--relink <new-artifact-id>" survive verbatim. For read-only runs the
// "actions" key is omitted entirely; repair runs always include it.
func WriteJSON(w io.Writer, report Report) error {
	checks := make([]jsonCheck, 0, len(report.Checks))

	for _, res := range report.Checks {
		checks = append(checks, jsonCheck{
			ID:      res.CheckID,
			Status:  res.Status,
			Summary: res.Summary,
			Remedy:  res.Remedy,
			Details: res.Details,
			Fixable: res.Fixable,
		})
	}

	out := jsonReport{
		ProjectDir: report.ProjectDir,
		ArtifactID: report.artifactIDForJSON(),
		Status:     report.OverallStatus(),
		Checks:     checks,
		Summary:    report.Counts(),
		Actions:    report.Actions,
	}

	enc := json.NewEncoder(w)

	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)

	return enc.Encode(out)
}
