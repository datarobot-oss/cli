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

// Package llm is the client for serving LLMs through the Workload API: the
// recipes surface (/api/v2/llmRecipes/) and, later, llm workloads. Every call
// returns errors that errors.As can unpack into *drapi.HTTPError, with the
// Workload API's {"detail": ...} lifted into the message and the two feature
// gates (cluster switch, user flag) explained.
package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload/apiclient"
)

const recipesPath = "/llmRecipes/"

// maxPageSize is the Workload API's ceiling on a list page (1..100).
const maxPageSize = 100

// Recipe statuses as the Workload API reports them.
const (
	StatusAvailable = "available"
	StatusInvalid   = "invalid"
)

// User is the creator or last editor of a recipe.
type User struct {
	ID       string `json:"id"`
	FullName string `json:"fullName,omitempty"`
	Email    string `json:"email,omitempty"`
	Username string `json:"username,omitempty"`
}

// Display is the best human label for the user.
func (u *User) Display() string {
	if u == nil {
		return ""
	}

	for _, s := range []string{u.Username, u.Email, u.FullName, u.ID} {
		if s != "" {
			return s
		}
	}

	return ""
}

// Recipe is a recipe as the Workload API returns it. Payload is present on a
// single read and omitted from lists.
type Recipe struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Kind        string          `json:"kind,omitempty"`
	Status      string          `json:"status,omitempty"`
	Version     int             `json:"version,omitempty"`
	ContentHash string          `json:"contentHash,omitempty"`
	Model       json.RawMessage `json:"model,omitempty"`
	Descriptor  json.RawMessage `json:"descriptor,omitempty"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	Provenance  json.RawMessage `json:"provenance,omitempty"`
	Creator     *User           `json:"creator,omitempty"`
	UpdatedBy   *User           `json:"updatedBy,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// Diagnostic is one validation finding on a recipe's content.
type Diagnostic struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
}

// Diagnostics returns the validation findings under provenance.diagnostics.
func (r Recipe) Diagnostics() []Diagnostic {
	if len(r.Provenance) == 0 {
		return nil
	}

	var p struct {
		Diagnostics []Diagnostic `json:"diagnostics"`
	}

	if err := json.Unmarshal(r.Provenance, &p); err != nil {
		return nil
	}

	return p.Diagnostics
}

// ModelName is the model the recipe serves, read from its content. The
// Workload API returns it as a document whose shape follows the payload; the
// repository id is what users recognise.
func (r Recipe) ModelName() string {
	if len(r.Model) == 0 {
		return ""
	}

	var plain string
	if err := json.Unmarshal(r.Model, &plain); err == nil {
		return plain
	}

	var doc map[string]any
	if err := json.Unmarshal(r.Model, &doc); err != nil {
		return ""
	}

	for _, key := range []string{"repoId", "id", "hfId", "name"} {
		if s, ok := doc[key].(string); ok && s != "" {
			return s
		}
	}

	return ""
}

type recipePage struct {
	Data       []Recipe `json:"data"`
	TotalCount int      `json:"totalCount"`
	Next       string   `json:"next"`
}

// ListOptions are the list filters, one-to-one with the API's query params.
type ListOptions struct {
	Search    string
	Model     string
	Kind      string
	Backend   string
	GPUSku    string
	Status    string
	CreatedBy string
	IDs       []string
	Limit     int
	Offset    int
}

func (o ListOptions) query() url.Values {
	q := url.Values{}

	set := func(key, value string) {
		if value != "" {
			q.Set(key, value)
		}
	}

	set("search", o.Search)
	set("model", o.Model)
	set("kind", o.Kind)
	set("backend", o.Backend)
	set("gpuSku", o.GPUSku)
	set("status", o.Status)
	set("createdBy", o.CreatedBy)

	for _, id := range o.IDs {
		q.Add("ids", id)
	}

	q.Set("limit", strconv.Itoa(min(o.Limit, maxPageSize)))
	q.Set("offset", strconv.Itoa(o.Offset))

	return q
}

