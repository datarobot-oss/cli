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

package doctor

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"

	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/version"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/datarobot/cli/internal/workload/wapi"
)

// RelinkActionID is the stable identifier for the relink action in the
// actions array. It surfaces in JSON output and must not change between
// releases.
const RelinkActionID = "relink"

// RelinkWarning is the canonical warning text shown before a relink. It is
// reused by the command layer for both interactive and non-interactive paths
// and always routed to stderr (JSON purity on stdout).
const RelinkWarning = "Relink repoints the project at a new artifact and resets the sync baseline; the next sync reconciles against the new artifact."

// Sentinel errors for the relink operation. The command layer distinguishes
// ErrRelinkNotLinked and ErrRelinkAPIUnreachable (which print a message to
// stderr) from ErrRelinkAbort (which forces exit 1 without a separate
// message — the actions array already describes the reason).
var (
	// ErrRelinkNotLinked is returned when the project has no linked state.
	// The command layer prints this to stderr; the checks also show
	// local.presence FAIL with the init remedy.
	ErrRelinkNotLinked = errors.New("project is not linked; run 'dr artifact code init <artifact-id>' first")

	// ErrRelinkAPIUnreachable is returned when the API cannot be reached or
	// the credentials are unusable. Relink hard-requires the API (it must
	// fetch the target artifact to validate it).
	ErrRelinkAPIUnreachable = errors.New("cannot reach the DataRobot API; relink requires the API — run 'dr auth login' or fix network connectivity")

	// ErrRelinkAbort is a sentinel for abort cases where the actions array
	// already describes the reason (404, locked, wrong type, lock held,
	// declined). The command layer forces exit 1 without printing a
	// separate error message.
	ErrRelinkAbort = errors.New("relink aborted")

	// ErrRelinkPartial means config.json was repointed but a later write
	// failed, so the project is relinked with a stale manifest or history.
	// The action carries what was written; `doctor --fix` finishes it.
	ErrRelinkPartial = errors.New("relink incomplete: config.json was repointed but a later write failed; run 'dr artifact code doctor --fix'")
)

// RelinkConfirmFunc is called with the warning text after all safety gates
// pass. Returning true proceeds with the relink; returning false aborts with
// state untouched. The command layer provides the implementation:
// interactive TTY shows a [y/N] prompt (empty Enter declines); non-interactive
// (--yes or non-TTY) prints the warning and proceeds.
type RelinkConfirmFunc func(warning string) bool

// RelinkOptions configures a relink operation.
type RelinkOptions struct {
	// ProjectDir is the resolved absolute project directory.
	ProjectDir string

	// NewArtifactID is the bare-hex id of the artifact to relink to.
	NewArtifactID string

	// Store is the artifact-store seam used to fetch the target artifact.
	// Production uses workload.GetArtifact; tests inject a fake.
	Store ArtifactGetter

	// Confirm is called after all safety gates pass. The command layer
	// provides the interactive or non-interactive implementation.
	Confirm RelinkConfirmFunc

	// Goos is the injected platform seam for the lock probe. Production
	// uses runtime.GOOS; tests inject "windows" to exercise the SKIP path.
	Goos string

	// Now returns the current time for the history entry timestamp.
	// Production uses time.Now; tests inject a fixed clock.
	Now func() time.Time
}

