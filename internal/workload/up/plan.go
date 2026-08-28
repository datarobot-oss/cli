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
	"slices"

	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/internal/workload/sync"
)

// Actions are what a run will do, and what the JSON envelope reports. A run
// does exactly one of them, so when several could apply the most significant
// wins: creating beats rolling, rolling beats retuning, retuning beats
// starting.
const (
	ActionCreated   = "created"
	ActionRolled    = "rolled"
	ActionUpdated   = "updated"
	ActionStarted   = "started"
	ActionUnchanged = "unchanged"
)

// CodeChange is the working tree measured against what the platform already
// holds. It only means anything when the manifest asks the platform to build
// the image; a container that names a published image has no code to sync,
// and reporting file counts for it would invite the reader to expect a
// rebuild that is never going to happen.
type CodeChange struct {
	// Applies is false when the manifest names an image rather than a build,
	// in which case the rest of this struct is meaningless.
	Applies bool

	// Files is how many differ. Zero with Applies true means the tree matches.
	Files int

	// FirstDeploy marks a project with nothing to compare against yet, so
	// every file is new rather than changed.
	FirstDeploy bool

	// IgnoreNotice is the sync engine's note about a deprecated ignore
	// filename, empty when there is nothing to say. Sizing the tree is the
	// only part of a run that a --dry-run does, so carrying it here is what
	// lets the preview mention it at all.
	IgnoreNotice string

	// ImageStale reports that the running image was built from code the
	// artifact has since moved past, which `dr artifact code sync` does.
	ImageStale bool

	// LinkLocked reports that the artifact this project pushes into can no
	// longer take code. The deploy answers by minting one that can and moving
	// the link onto it, which the plan says out loud because nothing in the
	// file asked for it.
	LinkLocked bool

	// SyncPlan is the dry-run plan the sync engine measured this tree with,
	// the same one Files counts. --diff renders it through the sync command's
	// own plan printer, so the two commands describe an upload with one
	// format instead of two. It is plain data rather than engine state -- the
	// lock is released right after measuring -- so carrying it past the
	// engine's Close costs nothing. Nil on a first deploy, for a manifest
	// that names an image, and when a test harness wires only the count.
	SyncPlan *sync.SyncPlan
}

// Changed reports whether the code needs syncing and rebuilding.
func (c CodeChange) Changed() bool {
	return c.Applies && (c.FirstDeploy || c.Files > 0)
}

// Plan is what a run intends to do, computed before it does any of it.
type Plan struct {
	// State is the live workload, so a plan can be printed for something that
	// cannot actually be deployed onto.
	State State

	// Creates is a workload the apply has to make rather than reconcile,
	// because nothing is bound or what was bound is gone. Artifact and Runtime
	// are empty in that case: everything in the file is new, and listing it
	// field by field would be noise rather than a plan.
	Creates bool

	// PriorWorkloadID is the binding this create replaces, set only when the
	// file named a workload the platform no longer has. A first deploy leaves
	// it empty. The plan prints it, because a run that creates something where
	// the file expected to find it is the one case where the user may be
	// pointed at the wrong instance rather than at a deleted workload.
	PriorWorkloadID string

	// LinkedArtifact means this project already pushes to an artifact. Whether
	// a create actually reuses it depends on the build mode: only a build
	// consults the link, while a manifest naming a published image posts its
	// artifact inline. Set by the caller rather than computed here, because
	// this package deliberately never touches the filesystem.
	LinkedArtifact bool

	Code     CodeChange
	Artifact []Change
	Runtime  []Change

	// InheritsImage reports that the new version can take the running image.
	// What the plan intends, not a promise; the envelope is corrected after.
	InheritsImage bool
	// DiffArtifact and DiffRuntime are the same two halves for the --diff
	// rendering: every leaf the file names, changed or not, so a diff can
	// draw context around its changes instead of listing only what moves.
	// They come from the walks that produce Artifact and Runtime, so the two
	// renderings of one plan cannot disagree about what differs. The changes
	// below that no walk can produce -- the artifact id and type -- are
	// merged into the rows as well, because a change the default plan prints
	// must never be invisible in the diff.
	DiffArtifact []DiffRow
	DiffRuntime  []DiffRow

	// Unmanaged lists the name-keyed elements the live object carries that
	// the file never names, from Extra. The diff renders them as a count
	// rather than as the removals they are not: the file leaving a field out
	// is the file declining to manage it, not asking for it to be deleted.
	Unmanaged []string

	// Locked reports that the version now serving is immutable. Its successor
	// has to be locked too before the platform will take it, so a deploy onto
	// locked production locks something whether or not --lock was passed, and
	// that cannot be undone. The plan is where it belongs: --dry-run is how a
	// locked deploy is reviewed before it happens.
	Locked bool
}

