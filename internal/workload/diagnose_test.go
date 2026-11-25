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

package workload

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fixtures are the platform's own answers, captured on staging: a busybox
// workload whose container exits at once and so crash-loops, and a workload
// that was stopped cleanly. The shape is theirs, not an assumed one, which is
// what makes the decoding worth testing.
const (
	crashLoopWorkloadID = "6abd2a3ec4b5e3476a311778"
	crashLoopProtonID   = "6abd2a3ec4b5e3476a311779"
	crashLoopArtifactID = "6abd2a3ec4b5e3476a311777"
)

func fixture(t *testing.T, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)

	return string(data)
}

// serveCrashLoop answers every route the diagnosis reads with the captured
// payloads.
func serveCrashLoop(t *testing.T) {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/workloads/"+crashLoopWorkloadID+"/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, serverWorkloadDocOn(crashLoopWorkloadID, "review-18958-crashloop", WorkloadStatusErrored, crashLoopArtifactID))
	})
	mux.HandleFunc("/api/v2/workloads/"+crashLoopWorkloadID+"/protons/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, fixture(t, "protons_crashloop.json"))
	})
	mux.HandleFunc("/api/v2/workloads/"+crashLoopWorkloadID+"/protons/"+crashLoopProtonID+"/statusDetails",
		func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, fixture(t, "proton_status_details_crashloop.json"))
		})
	serveAPI(t, mux)
}

// The acceptance case: a crash-looping workload reports the container's
// reason, its restart count and the exit code of the run that failed, and
// the read is not an error just because the workload is.
func TestDiagnose_CrashLoopNamesReasonRestartsAndExitCode(t *testing.T) {
	serveCrashLoop(t)

	d, err := Diagnose(crashLoopWorkloadID)
	require.NoError(t, err, "an errored workload is the answer, not a failure")

	assert.Equal(t, WorkloadStatusErrored, d.Status)
	assert.Equal(t, "review-18958-crashloop", d.Name)
	require.Len(t, d.Generations, 1)

	g := d.Generations[0]
	assert.Equal(t, crashLoopProtonID, g.ID)
	assert.Equal(t, crashLoopArtifactID, g.ArtifactID)
	assert.Equal(t, "errored", g.Status)
	assert.Equal(t, ProtonRoleActive, g.Role)

	require.NotNil(t, g.Details)
	assert.Equal(t, "errored", g.Details.OverallStatus.State)
	assert.Equal(t, "Workload is in state errored based on pod states", g.Details.OverallStatus.Summary)
	require.NotNil(t, g.Details.OverallStatus.LastUpdated)

	require.Len(t, g.Details.Replicas, 1)
	replica := g.Details.Replicas[0]
	assert.Equal(t, "lrs-"+crashLoopProtonID+"-6945b6ddd7-6mhh8", replica.Name)
	assert.Equal(t, "running", replica.Status)
	assert.Len(t, replica.Conditions, 5)

	require.Len(t, replica.Containers, 1)
	c := replica.Containers[0]
	assert.Equal(t, "waiting", c.Status)
	assert.Equal(t, "CrashLoopBackOff", c.Reason)
	assert.Equal(t, 3, c.RestartCount)
	assert.False(t, c.Ready)
	assert.Equal(t, "docker.io/library/busybox:1.36", c.Image)
	assert.Nil(t, c.ExitCode, "a waiting container has no exit code of its own")

	require.NotNil(t, c.LastState)
	assert.Equal(t, "terminated", c.LastState.Status)
	assert.Equal(t, "Completed", c.LastState.Reason)
	require.NotNil(t, c.LastState.ExitCode)
	assert.Equal(t, 0, *c.LastState.ExitCode)
	require.NotNil(t, c.LastState.FinishedAt)

	// The same sentence `dr workload up` prints beside an errored state.
	assert.Equal(t, []string{"primary: CrashLoopBackOff; last run exited 0 (Completed)"}, g.Findings)
}

