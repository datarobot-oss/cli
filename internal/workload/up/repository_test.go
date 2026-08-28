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
	"net/http"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// liveRepositoryID is the repository the running version in the roll fixtures
// belongs to. Anything a roll mints should join it.
const liveRepositoryID = "68c0000000000000000000f1"

// inRepository puts the running version in liveRepositoryID, which is what
// every artifact the platform has actually created looks like. The base
// fixture leaves it out so the older shape stays covered too.
func inRepository(f fakes) fakes {
	return liveArtifact(f, func(d workload.Document) {
		d[keyArtifactRepositoryID] = liveRepositoryID
	})
}

// asType is inRepository with the running version relabelled, so a file asking
// for another kind has something to disagree with.
func asType(f fakes, artifactType string) fakes {
	return liveArtifact(inRepository(f), func(d workload.Document) {
		d[keyType] = artifactType
	})
}

// liveArtifact rebuilds the artifact seam with edit applied, so each helper
// above says only what it changes.
func liveArtifact(f fakes, edit func(workload.Document)) fakes {
	previous := f.artifactD

	f.artifactD = func(id string) (workload.Document, error) {
		d := docOf(liveImageArtifactJSON)

		if previous != nil {
			var err error
			if d, err = previous(id); err != nil {
				return nil, err
			}
		}

		edit(d)

		return d, nil
	}

	return f
}

// agentManifest is newImage with the artifact relabelled an agent, which is
// the change that has to start a lineage of its own. The type goes at the
// artifact's top level because that is where the platform reads it.
func agentManifest() string {
	return strings.Replace(newImage(), "  name: my-app-artifact\n", "  name: my-app-artifact\n  type: agent\n", 1)
}

// sentRepository is the repository id on the create the run actually sent, ""
// when it sent none.
func sentRepository(t *testing.T, payload any) string {
	t.Helper()

	raw, ok := payload.(json.RawMessage)
	require.True(t, ok, "the create payload is the artifact block as raw JSON")

	var block map[string]any

	require.NoError(t, json.Unmarshal(raw, &block))

	id, _ := block[keyArtifactRepositoryID].(string)

	return id
}

// notFound is the answer the platform gives for a repository it will not let
// this caller write to, whether because it is gone or because it was never
// shared with them.
func notFound() error {
	return &drapi.HTTPError{StatusCode: http.StatusNotFound, Detail: "Artifact repository not found."}
}

// The point of the whole change: the version a roll mints joins the one the
// running version is already in, so the Registry shows one lineage rather than
// a new row per deploy.
func TestRun_RollJoinsTheRunningVersionsRepository(t *testing.T) {
	var tr track

	install(t, inRepository(wiredRoll(&tr)))

	_, _, err := runIn(t, newImage(), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, liveRepositoryID, sentRepository(t, tr.artifactIn))
}

