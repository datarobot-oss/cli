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
	"github.com/datarobot/cli/internal/uidiff"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/internal/workload/sync/display"
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
const (
	// envVarsKey is the block's own key in a decoded manifest, the one the
	// subtree scrub looks for: a whole-element row's path names the element
	// around it, so catching secrets below it means reading the value, not
	// the path.
	envVarsKey = "environmentVars"

	// envVarsSegment is how envVarsKey reads inside a leaf's path.
	envVarsSegment = "." + envVarsKey + "["
)

// Render writes the plan block that `up` prints before it acts, and that
// --dry-run prints instead of acting.
func Render(w io.Writer, s Summary, plan Plan) error {
	var b strings.Builder

	if head := header(s, plan); head != "" {
		b.WriteString(planTitleStyle.Render(head))
		b.WriteString("\n")
	}

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

// RenderDiff writes the plan block as a unified diff, for `up --diff`.
//
// Where Render summarises, this lays the plan out leaf by leaf: a changed
// field states both sides of itself, `- old` then `+ new`; an agreeing field
// is context that collapses once it runs past the window; and what the live
// object carries that the file never names is counted once, because none of
// it is a removal. The default plan caps its detail list because a plan is a
// summary, and the file is a better place to read the rest; a diff is the
// detail, so nothing here is capped.
func RenderDiff(w io.Writer, s Summary, plan Plan) error {
	var b strings.Builder

	if head := diffHeader(s, plan); head != "" {
		b.WriteString(planTitleStyle.Render(head))
		b.WriteString("\n")
	}

	if plan.Empty() {
		// The three-state rule is the point of --diff, so an empty plan does
		// not skip the block the way Render's does: what the live object
		// carries that the file never names still gets said. The verdict is
		// the line the default mode prints, because the run's outcome, and
		// its exit code, are the same run's.
		b.WriteString("\n" + settledVerdict(plan) + "\n")

		if note := unmanagedNote(plan.Unmanaged); note != "" {
			b.WriteString(note)
			b.WriteString("\n")
		}

		_, err := io.WriteString(w, b.String())

		return err
	}

	b.WriteString("\n")

	actions, err := diffActionLines(s, plan)
	if err != nil {
		return err
	}

	for _, line := range actions {
		b.WriteString(line)
		b.WriteString("\n")
	}

	if err := uidiff.Render(&b, diffRows(plan), uidiff.Options{Redact: redacted}); err != nil {
		return err
	}

	if note := unmanagedNote(plan.Unmanaged); note != "" {
		b.WriteString(note)
		b.WriteString("\n")
	}

	_, err = io.WriteString(w, b.String())

	return err
}

// diffActionLines are the lines a diff keeps from the default plan: the
// reasons the run will act. None of them is a field change, so each keeps
// the entry form and position the default plan gives it -- rendered as a
// hunk, a stopped workload or a locked version would read as an edit to a
// field nobody wrote.
func diffActionLines(s Summary, plan Plan) ([]string, error) {
	var out []string

	if plan.Creates {
		out = append(out, entry("+", "workload", createDetail(s, plan)))
	}

	if line := stateLine(plan.State, s.Status); line != "" {
		out = append(out, line)
	}

	code, err := codeBlock(plan.Code)
	if err != nil {
		return nil, err
	}

	out = append(out, code...)
	out = append(out, lockLines(plan)...)
	out = append(out, artifactEntry(plan)...)

	return out, nil
}

// codeBlock is the diff's code section, and it exists only for code the
// deploy will push: an entry saying nothing moved, or a file list for a sync
// the run will not perform, would both invent work. The gate is Changed(),
// one predicate for the whole block, because Files counts exactly the plan's
// Uploads and Deletes -- the rows a deploy pushes. IsEmpty() would also count
// Downloads and Conflicts, which the deploy never pulls, and would then print
// a file list for a tree the run leaves alone. A first deploy has no sync
// plan to list, because nothing was ever uploaded to compare the tree
// against, so it keeps the default plan's wording. Otherwise the section is
// the file list the sync would upload, drawn by the sync command's own plan
// printer and fed the dry-run plan the code-change seam carried: two formats
// for one upload would eventually disagree, which is the whole reason the
// list is borrowed rather than redrawn. A plan measured without its list --
// no production path makes one, but a harness wiring only the count does --
// falls back to the count rather than printing nothing about real drift.
func codeBlock(code CodeChange) ([]string, error) {
	if !code.Changed() {
		return nil, nil
	}

	if code.FirstDeploy || code.SyncPlan == nil || len(code.SyncPlan.Uploads)+len(code.SyncPlan.Deletes) == 0 {
		return []string{entry("~", "code", codeDetail(code))}, nil
	}

	var b strings.Builder

	if err := display.PrintPlan(&b, code.SyncPlan); err != nil {
		return nil, fmt.Errorf("render the sync file list: %w", err)
	}

	return strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n"), nil
}

// diffHeader names the plan's subject. A live workload is named exactly as
// the default plan names it. A first deploy has no live side to name, and
// above a diff of the file itself the default plan's silence would leave the
// block unheaded, so the header states the subject instead. A create that
// replaces a dead binding is not a first deploy and still gets no header:
// the create line is the warning, as it is in the default mode.
func diffHeader(s Summary, plan Plan) string {
	if !plan.Creates {
		return header(s, plan)
	}

	if plan.PriorWorkloadID != "" {
		return ""
	}

	name := s.Name
	if name == "" {
		name = "workload"
	}

	return name + ", first deploy"
}

// diffRows flattens the plan's two row halves into the order the diff draws
// them, the artifact spec first and the runtime sizing second, one diff
// either way: they are one deploy and the reader reviews them together.
func diffRows(plan Plan) []uidiff.Row {
	rows := make([]uidiff.Row, 0, len(plan.DiffArtifact)+len(plan.DiffRuntime))

	rows = appendLeafRows(rows, plan.DiffArtifact)
	rows = appendLeafRows(rows, plan.DiffRuntime)

	return rows
}

// appendLeafRows turns whole-leaf rows into diff lines. A changed leaf
// becomes two lines, the value it replaces and the value it becomes, because
// that is what a unified diff states; an addition is one line, and an
// agreeing leaf is context. Redaction is left to the uidiff hook, which
// rewrites the whole line for a path it refuses, so a value formatted into
// the text here is never what prints.
func appendLeafRows(rows []uidiff.Row, leaves []DiffRow) []uidiff.Row {
	for _, leaf := range leaves {
		switch {
		case !leaf.Changed:
			rows = append(rows, uidiff.Row{
				Kind: uidiff.Context,
				Path: leaf.Path,
				Text: leafText(leaf.Path, leaf.Want),
			})
		case leaf.Absent:
			rows = append(rows, uidiff.Row{
				Kind: uidiff.Add,
				Path: leaf.Path,
				Text: leafText(leaf.Path, leaf.Want),
			})
		default:
			rows = append(rows,
				uidiff.Row{Kind: uidiff.Del, Path: leaf.Path, Text: leafText(leaf.Path, leaf.Have)},
				uidiff.Row{Kind: uidiff.Add, Path: leaf.Path, Text: leafText(leaf.Path, leaf.Want)},
			)
		}
	}

	return rows
}

// leafText renders one leaf the way the plan's detail lines spell a value,
// reusing format so a composite still reads as a summary rather than a dump.
func leafText(path string, v any) string {
	return path + ": " + format(v)
}

// unmanagedNote is the diff's whole account of the live object's unmanaged
// fields, one counted line rather than a marking per field. Each such field
// is live state the file declines to manage, so none of them is a removal,
// and interleaving them with the rows would put them next to edits they have
// nothing to do with.
func unmanagedNote(paths []string) string {
	if len(paths) == 0 {
		return ""
	}

	return tui.HintStyle.Render(fmt.Sprintf("%d %s not managed by this file",
		len(paths), plural(len(paths), "field", "fields")))
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
	line := fmt.Sprintf("%s will be created: %s is bound to %s, which no longer exists",
		s.Name, manifest.FileName, shortID(plan.PriorWorkloadID))

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
			"the running version is locked, so a new one is created and locked to match. Locking is permanent")}

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
	head := artifactEntry(plan)
	if head == nil {
		return nil
	}

	return append(head, details(plan.Artifact)...)
}

