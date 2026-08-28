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

package drapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type routeResponse struct {
	status int
	body   string
}

// setupRoutedServer serves distinct bodies for the LLM Gateway catalog and the
// deployments routes, so the two sources GetLLMsAndDeployed queries can be
// exercised (and failed) independently. A zero status defaults to 200.
func setupRoutedServer(t *testing.T, catalog, deployments routeResponse) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var resp routeResponse

		switch {
		case strings.Contains(r.URL.Path, "/deployments"):
			resp = deployments
		case strings.Contains(r.URL.Path, "/catalog"):
			resp = catalog
		default:
			http.NotFound(w, r)
			return
		}

		if resp.status == 0 {
			resp.status = http.StatusOK
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.status)
		_, _ = io.WriteString(w, resp.body)
	}))

	viperx.Reset()
	viperx.Set(config.DataRobotURL, srv.URL)
	viperx.Set(config.DataRobotAPIKey, "test-token")
	// skip_auth trusts the viper token directly; without it resolveToken makes a
	// server-side verification request the routed handler would 404.
	viperx.Set(config.SkipAuthKey, true)

	t.Cleanup(func() {
		srv.Close()
		viperx.Reset()
	})
}

const gatewayBody = `{"data":[{"llmId":"llm-001","name":"GPT-4o","provider":"azure","model":"gpt-4o","isActive":true}],"count":1,"totalCount":1}`

// deployedBody mixes a valid chat deployment with two that must be filtered
// out client-side: an inactive TextGeneration deployment and an active
// non-TextGeneration deployment.
const deployedBody = `{"data":[
	{"id":"dep-active-tg","label":"RAG LLM","status":"active","model":{"targetType":"TextGeneration"}},
	{"id":"dep-inactive","label":"Old LLM","status":"inactive","model":{"targetType":"TextGeneration"}},
	{"id":"dep-binary","label":"Churn model","status":"active","model":{"targetType":"Binary"}}
],"count":3,"totalCount":3}`

func TestGetDeployedLLMs_MapsAndFilters(t *testing.T) {
	setupRoutedServer(t, routeResponse{}, routeResponse{body: deployedBody})

	deployed, err := GetDeployedLLMs()
	require.NoError(t, err)

	require.Len(t, deployed, 1)
	got := deployed[0]
	assert.Equal(t, "dep-active-tg", got.LlmID)
	assert.Equal(t, "dep-active-tg", got.DeploymentID)
	assert.Equal(t, "RAG LLM", got.Name)
	assert.Equal(t, deployedModelSentinel, got.Model)
	assert.Equal(t, LLMKindDeployed, got.Kind)
	assert.True(t, got.IsActive)
}

func TestGetDeployedLLMs_Pagination(t *testing.T) {
	var srv *httptest.Server

	page := 0
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		page++
		if page == 1 {
			// Point Next at the same host (AssertNextOnSameHost requirement).
			_, _ = io.WriteString(w, `{"data":[{"id":"dep-1","label":"LLM One","status":"active","model":{"targetType":"TextGeneration"}}],"next":"`+srv.URL+`/api/v2/deployments/?offset=100"}`)
			return
		}

		_, _ = io.WriteString(w, `{"data":[{"id":"dep-2","label":"LLM Two","status":"active","model":{"targetType":"TextGeneration"}}],"next":""}`)
	}))

	viperx.Reset()
	viperx.Set(config.DataRobotURL, srv.URL)
	viperx.Set(config.DataRobotAPIKey, "test-token")
	viperx.Set(config.SkipAuthKey, true)

	t.Cleanup(func() {
		srv.Close()
		viperx.Reset()
	})

	deployed, err := GetDeployedLLMs()
	require.NoError(t, err)

	require.Len(t, deployed, 2)
	assert.Equal(t, "dep-1", deployed[0].LlmID)
	assert.Equal(t, "dep-2", deployed[1].LlmID)
}

func TestGetLLMsAndDeployed_Union(t *testing.T) {
	setupRoutedServer(t, routeResponse{body: gatewayBody}, routeResponse{body: deployedBody})

	list, err := GetLLMsAndDeployed()
	require.NoError(t, err)

	require.Len(t, list.LLMs, 2)
	assert.Equal(t, LLMKindGateway, list.LLMs[0].Kind)
	assert.Equal(t, "llm-001", list.LLMs[0].LlmID)
	assert.Equal(t, LLMKindDeployed, list.LLMs[1].Kind)
	assert.Equal(t, "dep-active-tg", list.LLMs[1].LlmID)
	assert.Equal(t, 2, list.Count)
}

func TestGetLLMsAndDeployed_GatewayFailsSoftDegrade(t *testing.T) {
	setupRoutedServer(t, routeResponse{status: http.StatusInternalServerError, body: "boom"}, routeResponse{body: deployedBody})

	list, err := GetLLMsAndDeployed()
	require.NoError(t, err)

	require.Len(t, list.LLMs, 1)
	assert.Equal(t, LLMKindDeployed, list.LLMs[0].Kind)

	require.Len(t, list.Warnings, 1)
	assert.Contains(t, list.Warnings[0], "LLM Gateway catalog unavailable")
}

