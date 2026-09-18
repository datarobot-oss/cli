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
	"strings"
	"testing"

	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeArtifactStore is the doctor's in-memory ArtifactGetter. It counts Get
// calls (the single-fetch contract) and can be rigged to return an artifact,
// an error, or both.
type fakeArtifactStore struct {
	getCalls int

	artifact *workload.Artifact

	err error
}

func (f *fakeArtifactStore) Get(_ string) (*workload.Artifact, error) {
	f.getCalls++

	return f.artifact, f.err
}

// testArtifact builds an artifact fixture with the given status and an
// optional codeRef planted on the primary container.
func testArtifact(status string, codeRef *workload.DatarobotCodeRef) *workload.Artifact {
	art := &workload.Artifact{
		ID:     testArtifactID,
		Name:   "doctor-test",
		Status: status,
	}

	if codeRef == nil {
		return art
	}

	primary := true

	art.Spec.ContainerGroups = []workload.ContainerGroup{
		{
			Containers: []workload.Container{
				{
					Primary: &primary,

					ImageBuildConfig: &workload.ImageBuildConfig{
						CodeRef: &workload.CodeRef{Datarobot: codeRef},
					},
				},
			},
		},
	}

	return art
}

// errNotFound is the 404 the doctor must map to FAIL-as-deleted.
var errNotFound = &drapi.HTTPError{StatusCode: http.StatusNotFound, URL: "https://test/artifacts/x/"}

// errServer is a representative non-404 failure (any non-404 must SKIP).
var errServer = &drapi.HTTPError{StatusCode: http.StatusInternalServerError, URL: "https://test/artifacts/x/"}

// errUnreachable is a connection-level failure with no HTTP status at all.
var errUnreachable = errors.New("get artifact: dial tcp: connection refused")

// runRemoteChecks runs the four remote checks against the fixture store and
// returns the results in the fixed remote-check order.
func runRemoteChecks(t *testing.T, projectDir string, store *fakeArtifactStore) []core.Result {
	t.Helper()

	return core.NewRunner(RemoteChecks(projectDir, store)...).Run(context.Background())
}

// byID indexes results by check id for lookups by id.
func byID(results []core.Result) map[string]core.Result {
	m := make(map[string]core.Result, len(results))

	for _, res := range results {
		m[res.CheckID] = res
	}

	return m
}

func TestRemoteChecks_FixedOrder(t *testing.T) {
	ids := make([]string, 0, 4)

	for _, c := range RemoteChecks(t.TempDir(), &fakeArtifactStore{}) {
		ids = append(ids, c.ID())
	}

	assert.Equal(t, []string{
		CheckIDArtifactExists,
		CheckIDArtifactLocked,
		CheckIDCatalogMismatch,
		CheckIDDrift,
	}, ids)
}

func TestRemoteChecks_HealthyDraft_AllOK(t *testing.T) {
	// Never-synced draft: no codeRef on the artifact, no pointers in config.
	// Everything must be OK — a fresh link is healthy, not drifted.
	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig("", "")))

	store := &fakeArtifactStore{artifact: testArtifact("draft", nil)}

	results := runRemoteChecks(t, dir, store)

	require.Len(t, results, 4)

	for _, res := range results {
		assert.Equal(t, core.StatusOK, res.Status, "check %s: summary %q", res.CheckID, res.Summary)
	}
}

func TestRemoteChecks_SingleFetch_CallCount(t *testing.T) {
	// All four checks share ONE GetArtifact snapshot per run: the fake store
	// must observe exactly one Get call no matter how many checks run.
	dir := healthyProject(t)

	store := &fakeArtifactStore{artifact: testArtifact("draft", &workload.DatarobotCodeRef{
		CatalogID:        testCatalogID,
		CatalogVersionID: testVersionID,
	})}

	results := runRemoteChecks(t, dir, store)

	for _, res := range results {
		assert.Equal(t, core.StatusOK, res.Status, "check %s: summary %q", res.CheckID, res.Summary)
	}

	assert.Equal(t, 1, store.getCalls, "exactly one artifact fetch per doctor run")
}

