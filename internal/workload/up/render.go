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
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/tui"
)

// Summary names what a plan is about. It comes from the live workload when
// there is one and from the manifest when there is not, which is why the
// renderer is handed it rather than digging for it.
type Summary struct {
	Name       string
	WorkloadID string

	// Status is the platform's own word for what the workload is doing. The
	// header prints it rather than the State it reduces to, because the
	// reduction is lossy in the one place it matters: stopped, suspended and
	// interrupted are one state to the deploy and three quite different things
	// to a reader, and only one of them can actually be started.
	Status string

	// Refused marks a plan that is not going to be applied, because the live
	// workload is in a state no deploy can land on. The block still prints:
	// knowing what drift exists is useful even when nothing can be done about
	// it yet, and a reader who has just been refused is usually about to ask
	// exactly that. What changes is that it has to read as a description, where
	// every other plan this command prints is an announcement.
	Refused bool
}

// shortIDLen is how much of an id is enough to label something with. The
// platform's ids are 24 hex characters and nobody reads past the first few; the
// full id is in the JSON envelope for anything that needs to act on it.
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

	writeRefusedNote(&b, s)

	if plan.Empty() {
		b.WriteString("\n" + settledVerdict(plan) + "\n")

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

// settledVerdict is what an empty plan means, which is not always that there
// is nothing to do.
//
// A workload still moving is the exception, and it only reaches here on a dry
// run, which does not wait. The plan for it was computed against a state that
// is on its way somewhere else, so "already up to date" would be a claim the
// run has no way to make: a workload halfway through stopping matches its file
// until it finishes, at which point the deploy that follows starts it again.
// Saying nothing differs, without saying it will differ, is how a preview comes
// to read as the opposite of what the deploy will do.
func settledVerdict(plan Plan) string {
	if plan.State == StateSettling {
		return tui.WarnStyle.Render("Nothing differs yet, but this workload is still settling") + "\n" +
			tui.HintStyle.Render("  What a deploy does depends on where it lands, so it waits first.")
	}

	return tui.SuccessStyle.Render("✓ Already up to date")
}

// writeRefusedNote says that what follows is not going to happen.
//
// It goes above the changes rather than below them, because below is where the
// error already is and the whole complaint is that the reader met the work
// first and the refusal last.
//
// It says only that, in one line. The stateLine directly beneath it already
// names what is wrong with the workload and the error directly below the block
// already names the remedy, so a note that explained either would be the third
// telling on one screen.
func writeRefusedNote(b *strings.Builder, s Summary) {
	if !s.Refused {
		return
	}

	b.WriteString("\n  " + tui.HintStyle.Render(
		"Nothing below will be applied; it is what differs, not what is about to happen.") + "\n")
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

	// The state is the fallback rather than the answer, for a caller that has
	// no status to hand: every live read has one, and the plans built without a
	// live workload never reach this line.
	state := s.Status
	if state == "" {
		state = plan.State.String()
	}

	return fmt.Sprintf("%s (%s), %s", name, shortID(s.WorkloadID), state)
}

// shortID trims an id to something a person can take in at a glance.
//
// Only for an id the output is labelling, never for one the reader is being
// asked to recognise. Comparing a bound id against what is in the manifest is
// the check that catches a token resolving to a different instance or
// organisation, and eight characters is not enough to do it with.
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
		out = append(out, entry("+", "workload", createDetail(s, plan)))
	}

	// The plans whose reason to act is the state rather than a difference.
	// Without a line the body is empty and the block reads as though nothing
	// is about to happen, while the JSON beside it says otherwise.
	if line := stateLine(plan.State, s.Status); line != "" {
		out = append(out, line)
	}

	if plan.Code.Changed() {
		out = append(out, entry("~", "code", codeDetail(plan.Code)))
	}

	out = append(out, lockLines(plan)...)
	out = append(out, artifactLines(plan)...)
	out = append(out, runtimeLines(plan)...)

	return out
}

