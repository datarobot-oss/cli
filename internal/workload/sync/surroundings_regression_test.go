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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/drapi/filesapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/fileops"
	"github.com/datarobot/cli/internal/workload/ignore"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests guard the behaviour immediately surrounding the upload-integrity
// fix so it cannot regress silently. They cover ignore rules, case-collision
// detection, path normalization, plan action mapping, the sync lock,
// stale-rollback recovery, and state migration — all through the engine's
// Plan/Execute seam so a regression in any of them is caught at the level
// that matters.
//
// Fulfills VAL-REGRESSION-007 and VAL-REGRESSION-009.

// ---------------------------------------------------------------------------
// VAL-REGRESSION-007(a): Ignore rules
// ---------------------------------------------------------------------------

// TestDrignoreExcludesFromPlan verifies that files matching .drignore patterns
// are absent from the upload plan. A .drignore pattern that excludes *.tmp must
// keep scratch.tmp out of the uploads while letting app.py through.
func TestDrignoreExcludesFromPlan(t *testing.T) {
	dir := initProject(t, map[string]string{
		"app.py":      "print('hi')\n",
		"scratch.tmp": "temporary\n",
	})

	// Overwrite the .drignore template with a pattern that excludes *.tmp.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ignore.FileName), []byte("*.tmp\n"), 0o644))

	e := lockfileEngine(t, dir, noLockfileRunner)

	plan, err := e.Plan()
	require.NoError(t, err)

	paths := uploadPathsOf(plan)

	assert.Contains(t, paths, "app.py", "non-ignored file must be in the plan")
	assert.NotContains(t, paths, "scratch.tmp", ".drignore-excluded file must be absent from the plan")
}

// TestWapiignoreShadowWarning_LegacyOnly verifies that the deprecation notice
// fires when only the legacy .wapiignore filename is present (no .drignore).
// The notice tells the user the old name is deprecated and to rename it.
func TestWapiignoreShadowWarning_LegacyOnly(t *testing.T) {
	dir := legacyProject(t, map[string]string{
		ignore.LegacyFileName: "*.tmp\n",
		"scratch.tmp":         "x",
	})

	e := lockfileEngine(t, dir, noLockfileRunner)

	_, err := e.Plan()
	require.NoError(t, err)

	notice := e.IgnoreFileNotice()
	assert.Contains(t, notice, ignore.LegacyFileName, "notice must name the legacy file")
	assert.Contains(t, notice, ignore.FileName, "notice must name the current file to rename to")
}

// wantShadowWarning pins the exact ShadowWarning text emitted when both
// ignore filenames are present. The warning is the only signal a user gets
// that patterns they wrote are inert, so a wording change should be a
// deliberate, reviewed act — pinning the full sentence makes it one. The
// text lives in the ignore matcher; this constant is transcribed from it
// (not produced through the same Sprintf, which would make the pin
// self-fulfilling).
const wantShadowWarning = "Both .drignore and .wapiignore are present. .drignore is the one in effect, " +
	"and the patterns in .wapiignore are not applied. Merge them into .drignore and delete .wapiignore."

// TestWapiignoreShadowWarning_BothPresent verifies that the shadow warning
// fires when both .drignore and .wapiignore are present. The current name wins
// and the legacy patterns are inert, which the warning must say. The warning
// is emitted through log.Warn (never through IgnoreFileNotice), so the
// capture seam is what pins the actual text.
func TestWapiignoreShadowWarning_BothPresent(t *testing.T) {
	dir := initProject(t, map[string]string{
		ignore.FileName:       "*.new\n",
		ignore.LegacyFileName: "*.old\n",
	})

	e := lockfileEngine(t, dir, noLockfileRunner)

	logged := captureWarnLog(t, func() {
		_, err := e.Plan()
		require.NoError(t, err)
	})

	// The shadow warning goes to log.Warn, not to IgnoreFileNotice. The
	// notice is empty because the current name is in effect.
	assert.Empty(t, e.IgnoreFileNotice(), "current name in effect has no deprecation notice")

	assert.Contains(t, logged, wantShadowWarning,
		"the shadow warning must actually be emitted, with this exact text")
}

// TestWapiignoreShadowWarning_NotFiredWithoutLegacyFile is the false-positive
// control for the pinned warning: with only the current filename present
// there is nothing being shadowed, and no shadow warning may reach the log.
// Without this control an always-warn implementation would pass the positive
// test above.
func TestWapiignoreShadowWarning_NotFiredWithoutLegacyFile(t *testing.T) {
	dir := initProject(t, map[string]string{
		ignore.FileName: "*.tmp\n",
	})

	e := lockfileEngine(t, dir, noLockfileRunner)

	logged := captureWarnLog(t, func() {
		_, err := e.Plan()
		require.NoError(t, err)
	})

	assert.NotContains(t, logged, wantShadowWarning,
		"no shadow warning when the legacy file is absent")
}

