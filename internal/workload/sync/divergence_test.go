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
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/ignore"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// diskBytes reads a file straight off the project tree so tests can seed the
// fake server with exactly the bytes the local side holds (or deliberately
// different ones) without re-deriving them.
func diskBytes(t *testing.T, dir, rel string) []byte {
	t.Helper()

	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	require.NoError(t, err)

	return b
}

// seededServerContents builds a version payload whose entries match the given
// project files byte for byte, plus optional overrides for paths that must
// diverge. Including .drignore in the default set keeps the ignore template
// from showing up as an unrelated base-only divergence.
func seededServerContents(t *testing.T, dir string, files map[string]string, overrides map[string][]byte) map[string][]byte {
	t.Helper()

	contents := make(map[string][]byte, len(files)+1)

	contents[ignore.FileName] = diskBytes(t, dir, ignore.FileName)

	for rel := range files {
		contents[rel] = diskBytes(t, dir, rel)
	}

	for rel, body := range overrides {
		contents[rel] = body
	}

	return contents
}

func TestDetectDivergence(t *testing.T) {
	base := BaseManifest{
		"same.py":    {Hash: "h-same", Size: 1},
		"changed.py": {Hash: "h-old", Size: 2},
		"gone.py":    {Hash: "h-gone", Size: 3},
	}

	remote := BaseManifest{
		"same.py":    {Hash: "h-same", Size: 1},
		"changed.py": {Hash: "h-new", Size: 4},
		"stray.py":   {Hash: "h-stray", Size: 5},
	}

	got := detectDivergence(base, remote)

	require.Len(t, got, 3, "every divergent path must be reported, not just the first")

	// Deterministic order: sorted by path, so a notice naming several paths
	// is stable across runs.
	assert.Equal(t, []Divergence{
		{Path: "changed.py", Kind: DivergenceHashMismatch, BaseHash: "h-old", RemoteHash: "h-new"},
		{Path: "gone.py", Kind: DivergenceBaseOnly, BaseHash: "h-gone"},
		{Path: "stray.py", Kind: DivergenceRemoteOnly, RemoteHash: "h-stray"},
	}, got)
}

func TestDetectDivergence_NoneAndEmpty(t *testing.T) {
	both := BaseManifest{"a.py": {Hash: "h1", Size: 1}}

	assert.Empty(t, detectDivergence(both, both), "identical manifests must not diverge")
	assert.Empty(t, detectDivergence(BaseManifest{}, BaseManifest{}), "empty BASE and empty REMOTE (first sync) must not diverge")
}

// The prose must let a reader tell the three kinds apart: a hash difference is
// not the same fact as a path existing on one side only.
func TestDivergenceNoticeText(t *testing.T) {
	mismatch := divergenceNotice(Divergence{
		Path: "app.py", Kind: DivergenceHashMismatch,
		BaseHash: "aaaa", RemoteHash: "bbbb",
	})
	baseOnly := divergenceNotice(Divergence{Path: "gone.py", Kind: DivergenceBaseOnly, BaseHash: "cccc"})
	remoteOnly := divergenceNotice(Divergence{Path: "stray.py", Kind: DivergenceRemoteOnly, RemoteHash: "dddd"})

	assert.Contains(t, mismatch, "app.py")
	assert.Contains(t, mismatch, "aaaa")
	assert.Contains(t, mismatch, "bbbb")
	assert.Contains(t, mismatch, "differs", "a hash mismatch must say the hashes differ")

	assert.Contains(t, baseOnly, "gone.py")
	assert.Contains(t, baseOnly, "BASE")
	assert.NotContains(t, baseOnly, "differs", "base-only must not read as a hash comparison")

	assert.Contains(t, remoteOnly, "stray.py")
	assert.Contains(t, remoteOnly, "REMOTE")

	assert.NotEqual(t, mismatch, baseOnly)
	assert.NotEqual(t, baseOnly, remoteOnly)
}