// Empty reports that the live state already matches the file and there is
// nothing for the run to do about it. `up` prints "Already up to date" and
// exits 0, having touched nothing.
//
// Matching the file is not sufficient, which is what actsOnState covers.
func (p Plan) Empty() bool {
	return !p.Creates &&
		!p.Code.Changed() &&
		len(p.Artifact) == 0 &&
		len(p.Runtime) == 0 &&
		!p.actsOnState()
}

// actsOnState reports the live states that give a run something to do even
// when the file and the workload agree about every field.
//
// A stopped workload is the plain case: the user asked to deploy, and one that
// is not running has not been deployed. The other two are about not lying. An
// errored or terminated workload matches its file in exactly the way a crashed
// process matches its source code, and the run that reaches them has to say so
// rather than print a green tick: `up` announces what it found before it acts,
// and "already up to date" over a header reading "errored" is the one summary
// that sends the reader away.
//
// That mattered less when a deploy refused a moving workload outright. It
// waits now, so a workload that dies while the deploy watches it is a case the
// command deliberately observes, and reporting success for it would be a
// success this run had a front-row seat to.
func (p Plan) actsOnState() bool {
	switch p.State {
	case StateStopped, StateErrored, StateTerminated:
		return true

	case StateUnbound, StateMissing, StateSettling, StateRunning:
		return false

	default:
		return false
	}
}

// creates reports the states whose apply is a create rather than a reconcile.
//
// Unbound is the first deploy. Missing is the same answer for a different
// reason: the file names a workload the platform does not have, and `up` is a
// reconciler, so a target that no longer exists is drift like any other and
// the file is what says what should be there. Refusing instead leaves the
// binding for the user to clear by hand, which is a dead end on exactly the
// recovery path where it is least welcome.
//
// Terminated is deliberately not among them, though it reads like it should
// be. A terminated workload still exists: the platform returns it, and it goes
// on holding its name and its artifact. Creating over it collides with the
// name it still owns, and every line the plan would print about it would be
// false, starting with the claim that it no longer exists.
//
// The risk this trades against is a 404 that means "not here" rather than
// "deleted": a manifest deployed against the wrong instance, organisation or
// token resolves to nothing and would be recreated somewhere unintended. That
// is why the plan names the id it could not find, and why Terraform, which
// treats a vanished resource the same way, is safe doing it while confirming
// at apply. `up` announces rather than confirms, so the announcement is what
// has to carry it.
func creates(state State) bool {
	switch state {
	case StateUnbound, StateMissing:
		return true

	case StateTerminated, StateStopped, StateSettling, StateRunning, StateErrored:
		return false

	default:
		return false
	}
}

// priorBinding is the id a create is about to replace. A first deploy has
// none, and only a binding that resolved to nothing has one worth printing:
// the plan says which id went missing so a run pointed at the wrong instance
// reads differently from one recovering a deleted workload.
func priorBinding(live Live) string {
	if live.State == StateMissing {
		return live.WorkloadID
	}

	return ""
}

// RollsArtifact reports whether a new artifact version has to be minted and
// rolled in. Code and spec are one question here because they have one
// answer: both produce a new immutable version, and a run that has to rebuild
// also has to replace.
func (p Plan) RollsArtifact() bool {
	return !p.Creates && (p.Code.Changed() || len(p.Artifact) > 0)
}

// RebuildsImage reports whether anything this run changes is an input to the
// image. runtimeFields are read when the container starts; everything else is
// a rebuild, where being wrong the other way promotes a version with no image.
func (p Plan) RebuildsImage() bool {
	if p.Code.Changed() || p.Code.ImageStale {
		return true
	}

	for _, change := range p.Artifact {
		if !runtimeOnly(change.Keys) {
			return true
		}
	}

	return false
}

// runtimeFields are the fields inside a container this release knows are read
// when it starts. An allow-list, so a field the platform grows later does not
// silently inherit a stale image.
var runtimeFields = []string{
	keyEnvironmentVars,
	keyLivenessProbe,
	keyPort,
	keyReadinessProbe,
	keyRoutes,
	keyStartupProbe,
}

// specRuntimeFields is runtimeFields for the fields directly under the spec,
// kept apart because the two sit at different depths.
var specRuntimeFields = []string{keyA2AEnabled}