func TestRemoteChecks_404_ArtifactExistsFAIL_OthersSKIP(t *testing.T) {
	dir := healthyProject(t)

	store := &fakeArtifactStore{err: errNotFound}

	results := runRemoteChecks(t, dir, store)

	res := byID(results)

	exists := res[CheckIDArtifactExists]

	assert.Equal(t, core.StatusFAIL, exists.Status)
	assert.Contains(t, exists.Summary, "deleted")
	assert.Contains(t, exists.Remedy, "--relink")

	// The dependent checks must SKIP (ordering contract): a vanished artifact
	// is artifact-exists' finding, not theirs.
	for _, id := range []string{CheckIDArtifactLocked, CheckIDCatalogMismatch, CheckIDDrift} {
		assert.Equal(t, core.StatusSKIP, res[id].Status, "check %s should SKIP on 404", id)
	}
}

func TestRemoteChecks_Wrapped404_StillFAIL(t *testing.T) {
	// 404 detection goes through errors.As, so a wrapped HTTPError still maps
	// to FAIL-as-deleted.
	dir := healthyProject(t)

	store := &fakeArtifactStore{err: fmt.Errorf("fetch artifact: %w", errNotFound)}

	results := runRemoteChecks(t, dir, store)

	assert.Equal(t, core.StatusFAIL, byID(results)[CheckIDArtifactExists].Status)
}

func TestRemoteChecks_Non404_AllSKIP_NeverDeleted(t *testing.T) {
	for name, err := range map[string]error{
		"500":          errServer,
		"unauthorized": &drapi.HTTPError{StatusCode: http.StatusUnauthorized, URL: "https://test/"},
		"forbidden":    &drapi.HTTPError{StatusCode: http.StatusForbidden, URL: "https://test/"},
		"unreachable":  errUnreachable,
	} {
		t.Run(name, func(t *testing.T) {
			dir := healthyProject(t)

			store := &fakeArtifactStore{err: err}

			results := runRemoteChecks(t, dir, store)

			require.Len(t, results, 4)

			for _, res := range results {
				assert.Equal(t, core.StatusSKIP, res.Status,
					"check %s: any non-404 failure must SKIP, never FAIL-as-deleted", res.CheckID)

				assert.NotContains(t, res.Summary, "deleted")

				assert.NotContains(t, res.Remedy, "--relink",
					"the connectivity remedy must never mention relink")

				assert.NotEmpty(t, res.Remedy, "SKIP still carries a remedy")
			}
		})
	}
}

func TestRemoteChecks_Unlinked_AllSKIP(t *testing.T) {
	store := &fakeArtifactStore{}

	results := runRemoteChecks(t, t.TempDir(), store)

	require.Len(t, results, 4)

	for _, res := range results {
		assert.Equal(t, core.StatusSKIP, res.Status, "check %s", res.CheckID)

		assert.Contains(t, res.Summary, "no linked state")
	}

	assert.Zero(t, store.getCalls, "remote checks never fetch without linked state")
}

func TestRemoteChecks_ConfigMissing_AllSKIP(t *testing.T) {
	// State dir present (presence OK) but config.json gone: the artifact id
	// is unknowable, so every remote check SKIPs.
	dir := t.TempDir()

	initStateDir(t, dir)

	store := &fakeArtifactStore{}

	results := runRemoteChecks(t, dir, store)

	for _, res := range results {
		assert.Equal(t, core.StatusSKIP, res.Status, "check %s", res.CheckID)
	}

	assert.Zero(t, store.getCalls)
}