// Text output carries the reason, the restarts and the exit code where a
// person looks for them, and calls the finding out.
func TestRenderDiagnosis_Text(t *testing.T) {
	serveCrashLoop(t)

	d, err := Diagnose(crashLoopWorkloadID)
	require.NoError(t, err)

	var out bytes.Buffer

	require.NoError(t, RenderDiagnosisTo(&out, outputformat.OutputFormatText, *d))

	text := out.String()
	assert.Contains(t, text, "Workload "+crashLoopWorkloadID+" (review-18958-crashloop) is")
	assert.Contains(t, text, "errored")
	assert.Contains(t, text, "Generation "+crashLoopProtonID+" · errored · active")
	assert.Contains(t, text, "artifact "+crashLoopArtifactID)
	assert.Contains(t, text, "Workload is in state errored based on pod states")

	// The table: names without the cluster prefix, the reason, the count,
	// readiness, and how the last run ended.
	assert.Contains(t, text, "6945b6ddd7-6mhh8 (running)")
	assert.NotContains(t, text, "lrs-"+crashLoopProtonID+"-primary", "the prefix says nothing and eats the width")
	assert.Contains(t, text, "primary")
	assert.Contains(t, text, "CrashLoopBackOff")
	assert.Contains(t, text, "exit 0 (Completed)")

	assert.Contains(t, text, "Findings:")
	assert.Contains(t, text, "⚠ primary: CrashLoopBackOff; last run exited 0 (Completed)")
}

// JSON output is one envelope and nothing else, in the platform's own shape,
// with findings as a list even when empty.
func TestRenderDiagnosis_JSONIsOneEnvelope(t *testing.T) {
	serveCrashLoop(t)

	d, err := Diagnose(crashLoopWorkloadID)
	require.NoError(t, err)

	var out bytes.Buffer

	require.NoError(t, RenderDiagnosisTo(&out, outputformat.OutputFormatJSON, *d))

	var envelope map[string]any

	require.NoError(t, json.Unmarshal(out.Bytes(), &envelope), "stdout must be one JSON document")

	body, ok := envelope["diagnosis"].(map[string]any)
	require.True(t, ok, "the envelope is keyed on what it carries")
	assert.Equal(t, crashLoopWorkloadID, body["workloadId"])
	assert.Equal(t, WorkloadStatusErrored, body["status"])

	generations, ok := body["generations"].([]any)
	require.True(t, ok)
	require.Len(t, generations, 1)

	g, ok := generations[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "active", g["role"])

	findings, ok := g["findings"].([]any)
	require.True(t, ok, "findings is a list, never null")
	assert.Len(t, findings, 1)

	details, ok := g["details"].(map[string]any)
	require.True(t, ok)

	overall, ok := details["overallStatus"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "errored", overall["state"])

	replicas, ok := details["replicas"].([]any)
	require.True(t, ok)
	require.Len(t, replicas, 1)

	// The platform's field names survive the round trip.
	replica, _ := replicas[0].(map[string]any)
	containers, _ := replica["containers"].([]any)
	require.Len(t, containers, 1)

	container, _ := containers[0].(map[string]any)
	assert.Equal(t, "CrashLoopBackOff", container["reason"])
	assert.InDelta(t, 3, container["restartCount"], 0)

	last, _ := container["lastState"].(map[string]any)
	assert.InDelta(t, 0, last["exitCode"], 0)
}

// A cleanly stopped workload has nothing wrong with it, and says so rather
// than printing an empty findings block.
func TestDiagnose_StoppedWorkloadReportsNothingWrong(t *testing.T) {
	const (
		workloadID = "6abbbaea720810ff290d71eb"
		protonID   = "6abbbaea720810ff290d71ec"
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/workloads/"+workloadID+"/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, serverWorkloadDoc(workloadID, "my-up-test-app", WorkloadStatusStopped))
	})
	mux.HandleFunc("/api/v2/workloads/"+workloadID+"/protons/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"count":1,"totalCount":1,"next":null,"data":[{"id":%q,"artifactId":"art-1","status":"stopped"}]}`, protonID)
	})
	mux.HandleFunc("/api/v2/workloads/"+workloadID+"/protons/"+protonID+"/statusDetails",
		func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, fixture(t, "proton_status_details_stopped.json"))
		})
	serveAPI(t, mux)

	d, err := Diagnose(workloadID)
	require.NoError(t, err)
	require.Len(t, d.Generations, 1)
	assert.Empty(t, d.Generations[0].Findings)
	assert.NotNil(t, d.Generations[0].Findings, "checked and clean is not the same as unchecked")

	var out bytes.Buffer

	require.NoError(t, RenderDiagnosisTo(&out, outputformat.OutputFormatText, *d))
	assert.Contains(t, out.String(), "No container is reporting a problem.")
	assert.Contains(t, out.String(), "exit 0 (Completed)")
	assert.NotContains(t, out.String(), "Findings:")
}

