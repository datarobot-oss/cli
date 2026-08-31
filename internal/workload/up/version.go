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
	"github.com/datarobot/cli/internal/workload/manifest"
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

	Fresh bool

	// BuildID names the image build this run ran, "" when it ran none. A
	// failed build sets it: it is the way to the logs.
	BuildID string

	ImageURI string

	// HasCode reports that it points at the code its image was built from.
	HasCode bool
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
	plan Plan,
	repositoryID string,
	opts Options,
	report *reporter,
) (version, error) {
	made, err := versionArtifact(loaded, live, plan, repositoryID, report)
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

	if err := inheritCode(loaded.ProjectDir, made, synced, report); err != nil {
		return made, err
	}

	buildID, err := maybeBuild(loaded.ProjectDir, made, plan.Code, synced, opts, report)
	made.BuildID = buildID

	return made, err
}

func versionArtifact(
	loaded Loaded,
	live Live,
	plan Plan,
	repositoryID string,
	report *reporter,
) (version, error) {
	if made, ok := abandoned(loaded, live, plan, repositoryID); ok {
		report.say("  Continuing with artifact %s from an earlier attempt.\n", made.ID)

		return made, nil
	}

	if plan.InheritsImage {
		made, err := copiedVersion(loaded, live, report)
		if err == nil {
			return made, nil
		}

		// A copy still standing stops the fallback: creating past it would
		// strand an artifact nothing points at. Every other failure left
		// nothing behind, so the deploy that builds is taken.
		if made.ID != "" {
			return made, err
		}

		log.Debug("the running version could not be copied; creating the new one instead", "err", err)
		report.say("  The running version could not be copied (%v), so the new version is built after all.\n", err)
	}

	return createVersion(loaded, repositoryID, labelNewVersion, report)
}

// inheritsImage reports whether this run may copy the running version rather
// than create one, which decides whether it spends a build.
//
// The running image is the whole of the evidence: a build-from-source
// container has its imageUri stripped at create and written back only once a
// build completes. A build row would answer wrong the moment that version is
// itself a copy, since a copy keeps the image and leaves the rows behind.
func inheritsImage(live Live, plan Plan, opts Options, kind, artifactName string) bool {
	return plan.RollsArtifact() &&
		plan.Code.Applies &&
		!opts.ForceBuild &&
		!plan.RebuildsImage() &&
		live.ImageURI != "" &&
		artifactName != "" &&
		sameArtifactTypeAs(kind, live.ArtifactType)
}

// copiedVersion is the version a runtime-only change rolls onto: a copy of the
// running one, written over with what the file says and read back, since an
// update replaces a container wholesale.
func copiedVersion(loaded Loaded, live Live, report *reporter) (version, error) {
	var made version

	err := report.run(labelCopiedVersion, func() error {
		spec, specErr := loaded.Compiled.ArtifactSpecPayload()
		if specErr != nil {
			return specErr
		}

		copied, cloneErr := copyArtifactFn(live.ArtifactID, loaded.Compiled.ArtifactName)
		if cloneErr != nil {
			return cloneErr
		}

		// Recorded before the update, so a failure below can name it.
		made = version{ID: copied.ID, Fresh: true}

		if err := sameLineage(copied, live); err != nil {
			return err
		}

		spec, specErr = keepCodeRef(spec, copied)
		if specErr != nil {
			return specErr
		}

		if updateErr := updateArtifactSpecFn(copied.ID, spec); updateErr != nil {
			return updateErr
		}

		hasCode, imageURI, matches := carried(loaded, copied.ID)
		if !matches {
			// Accepted and still not what the file says: promoting it would
			// report a deploy that changed nothing.
			return errors.New("the copy does not say what " + manifest.FileName + " asks for")
		}

		made.HasCode, made.ImageURI = hasCode, imageURI

		return nil
	})
	if err != nil && made.ID != "" {
		return discardCopy(made, live.ArtifactID, err)
	}

	return made, err
}

// sameLineage refuses a copy the platform put somewhere other than where the
// running version lives: an artifact cannot be moved once it exists, so
// promoting one from elsewhere forks the version history permanently. A copy
// stating no repository is not refused, only one that disagrees.
func sameLineage(copied *workload.Artifact, live Live) error {
	if live.ArtifactRepositoryID == "" || copied.ArtifactRepositoryID == "" {
		return nil
	}

	if copied.ArtifactRepositoryID == live.ArtifactRepositoryID {
		return nil
	}

	return fmt.Errorf(
		"the copy landed in artifact repository %s rather than %s, which the running version cannot leave",
		copied.ArtifactRepositoryID, live.ArtifactRepositoryID)
}