func TestRemoteChecks_LockedArtifact_WARN_NeverFAIL(t *testing.T) {
	dir := healthyProject(t) // config pins testCatalogID/testVersionID

	// The artifact carries a matching codeRef so only the lock state varies.
	store := &fakeArtifactStore{artifact: testArtifact("locked", &workload.DatarobotCodeRef{
		CatalogID:        testCatalogID,
		CatalogVersionID: testVersionID,
	})}

	results := runRemoteChecks(t, dir, store)

	res := byID(results)

	locked := res[CheckIDArtifactLocked]

	assert.Equal(t, core.StatusWARN, locked.Status, "locked must WARN, never FAIL")
	assert.False(t, locked.Fixable, "nothing --fix can do about a remote lock")
	assert.Contains(t, locked.Summary, "preview")
	assert.Contains(t, locked.Summary, "execute")

	// The other three checks still judge the artifact itself.
	assert.Equal(t, core.StatusOK, res[CheckIDArtifactExists].Status)
	assert.Equal(t, core.StatusOK, res[CheckIDCatalogMismatch].Status)
	assert.Equal(t, core.StatusOK, res[CheckIDDrift].Status)
}

func TestRemoteChecks_CatalogMismatch_FAIL_OnlyOwnCheck(t *testing.T) {
	dir := healthyProject(t) // config pins testCatalogID/testVersionID

	// The artifact's codeRef points at a different catalog but the SAME
	// version, so drift must stay OK — proving the mismatch is scoped to the
	// catalog comparison only.
	store := &fakeArtifactStore{artifact: testArtifact("draft", &workload.DatarobotCodeRef{
		CatalogID:        "65ffffffffffffffffffffff",
		CatalogVersionID: testVersionID,
	})}

	results := runRemoteChecks(t, dir, store)

	res := byID(results)

	mismatch := res[CheckIDCatalogMismatch]

	assert.Equal(t, core.StatusFAIL, mismatch.Status)
	assert.Contains(t, mismatch.Summary, testCatalogID)
	assert.Contains(t, mismatch.Remedy, "--relink")

	assert.Equal(t, core.StatusOK, res[CheckIDArtifactExists].Status)
	assert.Equal(t, core.StatusOK, res[CheckIDArtifactLocked].Status)
	assert.Equal(t, core.StatusOK, res[CheckIDDrift].Status)
}

func TestRemoteChecks_Drift_WARN_OnlyOwnCheck(t *testing.T) {
	dir := healthyProject(t) // config pins testVersionID

	// The catalog still matches; only the version moved, so catalog-mismatch
	// must stay OK — proving drift is scoped to the version comparison only.
	store := &fakeArtifactStore{artifact: testArtifact("draft", &workload.DatarobotCodeRef{
		CatalogID:        testCatalogID,
		CatalogVersionID: "65fffffffffffffffffffffe",
	})}

	results := runRemoteChecks(t, dir, store)

	res := byID(results)

	drift := res[CheckIDDrift]

	assert.Equal(t, core.StatusWARN, drift.Status, "drift must WARN, never FAIL")
	assert.Contains(t, drift.Remedy, "sync --dry-run")
	assert.False(t, drift.Fixable)

	assert.Equal(t, core.StatusOK, res[CheckIDArtifactExists].Status)
	assert.Equal(t, core.StatusOK, res[CheckIDArtifactLocked].Status)
	assert.Equal(t, core.StatusOK, res[CheckIDCatalogMismatch].Status)
}

func TestRemoteChecks_EmptyCodeRefFields_NormalizedToNil(t *testing.T) {
	// ExtractCodeRef may return a non-nil pointer with empty fields; empty
	// must be treated as absent so a never-synced artifact with an empty
	// codeRef reports OK, not a spurious mismatch/drift.
	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig("", "")))

	store := &fakeArtifactStore{artifact: testArtifact("draft", &workload.DatarobotCodeRef{
		CatalogID:        "",
		CatalogVersionID: "",
	})}

	results := runRemoteChecks(t, dir, store)

	res := byID(results)

	assert.Equal(t, core.StatusOK, res[CheckIDCatalogMismatch].Status)
	assert.Equal(t, core.StatusOK, res[CheckIDDrift].Status)
}

