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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi/filesapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/ignore"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests cover --verify's second effect: after ApplyUploads, Phase 5
// asks the server what the new version actually holds and fails before any
// state is persisted when an uploaded path's server checksum differs from
// the streamed hash. The check lives in Phase 5 so a mismatch trips the
// existing rollback and Phase 6 never runs — asserted here at the state-file
// level: manifest.json and config.json must be byte-identical to their
// pre-sync content after a failed verification.

const (
	postApplyCatalogID = "cid-verify"
	postApplyOldVer    = "ver-old"
	postApplyNewVer    = "ver-new"
)

// postApplyScenario builds a synced project (BASE == LOCAL == the server's
// pre-apply version) with one modified file, so Plan yields exactly one
// upload row for it. Mutate chains fake fault hooks.
type postApplyScenario struct {
	dir     string
	fake    *fakeFilesClient
	newBody string
}

func newPostApplyScenario(t *testing.T, mutate func(*fakeFilesClient) *fakeFilesClient) postApplyScenario {
	t.Helper()

	appPy := "print('A')\n"
	untouched := "print('untouched')\n"

	// The manifest matches disk, and the server's pre-apply version matches
	// both, so Phase 2's verify-forced fetch finds no divergence and the
	// plan is exactly one upload.
	dir := syncedProject(t, map[string]string{
		"app.py":       appPy,
		"untouched.py": untouched,
	}, postApplyCatalogID, postApplyOldVer)

	// The server seed is built from the pre-change bytes explicitly, NOT by
	// reading the tree after the modification below: a seed captured after
	// the edit would make the server agree with disk, classify app.py as
	// CONVERGED, and yield an empty plan — the divergence scenario, not the
	// plain-upload one these tests need.
	drignoreBytes, err := os.ReadFile(filepath.Join(dir, ignore.FileName))
	require.NoError(t, err)

	seed := map[string][]byte{
		ignore.FileName: drignoreBytes,
		"app.py":        []byte(appPy),
		"untouched.py":  []byte(untouched),
	}

	newBody := "print('B')\n"
	modifyFile(t, dir, "app.py", newBody)

	fake := (&fakeFilesClient{
		catalogID: postApplyCatalogID,
		stageID:   "stage-verify",
		versionID: postApplyNewVer,
	}).withVersionContent(postApplyCatalogID, postApplyOldVer, seed)

	if mutate != nil {
		fake = mutate(fake)
	}

	return postApplyScenario{dir: dir, fake: fake, newBody: newBody}
}