// RunRelink executes the `doctor --relink <new-artifact-id>` operation for
// projectDir: an in-place repoint with a fresh-BASE reset.
//
// Safety gates (every abort leaves state byte-identical):
//  1. Lock probe (non-creating) — held by a live process → abort.
//  2. Not-linked project → error pointing to init.
//  3. Fetch new artifact — unreachable/unauthenticated → error abort.
//  4. Target 404 → abort.
//  5. Target locked → abort (cannot sync to a locked artifact).
//  6. Target type differs from the linked artifact's → abort.
//
// The sync lock is then held across the confirm and the writes, so a sync
// started in another terminal cannot undo the relink under the prompt.
// After all gates pass, the confirm function is called. On confirmation:
//   - Config rewritten (artifactId=new, catalogId=new codeRef.CatalogID
//     normalized empty→nil, lastSyncedVersionId=nil).
//   - Manifest reset to empty BASE (Files={}, synced fields nil).
//   - History.log appended {op:relink, from, to, ts}.
//   - Working tree untouched. Zero server writes.
//
// The same-id relink (target == currently linked) is allowed, warned, and
// resets BASE.
//
// The returned actions describe the relink; the returned error is non-nil for
// every abort case (the command layer forces exit 1).
func RunRelink(ctx context.Context, opts RelinkOptions) ([]core.Action, error) {
	if opts.Goos == "" {
		opts.Goos = runtime.GOOS
	}

	if opts.Now == nil {
		opts.Now = time.Now
	}

	// Gate 1: lock probe (non-creating, same as --fix's global safety gate).
	if actions, abort := relinkLockGate(ctx, opts); abort != nil {
		return actions, abort
	}

	// Gate 2: not-linked project → error pointing to init, before any network fetch.
	oldCfg, err := relinkLoadOldConfig(opts.ProjectDir)
	if err != nil {
		return nil, err
	}

	// Gate 3-5: fetch the target artifact and validate it (404, locked, type).
	art, fetchActions, err := relinkFetchAndValidate(opts, oldCfg)
	if err != nil {
		return fetchActions, err
	}

	// Same-id relink → allowed, warned, BASE reset.
	warning := relinkWarning(oldCfg.ArtifactID, opts.NewArtifactID)

	// Confirm prompt (defaults to No; empty Enter declines). A nil Confirm
	// function is treated as a decline so an internal caller that forgets to
	// set it cannot accidentally proceed with a destructive operation.
	confirm := opts.Confirm

	if confirm == nil {
		return relinkSkipped("no confirm function provided; relink declined as a safety default"), ErrRelinkAbort
	}

	// Held from here until the writes are done, so nothing can sync between
	// the question and the answer.
	lock, err := acquireRepairLock(opts.ProjectDir, opts.Goos)
	if err != nil {
		return relinkSkipped(ReasonSyncInProgress), ErrRelinkAbort
	}

	defer func() { _ = lock.Release() }()

	if !confirm(warning) {
		return relinkSkipped("declined by user"), ErrRelinkAbort
	}

	// All gates passed and the user confirmed. Perform the writes.
	return relinkWrite(opts, oldCfg, art)
}

// acquireRepairLock takes the sync lock for a repair. On Windows the lock is
// not enforced, so a nil lock stands in and Release is a no-op.
func acquireRepairLock(projectDir, goos string) (*sync.SyncLock, error) {
	if goos == "windows" {
		return nil, nil
	}

	return sync.AcquireSyncLock(projectDir)
}

// relinkLockGate probes the sync lock (non-creating). Returns (nil, nil) when
// the gate is open; (skippedActions, ErrRelinkAbort) when a live process holds
// the lock or it cannot be inspected.
func relinkLockGate(ctx context.Context, opts RelinkOptions) ([]core.Action, error) {
	switch gate := newLockCheckWithGoos(opts.ProjectDir, opts.Goos).Run(ctx); gate.Status {
	case core.StatusFAIL:
		return relinkSkipped(ReasonSyncInProgress), ErrRelinkAbort
	case core.StatusWARN:
		return relinkSkipped(ReasonLockUninspectable), ErrRelinkAbort
	case core.StatusOK, core.StatusSKIP:
		// OK: nothing held. SKIP: Windows (flock not enforced). Both let the
		// relink through.
		return nil, nil
	}

	// Unreachable: Status is an exhaustive enum.
	return nil, nil
}

// relinkLoadOldConfig loads the current config to get the old artifact id.
// Returns ErrRelinkNotLinked when the project has no state directory. A
// missing or unreadable config.json inside one is relinked over: the relink
// is the only repair for it, since nothing else can name the artifact.
func relinkLoadOldConfig(projectDir string) (wapi.Config, error) {
	if !wapi.Exists(projectDir) {
		return wapi.Config{}, ErrRelinkNotLinked
	}

	cfg, err := wapi.LoadConfig(projectDir)
	if err != nil {
		return wapi.Config{}, nil
	}

	return cfg, nil
}