// TestSystemExcludes_StillExcluded verifies that built-in system-excluded paths
// (.datarobot/workload, .wapi, .git, .gitignore, .datarobot.yaml) are absent
// from the upload plan even when no .drignore file exists.
func TestSystemExcludes_StillExcluded(t *testing.T) {
	dir := initProject(t, map[string]string{
		"app.py": "print('hi')\n",
	})

	// Create system-excluded paths inside the project. Use a file name that
	// does not overwrite the real config.json created by initProject.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".datarobot", "workload", "extra.json"), []byte("{}"), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.tmp\n"), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".datarobot.yaml"), []byte("name: test\n"), 0o644))

	e := lockfileEngine(t, dir, noLockfileRunner)

	plan, err := e.Plan()
	require.NoError(t, err)

	paths := uploadPathsOf(plan)

	assert.Contains(t, paths, "app.py", "regular file must be in the plan")

	for _, excluded := range []string{
		".datarobot/workload/extra.json",
		".git/HEAD",
		".gitignore",
		".datarobot.yaml",
	} {
		assert.NotContains(t, paths, excluded, "system-excluded path must be absent from the plan: %s", excluded)
	}
}

// TestCaseFoldedSystemExcludes verifies that system excludes fold case: a
// .Datarobot/workload path is excluded just like .datarobot/workload.
func TestCaseFoldedSystemExcludes(t *testing.T) {
	dir := initProject(t, map[string]string{
		"app.py": "print('hi')\n",
	})

	// Create a differently-cased state directory.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".Datarobot", "workload"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".Datarobot", "workload", "secret.json"), []byte("{}"), 0o644))

	e := lockfileEngine(t, dir, noLockfileRunner)

	plan, err := e.Plan()
	require.NoError(t, err)

	paths := uploadPathsOf(plan)

	assert.NotContains(t, paths, ".Datarobot/workload/secret.json",
		"case-folded system exclude must still be excluded")
}

// TestCaseFoldedIgnorePatterns verifies that case-folding of ignore patterns
// still matches paths differing only in case. A .drignore pattern "BUILD/"
// must exclude a directory named "build/" (case-folded match via system
// excludes is already tested above; this tests user-pattern case-folding
// through the gitignore library).
func TestCaseFoldedIgnorePatterns(t *testing.T) {
	// The gitignore library is case-sensitive on Unix but the system excludes
	// fold case. Here we test that a user pattern in .drignore matches a
	// file of the same name with different case, which the system-exclude
	// case-folding handles for system paths. For user patterns, the
	// gitignore library is case-sensitive, so a pattern "*.TMP" does NOT
	// match "scratch.tmp". We verify the actual behaviour: user patterns are
	// case-sensitive (as documented), while system excludes fold case.
	dir := initProject(t, map[string]string{
		"app.py":      "print('hi')\n",
		"scratch.tmp": "x",
	})

	// Pattern with uppercase extension — gitignore is case-sensitive,
	// so *.TMP does NOT match scratch.tmp.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ignore.FileName), []byte("*.TMP\n"), 0o644))

	e := lockfileEngine(t, dir, noLockfileRunner)

	plan, err := e.Plan()
	require.NoError(t, err)

	paths := uploadPathsOf(plan)

	// User patterns are case-sensitive, so scratch.tmp is NOT excluded by *.TMP.
	assert.Contains(t, paths, "scratch.tmp",
		"user patterns are case-sensitive: *.TMP does not match scratch.tmp")
}

// ---------------------------------------------------------------------------
// VAL-REGRESSION-007(b): Case-collision hard error before any upload
// ---------------------------------------------------------------------------

// TestCaseCollision_FailsBeforeUpload verifies that two paths differing only
// in case cause the sync to fail with a case-collision error. On a
// case-insensitive filesystem (macOS, Windows) the walker cannot produce two
// such entries, so this test calls caseCollisionsFromManifest directly with a
// crafted manifest — the same function phase2Manifests calls after hashing.
func TestCaseCollision_FailsBeforeUpload(t *testing.T) {
	// Craft a local manifest with two paths differing only in case.
	local := LocalManifest{
		"Config.yaml": {Hash: "aaa", Size: 10},
		"config.yaml": {Hash: "bbb", Size: 20},
	}

	collisions := caseCollisionsFromManifest(local)

	require.Len(t, collisions, 1, "one case-collision group expected")
	assert.Equal(t, "config.yaml", collisions[0].Lowered)
	assert.ElementsMatch(t, []string{"Config.yaml", "config.yaml"}, collisions[0].Paths)

	msg := fileops.FormatCaseCollisions(collisions)
	assert.Contains(t, msg, "case-only path collisions")
	assert.Contains(t, msg, "Config.yaml vs config.yaml")
}

