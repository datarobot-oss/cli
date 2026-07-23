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
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/datarobot/cli/internal/workload/wlconfig"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validArtifactID is a dr_id-shaped id accepted by wapi.Initialize.
const validArtifactID = "6a61e855fc6f074dbf0283f9"

// fakes carries the seam overrides a test wants; nil fields fall back to safe
// stubs so an unexpected call is caught rather than hitting the network.
type fakes struct {
	getArtifact     func(string) (*workload.Artifact, error)
	createArtifact  func(any) (*workload.Artifact, error)
	resolveEE       func(string) (string, string, error)
	initProject     func(string, wapi.InitOptions) error
	runSync         func(string) (*sync.Result, error)
	triggerBuild    func(string) (*workload.BuildTriggerResponse, error)
	waitForBuild    func(string, string, time.Duration, time.Duration, func(*workload.Build)) (*workload.Build, error)
	lockArtifact    func(string) (*workload.Artifact, error)
	patchContainer  func(string, map[string]any) error
	createWorkload  func(any) (*workload.Workload, error)
	getWorkload     func(string) (*workload.Workload, error)
	listWorkloads   func(int, []string) ([]workload.Workload, error)
	startWorkload   func(string) (*workload.WorkloadOperationResponse, error)
	waitForWorkload func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error)
}

func installFakes(t *testing.T, f fakes) {
	t.Helper()

	origGetArt := getArtifactFn
	origCreateArt := createArtifactFn
	origResolveEE := resolveEEFn
	origInit := initProjectFn
	origSync := runSyncFn
	origTrigger := triggerBuildFn
	origWaitBuild := waitForBuildFn
	origLock := lockArtifactFn
	origPatch := patchContainerFn
	origCreate := createWorkloadFn
	origGet := getWorkloadFn
	origList := listWorkloadsFn
	origStart := startWorkloadFn
	origWaitWL := waitForWorkloadFn

	t.Cleanup(func() {
		getArtifactFn = origGetArt
		createArtifactFn = origCreateArt
		resolveEEFn = origResolveEE
		initProjectFn = origInit
		runSyncFn = origSync
		triggerBuildFn = origTrigger
		waitForBuildFn = origWaitBuild
		lockArtifactFn = origLock
		patchContainerFn = origPatch
		createWorkloadFn = origCreate
		getWorkloadFn = origGet
		listWorkloadsFn = origList
		startWorkloadFn = origStart
		waitForWorkloadFn = origWaitWL
	})

	fail := func(name string) { t.Errorf("unexpected call to %s", name) }

	getArtifactFn = fnOr(f.getArtifact, func(string) (*workload.Artifact, error) { return &workload.Artifact{}, nil })
	createArtifactFn = fnOr(f.createArtifact, func(any) (*workload.Artifact, error) {
		return &workload.Artifact{ID: "art-created"}, nil
	})
	resolveEEFn = fnOr(f.resolveEE, func(string) (string, string, error) { return "ee-1", "eev-1", nil })
	initProjectFn = fnOr(f.initProject, func(string, wapi.InitOptions) error { return nil })
	runSyncFn = fnOr(f.runSync, func(string) (*sync.Result, error) { return &sync.Result{}, nil })
	triggerBuildFn = fnOr(f.triggerBuild, func(string) (*workload.BuildTriggerResponse, error) {
		return &workload.BuildTriggerResponse{BuildIDs: []string{"b-1"}}, nil
	})
	waitForBuildFn = fnOr(f.waitForBuild, func(string, string, time.Duration, time.Duration, func(*workload.Build)) (*workload.Build, error) {
		return &workload.Build{ID: "b-1", Status: workload.BuildStatusCompleted}, nil
	})
	lockArtifactFn = fnOr(f.lockArtifact, func(string) (*workload.Artifact, error) { fail("lockArtifact"); return nil, nil })
	patchContainerFn = fnOr(f.patchContainer, func(string, map[string]any) error { return nil })
	createWorkloadFn = fnOr(f.createWorkload, func(any) (*workload.Workload, error) { fail("createWorkload"); return nil, nil })
	getWorkloadFn = fnOr(f.getWorkload, func(string) (*workload.Workload, error) { fail("getWorkload"); return nil, nil })
	listWorkloadsFn = fnOr(f.listWorkloads, func(int, []string) ([]workload.Workload, error) { fail("listWorkloads"); return nil, nil })
	startWorkloadFn = fnOr(f.startWorkload, func(string) (*workload.WorkloadOperationResponse, error) { fail("startWorkload"); return nil, nil })
	waitForWorkloadFn = fnOr(f.waitForWorkload, func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
		fail("waitForWorkload")

		return nil, nil
	})
}

