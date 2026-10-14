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

package sync

import (
	"errors"
	"fmt"
	"time"

	"github.com/datarobot/cli/internal/drapi/filesapi"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
)

// Options configures the Engine.
type Options struct {
	DryRun    bool
	ShowDiffs bool
	Yes       bool
	// PushOnly uploads local changes and leaves every remote-side change
	// as it is on both sides: nothing is downloaded or removed locally, and
	// the base keeps the old entry so the next plain sync still sees it.
	PushOnly bool

	// Quiet keeps the phases from logging the symlink and divergence notices.
	// For a caller that already ran another engine over the same tree and
	// reported them, so they are not said twice in one run.
	Quiet bool

	// Verify opts into the network-cost integrity checks: a remote
	// round-trip even when the artifact is not drifted, and post-apply
	// verification that the server holds what was uploaded. It changes how
	// much a run checks, not whether the run applies its plan, so it must
	// never be treated as a preview mode: previewOnly must not consider it,
	// and a Verify run without DryRun/ShowDiffs still reaches Execute.
	Verify bool
}

// Result is the outcome of a successful sync.
type Result struct {
	OldVersion      string // full version ID; "" before first sync
	NewVersion      string
	UploadedCount   int
	DownloadedCount int
	DeletedCount    int
	ConflictCount   int
	ConflictCopies  []string // every *.LOCAL.<ts> backup made this sync (conflicts and overwritten/deleted downloads alike)
	Duration        time.Duration

	// ExecutableNotice warns that image builds drop the executable bit of the
	// files it names; "" when this sync uploaded none. Returned rather than
	// logged, because up's progress display silences the stderr logger.
	ExecutableNotice string
}

var ErrNoPlan = errors.New("sync engine: Execute called before Plan")

// artifactStore is the small surface of the workload artifact API the
// engine depends on. The default implementation delegates to the
// workload package; tests inject a fake.
type artifactStore interface {
	Get(artifactID string) (*workload.Artifact, error)
	PatchCodeRef(artifactID, catalogID, catalogVersionID string) error
}

type workloadArtifactStore struct{}

func (workloadArtifactStore) Get(id string) (*workload.Artifact, error) {
	return workload.GetArtifact(id)
}

func (workloadArtifactStore) PatchCodeRef(artifactID, catalogID, catalogVersionID string) error {
	return workload.PatchArtifactCodeRef(artifactID, catalogID, catalogVersionID)
}

// Deps are the external dependencies injected into an Engine. Use
// defaultDeps for production wiring; tests build their own. A nil
// Lockfile or LockfileCheck falls back to the production runner
// (runUvLock, runUvLockCheck).
type Deps struct {
	Files         filesapi.Client
	Artifacts     artifactStore
	Now           func() time.Time
	Lockfile      LockfileRunner
	LockfileCheck LockfileChecker
}

func defaultDeps() Deps {
	return Deps{
		Files:         filesapi.New(),
		Artifacts:     workloadArtifactStore{},
		Now:           time.Now,
		Lockfile:      runUvLock,
		LockfileCheck: runUvLockCheck,
	}
}

// Engine wires the sync pipeline. Construct with New, call Plan, Execute,
// or Run, then Close to release the project lock.
type Engine struct {
	projectDir string
	opts       Options

	files     filesapi.Client
	artifacts artifactStore
	nowFn     func() time.Time

	config        wapi.Config
	base          BaseManifest
	artifact      *workload.Artifact
	remoteVer     string
	drifted       bool
	execBackfill  bool
	remoteListed  bool
	execNotice    string
	local         LocalManifest
	remote        RemoteManifest
	plan          *SyncPlan
	divergences   []Divergence
	lock          *SyncLock
	rollback      *Rollback
	newCatalogID  string
	newVersionID  string
	uploadOutcome *UploadOutcome
	localBackups  []string
	result        *Result
	startedAt     time.Time
	staleNote     bool
	migrationNote string
	ignoreNotice  string
	lockedNote    string

	// skippedSymlinks holds symlinks the walk did not follow, filtered
	// through the ignore matcher so deliberately-ignored or system-excluded
	// links are absent. Populated in Phase 2; exposed by the display layer.
	skippedSymlinks []SkippedSymlink

	lockfileFn        LockfileRunner
	lockfileCheckFn   LockfileChecker
	lockfileGenerated bool
	lockfileHint      string
}

// New constructs an Engine bound to projectDir with production deps.
func New(projectDir string, opts Options) (*Engine, error) {
	return newWithDeps(projectDir, opts, defaultDeps())
}

// newWithDeps is the test seam: callers in this package supply a custom
// Deps to inject fakes for Files, Artifacts, or Now.
func newWithDeps(projectDir string, opts Options, deps Deps) (*Engine, error) {
	if projectDir == "" {
		return nil, errors.New("sync.New: projectDir is required")
	}

	if deps.Lockfile == nil {
		deps.Lockfile = runUvLock
	}

	if deps.LockfileCheck == nil {
		deps.LockfileCheck = runUvLockCheck
	}

	return &Engine{
		projectDir:      projectDir,
		opts:            opts,
		files:           deps.Files,
		artifacts:       deps.Artifacts,
		nowFn:           deps.Now,
		lockfileFn:      deps.Lockfile,
		lockfileCheckFn: deps.LockfileCheck,
	}, nil
}

