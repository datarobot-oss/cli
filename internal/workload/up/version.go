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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/datarobot/cli/internal/workload/wapi"
)

// keyArtifactName is the artifact block's own name field, dropped before the
// spec comparison below.
const keyArtifactName = "name"

// version is the artifact a roll promotes, and what making it cost.
type version struct {
	// ID is the artifact. It is set as soon as one exists, including when
	// what came back alongside it was an error, so the caller can say what
	// was left behind.
	ID string

	// Fresh reports that this run created it, which means it is certainly a
	// draft and certainly has no image yet.
	Fresh bool

	// BuildID names the image build this run ran, "" when it ran none. A
	// failed build sets it: it is the way to the logs.
	BuildID string
}

// buildVersion produces the version a built project rolls onto: a new
// artifact carrying the working tree, with an image made from it.
//
// The artifact is created and the project pointed at it before the sync, not
// after. The sync engine patches the code reference of whichever artifact the
// project is linked to, and until this runs that is the version currently
// serving: syncing first would rewrite what production points at while
// production is running it, and would fail outright once that version is
// locked, halfway through, with the upload already done.
//
// Every step here is recoverable except by leaving a draft artifact behind,
// which is why the next run looks for one before making another.
func buildVersion(
	loaded Loaded,
	live Live,
	code CodeChange,
	repositoryID string,
	opts Options,
	report *reporter,
) (version, error) {
	made, err := versionArtifact(loaded, live, repositoryID, report)
	if err != nil {
		return made, err
	}

	if err := linkProject(loaded.ProjectDir, made, report); err != nil {
		return made, err
	}

	synced, err := syncCode(loaded.ProjectDir, report)
	if err != nil {
		return made, err
	}

	if err := inheritCode(loaded.ProjectDir, made.ID, synced, report); err != nil {
		return made, err
	}

	// A version minted here has no image whatever the working tree says, so
	// Fresh is what stops maybeBuild from skipping the build and promoting an
	// artifact with nothing to run. One picked up from an earlier attempt may
	// already have its image, and that is the skip worth having.
	buildID, err := maybeBuild(made.ID, made.Fresh, code, synced, opts, report)
	made.BuildID = buildID

	return made, err
}

// versionArtifact returns the artifact to fill and build, reusing the one an
// earlier attempt left behind rather than adding to a pile of drafts.
func versionArtifact(loaded Loaded, live Live, repositoryID string, report *reporter) (version, error) {
	if id, ok := abandoned(loaded, live); ok {
		report.say("  Continuing with artifact %s from an earlier attempt.\n", id)

		return version{ID: id}, nil
	}

	return createVersion(loaded, repositoryID, report)
}

// createVersion mints an artifact from the file's artifact block, into
// repositoryID when the run has a lineage for it to join. An empty
// repositoryID leaves the platform to open one, which is what the first deploy
// of anything wants.
func createVersion(loaded Loaded, repositoryID string, report *reporter) (version, error) {
	var created *workload.Artifact

	err := report.run("Creating the new version", func() error {
		payload, payloadErr := loaded.Compiled.ArtifactPayload()
		if payloadErr != nil {
			return payloadErr
		}

		artifact, createErr := createInRepository(payload, repositoryID)
		created = artifact

		return createErr
	})
	if err != nil {
		return version{}, err
	}

	return version{ID: created.ID, Fresh: true}, nil
}

// createInRepository mints the artifact inside repositoryID, and falls back to
// letting the platform open one when it will not take that repository.
//
// The fallback is what keeps this from turning deploys that worked into
// failures. The id is read off the live artifact rather than typed by anyone,
// so a refusal names something the user cannot correct: a repository deleted
// from the UI and one belonging to whoever created the workload and never
// shared both answer 404, and neither is a reason to leave a workload on the
// version it was. Landing in a repository of its own is precisely the
// behaviour this change exists to stop being the default, which is what makes
// it the right thing to fall back to.
//
// The retry covers both answers the repository can produce, and only when one
// was actually asked for. A repository that is gone or was never shared comes
// back 404; one whose type disagrees with the artifact comes back 422, which
// sameRepository tries to predict but cannot promise, since the prediction
// reads the live artifact's type rather than the repository's own. Retrying
// both is what keeps a wrong guess from failing a deploy instead of costing a
// round trip.
//
// Dropping the id cannot make a refused artifact acceptable, so a 422 the
// repository had nothing to do with fails again, identically, and that second
// answer is the one reported. The cost of covering the case is one wasted
// request on a deploy that was going to fail anyway. Reading the message to
// tell the two apart would be exact until the day the API rewords it.
func createInRepository(payload json.RawMessage, repositoryID string) (*workload.Artifact, error) {
	if repositoryID == "" {
		return createArtifactFn(payload)
	}

	joined, err := withRepository(payload, repositoryID)
	if err != nil {
		return nil, err
	}

	artifact, err := createArtifactFn(joined)
	if err == nil || !repositoryRefused(err) {
		return artifact, err
	}

	log.Debug("artifact repository refused; creating the version in a new one",
		"artifact_repository_id", repositoryID, "err", err)

	return createArtifactFn(payload)
}

