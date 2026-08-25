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
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// notActiveBody is what the platform answers when nothing is in flight.
	// The same 404 also means "no such workload", which is the ambiguity
	// GuardNoActiveReplacement and CancelReplacement have to resolve.
	notActiveBody = `{"detail":"There is no active replacement for this workload."}`

	replacementPath = "/api/v2/workloads/wl-1/replacement/"
	workloadPath    = "/api/v2/workloads/wl-1/"
)

// notFound writes the platform's empty-replacement answer: the 404 and the
// notActiveBody that comes with it, so a test reading the response sees the
// same pair the route sends.
func notFound(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprint(w, notActiveBody)
}

// serveReplacement answers the replacement route with handler, and the
// workload route as a workload that exists. Tests that care about a missing
// workload override the second route themselves.
func serveReplacement(t *testing.T, handler http.HandlerFunc) {
	t.Helper()

	mux := http.NewServeMux()
	mux.Handle(replacementPath, handler)
	mux.HandleFunc(workloadPath, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":"wl-1","name":"my-app","status":"running"}`)
	})

	serveAPI(t, mux)
}

func TestIsTerminalReplacementStatus(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{ReplacementStatusCompleted, true},
		{ReplacementStatusFailed, true},
		{ReplacementStatusErrored, true},
		{"submitted", false},
		{"initializing", false},
		{"candidate-warming", false},
		{"switching", false},
		{"", false},

		// The platform's own docs disagree with themselves about the casing of
		// status enums. Read case-sensitively, a settled "COMPLETED" counts as
		// still in progress, which makes the rollout guard refuse every deploy
		// for as long as the record lingers.
		{"COMPLETED", true},
		{"Failed", true},
		{"ERRORED", true},
		{"SWITCHING", false},
	}

	for _, c := range cases {
		assert.Equal(t, c.want, IsTerminalReplacementStatus(c.status), "status %q", c.status)
	}
}

func TestIsFailedReplacementStatus(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{ReplacementStatusFailed, true},
		{ReplacementStatusErrored, true},
		{ReplacementStatusCompleted, false},
		{"submitted", false},
		{"", false},
		{"FAILED", true},
		{"Errored", true},
		{"COMPLETED", false},
	}

	for _, c := range cases {
		assert.Equal(t, c.want, IsFailedReplacementStatus(c.status), "status %q", c.status)
	}
}