// fnOr returns override when it is a non-nil func value, otherwise def.
func fnOr[T any](override, def T) T {
	if v := reflect.ValueOf(override); v.Kind() == reflect.Func && v.IsNil() {
		return def
	}

	return override
}

func newTestCmd(args []string) (*cobra.Command, *bytes.Buffer) {
	cmd := Cmd()
	cmd.PreRunE = nil

	var out bytes.Buffer

	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)

	return cmd, &out
}

func newTestCmdWithErr(args []string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	cmd := Cmd()
	cmd.PreRunE = nil

	var out, errBuf bytes.Buffer

	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)

	return cmd, &out, &errBuf
}

// linkDir writes a real .wapi/ so ensureLinked takes the already-linked path.
func linkDir(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, wapi.Initialize(dir, wapi.InitOptions{ArtifactID: validArtifactID}))
}

func TestUp_MissingConfigErrors(t *testing.T) {
	dir := t.TempDir()

	installFakes(t, fakes{})

	cmd, _ := newTestCmd([]string{"--yes", "--dir", dir})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dr workload config")
}

func TestUp_CreatesArtifactLinksAndDeploys(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, wlconfig.Save(dir, wlconfig.Default("my-app")))

	var (
		artifactSpec  map[string]any
		linked        bool
		createPayload map[string]any
	)

	installFakes(t, fakes{
		createArtifact: func(spec any) (*workload.Artifact, error) {
			artifactSpec, _ = spec.(map[string]any)

			return &workload.Artifact{ID: "art-new"}, nil
		},
		initProject: func(string, wapi.InitOptions) error {
			linked = true

			return nil
		},
		createWorkload: func(payload any) (*workload.Workload, error) {
			createPayload, _ = payload.(map[string]any)

			return &workload.Workload{ID: "wl-1", Status: workload.WorkloadStatusSubmitted, Endpoint: "https://e/"}, nil
		},
		waitForWorkload: func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			return &workload.Workload{ID: "wl-1", Status: workload.WorkloadStatusRunning, Endpoint: "https://e/"}, nil
		},
	})

	cmd, out := newTestCmd([]string{"--yes", "--dir", dir})
	require.NoError(t, cmd.Execute())

	assert.NotNil(t, artifactSpec, "up must create the artifact when not linked")
	assert.True(t, linked, "up must link the project directory")
	assert.NotNil(t, createPayload["runtime"], "workload create must include a runtime resource signal")
	assert.Equal(t, "https://e/", strings.TrimSpace(out.String()))

	cfg, err := wlconfig.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "wl-1", cfg.WorkloadID, "id is recorded for idempotent re-runs")
}

func TestBuildArtifactSpec_ProvidedDockerfile(t *testing.T) {
	cfg := wlconfig.Default("app") // provided mode by default

	spec, err := buildArtifactSpec(&cfg)
	require.NoError(t, err)

	df := dockerfileOf(t, spec)
	assert.Equal(t, "provided", df["source"])
	assert.Equal(t, wlconfig.DefaultDockerfile, df["path"])
}

func TestBuildArtifactSpec_GeneratedResolvesEE(t *testing.T) {
	installFakes(t, fakes{
		resolveEE: func(name string) (string, string, error) {
			assert.Equal(t, "my-ee", name)

			return "ee-9", "eev-9", nil
		},
	})

	cfg := wlconfig.Config{
		Name:  "app",
		Build: &wlconfig.Build{ExecutionEnvironment: "my-ee", Entrypoint: []string{"uvicorn", "app:app"}, Port: 8080, Health: "/h"},
	}

	spec, err := buildArtifactSpec(&cfg)
	require.NoError(t, err)

	df := dockerfileOf(t, spec)
	assert.Equal(t, "generated", df["source"])
	assert.Equal(t, "ee-9", df["executionEnvironmentId"])
	assert.Equal(t, "eev-9", df["executionEnvironmentVersionId"])
}