// runtimeOnly reports whether a change names something a container reads at
// start, from the walk's key segments rather than the rendered path.
func runtimeOnly(keys []string) bool {
	// A field directly under the spec, not in any container.
	if len(keys) == 1 {
		return slices.Contains(specRuntimeFields, keys[0])
	}

	for i := 0; i+2 < len(keys); i++ {
		// The group's name sits between the two, which makes this a container
		// group's list rather than a "containers" elsewhere.
		if keys[i] != keyContainerGroups || keys[i+2] != keyContainers {
			continue
		}

		// keys[i+3] is the container's name, so the field follows it.
		if i+4 >= len(keys) {
			return false
		}

		return slices.Contains(runtimeFields, keys[i+4])
	}

	return false
}

// MintsVersion reports whether this run will produce a new artifact. Only a
// run that does can lock one, so it is what decides whether the plan says
// anything about locking: a change that moves the sizing alone is applied in
// place, and a stopped workload is simply started, both leaving the locked
// artifact exactly as it is.
func (p Plan) MintsVersion() bool {
	return p.Creates || p.RollsArtifact()
}

// OnlyStarts reports that starting the workload is the whole of what this run
// does. It is asked twice on the stopped path, by the branch that decides
// whether anything follows the start and by the start itself deciding whether
// it may lock, and the two must not come to disagree: locking is one-way, so a
// run that locked the version it was about to roll off could not take it back.
func (p Plan) OnlyStarts() bool {
	return p.Action() == ActionStarted
}

// Retunes reports that the sizing is the only thing this run changes: no
// version is minted and the workload is updated where it stands. It is the
// branch the apply takes, and it says nothing about whether the workload was
// running when the run started, because by the time that branch is reached a
// stopped one has been started.
func (p Plan) Retunes() bool {
	return !p.Creates && !p.RollsArtifact() && len(p.Runtime) > 0
}

// SendsNoEnvironment reports the one shape of deploy that resolves no
// credential references, which is why the check that costs a round trip each
// skips it: a resize sends the runtime block alone, so the artifact keeps every
// variable it already has and a reference this deploy never touches must not be
// able to refuse it.
//
// It is Retunes with one thing added rather than a second spelling of it. A
// stopped workload whose file moved only its sizing is still started by the
// run, and a start is when the container resolves its references from cold, so
// that one verifies.
func (p Plan) SendsNoEnvironment() bool {
	return p.Retunes() && p.State != StateStopped
}

// Action names the outcome, for the summary line and the JSON envelope.
func (p Plan) Action() string {
	switch {
	case p.Creates:
		return ActionCreated
	case p.RollsArtifact():
		return ActionRolled
	case len(p.Runtime) > 0:
		return ActionUpdated
	case p.State == StateStopped:
		return ActionStarted
	default:
		return ActionUnchanged
	}
}