// Before the monitor has reported, the route answers 204. That is a
// generation with no snapshot, said as such, not an error and not an empty
// table.
func TestDiagnose_NoSnapshotYet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/workloads/wl-1/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, serverWorkloadDoc("wl-1", "fresh", WorkloadStatusProvisioning))
	})
	mux.HandleFunc("/api/v2/workloads/wl-1/protons/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"count":1,"totalCount":1,"next":null,"data":[{"id":"p1","artifactId":"art-1","status":"provisioning"}]}`)
	})
	mux.HandleFunc("/api/v2/workloads/wl-1/protons/p1/statusDetails", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	serveAPI(t, mux)

	d, err := Diagnose("wl-1")
	require.NoError(t, err)
	require.Len(t, d.Generations, 1)
	assert.Nil(t, d.Generations[0].Details)
	assert.NotNil(t, d.Generations[0].Findings)

	var out bytes.Buffer

	require.NoError(t, RenderDiagnosisTo(&out, outputformat.OutputFormatText, *d))
	assert.Contains(t, out.String(), "No status snapshot yet")
	assert.NotContains(t, out.String(), "REPLICA")
}

// During a rolling replacement two generations exist. The one answering the
// endpoint is listed first whatever order the platform used.
func TestDiagnose_ActiveGenerationComesFirst(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/workloads/wl-1/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, serverWorkloadDoc("wl-1", "rolling", WorkloadStatusRunning))
	})
	mux.HandleFunc("/api/v2/workloads/wl-1/protons/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"count":2,"totalCount":2,"next":null,"data":[
			{"id":"p-old","artifactId":"art-1","status":"draining"},
			{"id":"p-new","artifactId":"art-2","status":"running","role":"active"}]}`)
	})
	mux.HandleFunc("/api/v2/workloads/wl-1/protons/p-old/statusDetails", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/api/v2/workloads/wl-1/protons/p-new/statusDetails", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	serveAPI(t, mux)

	d, err := Diagnose("wl-1")
	require.NoError(t, err)
	require.Len(t, d.Generations, 2)
	assert.Equal(t, "p-new", d.Generations[0].ID)
	assert.Equal(t, "p-old", d.Generations[1].ID)
}

// A workload with no generations listed is reported as such, not as clean.
func TestDiagnose_NoGenerations(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/workloads/wl-1/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, serverWorkloadDoc("wl-1", "empty", WorkloadStatusSubmitted))
	})
	mux.HandleFunc("/api/v2/workloads/wl-1/protons/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"count":0,"totalCount":0,"next":null,"data":[]}`)
	})
	serveAPI(t, mux)

	d, err := Diagnose("wl-1")
	require.NoError(t, err)
	assert.Empty(t, d.Generations)

	var out bytes.Buffer

	require.NoError(t, RenderDiagnosisTo(&out, outputformat.OutputFormatText, *d))
	assert.Contains(t, out.String(), "lists no container generations")
}

// An install without the protons route is a platform with nothing to say,
// and the sentence is about that, not about HTTP.
func TestDiagnose_RouteAbsentIsSaidAsSuch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/workloads/wl-1/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, serverWorkloadDoc("wl-1", "x", WorkloadStatusErrored))
	})
	mux.HandleFunc("/api/v2/workloads/wl-1/protons/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	serveAPI(t, mux)

	_, err := Diagnose("wl-1")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrGenerationsUnavailable)
	assert.Contains(t, err.Error(), "dr workload get wl-1")

	// The platform's 404 is not in the chain: a caller that reads a 404 as
	// "no such workload" (the manifest-sourced id wording) must not, since
	// the workload itself was just read.
	var httpErr *drapi.HTTPError

	assert.NotErrorAs(t, err, &httpErr, "the route's 404 must not pass as the workload being missing")
}