// dockerfileOf digs the imageBuildConfig.dockerfile map out of an artifact spec.
func dockerfileOf(t *testing.T, spec map[string]any) map[string]any {
	t.Helper()

	s := spec["spec"].(map[string]any)
	groups := s["containerGroups"].([]any)
	group := groups[0].(map[string]any)
	container := group["containers"].([]any)[0].(map[string]any)
	ibc := container["imageBuildConfig"].(map[string]any)

	return ibc["dockerfile"].(map[string]any)
}

func TestBuildCreatePayload_UsesManifestRuntime(t *testing.T) {
	cfg := &wlconfig.Config{Runtime: &wlconfig.Runtime{Replicas: 3, CPU: 2, Memory: "1GB"}}

	payload := buildCreatePayload("app", "art-1", nil, cfg)

	runtime := payload["runtime"].(map[string]any)
	group := runtime["containerGroups"].([]any)[0].(map[string]any)
	assert.Equal(t, 3, group["replicaCount"])

	container := group["containers"].([]any)[0].(map[string]any)
	ra := container["resourceAllocation"].(map[string]any)
	assert.InDelta(t, 2.0, ra["cpu"], 0.0001)
	assert.Equal(t, "1GB", ra["memory"])
}

func TestUp_LockedArtifactFailsFast(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, wlconfig.Save(dir, wlconfig.Default("app")))
	linkDir(t, dir)

	installFakes(t, fakes{
		getArtifact: func(string) (*workload.Artifact, error) {
			return &workload.Artifact{Status: workload.ArtifactStatusLocked}, nil
		},
		runSync: func(string) (*sync.Result, error) {
			t.Error("sync should not run on a locked artifact")

			return nil, errors.New("unexpected")
		},
	})

	cmd, _ := newTestCmd([]string{"--yes", "--dir", dir})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "locked")
}

func TestUp_LockFlagLocksAfterDeploy(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, wlconfig.Save(dir, wlconfig.Default("app")))

	locked := false

	installFakes(t, fakes{
		createArtifact: func(any) (*workload.Artifact, error) { return &workload.Artifact{ID: "art-x"}, nil },
		lockArtifact: func(id string) (*workload.Artifact, error) {
			locked = true

			assert.Equal(t, "art-x", id)

			return &workload.Artifact{ID: id}, nil
		},
		createWorkload: func(any) (*workload.Workload, error) {
			return &workload.Workload{ID: "wl-1", Endpoint: "https://e/"}, nil
		},
		waitForWorkload: func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			return &workload.Workload{ID: "wl-1", Status: workload.WorkloadStatusRunning, Endpoint: "https://e/"}, nil
		},
	})

	cmd, _ := newTestCmd([]string{"--yes", "--dir", dir, "--lock"})
	require.NoError(t, cmd.Execute())
	assert.True(t, locked, "--lock must call LockArtifact")
}

func TestUp_DetachErroredWorkloadExitsNonZero(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, wlconfig.Save(dir, wlconfig.Config{WorkloadID: "wl-9", Name: "app"}))
	linkDir(t, dir)

	installFakes(t, fakes{
		getWorkload: func(string) (*workload.Workload, error) {
			return &workload.Workload{ID: "wl-9", Status: workload.WorkloadStatusErrored, Endpoint: "https://e/"}, nil
		},
		startWorkload: func(string) (*workload.WorkloadOperationResponse, error) {
			return &workload.WorkloadOperationResponse{}, nil
		},
	})

	cmd, _ := newTestCmd([]string{"--yes", "--dir", dir, "--detach"})
	err := cmd.Execute()
	require.Error(t, err, "an errored workload must not exit 0 even with --detach")
	assert.Contains(t, err.Error(), "failed state")
}