// discardCopy takes back a copy this run cannot use: nothing points at it, so
// leaving it costs one artifact per failed attempt. The version it returns
// says whether anything is still standing.
func discardCopy(made version, sourceID string, cause error) (version, error) {
	if err := deleteArtifactFn(made.ID); err != nil {
		log.Debug("cannot remove the unusable copy", "artifact_id", made.ID, "err", err)

		return made, fmt.Errorf(
			"artifact %s was copied from %s, could not be made to match %s, and could not be removed, "+
				"so it is left behind: %w",
			made.ID, sourceID, manifest.FileName, cause)
	}

	return version{}, fmt.Errorf(
		"the copy of artifact %s could not be made to match %s: %w",
		sourceID, manifest.FileName, cause)
}

// carried reports what the artifact has once the write is done: code, image,
// and whether it says what the file asks for. A read that fails answers no to
// the first two and yes to the third, which is the safe way round.
func carried(loaded Loaded, artifactID string) (hasCode bool, imageURI string, matches bool) {
	doc, err := getArtifactDocFn(artifactID)
	if err != nil {
		log.Debug("cannot read back the copied artifact; treating it as carrying nothing",
			"artifact_id", artifactID, "err", err)

		return false, "", true
	}

	container := workload.PrimaryContainerInDocument(doc)
	uri, _ := container[keyImageURI].(string)

	matched, err := specMatches(loaded, doc)
	if err != nil {
		log.Debug("cannot tell whether the copy took the change", "artifact_id", artifactID, "err", err)

		matched = true
	}

	return workload.CodeRefInContainer(container) != nil, uri, matched
}

// keepCodeRef puts the copy's own code reference into the spec about to be
// written over it. The manifest states none, so the file's spec alone would
// take it away, and the platform refuses a container that says how to build
// itself but not from what. Nothing else puts it back: this deploy never builds.
func keepCodeRef(spec json.RawMessage, copied *workload.Artifact) (json.RawMessage, error) {
	if copied == nil {
		return spec, nil
	}

	ref := workload.ExtractCodeRef(*copied)
	if ref == nil {
		return spec, nil
	}

	return workload.SpecWithPrimaryCodeRef(spec, ref.CatalogID, ref.CatalogVersionID)
}

// The two things a create can be. A project's first artifact is not a new
// version of anything, and saying so in a fresh directory reads as though the
// run found something it did not.
const (
	labelFirstArtifact = "Creating the artifact"
	labelNewVersion    = "Creating the new version"

	labelCopiedVersion = "Copying the running version"
)

// createVersion mints an artifact from the file's artifact block, into
// repositoryID when the run has a lineage for it to join. An empty
// repositoryID leaves the platform to open one, which is what the first deploy
// of anything wants.
func createVersion(loaded Loaded, repositoryID, label string, report *reporter) (version, error) {
	var (
		created  *workload.Artifact
		fellBack bool
	)

	err := report.run(label, func() error {
		payload, payloadErr := loaded.Compiled.ArtifactPayload()
		if payloadErr != nil {
			return payloadErr
		}

		artifact, forked, createErr := createInRepository(payload, repositoryID)
		created, fellBack = artifact, forked

		return createErr
	})
	if err != nil {
		return version{}, err
	}

	// Said out loud rather than only to the debug log. The deploy itself is
	// fine and there is nothing to do about it, but from here the workload's
	// versions are two lineages and the Registry carries a second entry under
	// the same name. Nobody goes looking for the cause of that after a run
	// that printed nothing but check marks.
	if fellBack {
		report.say("  Artifact repository %s would not take the new version, so it starts one of its own.\n",
			repositoryID)
	}

	// An id is the whole of what a link records, and SaveConfig does not
	// validate the way Initialize does: linking to an empty one writes a
	// config.json every later command rejects as corrupt. Refusing here keeps a
	// malformed response from turning into damaged local state, and keeps the
	// dereference below from panicking on a body that decoded to null.
	if created == nil || created.ID == "" {
		return version{}, errors.New("the platform accepted the artifact but reported no id for it")
	}

	return version{ID: created.ID, Fresh: true}, nil
}

