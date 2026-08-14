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

package up

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/tui"
)

// Summary names what a plan is about. It comes from the live workload when
// there is one and from the manifest when there is not, which is why the
// renderer is handed it rather than digging for it.
type Summary struct {
	Name       string
	WorkloadID string
}

// shortIDLen is how much of an id is enough to recognise it. The platform's
// ids are 24 hex characters and nobody reads past the first few; the full id
// is in the JSON envelope for anything that needs to act on it.
const shortIDLen = 8

// detailLimit caps how many individual changes a plan spells out. Past this
// the reader is not reading any more, and the file itself is the better
// place to look. Whatever is dropped is counted out loud rather than
// silently, so the plan never understates what it is about to do.
const detailLimit = 6

// envVarsSegment marks a path whose values must not be printed. A literal
// environment variable can be a secret someone pasted in plaintext, and a
// plan that echoed it would put it in terminal scrollback and CI logs. Names
// are enough to see what changed.
const envVarsSegment = ".environmentVars["

// Render writes the plan block that `up` prints before it acts, and that
// --dry-run prints instead of acting.
func Render(w io.Writer, s Summary, plan Plan) error {
	var b strings.Builder

	if head := header(s, plan); head != "" {
		b.WriteString(planTitleStyle.Render(head))
		b.WriteString("\n")
	}

	if plan.Empty() {
		b.WriteString("\n" + tui.SuccessStyle.Render("✓ Already up to date") + "\n")

		_, err := io.WriteString(w, b.String())

		return err
	}

	b.WriteString("\n")

	for _, line := range lines(s, plan) {
		b.WriteString(line)
		b.WriteString("\n")
	}

	_, err := io.WriteString(w, b.String())

	return err
}

// header names the workload being deployed onto and says what state it is in.
//
// There is nothing to head a plan with when no workload exists yet: naming a
// state the reader is about to leave, above a line saying what is about to be
// created, says the same thing twice and the second time is clearer. The
// create line carries the name instead.
func header(s Summary, plan Plan) string {
	if plan.Creates || s.WorkloadID == "" {
		return ""
	}

	name := s.Name
	if name == "" {
		name = "workload"
	}

	return fmt.Sprintf("%s (%s), %s", name, shortID(s.WorkloadID), plan.State)
}

// shortID trims an id to something a person can compare at a glance.
func shortID(id string) string {
	if len(id) <= shortIDLen {
		return id
	}

	return id[:shortIDLen]
}

// lines is the body of the plan, one entry per class of change plus the
// details each one carries.
func lines(s Summary, plan Plan) []string {
	var out []string

	if plan.Creates {
		out = append(out, entry("+", "workload", createDetail(s)))
	}

	// A stopped workload is the one plan whose reason to act is the state
	// rather than a difference. Without a line for it the body is empty and
	// the block reads as though nothing is about to happen, while the JSON
	// beside it says the action is "started".
	if plan.State == StateStopped {
		out = append(out, entry("~", "workload", "started, having been stopped"))
	}

	if plan.Code.Changed() {
		out = append(out, entry("~", "code", codeDetail(plan.Code)))
	}

	out = append(out, artifactLines(plan)...)
	out = append(out, runtimeLines(plan)...)

	return out
}

// createDetail names the workload about to be created. The plan is printed
// before anything happens, and --dry-run prints it instead of acting, so it
// says what will happen rather than what has.
func createDetail(s Summary) string {
	if s.Name == "" {
		return "will be created, with its first artifact"
	}

	return s.Name + " will be created, with its first artifact"
}

// codeDetail says how much of the tree is going up.
func codeDetail(code CodeChange) string {
	if code.FirstDeploy {
		return "all project files, uploaded for the first time"
	}

	return fmt.Sprintf("%d %s changed since the last deploy", code.Files, plural(code.Files, "file", "files"))
}

// artifactLines describes the new immutable version, when there is one. Code
// and spec both produce one, and they are reported as a single entry because
// they are a single act: one version, built and rolled once.
func artifactLines(plan Plan) []string {
	if !plan.RollsArtifact() {
		return nil
	}

	reason := "rebuilt from the synced code"
	if len(plan.Artifact) > 0 {
		reason = fmt.Sprintf("new version, %d spec %s",
			len(plan.Artifact), plural(len(plan.Artifact), "change", "changes"))
	}

	return append([]string{entry("+", "artifact", reason)}, details(plan.Artifact)...)
}