func TestUp_DeletedWorkloadGivesReconfigureHint(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, wlconfig.Save(dir, wlconfig.Config{WorkloadID: "wl-gone", Name: "app"}))
	linkDir(t, dir)

	installFakes(t, fakes{
		getWorkload: func(string) (*workload.Workload, error) {
			return nil, &drapi.HTTPError{StatusCode: http.StatusNotFound}
		},
	})

	cmd, _ := newTestCmd([]string{"--yes", "--dir", dir})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no longer exists")
	assert.Contains(t, err.Error(), "dr workload config")
}

func TestUp_Create409AdoptsExistingWorkloadForArtifact(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, wlconfig.Save(dir, wlconfig.Default("my-app")))
	linkDir(t, dir) // .wapi pins validArtifactID; manifest has no workloadId

	installFakes(t, fakes{
		createWorkload: func(any) (*workload.Workload, error) {
			// The server's one-workload-per-draft-artifact rule.
			return nil, &drapi.HTTPError{StatusCode: http.StatusConflict}
		},
		listWorkloads: func(int, []string) ([]workload.Workload, error) {
			return []workload.Workload{
				{ID: "wl-other", ArtifactID: "someone-elses-artifact"},
				{ID: "wl-mine", Name: "my-app", ArtifactID: validArtifactID, Status: workload.WorkloadStatusRunning, Endpoint: "https://e/"},
			}, nil
		},
		waitForWorkload: func(id string, _, _ time.Duration, _ func(*workload.Workload)) (*workload.Workload, error) {
			assert.Equal(t, "wl-mine", id)

			return &workload.Workload{ID: "wl-mine", Status: workload.WorkloadStatusRunning, Endpoint: "https://e/"}, nil
		},
	})

	cmd, out := newTestCmd([]string{"--yes", "--dir", dir})
	require.NoError(t, cmd.Execute(), "409 must self-heal by adopting the artifact's existing workload")
	assert.Equal(t, "https://e/", strings.TrimSpace(out.String()))

	cfg, err := wlconfig.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "wl-mine", cfg.WorkloadID, "adopted id is recorded for the next run")
}

func TestUp_Create409WithNoMatchingWorkloadKeepsError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, wlconfig.Save(dir, wlconfig.Default("my-app")))
	linkDir(t, dir)

	installFakes(t, fakes{
		createWorkload: func(any) (*workload.Workload, error) {
			return nil, &drapi.HTTPError{StatusCode: http.StatusConflict}
		},
		listWorkloads: func(int, []string) ([]workload.Workload, error) {
			return []workload.Workload{{ID: "wl-other", ArtifactID: "someone-elses-artifact"}}, nil
		},
	})

	cmd, _ := newTestCmd([]string{"--yes", "--dir", dir})
	err := cmd.Execute()
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusConflict, httpErr.StatusCode)
}

func imageManifest(name string) wlconfig.Config {
	return wlconfig.Config{
		Name:       name,
		Importance: "high",
		Build:      &wlconfig.Build{Image: "repo/app:1", Entrypoint: []string{"python", "server.py"}, Port: 8080, Health: "/readyz"},
		Runtime:    &wlconfig.Runtime{Replicas: 1, CPU: 1, Memory: "512MB"},
		Env:        map[string]string{"MODEL": "azure/gpt-5-nano-2025-08-07"},
	}
}