// Nothing to join means nothing to send, and the platform opens one. This is
// also the shape of a live artifact from before repositories existed.
func TestRun_RollWithNoRepositoryToJoinSendsNone(t *testing.T) {
	var tr track

	install(t, wiredRoll(&tr))

	_, _, err := runIn(t, newImage(), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Empty(t, sentRepository(t, tr.artifactIn),
		"the base fixture has no repository, so there is nothing to ask for")
}

// A repository carries the type of the artifact that opened it and the
// platform refuses one that disagrees, so a service that became an agent is a
// new lineage rather than the next version of the old one.
func TestRun_RollOfADifferentArtifactTypeStartsItsOwnRepository(t *testing.T) {
	var tr track

	install(t, asType(wiredRoll(&tr), "service"))

	_, _, err := runIn(t, agentManifest(), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Empty(t, sentRepository(t, tr.artifactIn),
		"asking a service's repository to hold an agent only earns a 422")
}

// The repository id is read off the live artifact, so a user cannot correct a
// refusal of it. Falling back to a repository of its own keeps the deploy
// working; refusing would strand it on a detail nobody can see.
func TestRun_RepositoryRefusedRollsIntoANewOneInstead(t *testing.T) {
	var tr track

	f := inRepository(wiredRoll(&tr))

	var sent []string

	f.newArtifact = func(payload any) (*workload.Artifact, error) {
		sent = append(sent, sentRepository(t, payload))

		if len(sent) == 1 {
			return nil, notFound()
		}

		tr.steps = append(tr.steps, "create-artifact")

		return &workload.Artifact{ID: "art-2", Status: workload.ArtifactStatusDraft}, nil
	}

	install(t, f)

	result, stderr, err := runIn(t, newImage(), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, []string{liveRepositoryID, ""}, sent, "the retry drops the repository and keeps everything else")
	assert.Equal(t, ActionRolled, result.Action, "a refused repository still leaves a deploy to finish")
	assert.Equal(t, "art-2", result.ArtifactID)
	assert.Contains(t, stderr, "starts one of its own",
		"the lineage forks here, and a run that said nothing would leave nobody to explain the second Registry row")
}

// One retry, not a loop. A 404 with nothing to drop is the artifact itself
// being refused, and asking twice would earn the same answer twice.
func TestRun_NotFoundWithNoRepositorySentIsNotRetried(t *testing.T) {
	var tr track

	f := wiredRoll(&tr)

	calls := 0
	f.newArtifact = func(any) (*workload.Artifact, error) {
		calls++

		return nil, notFound()
	}

	install(t, f)

	_, _, err := runIn(t, newImage(), Options{NonInteractive: true})
	require.Error(t, err)
	assert.Equal(t, 1, calls)
}

// The type check before the create is a prediction, and it reads the live
// artifact's type rather than the repository's own, so it can be wrong. When
// it is, the platform answers 422 and the retry has to cover it, or a guess
// this code made turns a deploy that worked into one that fails.
func TestRun_RepositoryTypeRefusedRollsIntoANewOneInstead(t *testing.T) {
	var tr track

	f := inRepository(wiredRoll(&tr))

	var sent []string

	f.newArtifact = func(payload any) (*workload.Artifact, error) {
		sent = append(sent, sentRepository(t, payload))

		if len(sent) == 1 {
			return nil, &drapi.HTTPError{
				StatusCode: http.StatusUnprocessableEntity,
				Detail:     "Artifact repository type 'agent' does not match artifact type 'service'",
			}
		}

		return &workload.Artifact{ID: "art-2", Status: workload.ArtifactStatusDraft}, nil
	}

	install(t, f)

	result, _, err := runIn(t, newImage(), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, []string{liveRepositoryID, ""}, sent)
	assert.Equal(t, ActionRolled, result.Action, "a mispredicted type still leaves a deploy to finish")
}

// Dropping the repository cannot make a refused artifact acceptable, so a 422
// the repository had nothing to do with fails again and that answer is the one
// reported. One wasted request on a deploy that was going to fail anyway is
// the price of not having to read the platform's wording to tell them apart.
func TestRun_RepositoryFallbackDoesNotSwallowOtherFailures(t *testing.T) {
	var tr track

	f := inRepository(wiredRoll(&tr))

	calls := 0
	f.newArtifact = func(any) (*workload.Artifact, error) {
		calls++

		return nil, &drapi.HTTPError{StatusCode: http.StatusUnprocessableEntity, Detail: "spec is incomplete"}
	}

	install(t, f)

	_, _, err := runIn(t, newImage(), Options{NonInteractive: true})
	require.Error(t, err)
	assert.Equal(t, 2, calls, "tried without the repository, then reported the answer that came back")
	assert.Contains(t, err.Error(), "spec is incomplete", "the artifact's own refusal is what the user sees")
}

// A failure the API layer never turned into an HTTPError says nothing about
// the repository, so it is reported rather than retried.
func TestRun_RepositoryFallbackLeavesPlainErrorsAlone(t *testing.T) {
	var tr track

	f := inRepository(wiredRoll(&tr))

	calls := 0
	f.newArtifact = func(any) (*workload.Artifact, error) {
		calls++

		return nil, errors.New("connection reset")
	}

	install(t, f)

	_, _, err := runIn(t, newImage(), Options{NonInteractive: true})
	require.Error(t, err)
	assert.Equal(t, 1, calls)
}

// The id rides the create and nothing else.
//
// Asserted on withRepository directly, because a single run cannot show it:
// the plan is computed before anything is created, so a run-level check passes
// whether or not the compiled payload was tagged. The damage would land on the
// NEXT run, as drift the file can never satisfy. What actually prevents it is
// that withRepository leaves its input alone, so every later read of
// ArtifactPayload() is still the file's own block.
func TestWithRepository_LeavesTheCompiledBlockAlone(t *testing.T) {
	payload := json.RawMessage(`{"name":"my-app-artifact","spec":{"containerGroups":[]}}`)
	before := string(payload)

	joined, err := withRepository(payload, liveRepositoryID)
	require.NoError(t, err)

	assert.Equal(t, before, string(payload), "the block the plan reads is not the block the create sends")
	assert.Equal(t, liveRepositoryID, sentRepository(t, joined))
	assert.Empty(t, sentRepository(t, payload))
}

// builtRollInRepository is the built-project roll with the running version in
// a repository, which is the only path that reaches the leftover-draft check.
func builtRollInRepository(tr *track, draftRepositoryID string) fakes {
	f := builtRoll(tr)
	f.linked = func(string) bool { return true }
	f.project = syncedProject("art-abandoned")
	f.getArtifact = func(id string) (*workload.Artifact, error) {
		return &workload.Artifact{
			ID:                   id,
			Status:               workload.ArtifactStatusDraft,
			ArtifactRepositoryID: draftRepositoryID,
		}, nil
	}

	// The leftover says what the file says, so only the repository is left to
	// decide whether it can be reused.
	docs := artifactDocs("art-abandoned", 9000)
	f.artifactD = func(id string) (workload.Document, error) {
		d, err := docs(id)
		if err != nil {
			return nil, err
		}

		d[keyArtifactRepositoryID] = liveRepositoryID

		return d, nil
	}

	return f
}

// A draft an earlier attempt left in another lineage is not promoted into this
// one. Artifacts cannot move between repositories, so reusing it would not
// merge the two: it would swap the workload onto the draft's, silently and for
// good. The drafts this happens with are the ones released before the id was
// sent at all.
func TestRun_LeftoverDraftFromAnotherRepositoryIsNotReused(t *testing.T) {
	var tr track

	install(t, builtRollInRepository(&tr, "68c0000000000000000000ff"))

	_, stderr, err := runIn(t, builtDrift(), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Contains(t, tr.steps, "create-artifact", "a fresh version is minted rather than the stray promoted")
	assert.NotContains(t, stderr, "earlier attempt")
	assert.Equal(t, liveRepositoryID, sentRepository(t, tr.artifactIn),
		"and it is minted into the workload's own lineage")
}

// The same leftover, already in the workload's lineage, is still the version
// to roll onto. Refusing it would put back the pile of drafts per attempt that
// reuse exists to prevent.
func TestRun_LeftoverDraftInTheSameRepositoryIsStillReused(t *testing.T) {
	var tr track

	f := builtRollInRepository(&tr, liveRepositoryID)
	f.newArtifact = func(any) (*workload.Artifact, error) {
		t.Fatal("a draft in the right lineage is already the version to roll onto")

		return nil, nil
	}

	install(t, f)

	_, stderr, err := runIn(t, builtDrift(), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Contains(t, stderr, "earlier attempt")
}

// The same file against the same live pair still plans nothing. A repository
// on the live artifact is not drift, and a run that reported it as such would
// roll a new version on every deploy of something nobody changed.
func TestRun_RepositoryOnTheLiveArtifactIsNotDrift(t *testing.T) {
	install(t, inRepository(liveImage(fakes{})))

	result, stderr, err := runIn(t, boundImageManifest, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, ActionUnchanged, result.Action)
	assert.Contains(t, stderr, "Already up to date")
}

// pinned is a file with a repository id hand-written into the artifact block.
// The key is the platform's own and nothing documents it as an input, but the
// compiler passes through what it does not recognize, so it reaches the create
// unless something takes it off.
func pinned(content, repositoryID string) string {
	return strings.Replace(content, "  name: my-app-artifact\n",
		"  name: my-app-artifact\n  artifactRepositoryId: "+repositoryID+"\n", 1)
}

// The repository a version joins is the workload's own or none, and a line in
// the file does not get a say. Honouring one would join a lineage on the
// strength of a stale checkout, and would defeat the one case that has to
// start fresh: a file that changed the artifact's kind.
func TestRun_RepositoryWrittenIntoTheFileIsNotSent(t *testing.T) {
	var tr track

	install(t, wiredRoll(&tr))

	_, _, err := runIn(t, pinned(newImage(), "68c0000000000000000000ee"), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Empty(t, sentRepository(t, tr.artifactIn),
		"the base fixture's live artifact has no repository, and the file's is not one")
}

// The fallback is a create carrying no repository at all, rather than one
// carrying whatever the file said. A retry that asked for a second repository
// would be answered the same way as the first and the deploy would fail on a
// field the run itself put there.
func TestRun_RepositoryFallbackDropsTheFilesRepositoryToo(t *testing.T) {
	var tr track

	f := inRepository(wiredRoll(&tr))

	var sent []string

	f.newArtifact = func(payload any) (*workload.Artifact, error) {
		sent = append(sent, sentRepository(t, payload))

		if len(sent) == 1 {
			return nil, notFound()
		}

		return &workload.Artifact{ID: "art-2", Status: workload.ArtifactStatusDraft}, nil
	}

	install(t, f)

	_, _, err := runIn(t, pinned(newImage(), "68c0000000000000000000ee"), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, []string{liveRepositoryID, ""}, sent)
}

// agentBuiltDrift is the built project's file with the artifact relabelled,
// which is the change that leaves the running version's lineage behind.
func agentBuiltDrift() string {
	return strings.Replace(builtDrift(), "  name: gpt-oss-20b-vllm-artifact\n",
		"  name: gpt-oss-20b-vllm-artifact\n  type: agent\n", 1)
}

// builtRollLeavingTheLineage is that roll with the draft an earlier attempt
// left behind sitting in the repository the platform opened for it, which is
// where an artifact of the new kind can only be.
func builtRollLeavingTheLineage(tr *track) fakes {
	const agentRepositoryID = "68c0000000000000000000f2"

	f := builtRoll(tr)
	f.linked = func(string) bool { return true }
	f.project = syncedProject("art-abandoned")
	f.getArtifact = func(id string) (*workload.Artifact, error) {
		return &workload.Artifact{
			ID:                   id,
			Status:               workload.ArtifactStatusDraft,
			ArtifactRepositoryID: agentRepositoryID,
		}, nil
	}

	docs := artifactDocs("art-abandoned", 9000)
	f.artifactD = func(id string) (workload.Document, error) {
		d, err := docs(id)
		if err != nil {
			return nil, err
		}

		if id == "art-abandoned" {
			d[keyType], d[keyArtifactRepositoryID] = "agent", agentRepositoryID

			return d, nil
		}

		d[keyType], d[keyArtifactRepositoryID] = "service", liveRepositoryID

		return d, nil
	}

	return f
}

// A roll that changed the artifact's kind is starting a lineage of its own, so
// the draft its own last attempt minted is still the version to fill. Measured
// against the live artifact's repository it would be refused instead, and a
// build that fails after the create would leave a draft and a Registry row per
// attempt: the pile reuse exists to prevent.
func TestRun_LeftoverDraftIsReusedWhenTheRollStartsItsOwnRepository(t *testing.T) {
	var tr track

	f := builtRollLeavingTheLineage(&tr)
	f.newArtifact = func(any) (*workload.Artifact, error) {
		t.Error("the draft the last attempt left behind is the version to roll onto")

		return nil, errors.New("nothing should be created")
	}

	install(t, f)

	_, stderr, err := runIn(t, agentBuiltDrift(), Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Contains(t, stderr, "earlier attempt")
	assert.NotContains(t, tr.steps, "create-artifact")
}