// relinkFetchAndValidate fetches the target artifact and runs the 404, locked,
// and type gates. The type has to match the linked artifact's, since a
// project's lineage never changes kind; an unreadable config has no kind to
// match, so the gate is skipped. Returns (artifact, nil, nil) on success;
// (nil, skippedActions, ErrRelinkAbort) for 404/locked/wrong-type;
// (nil, nil, ErrRelinkAPIUnreachable) for any other fetch failure.
func relinkFetchAndValidate(opts RelinkOptions, oldCfg wapi.Config) (*workload.Artifact, []core.Action, error) {
	art, err := opts.Store.Get(opts.NewArtifactID)
	if err != nil {
		if isNotFound(err) {
			return nil, relinkSkipped(fmt.Sprintf(
				"target artifact %s not found (deleted?)", opts.NewArtifactID,
			)), ErrRelinkAbort
		}

		return nil, nil, fmt.Errorf("%w: %w", ErrRelinkAPIUnreachable, err)
	}

	if art.IsLocked() {
		return nil, relinkSkipped(fmt.Sprintf(
			"target artifact %s is locked; cannot sync to a locked artifact — use a draft or relink to one",
			opts.NewArtifactID,
		)), ErrRelinkAbort
	}

	if oldCfg.ArtifactID == "" {
		return art, nil, nil
	}

	old, err := opts.Store.Get(oldCfg.ArtifactID)
	if err != nil {
		// The old artifact is gone or unreadable: nothing to match against.
		return art, nil, nil
	}

	want := manifest.ArtifactTypeOrDefault(old.Type)

	if !manifest.SameArtifactType(manifest.ArtifactTypeOrDefault(art.Type), want) {
		return nil, relinkSkipped(fmt.Sprintf(
			"target artifact %s has type %q, not %q; cross-type lineage is refused",
			opts.NewArtifactID, manifest.ArtifactTypeOrDefault(art.Type), want,
		)), ErrRelinkAbort
	}

	return art, nil, nil
}

// relinkWarning builds the warning text, adding a same-id note when the
// target is the same as the currently linked artifact.
func relinkWarning(oldID, newID string) string {
	warning := RelinkWarning

	if oldID == "" {
		warning += "\nNote: the current config.json is unreadable and will be replaced."
	}

	if oldID == newID {
		warning = fmt.Sprintf("%s\nNote: re-linking to the same artifact (%s); the sync baseline will be reset.", warning, newID)
	}

	return warning
}

// relinkWrite performs the config/manifest/history writes after all gates pass
// and the user confirms. A failed SaveConfig is a skipped action with state
// untouched. A failure after it is a performed action with ErrRelinkPartial:
// the config is already repointed, and `--fix` rebuilds the rest from it.
func relinkWrite(opts RelinkOptions, oldCfg wapi.Config, art *workload.Artifact) ([]core.Action, error) {
	// Everything the old config carried survives except what names the old
	// artifact or its baseline. A config that could not be read is replaced
	// from scratch.
	newCfg := oldCfg
	newCfg.ArtifactID = opts.NewArtifactID
	newCfg.CatalogID = codeRefCatalog(art)
	newCfg.LastSyncedVersionID = nil
	newCfg.LastBuiltVersionID = nil
	newCfg.RemoteChangesSkipped = false

	if newCfg.CreatedAt.IsZero() {
		newCfg.CreatedAt = opts.Now()
	}

	if newCfg.CLIVersion == "" {
		newCfg.CLIVersion = version.Version
	}

	if err := wapi.SaveConfig(opts.ProjectDir, newCfg); err != nil {
		return relinkSkipped(fmt.Sprintf("write config: %v", err)), ErrRelinkAbort
	}

	newManifest := wapi.Manifest{
		Version: wapi.ManifestVersion,
		Files:   map[string]wapi.FileMeta{},
	}

	repointed := fmt.Sprintf("repointed from %s to %s", oldCfg.ArtifactID, opts.NewArtifactID)

	if err := wapi.SaveManifest(opts.ProjectDir, newManifest); err != nil {
		return relinkPartial(repointed + "; write manifest: " + err.Error()), ErrRelinkPartial
	}

	historyEntry := wapi.HistoryEntry{
		"op":   "relink",
		"from": oldCfg.ArtifactID,
		"to":   opts.NewArtifactID,
		"ts":   opts.Now().UTC().Format(time.RFC3339),
	}

	if err := wapi.AppendHistory(opts.ProjectDir, historyEntry); err != nil {
		return relinkPartial(repointed + "; sync baseline reset; append history: " + err.Error()), ErrRelinkPartial
	}

	return []core.Action{{
		ID:     RelinkActionID,
		Status: core.ActionPerformed,
		Reason: repointed + "; sync baseline reset",
	}}, nil
}

// relinkPartial is the action for a relink that repointed the config and
// then failed: performed, because it was, with the failure in the reason.
func relinkPartial(reason string) []core.Action {
	return []core.Action{{
		ID:     RelinkActionID,
		Status: core.ActionPerformed,
		Reason: reason,
	}}
}

// relinkSkipped builds a single skipped action for an abort case.
func relinkSkipped(reason string) []core.Action {
	return []core.Action{{
		ID:     RelinkActionID,
		Status: core.ActionSkipped,
		Reason: reason,
	}}
}
