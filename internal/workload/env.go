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
	"strconv"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
)

// ResolveWorkloadArtifact fetches workloadID and the artifact it currently
// points at in one call. Every `dr workload env` subcommand needs both: the
// workload for its id and bound artifactId, the artifact to read or mutate
// environmentVars on.
func ResolveWorkloadArtifact(workloadID string) (*Workload, *Artifact, error) {
	wl, err := GetWorkload(workloadID)
	if err != nil {
		return nil, nil, err
	}

	artifact, err := GetArtifact(wl.ArtifactID)
	if err != nil {
		return nil, nil, err
	}

	return wl, artifact, nil
}

// PresentEnvironmentVarNames returns the subset of names that actually
// appear in vars, preserving names' order. Callers use this to short-circuit
// RemoveEnvironmentVars before it clones a locked artifact for what would
// turn out to be a no-op removal -- without this check, deleting a
// nonexistent name from a workload currently running a locked artifact
// would still create a throwaway (if harmless, deletable) clone before
// discovering nothing matched.
func PresentEnvironmentVarNames(vars []EnvironmentVar, names []string) []string {
	existing := make(map[string]bool, len(vars))
	for _, v := range vars {
		existing[v.Name] = true
	}

	present := make([]string, 0, len(names))

	for _, n := range names {
		if existing[n] {
			present = append(present, n)
		}
	}

	return present
}

// UpsertEnvironmentVars merges vars into the primary container's
// environmentVars by name (case-sensitive), replacing an existing entry in
// place or appending a new one. See ApplyEnvironmentVars for what
// targetArtifactID and needsLock mean.
func UpsertEnvironmentVars(artifactID string, vars []EnvironmentVar) (targetArtifactID string, needsLock bool, err error) {
	return ApplyEnvironmentVars(artifactID, func(current []any) ([]any, error) {
		existing, err := toEnvironmentVars(current)
		if err != nil {
			return nil, err
		}

		return toRawEnvironmentVars(upsertByName(existing, vars))
	})
}

// RemoveEnvironmentVars drops any environmentVars entries matching names from
// the primary container. removedNames reports which of the requested names
// were actually present, so callers can tell the difference between "no
// change needed" and "everything removed." See ApplyEnvironmentVars for what
// targetArtifactID and needsLock mean.
func RemoveEnvironmentVars(artifactID string, names []string) (targetArtifactID string, needsLock bool, removedNames []string, err error) {
	targetArtifactID, needsLock, err = ApplyEnvironmentVars(artifactID, func(current []any) ([]any, error) {
		existing, terr := toEnvironmentVars(current)
		if terr != nil {
			return nil, terr
		}

		var remaining []EnvironmentVar

		remaining, removedNames = removeByName(existing, names)

		return toRawEnvironmentVars(remaining)
	})

	return targetArtifactID, needsLock, removedNames, err
}

// ApplyEnvironmentVars runs mutate against the primary container's current
// environmentVars (as raw JSON values, matching mutate's []any signature) and
// writes the result back. It always leaves targetArtifactID in draft status
// -- locking (when required) is the caller's job, deferred to rollout time
// rather than done here, because a staged edit
// (dr workload env set/delete --stage) must stay inspectable and deletable
// until it is actually rolled out. Locked artifacts can never be deleted
// (see LockArtifact's doc), so locking eagerly here would leave an orphaned,
// permanent artifact behind every time --stage is used against a workload
// currently running a locked artifact.
//
// Per the artifact lifecycle rules (see internal/workload/artifact.go's
// LockArtifact doc and the workload-api skill's lifecycle-flows reference): a
// draft artifact is patched in place (needsLock is false: the running
// artifact was already a draft, so replacing onto another draft needs no
// lock). A locked artifact is immutable, so the change is applied to a new
// draft cloned from it instead (needsLock is true: the status-match rule
// requires the new artifact to be locked, like the one it replaces, before a
// replacement can be started).
func ApplyEnvironmentVars(artifactID string, mutate func([]any) ([]any, error)) (targetArtifactID string, needsLock bool, err error) {
	artifact, err := GetArtifact(artifactID)
	if err != nil {
		return "", false, err
	}

	if !artifact.IsLocked() {
		if err := patchEnvironmentVarsInPlace(artifactID, mutate); err != nil {
			return "", false, err
		}

		return artifactID, false, nil
	}

	clone, err := CreateArtifactClone(artifactID, cloneEnvName(artifact.Name))
	if err != nil {
		return "", false, fmt.Errorf("clone locked artifact %s: %w", artifactID, err)
	}

	if err := patchEnvironmentVarsInPlace(clone.ID, mutate); err != nil {
		// The clone already exists as an unlocked, deletable draft even
		// though this update failed -- name it so the caller isn't left
		// hunting for stray litter via 'dr artifact list'. It can be
		// retried by editing artifact clone.ID directly, or safely deleted.
		return "", false, fmt.Errorf("update environment vars on artifact %s (cloned from locked artifact %s): %w",
			clone.ID, artifactID, err)
	}

	return clone.ID, true, nil
}