func TestGetLLMsAndDeployed_DeployedFailsSoftDegrade(t *testing.T) {
	setupRoutedServer(t, routeResponse{body: gatewayBody}, routeResponse{status: http.StatusInternalServerError, body: "boom"})

	list, err := GetLLMsAndDeployed()
	require.NoError(t, err)

	require.Len(t, list.LLMs, 1)
	assert.Equal(t, LLMKindGateway, list.LLMs[0].Kind)

	require.Len(t, list.Warnings, 1)
	assert.Contains(t, list.Warnings[0], "DataRobot-deployed LLMs unavailable")
}

func TestGetLLMsAndDeployed_BothSucceedNoWarnings(t *testing.T) {
	setupRoutedServer(t, routeResponse{body: gatewayBody}, routeResponse{body: deployedBody})

	list, err := GetLLMsAndDeployed()
	require.NoError(t, err)

	assert.Empty(t, list.Warnings)
}

func TestGetDeployedLLMs_EmptyLabelFallsBackToID(t *testing.T) {
	body := `{"data":[{"id":"dep-no-label","label":"","status":"active","model":{"targetType":"TextGeneration"}}]}`
	setupRoutedServer(t, routeResponse{}, routeResponse{body: body})

	deployed, err := GetDeployedLLMs()
	require.NoError(t, err)

	require.Len(t, deployed, 1)
	assert.Equal(t, "dep-no-label", deployed[0].Name)
}

// TestGetDeployedLLMs_SendsFilterAndLimit asserts the outgoing request carries
// the server-side target-type filter and page limit. Without this, dropping the
// filter from the URL would still pass the mapping tests (the client re-filter
// masks it) while silently losing the server-side narrowing.
func TestGetDeployedLLMs_SendsFilterAndLimit(t *testing.T) {
	// Buffered so the handler goroutine's send happens-before the test's read.
	queryCh := make(chan string, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queryCh <- r.URL.RawQuery

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))

	viperx.Reset()
	viperx.Set(config.DataRobotURL, srv.URL)
	viperx.Set(config.DataRobotAPIKey, "test-token")
	viperx.Set(config.SkipAuthKey, true)

	t.Cleanup(func() {
		srv.Close()
		viperx.Reset()
	})

	_, err := GetDeployedLLMs()
	require.NoError(t, err)

	values, err := url.ParseQuery(<-queryCh)
	require.NoError(t, err)
	assert.Equal(t, targetTypeTextGeneration, values.Get("championModelTargetType"))
	assert.Equal(t, "100", values.Get("limit"))
}

func TestGetLLMsAndDeployed_BothFail(t *testing.T) {
	setupRoutedServer(t,
		routeResponse{status: http.StatusInternalServerError, body: "boom"},
		routeResponse{status: http.StatusInternalServerError, body: "boom"},
	)

	_, err := GetLLMsAndDeployed()
	assert.Error(t, err)
}

// TestGetLLMsAndDeployed_FetchesConcurrently makes each route block until the
// other one has arrived. A serial fetch deadlocks on that: the second request
// cannot start until the first returns, and the first is waiting on the second.
// It stalls until the timeout and fails. Only overlapping requests get through.
//
// Every other test in this file passes either way, so this is the only thing
// standing between a later refactor and a silent return to sum-of-both latency.
//
// The barrier keys on the route rather than counting requests: if either
// fixture grows a Next page, two pages from one source would satisfy a counter
// and the test would pass against a serial fetch.
func TestGetLLMsAndDeployed_FetchesConcurrently(t *testing.T) {
	var (
		mu    sync.Mutex
		seen  = map[string]bool{}
		both  = make(chan struct{})
		route = func(path string) string {
			if strings.Contains(path, "/deployments") {
				return "deployments"
			}

			return "catalog"
		}
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := route(r.URL.Path)

		mu.Lock()
		if !seen[name] {
			seen[name] = true

			if len(seen) == 2 {
				close(both)
			}
		}
		mu.Unlock()

		select {
		case <-both:
		case <-time.After(3 * time.Second):
			t.Errorf("%s did not overlap the other source: the two are being fetched serially", name)
		}

		body := gatewayBody
		if name == "deployments" {
			body = deployedBody
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))

	viperx.Reset()
	viperx.Set(config.DataRobotURL, srv.URL)
	viperx.Set(config.DataRobotAPIKey, "test-token")
	viperx.Set(config.SkipAuthKey, true)

	t.Cleanup(func() {
		srv.Close()
		viperx.Reset()
	})

	list, err := GetLLMsAndDeployed()
	require.NoError(t, err)
	assert.Len(t, list.LLMs, 2)
}

// TestGetLLMsAndDeployed_ColdTokenIsRaceFree drives the concurrent fetch with an
// unresolved token so both goroutines race to populate the `token` global. It
// only fails under -race, which is how the suite runs.
func TestGetLLMsAndDeployed_ColdTokenIsRaceFree(t *testing.T) {
	setupRoutedServer(t, routeResponse{body: gatewayBody}, routeResponse{body: deployedBody})

	previous := token
	token = ""

	t.Cleanup(func() { token = previous })

	list, err := GetLLMsAndDeployed()
	require.NoError(t, err)
	assert.Len(t, list.LLMs, 2)
}