// TestReplacement_TimestampsMarshalPlainly guards against reintroducing
// omitempty on the time.Time fields. encoding/json never treats a struct as
// empty, so the tag does nothing and a zero timestamp would print as year 1
// the moment a renderer exists.
func TestReplacement_TimestampsMarshalPlainly(t *testing.T) {
	var r Replacement

	require.NoError(t, json.Unmarshal([]byte(`{"candidateArtifactId":"art-2","status":"submitted"}`), &r))

	encoded, err := json.Marshal(r)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"createdAt":"0001-01-01T00:00:00Z"`,
		"the zero value must be visible rather than silently omitted, matching Build and Artifact")
}

// TestGetActiveReplacement_Decodes pins the field name the platform actually
// uses. Its own example code says artifactId; the wire says
// candidateArtifactId, and reading the wrong one leaves the rollout unable to
// tell which artifact it is waiting for.
func TestGetActiveReplacement_Decodes(t *testing.T) {
	serveReplacement(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		fmt.Fprint(w, `{"id":"rep-1","workloadId":"wl-1","candidateArtifactId":"art-2","status":"submitted","strategy":"rolling"}`)
	})

	replacement, err := GetActiveReplacement("wl-1")
	require.NoError(t, err)
	require.NotNil(t, replacement)
	assert.Equal(t, "art-2", replacement.ArtifactID)
	assert.Equal(t, "submitted", replacement.Status)
}

func TestGetActiveReplacement_404ReturnsNilNil(t *testing.T) {
	serveReplacement(t, func(w http.ResponseWriter, _ *http.Request) {
		notFound(w)
	})

	replacement, err := GetActiveReplacement("wl-1")
	require.NoError(t, err)
	assert.Nil(t, replacement)
}

func TestGetActiveReplacement_OtherErrorPropagates(t *testing.T) {
	serveReplacement(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := GetActiveReplacement("wl-1")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusInternalServerError, httpErr.StatusCode)
}

// TestReplacementRoute_EscapesID keeps a pasted id from walking out of the
// replacement route. It matters more here than elsewhere because the same
// route carries all three verbs, one of which mutates.
func TestReplacementRoute_EscapesID(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/workloads/a%2Fb/replacement/", r.URL.EscapedPath())
		fmt.Fprint(w, `{"candidateArtifactId":"art-2","status":"submitted"}`)
	}))

	_, err := GetActiveReplacement("a/b")
	require.NoError(t, err)
}

func TestStartReplacement_PostsArtifactIDAndRollingStrategy(t *testing.T) {
	serveReplacement(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)

		var body map[string]any

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "art-2", body["artifactId"])
		assert.Equal(t, "rolling", body["strategy"])
		assert.NotContains(t, body, "runtime",
			"a swap that changes no sizing must not read as a settings change as well")

		fmt.Fprint(w, `{"candidateArtifactId":"art-2","status":"submitted"}`)
	})

	replacement, err := StartReplacement("wl-1", "art-2", nil)
	require.NoError(t, err)
	assert.Equal(t, "art-2", replacement.ArtifactID)
	assert.Equal(t, "submitted", replacement.Status)
}

// A deploy that changes the sizing and the version sends both, so the
// candidate comes up under the sizing it was asked for rather than under the
// one it is replacing.
func TestStartReplacement_CarriesTheRuntimeWhenThereIsOne(t *testing.T) {
	serveReplacement(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))

		runtime, ok := body["runtime"].(map[string]any)
		if assert.True(t, ok, "the runtime block travels with the swap") {
			assert.InDelta(t, 3, runtime["replicaCount"], 0)
		}

		fmt.Fprint(w, `{"candidateArtifactId":"art-2","status":"submitted"}`)
	})

	_, err := StartReplacement("wl-1", "art-2", json.RawMessage(`{"replicaCount":3}`))
	require.NoError(t, err)
}

// TestStartReplacement_ErrorPropagates covers the only call in this file that
// changes production state. A 409 here is the platform refusing the swap, for
// instance because the two artifacts disagree on draft-versus-locked status,
// and the caller has to see it rather than press on.
func TestStartReplacement_ErrorPropagates(t *testing.T) {
	serveReplacement(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `{"detail":"candidate artifact must be locked"}`)
	})

	replacement, err := StartReplacement("wl-1", "art-2", nil)
	require.Error(t, err)
	assert.Nil(t, replacement)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusConflict, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "must be locked", "the server's reason has to survive to the user")
}

func TestCancelReplacement_DeletesTheRoute(t *testing.T) {
	var called int32

	serveReplacement(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&called, 1)
		assert.Equal(t, http.MethodDelete, r.Method)
		w.WriteHeader(http.StatusNoContent)
	})

	require.NoError(t, CancelReplacement("wl-1"))
	assert.Equal(t, int32(1), atomic.LoadInt32(&called), "cancel must actually reach the server")
}

// TestCancelReplacement_AcceptsEverySuccessCode pins all three, because the
// route has never been driven live and the platform may well answer 202 for
// an asynchronous teardown rather than 204.
func TestCancelReplacement_AcceptsEverySuccessCode(t *testing.T) {
	for _, code := range []int{http.StatusOK, http.StatusAccepted, http.StatusNoContent} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			serveReplacement(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(code)

				if code != http.StatusNoContent {
					fmt.Fprint(w, `{"status":"cancelling"}`)
				}
			})

			assert.NoError(t, CancelReplacement("wl-1"))
		})
	}
}

// TestCancelReplacement_NothingToCancelIsSuccess keeps cancelling idempotent:
// a rollout that settled between the decision to cancel and the call itself
// leaves nothing to tear down, which is the outcome the caller wanted anyway.
func TestCancelReplacement_NothingToCancelIsSuccess(t *testing.T) {
	serveReplacement(t, func(w http.ResponseWriter, _ *http.Request) {
		notFound(w)
	})

	assert.NoError(t, CancelReplacement("wl-1"))
}

// TestCancelReplacement_MissingWorkloadIsAnError is the other half of the
// same 404. Reporting a clean cancel for a mistyped id would tell the user
// they stopped a rollout that is in fact still running.
func TestCancelReplacement_MissingWorkloadIsAnError(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		notFound(w)
	}))

	err := CancelReplacement("wl-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

func TestCancelReplacement_OtherErrorPropagates(t *testing.T) {
	serveReplacement(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
	})

	err := CancelReplacement("wl-1")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusConflict, httpErr.StatusCode)
}

func TestGuardNoActiveReplacement_PassesWhenNoneActive(t *testing.T) {
	serveReplacement(t, func(w http.ResponseWriter, _ *http.Request) {
		notFound(w)
	})

	assert.NoError(t, GuardNoActiveReplacement("wl-1"))
}

// TestGuardNoActiveReplacement_MissingWorkloadFailsFast is the whole reason
// the guard pays for a second request. Without it the mistyped id sails
// through the up-front check, and the command goes on to create and possibly
// lock an artifact, which cannot be undone, before finding out the target was
// never real.
func TestGuardNoActiveReplacement_MissingWorkloadFailsFast(t *testing.T) {
	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		notFound(w)
	}))

	err := GuardNoActiveReplacement("wl-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

// TestGuardNoActiveReplacement_SkipsTheExistenceCheckWhenOneIsActive: a
// replacement in flight is proof enough that the workload is there.
func TestGuardNoActiveReplacement_SkipsTheExistenceCheckWhenOneIsActive(t *testing.T) {
	var workloadFetches int32

	mux := http.NewServeMux()
	mux.HandleFunc(replacementPath, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"candidateArtifactId":"art-9","status":"switching"}`)
	})
	mux.HandleFunc(workloadPath, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&workloadFetches, 1)
		fmt.Fprint(w, `{"id":"wl-1"}`)
	})

	serveAPI(t, mux)

	err := GuardNoActiveReplacement("wl-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "art-9")
	assert.Contains(t, err.Error(), "switching")
	assert.Zero(t, atomic.LoadInt32(&workloadFetches))
}