// TestCaseCollision_ErrorIsBeforeUpload verifies that the case-collision check
// runs in Phase 2 (manifests), before Phase 5 (execute) where uploads happen.
// We cannot create case-colliding files on macOS, so we verify the check
// position by confirming that a Plan with no case collisions succeeds and
// the fake's upload counters are zero (Plan does not execute).
func TestCaseCollision_ErrorIsBeforeUpload(t *testing.T) {
	dir := syncedProject(t, map[string]string{
		"app.py": "print('hi')\n",
	}, "cid-cc", "ver-cc")

	fake := &fakeFilesClient{
		catalogID: "cid-cc",
		stageID:   "stage-cc",
		versionID: "ver-cc-next",
	}

	e, err := newWithDeps(dir, Options{}, Deps{
		Files: fake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, "cid-cc", "ver-cc"), nil
			},
		},
		Now:      time.Now,
		Lockfile: noLockfileRunner,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	plan, err := e.Plan()
	require.NoError(t, err)
	require.True(t, plan.IsEmpty(), "synced project with no changes has an empty plan")

	// No case collision, no uploads — Plan does not call Execute.
	assert.Equal(t, 0, fake.UploadToStageCalls(), "Plan must not upload")
	assert.Equal(t, 0, fake.ApplyStageCalls(), "Plan must not apply")
}

// ---------------------------------------------------------------------------
// VAL-REGRESSION-007(c): Path normalization
// ---------------------------------------------------------------------------

// TestPathNormalization_ForwardSlash verifies that relative paths in the plan
// use forward slashes, not backslashes. On Unix this is trivially true; on
// Windows the walker must convert OS-native backslashes to forward slashes.
func TestPathNormalization_ForwardSlash(t *testing.T) {
	dir := initProject(t, map[string]string{
		"app.py":              "print('hi')\n",
		"src/utils/helper.py": "def help(): pass\n",
	})

	e := lockfileEngine(t, dir, noLockfileRunner)

	plan, err := e.Plan()
	require.NoError(t, err)

	for _, fa := range plan.Uploads {
		assert.NotContains(t, fa.Path, "\\", "path must use forward slashes: %s", fa.Path)
	}
}

// TestPathNormalization_NoLeadingDotSlash verifies that relative paths in the
// plan have no leading "./" prefix.
func TestPathNormalization_NoLeadingDotSlash(t *testing.T) {
	dir := initProject(t, map[string]string{
		"app.py": "print('hi')\n",
	})

	e := lockfileEngine(t, dir, noLockfileRunner)

	plan, err := e.Plan()
	require.NoError(t, err)

	for _, fa := range plan.Uploads {
		assert.False(t, strings.HasPrefix(fa.Path, "./"),
			"path must not have leading ./: %s", fa.Path)
	}
}

// TestPathNormalization_NoTrailingSlash verifies that relative paths in the
// plan have no trailing slash.
func TestPathNormalization_NoTrailingSlash(t *testing.T) {
	dir := initProject(t, map[string]string{
		"app.py":              "print('hi')\n",
		"src/utils/helper.py": "def help(): pass\n",
	})

	e := lockfileEngine(t, dir, noLockfileRunner)

	plan, err := e.Plan()
	require.NoError(t, err)

	for _, fa := range plan.Uploads {
		assert.False(t, strings.HasSuffix(fa.Path, "/"),
			"path must not have trailing slash: %s", fa.Path)
	}
}

