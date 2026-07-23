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

// Replacement statuses observed to be terminal. The workload-api skill's
// docs only name "completed"/"failed"; live testing against staging also
// produced "errored" as a terminal failure (the candidate never became
// healthy) that the platform later clears via 404 like any other settled
// replacement -- without treating it as failed here, that 404 would be
// mistaken for a quiet success (see WaitForReplacement's doc). The full
// non-terminal vocabulary isn't documented; IsTerminalReplacementStatus
// treats anything else as still in progress rather than guessing, the same
// defensive stance IsTerminalBuildStatus takes for unknown build statuses.
const (
	ReplacementStatusCompleted = "completed"
	ReplacementStatusFailed    = "failed"
	ReplacementStatusErrored   = "errored"
)

// Replacement is the resource behind GET/POST
// /workloads/{id}/replacement/. Field names and shape confirmed live against
// staging: the target artifact id comes back as candidateArtifactId, not
// artifactId as the workload-api skill's example code implies. ArtifactID
// and Status are load-bearing for this CLI's polling and rollout logic;
// the rest are best-effort display fields.
type Replacement struct {
	ID         string    `json:"id"`
	WorkloadID string    `json:"workloadId"`
	ArtifactID string    `json:"candidateArtifactId"`
	Status     string    `json:"status"`
	Strategy   string    `json:"strategy,omitempty"`
	CreatedAt  time.Time `json:"createdAt,omitempty"`
	UpdatedAt  time.Time `json:"updatedAt,omitempty"`
}

// IsTerminalReplacementStatus reports whether s is a status the replacement
// will not progress from.
func IsTerminalReplacementStatus(s string) bool {
	switch s {
	case ReplacementStatusCompleted, ReplacementStatusFailed, ReplacementStatusErrored:
		return true
	}

	return false
}

// IsFailedReplacementStatus reports whether s is a terminal failure status.
// On failure the workload reverts to the artifact it was running before the
// replacement was started.
func IsFailedReplacementStatus(s string) bool {
	return s == ReplacementStatusFailed || s == ReplacementStatusErrored
}

// GetActiveReplacement fetches the in-flight replacement for workloadID, if
// any. The server responds 404 when none is active -- that is not an error
// condition here, it is the expected "nothing to guard against" answer, so
// it is translated to (nil, nil) rather than surfaced as *drapi.HTTPError.
func GetActiveReplacement(workloadID string) (*Replacement, error) {
	url, err := config.GetEndpointURL("/api/v2/workloads/" + escapeID(workloadID) + "/replacement/")
	if err != nil {
		return nil, err
	}

	var replacement Replacement

	err = drapi.GetJSON(url, "replacement", &replacement)
	if err != nil {
		var httpErr *drapi.HTTPError

		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return nil, nil
		}

		return nil, err
	}

	return &replacement, nil
}

// StartReplacement triggers a rolling replacement of workloadID's running
// artifact with artifactID. Not idempotent: calling this while a replacement
// is already in flight queues a second swap rather than erroring, so callers
// must check GetActiveReplacement first.
func StartReplacement(workloadID, artifactID string) (*Replacement, error) {
	url, err := config.GetEndpointURL("/api/v2/workloads/" + escapeID(workloadID) + "/replacement/")
	if err != nil {
		return nil, err
	}

	body := map[string]string{
		"artifactId": artifactID,
		"strategy":   "rolling",
	}

	var replacement Replacement

	if err := drapi.PostJSON(url, "replacement", body, &replacement); err != nil {
		return nil, err
	}

	return &replacement, nil
}

// WaitForReplacement polls GetActiveReplacement on interval until it reaches
// a terminal status, is cleared (404) after having been seen at least once,
// or deadline expires. Mirrors the workload-api skill's
// scripts/wait_for_replacement.py: a 404 on the very first poll is treated as
// an error (nothing was ever active to wait for), while a 404 after a
// non-terminal status was previously observed means the platform settled the
// replacement and garbage-collected the record -- reported as success via the
// last-seen record.
func WaitForReplacement(workloadID string, interval, timeout time.Duration) (*Replacement, error) {
	deadline := time.Now().Add(timeout)

	var lastSeen *Replacement

	for {
		replacement, err := GetActiveReplacement(workloadID)
		if err != nil {
			return nil, fmt.Errorf("poll replacement for workload %s: %w", workloadID, err)
		}

		if replacement == nil {
			if lastSeen == nil {
				return nil, fmt.Errorf("no active replacement found for workload %s immediately after starting one", workloadID)
			}

			return lastSeen, nil
		}

		lastSeen = replacement

		if IsTerminalReplacementStatus(replacement.Status) {
			if IsFailedReplacementStatus(replacement.Status) {
				return replacement, fmt.Errorf(
					"replacement for workload %s ended with status %s; the workload reverted to its previous artifact",
					workloadID, replacement.Status,
				)
			}

			return replacement, nil
		}

		if time.Now().After(deadline) {
			return replacement, fmt.Errorf("timeout waiting for replacement on workload %s after %s", workloadID, timeout)
		}

		time.Sleep(interval)
	}
}
