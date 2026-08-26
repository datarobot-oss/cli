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
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/datarobot/cli/tui"
)

// buildAndCreate is the first deploy of a project whose image the platform
// builds. It is four acts where a published image is one: the artifact has to
// exist before there is anywhere to put code, the code has to be there before
// there is anything to build, and the image has to exist before a workload
// can run it.
//
// Each act is separately resumable, which is the point of doing them in this
// order rather than creating the workload first. A run that dies after the
// sync leaves an artifact holding the code; the next one picks it up instead
// of starting again. A workload created up front would instead sit there
// pointing at an image that does not exist yet, visible in the UI and unable
// to start, for as long as the build took.
func buildAndCreate(loaded Loaded, code CodeChange, result Result, opts Options, report *reporter) (Result, error) {
	made, err := ensureArtifact(loaded, report)

	// Recorded before the error check, the same way roll records the build it
	// ran: an artifact created and then stranded by a link that would not write
	// is the one failure the envelope has to be able to name.
	result.ArtifactID = made.ID

	if err != nil {
		return result, err
	}

	artifactID, fresh := made.ID, made.Fresh

	synced, err := syncCode(loaded.ProjectDir, report)
	if err != nil {
		return result, err
	}

	// Asked unconditionally, exactly as the roll path asks it. A sync that
	// minted a version answers immediately; the cases that reach further are a
	// version standing in for a locked one, whose tree was already synced and
	// so may upload nothing, and a run resuming after an earlier one relinked
	// and died. Neither is distinguishable from the ordinary case by then: the
	// link has already moved, so the only record of what happened is the
	// catalog version in the project's own state, which is what this reads.
	if err := inheritCode(loaded.ProjectDir, artifactID, synced, report); err != nil {
		return result, err
	}

	buildID, err := maybeBuild(artifactID, fresh, code, synced, opts, report)

	// Recorded before the error check: a failed build is still a build, and
	// the caller's envelope should be able to name the one to go and read.
	result.BuildID = buildID

	if err != nil {
		return result, err
	}

	payload, err := loaded.Compiled.BindArtifact(artifactID)
	if err != nil {
		return result, err
	}

	result, err = create(loaded, payload, result, opts, report)
	if err != nil && !fresh {
		// The artifact was not minted by this run, so a refusal to create a
		// workload on it is most likely the platform's one-per-draft-artifact
		// rule, and the link that chose it is the thing to explain.
		err = reusedArtifactConflict(err, artifactID, loaded.ProjectDir)
	}

	return result, err
}

// reusedArtifactConflict explains a create refused because the artifact this
// checkout is linked to is already spoken for.
//
// The link is invisible: it lives in a gitignored directory nobody looks at,
// and it is what makes a second deploy update one artifact instead of piling
// up new ones. When it goes stale, the platform's own message names an
// artifact id and nothing about where that id came from, which leaves the
// reader with a conflict and no idea what chose the thing that conflicted.
func reusedArtifactConflict(createErr error, artifactID, projectDir string) error {
	if !isConflict(createErr) {
		return createErr
	}

	// Only rewrite once the claim is verified. A 409 on this path is just as
	// likely to be a duplicate workload name, which create() has already
	// explained accurately; replacing that with advice to delete the link
	// directory would send the user to orphan an artifact for no reason.
	owner := workloadOn(artifactID)
	if owner == "" {
		return createErr
	}

	return fmt.Errorf(
		"this project is linked to artifact %s, which workload %s already has, and the platform allows "+
			"only one workload per draft artifact.\n"+
			"  The link lives in %s and is what makes repeated deploys update one artifact rather than "+
			"making a new one each time.\n"+
			"  To deploy to that workload, set 'workloadId: %s' in %s, replacing any already there.\n"+
			"  To deploy a separate workload from this directory, delete %s: the next run then creates "+
			"an artifact of its own.\n"+
			"  %w",
		artifactID, owner, wapi.Dir(projectDir), owner, manifest.FileName, wapi.Dir(projectDir), createErr)
}

// workloadOn names the workload already using artifactID, "" when the lookup
// cannot say. It turns the advice from "find out what owns this" into a line
// the reader can paste.
func workloadOn(artifactID string) string {
	existing, err := listWorkloadsFn(conflictSearchLimit, nil, "")
	if err != nil {
		return ""
	}

	for i := range existing {
		if existing[i].ArtifactID == artifactID {
			return existing[i].ID
		}
	}

	return ""
}