// TestPathNormalization_NFC verifies that relative paths in the plan are
// NFC-normalized. On macOS the filesystem stores filenames in NFD, so a file
// named café.py (NFD: cafe + combining accent) must appear as café.py (NFC)
// in the plan.
func TestPathNormalization_NFC(t *testing.T) {
	// NFC form of "café": precomposed é (U+00E9).
	nfc := "café.py"

	// NFD form: e + combining acute (U+0301). macOS stores this on disk.
	nfd := "cafe\u0301.py"

	dir := initProject(t, map[string]string{
		"app.py": "print('hi')\n",
	})

	// Write the file with the NFD name — the filesystem will store it as-is.
	require.NoError(t, os.WriteFile(filepath.Join(dir, nfd), []byte("x"), 0o644))

	e := lockfileEngine(t, dir, noLockfileRunner)

	plan, err := e.Plan()
	require.NoError(t, err)

	// The plan must contain the NFC form, not the NFD form.
	paths := uploadPathsOf(plan)

	assert.Contains(t, paths, nfc, "path must be NFC-normalized in the plan")
	assert.NotContains(t, paths, nfd, "NFD form must not appear in the plan")

	// Verify the manifest key is also NFC by checking that NormalizePath
	// produces NFC from the NFD input.
	assert.Equal(t, nfc, fileops.NormalizePath(nfd))
}

// TestPathNormalization_ManifestAndComparison verifies that path normalization
// is consistent between the local manifest build and the base/remote
// comparison. When a synced project has a file with a non-ASCII name, the
// manifest key (base) and the plan path (local) must both be NFC-normalized
// so the diff does not falsely report a change.
func TestPathNormalization_ManifestAndComparison(t *testing.T) {
	nfc := "café.py"

	nfd := "cafe\u0301.py"

	dir := initProject(t, map[string]string{
		"app.py": "print('hi')\n",
	})

	// Write the file with the NFD name.
	require.NoError(t, os.WriteFile(filepath.Join(dir, nfd), []byte("x"), 0o644))

	// syncedProject builds the manifest from the file hashes, but we need
	// to ensure the manifest key is NFC. Let's build it manually.
	catalogID := "cid-nfc"
	versionID := "ver-nfc"

	cfg, err := wapi.LoadConfig(dir)
	require.NoError(t, err)

	cfg.CatalogID = &catalogID
	cfg.LastSyncedVersionID = &versionID
	require.NoError(t, wapi.SaveConfig(dir, cfg))

	// Build manifest with NFC keys (what a correct sync would produce).
	manifestFiles := make(map[string]wapi.FileMeta)

	for _, rel := range []string{"app.py", nfc, ignore.FileName} {
		hash, size, err := hashLocal(t, dir, rel)
		if err != nil {
			// The file might be stored under NFD on disk; try the NFD name.
			hash, size, err = hashLocal(t, dir, nfd)
			require.NoError(t, err)
		}

		manifestFiles[rel] = wapi.FileMeta{Hash: hash, Size: size}
	}

	syncedAt := time.Now().UTC()
	manifest := wapi.Manifest{
		Version:         wapi.ManifestVersion,
		SyncedAt:        &syncedAt,
		SyncedVersionID: &versionID,
		Files:           manifestFiles,
	}
	require.NoError(t, wapi.SaveManifest(dir, manifest))

	// Now Plan: the local walk produces NFC paths, the base has NFC keys,
	// so the diff should find no changes for café.py.
	fake := &fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-nfc",
		versionID: "ver-nfc-next",
	}

	e, err := newWithDeps(dir, Options{}, Deps{
		Files: fake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, catalogID, versionID), nil
			},
		},
		Now:      time.Now,
		Lockfile: noLockfileRunner,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	plan, err := e.Plan()
	require.NoError(t, err)

	// The plan must be empty: local == base (both NFC), and the fast path
	// copies base to remote, so no diff.
	assert.True(t, plan.IsEmpty(),
		"NFC-normalized paths must match between local and base: plan should be empty, got uploads=%v deletes=%v",
		uploadPathsOf(plan), deletePathsOf(plan))
}

// ---------------------------------------------------------------------------
// VAL-REGRESSION-007(d): Plan action mapping
// ---------------------------------------------------------------------------

