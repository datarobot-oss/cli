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

package llm

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/datarobot/cli/internal/drapi"
)

// RecipeRef is the recipe an llm workload asked for (runtime.recipe).
type RecipeRef struct {
	ID      string `json:"id"`
	Version int    `json:"version,omitempty"`
}

// ResolvedRecipe is the recipe version the workload was scheduled with.
type ResolvedRecipe struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

// Deployment is an llm workload, with the recipe fields of its runtime.
type Deployment struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Endpoint  string    `json:"endpoint,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	Runtime   struct {
		Recipe         *RecipeRef      `json:"recipe,omitempty"`
		ResolvedRecipe *ResolvedRecipe `json:"resolvedRecipe,omitempty"`
	} `json:"runtime"`
}

// RecipeID is the recipe the workload runs: the resolved one once
// scheduled, else the one it asked for.
func (d Deployment) RecipeID() string {
	if d.Runtime.ResolvedRecipe != nil {
		return d.Runtime.ResolvedRecipe.ID
	}

	if d.Runtime.Recipe != nil {
		return d.Runtime.Recipe.ID
	}

	return ""
}

// RecipeVersion is the version it runs: resolved, else pinned, else "latest".
func (d Deployment) RecipeVersion() string {
	if d.Runtime.ResolvedRecipe != nil {
		return strconv.Itoa(d.Runtime.ResolvedRecipe.Version)
	}

	if d.Runtime.Recipe != nil && d.Runtime.Recipe.Version > 0 {
		return strconv.Itoa(d.Runtime.Recipe.Version)
	}

	return "latest"
}

type deploymentPage struct {
	Data []Deployment `json:"data"`
	Next string       `json:"next"`
}

// RecipeDeployments returns up to limit llm workloads that run recipeID,
// optionally filtered by status. The Workload API has no recipe filter, so
// every llm workload in the tenant is read and matched here.
func RecipeDeployments(recipeID string, statuses []string, limit int) ([]Deployment, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("invalid limit %d: must be positive", limit)
	}

	q := url.Values{}
	q.Set("type", "llm")
	q.Set("limit", strconv.Itoa(maxPageSize))
	q.Set("offset", "0")

	for _, s := range statuses {
		q.Add("status", s)
	}

	pageURL, err := drapi.EndpointURL("/workloads/", q)
	if err != nil {
		return nil, err
	}

	var matched []Deployment

	for pageURL != "" {
		var page deploymentPage

		if err := get(pageURL, "workloads", &page); err != nil {
			return nil, err
		}

		matched = appendMatches(matched, page.Data, recipeID, limit)
		if len(matched) == limit {
			return matched, nil
		}

		if page.Next == "" {
			break
		}

		if err := drapi.AssertNextOnSameHost(page.Next); err != nil {
			return nil, err
		}

		pageURL = page.Next
	}

	return matched, nil
}

// appendMatches adds the workloads in page that run recipeID, up to limit.
func appendMatches(matched, page []Deployment, recipeID string, limit int) []Deployment {
	for _, d := range page {
		if len(matched) == limit {
			break
		}

		if d.RecipeID() == recipeID {
			matched = append(matched, d)
		}
	}

	return matched
}
