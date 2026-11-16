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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/workload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	useCaseA = "68b0aa11bb22cc33dd44ee55"
	useCaseB = "68b0aa11bb22cc33dd44ee66"
)

// boundUpToDate is the bound manifest whose plan is otherwise empty.
func boundUpToDate() string {
	return "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundLiveManifest
}

// liveFakes is the running, matching workload the bound fixtures deploy onto.
func liveFakes(t *testing.T) fakes {
	t.Helper()

	return fakes{
		workloadD: func(string) (workload.Document, error) { return doc(t, liveWorkloadJSON), nil },
		artifactD: func(string) (workload.Document, error) { return doc(t, liveArtifactJSON), nil },
	}
}

// createdPayload decodes what a create was sent.
func createdPayload(t *testing.T, payload any) map[string]any {
	t.Helper()

	raw, ok := payload.(json.RawMessage)
	require.True(t, ok, "the create is sent the compiled payload")

	var out map[string]any

	require.NoError(t, json.Unmarshal(raw, &out))

	return out
}

func TestRun_UseCaseFlagGoesInTheCreatePayload(t *testing.T) {
	var payload any

	install(t, fakes{
		create: func(p any) (*workload.Workload, error) {
			payload = p

			return running("wl-new"), nil
		},
		wait: func(context.Context, string, workload.Serving, time.Duration, time.Duration,
			func(*workload.Workload),
		) (*workload.Workload, error) {
			return running("wl-new"), nil
		},
	})

	result, stderr, err := runIn(t, unboundImageManifest, Options{NonInteractive: true, UseCaseID: useCaseA})
	require.NoError(t, err)

	assert.Equal(t, useCaseA, createdPayload(t, payload)["useCaseId"])
	assert.Equal(t, ActionCreated, result.Action)
	assert.Contains(t, stderr, "linked to "+useCaseA+" when the workload is created")

	planJSON := result.Plan.JSON()
	require.NotNil(t, planJSON.UseCase)
	assert.Equal(t, useCaseA, planJSON.UseCase.ID)
	assert.True(t, planJSON.UseCase.Link)
}

func TestRun_UseCaseFlagGoesInTheBuiltCreatePayload(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM scratch\n"), 0o600))

	loaded, err := LoadRendered([]byte(unboundDockerfileManifest), filepath.Join(dir, ".datarobot.yaml"))
	require.NoError(t, err)

	withID, desired, err := withUseCase(loaded, useCaseA)
	require.NoError(t, err)
	assert.Equal(t, useCaseA, desired)

	// The build path sends BindArtifact's payload, which is read from the
	// same compiled payload.
	bound, err := withID.Compiled.BindArtifact("art-9")
	require.NoError(t, err)

	var sent map[string]any

	require.NoError(t, json.Unmarshal(bound, &sent))
	assert.Equal(t, useCaseA, sent["useCaseId"])
	assert.Equal(t, "art-9", sent["artifactId"])

	var original map[string]any

	require.NoError(t, json.Unmarshal(loaded.Compiled.Payload, &original))
	assert.NotContains(t, original, "useCaseId", "the caller's payload is not edited in place")
}

func TestRun_UseCaseFlagThatDisagreesWithTheFileIsRefusedBeforeAnyRequest(t *testing.T) {
	install(t, fakes{})

	manifest := "useCaseId: " + useCaseB + "\n" + unboundImageManifest

	_, _, err := runIn(t, manifest, Options{NonInteractive: true, UseCaseID: useCaseA})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already sets useCaseId to "+useCaseB)
}

func TestRun_UseCaseFlagThatAgreesWithTheFileIsAccepted(t *testing.T) {
	loaded, err := LoadRendered([]byte("useCaseId: "+useCaseA+"\n"+unboundImageManifest),
		t.TempDir()+"/.datarobot.yaml")
	require.NoError(t, err)

	_, desired, err := withUseCase(loaded, useCaseA)
	require.NoError(t, err)
	assert.Equal(t, useCaseA, desired)
}