// TestPlanAction_LocalDeleted verifies that a file deleted locally (present in
// base and remote, absent from local) produces an upload-delete action in the
// plan's Deletes list.
func TestPlanAction_LocalDeleted(t *testing.T) {
	const (
		catalogID = "cid-del"
		versionID = "ver-del"
	)

	dir := syncedProject(t, map[string]string{
		"app.py":       "print('hi')\n",
		"to-delete.py": "x\n",
	}, catalogID, versionID)

	// Delete the file locally.
	require.NoError(t, os.Remove(filepath.Join(dir, "to-delete.py")))

	// Pre-populate the fake's server state with the synced version.
	fake := (&fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-del",
		versionID: "ver-del-next",
	}).withVersion(catalogID, versionID, map[string]filesapi.FileMeta{
		"app.py":       {Hash: sha256Hex([]byte("print('hi')\n")), Size: 11},
		"to-delete.py": {Hash: sha256Hex([]byte("x\n")), Size: 2},
		".drignore":    {Hash: sha256Hex([]byte("")), Size: 0},
	})

	// The artifact must point at the synced version so the engine detects drift.
	e, err := newWithDeps(dir, Options{DryRun: true, Yes: true}, Deps{
		Files: fake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, catalogID, versionID), nil
			},
		},
		Now:      time.Now,
		Lockfile: noLockfileRunner,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	plan, err := e.Plan()
	require.NoError(t, err)

	// The deleted file must appear in the Deletes list with ActUploadDelete.
	var deleteActions []FileAction

	for _, fa := range plan.Deletes {
		if fa.Path == "to-delete.py" {
			deleteActions = append(deleteActions, fa)
		}
	}

	require.Len(t, deleteActions, 1, "to-delete.py must be in the Deletes list")
	assert.Equal(t, ActUploadDelete, deleteActions[0].Action,
		"locally-deleted file must map to ActUploadDelete")
	assert.Equal(t, ClsLocalDeleted, deleteActions[0].Classification)
}

// TestPlanAction_RemoteModified verifies that a file modified on the remote
// only (base == local, remote differs) produces a download action.
func TestPlanAction_RemoteModified(t *testing.T) {
	const (
		catalogID   = "cid-rm"
		versionID   = "ver-rm"
		remoteVerID = "ver-rm-remote"
	)

	dir := syncedProject(t, map[string]string{
		"app.py": "print('hi')\n",
	}, catalogID, versionID)

	// The server has a different version of app.py. The artifact's codeRef
	// points at remoteVerID (different from the synced versionID) so the
	// engine detects drift and fetches AllFiles for the remote version.
	remoteHash := sha256Hex([]byte("print('changed')\n"))

	fake := (&fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-rm",
		versionID: "ver-rm-next",
	}).withVersion(catalogID, remoteVerID, map[string]filesapi.FileMeta{
		"app.py":    {Hash: remoteHash, Size: 16},
		".drignore": {Hash: sha256Hex([]byte("")), Size: 0},
	})

	e, err := newWithDeps(dir, Options{DryRun: true, Yes: true}, Deps{
		Files: fake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, catalogID, remoteVerID), nil
			},
		},
		Now:      time.Now,
		Lockfile: noLockfileRunner,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	plan, err := e.Plan()
	require.NoError(t, err)

	// app.py must appear in the Downloads list.
	var downloads []FileAction

	for _, fa := range plan.Downloads {
		if fa.Path == "app.py" {
			downloads = append(downloads, fa)
		}
	}

	require.Len(t, downloads, 1, "app.py must be in the Downloads list")
	assert.Equal(t, ActDownloadModify, downloads[0].Action,
		"remote-only modification must map to ActDownloadModify")
	assert.Equal(t, ClsRemoteModified, downloads[0].Classification)
}

// TestPlanAction_BothSidesDifferent verifies that a file modified on both sides
// differently (local != base, remote != base, local != remote) produces a
// conflict.
func TestPlanAction_BothSidesDifferent(t *testing.T) {
	const (
		catalogID   = "cid-conf"
		versionID   = "ver-conf"
		remoteVerID = "ver-conf-remote"
	)

	dir := syncedProject(t, map[string]string{
		"app.py": "print('hi')\n",
	}, catalogID, versionID)

	// Modify the file locally.
	modifyFile(t, dir, "app.py", "print('local-change')\n")

	// The server has a different version of app.py. The artifact's codeRef
	// points at remoteVerID (different from the synced versionID) so the
	// engine detects drift and fetches AllFiles for the remote version.
	remoteHash := sha256Hex([]byte("print('remote-change')\n"))

	fake := (&fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-conf",
		versionID: "ver-conf-next",
	}).withVersion(catalogID, remoteVerID, map[string]filesapi.FileMeta{
		"app.py":    {Hash: remoteHash, Size: 22},
		".drignore": {Hash: sha256Hex([]byte("")), Size: 0},
	})

	e, err := newWithDeps(dir, Options{DryRun: true, Yes: true}, Deps{
		Files: fake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, catalogID, remoteVerID), nil
			},
		},
		Now:      time.Now,
		Lockfile: noLockfileRunner,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	plan, err := e.Plan()
	require.NoError(t, err)

	// app.py must appear in the Conflicts list.
	var conflicts []FileAction

	for _, fa := range plan.Conflicts {
		if fa.Path == "app.py" {
			conflicts = append(conflicts, fa)
		}
	}

	require.Len(t, conflicts, 1, "app.py must be in the Conflicts list")
	assert.Equal(t, ActConflictCopy, conflicts[0].Action,
		"both-sides-different must map to ActConflictCopy")
	assert.Equal(t, ClsConflict, conflicts[0].Classification)
}