func (s postApplyScenario) engine(t *testing.T, opts Options) *Engine {
	t.Helper()

	e, err := newWithDeps(s.dir, opts, Deps{
		Files: s.fake,
		Artifacts: &fakeArtifactStore{
			GetFn: func(id string) (*workload.Artifact, error) {
				return draftArtifact(id, postApplyCatalogID, postApplyOldVer), nil
			},
		},
		Now:      time.Now,
		Lockfile: noLockfileRunner,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	return e
}

// persistedStateSnapshot reads both state files' raw bytes so a test can
// assert Phase 6 never ran: byte-identical after a failed verification means
// neither SaveManifest nor SaveConfig happened.
func persistedStateSnapshot(t *testing.T, dir string) (manifest, config []byte) {
	t.Helper()

	manifestPath, configPath := statePaths(dir)

	manifest, err := os.ReadFile(manifestPath)
	require.NoError(t, err)

	config, err = os.ReadFile(configPath)
	require.NoError(t, err)

	return manifest, config
}

// TestEngine_VerifyPostApplyChecksumMismatchFailsAndPersistsNothing is
// VAL-VERIFY-009: a server checksum that differs from the streamed hash for
// an uploaded path fails the sync with an error naming that path, and both
// persisted state files are byte-identical to their pre-sync content —
// Phase 6 must never run on a failed verification.
func TestEngine_VerifyPostApplyChecksumMismatchFailsAndPersistsNothing(t *testing.T) {
	t.Run("wrong checksum on the uploaded path", func(t *testing.T) {
		sentHash := sha256Hex([]byte("print('B')\n"))
		serverHash := sha256Hex([]byte("the server holds something else\n"))

		s := newPostApplyScenario(t, func(f *fakeFilesClient) *fakeFilesClient {
			// Scoped to the NEW version so Phase 2's fetch of the old
			// version stays truthful: the corruption must be visible only
			// to the post-apply check.
			return f.withWrongChecksumForVersion(postApplyNewVer, "app.py", serverHash)
		})
		e := s.engine(t, Options{Yes: true, Verify: true})

		plan, err := e.Plan()
		require.NoError(t, err)
		require.Len(t, plan.Uploads, 1)

		manifestBefore, configBefore := persistedStateSnapshot(t, s.dir)

		_, err = e.Execute(plan)
		require.Error(t, err, "a post-apply checksum mismatch must fail the sync")
		assert.Contains(t, err.Error(), "app.py", "the error must name the path whose hash differs")

		manifestAfter, configAfter := persistedStateSnapshot(t, s.dir)

		assert.Equal(t, string(manifestBefore), string(manifestAfter),
			"manifest.json must be byte-identical to its pre-sync content when verification fails")
		assert.Equal(t, string(configBefore), string(configAfter),
			"config.json must be byte-identical to its pre-sync content when verification fails")

		// The failure is the point: the streamed hash the manifest WOULD
		// have recorded must be the one the server refused to confirm.
		assert.Contains(t, err.Error(), sentHash, "the error must carry the hash that was sent")
		assert.Contains(t, err.Error(), serverHash, "the error must carry the hash the server holds")
	})

	t.Run("uploaded path dropped from the resulting version", func(t *testing.T) {
		s := newPostApplyScenario(t, func(f *fakeFilesClient) *fakeFilesClient {
			// The server accepted the upload but dropped the path from
			// the resulting version: the post-apply listing must not
			// contain it, and verification must catch the absence before
			// Phase 6 records a hash for bytes the server does not hold.
			return f.withDropPath("app.py")
		})
		e := s.engine(t, Options{Yes: true, Verify: true})

		plan, err := e.Plan()
		require.NoError(t, err)
		require.Len(t, plan.Uploads, 1)

		manifestBefore, configBefore := persistedStateSnapshot(t, s.dir)

		_, err = e.Execute(plan)
		require.Error(t, err, "an uploaded path missing from the server version must fail the sync")
		assert.Contains(t, err.Error(), "app.py", "the error must name the dropped path")

		manifestAfter, configAfter := persistedStateSnapshot(t, s.dir)

		assert.Equal(t, string(manifestBefore), string(manifestAfter),
			"manifest.json must not gain an entry the server does not hold")
		assert.Equal(t, string(configBefore), string(configAfter),
			"config.json must not advance past the manifest")
	})

	t.Run("without verify no post-apply check exists", func(t *testing.T) {
		// The negative control for the whole feature: the same server-side
		// checksum fault, but without --verify, the run must succeed — the
		// default path gains no verification and no extra round-trip.
		s := newPostApplyScenario(t, func(f *fakeFilesClient) *fakeFilesClient {
			return f.withWrongChecksumForVersion(postApplyNewVer, "app.py", sha256Hex([]byte("wrong\n")))
		})
		e := s.engine(t, Options{Yes: true})

		plan, err := e.Plan()
		require.NoError(t, err)
		require.Len(t, plan.Uploads, 1)

		result, err := e.Execute(plan)
		require.NoError(t, err, "without --verify the post-apply check must not exist")
		require.NotNil(t, result)

		assert.Equal(t, 0, s.fake.AllFilesCalls(),
			"a non-drifted run without --verify must make no AllFiles call at all")
	})
}

// TestEngine_VerifyPostApplyOnlyComparesUploadedPaths is VAL-VERIFY-010(a):
// stage REPLACE merges in place, so the post-apply listing legitimately holds
// files this sync never touched. A wrong checksum on such a path must not
// fail the run — only uploaded paths are compared.
func TestEngine_VerifyPostApplyOnlyComparesUploadedPaths(t *testing.T) {
	s := newPostApplyScenario(t, func(f *fakeFilesClient) *fakeFilesClient {
		return f.withWrongChecksumForVersion(postApplyNewVer, "untouched.py", sha256Hex([]byte("stale\n")))
	})
	e := s.engine(t, Options{Yes: true, Verify: true})

	plan, err := e.Plan()
	require.NoError(t, err)
	require.Len(t, plan.Uploads, 1)
	assert.Equal(t, "app.py", plan.Uploads[0].Path, "untouched.py must not be in the upload plan")

	result, err := e.Execute(plan)
	require.NoError(t, err, "a wrong checksum on a path this sync did not upload must not fail the run")
	require.NotNil(t, result)

	// Exactly two AllFiles calls: Phase 2's verify-forced fetch of the old
	// version, then the post-apply check of the new one. This is the
	// network cost --verify adds beyond the single Phase-2 fetch.
	assert.Equal(t, 2, s.fake.AllFilesCalls(),
		"a non-drifted verify run with uploads makes exactly two AllFiles calls")
}

// TestEngine_VerifyPostApplyNumFilesDoesNotGate is VAL-VERIFY-010(c):
// ApplyStage's numFiles counts every file in the resulting version, not the
// ones uploaded, so it cannot relate to the plan's size. A nonsense value
// must neither gate, skip, nor abort verification — the sync completes
// successfully.
func TestEngine_VerifyPostApplyNumFilesDoesNotGate(t *testing.T) {
	s := newPostApplyScenario(t, func(f *fakeFilesClient) *fakeFilesClient {
		return f.withNumFilesOverride(99)
	})
	e := s.engine(t, Options{Yes: true, Verify: true})

	plan, err := e.Plan()
	require.NoError(t, err)
	require.Len(t, plan.Uploads, 1, "the fixture uploads one file; numFiles=99 must not matter")

	result, err := e.Execute(plan)
	require.NoError(t, err, "a numFiles that does not match the upload count must not affect the run")
	require.NotNil(t, result)
	assert.Equal(t, 1, result.UploadedCount)

	assert.Equal(t, 2, s.fake.AllFilesCalls(),
		"verification must still run (Phase-2 fetch + post-apply check) with a nonsense numFiles")

	manifest, err := wapi.LoadManifest(s.dir)
	require.NoError(t, err)
	assert.Equal(t, sha256Hex([]byte(s.newBody)), manifest.Files["app.py"].Hash,
		"the manifest must record the streamed hash")
}

// TestEngine_VerifyPostApplySkippedWhenNoUploads is VAL-VERIFY-023 plus the
// empty-plan corner: with nothing uploaded there is nothing to verify, and
// the post-apply step must make no AllFiles call at all rather than erroring
// on a nil outcome or an empty Sent map.
func TestEngine_VerifyPostApplySkippedWhenNoUploads(t *testing.T) {
	t.Run("downloads-only plan", func(t *testing.T) {
		s := newDownloadScenario(t, nil)

		e, err := newWithDeps(s.dir, Options{Yes: true, Verify: true}, Deps{
			Files: s.fake,
			Artifacts: &fakeArtifactStore{
				GetFn: func(id string) (*workload.Artifact, error) {
					return draftArtifact(id, "cid-dl", "ver-dl-remote"), nil
				},
			},
			Now:      time.Now,
			Lockfile: noLockfileRunner,
		})
		require.NoError(t, err)

		t.Cleanup(func() { _ = e.Close() })

		plan, err := e.Plan()
		require.NoError(t, err)
		require.Len(t, plan.Downloads, 1)
		assert.Empty(t, plan.Uploads)

		// The drifted artifact made Phase 2 fetch the remote; that call is
		// Phase 2's, not verification's.
		callsAfterPlan := s.fake.AllFilesCalls()
		require.Equal(t, 1, callsAfterPlan, "the fixture expects exactly the Phase-2 fetch before Execute")

		result, err := e.Execute(plan)
		require.NoError(t, err, "a downloads-only plan must not error on a nil upload outcome")
		require.NotNil(t, result)
		assert.Equal(t, 1, result.DownloadedCount)

		assert.Equal(t, callsAfterPlan, s.fake.AllFilesCalls(),
			"post-apply verification must make zero AllFiles calls when nothing was uploaded")

		manifest, err := wapi.LoadManifest(s.dir)
		require.NoError(t, err)
		assert.Equal(t, s.remoteHash, manifest.Files["app.py"].Hash,
			"the manifest must record the downloaded file's remote hash")
	})

	t.Run("empty plan that reaches phase 5 through a divergence", func(t *testing.T) {
		// BASE poisoned while disk and server agree: the plan is empty but
		// the divergence drives the run into Phase 5 (a no-op there) and
		// Phase 6's repair. No uploads, so no post-apply call.
		contentA := "print('A')\n"
		contentB := "print('B')\n"

		dir := syncedProject(t, map[string]string{"app.py": contentB}, postApplyCatalogID, postApplyOldVer)

		poisonManifestHash(t, dir, "app.py", sha256Hex([]byte(contentA)))

		fake := (&fakeFilesClient{}).withVersionContent(postApplyCatalogID, postApplyOldVer, seededServerContents(t, dir, map[string]string{
			"app.py": contentB,
		}, nil))

		var runErr error

		out := captureWarnLog(t, func() {
			e, err := newWithDeps(dir, Options{Yes: true, Verify: true}, Deps{
				Files: fake,
				Artifacts: &fakeArtifactStore{
					GetFn: func(id string) (*workload.Artifact, error) {
						return draftArtifact(id, postApplyCatalogID, postApplyOldVer), nil
					},
				},
				Now:      time.Now,
				Lockfile: noLockfileRunner,
			})
			require.NoError(t, err)

			t.Cleanup(func() { _ = e.Close() })

			_, runErr = e.Run()
		})

		require.NoError(t, runErr)
		assert.Contains(t, out, "divergence", "the repair run must still report the stale BASE")

		assert.Equal(t, 1, fake.AllFilesCalls(),
			"the only AllFiles call is Phase 2's fetch; the empty plan must add none")
	})

	t.Run("empty plan short-circuits before phase 5", func(t *testing.T) {
		dir := syncedProject(t, map[string]string{"app.py": "print('hi')\n"}, postApplyCatalogID, postApplyOldVer)

		fake := (&fakeFilesClient{}).withVersionContent(postApplyCatalogID, postApplyOldVer, seededServerContents(t, dir, map[string]string{
			"app.py": "print('hi')\n",
		}, nil))

		e, err := newWithDeps(dir, Options{Yes: true, Verify: true}, Deps{
			Files: fake,
			Artifacts: &fakeArtifactStore{
				GetFn: func(id string) (*workload.Artifact, error) {
					return draftArtifact(id, postApplyCatalogID, postApplyOldVer), nil
				},
			},
			Now:      time.Now,
			Lockfile: noLockfileRunner,
		})
		require.NoError(t, err)

		t.Cleanup(func() { _ = e.Close() })

		result, err := e.Run()
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, 1, fake.AllFilesCalls(),
			"Phase 2's verify-forced fetch is the only AllFiles call on an up-to-date project")
	})
}

// TestVerifyPostApplyUploads_FollowsPaginationViaRealClient drives the
// post-apply check at the httptest level against the REAL httpClient. The
// next-link loop lives below the filesapi.Client interface (inside
// httpClient.AllFiles), so the fake's page-size hook can never force
// production down a multi-page path — an engine test with the fake would
// assert nothing about pagination. Here the uploaded path's checksum exists
// only on page 2, behind a next link, with an unrelated decoy on page 1: a
// page-1-only implementation could neither find the path to verify nor
// mismatch it.
func TestVerifyPostApplyUploads_FollowsPaginationViaRealClient(t *testing.T) {
	const (
		catalogID = "cid-pag"
		versionID = "ver-pag"
	)

	targetBody := []byte("page two content\n")
	decoyBody := []byte("page one decoy\n")

	targetHash := sha256Hex(targetBody)
	decoyHash := sha256Hex(decoyBody)

	var srv *httptest.Server

	mux := http.NewServeMux()

	mux.HandleFunc("/api/v2/files/"+catalogID+"/versions/"+versionID+"/allFiles/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Query().Get("offset") == "" {
			// Page 1: the decoy only, plus a same-host next link. The
			// uploaded path is deliberately absent here.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"fileName": "decoy.txt", "fileSize": len(decoyBody), "fileChecksum": decoyHash},
				},
				"next": srv.URL + "/api/v2/files/" + catalogID + "/versions/" + versionID + "/allFiles/?offset=1",
			})

			return
		}

		// Page 2: only the uploaded path.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"fileName": "app.py", "fileSize": len(targetBody), "fileChecksum": targetHash},
			},
		})
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Point the production client at the test server and skip the token
	// resolution round-trip; previous values are restored afterwards.
	prevURL := viperx.GetString(config.DataRobotURL)
	prevKey := viperx.GetString(config.DataRobotAPIKey)
	prevSkipAuth := viperx.GetBool(config.SkipAuthKey)

	viperx.Set(config.DataRobotURL, srv.URL)
	viperx.Set(config.DataRobotAPIKey, "pagination-test-token")
	viperx.Set(config.SkipAuthKey, true)

	t.Cleanup(func() {
		viperx.Set(config.DataRobotURL, prevURL)
		viperx.Set(config.DataRobotAPIKey, prevKey)
		viperx.Set(config.SkipAuthKey, prevSkipAuth)
	})

	e, err := newWithDeps(t.TempDir(), Options{Verify: true}, Deps{Files: filesapi.New()})
	require.NoError(t, err)

	t.Cleanup(func() { _ = e.Close() })

	outcome := &UploadOutcome{
		CatalogID: catalogID,
		VersionID: versionID,
		Sent: map[string]FileEntry{
			"app.py": {Hash: targetHash, Size: int64(len(targetBody))},
		},
	}

	// Match: the page-2 checksum equals the streamed hash.
	require.NoError(t, verifyPostApplyUploads(e, outcome),
		"a checksum that only appears on page 2 must be found and compared")

	// Mismatch: the same page-2 path, a different streamed hash.
	sentBody := []byte("what we actually sent\n")
	outcome.Sent["app.py"] = FileEntry{Hash: sha256Hex(sentBody), Size: int64(len(sentBody))}

	err = verifyPostApplyUploads(e, outcome)
	require.Error(t, err, "a page-2 checksum mismatch must fail verification")
	assert.Contains(t, err.Error(), "app.py", "the error must name the path seen on page 2")
}