// repositoryRefused reports whether err is an answer a repository can cause.
func repositoryRefused(err error) bool {
	var httpErr *drapi.HTTPError

	if !errors.As(err, &httpErr) {
		return false
	}

	return httpErr.StatusCode == http.StatusNotFound ||
		httpErr.StatusCode == http.StatusUnprocessableEntity
}

// withRepository is the artifact block with the repository it should join
// added to it.
//
// The id goes on the payload and never on the compiled manifest, and that
// placement is load-bearing twice. The plan measures the file against the live
// spec, so a server-owned id on the file's side of that comparison would read
// as drift on every run, for ever. And a leftover draft is reused only when the
// file still describes it, a check that compares this same block against the
// artifact: an id in the block would fail every draft minted before this
// change and quietly mint a replacement each time.
func withRepository(payload json.RawMessage, repositoryID string) (json.RawMessage, error) {
	var block map[string]any

	if err := json.Unmarshal(payload, &block); err != nil {
		return nil, fmt.Errorf("cannot read the artifact block: %w", err)
	}

	block[keyArtifactRepositoryID] = repositoryID

	raw, err := json.Marshal(block)
	if err != nil {
		return nil, fmt.Errorf("cannot convert the artifact block to JSON: %w", err)
	}

	return raw, nil
}

// abandoned reports an artifact a previous roll made and never promoted, and
// which still describes what the file asks for.
//
// The project's link is the only record of it, and it is only a candidate
// when it is not the version currently serving: linked to the live one means
// the last roll finished, and the next change needs an artifact of its own.
// A locked one is no use either, since nothing more can be pushed into it,
// which is the state a roll onto production leaves behind when it dies
// between the lock and the swap.
//
// The spec is checked as well, because an artifact's is fixed when it is
// created. Between the attempt that left this one behind and now, the file
// may have moved a port, a probe or an environment variable; the draft still
// carries the old answer, and promoting it would roll out a version the file
// no longer describes, report success, and leave the same drift to be found
// again next run. A draft that no longer matches is left where it is and a
// new one minted, which is what happens anyway when there is nothing to
// reuse.
//
// The repository is checked for the same reason as the spec, and the one the
// running version is in decides. An artifact cannot be moved between
// repositories once it exists, so promoting a draft from elsewhere does not
// merge two lineages: it abandons the workload's own and adopts the draft's,
// from that run onwards and with nothing said. The drafts this can happen with
// are the ones a release before this one left behind and the ones
// `dr artifact create` makes, both of which sit in a repository of their own.
// Minting a fresh version costs the stray draft that reuse exists to avoid;
// reusing costs the lineage, permanently.
func abandoned(loaded Loaded, live Live) (string, bool) {
	if !projectLinkedFn(loaded.ProjectDir) {
		return "", false
	}

	cfg, err := loadProjectFn(loaded.ProjectDir)
	if err != nil || cfg.ArtifactID == "" || cfg.ArtifactID == live.ArtifactID {
		return "", false
	}

	artifact, err := getArtifactFn(cfg.ArtifactID)
	if err != nil || artifact.IsLocked() {
		return "", false
	}

	if !joinable(artifact, live) {
		return "", false
	}

	if !describes(loaded, cfg.ArtifactID) {
		return "", false
	}

	return cfg.ArtifactID, true
}

// joinable reports whether promoting this draft would leave the workload in
// the repository it is already in. A workload in none has nothing to lose,
// which is every workload created before repositories were sent at all.
func joinable(draft *workload.Artifact, live Live) bool {
	if live.ArtifactRepositoryID == "" {
		return true
	}

	return draft.ArtifactRepositoryID == live.ArtifactRepositoryID
}