// Build works out what differs between the file and the live workload.
//
// code is passed in rather than computed here so this package never acquires
// the project lock the sync engine holds between plan and execute, and so the
// tests never touch a filesystem. The caller runs the sync engine's own plan
// and hands over the count. opts arrives the same way, because whether the
// image can be inherited depends on --force-build as well as on the file.
func Build(loaded Loaded, live Live, code CodeChange, opts Options) (Plan, error) {
	plan := Plan{
		State:           live.State,
		Creates:         creates(live.State),
		PriorWorkloadID: priorBinding(live),
		Code:            code,
		Locked:          live.Locked,
	}

	// Nothing exists to compare against, so every field is trivially an
	// addition. Saying so once is a plan; saying it per field is a wall.
	if plan.Creates {
		artifactRows, runtimeRows, err := createRows(loaded)
		if err != nil {
			return Plan{}, err
		}

		// The diff has no live side to draw context from either, so it walks
		// the file against nil and shows what will be created, leaf by leaf.
		plan.DiffArtifact = artifactRows
		plan.DiffRuntime = runtimeRows

		return plan, nil
	}

	spec, err := loaded.Spec()
	if err != nil {
		return Plan{}, err
	}

	runtime, err := loaded.Runtime()
	if err != nil {
		return Plan{}, err
	}

	plan.Artifact = Subset(spec, live.Spec)
	plan.Runtime = Subset(runtime, live.Runtime)

	// The diff rows are the same two walks with the agreeing leaves kept, so
	// the changes above and the context around them come from one comparison
	// and cannot disagree about what differs.
	plan.DiffArtifact = DiffRows(spec, live.Spec)
	plan.DiffRuntime = DiffRows(runtime, live.Runtime)

	// A diff that stayed silent about what the live object carries that the
	// file never names would read as an exhaustive account of the workload,
	// so the paths land on the plan for the renderer to count. The two
	// documents can hold the same unmanaged element -- a sidecar exists in
	// the artifact's spec and in the workload's runtime -- and one element is
	// one field to a reader, so the list holds each path once.
	plan.Unmanaged = dedupePaths(Extra(spec, live.Spec), Extra(runtime, live.Runtime))

	// A file that names an artifact by id describes no spec to compare, so
	// the walk above has nothing to say about it. Pointing at a different
	// version than the one running is still the whole plan: it is a roll, and
	// without this the run reports "already up to date" while the file and
	// the workload disagree about what should be serving.
	if bound := loaded.Compiled.ArtifactID; bound != "" && bound != live.ArtifactID {
		plan.Artifact = append(plan.Artifact, Change{
			Path: keyArtifactID,
			Keys: []string{keyArtifactID},
			Have: live.ArtifactID,
			Want: bound,
		})

		plan.DiffArtifact = append(plan.DiffArtifact, DiffRow{
			Path:    keyArtifactID,
			Have:    live.ArtifactID,
			Want:    bound,
			Changed: true,
		})
	}

	kind, err := loaded.ArtifactType()
	if err != nil {
		return Plan{}, err
	}

	// The type is the same kind of blind spot for the same reason. It sits
	// beside the spec because the platform reads the discriminator off the
	// artifact and pops any type sent inside the spec, so the walk above
	// cannot see it either. Turning a service into an agent is a real change
	// and needs a new version; without this it reads as already up to date.
	//
	// Only when the file names one. Saying nothing about the type is not the
	// same as asking for a service, and this walk never reverts what the file
	// leaves out.
	//
	// The comparison folds case, which is not cosmetic: nothing in the file can
	// resolve a difference the platform does not consider one, so a spelling
	// the ledger accepts and the platform normalises would be drift that every
	// run mints a version for and never clears.
	// The live side is defaulted and the file's side is not, which is the same
	// asymmetry the roll's repository check makes for the same reason. An
	// unstated type in the file is the file having no opinion. An unstated type
	// on the artifact is not: the platform defaults it to a service, so reading
	// it as a difference would mint a version on every run of a file that
	// correctly says service, and nothing the file can say would ever settle it.
	//
	// Have carries the defaulted value rather than the raw one, so the line the
	// reader sees is the comparison that was actually made. Printing the empty
	// string would render "artifact.type:  -> agent", a change with nothing on
	// the left, about a side this code has just decided means service.
	if running := manifest.ArtifactTypeOrDefault(live.ArtifactType); kind != "" &&
		!manifest.SameArtifactType(kind, running) {
		plan.Artifact = append(plan.Artifact, Change{
			Path: keyArtifactType,
			Keys: []string{keyArtifactType},
			Have: running,
			Want: kind,
		})

		plan.DiffArtifact = append(plan.DiffArtifact, DiffRow{
			Path:    keyArtifactType,
			Have:    running,
			Want:    kind,
			Changed: true,
		})
	}

	// Last: every drift has to be in hand before RebuildsImage can answer.
	plan.InheritsImage = inheritsImage(live, plan, opts, kind, loaded.Compiled.ArtifactName)

	return plan, nil
}

// createRows walks the file against nothing, which is what a create compares
// against: the spec half and the runtime half both come back as additions,
// one row per leaf, so a diff can show what will be created rather than
// summarise it.
func createRows(loaded Loaded) ([]DiffRow, []DiffRow, error) {
	spec, err := loaded.Spec()
	if err != nil {
		return nil, nil, err
	}

	runtime, err := loaded.Runtime()
	if err != nil {
		return nil, nil, err
	}

	return DiffRows(spec, nil), DiffRows(runtime, nil), nil
}

// dedupePaths merges the unmanaged path lists from the two halves of the
// plan, keeping the first sighting of each path. The lists each arrive in a
// deterministic order, and the same element can be reported by both -- a
// sidecar the file never names exists in the artifact's spec and in the
// workload's runtime alike -- so counting it twice would promise a reader
// two fields where there is one name to go look at.
func dedupePaths(lists ...[]string) []string {
	var total int

	for _, list := range lists {
		total += len(list)
	}

	seen := make(map[string]struct{}, total)

	out := make([]string, 0, total)

	for _, list := range lists {
		for _, path := range list {
			if _, ok := seen[path]; ok {
				continue
			}

			seen[path] = struct{}{}

			out = append(out, path)
		}
	}

	return out
}
