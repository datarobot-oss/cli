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
	"context"
	"errors"
	"time"

	"github.com/datarobot/cli/internal/drapi/filesapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
)

// Options configures the Engine.
type Options struct {
	DryRun    bool
	ShowDiffs bool
	Yes       bool
}

// Result is the outcome of a successful sync.
type Result struct {
	OldVersion      string // full version ID; "" before first sync
	NewVersion      string
	UploadedCount   int
	DownloadedCount int
	DeletedCount    int
	ConflictCount   int
	ConflictCopies  []string // *.LOCAL.<ts> paths created during sync
	Duration        time.Duration
}

var ErrNoPlan = errors.New("sync engine: Execute called before Plan")

// Engine wires the sync pipeline. Construct with New, call Plan, Execute,
// or Run, then Close to release the project lock.
type Engine struct {
	projectDir string
	opts       Options

	files           filesapi.Client
	getArtifactFn   func(string) (*workload.Artifact, error)
	patchArtifactFn func(artifactID, catalogID, catalogVersionID string) error
	nowFn           func() time.Time

	config         wapi.Config
	base           BaseManifest
	artifact       *workload.Artifact
	remoteVer      string
	drifted        bool
	local          LocalManifest
	remote         RemoteManifest
	plan           *SyncPlan
	lock           *SyncLock
	rollback       *Rollback
	newCatalogID   string
	newVersionID   string
	conflictCopies []string
	result         *Result
	startedAt      time.Time
	staleNote      bool
}

// New constructs an Engine bound to projectDir.
func New(projectDir string, opts Options) (*Engine, error) {
	if projectDir == "" {
		return nil, errors.New("sync.New: projectDir is required")
	}

	return &Engine{
		projectDir:      projectDir,
		opts:            opts,
		files:           filesapi.New(),
		getArtifactFn:   workload.GetArtifact,
		patchArtifactFn: workload.PatchArtifactCodeRef,
		nowFn:           time.Now,
	}, nil
}

// SetFilesClient swaps the FilesAPI client. Test seam.
func (e *Engine) SetFilesClient(c filesapi.Client) { e.files = c }

// SetGetArtifactFn swaps the artifact-fetch function. Test seam.
func (e *Engine) SetGetArtifactFn(fn func(string) (*workload.Artifact, error)) { e.getArtifactFn = fn }

// SetPatchArtifactFn swaps the artifact codeRef patch function. Test seam.
func (e *Engine) SetPatchArtifactFn(fn func(string, string, string) error) {
	e.patchArtifactFn = fn
}

// SetNowFn swaps time.Now for deterministic timestamps. Test seam.
func (e *Engine) SetNowFn(fn func() time.Time) { e.nowFn = fn }

// Plan runs phases 0-4 and returns the SyncPlan. The lock acquired in
// Phase 0 is held until Close, Execute, or Run releases it.
func (e *Engine) Plan(ctx context.Context) (*SyncPlan, error) {
	e.startedAt = e.nowFn()

	err := runPhases(ctx, e,
		phase{name: "preflight", run: phase0Preflight},
		phase{name: "gather", run: phase1Gather},
		phase{name: "manifests", run: phase2Manifests},
		phase{name: "diff", run: phase3Diff},
		phase{name: "preview", run: phase4Preview},
	)
	if err != nil {
		_ = e.releaseLock()

		return nil, err
	}

	return e.plan, nil
}

// Execute runs phases 5-6 against the plan returned by Plan. The lock
// is released on completion (success or error).
func (e *Engine) Execute(ctx context.Context, plan *SyncPlan) (*Result, error) {
	if e.plan == nil || plan == nil {
		_ = e.releaseLock()

		return nil, ErrNoPlan
	}

	if plan != e.plan {
		// Caller may have shallow-copied the plan; the engine's own
		// plan remains the source of truth.
		_ = plan
	}

	defer func() { _ = e.releaseLock() }()

	if err := runPhases(ctx, e,
		phase{name: "execute", run: phase5Execute},
		phase{name: "state", run: phase6State},
	); err != nil {
		return nil, err
	}

	return e.result, nil
}

// Run is Plan + Execute. With DryRun or ShowDiffs it stops after Plan.
func (e *Engine) Run(ctx context.Context) (*Result, error) {
	plan, err := e.Plan(ctx)
	if err != nil {
		return nil, err
	}

	if e.opts.DryRun || e.opts.ShowDiffs || plan.IsEmpty() {
		_ = e.releaseLock()

		return &Result{
			OldVersion: ptrOrEmpty(e.config.LastSyncedVersionID),
			Duration:   e.nowFn().Sub(e.startedAt),
		}, nil
	}

	return e.Execute(ctx, plan)
}

// Close releases the project lock. Idempotent.
func (e *Engine) Close() error {
	return e.releaseLock()
}

// StaleRollbackRestored reports whether Phase 0 restored a stale rollback
// from a previously crashed sync.
func (e *Engine) StaleRollbackRestored() bool { return e.staleNote }

func (e *Engine) releaseLock() error {
	if e.lock == nil {
		return nil
	}

	err := e.lock.Release()
	e.lock = nil

	return err
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