// describes reports whether the draft still says what the file's artifact
// block says. The comparison is the plan's own, one-directional: a field the
// file does not mention is not a difference, so a draft carrying something
// the platform filled in for itself is still a match.
//
// A read that fails answers no. Minting a second draft costs an artifact
// nobody promotes; promoting one whose spec was never confirmed costs a
// rollout of the wrong thing.
func describes(loaded Loaded, artifactID string) bool {
	payload, err := loaded.Compiled.ArtifactPayload()
	if err != nil {
		return false
	}

	var want map[string]any

	if err := json.Unmarshal(payload, &want); err != nil {
		return false
	}

	doc, err := getArtifactDocFn(artifactID)
	if err != nil {
		return false
	}

	// The name is the CLI's own, derived from the workload's, and the block
	// carries it only so a create has one to send. It says nothing about what
	// the version runs, and an artifact renamed in the UI is not a reason to
	// mint another.
	delete(want, keyArtifactName)

	// Both directions, which is the difference between this and measuring
	// drift. Subset catches a value the file changed or added; Extra catches
	// one it removed. Without the second, deleting an environment variable
	// leaves everything the file still mentions matching, the leftover is
	// reused, and it is rolled out still carrying the variable that was
	// deleted, which minting a fresh artifact would have dropped.
	return len(Subset(want, doc)) == 0 && len(Extra(want, doc)) == 0
}

// linkProject points the project at the new version, so the sync uploads into
// it and the next run can find it. The catalog holding the code and the
// version last pushed to it are carried across untouched: they describe the
// code store, which is shared, and clearing them would make the next sync
// treat every file as new.
func linkProject(projectDir string, made version, report *reporter) error {
	// An artifact picked up from an earlier attempt is the one the project is
	// already pointed at, which is how it was found.
	if !made.Fresh {
		return nil
	}

	if !projectLinkedFn(projectDir) {
		if err := initProjectFn(projectDir, wapi.InitOptions{ArtifactID: made.ID}); err != nil {
			return linkFailure(made.ID, projectDir, err)
		}

		return nil
	}

	cfg, err := loadProjectFn(projectDir)
	if err != nil {
		return fmt.Errorf("cannot read which artifact %s is linked to: %w", projectDir, err)
	}

	cfg.ArtifactID = made.ID

	if err := saveProjectFn(projectDir, cfg); err != nil {
		return linkFailure(made.ID, projectDir, err)
	}

	report.say("  Project now pushes to artifact %s.\n", made.ID)

	return nil
}

// linkFailure says which artifact was stranded. The artifact exists and
// nothing records that fact, so the next run would make another; naming the
// id is the difference between a re-run and a support ticket.
func linkFailure(artifactID, projectDir string, err error) error {
	return fmt.Errorf(
		"artifact %s was created but %s could not be pointed at it, so the next deploy would create another. "+
			"Run 'dr artifact code init %s' here, then deploy again: %w",
		artifactID, projectDir, artifactID, err)
}

// inheritCode gives the new version the code the old one was running, when
// the sync had nothing to upload.
//
// A spec-only roll is the case: the file changed a port or a probe, the
// working tree did not, and the engine returns without minting a version or
// patching anything. The new artifact would then go to the builder with no
// code reference at all, and the roll would promote something that cannot
// start. Pointing it at the version last synced is what makes "a new version
// of the same code" true.
func inheritCode(projectDir, artifactID string, synced *sync.Result, report *reporter) error {
	if synced != nil && synced.NewVersion != "" {
		return nil
	}

	cfg, err := loadProjectFn(projectDir)
	if err != nil {
		return fmt.Errorf("cannot read the code this project last pushed: %w", err)
	}

	catalog, lastVersion := deref(cfg.CatalogID), deref(cfg.LastSyncedVersionID)
	if catalog == "" || lastVersion == "" {
		return fmt.Errorf(
			"artifact %s has no code and the working tree had nothing to upload, so there would be "+
				"nothing to build. Check that %s holds the project's source",
			artifactID, projectDir)
	}

	err = report.run("Carrying the code over", func() error {
		return patchCodeRefFn(artifactID, catalog, lastVersion)
	})
	if err != nil {
		return fmt.Errorf("cannot point artifact %s at the code it should build (%s): %w",
			artifactID, lastVersion, err)
	}

	return nil
}

// deref reads an optional recorded id.
func deref(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}
