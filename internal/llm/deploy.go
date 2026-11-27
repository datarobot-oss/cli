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
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/datarobot/cli/internal/drapi"
)

// Model source kinds an llm artifact accepts.
const (
	SourceHuggingFace = "huggingFace"
	SourceS3          = "s3"
)

// maxServedNameLength is the Workload API's limit on spec.model.servedName.
const maxServedNameLength = 63

// Capability is a serving capability setting: "" or "auto" omits it (on when
// the platform knows how), "on" requires it, "off" turns it off.
type Capability string

func (c Capability) field() (map[string]bool, error) {
	switch strings.ToLower(string(c)) {
	case "", "auto":
		return nil, nil
	case "on":
		return map[string]bool{"enabled": true}, nil
	case "off":
		return map[string]bool{"enabled": false}, nil
	default:
		return nil, fmt.Errorf("invalid capability %q: must be auto, on or off", string(c))
	}
}

// DeployOptions are what `dr llm deploy` turns into one POST /workloads/.
type DeployOptions struct {
	Name          string
	Description   string
	RecipeID      string
	RecipeVersion int
	Bundles       []string

	// Model; ignored when ArtifactID is set.
	Model       string
	Revision    string
	ModelSource string
	ServedName  string
	ToolCalling Capability
	Reasoning   Capability

	ArtifactID string

	TokenGrid        bool
	ConcurrencySlots int
	TokensPerSecond  int
}

var servedNameUnsafe = regexp.MustCompile(`[^a-z0-9._-]+`)

// DefaultServedName derives a served name from a model repository id: its
// last path segment, lowercased, with anything outside [a-z0-9._-] turned
// into a hyphen, trimmed to start and end with a letter or digit.
func DefaultServedName(model string) string {
	name := model
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}

	name = servedNameUnsafe.ReplaceAllString(strings.ToLower(name), "-")
	if len(name) > maxServedNameLength {
		name = name[:maxServedNameLength]
	}

	return strings.Trim(name, "._-")
}

// BuildDeployRequest is the POST /workloads/ body: an inline llm artifact
// (or an existing one by id) and a runtime naming the recipe. It does not
// require a recipe, so a deploy without one is an addition later, not a
// rewrite; `dr llm deploy` requires --recipe for now.
func BuildDeployRequest(o DeployOptions) (map[string]any, error) {
	if o.Name == "" {
		return nil, errors.New("--name is required")
	}

	if len(o.Bundles) == 0 {
		return nil, errors.New("--bundle is required: the GPU resource bundles to run the workers on")
	}

	body := map[string]any{"name": o.Name, "runtime": deployRuntime(o)}
	if o.Description != "" {
		body["description"] = o.Description
	}

	if o.ArtifactID != "" {
		body["artifactId"] = o.ArtifactID

		return body, nil
	}

	artifact, err := inlineArtifact(o)
	if err != nil {
		return nil, err
	}

	body["artifact"] = artifact

	return body, nil
}

// deployRuntime is the workers' bundles, the recipe and the Token Grid opt-in.
func deployRuntime(o DeployOptions) map[string]any {
	runtime := map[string]any{"workers": map[string]any{"resourceBundles": o.Bundles}}

	if o.RecipeID != "" {
		recipe := map[string]any{"id": o.RecipeID, "mode": "strict"}
		if o.RecipeVersion > 0 {
			recipe["version"] = o.RecipeVersion
		}

		runtime["recipe"] = recipe
	}

	if o.TokenGrid {
		grid := map[string]any{"enabled": true}
		if o.ConcurrencySlots > 0 {
			grid["concurrencySlots"] = o.ConcurrencySlots
		}

		if o.TokensPerSecond > 0 {
			grid["tokensPerSecond"] = o.TokensPerSecond
		}

		runtime["tokenGrid"] = grid
	}

	return runtime
}

func inlineArtifact(o DeployOptions) (map[string]any, error) {
	if o.Model == "" {
		return nil, errors.New("no model: pass --model, or --artifact-id to reuse an llm artifact")
	}

	source, err := modelSource(o)
	if err != nil {
		return nil, err
	}

	served := o.ServedName
	if served == "" {
		served = DefaultServedName(o.Model)
	}

	if served == "" {
		return nil, fmt.Errorf("cannot derive a served name from %q: pass --served-name", o.Model)
	}

	model := map[string]any{"source": source, "servedName": served}

	if err := setCapabilities(model, o); err != nil {
		return nil, err
	}

	return map[string]any{
		"name": o.Name + "-artifact",
		"type": "llm",
		"spec": map[string]any{"model": model},
	}, nil
}

func modelSource(o DeployOptions) (map[string]any, error) {
	kind := o.ModelSource
	if kind == "" {
		kind = SourceHuggingFace
	}

	if kind != SourceHuggingFace && kind != SourceS3 {
		return nil, fmt.Errorf("invalid --model-source %q: must be %s or %s", kind, SourceHuggingFace, SourceS3)
	}

	source := map[string]any{"kind": kind, "repoId": o.Model}
	if o.Revision != "" {
		source["revision"] = o.Revision
	}

	return source, nil
}