// TestMaybeDetectDivergence_WarnBoundedAtFive verifies that the per-path
// divergence warnings are bounded the same way the sibling symlink notices
// are: the first DivergenceNoticeBound paths in deterministic order, then a
// count of the remainder. The engine's divergences field still carries every
// path (the plan JSON lists the full set).
func TestMaybeDetectDivergence_WarnBoundedAtFive(t *testing.T) {
	remote := BaseManifest{}

	for i := 0; i < 7; i++ {
		name := fmt.Sprintf("%c.py", rune('a'+i))
		remote[name] = FileEntry{Hash: "h", Size: 1}
	}

	e := &Engine{opts: Options{Verify: true}, base: BaseManifest{}, remote: remote}

	logged := captureWarnLog(t, func() { maybeDetectDivergence(e) })

	// The structured field carries every divergence regardless of the prose
	// bound.
	require.Len(t, e.divergences, 7)

	// The first five appear in the prose.
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("%c.py", rune('a'+i))
		assert.Contains(t, logged, name,
			"the first %d divergences must appear in the bounded prose", 5)
	}

	// The sixth and seventh do not appear individually.
	assert.NotContains(t, logged, "f.py",
		"the 6th divergence must not appear in the bounded prose")
	assert.NotContains(t, logged, "g.py",
		"the 7th divergence must not appear in the bounded prose")

	// The count of the remainder appears.
	assert.Contains(t, logged, "2 more divergence(s)",
		"the count of the remainder must appear in the prose")
}

// engineFor returns an engine bound to dir with the given options and a fake
// artifact store pointing at the given catalog/version — the common fixture
// shape of every Phase-2 divergence test.
func engineFor(t *testing.T, dir string, opts Options, fake *fakeFilesClient, catalogID, versionID string) *Engine {
	t.Helper()

	e, err := newWithDeps(dir, opts, Deps{
		Files: fake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, catalogID, versionID), nil
			},
		},
		Now: time.Now,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	return e
}

// The flagship poisoned-manifest scenario at the engine level: local == base,
// the server holds different bytes for the same version, and only --verify
// looks. Expect exactly one AllFiles call, the divergence recorded on the
// engine, and a reconciling download row in the plan instead of "Up to date.".
func TestPhase2_VerifyForcesAllFilesAndDetectsDivergence(t *testing.T) {
	const (
		catalogID = "cid-1"
		versionID = "ver-1"
	)

	serverAppPy := []byte("server holds different bytes\n")
	localAppPy := "print('A')\n"

	dir := syncedProject(t, map[string]string{"app.py": localAppPy}, catalogID, versionID)

	fake := (&fakeFilesClient{}).withVersionContent(catalogID, versionID, seededServerContents(t, dir, map[string]string{
		"app.py": localAppPy,
	}, map[string][]byte{"app.py": serverAppPy}))

	e := engineFor(t, dir, Options{Verify: true}, fake, catalogID, versionID)

	plan, err := e.Plan()
	require.NoError(t, err)

	assert.Equal(t, 1, fake.AllFilesCalls(), "--verify must force exactly one AllFiles round-trip on a non-drifted artifact")

	divs := e.Divergences()
	require.Len(t, divs, 1)

	assert.Equal(t, "app.py", divs[0].Path)
	assert.Equal(t, DivergenceHashMismatch, divs[0].Kind)
	assert.Equal(t, sha256Hex([]byte(localAppPy)), divs[0].BaseHash)
	assert.Equal(t, sha256Hex(serverAppPy), divs[0].RemoteHash)

	// The real remote is now known, so Diff produces the reconciling row.
	require.Len(t, plan.Downloads, 1, "the plan must reconcile BASE with the real REMOTE instead of skipping")
	assert.Equal(t, "app.py", plan.Downloads[0].Path)
	assert.Equal(t, ClsRemoteModified, plan.Downloads[0].Classification)
}

// Hard rule 9: the default sync must stay exactly as it was — no AllFiles
// round-trip on a non-drifted artifact and no divergence findings.
func TestPhase2_DefaultPath_NoAllFiles_NoDivergence(t *testing.T) {
	const (
		catalogID = "cid-1"
		versionID = "ver-1"
	)

	serverAppPy := []byte("server holds different bytes\n")
	localAppPy := "print('A')\n"

	dir := syncedProject(t, map[string]string{"app.py": localAppPy}, catalogID, versionID)

	fake := (&fakeFilesClient{}).withVersionContent(catalogID, versionID, seededServerContents(t, dir, map[string]string{
		"app.py": localAppPy,
	}, map[string][]byte{"app.py": serverAppPy}))

	e := engineFor(t, dir, Options{}, fake, catalogID, versionID)

	plan, err := e.Plan()
	require.NoError(t, err)

	assert.Zero(t, fake.AllFilesCalls(), "the fast path must not fetch AllFiles")
	assert.Empty(t, e.Divergences(), "no divergence findings without --verify")
	assert.True(t, plan.IsEmpty(), "without --verify the poisoned BASE stays invisible (the defect --verify exists to expose)")
}

