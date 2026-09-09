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
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProtonsServing(t *testing.T) {
	running := func(a string) Proton { return Proton{ArtifactID: a, Status: ProtonStatusRunning} }
	draining := func(a string) Proton { return Proton{ArtifactID: a, Status: ProtonStatusDraining} }
	active := func(a string) Proton {
		return Proton{ArtifactID: a, Status: ProtonStatusRunning, Role: ProtonRoleActive}
	}
	roll := func(a string) Serving { return Serving{ArtifactID: a, AwaitDrain: true} }
	// The default a roll uses: it names its successor, so it settles on the
	// promotion rather than on the predecessor going away.
	promote := func(a string) Serving { return Serving{ArtifactID: a, Replaced: true} }

	cases := []struct {
		name    string
		protons []Proton
		want    Serving
		serving bool
	}{
		{
			// An empty list is also the blink between one generation being
			// collected and the next being listed.
			"an empty list does not confirm an artifact nobody is running",
			nil, roll("art-2"), false,
		},
		{"an empty list settles a wait with nothing to look for", nil, Serving{}, true},
		{"the new generation alone", []Proton{running("art-2")}, roll("art-2"), true},
		{
			"the previous generation still draining",
			[]Proton{running("art-2"), draining("art-1")},
			roll("art-2"), false,
		},
		{
			// Measured at t+27s: both generations running, endpoint still on
			// the old build. Draining is not the only way a predecessor serves.
			"the previous generation still running",
			[]Proton{active("art-2"), running("art-1")},
			roll("art-2"), false,
		},
		{
			// The endpoint has already moved on by the time an outgoing
			// generation reaches stopping, and it sits there long after.
			"a previous generation past draining does not hold the wait",
			[]Proton{running("art-2"), {ArtifactID: "art-1", Status: "stopping"}},
			roll("art-2"), true,
		},
		{
			"drained away with nothing to replace it",
			[]Proton{{ArtifactID: "art-1", Status: "stopping"}},
			roll("art-2"), false,
		},
		{
			"a resize waits out a drain it cannot tell apart by artifact",
			[]Proton{running("art-1"), draining("art-1")},
			Serving{AwaitDrain: true},
			false,
		},
		{
			// A create and a start replace no generation, so a leftover drain
			// from an earlier rollout must not hold them up.
			"a create ignores a draining generation entirely",
			[]Proton{running("art-1"), draining("art-1")},
			Serving{},
			true,
		},
		{
			// A candidate runs for a while before it is promoted.
			"a running candidate is not a promoted one",
			[]Proton{active("art-1"), running("art-2")},
			roll("art-2"), false,
		},
		{
			"promoted, and the role says so",
			[]Proton{active("art-2"), {ArtifactID: "art-1", Status: "stopping"}},
			roll("art-2"), true,
		},
		{
			// The whole of the saving: the same list that holds the strict
			// wait open for another seven minutes settles this one.
			"promotion settles a roll that does not wait the drain",
			[]Proton{active("art-2"), draining("art-1")},
			promote("art-2"), true,
		},
		{
			// A candidate runs for a while before it is promoted, so this is
			// the wait doing its job rather than an early exit.
			"an unpromoted candidate holds it even without the drain wait",
			[]Proton{active("art-1"), running("art-2")},
			promote("art-2"), false,
		},
		{
			// The one branch where dropping the drain wait would settle a
			// roll mid-swap: with no role reported, "something runs the
			// version" is equally true of a candidate nobody promoted.
			"with no role reported the predecessor still has to be gone",
			[]Proton{running("art-1"), running("art-2")},
			promote("art-2"), false,
		},
		{
			"with no role reported a draining predecessor still holds it",
			[]Proton{draining("art-1"), running("art-2")},
			promote("art-2"), false,
		},
		{
			"with no role reported a predecessor past draining settles it",
			[]Proton{{ArtifactID: "art-1", Status: "stopping"}, running("art-2")},
			promote("art-2"), true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.serving, protonsServing(c.protons, c.want).Serving)
		})
	}
}

