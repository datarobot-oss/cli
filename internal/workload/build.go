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
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/log"
)

// Build statuses are server-side enum values (UPPERCASE). All five were
// observed live during smoke against staging.
const (
	BuildStatusPending    = "PENDING"
	BuildStatusInProgress = "IN_PROGRESS"
	BuildStatusCompleted  = "COMPLETED"
	BuildStatusFailed     = "FAILED"
	BuildStatusCancelled  = "CANCELLED"
)

// BuildStatusCLIUnknown is a CLI-side sentinel for "the server never gave
// us a Build object" (e.g. the very first poll errored out). It is never
// emitted by the server and intentionally lives outside the server-status
// const block so it cannot be confused for a real enum value.
const BuildStatusCLIUnknown = "UNKNOWN"

// Default tail length used by BuildSummaryFor when a build ends in an error
// status. Kept conservative to keep `--wait` summaries reasonable in stderr.
const DefaultBuildLogTail = 50

// BuildTriggerResponse is the body returned by POST /artifacts/{id}/builds/.
// Field name is per the C2W tutorial; first smoke POST verifies casing.
type BuildTriggerResponse struct {
	BuildIDs []string `json:"buildIds"`
}

// Build is the per-build resource the platform owns. UPDATEs from
// PENDING -> IN_PROGRESS -> {COMPLETED|FAILED|CANCELLED}; the user-facing
// duration is UpdatedAt - CreatedAt.
type Build struct {
	ID         string    `json:"id"`
	Name       string    `json:"name,omitempty"`
	ArtifactID string    `json:"artifactId"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// BuildList is the paginated envelope returned by GET /artifacts/{id}/builds/.
type BuildList struct {
	Data       []Build `json:"data"`
	Count      int     `json:"count"`
	TotalCount int     `json:"totalCount"`
	Next       string  `json:"next"`
	Previous   string  `json:"previous"`
}

// BuildLogEntry is one record from the JSONL log stream. Decoded fields are
// the ones we read for filtering and human rendering; Raw is the verbatim
// source line so JSON output can pass through every server field unchanged.
type BuildLogEntry struct {
	Asctime   string          `json:"asctime"`
	Levelname string          `json:"levelname"`
	Name      string          `json:"name,omitempty"`
	Message   string          `json:"message"`
	BuildID   string          `json:"build_id,omitempty"`
	Raw       json.RawMessage `json:"-"`
}

// MarshalJSON emits the verbatim source line when Raw is set so renderers
// preserve every server field; falls back to the named fields otherwise.
func (e BuildLogEntry) MarshalJSON() ([]byte, error) {
	if len(e.Raw) > 0 {
		return e.Raw, nil
	}

	type plain BuildLogEntry

	return json.Marshal(plain(e))
}

// BuildOutput is the JSON projection of Build we emit to users. Mirrors
// ArtifactOutput's role: a deliberate, narrow shape so server-side
// additions (verbose internals, creator PII like email/userhash) do not
// leak into our scripted output contract.
type BuildOutput struct {
	ID         string `json:"id"`
	Name       string `json:"name,omitempty"`
	ArtifactID string `json:"artifactId"`
	Status     string `json:"status"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

// NewBuildOutput projects a Build into its user-facing JSON shape.
func NewBuildOutput(b Build) BuildOutput {
	return BuildOutput{
		ID:         b.ID,
		Name:       b.Name,
		ArtifactID: b.ArtifactID,
		Status:     b.Status,
		CreatedAt:  b.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  b.UpdatedAt.Format(time.RFC3339),
	}
}

// BuildSummary is the JSON document RenderBuildSummary emits for `--wait`
// and the same shape used to assemble the human one-liner in text mode.
// LogTail is NOT omitempty: a stable JSON shape lets consumers `jq` it
// uniformly whether or not logs were fetched successfully.
type BuildSummary struct {
	BuildID         string          `json:"buildId"`
	Status          string          `json:"status"`
	DurationSeconds int64           `json:"durationSeconds"`
	ImageURI        string          `json:"imageUri"`
	LogTail         []BuildLogEntry `json:"logTail"`
}

// IsTerminalBuildStatus reports whether s is a state from which the build
// will not progress further (COMPLETED, FAILED, CANCELLED). Unknown statuses
// are treated as non-terminal so polling keeps going rather than declaring
// success on a status the server added without telling us.
func IsTerminalBuildStatus(s string) bool {
	switch s {
	case BuildStatusCompleted, BuildStatusFailed, BuildStatusCancelled:
		return true
	}

	return false
}

// IsBuildErrorStatus reports whether s is a terminal failure.
func IsBuildErrorStatus(s string) bool {
	switch s {
	case BuildStatusFailed, BuildStatusCancelled:
		return true
	}

	return false
}

// TriggerArtifactBuild POSTs an empty body to /artifacts/{id}/builds/ and
// returns the trigger response. The defensive empty-slice check is in the
// caller so the service layer remains a thin pass-through of the server
// shape.
func TriggerArtifactBuild(artifactID string) (*BuildTriggerResponse, error) {
	url, err := config.GetEndpointURL("/api/v2/artifacts/" + escapeID(artifactID) + "/builds/")
	if err != nil {
		return nil, err
	}

	var resp BuildTriggerResponse

	if err := drapi.PostJSON(url, "build", map[string]any{}, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// GetArtifactBuild fetches a single Build by id.
func GetArtifactBuild(artifactID, buildID string) (*Build, error) {
	url, err := config.GetEndpointURL("/api/v2/artifacts/" + escapeID(artifactID) + "/builds/" + escapeID(buildID))
	if err != nil {
		return nil, err
	}

	var build Build

	if err := drapi.GetJSON(url, "build", &build); err != nil {
		return nil, err
	}

	return &build, nil
}

// ListArtifactBuilds returns up to limit Builds for the artifact, walking
// pagination the same way ListArtifacts does.
func ListArtifactBuilds(artifactID string, limit int) ([]Build, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("invalid limit %d: must be positive", limit)
	}

	endpoint := "/api/v2/artifacts/" + escapeID(artifactID) + "/builds/?limit=" + strconv.Itoa(limit)

	pageURL, err := config.GetEndpointURL(endpoint)
	if err != nil {
		return nil, err
	}

	var all []Build

	for pageURL != "" {
		var list BuildList

		if err := drapi.GetJSON(pageURL, "builds", &list); err != nil {
			return nil, err
		}

		all = append(all, list.Data...)

		if len(all) >= limit {
			return all[:limit], nil
		}

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

// GetArtifactBuildLogs returns parsed log entries for a build. The endpoint
// emits newline-delimited JSON; we tolerate malformed lines so a single bad
// record cannot blank the whole tail. The original bytes for each line are
// preserved in Raw so JSON output can pass them through unchanged.
func GetArtifactBuildLogs(artifactID, buildID string) ([]BuildLogEntry, error) {
	url, err := config.GetEndpointURL("/api/v2/artifacts/" + escapeID(artifactID) + "/builds/" + escapeID(buildID) + "/logs")
	if err != nil {
		return nil, err
	}

	resp, err := drapi.Get(url, "build logs")
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	return parseBuildLogs(resp.Body)
}

func parseBuildLogs(r io.Reader) ([]BuildLogEntry, error) {
	scanner := bufio.NewScanner(r)
	// Each line can be a multi-KB structured log; bump the buffer past the
	// 64KiB default to accommodate verbose entries without truncation.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var entries []BuildLogEntry

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}

		var entry BuildLogEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			// Skip malformed lines rather than fail the whole fetch;
			// log payloads regularly include non-JSON tail lines from
			// the underlying buildkit pipe.
			continue
		}

		entry.Raw = append(json.RawMessage(nil), line...)
		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read build logs: %w", err)
	}

	return entries, nil
}

// WaitForBuild polls GetArtifactBuild on interval until IsTerminalBuildStatus
// returns true or deadline expires. On terminal error status it returns the
// final Build alongside an error so callers get both pieces. onTick may be
// nil and is invoked after each poll for debug-only observation (Bubble Tea
// cannot redraw the spinner label from inside fn(), so this is a passive
// seam in PR 1).
func WaitForBuild(
	artifactID, buildID string,
	interval, timeout time.Duration,
	onTick func(*Build),
) (*Build, error) {
	deadline := time.Now().Add(timeout)

	for {
		build, err := GetArtifactBuild(artifactID, buildID)
		if err != nil {
			return nil, fmt.Errorf("poll build %s: %w", buildID, err)
		}

		if onTick != nil {
			onTick(build)
		}

		if IsTerminalBuildStatus(build.Status) {
			if IsBuildErrorStatus(build.Status) {
				return build, fmt.Errorf("build %s ended with status %s; run 'dr artifact build logs %s' to inspect", buildID, build.Status, buildID)
			}

			return build, nil
		}

		if time.Now().After(deadline) {
			return build, fmt.Errorf("timeout waiting for build %s after %s", buildID, timeout)
		}

		time.Sleep(interval)
	}
}

// BuildSummaryFor composes the terminal-state summary RenderBuildSummary
// renders. Duration comes from the Build timestamps; ImageURI is fetched
// from the parent artifact's primary container only on COMPLETED (the
// server only updates imageUri on a successful build); for any other
// status -- FAILED, CANCELLED, or a still-PENDING/IN_PROGRESS build
// returned alongside a timeout error -- we skip the artifact fetch so a
// stale imageUri from a prior successful build cannot leak into the
// current build's summary. On error status we additionally pull the log
// tail so the user has something to act on.
func BuildSummaryFor(build *Build, tailLen int) (BuildSummary, error) {
	if build == nil {
		return BuildSummary{}, errors.New("nil build")
	}

	summary := BuildSummary{
		BuildID:         build.ID,
		Status:          build.Status,
		DurationSeconds: buildDurationSeconds(*build),
	}

	if build.Status != BuildStatusCompleted {
		if IsBuildErrorStatus(build.Status) {
			logs, lerr := GetArtifactBuildLogs(build.ArtifactID, build.ID)
			if lerr != nil {
				// Surface the fetch error via debug logging rather than
				// failing the whole summary -- the user still benefits
				// from seeing the build's terminal state even when the
				// build-service logs endpoint is unavailable (which is
				// common right after a CANCELLED build, when the logs
				// have been garbage-collected).
				log.Debug("BuildSummaryFor: log tail fetch failed", "build_id", build.ID, "err", lerr)
			} else {
				summary.LogTail = lastN(logs, tailLen)
			}
		}

		return summary, nil
	}

	artifact, err := GetArtifact(build.ArtifactID)
	if err != nil {
		// Surface the partial summary so callers can still render
		// duration and status even when the artifact fetch fails.
		// ImageURI stays empty in that case.
		return summary, fmt.Errorf("fetch parent artifact for build %s: %w", build.ID, err)
	}

	summary.ImageURI = GetPrimaryContainerImageURI(*artifact)

	return summary, nil
}

func buildDurationSeconds(build Build) int64 {
	if build.UpdatedAt.IsZero() || build.CreatedAt.IsZero() {
		return 0
	}

	d := build.UpdatedAt.Sub(build.CreatedAt)
	if d < 0 {
		return 0
	}

	return int64(d.Seconds())
}

func lastN(entries []BuildLogEntry, n int) []BuildLogEntry {
	if n <= 0 || len(entries) <= n {
		return entries
	}

	return entries[len(entries)-n:]
}
