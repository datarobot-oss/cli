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
	"net/http"
	"sync"

	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
)

// Stable check identifiers for the remote checks. They surface in reports
// and JSON output, so they must not change between releases.
const (
	CheckIDArtifactExists  = "remote.artifact-exists"
	CheckIDArtifactLocked  = "remote.artifact-locked"
	CheckIDCatalogMismatch = "remote.catalog-mismatch"
	CheckIDDrift           = "remote.drift"
)

// ArtifactGetter is the doctor's remote seam: the small surface of the
// artifact API the remote checks depend on. Production uses
// ProductionArtifactGetter (which delegates to workload.GetArtifact); tests
// inject a fake so no network is touched.
type ArtifactGetter interface {
	// Get fetches the artifact by id, mirroring workload.GetArtifact.
	Get(artifactID string) (*workload.Artifact, error)
}

// ArtifactGetterFunc adapts a plain function to the ArtifactGetter seam,
// so the command layer can hand over its test seam variable directly.
type ArtifactGetterFunc func(artifactID string) (*workload.Artifact, error)

// Get implements ArtifactGetter.
func (f ArtifactGetterFunc) Get(artifactID string) (*workload.Artifact, error) {
	return f(artifactID)
}

// ProductionArtifactGetter returns the seam implementation backed by the
// real workload package (workload.GetArtifact).
func ProductionArtifactGetter() ArtifactGetter {
	return ArtifactGetterFunc(workload.GetArtifact)
}

// RemoteChecks returns the four remote sync-state checks in the fixed report
// order: artifact-exists, artifact-locked, catalog-mismatch, drift. All four
// share the single artifact snapshot fetched through store (exactly one
// GetArtifact per doctor run; TOCTOU within a run collapses to one read).
//
// Each check re-reads local state at Run time (same pattern as the local
// checks), so the SKIP cascades (unlinked project, unreadable config) are
// honest per-run observations rather than construction-time snapshots.
func RemoteChecks(projectDir string, store ArtifactGetter) []core.Check {
	snapshot := &remoteSnapshot{store: store}

	return []core.Check{
		&artifactExistsCheck{remoteBase{projectDir: projectDir, snapshot: snapshot}},
		&artifactLockedCheck{remoteBase{projectDir: projectDir, snapshot: snapshot}},
		&catalogMismatchCheck{remoteBase{projectDir: projectDir, snapshot: snapshot}},
		&driftCheck{remoteBase{projectDir: projectDir, snapshot: snapshot}},
	}
}

// remoteSnapshot lazily fetches the linked artifact exactly once and hands
// the same snapshot to every remote check in the run. If the fetch fails,
// every check sees the same error — a mid-run disappearance can produce at
// most one fetch, never a partial per-check picture.
type remoteSnapshot struct {
	store ArtifactGetter

	once sync.Once

	artifact *workload.Artifact

	err error
}

// get returns the shared snapshot, fetching it on first use with the given
// artifact id. Subsequent calls (whatever id they pass) return the memoized
// result: within one run the config cannot legitimately change identity.
func (s *remoteSnapshot) get(artifactID string) (*workload.Artifact, error) {
	s.once.Do(func() {
		s.artifact, s.err = s.store.Get(artifactID)
	})

	return s.artifact, s.err
}

// remoteBase is the shared plumbing of the four remote checks: the project
// directory plus the shared snapshot.
type remoteBase struct {
	projectDir string

	snapshot *remoteSnapshot
}

// linkedConfig resolves the local preconditions every remote check depends
// on: a linked state dir and a readable config carrying a usable artifact id.
// On failure it returns a SKIP result (honest cascade reporting) and ok=false.
func (b remoteBase) linkedConfig() (wapi.Config, core.Result, bool) {
	if res, skip := skipIfUnlinked(b.projectDir); skip {
		return wapi.Config{}, res, false
	}

	cfg, err := wapi.LoadConfig(b.projectDir)
	if err != nil {
		return wapi.Config{}, core.Result{
			Status:  core.StatusSKIP,
			Summary: "linked state is unreadable; cannot determine the linked artifact id",
			Remedy:  RemedyConfig,
		}, false
	}

	if normalizeStringPtr(&cfg.ArtifactID) == nil {
		return wapi.Config{}, core.Result{
			Status:  core.StatusSKIP,
			Summary: "config carries no usable artifact id",
			Remedy:  RemedyConfig,
		}, false
	}

	return cfg, core.Result{}, true
}