// CreateArtifactClone POSTs to /api/v2/artifacts/{id}/clone/, the only way to
// produce an editable draft from a locked artifact (locking is one-way).
// name is the only field the endpoint requires; the response is a full new
// draft artifact carrying the source artifact's spec verbatim.
func CreateArtifactClone(artifactID, name string) (*Artifact, error) {
	url, err := config.GetEndpointURL("/api/v2/artifacts/" + escapeID(artifactID) + "/clone/")
	if err != nil {
		return nil, err
	}

	var artifact Artifact

	body := map[string]string{"name": name}

	if err := drapi.PostJSON(url, "artifact clone", body, &artifact); err != nil {
		return nil, err
	}

	return &artifact, nil
}

// cloneEnvName derives a name for the clone created to carry an env var edit
// off of a locked artifact. Timestamped so repeated edits against the same
// locked artifact never collide on name.
func cloneEnvName(originalName string) string {
	return originalName + "-env-" + strconv.FormatInt(time.Now().UnixMilli(), 10)
}

// patchEnvironmentVarsInPlace fetches artifactID as raw JSON (so unknown
// fields survive the round trip, matching PatchArtifactCodeRef's approach),
// mutates the primary container's environmentVars, and PATCHes the whole
// spec back -- the server replaces the entire containerGroups array on
// write, so every other container's fields must be preserved untouched.
func patchEnvironmentVarsInPlace(artifactID string, mutate func([]any) ([]any, error)) error {
	url, err := config.GetEndpointURL("/api/v2/artifacts/" + escapeID(artifactID) + "/")
	if err != nil {
		return err
	}

	var raw map[string]any

	if err := drapi.GetJSON(url, "artifact", &raw); err != nil {
		return fmt.Errorf("fetch artifact for environment var update: %w", err)
	}

	if err := mutateEnvironmentVarsInRawArtifact(raw, mutate); err != nil {
		return err
	}

	body := map[string]any{"spec": raw["spec"]}

	return drapi.PatchJSON(url, "artifact", body, nil)
}

// mutateEnvironmentVarsInRawArtifact locates the primary container within
// raw's spec.containerGroups and replaces its environmentVars with the
// result of mutate. This is intentionally a standalone traversal rather than
// a generalization of artifact.go's assignToPrimaryContainer/
// assignToFirstContainer (which are tested and working for codeRef): bending
// those into a shared mutate-callback for one new caller isn't worth the
// coupling risk.
func mutateEnvironmentVarsInRawArtifact(raw map[string]any, mutate func([]any) ([]any, error)) error {
	spec, ok := raw["spec"].(map[string]any)
	if !ok {
		return errors.New("artifact: spec missing or wrong type")
	}

	groups, ok := spec["containerGroups"].([]any)
	if !ok || len(groups) == 0 {
		return errors.New("artifact: spec.containerGroups missing or empty")
	}

	container, err := primaryContainerRaw(groups)
	if err != nil {
		return err
	}

	current, _ := container["environmentVars"].([]any)

	updated, err := mutate(current)
	if err != nil {
		return err
	}

	container["environmentVars"] = updated

	return nil
}