// Plan runs phases 0-4 and returns the SyncPlan. The lock acquired in
// Phase 0 is held until Close, Execute, or Run releases it.
func (e *Engine) Plan() (*SyncPlan, error) {
	e.startedAt = e.nowFn()

	err := runPhases(
		e,
		phase{name: "preflight", run: phase0Preflight},
		phase{name: "gather", run: phase1Gather},
		// Before the manifest walk so a generated uv.lock is collected,
		// diffed, and uploaded by the normal pipeline.
		phase{name: "lockfile", run: phaseLockfile},
		phase{name: "manifests", run: phase2Manifests},
		phase{name: "diff", run: phase3Diff},
		phase{name: "preview", run: phase4Preview},
	)
	if err != nil {
		return nil, e.joinReleaseErr(err)
	}

	// An empty plan never reaches the state phase, so what a backfill
	// learned is written here; a preview writes nothing.
	if !e.previewOnly() && e.plan.IsEmpty() {
		if err := persistExecutableBackfill(e); err != nil {
			log.Debug("Could not record the executable bits", "error", err)
		}
	}

	return e.plan, nil
}

// Execute runs the execute and state phases against the plan returned by
// Plan. The lock is released on completion (success or error). A failure to
// release the lock is joined into the returned error so callers see both.
func (e *Engine) Execute(plan *SyncPlan) (_ *Result, retErr error) {
	if e.plan == nil || plan == nil {
		return nil, e.joinReleaseErr(ErrNoPlan)
	}

	if plan != e.plan {
		// Caller may have shallow-copied the plan; the engine's own
		// plan remains the source of truth.
		_ = plan
	}

	defer func() {
		retErr = e.joinReleaseErr(retErr)
	}()

	if err := runPhases(
		e,
		phase{name: "execute", run: phase5Execute},
		phase{name: "state", run: phase7State},
	); err != nil {
		return nil, err
	}

	return e.result, nil
}

// Run is Plan + Execute. With DryRun or ShowDiffs it stops after Plan.
// An empty plan normally short-circuits before Execute too, but a --verify
// run that recorded BASE-vs-REMOTE divergences must still run the state
// phase: the plan has nothing to apply, yet the manifest on disk is a lie
// about the server, and that phase is what rewrites it from the real remote.
// The sharpest shape — BASE poisoned to A while disk and server both hold B
// — classifies as CONVERGED and plans nothing, so without this the poison
// survives the very run that detected it.
func (e *Engine) Run() (*Result, error) {
	plan, err := e.Plan()
	if err != nil {
		return nil, err
	}

	if e.previewOnly() || (plan.IsEmpty() && len(e.divergences) == 0) {
		if relErr := e.releaseLock(); relErr != nil {
			return nil, fmt.Errorf("release lock: %w", relErr)
		}

		return &Result{
			OldVersion: ptrOrEmpty(e.config.LastSyncedVersionID),
			Duration:   e.nowFn().Sub(e.startedAt),
		}, nil
	}

	return e.Execute(plan)
}

// Close releases the project lock. Idempotent.
func (e *Engine) Close() error {
	return e.releaseLock()
}

// StaleRollbackRestored reports whether Phase 0 restored a stale rollback
// from a previously crashed sync.
func (e *Engine) StaleRollbackRestored() bool { return e.staleNote }

// StateMigrationNotice is Phase 0's one-line account of where local state now
// lives, empty when the location did not change. The preview modes never
// migrate, so it is always empty for --dry-run and --diff.
func (e *Engine) StateMigrationNotice() string { return e.migrationNote }

// IgnoreFileNotice reports that the project's ignore patterns came from the
// deprecated filename, empty when they came from the current one or from
// nowhere. Set in Phase 2, so it is only meaningful after Plan.
//
// This one is returned rather than logged because it is housekeeping: the
// patterns still apply, and a caller emitting JSON needs it off stdout. The
// ignore-file problems that cost the user something go through log.Warn with
// the rest of the phase warnings, so they survive a later phase failing.
func (e *Engine) IgnoreFileNotice() string { return e.ignoreNotice }

// LockedNotice is Phase 1's one-line account of a plan computed against an
// artifact that can no longer take code, empty in the ordinary case. Only a
// preview can reach it, and a preview that printed a plan without saying so
// would read as a sync that is going to work.
func (e *Engine) LockedNotice() string { return e.lockedNote }

// Divergences reports the paths where BASE (manifest.json) and the real
// REMOTE listing disagree, as detected in Phase 2 of a --verify run. A
// non-drifted artifact without --verify never fetches the remote (the fast
// path copies BASE into REMOTE), so an empty result can mean "checked and
// clean", "nothing was checked", or "nothing can be checked" (first sync) —
// only a --verify run with a fetched remote populates the slice.
//
// The findings are diagnostics, not errors: the plan already reconciles them
// because the real remote is in hand, and the exit status must not change.
func (e *Engine) Divergences() []Divergence { return e.divergences }