// fetchedArtifact returns the shared artifact snapshot after the local
// preconditions pass. A 404 maps to notFound=true (the caller decides
// whether that is its own FAIL or a SKIP); any other fetch failure maps to
// a SKIP with the connectivity remedy — never a misleading OK and never
// FAIL-as-deleted.
func (b remoteBase) fetchedArtifact(cfg wapi.Config) (art *workload.Artifact, res core.Result, notFound, ok bool) {
	art, err := b.snapshot.get(cfg.ArtifactID)
	if err == nil {
		return art, core.Result{}, false, true
	}

	if isNotFound(err) {
		// artifact-exists owns the deleted finding; the dependent checks
		// honestly SKIP rather than piling on with findings of their own.
		return nil, core.Result{
			Status:  core.StatusSKIP,
			Summary: "linked artifact not found; nothing to check",
		}, true, false
	}

	return nil, remoteSkipResult(err), false, false
}

// remoteSkipResult builds the SKIP every non-404 remote failure maps to. The
// remedy names re-authentication/connectivity and never mentions --relink:
// nothing here suggests the artifact is gone when the evidence only says the
// API is out of reach.
func remoteSkipResult(err error) core.Result {
	return core.Result{
		Status:  core.StatusSKIP,
		Summary: fmt.Sprintf("could not fetch the linked artifact: %s", err),
		Remedy:  RemedyRemoteConnectivity,
	}
}

// isNotFound reports whether err is the API's 404 (possibly wrapped),
// detected via drapi.HTTPError status rather than string matching.
func isNotFound(err error) bool {
	var httpErr *drapi.HTTPError

	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound
}

// codeRefCatalog returns the artifact's pinned catalog id with empty-vs-nil
// normalization: no usable codeRef or an empty field reads as absent.
func codeRefCatalog(art *workload.Artifact) *string {
	codeRef := workload.ExtractCodeRef(*art)

	if codeRef == nil {
		return nil
	}

	return normalizeStringPtr(&codeRef.CatalogID)
}

// codeRefVersion returns the artifact's catalog version id with the same
// empty-vs-nil normalization.
func codeRefVersion(art *workload.Artifact) *string {
	codeRef := workload.ExtractCodeRef(*art)

	if codeRef == nil {
		return nil
	}

	return normalizeStringPtr(&codeRef.CatalogVersionID)
}

// artifactExistsCheck verifies that the linked artifact still exists
// remotely. It is the only check allowed to interpret a 404, and the only
// one that FAILs on one.
type artifactExistsCheck struct {
	remoteBase
}

func (c *artifactExistsCheck) ID() string {
	return CheckIDArtifactExists
}

func (c *artifactExistsCheck) Name() string {
	return "Linked artifact exists"
}

// Run fetches the shared snapshot. 404 → FAIL (deleted, --relink remedy);
// any other failure → SKIP (connectivity remedy); success → OK.
func (c *artifactExistsCheck) Run(_ context.Context) core.Result {
	cfg, res, ok := c.linkedConfig()
	if !ok {
		return res
	}

	_, res, notFound, ok := c.fetchedArtifact(cfg)

	switch {
	case !ok && notFound:
		return core.Result{
			Status:  core.StatusFAIL,
			Summary: "linked artifact not found (deleted?)",
			Remedy:  RemedyRelink,
		}
	case !ok:
		return res
	}

	return core.Result{
		Status:  core.StatusOK,
		Summary: "linked artifact exists",
	}
}

// artifactLockedCheck reports whether the linked artifact is locked. Locking
// is one-way and blocks sync execution (preview still works), so it is a
// WARN with fixable=false, never a FAIL and never --fix repairable.
type artifactLockedCheck struct {
	remoteBase
}

func (c *artifactLockedCheck) ID() string {
	return CheckIDArtifactLocked
}

func (c *artifactLockedCheck) Name() string {
	return "Artifact lock state"
}

// Run judges only the lock state; a 404 SKIPs (artifact-exists owns that
// finding), any other fetch failure SKIPs with the connectivity remedy.
func (c *artifactLockedCheck) Run(_ context.Context) core.Result {
	cfg, res, ok := c.linkedConfig()
	if !ok {
		return res
	}

	art, res, _, ok := c.fetchedArtifact(cfg)
	if !ok {
		return res
	}

	if art.IsLocked() {
		return core.Result{
			Status:  core.StatusWARN,
			Summary: "artifact is locked: sync execute refused, preview still allowed",
			Remedy:  RemedyArtifactLocked,
			Fixable: false,
		}
	}

	return core.Result{
		Status:  core.StatusOK,
		Summary: "artifact is a draft (not locked)",
	}
}