// ensureArtifact returns the artifact this project builds into, creating one
// and linking the directory to it the first time. Fresh reports that this run
// made it, because an artifact that was just created has no image and
// therefore always needs a build.
//
// The link lives in the project's own state directory rather than in the
// manifest. The manifest describes what to deploy and is committed; which
// artifact a particular checkout has been pushing to is local, and writing it
// into a shared file would have two developers fighting over one draft.
//
// A linked artifact is reused only while it can still take this deploy and
// still says what the file says. Two things end that, and they have one
// answer. Locking is one-way, so a locked link can take neither new code nor a
// new image. A spec that has diverged is the other: the link records which
// artifact this checkout pushes to, not what that artifact contains, and an
// artifact's spec is fixed when it is created, so reusing a diverged one
// deploys the frozen answer and reports success. The reconcile path guards
// that already, in describes(); this path reaches the same artifact by another
// route and needs the same guard, or editing a port in the file and
// redeploying changes nothing and says nothing.
//
// Either way the answer is the one the roll path already gives: mint a new
// version, in the lineage the old one belongs to, and move the link onto it.
func ensureArtifact(loaded Loaded, report *reporter) (version, error) {
	if !projectLinkedFn(loaded.ProjectDir) {
		return freshArtifact(loaded, "", labelFirstArtifact, report)
	}

	cfg, err := loadProjectFn(loaded.ProjectDir)
	if err != nil {
		return version{}, fmt.Errorf("cannot read which artifact %s is linked to: %w", loaded.ProjectDir, err)
	}

	// One read answers every question asked of the linked artifact below:
	// whether it is locked, which lineage a replacement would join, and whether
	// its spec still says what the file says. Reading it once per question had
	// every deploy of a linked project pay several sequential round trips to a
	// single resource on the way to one decision.
	linked, err := getArtifactDocFn(cfg.ArtifactID)
	if err != nil {
		return version{}, fmt.Errorf(
			"cannot read artifact %s, which this project is linked to: %w", cfg.ArtifactID, err)
	}

	// Nothing came back and nothing failed, which says neither that the
	// artifact can take this deploy nor that it cannot. Reusing it on that
	// basis rolls out whatever it happens to contain; replacing it discards the
	// lineage over a fault nobody established. Stopping is the only answer that
	// does not act on an unread state.
	if linked == nil {
		return version{}, fmt.Errorf(
			"the platform returned nothing for artifact %s, which this project is linked to, so there is no "+
				"way to tell whether it can still take this deploy", cfg.ArtifactID)
	}

	if workload.IsLockedStatus(linked.String(keyStatus)) {
		// Phrased as what the run needs rather than what it did, because the
		// create below can still fail and a line already claiming the version
		// exists would be the last thing printed before the error saying it
		// does not.
		//
		// The alternative was telling the reader to delete the state
		// directory, which works but throws away the catalog and the
		// last-synced version with it, so the following sync re-uploads a tree
		// the platform already holds.
		report.say("  Artifact %s is locked, so this deploy needs a new version.\n", cfg.ArtifactID)

		return freshArtifact(loaded, redraftRepository(loaded, linked), labelNewVersion, report)
	}

	// Cannot tell reads as yes. Replacing on a comparison that never completed
	// would discard the artifact this project pushes to over a fault that was
	// never established, and the read that could have failed already has.
	if matches, err := specMatches(loaded, linked); err != nil || matches {
		return version{ID: cfg.ArtifactID}, nil
	}

	report.say("  Artifact %s no longer matches %s; making a new one.\n", cfg.ArtifactID, manifest.FileName)

	return freshArtifact(loaded, redraftRepository(loaded, linked), labelNewVersion, report)
}

// redraftRepository is the lineage the replacement joins, "" when it has to
// start one of its own. It is the same question sameRepository answers for a
// roll, asked of the linked artifact rather than the live one because on this
// path there is no live workload to read a type off.
//
// A file that changed the artifact's kind is the case that has to start fresh:
// a repository carries the type of the artifact that opened it, and the
// platform answers 422 for one that disagrees. createInRepository would recover
// from that by retrying without the id, so the cost of not asking is a wasted
// round trip and a line blaming the repository for something the file did.
//
// The type and the repository are read off the document the caller already
// has, rather than a typed artifact fetched again for two fields.
func redraftRepository(loaded Loaded, linked workload.Document) string {
	wanted, err := loaded.ArtifactType()
	if err != nil {
		return ""
	}

	if !manifest.SameArtifactType(
		manifest.ArtifactTypeOrDefault(wanted),
		manifest.ArtifactTypeOrDefault(linked.String(keyType)),
	) {
		return ""
	}

	return linked.String(keyArtifactRepositoryID)
}

// freshArtifact mints the artifact and points the project at it. The two are
// one act: an artifact nothing records is one the next run cannot find, so it
// would create a second and leave the first unreachable.
func freshArtifact(loaded Loaded, repositoryID, label string, report *reporter) (version, error) {
	made, err := createVersion(loaded, repositoryID, label, report)
	if err != nil {
		return made, err
	}

	return made, linkProject(loaded.ProjectDir, made, report)
}