// Verified reports whether the BASE-vs-REMOTE check ran: --verify was given
// and the artifact had not moved, so an empty Divergences means clean rather
// than unchecked.
func (e *Engine) Verified() bool { return e.opts.Verify && !e.drifted && e.remote != nil }

// SkippedSymlinks reports the symlinks the walk did not follow, filtered
// through the ignore matcher so deliberately-ignored or system-excluded
// links are absent. Each entry distinguishes a single skipped file from an
// entire omitted subtree (a directory symlink prunes all of its children).
// The findings are diagnostics: the plan already excludes them, and the
// exit status must not change.
func (e *Engine) SkippedSymlinks() []SkippedSymlink { return e.skippedSymlinks }

// previewOnly reports that this run stops after Plan and sends nothing to the
// platform, which is what makes the artifact's own mutability beside the point
// in phase 1. It is not a promise that the working tree is untouched: phase 0
// restores a stale rollback and the lockfile phase can still generate a
// uv.lock, both of which a preview needs in order to plan the tree it would
// actually upload.
//
// One owner for the question, because phase 1's locked-artifact exemption is
// only safe while the set of modes that skip Execute is the same set that gets
// exempted. Two spellings of it could drift into a preview that plans against
// something immutable and then executes.
func (e *Engine) previewOnly() bool { return e.opts.DryRun || e.opts.ShowDiffs }

func (e *Engine) releaseLock() error {
	if e.lock == nil {
		return nil
	}

	err := e.lock.Release()
	e.lock = nil

	if err != nil {
		log.Warn("failed to release sync lock", "err", err, "project", e.projectDir)
	}

	return err
}

// joinReleaseErr releases the lock and joins any release failure with
// err. The primary err is preserved; a release failure is reported
// alongside so callers see both. Returns nil if both are nil.
func (e *Engine) joinReleaseErr(err error) error {
	relErr := e.releaseLock()
	if relErr == nil {
		return err
	}

	return errors.Join(err, fmt.Errorf("release lock: %w", relErr))
}

func ptrOrEmpty(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

// resolveExistingCatalogID returns the catalog ID to reuse for an upload.
// Config wins because it is pinned for the artifact's DRAFT lifetime; the
// artifact's codeRef is the fallback for first-sync against an existing
// artifact. Returns "" when neither source has a catalog.
func resolveExistingCatalogID(e *Engine) string {
	if e.config.CatalogID != nil && *e.config.CatalogID != "" {
		return *e.config.CatalogID
	}

	return refFromArtifact(e).CatalogID
}

// catalogNameLabel prefixes the id in a new catalog's name, so the File
// Registry row says what the id is rather than leaving a bare hex string
// for a reader to guess at. It names the artifact, which is the object the
// id belongs to; the repository an artifact sits in is a separate entity
// in the Workload API and naming it here would point a reader at the wrong
// kind of thing.
const catalogNameLabel = "Artifact: "

// newCatalogName is the name to give a catalog this sync creates: the
// artifact pushing the code, so the File Registry row identifies its
// project instead of showing the platform's "Untitled Dataset" or, on the
// zip path, the internal "wapi-sync.zip".
//
// The artifact's name is deliberately not what goes here. Naming is a
// create-time parameter on the Files API, so whatever goes in is what the
// entry shows for the rest of its life, and a name the project is free to
// change afterwards would end up telling a reader something that had
// stopped being true, which is the same defect as the default titles and
// harder to spot. It would not identify the row either, since nothing
// stops two projects from choosing the same artifact name and `dr workload
// up` derives that name from the directory.
//
// The id names the artifact that created the entry, which is not always
// the one deploying from it later: every successful `dr workload up` locks
// the artifact it deployed, so the next deploy mints a fresh one in the
// same repository and repoints the project at it, while the catalog stays
// where it is. A locked artifact is kept rather than deleted, so the id
// goes on resolving and goes on being true about where this code came
// from; it is the first version of the lineage rather than the current
// one.
//
// Only the first sync of a project gets here, so an entry created before
// the CLI sent a name at all keeps whatever the platform gave it.
//
// Returns "" when the artifact is unknown, which leaves the name off the
// request. Phase 1 always fetches the artifact before Phase 5 uploads, so
// that is a guard against future reordering rather than a state a sync
// reaches today.
func newCatalogName(e *Engine) string {
	if e.artifact == nil {
		return ""
	}

	// An artifact the platform gave no id is not a case a sync reaches, since
	// the id is how phase 1 fetched it. The name is the only other thing here
	// worth showing, and it beats a label with nothing after it.
	if e.artifact.ID == "" {
		return filesapi.ClampCatalogName(e.artifact.Name)
	}

	// Label plus a platform id is nowhere near the API's ceiling, so unlike
	// the fallback this needs no budgeting.
	return catalogNameLabel + e.artifact.ID
}
