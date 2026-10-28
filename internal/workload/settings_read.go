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
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
)

// WorkloadSettings is GET /workloads/{id}/settings/: how much the workload
// runs with, as the platform holds it, and the replacement carrying a change
// that is still being rolled out, nil when none is.
//
// Runtime is kept as the platform sent it. It is re-emitted under JSON, and
// it is what a replica change is built from, whole, because the settings
// route takes a PATCH as the entire runtime rather than as a patch to it.
type WorkloadSettings struct {
	Runtime     json.RawMessage `json:"runtime"`
	Replacement *Replacement    `json:"replacement"`
}

// GetWorkloadSettings reads a workload's runtime settings.
func GetWorkloadSettings(workloadID string) (*WorkloadSettings, error) {
	url, err := config.GetEndpointURL("/api/v2/workloads/" + escapeID(workloadID) + "/settings/")
	if err != nil {
		return nil, err
	}

	var settings WorkloadSettings

	if err := drapi.GetJSON(url, "workload settings", &settings); err != nil {
		return nil, err
	}

	return &settings, nil
}

// RuntimeSettings is the runtime block as the CLI reads it, for display and
// for deciding what a replica change may touch. Only the fields the CLI shows
// or checks are typed; the block itself travels back through WithReplicaCount
// untyped, so a field the platform adds is neither lost nor misnamed.
type RuntimeSettings struct {
	ContainerGroups []ContainerGroupSettings `json:"containerGroups"`
}

// ContainerGroupSettings is one group's sizing. ReplicaCount is nil on a
// group that autoscales; ResolvedBundle is the platform's answer to the
// bundle request, nil where none was resolved.
type ContainerGroupSettings struct {
	Name                  string               `json:"name"`
	ReplicaCount          *int                 `json:"replicaCount"`
	ResourceBundles       []string             `json:"resourceBundles"`
	BundleSelectionPolicy string               `json:"bundleSelectionPolicy"`
	Autoscaling           *AutoscalingSettings `json:"autoscaling"`
	Containers            []ContainerSettings  `json:"containers"`
	ResolvedBundle        *ResolvedBundle      `json:"resolvedBundle"`
}

// AutoscalingSettings is a group's scaling policy.
type AutoscalingSettings struct {
	Enabled         bool            `json:"enabled"`
	MinReplicaCount *int            `json:"minReplicaCount"`
	MaxReplicaCount *int            `json:"maxReplicaCount"`
	Policies        []ScalingPolicy `json:"policies"`
}

// ScalingPolicy is one metric the autoscaler follows and its target.
type ScalingPolicy struct {
	ScalingMetric string  `json:"scalingMetric"`
	Target        float64 `json:"target"`
}

// ContainerSettings is one container's allocation.
type ContainerSettings struct {
	Name               string             `json:"name"`
	ResourceAllocation ResourceAllocation `json:"resourceAllocation"`
}

// ResourceAllocation is what a container is given. Memory is a number of
// bytes, which is how the platform answers; the manifest package knows how
// to spell it.
type ResourceAllocation struct {
	CPU    float64  `json:"cpu"`
	Memory int64    `json:"memory"`
	GPU    *float64 `json:"gpu"`
}

// ResolvedBundle is the resource bundle the platform picked for a group.
type ResolvedBundle struct {
	ID           string  `json:"id"`
	CPUCount     float64 `json:"cpuCount"`
	MemoryBytes  int64   `json:"memoryBytes"`
	GPUCount     int     `json:"gpuCount"`
	GPUMaker     *string `json:"gpuMaker"`
	GPUTypeLabel *string `json:"gpuTypeLabel"`
}

// Autoscales reports whether the group's replica count is the autoscaler's
// to set rather than a number of its own.
func (g ContainerGroupSettings) Autoscales() bool {
	return g.Autoscaling != nil && g.Autoscaling.Enabled
}

// Decode reads the typed view off the runtime block.
func (s *WorkloadSettings) Decode() (*RuntimeSettings, error) {
	var runtime RuntimeSettings

	if err := json.Unmarshal(s.Runtime, &runtime); err != nil {
		return nil, fmt.Errorf("cannot read the runtime settings: %w", err)
	}

	return &runtime, nil
}

// ErrGroupAutoscales reports a replica change asked of a group whose count
// belongs to the autoscaler.
var ErrGroupAutoscales = errors.New("the container group autoscales, so its replica count is not a number to set")

// WithReplicaCount is the runtime to send for a replica change on one group:
// the block as the platform holds it, re-sent whole with one number changed.
//
// Whole, because the route reads a PATCH as the entire runtime. Measured on
// staging: a body naming only the group and the count was accepted with a
// 202 and a replacement whose runtime had no containers and no bundles in
// it; that replacement then errored (ResourceBundleInferenceError: at least
// one resource signal is required) and the workload stayed as it was. The
// platform's own answer to the bundle request, resolvedBundle, is dropped on
// the way out: it is a result, not a request, and re-sending it would pin
// the platform to a choice it made once.
//
// A group that autoscales is refused: the autoscaler owns its count, and a
// number set beside an enabled policy is at best ignored. Changing the
// policy is a settings body, not a count.
func (s *WorkloadSettings) WithReplicaCount(group string, replicas int) (json.RawMessage, error) {
	var runtime map[string]any

	if err := json.Unmarshal(s.Runtime, &runtime); err != nil {
		return nil, fmt.Errorf("cannot read the runtime settings: %w", err)
	}

	groups, _ := runtime["containerGroups"].([]any)

	var (
		names []string
		found bool
	)

	for _, raw := range groups {
		g, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		name, _ := g["name"].(string)
		names = append(names, name)

		delete(g, "resolvedBundle")

		if name != group {
			continue
		}

		if autoscaling, ok := g["autoscaling"].(map[string]any); ok && autoscaling["enabled"] == true {
			return nil, fmt.Errorf("%w: %s", ErrGroupAutoscales, name)
		}

		g["replicaCount"] = replicas
		found = true
	}

	if !found {
		if len(names) == 0 {
			return nil, fmt.Errorf("no container group named %q; the workload has no container groups", group)
		}

		return nil, fmt.Errorf("no container group named %q; the workload has %s", group, strings.Join(names, ", "))
	}

	payload, err := json.Marshal(runtime)
	if err != nil {
		return nil, fmt.Errorf("cannot encode the runtime settings: %w", err)
	}

	return payload, nil
}