// catalogMismatchCheck compares the locally pinned catalog id
// (config.CatalogID) with the artifact's codeRef catalog id. A mismatch
// means the artifact was re-pointed server-side; sync would target the wrong
// lineage, so it FAILs with a relink remedy. Both-absent (never synced)
// agrees.
type catalogMismatchCheck struct {
	remoteBase
}

func (c *catalogMismatchCheck) ID() string {
	return CheckIDCatalogMismatch
}

func (c *catalogMismatchCheck) Name() string {
	return "Catalog pin match"
}

// Run compares the two catalog pointers with empty≈nil normalization on both
// sides. A 404 SKIPs (artifact-exists owns it); other fetch failures SKIP.
func (c *catalogMismatchCheck) Run(_ context.Context) core.Result {
	cfg, res, ok := c.linkedConfig()
	if !ok {
		return res
	}

	art, res, _, ok := c.fetchedArtifact(cfg)
	if !ok {
		return res
	}

	local := normalizeStringPtr(cfg.CatalogID)

	remote := codeRefCatalog(art)

	// Comparisons anchor on the local pin: nothing pinned locally is the
	// healthy never-synced state, while a pin whose remote counterpart
	// vanished is as divergent as a different catalog id.
	switch {
	case local == nil:
		return core.Result{
			Status:  core.StatusOK,
			Summary: "catalog not pinned (never synced)",
		}
	case remote == nil:
		return catalogMismatchResult(local, remote)
	case *local == *remote:
		return core.Result{
			Status:  core.StatusOK,
			Summary: fmt.Sprintf("catalog pin matches the artifact (%s)", *local),
		}
	default:
		return catalogMismatchResult(local, remote)
	}
}

// catalogMismatchResult builds the FAIL for a one-sided or divergent catalog
// pin, naming both values (null when absent) so the report shows exactly
// which side moved.
func catalogMismatchResult(local, remote *string) core.Result {
	return core.Result{
		Status:  core.StatusFAIL,
		Summary: fmt.Sprintf("catalog mismatch: config pinned %s but artifact codeRef points at %s", ptrDisplay(local), ptrDisplay(remote)),
		Remedy:  RemedyCatalogMismatch,
	}
}

// driftCheck compares the artifact's current codeRef catalog version with
// the locally last-synced version. A difference means the remote moved on;
// the next sync reconciles (possibly overwriting), so it WARNs and points at
// a dry-run review. Both-absent (never synced) cannot drift.
type driftCheck struct {
	remoteBase
}

func (c *driftCheck) ID() string {
	return CheckIDDrift
}

func (c *driftCheck) Name() string {
	return "Remote version drift"
}

// Run compares the two version pointers with empty≈nil normalization. A 404
// SKIPs; other fetch failures SKIP with the connectivity remedy.
func (c *driftCheck) Run(_ context.Context) core.Result {
	cfg, res, ok := c.linkedConfig()
	if !ok {
		return res
	}

	art, res, _, ok := c.fetchedArtifact(cfg)
	if !ok {
		return res
	}

	local := normalizeStringPtr(cfg.LastSyncedVersionID)

	remote := codeRefVersion(art)

	// Anchored on the local baseline: before a first sync there is no
	// baseline to drift from, while a baseline whose remote counterpart
	// vanished has drifted as surely as one pointing elsewhere.
	switch {
	case local == nil:
		return core.Result{
			Status:  core.StatusOK,
			Summary: "no synced version yet; nothing to drift",
		}
	case remote == nil:
		return driftResult(local, remote)
	case *local == *remote:
		return core.Result{
			Status:  core.StatusOK,
			Summary: "in sync with the last synced version",
		}
	default:
		return driftResult(local, remote)
	}
}

// driftResult builds the WARN for divergent version pointers, naming both
// values and pointing at a dry-run review rather than an automatic repair.
func driftResult(local, remote *string) core.Result {
	return core.Result{
		Status: core.StatusWARN,

		Summary: fmt.Sprintf("remote version drifted: last synced %s but artifact codeRef now points at %s; next sync reconciles", ptrDisplay(local), ptrDisplay(remote)),

		Remedy: RemedyDrift,
	}
}