// On an already-drifted artifact the drift path fetches AllFiles anyway, so
// --verify must not add a second fetch — and BASE-vs-REMOTE differences are
// ordinary drift there, not findings.
func TestPhase2_VerifyOnDriftedArtifact_OneAllFilesCall_NoDivergence(t *testing.T) {
	const (
		catalogID  = "cid-1"
		lastSynced = "ver-1"
		currentVer = "ver-2"
	)

	files := map[string]string{"app.py": "print('A')\n"}

	dir := syncedProject(t, files, catalogID, lastSynced)

	fake := (&fakeFilesClient{}).withVersionContent(catalogID, currentVer, seededServerContents(t, dir, files, nil))

	e := engineFor(t, dir, Options{Verify: true}, fake, catalogID, currentVer)

	_, err := e.Plan()
	require.NoError(t, err)

	assert.Equal(t, 1, fake.AllFilesCalls(), "the drift fetch already happens; --verify must not add a second")
	assert.Empty(t, e.Divergences(), "a remote that moved on is drift, not a BASE-vs-REMOTE finding")
}

// A healthy project must stay silent under --verify.
func TestPhase2_Verify_HealthyProject_NoDivergence(t *testing.T) {
	const (
		catalogID = "cid-1"
		versionID = "ver-1"
	)

	files := map[string]string{"app.py": "print('A')\n", "util.py": "def u(): pass\n"}

	dir := syncedProject(t, files, catalogID, versionID)

	fake := (&fakeFilesClient{}).withVersionContent(catalogID, versionID, seededServerContents(t, dir, files, nil))

	e := engineFor(t, dir, Options{Verify: true}, fake, catalogID, versionID)

	plan, err := e.Plan()
	require.NoError(t, err)

	assert.True(t, plan.IsEmpty(), "a healthy project must stay Up to date under --verify")
	assert.Empty(t, e.Divergences(), "no divergence when BASE describes the server")
}

// VAL-VERIFY-006: a plain local modification is not a divergence, and the
// upload plan must be identical with and without --verify.
func TestPhase2_Verify_LocalOnlyModification_NoDivergence_PlanUnchanged(t *testing.T) {
	const (
		catalogID = "cid-1"
		versionID = "ver-1"
	)

	files := map[string]string{"app.py": "print('A')\n", "util.py": "def u(): pass\n"}

	dir := syncedProject(t, files, catalogID, versionID)

	// Seed the server with the ORIGINAL bytes (== BASE) before touching disk,
	// so remote == base and only the disk moves.
	seed := seededServerContents(t, dir, files, nil)

	modifyFile(t, dir, "app.py", "print('A2')\n")

	// Each engine gets its own fake over the same server state, so neither
	// Plan() call can influence the other's call counts. The project lock is
	// held until Close, so the first engine must be closed before the second
	// one plans.
	newFake := func() *fakeFilesClient {
		return (&fakeFilesClient{}).withVersionContent(catalogID, versionID, seed)
	}

	ePlain := engineFor(t, dir, Options{}, newFake(), catalogID, versionID)

	planPlain, err := ePlain.Plan()
	require.NoError(t, err)
	require.NoError(t, ePlain.Close(), "release the project lock before the second engine plans")

	eVerify := engineFor(t, dir, Options{Verify: true}, newFake(), catalogID, versionID)

	planVerify, err := eVerify.Plan()
	require.NoError(t, err)

	assert.Empty(t, eVerify.Divergences(), "a local modification is not a BASE-vs-REMOTE divergence")
	assert.Equal(t, planPlain, planVerify, "the upload plan must be identical with and without --verify")
	require.Len(t, planVerify.Uploads, 1)
	assert.Equal(t, "app.py", planVerify.Uploads[0].Path)
}

// First sync against an empty artifact: there is no BASE and no REMOTE to
// diverge, so --verify must stay silent.
func TestPhase2_Verify_FirstSyncEmptyArtifact_NoDivergence(t *testing.T) {
	dir := initProject(t, map[string]string{"agent.py": "print('hi')\n"})

	fake := &fakeFilesClient{catalogID: "cid-new", stageID: "stage-1", versionID: "ver-1"}

	e := engineFor(t, dir, Options{Verify: true}, fake, "", "")

	plan, err := e.Plan()
	require.NoError(t, err)

	assert.Empty(t, e.Divergences(), "a first sync has nothing to diverge")
	assert.NotEmpty(t, plan.Uploads, "the first sync still plans the uploads")
}

