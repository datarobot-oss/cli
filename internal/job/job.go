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

// Package job is the client for the Workload API's run-to-completion jobs.
//
// A job is a workload of type job: it stores the artifact and runtime its runs
// use, and has no status of its own. Each run is a proton that ends instead of
// serving, and carries its own status and, once it ends, its exit details. The
// job's code is an ordinary job artifact, edited and built with the artifact
// commands.
//
// Runtime is kept as raw JSON on purpose, as the workload manifest is: the
// schema moves faster than a struct here would, and a field this release has
// not heard of must survive a read and a write.
package job

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload/apiclient"
)

// maxPageSize is the server's per-page limit for the job and run lists.
const maxPageSize = 100

// Run statuses, as the API reports them.
const (
	StatusSubmitted    = "submitted"
	StatusProvisioning = "provisioning"
	StatusLaunching    = "launching"
	StatusRunning      = "running"
	StatusStopping     = "stopping"
	StatusSucceeded    = "succeeded"
	StatusFailed       = "failed"
	StatusErrored      = "errored"
	StatusStopped      = "stopped"
)

// runStatuses are the values the run list's status filter accepts.
var runStatuses = []string{
	StatusSubmitted, StatusProvisioning, StatusLaunching, StatusRunning, StatusStopping,
	StatusSucceeded, StatusFailed, StatusErrored, StatusStopped,
}

// endStatuses are the statuses a run never leaves.
var endStatuses = []string{StatusSucceeded, StatusFailed, StatusErrored, StatusStopped}

// Creator is who made a job or started a run.
type Creator struct {
	ID       string `json:"id"`
	Username string `json:"username,omitempty"`
}

// Job is a run-to-completion job: what its runs execute and with which runtime.
type Job struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	ArtifactID  string          `json:"artifactId"`
	Runtime     json.RawMessage `json:"runtime,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
	Creator     *Creator        `json:"creator,omitempty"`
}

// Placement is where a run was placed. Empty on the Hub.
type Placement struct {
	EnclaveID string `json:"enclaveId,omitempty"`
	Enclave   string `json:"enclave,omitempty"`
}

// Run is one run of a job.
type Run struct {
	ID                 string          `json:"id"`
	JobID              string          `json:"jobId"`
	ArtifactID         string          `json:"artifactId"`
	Status             string          `json:"status"`
	Runtime            json.RawMessage `json:"runtime,omitempty"`
	Placement          *Placement      `json:"placement,omitempty"`
	StartedAt          *time.Time      `json:"startedAt,omitempty"`
	FinishedAt         *time.Time      `json:"finishedAt,omitempty"`
	ExitCode           *int            `json:"exitCode,omitempty"`
	TerminationReason  string          `json:"terminationReason,omitempty"`
	TerminationMessage string          `json:"terminationMessage,omitempty"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
	Creator            *Creator        `json:"creator,omitempty"`
}

// Ended reports whether the run reached a status it never leaves.
func (r Run) Ended() bool {
	return slices.Contains(endStatuses, r.Status)
}

// Succeeded reports whether the run ended with its code exiting 0.
func (r Run) Succeeded() bool {
	return r.Status == StatusSucceeded
}

type page[T any] struct {
	Data       []T    `json:"data"`
	TotalCount int    `json:"totalCount"`
	Next       string `json:"next"`
}

func escapeID(id string) string {
	return url.PathEscape(id)
}

func jobURL(jobID string, rest ...string) (string, error) {
	var path strings.Builder

	path.WriteString("/api/v2/jobs/" + escapeID(jobID) + "/")

	for _, part := range rest {
		path.WriteString(escapeID(part) + "/")
	}

	return config.GetEndpointURL(path.String())
}

// CreateJob creates a job from the POST /jobs body. It does not start a run.
// The body is a map so fields this release does not know reach the server.
func CreateJob(request map[string]any) (*Job, error) {
	url, err := config.GetEndpointURL("/api/v2/jobs/")
	if err != nil {
		return nil, err
	}

	var job Job

	if err := apiclient.PostJSON(url, "job", request, &job); err != nil {
		return nil, err
	}

	return &job, nil
}