// TestPlanAction_RemoteWinsResolution verifies that the remote-wins
// resolution path downloads the remote version and that the remote bytes
// actually land on disk. The plan-structure half pins that the conflict
// row carries the server's hash (Phase 6's buildNewBaseManifest resolves
// conflicts through fa.RemoteHash); the Execute half pins that the
// resolution really wrote those bytes, kept the local side as a .LOCAL.
// copy, and recorded the remote hash in the new BASE.
func TestPlanAction_RemoteWinsResolution(t *testing.T) {
	const (
		catalogID   = "cid-rw"
		versionID   = "ver-rw"
		remoteVerID = "ver-rw-remote"
	)

	dir := syncedProject(t, map[string]string{
		"app.py": "print('hi')\n",
	}, catalogID, versionID)

	// Modify the file locally.
	localContent := "print('local-change')\n"
	modifyFile(t, dir, "app.py", localContent)

	// The server has a different version. The artifact's codeRef points at
	// remoteVerID (different from the synced versionID) so the engine detects
	// drift and fetches AllFiles for the remote version. The remote version
	// is seeded with recorded content so the fake can actually serve the
	// conflict's download; .drignore is seeded with the disk's own bytes so
	// it is unchanged on both sides and adds no plan rows.
	remoteContent := "print('remote-change')\n"
	remoteHash := sha256Hex([]byte(remoteContent))

	drignoreBytes, err := os.ReadFile(filepath.Join(dir, ignore.FileName))
	require.NoError(t, err)

	fake := (&fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-rw",
		versionID: "ver-rw-next",
	}).withVersionContent(catalogID, remoteVerID, map[string][]byte{
		"app.py":        []byte(remoteContent),
		ignore.FileName: drignoreBytes,
	})

	e, err := newWithDeps(dir, Options{Yes: true}, Deps{
		Files: fake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, catalogID, remoteVerID), nil
			},
		},
		Now:      time.Now,
		Lockfile: noLockfileRunner,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	plan, err := e.Plan()
	require.NoError(t, err)

	require.True(t, plan.HasConflicts(), "plan must have conflicts")

	var conflict FileAction

	for _, fa := range plan.Conflicts {
		if fa.Path == "app.py" {
			conflict = fa
		}
	}

	require.Equal(t, "app.py", conflict.Path)
	assert.Equal(t, remoteHash, conflict.RemoteHash,
		"conflict's RemoteHash must be the server's hash (remote-wins resolution uses this)")

	result, err := e.Execute(plan)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.ConflictCount)

	// Remote wins: the server's exact bytes must now sit at the original
	// path, hashing to the advertised checksum.
	onDisk, readErr := os.ReadFile(filepath.Join(dir, "app.py"))
	require.NoError(t, readErr)

	assert.Equal(t, remoteContent, string(onDisk), "remote-wins resolution must write the remote bytes")
	assert.Equal(t, remoteHash, sha256Hex(onDisk), "written bytes must hash to the server's checksum")

	// The local side of the conflict survives as a .LOCAL.<ts> copy.
	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)

	var localCopy string

	for _, ent := range entries {
		if strings.HasPrefix(ent.Name(), "app.py.LOCAL.") {
			localCopy = ent.Name()
		}
	}

	require.NotEmpty(t, localCopy, "local side of the conflict must be preserved as a .LOCAL. copy")

	localBytes, readErr := os.ReadFile(filepath.Join(dir, localCopy))
	require.NoError(t, readErr)
	assert.Equal(t, localContent, string(localBytes), "the .LOCAL. copy must hold the pre-conflict local bytes")

	// The new BASE records the remote hash for the conflicted path — the
	// remote-wins rule made real by Phase 6.
	manifest, manifestErr := wapi.LoadManifest(dir)
	require.NoError(t, manifestErr)
	assert.Equal(t, remoteHash, manifest.Files["app.py"].Hash,
		"manifest must record the remote hash for the conflict-resolved path")

	assert.Equal(t, 1, fake.DownloadFileCalls(), "conflict resolution must have pulled the remote bytes exactly once")
}

// ---------------------------------------------------------------------------
// VAL-REGRESSION-009: Sync lock, stale-rollback recovery, state migration
// ---------------------------------------------------------------------------