// artifactEntry is the artifact line without its detail list, which --diff
// replaces with the full rows underneath. It stays in diff mode because the
// one fact it carries, that a new version will be minted, is not visible in
// any field value.
func artifactEntry(plan Plan) []string {
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

	return []string{entry("+", "artifact", reason)}
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

	// Diff carries the structured form of what --diff renders, and is
	// present only when --diff was asked for: a caller that never passed the
	// flag reads the same document it always did, and one that did gets the
	// changes as data rather than scraping them out of the human block.
	Diff *DiffJSON `json:"diff,omitempty"`
}

// DiffJSON is the plan's diff section. Both of its lists are the plan's own
// accounting, carried so a machine-readable envelope says everything the
// human diff does about what moves and what is left alone.
type DiffJSON struct {
	// Changes is one entry per changing leaf of the manifest, in the order
	// the diff body prints them. The unchanged leaves the diff draws as
	// context are not here: a list of everything the file already agrees
	// with would bury the few entries a consumer acts on.
	Changes []ChangeJSON `json:"changes"`

	// Unmanaged holds the paths of what the live object carries that the
	// file never names, so a caller can see what this deploy leaves alone
	// without parsing the count out of the human summary.
	Unmanaged []string `json:"unmanaged"`
}

// ChangeJSON is one changing leaf, the structured twin of the `- old`/`+ new`
// pair the diff prints for it. Have is null and Absent true for a leaf the
// live object does not carry, which is an addition and not a swap.
type ChangeJSON struct {
	Path   string `json:"path"`
	Have   any    `json:"have"`
	Want   any    `json:"want"`
	Absent bool   `json:"absent"`

	// Redacted marks a path whose values were withheld, so an entry with
	// null sides reads as refused rather than empty. The omitempty keeps the
	// marker off the entries that carried their values in plain, which is
	// most of them.
	Redacted bool `json:"redacted,omitempty"`
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

// JSONWithDiff is JSON plus the diff section --diff renders. Both come from
// the one plan, so the envelope and the human block cannot disagree about
// what changes; the plain JSON() keeps the default path's exact shape, which
// is what a caller that never passed the flag is owed.
func (p Plan) JSONWithDiff() PlanJSON {
	out := p.JSON()
	out.Diff = p.diffJSON()

	return out
}

// diffJSON projects the diff rows into their machine-readable shape, the
// artifact side first with the synthetic id and type entries where Build
// appended them, then the runtime side: the order the diff body draws, so
// the two renderings of one plan list their changes as one sequence a
// consumer can zip.
func (p Plan) diffJSON() *DiffJSON {
	return &DiffJSON{
		Changes:   changesJSON(p.DiffArtifact, p.DiffRuntime),
		Unmanaged: unmanagedJSON(p.Unmanaged),
	}
}

// changesJSON walks the two row halves keeping only the changing leaves. The
// agreeing ones are context on the way to a reader and noise in a list, and
// the walk they come from already put the changing ones in the body's order.
func changesJSON(artifact, runtime []DiffRow) []ChangeJSON {
	leaves := make([]DiffRow, 0, len(artifact)+len(runtime))

	leaves = append(leaves, artifact...)
	leaves = append(leaves, runtime...)

	out := make([]ChangeJSON, 0, len(leaves))

	for _, leaf := range leaves {
		if !leaf.Changed {
			continue
		}

		out = append(out, changeJSON(leaf))
	}

	return out
}

// changeJSON serialises one leaf, redacting before it marshals. The values
// of an environment variable are withheld here exactly as the human diff
// withholds them, because a machine-readable plan is the one that ends up in
// CI artifacts; the entry still says which path moved, so the change itself
// is not silently lost to the refusal.
//
// Path-based redaction alone is not enough, twice over. A NEW name-keyed list
// element arrives as one whole-element row (a new container is one row for
// the container, not one per field), so the row's own path names the
// container and never trips redacted() while its value carries an
// environmentVars block complete with secrets. And when the live side
// carries no such list at all, there is nothing to match variables against
// by name, so the file's whole list arrives as one row whose path ENDS at
// the list -- a row that is itself the block the redaction refuses to print.
// Of the two ways to close either leak, dropping the whole entry would hide
// what the element carries from a consumer that has to review the plan, so
// the values are scrubbed instead: variable names survive, values do not,
// and the scrub copies rather than mutates because the same rows feed the
// human diff after this.
func changeJSON(leaf DiffRow) ChangeJSON {
	out := ChangeJSON{
		Path:   leaf.Path,
		Have:   leaf.Have,
		Want:   leaf.Want,
		Absent: leaf.Absent,
	}

	if redacted(leaf.Path) {
		out.Have = nil
		out.Want = nil
		out.Redacted = true

		return out
	}

	if endsAtEnvVars(leaf.Path) {
		// The row IS the environmentVars list, so the scrub is the list
		// scrub itself; routing the value through the generic walker would
		// copy every element verbatim, because the elements carry no
		// environmentVars key of their own to catch.
		out.Want = scrubEnvList(leaf.Want)
		out.Have = scrubEnvList(leaf.Have)
		out.Redacted = true
	} else if carriesEnvVars(leaf.Want) || carriesEnvVars(leaf.Have) {
		out.Want = scrubEnvVars(leaf.Want)
		out.Have = scrubEnvVars(leaf.Have)
		out.Redacted = true
	}

	return out
}

// endsAtEnvVars reports whether a leaf's path ends at the environmentVars
// list itself, the shape the walker emits when the live side has no list to
// walk element-by-element. redacted() misses it because that hook matches
// the per-variable spelling (".environmentVars["), while this row's path
// stops at the list, so the constructor names the shape itself and scrubs
// the values exactly as it scrubs them below a whole-element row.
func endsAtEnvVars(path string) bool {
	return strings.HasSuffix(path, "."+envVarsKey)
}

// carriesEnvVars reports whether a value holds an environmentVars block
// anywhere below it, which is what turns a whole-element row into a leak
// however innocent its own path reads.
func carriesEnvVars(v any) bool {
	switch typed := v.(type) {
	case map[string]any:
		if _, ok := typed[envVarsKey]; ok {
			return true
		}

		for _, child := range typed {
			if carriesEnvVars(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if carriesEnvVars(child) {
				return true
			}
		}
	}

	return false
}

// scrubEnvVars deep-copies a composite value with every environment variable
// reduced to its name. Everything else keeps its structure, so the entry
// still says what the element carries and a consumer can tell a container
// apart from its variables; only the refuse-to-print part is dropped.
func scrubEnvVars(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))

		for key, child := range typed {
			if key == envVarsKey {
				out[key] = scrubEnvList(child)

				continue
			}

			out[key] = scrubEnvVars(child)
		}

		return out
	case []any:
		out := make([]any, len(typed))

		for i, child := range typed {
			out[i] = scrubEnvVars(child)
		}

		return out
	default:
		return v
	}
}

// scrubEnvList copies one environmentVars block, keeping the names and
// nothing else. A name is the part a reader acts on; every other key of an
// element is a value or the address of one, whether a plaintext literal or a
// dr-credential ref, so none of it survives the copy. An element without a
// usable name has nothing safe to say and is dropped rather than passed
// through.
func scrubEnvList(v any) []any {
	list, ok := v.([]any)
	if !ok {
		return nil
	}

	out := make([]any, 0, len(list))

	for _, item := range list {
		element, ok := item.(map[string]any)
		if !ok {
			continue
		}

		name, ok := element["name"].(string)
		if !ok || name == "" {
			continue
		}

		out = append(out, map[string]any{"name": name})
	}

	return out
}

// unmanagedJSON copies the unmanaged paths, always into a non-nil slice so
// the JSON carries [] rather than null for "nothing unmanaged".
func unmanagedJSON(paths []string) []string {
	out := make([]string, 0, len(paths))

	return append(out, paths...)
}