// A read that cannot be made is an error that names which read.
func TestDiagnose_ReadFailuresAreErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/workloads/wl-1/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, serverWorkloadDoc("wl-1", "x", WorkloadStatusErrored))
	})
	mux.HandleFunc("/api/v2/workloads/wl-1/protons/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	serveAPI(t, mux)

	_, err := Diagnose("wl-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot list the container generations", err.Error())
}

// One generation's details failing to read does not sink the diagnosis when
// it is not the active one: the failure is recorded on that generation and
// the active generation's answer, which is what the user came for, stands.
// The active generation's read failing is the whole read failing.
func TestDiagnose_ADrainingGenerationsFailedReadIsANote(t *testing.T) {
	serve := func(t *testing.T, activeStatus, oldStatus int) {
		t.Helper()

		mux := http.NewServeMux()
		mux.HandleFunc("/api/v2/workloads/wl-1/", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, serverWorkloadDoc("wl-1", "x", WorkloadStatusRunning))
		})
		mux.HandleFunc("/api/v2/workloads/wl-1/protons/", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, `{"count":2,"totalCount":2,"next":null,"data":[
				{"id":"p-old","artifactId":"art-1","status":"running","role":"draining"},
				{"id":"p-new","artifactId":"art-2","status":"running","role":"active"}]}`)
		})
		mux.HandleFunc("/api/v2/workloads/wl-1/protons/p-old/statusDetails", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(oldStatus)
		})
		mux.HandleFunc("/api/v2/workloads/wl-1/protons/p-new/statusDetails", func(w http.ResponseWriter, _ *http.Request) {
			if activeStatus != http.StatusOK {
				w.WriteHeader(activeStatus)

				return
			}

			fmt.Fprint(w, fixture(t, "proton_status_details_stopped.json"))
		})
		serveAPI(t, mux)
	}

	t.Run("the draining generation", func(t *testing.T) {
		serve(t, http.StatusOK, http.StatusNotFound)

		d, err := Diagnose("wl-1")
		require.NoError(t, err)
		require.Len(t, d.Generations, 2)

		assert.Equal(t, "p-new", d.Generations[0].ID)
		assert.Empty(t, d.Generations[0].Error)
		assert.NotNil(t, d.Generations[0].Details)

		assert.Equal(t, "p-old", d.Generations[1].ID)
		assert.Contains(t, d.Generations[1].Error, "404")
		assert.Nil(t, d.Generations[1].Details)
		assert.NotNil(t, d.Generations[1].Findings, "findings stay a list under JSON")

		var out bytes.Buffer

		require.NoError(t, RenderDiagnosisTo(&out, outputformat.OutputFormatText, *d))
		assert.Contains(t, out.String(), "Status details could not be read")
	})

	t.Run("the active generation", func(t *testing.T) {
		serve(t, http.StatusForbidden, http.StatusOK)

		_, err := Diagnose("wl-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot read the status details of generation p-new")
	})
}