// runtimeLines describes a sizing change, which needs no new version. A
// single change reads better on the entry line itself than under it.
func runtimeLines(plan Plan) []string {
	if len(plan.Runtime) == 0 {
		return nil
	}

	if len(plan.Runtime) == 1 {
		return []string{entry("~", "runtime", describe(plan.Runtime[0]))}
	}

	head := entry("~", "runtime", fmt.Sprintf("%d %s",
		len(plan.Runtime), plural(len(plan.Runtime), "change", "changes")))

	return append([]string{head}, details(plan.Runtime)...)
}

// entry formats one plan line: a marker, the class, and what happens to it.
func entry(marker, class, detail string) string {
	style := changeStyle
	if marker == "+" {
		style = addStyle
	}

	return fmt.Sprintf("  %s %s %s",
		style.Render(marker),
		style.Render(fmt.Sprintf("%-10s", class)),
		tui.HintStyle.Render(detail))
}

// The plan block's palette: an addition reads as new, a change as something
// being moved, and the detail after either is commentary on it.
var (
	planTitleStyle = lipgloss.NewStyle().Bold(true)
	addStyle       = tui.SuccessStyle
	changeStyle    = tui.WarnStyle
)

// details lists individual changes under their entry, capped, saying out loud
// how many it left out.
func details(changes []Change) []string {
	shown := changes
	if len(shown) > detailLimit {
		shown = shown[:detailLimit]
	}

	out := make([]string, 0, len(shown)+1)
	for _, c := range shown {
		out = append(out, "      "+describe(c))
	}

	if dropped := len(changes) - len(shown); dropped > 0 {
		out = append(out, fmt.Sprintf("      and %d more", dropped))
	}

	return out
}

// describe renders one change, redacting the values of environment variables.
func describe(c Change) string {
	if redacted(c.Path) {
		if c.Absent {
			return c.Path + ": set"
		}

		return c.Path + ": changed"
	}

	return c.String()
}

// redacted reports whether a path sits inside an environmentVars list, whose
// values never reach the output.
func redacted(path string) bool {
	return strings.Contains(path, envVarsSegment)
}

// plural picks the right noun for a count.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}

	return many
}

// PlanJSON is the plan as --dry-run reports it in JSON mode. Change values
// are carried as rendered strings rather than raw values so the same
// redaction applies to both outputs: a machine-readable plan that leaked a
// secret would be worse than a human-readable one, since it ends up in CI
// artifacts.
type PlanJSON struct {
	Action   string   `json:"action"`
	State    string   `json:"state"`
	Creates  bool     `json:"creates"`
	Code     CodeJSON `json:"code"`
	Artifact []string `json:"artifact"`
	Runtime  []string `json:"runtime"`
}

// CodeJSON is the working tree's part of the answer.
type CodeJSON struct {
	// Applies is false for a manifest that names a published image, where
	// there is no code to sync and Files means nothing.
	Applies     bool `json:"applies"`
	Changed     bool `json:"changed"`
	Files       int  `json:"files"`
	FirstDeploy bool `json:"firstDeploy"`
}

// JSON projects the plan into its machine-readable shape.
func (p Plan) JSON() PlanJSON {
	return PlanJSON{
		Action:  p.Action(),
		State:   p.State.String(),
		Creates: p.Creates,
		Code: CodeJSON{
			Applies:     p.Code.Applies,
			Changed:     p.Code.Changed(),
			Files:       p.Code.Files,
			FirstDeploy: p.Code.FirstDeploy,
		},
		Artifact: describeAll(p.Artifact),
		Runtime:  describeAll(p.Runtime),
	}
}

// describeAll renders every change, always to a non-nil slice so the JSON
// carries [] rather than null for "nothing changed".
func describeAll(changes []Change) []string {
	out := make([]string, 0, len(changes))
	for _, c := range changes {
		out = append(out, describe(c))
	}

	return out
}
