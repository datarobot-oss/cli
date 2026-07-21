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

package pipeline

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListTaskExecutions_URLAndDecode(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v2/pipelines/p-1/dispatches/d-1/tasks", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"taskId":1,"name":"load_data","status":"COMPLETED"},
			{"taskId":2,"name":"train_model","status":"RUNNING"}
		]`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	tasks, err := ListTaskExecutions("p-1", "d-1")
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	assert.Equal(t, "load_data", tasks[0].Name)
	assert.Equal(t, "COMPLETED", tasks[0].Status)
	require.NotNil(t, tasks[0].TaskID)
	assert.Equal(t, 1, *tasks[0].TaskID)
}

func TestGetTaskExecution_URLAndDecode(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v2/pipelines/p-1/dispatches/d-1/tasks/2", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"taskId":2,"name":"train_model","status":"FAILED","errorDetail":"OOM"}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	task, err := GetTaskExecution("p-1", "d-1", 2, nil)
	require.NoError(t, err)
	assert.Equal(t, "train_model", task.Name)
	assert.Equal(t, "FAILED", task.Status)
	require.NotNil(t, task.ErrorDetail)
	assert.Equal(t, "OOM", *task.ErrorDetail)
}

func TestListTaskExecutions_DecodesNodeAndGraphNodeID(t *testing.T) {
	installSkipAuth(t)

	// Fan-out: two invocations of the same @task share taskId 3 but carry
	// distinct nodeIds and graphNodeIds.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"taskId":3,"nodeId":4,"graphNodeId":0,"name":"to_dataframe","status":"COMPLETED"},
			{"taskId":3,"nodeId":7,"graphNodeId":1,"name":"to_dataframe","status":"RUNNING"}
		]`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	tasks, err := ListTaskExecutions("p-1", "d-1")
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	require.NotNil(t, tasks[0].NodeID)
	require.NotNil(t, tasks[1].NodeID)
	assert.Equal(t, 4, *tasks[0].NodeID)
	assert.Equal(t, 7, *tasks[1].NodeID)
	require.NotNil(t, tasks[0].GraphNodeID)
	assert.Equal(t, 0, *tasks[0].GraphNodeID)
	// Fan-out siblings share the taskId; the nodeId is what disambiguates them.
	assert.Equal(t, *tasks[0].TaskID, *tasks[1].TaskID)
}