// stateLine is what the live state puts in a plan, empty when the state is not
// a reason to act by itself.
//
// The states that cannot be deployed onto get a line rather than being left to
// the error below the block. A plan that renders as a bare header reads as a
// run with nothing to do, which is the opposite of what a refusal is about to
// say, and --dry-run never prints the error at all.
//
// The status is needed as well as the state, for the one case where the
// reduction is lossy in a way that changes what the run will do. Stopped,
// suspended and interrupted are one state, and a start brings two of them back;
// the platform ignores a start of a suspended workload, so promising one here
// would preview an act the apply is about to refuse, and a dry run never gets
// as far as the refusal.
func stateLine(state State, status string) string {
	switch state {
	case StateStopped:
		if workload.IsSuspendedWorkloadStatus(status) {
			return entry("!", "workload", "suspended, which a deploy cannot undo")
		}

		return entry("~", "workload", "started, having been "+stoppedAs(status))

	case StateErrored:
		return entry("!", "workload", "errored, so there is nothing healthy to deploy onto")

	case StateTerminated:
		return entry("!", "workload", "terminated, and it still holds its name and artifact")

	case StateUnbound, StateMissing, StateSettling, StateRunning:
		return ""

	default:
		return ""
	}
}

// stoppedAs is how the workload came to be switched off, for the line that says
// it is about to be switched back on. Interrupted is not the same as stopped to
// the person reading it, and the state they reduce to cannot tell them apart.
func stoppedAs(status string) string {
	if status == "" {
		return "stopped"
	}

	return status
}

// createDetail names the workload about to be created. The plan is printed
// before anything happens, and --dry-run prints it instead of acting, so it
// says what will happen rather than what has.
//
// A create that replaces a binding reads differently from a first deploy and
// has to say so. The file expected to find something and did not, which is
// either a workload that was deleted, the ordinary case, or a run pointed at
// an instance that never had it. Naming the id is what lets the reader tell
// those apart, and it is the only warning there is: the plan is announced and
// then applied, never confirmed.
func createDetail(s Summary, plan Plan) string {
	if s.Name == "" {
		// Only reachable before a name exists to print; Validate requires one,
		// so both branches below name the workload.
		return "will be created, with its first artifact"
	}

	if plan.PriorWorkloadID == "" {
		return s.Name + " will be created" + fromArtifact(plan)
	}

	// The dead binding leads, because it is the warning: the plan is announced
	// and then applied, never confirmed, so the one line that distinguishes a
	// deleted workload from a run pointed at the wrong instance has to come
	// first. Which artifact the new workload comes up on follows it rather than
	// being dropped, since the flow this most often follows produces both facts
	// at once: a workload deleted outside the CLI leaves the binding stale and
	// the artifact link exactly where it was.
	// The id is printed whole: this is the line that separates a deleted
	// workload from a run pointed at the wrong instance, and comparing it
	// against what is in the file is the only way to know which happened.
	line := fmt.Sprintf("%s will be created: %s is bound to %s, which no longer exists",
		s.Name, manifest.FileName, plan.PriorWorkloadID)

	if !reusesLink(plan) {
		return line
	}

	return line + "; it comes up on the artifact this project is linked to"
}

// fromArtifact says which artifact a create is working from.
func fromArtifact(plan Plan) string {
	// A locked link is neither reused nor a first artifact: a new one is made
	// and the link moves onto it. The lock line below says exactly that, so
	// this says nothing rather than contradicting it two lines above.
	if plan.Code.LinkLocked {
		return ""
	}

	// Saying "its first artifact" of a project that already pushes to one is
	// wrong in the flow this most often follows: a delete clears the binding
	// and leaves the artifact link exactly where it was.
	if reusesLink(plan) {
		return ", on the artifact this project is linked to"
	}

	return ", with its first artifact"
}

// reusesLink reports whether the create will consult the project's artifact
// link at all. Only a build does: a manifest naming a published image posts its
// artifact inline and never reads the state directory, so a project can be
// linked and still be getting a brand new artifact.
// A locked link is not reused either: it cannot take code, so the run mints a
// replacement and moves the project onto that.
func reusesLink(plan Plan) bool {
	return plan.LinkedArtifact && plan.Code.Applies && !plan.Code.LinkLocked
}

// codeDetail says how much of the tree is going up.
func codeDetail(code CodeChange) string {
	if code.FirstDeploy {
		return "all project files, uploaded for the first time"
	}

	return fmt.Sprintf("%d %s changed since the last deploy", code.Files, plural(code.Files, "file", "files"))
}