// primaryContainerRaw returns the raw container map flagged "primary": true,
// falling back to containerGroups[0].containers[0] -- the same selection
// rule as ExtractCodeRef/GetPrimaryContainerImageURI, applied to the raw JSON
// shape instead of the typed Artifact.
func primaryContainerRaw(groups []any) (map[string]any, error) {
	if container := findFlaggedPrimaryRaw(groups); container != nil {
		return container, nil
	}

	return firstContainerRaw(groups)
}

// findFlaggedPrimaryRaw scans every group for a container with
// "primary": true, returning nil (not an error) when none is found so the
// caller can fall back to the first-container rule.
func findFlaggedPrimaryRaw(groups []any) map[string]any {
	for _, g := range groups {
		group, ok := g.(map[string]any)
		if !ok {
			continue
		}

		containers, ok := group["containers"].([]any)
		if !ok {
			continue
		}

		for _, c := range containers {
			container, ok := c.(map[string]any)
			if !ok {
				continue
			}

			if primary, ok := container["primary"].(bool); ok && primary {
				return container
			}
		}
	}

	return nil
}

func firstContainerRaw(groups []any) (map[string]any, error) {
	firstGroup, ok := groups[0].(map[string]any)
	if !ok {
		return nil, errors.New("artifact: spec.containerGroups[0] missing or wrong type")
	}

	containers, ok := firstGroup["containers"].([]any)
	if !ok || len(containers) == 0 {
		return nil, errors.New("artifact: spec.containerGroups[0].containers missing or empty")
	}

	firstContainer, ok := containers[0].(map[string]any)
	if !ok {
		return nil, errors.New("artifact: spec.containerGroups[0].containers[0] missing or wrong type")
	}

	return firstContainer, nil
}

// toEnvironmentVars and toRawEnvironmentVars round-trip between the raw JSON
// []any shape used while patching an artifact and the typed []EnvironmentVar
// shape the upsert/remove logic operates on, so that logic never has to
// hand-build map[string]any values.
func toEnvironmentVars(raw []any) ([]EnvironmentVar, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}

	var vars []EnvironmentVar

	if err := json.Unmarshal(data, &vars); err != nil {
		return nil, err
	}

	return vars, nil
}

func toRawEnvironmentVars(vars []EnvironmentVar) ([]any, error) {
	data, err := json.Marshal(vars)
	if err != nil {
		return nil, err
	}

	raw := []any{}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	return raw, nil
}

// upsertByName replaces the entry named update.Name if one exists, else
// appends update. Order of untouched entries is preserved; new entries land
// at the end.
func upsertByName(existing, updates []EnvironmentVar) []EnvironmentVar {
	result := make([]EnvironmentVar, len(existing))

	copy(result, existing)

	for _, update := range updates {
		idx := indexByName(result, update.Name)
		if idx >= 0 {
			result[idx] = update

			continue
		}

		result = append(result, update)
	}

	return result
}

func indexByName(vars []EnvironmentVar, name string) int {
	for i, v := range vars {
		if v.Name == name {
			return i
		}
	}

	return -1
}

// removeByName drops every entry whose name appears in names, returning the
// survivors plus the subset of names that were actually found (so a caller
// asking to delete a nonexistent var can tell that apart from a no-op).
func removeByName(existing []EnvironmentVar, names []string) (remaining []EnvironmentVar, removed []string) {
	toRemove := make(map[string]bool, len(names))
	for _, n := range names {
		toRemove[n] = true
	}

	remaining = make([]EnvironmentVar, 0, len(existing))
	removed = make([]string, 0, len(names))

	for _, v := range existing {
		if toRemove[v.Name] {
			removed = append(removed, v.Name)

			continue
		}

		remaining = append(remaining, v)
	}

	return remaining, removed
}
