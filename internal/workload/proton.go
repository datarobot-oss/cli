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
	"strings"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
)

// Proton statuses observed live during a rolling replacement. An unrecognised
// one is not assumed to mean anything.
const (
	ProtonStatusRunning  = "running"
	ProtonStatusDraining = "draining"
)

// ProtonRoleActive marks the generation the platform considers to be serving;
// the others carry no role. Sharper than "which one is running", since a
// candidate runs for a while before it is promoted.
const ProtonRoleActive = "active"

// maxProtonPages bounds the next-link walk. A workload has one generation most
// of the time and two during a replacement, so anything approaching this is a
// route that is not ending rather than a workload that is unusually busy.
const maxProtonPages = 20

// Proton is one generation of a workload's running containers: the thing that
// answers the endpoint. A workload has one most of the time and two during a
// rolling replacement, the outgoing one in "draining". It is the only place the
// platform says whether the previous version has stopped serving.
type Proton struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	WorkloadID string    `json:"workloadId"`
	ArtifactID string    `json:"artifactId"`
	Status     string    `json:"status"`
	Role       string    `json:"role"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type protonList struct {
	Data       []Proton `json:"data"`
	Count      int      `json:"count"`
	TotalCount int      `json:"totalCount"`
	Next       string   `json:"next"`
}

// IsDrainingProtonStatus reports whether s is a generation on its way out that
// is still answering requests.
func IsDrainingProtonStatus(s string) bool {
	return strings.EqualFold(s, ProtonStatusDraining)
}

// IsRunningProtonStatus reports whether s is serving and not on its way out.
func IsRunningProtonStatus(s string) bool {
	return strings.EqualFold(s, ProtonStatusRunning)
}

// IsActiveProtonRole reports whether r marks the serving generation.
func IsActiveProtonRole(r string) bool {
	return strings.EqualFold(r, ProtonRoleActive)
}

// ListProtons fetches the generations currently attached to a workload.
//
// It follows next-links like every other list in this package. A page-one-only
// read would be worse here than elsewhere: the generation being replaced is the
// oldest, so a truncated page can omit exactly the draining proton the wait
// exists to see, and the wait would settle early with no error to show for it.
//
// The walk is bounded. It runs inside a poll predicate, which sits downstream
// of the deadline check, so a cursor that points back at itself would spin
// past --poll-timeout with no way out. Every other list here is bounded by a
// caller-supplied limit; this one has no limit to pass, so it counts pages.
//
// A body carrying no recognisable list is an error rather than an empty one.
// Decoding into protonList succeeds for any JSON object, so a route that
// answers a different shape would otherwise read as "no generations" and settle
// the wait silently, which is the one failure mode that looks exactly like
// success.
func ListProtons(workloadID string) ([]Proton, error) {
	pageURL, err := config.GetEndpointURL("/api/v2/workloads/" + escapeID(workloadID) + "/protons/")
	if err != nil {
		return nil, err
	}

	var all []Proton

	for page := 0; pageURL != ""; page++ {
		if page >= maxProtonPages {
			return nil, fmt.Errorf(
				"proton list for workload %s did not end after %d pages; refusing to follow more",
				workloadID, maxProtonPages)
		}

		var list protonList

		if err := drapi.GetJSON(pageURL, "protons", &list); err != nil {
			return nil, err
		}

		if list.Data == nil && list.TotalCount == 0 && list.Count == 0 {
			return nil, fmt.Errorf("proton list for workload %s carried no data field", workloadID)
		}

		all = append(all, list.Data...)

		if list.Next == "" {
			break
		}

		if err := drapi.AssertNextOnSameHost(list.Next); err != nil {
			return nil, err
		}

		pageURL = list.Next
	}

	return all, nil
}

// protonsServing reports whether the generation the caller waits for is the one
// answering, and the one it replaced has stopped.
//
// This is what the workload document cannot answer. Measured live: at t+110s it
// names the NEW artifact, the old generation starts draining and the new one is
// marked active, yet the endpoint answers with the old build until t+171s; the
// old generation only leaves draining at t+530s.
//
// A predecessor blocks the wait while it is running or draining, which are the
// two states in which it was observed still answering. Both matter: at t+27s
// both generations were running and the endpoint was still on the old build.
// Anything past those is treated as gone, because an outgoing generation sits
// in stopping long after it stopped mattering and waiting for the list to empty
// would hold a finished deploy open for minutes.
//
// Known limitation: a predecessor status this code has not seen is read as
// gone. The alternative, blocking on anything unrecognised, trades a rare early
// exit for a deploy that hangs to its timeout on a status the platform added.
//
// Without an artifact named, which is a resize, both generations share one and
// draining is the only thing that tells them apart. An empty list settles that
// case rather than blocking, so a cluster whose route answers differently does
// not hang; with an artifact named it does not, since an empty list is also
// what the gap between one generation being collected and the next appearing
// looks like.
func protonsServing(protons []Proton, want Serving) protonVerdict {
	if len(protons) == 0 {
		// An empty list is two things at once: a route with nothing to say,
		// and the blink between one generation being collected and the next
		// being listed. With an artifact named the second reading matters, so
		// it is reported as unresolved rather than settled, and the caller
		// gives it a bounded number of polls to resolve before accepting it.
		return protonVerdict{Serving: want.ArtifactID == "", Empty: true}
	}

	if want.AwaitDrain && anyServingPredecessor(protons, want.ArtifactID) {
		return protonVerdict{}
	}

	if want.ArtifactID == "" {
		return protonVerdict{Serving: true}
	}

	// The platform's own answer, when it gives one.
	if at := activeProtonIndex(protons); at >= 0 {
		active := protons[at]

		return protonVerdict{
			Serving: active.ArtifactID == want.ArtifactID && IsRunningProtonStatus(active.Status),
		}
	}

	// No role reported, so fall back to whether anything runs the version.
	return protonVerdict{Serving: anyRunning(protons, want.ArtifactID)}
}

// protonVerdict is what the proton list said. Empty separates "the list is
// there and the handover is not done" from "the list said nothing at all",
// which are the same answer to the wait and different answers to how long it
// is worth waiting.
type protonVerdict struct {
	Serving bool
	Empty   bool
}

// anyServingPredecessor reports whether a generation other than the one being
// waited for is still taking requests.
//
// Draining and running both count. Measured at t+27s of a rollout, both
// generations were running and the endpoint was still on the old build, so
// treating running as settled would return before the handover.
//
// The successor is identified by artifact when one is named, and otherwise by
// the active role, which is what makes this work for a resize: a resize
// replaces a workload with itself, so both generations carry the same artifact
// and only the role separates them. On an install that reports no role, a
// resize falls back to draining alone, since there is then nothing to tell a
// predecessor from its replacement.
func anyServingPredecessor(protons []Proton, wantArtifactID string) bool {
	// By index, not by id. Identifying the successor by Proton.ID would treat
	// every generation as the successor on any response that omits the field.
	activeAt := activeProtonIndex(protons)

	for i := range protons {
		p := &protons[i]

		if wantArtifactID != "" && p.ArtifactID == wantArtifactID {
			continue
		}

		if i == activeAt {
			continue
		}

		if IsDrainingProtonStatus(p.Status) {
			return true
		}

		// Running only counts as a predecessor once something identifies the
		// successor, or every generation would look like one.
		if IsRunningProtonStatus(p.Status) && (wantArtifactID != "" || activeAt >= 0) {
			return true
		}
	}

	return false
}

// activeProtonIndex is where the generation marked as serving sits, or -1 when
// none is marked.
func activeProtonIndex(protons []Proton) int {
	for i := range protons {
		if IsActiveProtonRole(protons[i].Role) {
			return i
		}
	}

	return -1
}

// anyRunning reports whether some generation runs wantArtifactID.
func anyRunning(protons []Proton, wantArtifactID string) bool {
	for i := range protons {
		if protons[i].ArtifactID == wantArtifactID && IsRunningProtonStatus(protons[i].Status) {
			return true
		}
	}

	return false
}
