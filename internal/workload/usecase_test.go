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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUseCaseCalls_AskTheUseCaseAPI(t *testing.T) {
	var got []string

	serveAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)

		switch r.URL.Path {
		case "/api/v2/useCases/":
			if r.URL.Query().Get("entityId") != "" {
				fmt.Fprint(w, `{"data":[]}`)

				return
			}

			fmt.Fprint(w, `{"data":[{"id":"68b0aa11bb22cc33dd44ee55","name":"Sales"}]}`)
		case "/api/v2/useCases/68b0aa11bb22cc33dd44ee55/resources/":
			fmt.Fprint(w, `{"data":[{"entityId":"e1"}]}`)
		default:
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{}`)
		}
	}))

	latest, err := LatestUseCase()
	require.NoError(t, err)
	assert.Equal(t, &UseCase{ID: "68b0aa11bb22cc33dd44ee55", Name: "Sales"}, latest)

	linked, err := WorkloadUseCase("6ae326a88ad1d49b09eaae94")
	require.NoError(t, err)
	assert.Nil(t, linked, "no row is no link")

	enclaves, err := UseCaseHasEnclaves("68b0aa11bb22cc33dd44ee55")
	require.NoError(t, err)
	assert.True(t, enclaves)

	require.NoError(t, LinkWorkloadToUseCase("68b0aa11bb22cc33dd44ee55", "6ae326a88ad1d49b09eaae94"))

	assert.Equal(t, []string{
		"GET /api/v2/useCases/?limit=1&orderBy=-updatedAt",
		"GET /api/v2/useCases/?entityId=6ae326a88ad1d49b09eaae94&entityType=workload&limit=1",
		"GET /api/v2/useCases/68b0aa11bb22cc33dd44ee55/resources/?entityType=enclave",
		"POST /api/v2/useCases/68b0aa11bb22cc33dd44ee55/workloads/6ae326a88ad1d49b09eaae94/?",
	}, got)
}