// The findings a snapshot yields, beyond the crash loop the fixture holds.
func TestFindings(t *testing.T) {
	one, two := 1, 2

	cases := []struct {
		name    string
		details *ProtonStatusDetails
		want    []string
	}{
		{"no snapshot", nil, []string{}},
		{
			"a pull failure carries the registry's answer",
			&ProtonStatusDetails{Replicas: []ReplicaStatus{{Containers: []ContainerStatus{{
				Name: "lrs-p1-primary", Reason: "ErrImagePull",
				Message: "rpc error: code = NotFound desc = failed to resolve image: docker.io/team/app:v9: not found",
			}}}}},
			[]string{"primary: ErrImagePull: failed to resolve image: docker.io/team/app:v9: not found"},
		},
		{
			"restarts on a container that is otherwise quiet",
			&ProtonStatusDetails{Replicas: []ReplicaStatus{{Containers: []ContainerStatus{{
				Name: "lrs-p1-primary", Status: "running", Ready: true, RestartCount: 2,
			}}}}},
			[]string{"primary: restarted 2 times"},
		},
		{
			"a healthy container says nothing",
			&ProtonStatusDetails{Replicas: []ReplicaStatus{{Containers: []ContainerStatus{{
				Name: "lrs-p1-primary", Status: "running", Ready: true,
			}}}}},
			[]string{},
		},
		{
			// On a workload that is not stopped, a process that exited zero
			// and did not come back is the reason it is not serving.
			"a completed container is named on a workload that should be running",
			&ProtonStatusDetails{Replicas: []ReplicaStatus{{Containers: []ContainerStatus{{
				Name: "lrs-p1-primary", Status: "terminated", Reason: "Completed", ExitCode: new(int),
			}}}}},
			[]string{"primary: Completed"},
		},
		{
			"a container still coming up is progress, not a fault",
			&ProtonStatusDetails{Replicas: []ReplicaStatus{{Containers: []ContainerStatus{{
				Name: "lrs-p1-primary", Status: "waiting", Reason: "ContainerCreating",
			}}}}},
			[]string{},
		},
		{
			"a container coming up again after a failed run is named",
			&ProtonStatusDetails{Replicas: []ReplicaStatus{{Containers: []ContainerStatus{{
				Name: "lrs-p1-primary", Status: "waiting", Reason: "PodInitializing",
				LastState: &ContainerState{Reason: "Error", ExitCode: &one},
			}}}}},
			[]string{"primary: PodInitializing; last run exited 1"},
		},
		{
			"a container terminated right now carries its own exit code",
			&ProtonStatusDetails{Replicas: []ReplicaStatus{{Containers: []ContainerStatus{{
				Name: "lrs-p1-primary", Status: "terminated", Reason: "Error", ExitCode: &two,
			}}}}},
			[]string{"primary: Error; exited 2"},
		},
		{
			"a replica the scheduler could not place has no containers and says why on its conditions",
			&ProtonStatusDetails{Replicas: []ReplicaStatus{{
				Name: "lrs-p1-7d9f-abcde", Status: "pending",
				Conditions: []ReplicaCondition{
					{
						Type: "PodScheduled", Value: false, Reason: "Unschedulable",
						Message: "0/3 nodes are available: 3 Insufficient nvidia.com/gpu.",
					},
					{Type: "Initialized", Value: true},
				},
			}}},
			[]string{"7d9f-abcde: Unschedulable: 0/3 nodes are available: 3 Insufficient nvidia.com/gpu."},
		},
		{
			"the pod repeating its containers is not a second finding",
			&ProtonStatusDetails{Replicas: []ReplicaStatus{{
				Name: "lrs-p1-7d9f-abcde", Status: "running",
				Conditions: []ReplicaCondition{
					{Type: "Ready", Value: false, Reason: "ContainersNotReady", Message: "containers with unready status: [x]"},
				},
				Containers: []ContainerStatus{{
					Name: "lrs-p1-primary", Reason: "CrashLoopBackOff",
					LastState: &ContainerState{Reason: "Error", ExitCode: &one},
				}},
			}}},
			[]string{"primary: CrashLoopBackOff; last run exited 1"},
		},
		{
			"every container is heard, across replicas",
			&ProtonStatusDetails{Replicas: []ReplicaStatus{
				{Containers: []ContainerStatus{{
					Name: "lrs-p1-primary", Reason: "CrashLoopBackOff",
					LastState: &ContainerState{Reason: "Error", ExitCode: &one},
				}}},
				{Containers: []ContainerStatus{{Name: "lrs-p1-sidecar", Status: "running", Ready: true, RestartCount: 1}}},
			}},
			[]string{"primary: CrashLoopBackOff; last run exited 1", "sidecar: restarted 1 time"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, findings("p1", tc.details, false))
		})
	}

	// On a stopped workload nothing is a finding: its containers ended
	// because they were told to, whether as Completed or as an Error with
	// the exit code of a process that did not trap the signal.
	stopped := &ProtonStatusDetails{Replicas: []ReplicaStatus{{Containers: []ContainerStatus{
		{Name: "lrs-p1-primary", Status: "terminated", Reason: "Completed", ExitCode: new(int)},
		{Name: "lrs-p1-sidecar", Status: "terminated", Reason: "Error", ExitCode: func() *int { c := 143; return &c }()},
	}}}}

	assert.Equal(t, []string{}, findings("p1", stopped, true))
	assert.NotEmpty(t, findings("p1", stopped, false), "the same snapshot on a workload that should be running is a finding")
}