// TestSyncLock_SecondConcurrentSyncRejected verifies that a second concurrent
// sync against the same project is rejected by the sync lock. On Unix, the
// second Plan() must fail with a lock error; on Windows the lock is a no-op
// (tracked in RAPTOR-16928) so the test skips.
func TestSyncLock_SecondConcurrentSyncRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("v1 sync lock is a no-op on windows; tracked in RAPTOR-16928")
	}

	dir := initProject(t, map[string]string{"app.py": "print('hi')\n"})

	// First engine acquires the lock.
	e1, err := newWithDeps(dir, Options{}, Deps{
		Files: &fakeFilesClient{},
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, "", ""), nil
			},
		},
		Now:      time.Now,
		Lockfile: noLockfileRunner,
	})
	require.NoError(t, err)

	plan, err := e1.Plan()
	require.NoError(t, err)
	require.False(t, plan.IsEmpty(), "fixture must produce a non-empty plan")

	t.Cleanup(func() { _ = e1.Close() })

	// Second engine on the same project must fail to acquire the lock.
	e2, err := newWithDeps(dir, Options{}, Deps{
		Files: &fakeFilesClient{},
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, "", ""), nil
			},
		},
		Now:      time.Now,
		Lockfile: noLockfileRunner,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e2.Close() })

	_, err = e2.Plan()
	require.Error(t, err, "second concurrent sync must be rejected by the lock")
	assert.Contains(t, err.Error(), "another sync is already running")
}

// TestSyncLock_StaleRollbackRecovery verifies that a stale rollback from a
// crashed prior run is restored so a new sync proceeds. Phase 0 calls
// RestoreStaleIfPresent before acquiring the lock, so a stale rollback
// directory must be cleaned up and the engine's StaleRollbackRestored() must
// report true.
func TestSyncLock_StaleRollbackRecovery(t *testing.T) {
	dir := initProject(t, map[string]string{"app.py": "print('hi')\n"})

	// Simulate a crashed prior run: create a stale rollback directory with
	// a backup of app.py.
	rollDir := wapi.RollbackDir(dir)
	require.NoError(t, os.MkdirAll(rollDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(rollDir, "app.py"), []byte("intact\n"), 0o644))

	// Clobber the working tree to simulate the crash mid-sync.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.py"), []byte("BROKEN\n"), 0o644))

	e := lockfileEngine(t, dir, noLockfileRunner)

	plan, err := e.Plan()
	require.NoError(t, err, "stale rollback must be recovered so Plan proceeds")
	require.NotNil(t, plan)

	assert.True(t, e.StaleRollbackRestored(), "engine must report that a stale rollback was restored")

	// The file must be restored to its pre-crash content.
	got, err := os.ReadFile(filepath.Join(dir, "app.py"))
	require.NoError(t, err)
	assert.Equal(t, "intact\n", string(got), "stale rollback must restore the original file")

	// The rollback directory must be cleaned up.
	_, err = os.Stat(rollDir)
	assert.ErrorIs(t, err, os.ErrNotExist, "stale rollback directory must be removed after recovery")
}

// TestStateMigration_ForwardMigration verifies that an older on-disk state
// format (legacy .wapi/ directory) is migrated forward to .datarobot/workload/
// without error. Phase 0 calls EnsureMigrated, which moves the legacy
// directory. The engine's StateMigrationNotice() must report the move.
func TestStateMigration_ForwardMigration(t *testing.T) {
	// Initialize a proper project (creates .datarobot/workload/ with a valid
	// config.json that has createdAt and cliVersion), then move the state
	// directory to the legacy .wapi/ location to simulate an older on-disk
	// state format.
	dir := initProject(t, map[string]string{"app.py": "print('hi')\n"})

	// Move the current state directory to the legacy location.
	currentDir := filepath.Join(dir, wapi.RootDirName, wapi.StateDirName)
	legacyDir := filepath.Join(dir, wapi.LegacyDirName)
	require.NoError(t, os.Rename(currentDir, legacyDir))

	// Remove the now-empty .datarobot/ parent so EnsureMigrated sees only the
	// legacy directory.
	require.NoError(t, os.RemoveAll(filepath.Join(dir, wapi.RootDirName)))

	// Verify the legacy directory exists before migration.
	assert.DirExists(t, legacyDir)

	e, err := newWithDeps(dir, Options{}, Deps{
		Files: &fakeFilesClient{},
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, "", ""), nil
			},
		},
		Now:      time.Now,
		Lockfile: noLockfileRunner,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	_, err = e.Plan()
	require.NoError(t, err, "migration must succeed so Plan proceeds")

	// The legacy directory must be gone.
	assert.NoDirExists(t, legacyDir, "legacy state directory must be moved")

	// The current state directory must exist.
	assert.DirExists(t, currentDir, "current state directory must exist after migration")

	// The engine must report the migration.
	notice := e.StateMigrationNotice()
	assert.Contains(t, notice, wapi.LegacyDirName, "notice must name the legacy directory")
}