// createInRepository mints the artifact inside repositoryID, and falls back to
// letting the platform open one when it will not take that repository. It
// reports whether that fallback fired, which is the one thing that happens
// here the caller has to say out loud.
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
func createInRepository(payload json.RawMessage, repositoryID string) (*workload.Artifact, bool, error) {
	// Every request below starts from a block with no repository on it. The
	// field is the platform's own and the file has no business carrying one,
	// but the compiler passes keys it does not recognize through untouched, so
	// a hand-written id would otherwise join a lineage this run may have just
	// decided to leave, and would turn the fallback below into a second
	// request for a repository rather than a request for none.
	free, err := withoutRepository(payload)
	if err != nil {
		return nil, false, err
	}

	if repositoryID == "" {
		artifact, createErr := createArtifactFn(free)

		return artifact, false, createErr
	}

	joined, err := withRepository(free, repositoryID)
	if err != nil {
		return nil, false, err
	}

	artifact, err := createArtifactFn(joined)
	if err == nil || !repositoryRefused(err) {
		return artifact, false, err
	}

	log.Debug("artifact repository refused; creating the version in a new one",
		"artifact_repository_id", repositoryID, "err", err)

	artifact, err = createArtifactFn(free)

	// A retry that failed too forked nothing, so there is nothing to announce
	// and the error it came back with is the whole story.
	return artifact, err == nil, err
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
// added to it, and with anything the file said about one taken off first: the
// id is the platform's own field, and this is the only place that decides it.
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

	delete(block, keyArtifactRepositoryID)

	if repositoryID != "" {
		block[keyArtifactRepositoryID] = repositoryID
	}

	raw, err := json.Marshal(block)
	if err != nil {
		return nil, fmt.Errorf("cannot convert the artifact block to JSON: %w", err)
	}

	return raw, nil
}