// TestGuardNoActiveReplacement_SettledReplacementDoesNotBlock covers the gap
// between a rollout ending and the platform collecting its record.
// WaitForReplacement returns the instant it sees a terminal status, so that
// record is usually still readable when the next command looks. Treating it
// as in-flight would refuse a retry, and would say so with a message naming
// the status as "completed" while calling it in progress.
func TestGuardNoActiveReplacement_SettledReplacementDoesNotBlock(t *testing.T) {
	for _, status := range []string{
		ReplacementStatusCompleted,
		ReplacementStatusFailed,
		ReplacementStatusErrored,
	} {
		t.Run(status, func(t *testing.T) {
			serveReplacement(t, func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprintf(w, `{"candidateArtifactId":"art-2","status":"%s"}`, status)
			})

			assert.NoError(t, GuardNoActiveReplacement("wl-1"))
		})
	}
}

// TestGuardNoActiveReplacement_RefusalNamesOnlyWhatThePlatformFilledIn covers
// the settings-initiated replacement. A resize rolls the workload onto the
// artifact it is already running, so the platform returns no candidate, and a
// message that named one unconditionally printed an empty field mid-sentence.
func TestGuardNoActiveReplacement_RefusalNamesOnlyWhatThePlatformFilledIn(t *testing.T) {
	serveReplacement(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":"rep-9","workloadId":"wl-1","status":"pending"}`)
	})

	err := GuardNoActiveReplacement("wl-1")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrReplacementInFlight,
		"a refusal is a state to wait out, not a failure to find out, and callers branch on the difference")
	assert.Contains(t, err.Error(), "workload wl-1: a replacement is already in progress (status pending)")
	assert.NotContains(t, err.Error(), "to artifact", "there is no candidate artifact to name")
}

func TestGuardNoActiveReplacement_PropagatesLookupFailure(t *testing.T) {
	serveReplacement(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	err := GuardNoActiveReplacement("wl-1")
	require.Error(t, err)

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr, "a transport failure must not be reported as an in-flight replacement")
	assert.Equal(t, http.StatusInternalServerError, httpErr.StatusCode)
	assert.NotErrorIs(t, err, ErrReplacementInFlight,
		"callers wrap a failure to find out and pass a refusal through, so the two must not carry the same mark")
}

// TestRefuseActiveReplacement_AsksOnceForAKnownWorkload is the difference
// between the two entry points: a caller that has already read the workload
// does not pay for the 404 disambiguation, which is a second GET of a document
// it is holding.
func TestRefuseActiveReplacement_AsksOnceForAKnownWorkload(t *testing.T) {
	var workloadFetches int32

	mux := http.NewServeMux()
	mux.HandleFunc(replacementPath, func(w http.ResponseWriter, _ *http.Request) {
		notFound(w)
	})
	mux.HandleFunc(workloadPath, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&workloadFetches, 1)
		fmt.Fprint(w, `{"id":"wl-1"}`)
	})

	serveAPI(t, mux)

	require.NoError(t, RefuseActiveReplacement("wl-1"))
	assert.Zero(t, atomic.LoadInt32(&workloadFetches),
		"the caller established the workload exists before asking")
}