func setCapabilities(model map[string]any, o DeployOptions) error {
	for key, c := range map[string]Capability{"toolCalling": o.ToolCalling, "reasoning": o.Reasoning} {
		f, err := c.field()
		if err != nil {
			return err
		}

		if f != nil {
			model[key] = f
		}
	}

	return nil
}

// CreatedWorkload is what POST /workloads/ answers that deploy reports.
type CreatedWorkload struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	ArtifactID string `json:"artifactId"`
	Endpoint   string `json:"endpoint,omitempty"`
}

// Deploy creates the llm workload.
func Deploy(body map[string]any) (*CreatedWorkload, error) {
	u, err := drapi.EndpointURL("/workloads/", nil)
	if err != nil {
		return nil, err
	}

	var w CreatedWorkload

	if err := explain(drapi.PostJSON(u, "workload", body, &w)); err != nil {
		return nil, err
	}

	return &w, nil
}

// ServedName reads the served model name from an llm artifact's spec: the
// name clients send as "model", and the AI gateway routes on.
func ServedName(artifactID string) (string, error) {
	u, err := drapi.EndpointURL("/artifacts/"+url.PathEscape(artifactID)+"/", nil)
	if err != nil {
		return "", err
	}

	var a struct {
		Type string `json:"type"`
		Spec struct {
			Model struct {
				ServedName string `json:"servedName"`
			} `json:"model"`
		} `json:"spec"`
	}

	if err := get(u, "artifact", &a); err != nil {
		return "", err
	}

	if a.Type != "llm" {
		return "", fmt.Errorf("artifact %s is a %q artifact, not an llm one", artifactID, a.Type)
	}

	if a.Spec.Model.ServedName == "" {
		return "", fmt.Errorf("artifact %s has no served name", artifactID)
	}

	return a.Spec.Model.ServedName, nil
}

// RolloutRuntime is the runtime a rollout sends: the workload's current
// runtime with read-only fields (resolved*) removed, mounts reduced to their
// storage references (a replacement refuses inline storage), and
// runtime.recipe replaced. recipeVersion 0 unpins.
func RolloutRuntime(current map[string]any, recipeID string, recipeVersion int) (json.RawMessage, error) {
	if current == nil {
		current = map[string]any{}
	}

	runtime, ok := stripResolved(current).(map[string]any)
	if !ok {
		return nil, errors.New("workload runtime is not an object")
	}

	// A workload without mounts sends none: "mounts": null is not "no
	// mounts" to every reader.
	if workers, ok := runtime["workers"].(map[string]any); ok {
		if mounts := workers["mounts"]; mounts != nil {
			workers["mounts"] = mountReferences(mounts)
		} else {
			delete(workers, "mounts")
		}
	}

	recipe := map[string]any{"id": recipeID, "mode": "strict"}
	if recipeVersion > 0 {
		recipe["version"] = recipeVersion
	}

	runtime["recipe"] = recipe

	return json.Marshal(runtime)
}

// stripResolved deep-copies v without keys that start with "resolved": the
// server fills those in and refuses them as input.
func stripResolved(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))

		for k, val := range t {
			if strings.HasPrefix(k, "resolved") {
				continue
			}

			out[k] = stripResolved(val)
		}

		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = stripResolved(val)
		}

		return out
	default:
		return v
	}
}

func mountReferences(v any) any {
	mounts, ok := v.([]any)
	if !ok {
		return v
	}

	out := make([]any, 0, len(mounts))

	for _, m := range mounts {
		mount, ok := m.(map[string]any)
		if !ok {
			continue
		}

		if _, hasID := mount["storageId"]; hasID {
			delete(mount, "storage")
		}

		out = append(out, mount)
	}

	return out
}

// GetWorkloadDocument reads a workload as a document with its numbers kept
// exact (json.Number): a rollout sends the runtime back, and a float64 round
// trip would round integers above 2^53.
func GetWorkloadDocument(workloadID string) (map[string]any, error) {
	u, err := drapi.EndpointURL("/workloads/"+url.PathEscape(workloadID)+"/", nil)
	if err != nil {
		return nil, err
	}

	resp, err := drapi.Get(u, "workload")
	if err != nil {
		return nil, explain(err)
	}

	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()

	var doc map[string]any

	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("read workload %s: %w", workloadID, err)
	}

	return doc, nil
}

// RecipeOf is the recipe a workload's runtime names, as an id and a version
// (0 when unpinned). resolved prefers runtime.resolvedRecipe, which says what
// was scheduled; otherwise runtime.recipe, which says what was asked for.
func RecipeOf(runtime map[string]any, resolved bool) (string, int) {
	keys := []string{"recipe"}
	if resolved {
		keys = []string{"resolvedRecipe", "recipe"}
	}

	for _, key := range keys {
		ref, ok := runtime[key].(map[string]any)
		if !ok {
			continue
		}

		id, _ := ref["id"].(string)

		return id, intOf(ref["version"])
	}

	return "", 0
}

func intOf(v any) int {
	switch n := v.(type) {
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0
		}

		return int(i)
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}
