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

import "github.com/datarobot/cli/internal/workload/manifest"

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

	// LinkLocked reports that the artifact this project pushes into can no
	// longer take code. The deploy answers by minting one that can and moving
	// the link onto it, which the plan says out loud because nothing in the
	// file asked for it.
	LinkLocked bool
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

	// Creates is a workload that does not exist yet, in which case Artifact
	// and Runtime are empty: everything in the file is new, and listing it
	// field by field would be noise rather than a plan.
	Creates bool

	Code     CodeChange
	Artifact []Change
	Runtime  []Change

	// Locked reports that the version now serving is immutable. Its successor
	// has to be locked too before the platform will take it, so a deploy onto
	// locked production locks something whether or not --lock was passed, and
	// that cannot be undone. The plan is where it belongs: --dry-run is how a
	// locked deploy is reviewed before it happens.
	Locked bool
}

// Empty reports that the live state already matches the file. `up` prints
// "Already up to date" and exits 0, having touched nothing.
//
// A stopped workload is never empty even when nothing differs: the user asked
// to deploy, and a workload that is not running has not been deployed.
func (p Plan) Empty() bool {
	return !p.Creates &&
		!p.Code.Changed() &&
		len(p.Artifact) == 0 &&
		len(p.Runtime) == 0 &&
		p.State != StateStopped
}

// RollsArtifact reports whether a new artifact version has to be minted and
// rolled in. Code and spec are one question here because they have one
// answer: both produce a new immutable version, and a run that has to rebuild
// also has to replace.
func (p Plan) RollsArtifact() bool {
	return !p.Creates && (p.Code.Changed() || len(p.Artifact) > 0)
}

// MintsVersion reports whether this run will produce a new artifact. Only a
// run that does can lock one, so it is what decides whether the plan says
// anything about locking: a change that moves the sizing alone is applied in
// place, and a stopped workload is simply started, both leaving the locked
// artifact exactly as it is.
func (p Plan) MintsVersion() bool {
	return p.Creates || p.RollsArtifact()
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
// and hands over the count.
func Build(loaded Loaded, live Live, code CodeChange) (Plan, error) {
	plan := Plan{
		State:   live.State,
		Creates: live.State == StateUnbound,
		Code:    code,
		Locked:  live.Locked,
	}

	// Nothing exists to compare against, so every field is trivially an
	// addition. Saying so once is a plan; saying it per field is a wall.
	if plan.Creates {
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

	// A file that names an artifact by id describes no spec to compare, so
	// the walk above has nothing to say about it. Pointing at a different
	// version than the one running is still the whole plan: it is a roll, and
	// without this the run reports "already up to date" while the file and
	// the workload disagree about what should be serving.
	if bound := loaded.Compiled.ArtifactID; bound != "" && bound != live.ArtifactID {
		plan.Artifact = append(plan.Artifact, Change{
			Path: keyArtifactID,
			Have: live.ArtifactID,
			Want: bound,
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
	if kind != "" && !manifest.SameArtifactType(kind, live.ArtifactType) {
		plan.Artifact = append(plan.Artifact, Change{
			Path: keyArtifactType,
			Have: live.ArtifactType,
			Want: kind,
		})
	}

	return plan, nil
}