// syncCode uploads the working tree to the artifact's code store. The engine
// does the whole of it: lock the project, diff against what was last synced,
// upload, and patch the artifact's codeRef to the version it minted, which is
// what points the next build at this code.
func syncCode(projectDir string, report *reporter) (*sync.Result, error) {
	var result *sync.Result

	err := report.run("Syncing code", func() error {
		r, syncErr := syncProjectFn(projectDir)
		result = r

		return syncErr
	})
	if err != nil {
		return nil, fmt.Errorf("cannot sync the project's code: %w", err)
	}

	// A conflict is not a failure: the engine keeps the local file and parks
	// a copy of the remote one beside it. It still has to be said out loud,
	// because what was just uploaded is not what the user last saw.
	if result != nil && result.ConflictCount > 0 {
		report.say("  %d file(s) differed from the last sync; local copies kept as %v.\n",
			result.ConflictCount, result.ConflictCopies)
	}

	return result, nil
}

// maybeBuild spends a build only when there is a reason to, and names the
// build it ran so the caller can report it. An empty id means no build
// happened, which is not the same as a build that produced nothing.
//
// A build is minutes, so skipping one that would produce the image already
// sitting on the artifact is worth the two conditions it takes to be sure.
// The reasons to build are: the artifact is new, so no image exists; the sync
// moved something, so any image is now stale; or the user said to.
//
// Otherwise the tree matches what was synced and the only open question is
// whether the previous run got as far as an image. That question is asked of
// the platform rather than assumed, because the common way to arrive here is
// a first deploy that failed after the sync.
func maybeBuild(
	artifactID string,
	fresh bool,
	code CodeChange,
	synced *sync.Result,
	opts Options,
	report *reporter,
) (string, error) {
	if !fresh && !opts.ForceBuild && !changed(code, synced) {
		builds, err := listBuildsFn(artifactID, buildHistoryLimit)
		if err != nil {
			return "", fmt.Errorf("cannot tell whether artifact %s has been built: %w", artifactID, err)
		}

		if hasImage(builds) {
			report.say("  Image is up to date; no rebuild needed.\n")

			return "", nil
		}

		// The tree matches what was synced and no image exists yet — if a
		// build of that code is already running (a previous up triggered it
		// and died, or another terminal is mid-deploy), attach to it instead
		// of starting a second one. Only safe on this path: when the code
		// changed, a running build is building the old tree, and a fresh
		// trigger — which supersedes it server-side — is the correct move.
		if running := runningBuild(builds); running != "" {
			report.say("  A build of this code is already running; attaching to it.\n")

			return buildImage(artifactID, running, opts, report)
		}
	}

	return buildImage(artifactID, "", opts, report)
}

// runningBuild names the newest non-terminal build, "" when there is none
// (the caller then just triggers, which is the pre-attach behavior and
// always safe).
func runningBuild(builds []workload.Build) string {
	if len(builds) == 0 || workload.IsTerminalBuildStatus(builds[0].Status) {
		return ""
	}

	return builds[0].ID
}

// changed reports whether anything moved. The sync's own count is preferred
// over the plan's: the plan was computed before the upload and is a
// prediction, while the result is what happened.
func changed(code CodeChange, synced *sync.Result) bool {
	if synced == nil {
		return code.Changed()
	}

	return synced.UploadedCount > 0 || synced.DeletedCount > 0
}

// hasImage reports whether the artifact has ever built successfully. Any
// completed build answers it, without regard to which is newest: this is only
// asked when the working tree matches what was last synced, so a build that
// completed at all built the code about to be deployed.
//
// The exception is code pushed by `dr artifact code sync` between deploys,
// which leaves a completed build of the previous version and a tree that
// looks unchanged to `up`. That is what --force-build is for.
func hasImage(builds []workload.Build) bool {
	for _, build := range builds {
		if workload.IsBuildCompleted(build.Status) {
			return true
		}
	}

	return false
}

// buildHistoryLimit bounds the look back for a completed build. A handful of
// attempts is all a deploy can have made; past that the answer is no.
const buildHistoryLimit = 20