// GetJob reads one job.
func GetJob(jobID string) (*Job, error) {
	url, err := jobURL(jobID)
	if err != nil {
		return nil, err
	}

	var job Job

	if err := apiclient.LiftDetail(drapi.GetJSON(url, "job", &job)); err != nil {
		return nil, err
	}

	return &job, nil
}

// ListJobs lists the jobs the caller can see, newest first. name, when set, is
// an exact match; names are not unique, so it can return more than one job.
func ListJobs(name string, limit, offset int) ([]Job, error) {
	query := pageQuery(limit, offset)

	if name != "" {
		query.Set("name", name)
	}

	pageURL, err := drapi.EndpointURL("/jobs/", query)
	if err != nil {
		return nil, err
	}

	return paginate[Job](pageURL, "jobs", limit)
}

// UpdateJob applies the fields in changes (camelCase API keys) and returns the
// job as it now stands. Runs already started keep what they started with.
func UpdateJob(jobID string, changes map[string]any) (*Job, error) {
	url, err := jobURL(jobID)
	if err != nil {
		return nil, err
	}

	var job Job

	if err := apiclient.LiftDetail(drapi.PatchJSON(url, "job", changes, &job)); err != nil {
		return nil, err
	}

	return &job, nil
}

// DeleteJob deletes a job. The server stops its active runs first.
func DeleteJob(jobID string) error {
	url, err := jobURL(jobID)
	if err != nil {
		return err
	}

	return apiclient.LiftDetail(drapi.DeleteJSON(url, "job", nil, nil))
}

// TriggerRun starts a run of the job with the artifact and runtime it stores.
func TriggerRun(jobID string) (*Run, error) {
	url, err := jobURL(jobID, "runs")
	if err != nil {
		return nil, err
	}

	var run Run

	if err := apiclient.PostJSON(url, "job run", map[string]any{}, &run); err != nil {
		return nil, err
	}

	return &run, nil
}

// GetRun reads one run of the job.
func GetRun(jobID, runID string) (*Run, error) {
	url, err := jobURL(jobID, "runs", runID)
	if err != nil {
		return nil, err
	}

	var run Run

	if err := apiclient.LiftDetail(drapi.GetJSON(url, "job run", &run)); err != nil {
		return nil, err
	}

	return &run, nil
}

// ListRuns lists the job's runs, newest first, only those in statuses when set.
func ListRuns(jobID string, statuses []string, limit, offset int) ([]Run, error) {
	query := pageQuery(limit, offset)

	for _, status := range statuses {
		query.Add("status", status)
	}

	pageURL, err := drapi.EndpointURL("/jobs/"+escapeID(jobID)+"/runs/", query)
	if err != nil {
		return nil, err
	}

	return paginate[Run](pageURL, "job runs", limit)
}

// CancelRun cancels a run that has not ended; the run returned is STOPPING.
// The server refuses a run that already ended with a 409.
func CancelRun(jobID, runID string) (*Run, error) {
	url, err := jobURL(jobID, "runs", runID, "cancel")
	if err != nil {
		return nil, err
	}

	// The cancel route has no trailing slash on its last segment.
	url = strings.TrimSuffix(url, "/")

	var run Run

	if err := apiclient.PostJSON(url, "job run cancel", map[string]any{}, &run); err != nil {
		return nil, err
	}

	return &run, nil
}

// ParseRunStatuses lowercases and validates run status filters.
func ParseRunStatuses(values []string) ([]string, error) {
	parsed := make([]string, 0, len(values))

	for _, value := range values {
		status := strings.ToLower(strings.TrimSpace(value))
		if !slices.Contains(runStatuses, status) {
			return nil, fmt.Errorf("invalid --status %q: must be one of %s", value, strings.Join(runStatuses, ", "))
		}

		parsed = append(parsed, status)
	}

	return parsed, nil
}

func pageQuery(limit, offset int) url.Values {
	query := url.Values{}
	query.Set("limit", strconv.Itoa(min(limit, maxPageSize)))
	query.Set("offset", strconv.Itoa(offset))

	return query
}

// paginate follows next-links from pageURL until limit items are collected or
// the pages run out.
func paginate[T any](pageURL, info string, limit int) ([]T, error) {
	var all []T

	for pageURL != "" {
		var list page[T]

		if err := apiclient.LiftDetail(drapi.GetJSON(pageURL, info, &list)); err != nil {
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