func TestRemoteChecks_EmptyConfigPointers_NormalizedToNil(t *testing.T) {
	// Config pointers to "" behave as nil (empty ≈ nil) even when the artifact
	// has a real codeRef: an empty pinned pointer pins nothing.
	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig("", "")))

	store := &fakeArtifactStore{artifact: testArtifact("draft", &workload.DatarobotCodeRef{
		CatalogID:        testCatalogID,
		CatalogVersionID: testVersionID,
	})}

	results := runRemoteChecks(t, dir, store)

	res := byID(results)

	assert.Equal(t, core.StatusOK, res[CheckIDCatalogMismatch].Status)
	assert.Equal(t, core.StatusOK, res[CheckIDDrift].Status)
}

func TestRemoteChecks_ReadOnly_StateUntouched(t *testing.T) {
	dir := healthyProject(t)

	store := &fakeArtifactStore{artifact: testArtifact("draft", nil)}

	before := stateFileHashes(t, dir)

	runRemoteChecks(t, dir, store)

	assert.Equal(t, before, stateFileHashes(t, dir), "remote checks are read-only diagnostics")
}

func TestRemoteChecks_ErrorSummaryIsInformative(t *testing.T) {
	// A SKIP must say why it skipped: the underlying error text appears in
	// the summary so 401 vs 5xx vs unreachable are distinguishable.
	dir := healthyProject(t)

	store := &fakeArtifactStore{err: errServer}

	results := runRemoteChecks(t, dir, store)

	for _, res := range results {
		assert.True(t,
			strings.Contains(res.Summary, errServer.Error()) || strings.Contains(res.Summary, "500"),
			"check %s summary %q should carry the underlying error", res.CheckID, res.Summary)
	}
}

// Compile-time proof that the production store satisfies the seam.
var _ ArtifactGetter = ProductionArtifactGetter()

// TestIsNotFound verifies the shared 404-detection predicate used by both the
// doctor remote checks and the init already-linked branch.
func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "404 HTTPError",
			err:  &drapi.HTTPError{StatusCode: 404, URL: "test"},
			want: true,
		},
		{
			name: "500 HTTPError",
			err:  &drapi.HTTPError{StatusCode: 500, URL: "test"},
			want: false,
		},
		{
			name: "wrapped 404",
			err:  fmt.Errorf("fetch failed: %w", &drapi.HTTPError{StatusCode: 404, URL: "test"}),
			want: true,
		},
		{
			name: "plain error",
			err:  errors.New("connection refused"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsNotFound(tt.err))
		})
	}
}

// TestIsCatalogMismatch verifies the shared catalog-mismatch predicate used by
// both the doctor catalog-mismatch check and the init already-linked branch.
func TestIsCatalogMismatch(t *testing.T) {
	matchingCodeRef := &workload.DatarobotCodeRef{CatalogID: "cat-123"}
	mismatchedCodeRef := &workload.DatarobotCodeRef{CatalogID: "cat-456"}
	emptyCodeRef := &workload.DatarobotCodeRef{CatalogID: ""}

	tests := []struct {
		name  string
		local *string
		art   *workload.Artifact
		want  bool
	}{
		{
			name:  "nil local pin always OK (anchor-on-local)",
			local: nil,
			art:   makeArtifact("id", "DRAFT", mismatchedCodeRef),
			want:  false,
		},
		{
			name:  "empty local pin always OK",
			local: strPtr(""),
			art:   makeArtifact("id", "DRAFT", mismatchedCodeRef),
			want:  false,
		},
		{
			name:  "matching catalogs OK",
			local: strPtr("cat-123"),
			art:   makeArtifact("id", "DRAFT", matchingCodeRef),
			want:  false,
		},
		{
			name:  "mismatched catalogs FAIL",
			local: strPtr("cat-123"),
			art:   makeArtifact("id", "DRAFT", mismatchedCodeRef),
			want:  true,
		},
		{
			name:  "local pin set, remote codeRef absent FAIL",
			local: strPtr("cat-123"),
			art:   makeArtifact("id", "DRAFT", nil),
			want:  true,
		},
		{
			name:  "local pin set, remote codeRef empty FAIL",
			local: strPtr("cat-123"),
			art:   makeArtifact("id", "DRAFT", emptyCodeRef),
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsCatalogMismatch(tt.local, tt.art))
		})
	}
}