func TestRun_InvalidUseCaseIDIsRefused(t *testing.T) {
	install(t, fakes{})

	_, _, err := runIn(t, unboundImageManifest, Options{NonInteractive: true, UseCaseID: "not-an-id"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --use-case-id")
}

func TestRun_ExistingWorkloadIsLinkedWithoutBuildingOrRolling(t *testing.T) {
	var linked []string

	f := liveFakes(t)
	f.useCases = func(string) (workload.UseCaseLinks, error) { return workload.UseCaseLinks{}, nil }
	f.linkUC = func(uc, id string) error {
		linked = append(linked, uc+"/"+id)

		return nil
	}

	install(t, f)

	result, stderr, err := runIn(t, boundUpToDate(), Options{NonInteractive: true, UseCaseID: useCaseA})
	require.NoError(t, err)

	assert.Equal(t, []string{useCaseA + "/68b0c1d2e3f4a5b6c7d8e9f0"}, linked)
	assert.Equal(t, ActionLinked, result.Action)
	assert.Equal(t, ActionLinked, result.Plan.Action())
	assert.Contains(t, stderr, "linked to "+useCaseA+"; nothing is built or rolled for it")
	assert.NotContains(t, stderr, "Already up to date")
}

func TestRun_AlreadyLinkedIsUpToDate(t *testing.T) {
	f := liveFakes(t)
	f.useCases = func(string) (workload.UseCaseLinks, error) {
		return workload.UseCaseLinks{IDs: []string{useCaseA}}, nil
	}

	install(t, f)

	result, stderr, err := runIn(t, boundUpToDate(), Options{NonInteractive: true, UseCaseID: useCaseA})
	require.NoError(t, err)

	assert.Equal(t, ActionUnchanged, result.Action)
	assert.Contains(t, stderr, "Already up to date")
	require.NotNil(t, result.Plan.JSON().UseCase)
	assert.True(t, result.Plan.JSON().UseCase.Linked)
	assert.False(t, result.Plan.JSON().UseCase.Link)
}

func TestRun_ManifestUseCaseIsLinkedOnAnExistingWorkload(t *testing.T) {
	var linked string

	f := liveFakes(t)
	f.useCases = func(string) (workload.UseCaseLinks, error) { return workload.UseCaseLinks{}, nil }
	f.linkUC = func(uc, _ string) error {
		linked = uc

		return nil
	}

	install(t, f)

	manifest := "useCaseId: " + useCaseA + "\n" + boundUpToDate()

	result, _, err := runIn(t, manifest, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, useCaseA, linked, "the file's useCaseId is the source, not only the create's")
	assert.Equal(t, ActionLinked, result.Action)
}

func TestRun_OtherUseCasesAreUnlinkedAfterTheNewLink(t *testing.T) {
	var order []string

	f := liveFakes(t)
	f.useCases = func(string) (workload.UseCaseLinks, error) {
		return workload.UseCaseLinks{IDs: []string{useCaseB}}, nil
	}
	f.linkUC = func(uc, _ string) error {
		order = append(order, "link:"+uc)

		return nil
	}
	f.unlinkUC = func(uc, _ string) error {
		order = append(order, "unlink:"+uc)

		return nil
	}

	install(t, f)

	result, stderr, err := runIn(t, boundUpToDate(), Options{NonInteractive: true, UseCaseID: useCaseA})
	require.NoError(t, err)

	assert.Equal(t, []string{"link:" + useCaseA, "unlink:" + useCaseB}, order,
		"the workload is never left in no Use Case")
	assert.Equal(t, ActionLinked, result.Action)
	assert.Contains(t, stderr, "unlinked from "+useCaseB)
	assert.Equal(t, []string{useCaseB}, result.Plan.JSON().UseCase.Unlink)
}

func TestRun_AlreadyLinkedButAlsoElsewhereOnlyUnlinks(t *testing.T) {
	var unlinked []string

	f := liveFakes(t)
	f.useCases = func(string) (workload.UseCaseLinks, error) {
		return workload.UseCaseLinks{IDs: []string{useCaseB, useCaseA}}, nil
	}
	f.unlinkUC = func(uc, _ string) error {
		unlinked = append(unlinked, uc)

		return nil
	}

	install(t, f)

	result, _, err := runIn(t, boundUpToDate(), Options{NonInteractive: true, UseCaseID: useCaseA})
	require.NoError(t, err)

	assert.Equal(t, []string{useCaseB}, unlinked)
	assert.Equal(t, ActionLinked, result.Action)
}

func TestRun_LinkComesBeforeTheResize(t *testing.T) {
	var order []string

	f := liveFakes(t)
	f.useCases = func(string) (workload.UseCaseLinks, error) { return workload.UseCaseLinks{}, nil }
	f.linkUC = func(string, string) error {
		order = append(order, "link")

		return nil
	}
	f.settings = func(id string, _ json.RawMessage) (*workload.Replacement, error) {
		order = append(order, "settings")

		return &workload.Replacement{ID: "rep-1", WorkloadID: id}, nil
	}
	f.waitReplace = func(_ context.Context, _ string, started *workload.Replacement, _, _ time.Duration,
		_ func(*workload.Replacement),
	) (*workload.Replacement, error) {
		started.Status = workload.ReplacementStatusCompleted

		return started, nil
	}
	f.wait = func(_ context.Context, id string, _ workload.Serving, _, _ time.Duration,
		_ func(*workload.Workload),
	) (*workload.Workload, error) {
		return running(id), nil
	}

	install(t, f)

	result, _, err := runIn(t, resizedManifest(), Options{NonInteractive: true, UseCaseID: useCaseA})
	require.NoError(t, err)

	assert.Equal(t, []string{"link", "settings"}, order,
		"the platform checks a rollout against the links, so the link goes first")
	assert.Equal(t, ActionUpdated, result.Action)
}

func TestRun_DryRunReadsTheLinksAndWritesNone(t *testing.T) {
	f := liveFakes(t)
	f.useCases = func(string) (workload.UseCaseLinks, error) {
		return workload.UseCaseLinks{IDs: []string{useCaseB}}, nil
	}

	install(t, f)

	for _, diff := range []bool{false, true} {
		result, stderr, err := runIn(t, boundUpToDate(),
			Options{NonInteractive: true, DryRun: true, Diff: diff, UseCaseID: useCaseA})
		require.NoError(t, err)

		assert.Equal(t, ActionLinked, result.Action)
		assert.Contains(t, stderr, "linked to "+useCaseA)
		assert.Contains(t, stderr, "unlinked from "+useCaseB)
	}
}

func TestRun_PartialLinkListLinksAnywayAndSaysSo(t *testing.T) {
	linked := false

	f := liveFakes(t)
	f.useCases = func(string) (workload.UseCaseLinks, error) {
		return workload.UseCaseLinks{IDs: []string{useCaseB}, More: true}, nil
	}
	f.linkUC = func(string, string) error {
		linked = true

		return nil
	}
	f.unlinkUC = func(string, string) error { return nil }

	install(t, f)

	result, stderr, err := runIn(t, boundUpToDate(), Options{NonInteractive: true, UseCaseID: useCaseA})
	require.NoError(t, err)

	assert.True(t, linked)
	assert.True(t, result.Plan.JSON().UseCase.Partial)
	assert.Contains(t, stderr, "more Use Case links than one read returns")
}

func TestRun_EnclaveRefusalOnCreateNamesTheRemedy(t *testing.T) {
	install(t, fakes{
		create: func(any) (*workload.Workload, error) {
			return nil, errors.New("Use Case has Enclaves; set a selection policy (ENCLAVE_TARGETING_REQUIRED)")
		},
	})

	_, _, err := runIn(t, unboundImageManifest, Options{NonInteractive: true, UseCaseID: useCaseA})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ENCLAVE_TARGETING_REQUIRED")
	assert.Contains(t, err.Error(), "runtime.enclaveSelectionPolicy: availability")
}

func TestRun_EnclaveRefusalOnLinkNamesTheRemedy(t *testing.T) {
	f := liveFakes(t)
	f.useCases = func(string) (workload.UseCaseLinks, error) { return workload.UseCaseLinks{}, nil }
	f.linkUC = func(string, string) error {
		return errors.New("refused (ENCLAVE_TARGETING_REQUIRED)")
	}

	install(t, f)

	_, _, err := runIn(t, boundUpToDate(), Options{NonInteractive: true, UseCaseID: useCaseA})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing was deployed")
	assert.Contains(t, err.Error(), "runtime.enclaveSelectionPolicy: availability")
}

func TestRun_UnreadableLinksStopTheRun(t *testing.T) {
	f := liveFakes(t)
	f.useCases = func(string) (workload.UseCaseLinks, error) { return workload.UseCaseLinks{}, errors.New("boom") }

	install(t, f)

	_, _, err := runIn(t, boundUpToDate(), Options{NonInteractive: true, UseCaseID: useCaseA})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot read which Use Cases")
}

func TestRun_NoUseCaseReadsNoLinks(t *testing.T) {
	// install fails the test if the links are read.
	install(t, liveFakes(t))

	result, _, err := runIn(t, boundUpToDate(), Options{NonInteractive: true})
	require.NoError(t, err)
	assert.Nil(t, result.Plan.UseCase)
	assert.Nil(t, result.Plan.JSON().UseCase)
}

func TestPlan_LinkOnlyActionAndEmpty(t *testing.T) {
	plan := Plan{State: StateRunning, UseCase: &UseCasePlan{ID: useCaseA}}
	assert.False(t, plan.Empty())
	assert.Equal(t, ActionLinked, plan.Action())
	assert.False(t, plan.RollsArtifact())
	assert.False(t, plan.Retunes())

	plan.UseCase.Linked = true
	assert.True(t, plan.Empty())
	assert.Equal(t, ActionUnchanged, plan.Action())

	stopped := Plan{State: StateStopped, UseCase: &UseCasePlan{ID: useCaseA}}
	assert.Equal(t, ActionStarted, stopped.Action(), "a start outranks a link")
}

func TestRender_UseCaseLines(t *testing.T) {
	var b bytes.Buffer

	plan := Plan{State: StateRunning, UseCase: &UseCasePlan{ID: useCaseA, Unlink: []string{useCaseB}}}
	require.NoError(t, Render(&b, Summary{Name: "app", WorkloadID: "wl-1", Status: "running"}, plan))

	out := b.String()
	assert.Contains(t, out, "use case")
	assert.Contains(t, out, "linked to "+useCaseA)
	assert.Contains(t, out, "unlinked from "+useCaseB)
	assert.Equal(t, 2, strings.Count(out, "use case"))
}

// resizeFakes is the running workload with a resize that lands, recording
// each mutation in order.
func resizeFakes(t *testing.T, order *[]string, settingsErr error) fakes {
	t.Helper()

	f := liveFakes(t)
	f.useCases = func(string) (workload.UseCaseLinks, error) {
		return workload.UseCaseLinks{IDs: []string{useCaseB}}, nil
	}
	f.linkUC = func(string, string) error {
		*order = append(*order, "link")

		return nil
	}
	f.unlinkUC = func(string, string) error {
		*order = append(*order, "unlink")

		return nil
	}
	f.settings = func(id string, _ json.RawMessage) (*workload.Replacement, error) {
		*order = append(*order, "settings")

		if settingsErr != nil {
			return nil, settingsErr
		}

		return &workload.Replacement{ID: "rep-1", WorkloadID: id}, nil
	}
	f.waitReplace = func(_ context.Context, _ string, started *workload.Replacement, _, _ time.Duration,
		_ func(*workload.Replacement),
	) (*workload.Replacement, error) {
		started.Status = workload.ReplacementStatusCompleted

		return started, nil
	}
	f.wait = func(_ context.Context, id string, _ workload.Serving, _, _ time.Duration,
		_ func(*workload.Workload),
	) (*workload.Workload, error) {
		return running(id), nil
	}

	return f
}

func TestRun_OtherUseCasesAreUnlinkedOnlyOnceTheDeployLands(t *testing.T) {
	var order []string

	install(t, resizeFakes(t, &order, nil))

	_, _, err := runIn(t, resizedManifest(), Options{NonInteractive: true, UseCaseID: useCaseA})
	require.NoError(t, err)

	assert.Equal(t, []string{"link", "settings", "unlink"}, order)
}

func TestRun_AFailedDeployKeepsTheOldUseCase(t *testing.T) {
	var order []string

	install(t, resizeFakes(t, &order, errors.New("refused")))

	_, _, err := runIn(t, resizedManifest(), Options{NonInteractive: true, UseCaseID: useCaseA})
	require.Error(t, err)

	assert.Equal(t, []string{"link", "settings"}, order,
		"the workload is not moved off its Use Case by a deploy that did not land")
}

func TestRun_AFailedUnlinkAfterALinkStillReportsTheLink(t *testing.T) {
	f := liveFakes(t)
	f.useCases = func(string) (workload.UseCaseLinks, error) {
		return workload.UseCaseLinks{IDs: []string{useCaseB}}, nil
	}
	f.linkUC = func(string, string) error { return nil }
	f.unlinkUC = func(string, string) error { return errors.New("forbidden") }

	install(t, f)

	result, _, err := runIn(t, boundUpToDate(), Options{NonInteractive: true, UseCaseID: useCaseA})
	require.Error(t, err)

	assert.Equal(t, ActionLinked, result.Action, "the link went through, so the run did change something")
	assert.Contains(t, err.Error(), "could not be unlinked from "+useCaseB)
}