func TestListProtons_ParsesTheList(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/workloads/wl-1/protons/", r.URL.Path)
		fmt.Fprint(w, serverProtonList(
			[2]string{"art-2", ProtonStatusRunning},
			[2]string{"art-1", ProtonStatusDraining},
		))
	}))

	protons, err := ListProtons("wl-1")
	require.NoError(t, err)
	require.Len(t, protons, 2)
	assert.Equal(t, "art-2", protons[0].ArtifactID)
	assert.Equal(t, ProtonStatusDraining, protons[1].Status)
	assert.Equal(t, "wl-1", protons[0].WorkloadID)
}

// The generation being replaced is the oldest, so a page-one-only read can omit
// exactly the draining proton the wait exists to see.
func TestListProtons_FollowsNextLinks(t *testing.T) {
	var srv *httptest.Server

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/workloads/wl-1/protons/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			fmt.Fprint(w, serverProtonList([2]string{"art-1", ProtonStatusDraining}))

			return
		}

		fmt.Fprintf(w, `{"count":1,"totalCount":2,"next":%q,"data":[%s]}`,
			srv.URL+"/api/v2/workloads/wl-1/protons/?page=2",
			`{"id":"p1","artifactId":"art-2","status":"running"}`)
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	installSkipAuth(t)
	installEndpoint(t, srv.URL)

	protons, err := ListProtons("wl-1")
	require.NoError(t, err)
	require.Len(t, protons, 2)
	assert.Equal(t, ProtonStatusDraining, protons[1].Status, "the second page carries the outgoing generation")
}

// Decoding into protonList succeeds for any JSON object, so a route answering a
// different shape would read as "no generations" and settle the wait silently.
func TestListProtons_RefusesABodyWithNoList(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"protons":[{"id":"p1"}]}`)
	}))

	_, err := ListProtons("wl-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no data field")
}

// This walk runs inside a poll predicate, which sits downstream of the deadline
// check, so a cursor that points at itself would spin past --poll-timeout with
// no way out but Ctrl-C.
func TestListProtons_RefusesAnEndlessCursor(t *testing.T) {
	var srv *httptest.Server

	var hits int32

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/workloads/wl-1/protons/", func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		fmt.Fprintf(w, `{"count":1,"totalCount":9,"next":%q,"data":[%s]}`,
			srv.URL+"/api/v2/workloads/wl-1/protons/",
			`{"id":"p1","artifactId":"art-2","status":"running"}`)
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	installSkipAuth(t)
	installEndpoint(t, srv.URL)

	_, err := ListProtons("wl-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not end")
	assert.LessOrEqual(t, atomic.LoadInt32(&hits), int32(maxProtonPages))
}

// The id is what lets a deploy put its endpoint check to the generation it
// promoted. Without it the check asks the workload, and the platform's router
// answers from the version being replaced for minutes after the promotion.
func TestProtonsServing_NamesTheGenerationThatSettledIt(t *testing.T) {
	verdict := protonsServing([]Proton{
		{ID: "p-2", ArtifactID: "art-2", Status: ProtonStatusRunning, Role: ProtonRoleActive},
		{ID: "p-1", ArtifactID: "art-1", Status: ProtonStatusDraining},
	}, Serving{ArtifactID: "art-2", Replaced: true})

	assert.True(t, verdict.Serving)
	assert.Equal(t, "p-2", verdict.ProtonID)
}

// An install that marks no generation active is not one to assume can pin a
// request to one either, so it settles without naming one and the check that
// follows asks the endpoint the ordinary way.
func TestProtonsServing_NamesNoGenerationWhereThePlatformMarksNone(t *testing.T) {
	verdict := protonsServing([]Proton{
		{ID: "p-2", ArtifactID: "art-2", Status: ProtonStatusRunning},
	}, Serving{ArtifactID: "art-2", Replaced: true})

	assert.True(t, verdict.Serving)
	assert.Empty(t, verdict.ProtonID)
}

// AwaitDrain implies a generation was replaced: there is nothing to drain
// otherwise, so a caller that sets only the strict flag still gets the strict
// wait rather than one that skips the proton list altogether.
func TestServing_AwaitDrainImpliesAReplacedGeneration(t *testing.T) {
	assert.True(t, Serving{AwaitDrain: true}.ReplacedGeneration())
	assert.True(t, Serving{Replaced: true}.ReplacedGeneration())
	assert.False(t, Serving{}.ReplacedGeneration())
}