// ListRecipes returns up to opts.Limit recipes, following next links.
func ListRecipes(opts ListOptions) ([]Recipe, error) {
	if opts.Limit <= 0 {
		return nil, fmt.Errorf("invalid limit %d: must be positive", opts.Limit)
	}

	if opts.Offset < 0 {
		return nil, fmt.Errorf("invalid offset %d: must be non-negative", opts.Offset)
	}

	pageURL, err := drapi.EndpointURL(recipesPath, opts.query())
	if err != nil {
		return nil, err
	}

	var all []Recipe

	pages := newPager(pageURL)

	for pageURL != "" {
		var page recipePage

		if err := get(pageURL, "recipes", &page); err != nil {
			return nil, err
		}

		all = append(all, page.Data...)

		if len(all) >= opts.Limit {
			return all[:opts.Limit], nil
		}

		if pageURL, err = pages.next(page.Next, len(page.Data)); err != nil {
			return nil, err
		}
	}

	return all, nil
}

// GetRecipe reads one recipe with its content. version 0 reads the latest.
func GetRecipe(id string, version int) (*Recipe, error) {
	q := url.Values{}
	if version > 0 {
		q.Set("version", strconv.Itoa(version))
	}

	u, err := drapi.EndpointURL(recipesPath+url.PathEscape(id), q)
	if err != nil {
		return nil, err
	}

	var r Recipe

	if err := get(u, "recipe", &r); err != nil {
		return nil, err
	}

	return &r, nil
}

// CreateRequest is the body of POST /llmRecipes/.
type CreateRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Descriptor  json.RawMessage `json:"descriptor,omitempty"`
	Payload     json.RawMessage `json:"payload"`
}

// CreateRecipe registers a recipe. Content that fails policy is still created,
// with status invalid and the reasons in its diagnostics.
func CreateRecipe(req CreateRequest) (*Recipe, error) {
	u, err := drapi.EndpointURL(recipesPath, nil)
	if err != nil {
		return nil, err
	}

	var r Recipe

	if err := explain(drapi.PostJSON(u, "recipe", req, &r)); err != nil {
		return nil, err
	}

	return &r, nil
}

// PatchRequest is the body of PATCH /llmRecipes/{id}. Metadata changes stand
// alone; a new payload makes a new version and must name the one it replaces.
type PatchRequest struct {
	Name        *string         `json:"name,omitempty"`
	Description *string         `json:"description,omitempty"`
	Descriptor  json.RawMessage `json:"descriptor,omitempty"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	IfVersion   *int            `json:"ifVersion,omitempty"`
}

// Validate applies the Workload API's rules locally so the error is the CLI's.
func (p PatchRequest) Validate() error {
	if p.Payload == nil && p.Descriptor != nil {
		return errors.New("a descriptor change is sent together with a new payload: the content is versioned as one")
	}

	if p.Payload != nil && p.IfVersion == nil {
		return errors.New("--if-version is required with a new payload: name the version it replaces (see 'dr recipe get <id>')")
	}

	if p.Name == nil && p.Description == nil && p.Payload == nil {
		return errors.New("nothing to change: pass --name, --description or a new payload file")
	}

	return nil
}

// PatchRecipe changes a recipe's metadata or replaces its content.
func PatchRecipe(id string, req PatchRequest) (*Recipe, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	u, err := drapi.EndpointURL(recipesPath+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}

	var r Recipe

	if err := explain(drapi.PatchJSON(u, "recipe", req, &r)); err != nil {
		return nil, err
	}

	return &r, nil
}

// DeleteRecipe deletes a recipe (its creator or a tenant admin).
func DeleteRecipe(id string) error {
	u, err := drapi.EndpointURL(recipesPath+url.PathEscape(id), nil)
	if err != nil {
		return err
	}

	return explain(drapi.DeleteJSON(u, "recipe", nil, nil))
}

func get(u, info string, v any) error {
	return explain(drapi.GetJSON(u, info, v))
}

// explain lifts the Workload API's detail into the error and, for the two
// feature-gate 403s, says which gate is closed and who can open it.
func explain(err error) error {
	return explainGate(apiclient.LiftDetail(err))
}