// RuntimeFromSettingsFile is the runtime block to send from a file the user
// wrote: the {"settings": ...} document the read prints, the {"runtime": ...}
// body the route takes, or the runtime block bare, which is what a manifest
// carries under the same name. All three, so that the read's output can be
// saved, edited and sent back as it is.
//
// The block is checked for what the route accepts and then fails on. A
// PATCH is the entire runtime, and the route answers 202 to a group with no
// containers or no resource signal; the replacement then errors on its own
// time, which without --wait nobody sees. So a group has to carry a name,
// its containers, and either resourceBundles or a resourceAllocation on
// every container, which is the platform's own list. The check reads the
// document loosely, since a file may spell memory the manifest's way
// ("512MB") where the platform answers in bytes.
func RuntimeFromSettingsFile(doc json.RawMessage) (json.RawMessage, error) {
	runtime, err := unwrapRuntime(doc)
	if err != nil {
		return nil, err
	}

	if err := checkRuntimeDoc(runtime); err != nil {
		return nil, err
	}

	return cleanRuntimeDoc(runtime)
}

// unwrapRuntime finds the runtime block in a document: under "settings" when
// the file is what `--output-format json` printed, under "runtime" when it is
// the body the route takes, or the document itself.
func unwrapRuntime(doc json.RawMessage) (json.RawMessage, error) {
	var body struct {
		Settings json.RawMessage `json:"settings"`
		Runtime  json.RawMessage `json:"runtime"`
	}

	if err := json.Unmarshal(doc, &body); err != nil {
		return nil, fmt.Errorf("the settings file is not a JSON object: %w", err)
	}

	if present(body.Settings) {
		return unwrapRuntime(body.Settings)
	}

	if present(body.Runtime) {
		return body.Runtime, nil
	}

	return doc, nil
}

func present(raw json.RawMessage) bool {
	return len(raw) > 0 && string(raw) != "null"
}

// cleanRuntimeDoc drops what the platform answers but does not take, and
// refuses what it takes and then fails on. resolvedBundle is in every
// document the read prints and is the platform's own answer to the bundle
// request; sent back, it would pin the platform to a choice it made once,
// which is why WithReplicaCount drops it too. A replicaCount beside an
// enabled autoscaling policy is the check `create` makes on the same block.
func cleanRuntimeDoc(doc json.RawMessage) (json.RawMessage, error) {
	var runtime map[string]any

	if err := json.Unmarshal(doc, &runtime); err != nil {
		return nil, fmt.Errorf("cannot read the settings file's runtime: %w", err)
	}

	groups, _ := runtime["containerGroups"].([]any)

	for i, raw := range groups {
		group, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		delete(group, "resolvedBundle")

		if err := validateContainerGroupAutoscaling(i, group); err != nil {
			return nil, fmt.Errorf("the settings file: %w", err)
		}
	}

	payload, err := json.Marshal(runtime)
	if err != nil {
		return nil, fmt.Errorf("cannot encode the settings file's runtime: %w", err)
	}

	return payload, nil
}

func checkRuntimeDoc(doc json.RawMessage) error {
	var runtime struct {
		ContainerGroups []map[string]json.RawMessage `json:"containerGroups"`
	}

	if err := json.Unmarshal(doc, &runtime); err != nil || len(runtime.ContainerGroups) == 0 {
		return errors.New("the settings file carries neither a runtime block nor containerGroups; " +
			"see 'dr workload settings <id> --output-format json' for the shape")
	}

	for i, group := range runtime.ContainerGroups {
		if err := checkGroupDoc(group); err != nil {
			return fmt.Errorf("the settings file's containerGroups[%d] %w", i, err)
		}
	}

	return nil
}

// checkGroupDoc is the platform's own conditions on a group, applied before
// the send: a name, at least one container, and a resource signal.
func checkGroupDoc(group map[string]json.RawMessage) error {
	var name string

	if json.Unmarshal(group["name"], &name) != nil || name == "" {
		return errors.New("needs a name")
	}

	var containers []map[string]json.RawMessage

	if json.Unmarshal(group["containers"], &containers) != nil || len(containers) == 0 {
		return fmt.Errorf("(%s) needs at least one container: the body replaces the whole runtime", name)
	}

	var bundles []string

	if json.Unmarshal(group["resourceBundles"], &bundles) == nil && len(bundles) > 0 {
		return nil
	}

	for _, c := range containers {
		var allocation map[string]any

		if json.Unmarshal(c["resourceAllocation"], &allocation) != nil || len(allocation) == 0 {
			return fmt.Errorf("(%s) needs a resource signal: resourceBundles on the group, "+
				"or a resourceAllocation on every container", name)
		}
	}

	return nil
}