// VAL-VERIFY-007 at the engine level: both one-sided directions are detected
// with distinct kinds, and the reconciling plan repairs the manifest to match
// the server exactly.
func TestPhase2_Verify_OneSidedDivergence_RepairMatchesServer(t *testing.T) {
	const (
		catalogID = "cid-1"
		versionID = "ver-1"
	)

	files := map[string]string{"app.py": "print('A')\n", "util.py": "def u(): pass\n"}

	stray := []byte("only on the server\n")
	remoteAppPy := []byte("print('B')\n")

	dir := syncedProject(t, files, catalogID, versionID)

	// The server version drops util.py (BASE-only), rewrites app.py (hash
	// mismatch) and carries a path BASE never recorded (REMOTE-only).
	seed := seededServerContents(t, dir, files, map[string][]byte{
		"app.py":   remoteAppPy,
		"stray.py": stray,
	})
	delete(seed, "util.py")

	fake := (&fakeFilesClient{}).withVersionContent(catalogID, versionID, seed)

	e := engineFor(t, dir, Options{Verify: true}, fake, catalogID, versionID)

	plan, err := e.Plan()
	require.NoError(t, err)

	// Both one-sided directions, each under its own kind.
	kinds := make(map[string]DivergenceKind, len(e.Divergences()))
	for _, d := range e.Divergences() {
		kinds[d.Path] = d.Kind
	}

	assert.Equal(t, DivergenceBaseOnly, kinds["util.py"], "a path in BASE but not on the server is base-only")
	assert.Equal(t, DivergenceRemoteOnly, kinds["stray.py"], "a path on the server but not in BASE is remote-only")

	// The reconciling plan: download the REMOTE-only and modified paths,
	// remove the BASE-only path locally (remote wins).
	downloaded := make([]string, 0, len(plan.Downloads))

	for _, fa := range plan.Downloads {
		downloaded = append(downloaded, fa.Path)
	}

	deleted := make([]string, 0, len(plan.Deletes))

	for _, fa := range plan.Deletes {
		deleted = append(deleted, fa.Path)
	}

	assert.ElementsMatch(t, []string{"app.py", "stray.py"}, downloaded)
	assert.ElementsMatch(t, []string{"util.py"}, deleted)

	_, err = e.Execute(plan)
	require.NoError(t, err)

	// After apply the manifest must describe the true server state.
	manifest, err := wapi.LoadManifest(dir)
	require.NoError(t, err)

	want := map[string]string{
		"app.py":        sha256Hex(remoteAppPy),
		ignore.FileName: sha256Hex(diskBytes(t, dir, ignore.FileName)),
		"stray.py":      sha256Hex(stray),
	}

	got := make(map[string]string, len(manifest.Files))
	for p, meta := range manifest.Files {
		got[p] = meta.Hash
	}

	assert.Equal(t, want, got, "the repaired manifest must match the server (util.py gone, stray.py present)")

	server, err := fake.AllFiles(catalogID, versionID)
	require.NoError(t, err)

	assert.Len(t, server, len(manifest.Files), "manifest and server must hold the same path set")
}

// VAL-VERIFY-017: the divergence warning is emitted from Phase 2 through the
// warn logger, so it must already be on stderr when a later phase fails —
// not rendered only after a successful plan render.
func TestPhase2_Verify_DivergenceWarningSurvivesPhase5Failure(t *testing.T) {
	const (
		catalogID = "cid-1"
		versionID = "ver-1"
	)

	files := map[string]string{"app.py": "print('A')\n"}

	dir := syncedProject(t, files, catalogID, versionID)

	// Poisoned app.py (BASE==local, server differs) plus a brand-new local
	// file, so the plan both downloads and uploads — and the upload fails.
	fake := (&fakeFilesClient{}).withVersionContent(catalogID, versionID, seededServerContents(t, dir, files, map[string][]byte{
		"app.py": []byte("server holds different bytes\n"),
	})).withFailNthUpload(1)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "new.py"), []byte("brand new\n"), 0o644))

	var execErr error

	out := captureWarnLog(t, func() {
		e, err := newWithDeps(dir, Options{Verify: true}, Deps{
			Files: fake,
			Artifacts: &fakeArtifactStore{
				GetFn: func(id string) (*workload.Artifact, error) {
					return draftArtifact(id, catalogID, versionID), nil
				},
			},
			Now: time.Now,
		})
		require.NoError(t, err)

		t.Cleanup(func() { _ = e.Close() })

		plan, perr := e.Plan()
		require.NoError(t, perr)
		require.False(t, plan.IsEmpty(), "the scenario must produce a plan with rows")

		_, execErr = e.Execute(plan)
	})

	require.Error(t, execErr, "Phase 5 must fail for this test to mean anything")
	assert.Contains(t, out, "app.py", "the divergence warning must name the affected path")
	assert.Contains(t, out, "divergence", "the divergence warning must survive the Phase 5 failure")
}