func TestUp_ImageModeSkipsLinkSyncAndBuild(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, wlconfig.Save(dir, imageManifest("agent-service")))

	var payload map[string]any

	installFakes(t, fakes{
		// Everything code-related left as failing stubs via explicit overrides:
		createArtifact: func(any) (*workload.Artifact, error) {
			t.Error("image mode must not create a standalone artifact")

			return nil, errors.New("unexpected")
		},
		initProject: func(string, wapi.InitOptions) error {
			t.Error("image mode must not link the project")

			return errors.New("unexpected")
		},
		runSync: func(string) (*sync.Result, error) {
			t.Error("image mode must not sync")

			return nil, errors.New("unexpected")
		},
		triggerBuild: func(string) (*workload.BuildTriggerResponse, error) {
			t.Error("image mode must not build")

			return nil, errors.New("unexpected")
		},
		createWorkload: func(p any) (*workload.Workload, error) {
			payload, _ = p.(map[string]any)

			return &workload.Workload{ID: "wl-img", ArtifactID: "art-inline", Status: workload.WorkloadStatusSubmitted, Endpoint: "https://e/"}, nil
		},
		waitForWorkload: func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			return &workload.Workload{ID: "wl-img", Status: workload.WorkloadStatusRunning, Endpoint: "https://e/"}, nil
		},
	})

	cmd, out := newTestCmd([]string{"--yes", "--dir", dir})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, "https://e/", strings.TrimSpace(out.String()))

	// Inline artifact, not artifactId (Tutorial 2 shape).
	require.NotNil(t, payload)
	assert.NotContains(t, payload, "artifactId")
	assert.Equal(t, "high", payload["importance"])

	art := payload["artifact"].(map[string]any)
	container := art["spec"].(map[string]any)["containerGroups"].([]any)[0].(map[string]any)["containers"].([]any)[0].(map[string]any)
	assert.Equal(t, "repo/app:1", container["imageUri"])
	assert.Equal(t, []string{"python", "server.py"}, container["entrypoint"])
	assert.NotNil(t, container["environmentVars"])
	assert.NotNil(t, payload["runtime"], "resource signal is still required")

	cfg, err := wlconfig.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "wl-img", cfg.WorkloadID)
}

func TestUp_ImageModeLockLocksInlineArtifact(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, wlconfig.Save(dir, imageManifest("agent-service")))

	locked := ""

	installFakes(t, fakes{
		createWorkload: func(any) (*workload.Workload, error) {
			return &workload.Workload{ID: "wl-img", ArtifactID: "art-inline", Endpoint: "https://e/"}, nil
		},
		lockArtifact: func(id string) (*workload.Artifact, error) {
			locked = id

			return &workload.Artifact{ID: id}, nil
		},
		waitForWorkload: func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			return &workload.Workload{ID: "wl-img", Status: workload.WorkloadStatusRunning, Endpoint: "https://e/"}, nil
		},
	})

	cmd, _ := newTestCmd([]string{"--yes", "--dir", dir, "--lock"})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, "art-inline", locked, "--lock must lock the artifact minted by the inline create")
}

func TestEnvVarsPayload_ExpandsShellVars(t *testing.T) {
	t.Setenv("UP_TEST_SECRET", "sekret")

	cfg := &wlconfig.Config{Env: map[string]string{
		"B_TOKEN": "${UP_TEST_SECRET}",
		"A_PLAIN": "value",
	}}

	env := envVarsPayload(cfg)
	require.Len(t, env, 2)
	assert.Equal(t, map[string]any{"name": "A_PLAIN", "value": "value"}, env[0])
	assert.Equal(t, map[string]any{"name": "B_TOKEN", "value": "sekret"}, env[1])
}

func TestUp_ReportsAutoResolvedSyncConflicts(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, wlconfig.Save(dir, wlconfig.Default("app")))

	installFakes(t, fakes{
		createArtifact: func(any) (*workload.Artifact, error) { return &workload.Artifact{ID: "art-x"}, nil },
		runSync: func(string) (*sync.Result, error) {
			return &sync.Result{ConflictCount: 1, ConflictCopies: []string{"main.py.LOCAL.123"}}, nil
		},
		createWorkload: func(any) (*workload.Workload, error) {
			return &workload.Workload{ID: "wl-1", Endpoint: "https://e/"}, nil
		},
		waitForWorkload: func(string, time.Duration, time.Duration, func(*workload.Workload)) (*workload.Workload, error) {
			return &workload.Workload{ID: "wl-1", Status: workload.WorkloadStatusRunning, Endpoint: "https://e/"}, nil
		},
	})

	cmd, _, errBuf := newTestCmdWithErr([]string{"--yes", "--dir", dir})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, errBuf.String(), "auto-resolved")
	assert.Contains(t, errBuf.String(), "main.py.LOCAL.123")
}