func TestPerTaskEndpoints_SendNodeIDQueryWhenSet(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Every per-task fetch must forward the nodeId selector verbatim.
		assert.Equal(t, "7", r.URL.Query().Get("nodeId"))

		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/v2/pipelines/p-1/dispatches/d-1/tasks/3":
			_, _ = w.Write([]byte(`{"taskId":3,"nodeId":7,"name":"to_dataframe","status":"RUNNING"}`))
		case "/api/v2/pipelines/p-1/dispatches/d-1/tasks/3/result":
			_, _ = w.Write([]byte(`{"url":"https://s3/x","expiresIn":900,"valueAvailable":false}`))
		case "/api/v2/pipelines/p-1/dispatches/d-1/tasks/3/logs":
			_, _ = w.Write([]byte(`{"logs":"","filteredLineCount":0}`))
		case "/api/v2/pipelines/p-1/dispatches/d-1/tasks/3/logs/stdout":
			_, _ = w.Write([]byte(`{"content":"","totalBytes":0}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	node := 7

	_, err := GetTaskExecution("p-1", "d-1", 3, &node)
	require.NoError(t, err)

	_, err = GetTaskResult("p-1", "d-1", 3, &node)
	require.NoError(t, err)

	_, err = GetTaskLogs("p-1", "d-1", 3, &node, nil, "")
	require.NoError(t, err)

	_, err = GetTaskDurableLog("p-1", "d-1", 3, &node, "stdout", "")
	require.NoError(t, err)
}

func TestGetTaskLogs_URLQueryAndDecode(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/pipelines/p-1/dispatches/d-1/tasks/1/logs", r.URL.Path)
		assert.Equal(t, "50", r.URL.Query().Get("tail_lines"))
		assert.Equal(t, "all", r.URL.Query().Get("verbosity"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"logs":"hello from task\n","filteredLineCount":0}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	tail := 50
	logs, err := GetTaskLogs("p-1", "d-1", 1, nil, &tail, "all")
	require.NoError(t, err)
	assert.Equal(t, "hello from task\n", logs.Logs)
	assert.Equal(t, 0, logs.FilteredLineCount)
}

func TestGetTaskLogs_NoQueryParamsWhenAbsent(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/pipelines/p-1/dispatches/d-1/tasks/1/logs", r.URL.Path)
		assert.Empty(t, r.URL.Query().Get("tail_lines"))
		assert.Empty(t, r.URL.Query().Get("verbosity"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"logs":"","filteredLineCount":0}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	_, err := GetTaskLogs("p-1", "d-1", 1, nil, nil, "")
	require.NoError(t, err)
}

func TestGetTaskDurableLog_URLAndDecode(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/pipelines/p-1/dispatches/d-1/tasks/3/logs/stdout", r.URL.Path)
		assert.Equal(t, "user", r.URL.Query().Get("verbosity"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":"task output\n","contentType":"text/plain","totalBytes":12,"truncated":false,"filteredLineCount":2}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	log, err := GetTaskDurableLog("p-1", "d-1", 3, nil, "stdout", "user")
	require.NoError(t, err)
	assert.Equal(t, "task output\n", log.Content)
	assert.Equal(t, 12, log.TotalBytes)
	assert.False(t, log.Truncated)
	assert.Equal(t, 2, log.FilteredLineCount)
}

func TestGetTaskResult_URLAndDecode(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/pipelines/p-1/dispatches/d-1/tasks/1/result", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"url":"https://s3.example.com/result.tobj?sig=abc",
			"expiresIn":900,
			"contentType":"application/octet-stream",
			"value":42,
			"valueAvailable":true
		}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	res, err := GetTaskResult("p-1", "d-1", 1, nil)
	require.NoError(t, err)
	assert.Equal(t, "https://s3.example.com/result.tobj?sig=abc", res.URL)
	assert.Equal(t, 900, res.ExpiresIn)
	assert.True(t, res.ValueAvailable)
}

func TestGetTaskResult_ValueUnavailable(t *testing.T) {
	installSkipAuth(t)

	reason := "not_json_serializable"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		resp := TaskExecutionResult{
			URL:                    "https://s3.example.com/result.tobj",
			ExpiresIn:              900,
			ContentType:            "application/octet-stream",
			ValueAvailable:         false,
			ValueUnavailableReason: &reason,
		}

		data, _ := json.Marshal(resp)
		_, _ = w.Write(data)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	res, err := GetTaskResult("p-1", "d-1", 1, nil)
	require.NoError(t, err)
	assert.False(t, res.ValueAvailable)
	require.NotNil(t, res.ValueUnavailableReason)
	assert.Equal(t, "not_json_serializable", *res.ValueUnavailableReason)
}

func TestGetTaskResult_TextPreviewForNonSerializable(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// A DataFrame result: no JSON value, but a str() text preview
		// the task pod recorded is surfaced instead.
		_, _ = w.Write([]byte(`{
			"url":"https://s3.example.com/result.tobj",
			"expiresIn":900,
			"contentType":"application/octet-stream",
			"value":null,
			"valueAvailable":false,
			"valueUnavailableReason":"not_json_serializable",
			"valueText":"   x  y\n0  1  3\n1  2  4",
			"valueTextTruncated":true
		}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	res, err := GetTaskResult("p-1", "d-1", 1, nil)
	require.NoError(t, err)
	assert.False(t, res.ValueAvailable)
	assert.Equal(t, "   x  y\n0  1  3\n1  2  4", res.ValueText)
	assert.True(t, res.ValueTextTruncated)
}
