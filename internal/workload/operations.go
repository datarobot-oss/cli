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
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
)

// WorkloadOperationResponse is the acknowledgement returned by the
// asynchronous lifecycle operations POST /workloads/{id}/start and /stop.
// Status is the server's human-readable outcome message ("stopping",
// "The proton is already running", ...), not a workload status enum value;
// TrackVia is the API path to poll to observe the resulting transition.
type WorkloadOperationResponse struct {
	Status     string `json:"status"`
	WorkloadID string `json:"workloadId"`
	TrackVia   string `json:"trackVia"`
}

// StartWorkload requests an asynchronous start of a stopped workload. The
// server replies 202 when the start was queued (stopped/unknown →
// submitted) and 200 when the workload is already running, initializing,
// or suspended (idempotent no-op); both decode into the operation
// response. Conflicting states (409, e.g. still stopping or a restart in
// progress), exceeded concurrency limits (403), and missing workloads
// (404) surface as *drapi.HTTPError with the server's detail message.
func StartWorkload(workloadID string) (*WorkloadOperationResponse, error) {
	return postWorkloadAction(workloadID, "start")
}

// StopWorkload requests an asynchronous stop of a workload. The server
// replies 202 when the stop was queued (status moves to stopping, then
// stopped) and 200 when the workload is already stopped (idempotent
// no-op); both decode into the operation response. Missing workloads
// (404) surface as *drapi.HTTPError.
func StopWorkload(workloadID string) (*WorkloadOperationResponse, error) {
	return postWorkloadAction(workloadID, "stop")
}

// postWorkloadAction POSTs an empty JSON object to a workload action
// sub-path. Action routes have no trailing slash, unlike the resource
// route used by GetWorkload.
func postWorkloadAction(workloadID, action string) (*WorkloadOperationResponse, error) {
	url, err := config.GetEndpointURL("/api/v2/workloads/" + escapeID(workloadID) + "/" + action)
	if err != nil {
		return nil, err
	}

	var resp WorkloadOperationResponse

	if err := drapi.PostJSON(url, "workload "+action+" request", map[string]any{}, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// IsSettledWorkloadStatus reports whether status is a state the workload
// will not leave without another action: running, stopped, suspended,
// errored, terminated, interrupted, or unknown. In-flight states
// (submitted, provisioning, launching, stopping) and unrecognized future
// statuses are non-settled, so polling keeps going until the deadline
// rather than declaring success on a status this CLI does not know.
func IsSettledWorkloadStatus(status string) bool {
	switch status {
	case WorkloadStatusRunning,
		WorkloadStatusStopped,
		WorkloadStatusSuspended,
		WorkloadStatusErrored,
		WorkloadStatusTerminated,
		WorkloadStatusInterrupted,
		WorkloadStatusUnknown:
		return true
	}

	return false
}

// maxTransientPollErrors bounds how many consecutive transient failures
// (5xx, 429, or network errors) WaitForWorkloadStatus tolerates before
// giving up. The counter resets on any successful poll, so it only trips on
// a sustained outage, not on isolated gateway blips during a long --wait.
const maxTransientPollErrors = 5

// WaitForWorkloadStatus polls GetWorkload on interval until
// IsSettledWorkloadStatus reports the status settled or the deadline
// expires. Settling on errored returns the final workload alongside an
// error so callers get both pieces; any other settled status (a user
// waiting after stop legitimately lands on stopped) is a plain success.
// onTick may be nil and is invoked after each successful poll for passive
// observation, e.g. printing status transitions.
//
// Transient poll failures are retried rather than aborting the whole wait:
// a single 502 over a multi-minute poll must not fail a CI deploy gate. A
// 4xx is terminal (a 404 means the workload is gone, a 403 means auth
// changed), and a sustained run of transient errors past
// maxTransientPollErrors still gives up.
func WaitForWorkloadStatus(
	workloadID string,
	interval, timeout time.Duration,
	onTick func(*Workload),
) (*Workload, error) {
	deadline := time.Now().Add(timeout)

	var transientErrors int

	for {
		wl, err := GetWorkload(workloadID)
		if err != nil {
			if !isTransientPollError(err) {
				return nil, fmt.Errorf("poll workload %s: %w", workloadID, err)
			}

			transientErrors++

			if transientErrors > maxTransientPollErrors {
				return nil, fmt.Errorf("poll workload %s: %d consecutive transient errors, last: %w", workloadID, transientErrors, err)
			}

			if time.Now().After(deadline) {
				return nil, fmt.Errorf("timeout waiting for workload %s after %s: %w", workloadID, timeout, err)
			}

			time.Sleep(interval)

			continue
		}

		transientErrors = 0

		if onTick != nil {
			onTick(wl)
		}

		if IsSettledWorkloadStatus(wl.Status) {
			if wl.Status == WorkloadStatusErrored {
				// Points at `dr workload get` until the events command ships;
				// the events PR flips this hint when the command exists.
				return wl, fmt.Errorf("workload %s settled with status %s; run 'dr workload get %s' to inspect", workloadID, wl.Status, workloadID)
			}

			return wl, nil
		}

		if time.Now().After(deadline) {
			return wl, fmt.Errorf("timeout waiting for workload %s after %s", workloadID, timeout)
		}

		time.Sleep(interval)
	}
}

// isTransientPollError reports whether a GetWorkload failure is worth
// retrying during a poll loop: server-side 5xx and 429 rate limits, plus
// non-HTTP errors (connection resets, timeouts). Client 4xx errors are
// terminal -- a retry will not bring back a deleted workload or fix lost
// auth.
func isTransientPollError(err error) bool {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) {
		return httpErr.StatusCode >= 500 || httpErr.StatusCode == http.StatusTooManyRequests
	}

	return true
}