// The refusal itself is the same one, so neither entry point can drift into
// wording the other does not have.
func TestRefuseActiveReplacement_RefusesTheSameWayTheGuardDoes(t *testing.T) {
	serveReplacement(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"candidateArtifactId":"art-9","status":"switching"}`)
	})

	err := RefuseActiveReplacement("wl-1")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrReplacementInFlight)
	assert.Contains(t, err.Error(), "art-9")
	assert.Contains(t, err.Error(), "switching")
}

func TestWaitForReplacement_TerminalCompletedReturnsNoError(t *testing.T) {
	var hits int32

	serveReplacement(t, func(w http.ResponseWriter, _ *http.Request) {
		status := "submitted"
		if atomic.AddInt32(&hits, 1) >= 2 {
			status = ReplacementStatusCompleted
		}

		fmt.Fprintf(w, `{"candidateArtifactId":"art-2","status":"%s"}`, status)
	})

	replacement, err := WaitForReplacement("wl-1", nil, time.Millisecond, time.Second, nil)
	require.NoError(t, err)
	assert.Equal(t, ReplacementStatusCompleted, replacement.Status)
}

func TestWaitForReplacement_FailedReturnsError(t *testing.T) {
	serveReplacement(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"candidateArtifactId":"art-2","status":"failed"}`)
	})

	replacement, err := WaitForReplacement("wl-1", nil, time.Millisecond, time.Second, nil)
	require.Error(t, err)
	require.NotNil(t, replacement, "a failure returns the final replacement alongside the error")
	assert.Equal(t, ReplacementStatusFailed, replacement.Status)
	assert.Contains(t, err.Error(), "reverted")
}

// TestWaitForReplacement_ErroredClearedViaNotFound guards the exact bug found
// against staging: a candidate can settle as "errored", which is not one of
// the two documented terminal statuses, and then have its record cleared with
// a 404 moments later. If "errored" were not recognised as terminal, the
// 404-after-seen branch would report the failure as a success.
func TestWaitForReplacement_ErroredClearedViaNotFound(t *testing.T) {
	var hits int32

	serveReplacement(t, func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			fmt.Fprint(w, `{"candidateArtifactId":"art-2","status":"errored"}`)

			return
		}

		notFound(w)
	})

	replacement, err := WaitForReplacement("wl-1", nil, time.Millisecond, time.Second, nil)
	require.Error(t, err, "an errored candidate must not read as success just because it was later cleared")
	require.NotNil(t, replacement)
	assert.Equal(t, ReplacementStatusErrored, replacement.Status)
}

// TestWaitForReplacement_NotFoundOnFirstPollIsError still holds when the
// caller has nothing to seed the wait with, which is the attach case.
func TestWaitForReplacement_NotFoundOnFirstPollIsError(t *testing.T) {
	serveReplacement(t, func(w http.ResponseWriter, _ *http.Request) {
		notFound(w)
	})

	replacement, err := WaitForReplacement("wl-1", nil, time.Millisecond, time.Second, nil)
	require.Error(t, err)
	assert.Nil(t, replacement)
	assert.Contains(t, err.Error(), "no replacement is in flight")
	assert.NotContains(t, err.Error(), "after starting one",
		"this branch is only reachable when the caller did not start one")
}

// TestWaitForReplacement_StartedSeedsTheWait covers the race the unseeded
// version got wrong: the platform settles a fast rollout and collects the
// record before the very first poll lands, so the wait never observes a
// status. Handed what StartReplacement returned, it knows the rollout was
// real and reports success instead of failing a deploy that worked.
//
// It also pins the two things that follow from the seed being the only record
// there is. The status that comes back is the seed's own, non-terminal one,
// which is why the docstring sends callers to IsTerminalReplacementStatus
// before rendering it. And onTick stays silent, because it reports poll
// results and this wait had none, rather than replaying the seed as if it
// were an observation.
func TestWaitForReplacement_StartedSeedsTheWait(t *testing.T) {
	serveReplacement(t, func(w http.ResponseWriter, _ *http.Request) {
		notFound(w)
	})

	started := &Replacement{ArtifactID: "art-2", Status: "submitted"}

	replacement, err := WaitForReplacement("wl-1", started, time.Millisecond, time.Second, func(*Replacement) {
		t.Error("onTick must not fire when the seed carries the wait")
	})
	require.NoError(t, err)
	require.NotNil(t, replacement)
	assert.Equal(t, "art-2", replacement.ArtifactID)
	assert.Equal(t, "submitted", replacement.Status,
		"the seed comes back as it was; a nil error here does not mean the status is terminal")
}