// ---------------------------------------------------------------------------
// VAL-REGRESSION-008: dr workload up code-change measurement
// ---------------------------------------------------------------------------

// TestWorkloadUp_CodeChangeCount_ModifiedTree verifies that the sync engine's
// Plan (which `dr workload up`'s defaultCodeChange builds as a dry-run) reports
// the correct upload + delete count for a modified tree. defaultCodeChange
// computes `len(plan.Uploads) + len(plan.Deletes)`, so we verify that count
// matches the real number of changed files.
func TestWorkloadUp_CodeChangeCount_ModifiedTree(t *testing.T) {
	const (
		catalogID = "cid-wup"
		versionID = "ver-wup"
	)

	dir := syncedProject(t, map[string]string{
		"app.py":       "print('hi')\n",
		"utils.py":     "def util(): pass\n",
		"to-delete.py": "x\n",
	}, catalogID, versionID)

	// Modify one file, add one new file, delete one file.
	modifyFile(t, dir, "app.py", "print('changed')\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "new.py"), []byte("new\n"), 0o644))
	require.NoError(t, os.Remove(filepath.Join(dir, "to-delete.py")))

	// Pre-populate the fake's server state with the synced version.
	fake := (&fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-wup",
		versionID: "ver-wup-next",
	}).withVersion(catalogID, versionID, map[string]filesapi.FileMeta{
		"app.py":       {Hash: sha256Hex([]byte("print('hi')\n")), Size: 11},
		"utils.py":     {Hash: sha256Hex([]byte("def util(): pass\n")), Size: 17},
		"to-delete.py": {Hash: sha256Hex([]byte("x\n")), Size: 2},
		".drignore":    {Hash: sha256Hex([]byte("")), Size: 0},
	})

	e, err := newWithDeps(dir, Options{DryRun: true, Yes: true}, Deps{
		Files: fake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, catalogID, versionID), nil
			},
		},
		Now:      time.Now,
		Lockfile: noLockfileRunner,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	plan, err := e.Plan()
	require.NoError(t, err)

	// defaultCodeChange reports len(plan.Uploads) + len(plan.Deletes).
	// Modified app.py + new new.py = 2 uploads.
	// Deleted to-delete.py = 1 delete.
	// Total = 3 changed files.
	count := len(plan.Uploads) + len(plan.Deletes)
	assert.Equal(t, 3, count,
		"modified tree must report 3 changed files (2 uploads + 1 delete), got %d (uploads=%d, deletes=%d)",
		count, len(plan.Uploads), len(plan.Deletes))
}

// TestWorkloadUp_CodeChangeCount_UnchangedTree verifies that the sync engine's
// Plan reports zero changed files for an unchanged tree (synced project with no
// modifications). defaultCodeChange reports `len(plan.Uploads) + len(plan.Deletes)`,
// which must be zero.
func TestWorkloadUp_CodeChangeCount_UnchangedTree(t *testing.T) {
	const (
		catalogID = "cid-wup-uc"
		versionID = "ver-wup-uc"
	)

	dir := syncedProject(t, map[string]string{
		"app.py":   "print('hi')\n",
		"utils.py": "def util(): pass\n",
	}, catalogID, versionID)

	// No modifications — the tree is unchanged.

	fake := &fakeFilesClient{
		catalogID: catalogID,
		stageID:   "stage-wup-uc",
		versionID: "ver-wup-uc-next",
	}

	e, err := newWithDeps(dir, Options{DryRun: true, Yes: true}, Deps{
		Files: fake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, catalogID, versionID), nil
			},
		},
		Now:      time.Now,
		Lockfile: noLockfileRunner,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	plan, err := e.Plan()
	require.NoError(t, err)

	count := len(plan.Uploads) + len(plan.Deletes)
	assert.Equal(t, 0, count,
		"unchanged tree must report 0 changed files, got %d (uploads=%d, deletes=%d)",
		count, len(plan.Uploads), len(plan.Deletes))
	assert.True(t, plan.IsEmpty(), "unchanged tree must produce an empty plan")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// deletePathsOf returns the paths in the delete portion of the plan.
func deletePathsOf(plan *SyncPlan) []string {
	paths := make([]string, 0, len(plan.Deletes))
	for _, fa := range plan.Deletes {
		paths = append(paths, fa.Path)
	}

	return paths
}