// withoutRepository is the artifact block with no repository on it at all,
// which is the create that asks the platform to open one.
func withoutRepository(payload json.RawMessage) (json.RawMessage, error) {
	return withRepository(payload, "")
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
// The repository is checked for the same reason as the spec, and the one this
// roll set out to land in decides. An artifact cannot be moved between
// repositories once it exists, so promoting a draft from elsewhere does not
// merge two lineages: it abandons the one the roll was joining and adopts the
// draft's, from that run onwards and with nothing said. The drafts this can
// happen with are the ones a release before this one left behind and the ones
// `dr artifact create` makes, both of which sit in a repository of their own.
// Minting a fresh version costs the stray draft that reuse exists to avoid;
// reusing costs the lineage, permanently.
func abandoned(loaded Loaded, live Live, plan Plan, repositoryID string) (version, bool) {
	if !projectLinkedFn(loaded.ProjectDir) {
		return version{}, false
	}

	cfg, err := loadProjectFn(loaded.ProjectDir)
	if err != nil || cfg.ArtifactID == "" || cfg.ArtifactID == live.ArtifactID {
		return version{}, false
	}

	artifact, err := getArtifactFn(cfg.ArtifactID)
	if err != nil || artifact.IsLocked() {
		return version{}, false
	}

	if !joinable(artifact, repositoryID) {
		return version{}, false
	}

	if !describes(loaded, cfg.ArtifactID) {
		return version{}, false
	}

	// A leftover keeps its code reference only when it also keeps the image
	// that reference was built into.
	image := inheritedImage(artifact, live, plan)

	return version{
		ID:       cfg.ArtifactID,
		HasCode:  image != "",
		ImageURI: image,
	}, true
}

// inheritedImage is the leftover's image when it is the one the version now
// serving runs, and "" otherwise. The plan decides first: without it, a
// leftover minted before the dockerfile changed would skip the build.
func inheritedImage(draft *workload.Artifact, live Live, plan Plan) string {
	if !plan.InheritsImage {
		return ""
	}

	uri := workload.GetPrimaryContainerImageURI(*draft)
	if uri == "" || uri != live.ImageURI {
		return ""
	}

	return uri
}

// joinable reports whether promoting this draft would leave the workload in
// the repository the roll is heading for.
//
// It is the roll's intended repository that decides, not the live artifact's.
// The two differ exactly when the file changed the artifact's kind: the roll
// is then starting a lineage of its own by design, so there is nothing left to
// protect, and measuring against the lineage being left would refuse the very
// draft the previous attempt minted and mint another one per attempt, which is
// the pile reuse exists to prevent.
//
// An empty repository is that case, and also a workload that has none to lose,
// which is every workload created before repositories were sent at all.
func joinable(draft *workload.Artifact, repositoryID string) bool {
	if repositoryID == "" {
		return true
	}

	return draft.ArtifactRepositoryID == repositoryID
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
	doc, err := getArtifactDocFn(artifactID)
	if err != nil {
		return false
	}

	matches, err := specMatches(loaded, doc)

	return err == nil && matches
}

// specMatches is describes with the failure kept separate, for a caller whose
// answer to "cannot tell" is not the same as its answer to "no".
//
// Reusing a draft that might not match costs a rollout of the wrong thing, so
// describes folds an unreadable artifact into "no". Replacing one costs the
// artifact being replaced, so a caller that mints on "no" must not mint on a
// fault: that orphans the artifact the project is pushing to and starts the
// lineage again for a reason that was never established.
//
// The document is passed in rather than fetched, so a caller that already has
// it, or that needs the lock status off the same read, pays one round trip
// instead of two to the same resource.
func specMatches(loaded Loaded, doc workload.Document) (bool, error) {
	// The comparison below normalises both sides in place, and the live side
	// belongs to the caller. ensureArtifact hands the same document to
	// redraftRepository immediately afterwards, which reads the type off it;
	// left shared, this would take that type away, an agent would read as a
	// service, and the replacement would open a new repository rather than
	// joining the lineage it is a version of. A silent lineage fork on the
	// recovery path is a poor price for one map allocation.
	doc = detachedArtifact(doc)

	payload, err := loaded.Compiled.ArtifactPayload()
	if err != nil {
		return false, err
	}

	var want map[string]any

	if err := json.Unmarshal(payload, &want); err != nil {
		return false, err
	}

	// The name is the CLI's own, derived from the workload's, and the block
	// carries it only so a create has one to send. It says nothing about what
	// the version runs, and an artifact renamed in the UI is not a reason to
	// mint another.
	delete(want, keyArtifactName)

	// The repository is not the file's to state either, and createInRepository
	// takes it off every request, so a draft can never carry the one written
	// there. Comparing it would make a hand-written id refuse every leftover
	// for good: a fresh draft per attempt, and the same id refusing that one
	// too.
	delete(want, keyArtifactRepositoryID)

	// The type is settled before the walk and taken out of both sides, because
	// the walk cannot answer it correctly. It compares by exact equality, so a
	// file saying "Service" against a live "service", or a document that
	// carries no type at all, would each read as a difference no deploy can
	// settle: the leftover is refused, another draft is minted, and the next
	// attempt refuses that one too. It is the same question the plan asks, so
	// it is asked the same way, folded and with only the live side defaulted.
	if !sameArtifactTypeIn(want, doc) {
		return false, nil
	}

	// Both directions, which is the difference between this and measuring
	// drift. Subset catches a value the file changed or added; Extra catches
	// one it removed. Without the second, deleting an environment variable
	// leaves everything the file still mentions matching, the leftover is
	// reused, and it is rolled out still carrying the variable that was
	// deleted, which minting a fresh artifact would have dropped.
	return len(Subset(want, doc)) == 0 && len(Extra(want, doc)) == 0, nil
}

// detachedArtifact is doc with the two maps the normalisation writes to copied,
// so a caller's document survives being compared.
//
// It copies exactly what hoistArtifactType and sameArtifactTypeIn touch: the
// top level, where the type is written and then removed, and the spec, where a
// type written inside it is deleted from. Everything deeper is shared, which is
// safe because nothing below reaches it, and a deep copy of a document that can
// carry an entire container spec is not worth paying for a two-key edit.
func detachedArtifact(doc workload.Document) workload.Document {
	if doc == nil {
		return nil
	}

	out := make(workload.Document, len(doc))
	for k, v := range doc {
		out[k] = v
	}

	if spec, ok := out[keySpec].(map[string]any); ok {
		copied := make(map[string]any, len(spec))
		for k, v := range spec {
			copied[k] = v
		}

		out[keySpec] = copied
	}

	return out
}

// sameArtifactTypeIn reports whether two artifact blocks name the same kind,
// and takes the type out of both so the caller's walk never sees it.
//
// Placement is normalised first, since a file may write the type inside the
// spec while the platform answers with it beside one. Then the comparison is
// the plan's: folded, because the platform does not agree with itself about
// the casing of its enums, and with only the live side defaulted, because an
// artifact that states no type is a service while a file that states none has
// no opinion and is owed no difference.
func sameArtifactTypeIn(want, have map[string]any) bool {
	hoistArtifactType(want)
	hoistArtifactType(have)

	wanted, _ := want[keyType].(string)
	running, _ := have[keyType].(string)

	delete(want, keyType)
	delete(have, keyType)

	return sameArtifactTypeAs(wanted, running)
}

// sameArtifactTypeAs compares the kind a file states against the kind a
// workload runs. Folded, because the platform disagrees with itself about the
// casing of its enums, and with only the live side defaulted, because a file
// stating no type has no opinion. One function rather than the same three
// lines twice, which is how the plan and the copy path came apart for an agent
// whose file leaves it out. Deliberately not sameRepository's question, which
// is what kind of thing is about to be created, so both sides default there.
func sameArtifactTypeAs(wanted, running string) bool {
	if wanted == "" {
		return true
	}

	return manifest.SameArtifactType(wanted, manifest.ArtifactTypeOrDefault(running))
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

	// From here the directory is already linked, which rules out the advice
	// below: 'dr artifact code init' refuses a directory that has state, so
	// naming it would send the reader to a command that cannot run.

	cfg, err := loadProjectFn(projectDir)
	if err != nil {
		return fmt.Errorf("cannot read which artifact %s is linked to: %w", projectDir, err)
	}

	cfg.ArtifactID = made.ID

	if err := saveProjectFn(projectDir, cfg); err != nil {
		return relinkFailure(made.ID, projectDir, err)
	}

	report.say("  Project now pushes to artifact %s.\n", made.ID)

	return nil
}

// linkFailure says which artifact was stranded, for a directory that had no
// link to begin with. The artifact exists and nothing records that fact, so the
// next run would make another; naming the id is the difference between a re-run
// and a support ticket.
func linkFailure(artifactID, projectDir string, err error) error {
	return fmt.Errorf(
		"artifact %s was created but %s could not be pointed at it, so the next deploy would create another. "+
			"Run 'dr artifact code init %s' here, then deploy again: %w",
		artifactID, projectDir, artifactID, err)
}

// relinkFailure is the same accident on a directory that is already linked,
// which is every roll and every redraft of a locked artifact. It cannot name
// 'dr artifact code init', because that command refuses a directory holding
// state and would send the reader in a circle; and deleting the state to force
// it would throw away the catalog and the last-synced version, which is the
// cost the relink exists to avoid. What is left is the file itself, so the
// error names it and the one field to change.
func relinkFailure(artifactID, projectDir string, err error) error {
	return fmt.Errorf(
		"artifact %s was created but %s could not be pointed at it, so the next deploy would create another. "+
			"Fix whatever stopped the write, then set \"artifactId\" to %s in %s by hand and deploy again: %w",
		artifactID, projectDir, artifactID, wapi.ConfigPath(projectDir), err)
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
//
// A version already carrying the code its image was built from is left
// pointing there: re-anchoring it at whatever the project last synced would
// claim code the image was never built from. Anything else is re-anchored,
// including a leftover with a reference of its own, since `dr artifact create`
// and an abandoned attempt both point at code older than the last sync.
func inheritCode(projectDir string, made version, synced *sync.Result, report *reporter) error {
	if made.HasCode {
		return nil
	}

	// A sync that moved anything has already pointed the artifact at what it
	// produced, so only one that found nothing to do leaves a version with no
	// code of its own. Minting a version is the usual proof of that, but not
	// the only one: a sync whose whole plan was remote deletes can move the
	// code store without the engine reporting a new version, and inheriting
	// then would pin the artifact at the state before those deletes.
	if synced != nil && (synced.NewVersion != "" || synced.UploadedCount > 0 || synced.DeletedCount > 0) {
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
			made.ID, projectDir)
	}

	err = report.run("Carrying the code over", func() error {
		return patchCodeRefFn(made.ID, catalog, lastVersion)
	})
	if err != nil {
		return fmt.Errorf("cannot point artifact %s at the code it should build (%s): %w",
			made.ID, lastVersion, err)
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