// TestWaitForReplacement_NonTerminalClearedViaNotFoundIsSuccess is the other
// side of the same coin: the platform settles a rollout and collects the
// record between two polls, which is the ordinary happy path.
func TestWaitForReplacement_NonTerminalClearedViaNotFoundIsSuccess(t *testing.T) {
	var hits int32

	serveReplacement(t, func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			fmt.Fprint(w, `{"candidateArtifactId":"art-2","status":"switching"}`)

			return
		}

		notFound(w)
	})

	replacement, err := WaitForReplacement("wl-1", nil, time.Millisecond, time.Second, nil)
	require.NoError(t, err)
	require.NotNil(t, replacement)
	assert.Equal(t, "switching", replacement.Status)
}

// TestWaitForReplacement_TimeoutReturnsTheLastSeen pins both halves of the
// timeout contract. The error is obvious; the non-nil result is not, and it
// is what lets a caller render what the rollout was doing when it gave up
// instead of printing nothing.
func TestWaitForReplacement_TimeoutReturnsTheLastSeen(t *testing.T) {
	serveReplacement(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"candidateArtifactId":"art-2","status":"candidate-warming"}`)
	})

	replacement, err := WaitForReplacement("wl-1", nil, 5*time.Millisecond, 25*time.Millisecond, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
	require.NotNil(t, replacement, "a timeout must still say what the rollout was doing")
	assert.Equal(t, "candidate-warming", replacement.Status)
}

// TestWaitForReplacement_PollsAtLeastOnce fixes the deadline check in place.
// Moving it above the terminal check would make an already-settled rollout
// with an exhausted budget report a timeout instead of its real outcome.
func TestWaitForReplacement_PollsAtLeastOnce(t *testing.T) {
	var hits int32

	serveReplacement(t, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		fmt.Fprint(w, `{"candidateArtifactId":"art-2","status":"completed"}`)
	})

	replacement, err := WaitForReplacement("wl-1", nil, time.Millisecond, 0, nil)
	require.NoError(t, err)
	assert.Equal(t, ReplacementStatusCompleted, replacement.Status)
	assert.Equal(t, int32(1), atomic.LoadInt32(&hits), "a zero budget still buys one look")
}

// TestWaitForReplacement_NonPositiveIntervalDoesNotSpin stops an exported
// poller from hammering the API when handed a zero interval. The CLI's flags
// cannot produce one, but the signature does not say so.
func TestWaitForReplacement_NonPositiveIntervalDoesNotSpin(t *testing.T) {
	var hits int32

	serveReplacement(t, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		fmt.Fprint(w, `{"candidateArtifactId":"art-2","status":"switching"}`)
	})

	_, err := WaitForReplacement("wl-1", nil, 0, 10*time.Millisecond, nil)
	require.Error(t, err)
	assert.LessOrEqual(t, atomic.LoadInt32(&hits), int32(2),
		"a non-positive interval must fall back to the default, not busy-spin")
}

func TestWaitForReplacement_OnTickSeesEveryPoll(t *testing.T) {
	var hits int32

	serveReplacement(t, func(w http.ResponseWriter, _ *http.Request) {
		status := "switching"
		if atomic.AddInt32(&hits, 1) >= 3 {
			status = ReplacementStatusCompleted
		}

		fmt.Fprintf(w, `{"candidateArtifactId":"art-2","status":"%s"}`, status)
	})

	var seen []string

	_, err := WaitForReplacement("wl-1", nil, time.Millisecond, time.Second, func(r *Replacement) {
		require.NotNil(t, r, "onTick is never handed a nil")

		seen = append(seen, r.Status)
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"switching", "switching", "completed"}, seen)
}

func TestWaitForReplacement_PropagatesPollFailure(t *testing.T) {
	serveReplacement(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})

	_, err := WaitForReplacement("wl-1", nil, time.Millisecond, time.Second, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "poll replacement")
}