// lockLines says what the deploy will do about a version that cannot be
// changed. Locking is the one act a deploy performs that the file never asks
// for and that cannot be undone, so it belongs in the plan rather than only in
// the running commentary: a run with --yes never sees a prompt, and --dry-run
// is the whole of the review it gets.
func lockLines(plan Plan) []string {
	// Only a run that actually mints a version can lock one. A change that
	// moves the sizing alone is applied to the workload in place, and a stopped
	// one is simply started: both leave the locked artifact exactly as it is,
	// so saying a version was created and locked would describe a deploy that
	// is not the one about to happen.
	if !plan.MintsVersion() {
		return nil
	}

	switch {
	case plan.Locked:
		return []string{entry("~", "lock",
			"the running version is locked, so a new one is created and permanently locked to match")}

	case plan.Code.LinkLocked:
		return []string{entry("~", "lock",
			"this project's artifact is locked, so a new one is created and the project pushes to that instead")}

	default:
		return nil
	}
}

// artifactLines describes the new immutable version, when there is one. Code
// and spec both produce one, and they are reported as a single entry because
// they are a single act: one version, built and rolled once.
//
// A version that keeps the running image says so: --dry-run is the whole of the
// review a deploy gets.
func artifactLines(plan Plan) []string {
	if !plan.RollsArtifact() {
		return nil
	}

	reason := "rebuilt from the synced code"
	if len(plan.Artifact) > 0 {
		reason = fmt.Sprintf("new version, %d spec %s",
			len(plan.Artifact), plural(len(plan.Artifact), "change", "changes"))
	}

	if plan.InheritsImage {
		reason += "; keeps the running image, so no rebuild"
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

	switch marker {
	case "+":
		style = addStyle
	case "!":
		style = blockStyle
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

	// A state the deploy cannot act on at all, which is neither an addition
	// nor a change but the reason there will be neither.
	blockStyle = tui.ErrorStyle
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
	Action  string `json:"action"`
	State   string `json:"state"`
	Creates bool   `json:"creates"`

	// Locked reports that this run will permanently lock the version it
	// deploys, because the one it replaces is locked and the platform will not
	// put a draft where a locked artifact was. It is the one consequence of a
	// deploy that cannot be undone and that nothing in the file asked for, so a
	// caller reading only this document has to be able to see it coming.
	Locked bool `json:"locked"`

	// PriorWorkloadID is the binding this create replaces, empty on a first
	// deploy. It is carried because the text plan's warning about a dead
	// binding is a line on stderr, which nothing machine-readable can gate on:
	// a CI job whose token resolves to the wrong organisation sees every
	// workload 404, and without this field its envelope is identical to a
	// legitimate first deploy.
	PriorWorkloadID string `json:"priorWorkloadId"`

	// KeepsImage reports that the version this run mints runs the image the
	// current one runs, so no build happens. Intent under --dry-run, and what
	// happened after a real run.
	KeepsImage bool `json:"keepsImage"`

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

	// LinkLocked reports that the artifact this project pushes into can no
	// longer take code, so the run will mint one that can and move the link
	// onto it. The live workload is not locked in that case, so the envelope's
	// own Locked field says nothing about it.
	LinkLocked bool `json:"linkLocked"`
}

// JSON projects the plan into its machine-readable shape.
func (p Plan) JSON() PlanJSON {
	// Both lock facts are gated on the same question the printed plan asks, so
	// the two renderings of one plan cannot come to disagree about whether this
	// run locks anything.
	mints := p.MintsVersion()

	return PlanJSON{
		Action:          p.Action(),
		State:           p.State.String(),
		Creates:         p.Creates,
		Locked:          p.Locked && mints,
		PriorWorkloadID: p.PriorWorkloadID,

		// Already gated on there being a version to mint.
		KeepsImage: p.InheritsImage,
		Code: CodeJSON{
			Applies:     p.Code.Applies,
			Changed:     p.Code.Changed(),
			Files:       p.Code.Files,
			FirstDeploy: p.Code.FirstDeploy,
			LinkLocked:  p.Code.LinkLocked && mints,
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