// buildImage builds the artifact's image and waits for it, streaming the
// build's own log lines beneath the phase header while it runs. attachTo, when
// set, is an already-running build to follow instead of triggering a new one.
//
// The wait is not optional even under --detach: a workload created against an
// artifact with no image would come up unable to start, and --detach promises
// not to wait for the workload, not to deploy something that cannot run.
func buildImage(artifactID, attachTo string, opts Options, report *reporter) (string, error) {
	var built *workload.Build

	err := report.stream("Building the image", func(say func(string, lipgloss.Style)) error {
		buildID := attachTo
		if buildID == "" {
			triggered, triggerErr := triggerBuild(artifactID)
			if triggerErr != nil {
				return triggerErr
			}

			buildID = triggered
		}

		// The line below is the phase's heartbeat: the trigger above can
		// legitimately take minutes against a slow platform, and the first
		// log line lags the build by however long ingestion takes, so without
		// it the header sits alone and reads as a hang.
		say("following build "+buildID+"; its first log lines can take a minute to arrive", tui.HintStyle)

		// The tail is progress feedback only: it never errors, and after
		// sustained fetch failures it says so once and goes quiet, while the
		// build wait carries on. Its notices go into the stream marked as the
		// CLI's own — silence here is indistinguishable from a hang, which is
		// worse than one meta line among the build's output.
		tail := workload.NewBuildLogTail(artifactID, buildID,
			func(e workload.WorkloadLogEntry) { line, style := buildLogLine(e); say(line, style) },
			func(w string) { say("(log stream) "+w, tui.WarnStyle) })

		// Each status transition is narrated, because the stages before the
		// builder speaks — accepted, waiting for a builder — are exactly the
		// quiet ones where the user wonders whether anything is happening.
		// Terminal statuses are left to the phase's own ending.
		lastStatus := ""

		onTick := func(b *workload.Build) {
			if b != nil && !workload.IsTerminalBuildStatus(b.Status) && !strings.EqualFold(b.Status, lastStatus) {
				lastStatus = b.Status

				say(buildStatusLine(b.Status), tui.HintStyle)
			}

			tail.Poll()
		}

		// WaitForBuild hands the build back alongside its error when the
		// build ends badly or the wait runs out, so the id is taken from it
		// before the error is looked at: it is the only way to the logs.
		b, waitErr := waitBuildFn(artifactID, buildID, opts.PollInterval, opts.PollTimeout, onTick)
		built = b

		// One more poll after the terminal status: ingestion lags the build,
		// so the last lines routinely land after the wait has already ended.
		tail.Poll()

		return waitErr
	})

	if built == nil {
		// The build was never triggered, or the very first poll failed.
		return "", err
	}

	if workload.IsBuildErrorStatus(built.Status) {
		// Said here rather than left to the wait's own wording, because this
		// is the only place that knows which artifact the build belongs to.
		return built.ID, fmt.Errorf("build %s finished as %s; see 'dr artifact build logs %s %s'",
			built.ID, built.Status, artifactID, built.ID)
	}

	// A build still running when the wait expires keeps its id too: it is
	// what the user needs to follow it to the end.
	return built.ID, err
}

// buildLogLine renders one streamed build log line with the style its level
// deserves. The message alone, dimmed, is the line for routine output; a
// non-info level is called out and colored, because an error scrolling past
// dimmed and unmarked defeats the point of streaming.
func buildLogLine(e workload.WorkloadLogEntry) (string, lipgloss.Style) {
	switch strings.ToLower(e.Level) {
	case "", "info", "debug":
		return e.Message, tui.HintStyle
	case "warn", "warning":
		return "[" + strings.ToUpper(e.Level) + "] " + e.Message, tui.WarnStyle
	default:
		return "[" + strings.ToUpper(e.Level) + "] " + e.Message, tui.ErrorStyle
	}
}

// buildStatusLine narrates a build status for the stream, in words rather
// than enum values for the two stages every build passes through.
func buildStatusLine(status string) string {
	switch strings.ToUpper(status) {
	case workload.BuildStatusPending:
		return "build accepted; waiting for a builder to pick it up"
	case workload.BuildStatusInProgress:
		return "builder running"
	case "BUILT":
		return "image built; pushing it to the registry"
	default:
		return "build is " + strings.ToLower(status)
	}
}

// triggerBuild starts one build and names it. The endpoint answers with a
// list because an artifact can have several containers to build; the deploy
// follows the first, since the CLI only ever describes one.
func triggerBuild(artifactID string) (string, error) {
	triggered, err := triggerBuildFn(artifactID)
	if err != nil {
		return "", err
	}

	if len(triggered.BuildIDs) == 0 {
		return "", errors.New("the platform accepted the build but named no build to follow")
	}

	return triggered.BuildIDs[0], nil
}

// defaultSync runs the sync engine over the project. Yes is set because `up`
// has already printed its plan and been confirmed, if confirmation was ever
// going to happen; a second prompt from inside a phase would be a surprise,
// and in CI it would be a hang.
func defaultSync(projectDir string) (*sync.Result, error) {
	engine, err := sync.New(projectDir, sync.Options{Yes: true})
	if err != nil {
		return nil, err
	}

	defer func() { _ = engine.Close() }()

	return engine.Run()
}
