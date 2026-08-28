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
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	stdsync "sync"

	"github.com/datarobot/cli/internal/drapi/filesapi"
	"github.com/datarobot/cli/internal/workload/fileops"
)

// fakeFilesClient is a self-consistent in-memory model of the Files API
// server. Unlike a shallow stub, it records per-path content and its SHA-256
// so tests can assert "what the server ended up holding" after an upload.
//
// Downloads are served from the same model: DownloadFile returns exactly the
// bytes recorded for the requested path at the requested version, so a test
// can assert the right bytes were pulled — not merely that no error was
// returned. Versions carry content only when it was actually recorded (via
// uploads or the withVersionContent builder); hash-only seeds honestly
// report nothing to download.
//
// Server state is modeled as versions: each ApplyStage (or zip upload) creates
// a new version whose files are the REPLACE-merge of the staged paths into the
// prior version. AllFiles serves exactly what was recorded, keyed by version,
// with configurable pagination via a next-link simulation. ApplyStage returns
// a numFiles that defaults to the count of ALL files in the resulting version
// (matching the real API semantics), not just the ones uploaded.
//
// Per-method call counters let tests assert that specific network calls were
// or were not issued. Fault-injection hooks are all opt-in: a test that does
// not ask for a fault gets a faithful server.
//
// Every shared map is guarded by a mutex so -race is clean when the uploader
// runs concurrently (UploadConcurrency = 4).
type fakeFilesClient struct {
	mu stdsync.Mutex

	// Configured IDs returned by the fake server. CreateCatalog returns
	// catalogID, CreateStage returns stageID, ApplyStage/zip uploads return
	// versionID.
	catalogID string
	stageID   string
	versionID string

	// Server state: versionID → (path → FileMeta with SHA-256 hash + size).
	versions map[string]map[string]filesapi.FileMeta

	// Per-catalog latest version ID, for REPLACE-merge semantics.
	latestVersion map[string]string

	// Current staging area: path → content bytes (stage path only).
	stagedFiles map[string][]byte

	// Content store: versionID → (path → the exact bytes recorded for that
	// path in that version). DownloadFile serves from this map so a download
	// returns byte-for-byte what an upload (or a withVersionContent seed)
	// recorded, and AllFiles' checksums stay consistent with those bytes.
	// Versions seeded through the hash-only withVersion builder carry no
	// content, and DownloadFile reports them as not found rather than
	// inventing bytes the server never held.
	versionContents map[string]map[string][]byte

	// Backward-compatible fields: recorded uploaded content and deleted
	// paths, kept so existing tests that inspect them still work.
	uploadedFiles map[string][]byte
	deletedPaths  []string

	// Call counters (guarded by mu).
	createCatalogCalls int
	createStageCalls   int
	uploadToStageCalls int
	applyStageCalls    int
	uploadFromZipCalls int
	allFilesCalls      int
	downloadFileCalls  int

	// Configurable AllFiles page size. 0 = return all at once (no pagination
	// simulation). When > 0, AllFiles internally splits files into pages and
	// follows next links, mirroring the real client's pagination behavior.
	allFilesPageSize int

	// Downloadable content: versionID → (path → bytes served by DownloadFile).
	// Opt-in: a test that never registers content gets the loud "not
	// expected" failure, so unexpected download calls stay visible.
	downloadContent map[string]map[string][]byte

	// Fault-injection hooks (all opt-in; zero values are no-ops).
	dropPathFromApply  string // omit this path from the resulting version after apply
	wrongChecksumPath  string // return wrong checksum for this path in AllFiles
	wrongChecksumValue string // the wrong checksum to substitute
	failNthUpload      int    // fail the Nth UploadToStage call (1-indexed; 0 = disabled)
	failApplyStage     bool   // ApplyStage returns error after staging succeeded
	numFilesOverride   *int   // if non-nil, ApplyStage returns this numFiles; otherwise real count

	// Version-scoped checksum fault: served only when AllFiles is called
	// for this version (see withWrongChecksumForVersion).
	wrongChecksumForVersion *wrongChecksumFault

	// Download faults, keyed by path. failDownloadPath makes DownloadFile
	// return an error for that path; corruptDownloadPath makes it serve
	// bytes whose SHA-256 differs from the advertised checksum (same
	// length, so the size check passes and the checksum check is what
	// catches the corruption).
	failDownloadPath    string
	corruptDownloadPath string
}

// --- Builder methods for test setup (chainable, mutex-safe) ---

// withVersion pre-populates a version in the fake's server state. Useful for
// testing REPLACE-merge semantics without going through the upload flow.
//
// The seed map is copied, not aliased: a caller that reuses or mutates its
// map after handing it over must not silently rewrite server state mid-test.
// (The fake used to store the caller's map directly, so a later mutation
// changed an already-seeded version behind the test's back.
// TestWithVersionCopiesTheSeed pins the copy.)
func (f *fakeFilesClient) withVersion(catalogID, versionID string, files map[string]filesapi.FileMeta) *fakeFilesClient {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.versions == nil {
		f.versions = make(map[string]map[string]filesapi.FileMeta)
	}

	if f.latestVersion == nil {
		f.latestVersion = make(map[string]string)
	}

	copied := make(map[string]filesapi.FileMeta, len(files))

	for path, fm := range files {
		copied[path] = fm
	}

	f.versions[versionID] = copied
	f.latestVersion[catalogID] = versionID

	// This builder seeds metadata only; drop any content a previous seed
	// may have left under the same version ID so the store never disagrees
	// with the advertised checksums.
	delete(f.versionContents, versionID)

	return f
}

// withVersionContent pre-populates a version from raw bytes, computing each
// path's SHA-256 hash and size the way the real server does. This is the
// builder for download tests: DownloadFile later serves exactly these bytes
// for this version, and AllFiles advertises the checksums those bytes hash
// to. Use withVersion instead when only metadata is needed.
func (f *fakeFilesClient) withVersionContent(catalogID, versionID string, contents map[string][]byte) *fakeFilesClient {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.versions == nil {
		f.versions = make(map[string]map[string]filesapi.FileMeta)
	}

	if f.versionContents == nil {
		f.versionContents = make(map[string]map[string][]byte)
	}

	if f.latestVersion == nil {
		f.latestVersion = make(map[string]string)
	}

	metas := make(map[string]filesapi.FileMeta, len(contents))

	for path, content := range contents {
		metas[path] = fileMetaOf(content)
	}

	f.versions[versionID] = metas
	f.versionContents[versionID] = contents
	f.latestVersion[catalogID] = versionID

	return f
}

// withDownloadable registers the content bytes DownloadFile serves for a
// version. The registered content must hash to the FileMeta the same test
// put in the version state — the download path verifies the streamed bytes
// against the plan's RemoteHash, exactly like the real server relationship.
func (f *fakeFilesClient) withDownloadable(versionID string, files map[string][]byte) *fakeFilesClient {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.downloadContent == nil {
		f.downloadContent = make(map[string]map[string][]byte)
	}

	f.downloadContent[versionID] = files

	return f
}

// withPageSize sets the AllFiles page size for pagination simulation.
func (f *fakeFilesClient) withPageSize(n int) *fakeFilesClient {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.allFilesPageSize = n

	return f
}

// withDropPath configures the fake to omit the given path from the resulting
// version after ApplyStage, simulating a server-side dropped upload.
func (f *fakeFilesClient) withDropPath(path string) *fakeFilesClient {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.dropPathFromApply = path

	return f
}

// withWrongChecksum configures AllFiles to return the given checksum for the
// given path instead of the real SHA-256, simulating a server-side checksum
// mismatch.
func (f *fakeFilesClient) withWrongChecksum(path, checksum string) *fakeFilesClient {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.wrongChecksumPath = path
	f.wrongChecksumValue = checksum

	return f
}

// withWrongChecksumForVersion is the version-scoped form of withWrongChecksum:
// the substitute checksum is served only when AllFiles is called for the given
// version. The engine's post-apply verification reads the NEW version, which
// Phase 2 never fetches, so a fault scoped to it models "the post-apply
// listing disagrees" without corrupting the Phase-2 fetch or the plan.
func (f *fakeFilesClient) withWrongChecksumForVersion(versionID, path, checksum string) *fakeFilesClient {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.wrongChecksumForVersion = &wrongChecksumFault{
		versionID: versionID,
		path:      path,
		checksum:  checksum,
	}

	return f
}

// withFailDownload configures DownloadFile to return an error for the given
// path, simulating a download that fails server-side or mid-transfer.
func (f *fakeFilesClient) withFailDownload(path string) *fakeFilesClient {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.failDownloadPath = path

	return f
}

// withCorruptDownload configures DownloadFile to serve, for the given path,
// bytes of the same length as the recorded content but with every byte
// flipped. The served bytes therefore hash to something other than the
// checksum AllFiles advertises, which is what lets a test exercise the
// client-side post-download checksum verification rather than the size
// check. The hooked path must have non-empty recorded content: flipping an
// empty byte string yields empty again, and the fake refuses to serve a
// "corruption" indistinguishable from the real bytes.
func (f *fakeFilesClient) withCorruptDownload(path string) *fakeFilesClient {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.corruptDownloadPath = path

	return f
}

// withFailNthUpload configures the fake to fail the Nth UploadToStage call
// (1-indexed). 0 disables the hook.
func (f *fakeFilesClient) withFailNthUpload(n int) *fakeFilesClient {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.failNthUpload = n

	return f
}

// withFailApplyStage configures ApplyStage to return an error after all files
// have been successfully staged, simulating a server-side apply failure.
func (f *fakeFilesClient) withFailApplyStage() *fakeFilesClient {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.failApplyStage = true

	return f
}

// withNumFilesOverride configures ApplyStage to return the given numFiles
// instead of the real count of all files in the resulting version.
func (f *fakeFilesClient) withNumFilesOverride(n int) *fakeFilesClient {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.numFilesOverride = &n

	return f
}

// --- Counter accessors (mutex-safe for reading after concurrent work) ---

func (f *fakeFilesClient) CreateCatalogCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.createCatalogCalls
}

func (f *fakeFilesClient) CreateStageCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.createStageCalls
}

func (f *fakeFilesClient) UploadToStageCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.uploadToStageCalls
}

func (f *fakeFilesClient) ApplyStageCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.applyStageCalls
}

func (f *fakeFilesClient) UploadFromZipCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.uploadFromZipCalls
}

func (f *fakeFilesClient) AllFilesCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.allFilesCalls
}

func (f *fakeFilesClient) DownloadFileCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.downloadFileCalls
}

// --- filesapi.Client implementation ---

func (f *fakeFilesClient) CreateCatalog() (*filesapi.CatalogResp, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.createCatalogCalls++

	if f.catalogID == "" {
		return nil, errors.New("fakeFilesClient.CreateCatalog: no catalogID configured")
	}

	return &filesapi.CatalogResp{CatalogID: f.catalogID}, nil
}

func (f *fakeFilesClient) CreateStage(_ string) (*filesapi.StageResp, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.createStageCalls++

	if f.stageID == "" {
		return nil, errors.New("fakeFilesClient.CreateStage: no stageID configured")
	}

	// Initialize a fresh staging area for this stage. The fake models ONE
	// active stage: the staging area is shared and wiped here, so anything
	// staged but not yet applied is discarded when a test calls CreateStage
	// again mid-flow. This matches production usage — exactly one
	// CreateStage per sync, then the uploads, then ApplyStage — so a test
	// that interleaves two stages exercises a flow the real server never
	// sees.
	f.stagedFiles = make(map[string][]byte)

	return &filesapi.StageResp{CatalogID: f.catalogID, StageID: f.stageID}, nil
}

func (f *fakeFilesClient) UploadToStage(_, _, name string, _ int64, body io.Reader) error {
	// Read the body outside the mutex: each call has its own reader and
	// holding the mutex during I/O would serialize uploads unnecessarily.
	data, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("read upload body: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.uploadToStageCalls++

	// Fault injection: fail the Nth upload.
	if f.failNthUpload > 0 && f.uploadToStageCalls == f.failNthUpload {
		return fmt.Errorf("fakeFilesClient.UploadToStage: injected failure on upload %d", f.uploadToStageCalls)
	}

	if f.stagedFiles == nil {
		f.stagedFiles = make(map[string][]byte)
	}

	f.stagedFiles[name] = data

	// Backward-compatible: record uploaded content for tests that inspect it.
	if f.uploadedFiles == nil {
		f.uploadedFiles = make(map[string][]byte)
	}

	f.uploadedFiles[name] = data

	return nil
}

// mergeStagedFiles merges staged files into a copy of the latest version's
// files, matching the real API's stage REPLACE semantics: staged paths
// replace existing entries, unmentioned paths from the prior version remain.
func (f *fakeFilesClient) mergeStagedFiles(catalogID string) map[string]filesapi.FileMeta {
	newVersion := make(map[string]filesapi.FileMeta)

	if latest, ok := f.latestVersion[catalogID]; ok {
		if files, ok := f.versions[latest]; ok {
			for k, v := range files {
				newVersion[k] = v
			}
		}
	}

	for path, data := range f.stagedFiles {
		h := sha256.Sum256(data)
		newVersion[path] = filesapi.FileMeta{
			Hash: hex.EncodeToString(h[:]),
			Size: int64(len(data)),
		}
	}

	return newVersion
}

// mergeStagedContents is mergeStagedFiles' content-side twin: it starts from
// the latest version's recorded bytes and overlays the staged files, so the
// resulting version's content store matches the metadata merge exactly.
// Prior paths with no recorded content (hash-only withVersion seeds) carry
// no entry, keeping "no content recorded" truthful through the merge.
func (f *fakeFilesClient) mergeStagedContents(catalogID string) map[string][]byte {
	newContents := make(map[string][]byte)

	if latest, ok := f.latestVersion[catalogID]; ok {
		if contents, ok := f.versionContents[latest]; ok {
			for k, v := range contents {
				newContents[k] = v
			}
		}
	}

	for path, data := range f.stagedFiles {
		newContents[path] = data
	}

	return newContents
}

func (f *fakeFilesClient) ApplyStage(catalogID, _, _ string) (*filesapi.ApplyStageResp, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.applyStageCalls++

	// Fault injection: fail ApplyStage after staging succeeded.
	if f.failApplyStage {
		return nil, errors.New("fakeFilesClient.ApplyStage: injected failure")
	}

	if f.versionID == "" {
		return nil, errors.New("fakeFilesClient.ApplyStage: no versionID configured")
	}

	newVersion := f.mergeStagedFiles(catalogID)

	newContents := f.mergeStagedContents(catalogID)

	// Fault injection: drop a path from the resulting version.
	if f.dropPathFromApply != "" {
		delete(newVersion, f.dropPathFromApply)
		delete(newContents, f.dropPathFromApply)
	}

	if f.versions == nil {
		f.versions = make(map[string]map[string]filesapi.FileMeta)
	}

	if f.versionContents == nil {
		f.versionContents = make(map[string]map[string][]byte)
	}

	if f.latestVersion == nil {
		f.latestVersion = make(map[string]string)
	}

	f.versions[f.versionID] = newVersion
	f.versionContents[f.versionID] = newContents
	f.latestVersion[catalogID] = f.versionID

	// Clear the staging area for the next stage.
	f.stagedFiles = make(map[string][]byte)

	// numFiles defaults to the count of ALL files in the resulting version,
	// not just the ones uploaded. This matches the real API semantics.
	numFiles := len(newVersion)
	if f.numFilesOverride != nil {
		numFiles = *f.numFilesOverride
	}

	return &filesapi.ApplyStageResp{
		CatalogID:        f.catalogID,
		CatalogVersionID: f.versionID,
		NumFiles:         numFiles,
	}, nil
}

func (f *fakeFilesClient) UploadFromZipNew(_ string, _ int64, body io.Reader) (*filesapi.FromFileResp, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("read zip body: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.uploadFromZipCalls++

	if f.catalogID == "" {
		return nil, errors.New("fakeFilesClient.UploadFromZipNew: no catalogID configured")
	}

	zipContents, err := extractZipEntries(data)
	if err != nil {
		return nil, err
	}

	if f.versions == nil {
		f.versions = make(map[string]map[string]filesapi.FileMeta)
	}

	if f.versionContents == nil {
		f.versionContents = make(map[string]map[string][]byte)
	}

	if f.latestVersion == nil {
		f.latestVersion = make(map[string]string)
	}

	f.versions[f.versionID] = zipFileMetas(zipContents)
	f.versionContents[f.versionID] = zipContents
	f.latestVersion[f.catalogID] = f.versionID

	// Inline completion (no StatusID), matching the real API for small archives.
	return &filesapi.FromFileResp{
		CatalogID:        f.catalogID,
		CatalogVersionID: f.versionID,
	}, nil
}

func (f *fakeFilesClient) UploadFromZipExisting(catalogID, _, _ string, _ int64, body io.Reader) (*filesapi.FromFileResp, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("read zip body: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.uploadFromZipCalls++

	zipContents, err := extractZipEntries(data)
	if err != nil {
		return nil, err
	}

	newVersion, newContents := f.mergeZipIntoExisting(catalogID, zipContents)

	if f.versions == nil {
		f.versions = make(map[string]map[string]filesapi.FileMeta)
	}

	if f.versionContents == nil {
		f.versionContents = make(map[string]map[string][]byte)
	}

	if f.latestVersion == nil {
		f.latestVersion = make(map[string]string)
	}

	f.versions[f.versionID] = newVersion
	f.versionContents[f.versionID] = newContents
	f.latestVersion[catalogID] = f.versionID

	return &filesapi.FromFileResp{
		CatalogID:        catalogID,
		CatalogVersionID: f.versionID,
	}, nil
}

// zipFileMetas derives the server's metadata map from extracted zip content,
// the only checksum derivation for the zip path.
func zipFileMetas(zipContents map[string][]byte) map[string]filesapi.FileMeta {
	metas := make(map[string]filesapi.FileMeta, len(zipContents))

	for path, content := range zipContents {
		metas[path] = fileMetaOf(content)
	}

	return metas
}

// mergeZipIntoExisting builds the resulting version for an incremental zip
// upload: the latest version's metadata and recorded content with the zip's
// entries overlaid, matching the real API's REPLACE merge. Both maps stay in
// lockstep so AllFiles' checksums always describe the bytes DownloadFile
// serves.
func (f *fakeFilesClient) mergeZipIntoExisting(catalogID string, zipContents map[string][]byte) (map[string]filesapi.FileMeta, map[string][]byte) {
	newVersion := make(map[string]filesapi.FileMeta)

	newContents := make(map[string][]byte)

	if latest, ok := f.latestVersion[catalogID]; ok {
		if files, ok := f.versions[latest]; ok {
			for k, v := range files {
				newVersion[k] = v
			}
		}

		if contents, ok := f.versionContents[latest]; ok {
			for k, v := range contents {
				newContents[k] = v
			}
		}
	}

	for k, v := range zipFileMetas(zipContents) {
		newVersion[k] = v
	}

	for k, v := range zipContents {
		newContents[k] = v
	}

	return newVersion, newContents
}

func (f *fakeFilesClient) PollStatus(_ string) (*filesapi.StatusResp, error) {
	return &filesapi.StatusResp{Status: filesapi.StatusCompleted}, nil
}

// applyWrongChecksum substitutes the configured wrong checksum for the
// chosen path in the result map, simulating a server-side checksum mismatch.
func (f *fakeFilesClient) applyWrongChecksum(result map[string]filesapi.FileMeta) {
	if f.wrongChecksumPath == "" {
		return
	}

	if fm, exists := result[f.wrongChecksumPath]; exists {
		fm.Hash = f.wrongChecksumValue
		result[f.wrongChecksumPath] = fm
	}
}

// wrongChecksumFault is the version-scoped checksum fault record (see
// withWrongChecksumForVersion).
type wrongChecksumFault struct {
	versionID string
	path      string
	checksum  string
}

// applyWrongChecksumForVersion applies the version-scoped checksum fault when
// AllFiles was called for the fault's version, leaving every other version's
// listing truthful.
func (f *fakeFilesClient) applyWrongChecksumForVersion(versionID string, result map[string]filesapi.FileMeta) {
	flt := f.wrongChecksumForVersion
	if flt == nil || flt.versionID != versionID {
		return
	}

	if fm, exists := result[flt.path]; exists {
		fm.Hash = flt.checksum
		result[flt.path] = fm
	}
}

// paginateFiles simulates the server's paginated response and the client's
// next-link following. With a page size of 1 and N files, the fake processes
// N pages; if it failed to follow next links, only the first file would be
// returned. A no-op when pageSize is 0 or when all files fit in one page.
func paginateFiles(files map[string]filesapi.FileMeta, pageSize int) map[string]filesapi.FileMeta {
	if pageSize <= 0 || len(files) <= pageSize {
		return files
	}

	paths := make([]string, 0, len(files))
	for k := range files {
		paths = append(paths, k)
	}

	sort.Strings(paths)

	collected := make(map[string]filesapi.FileMeta, len(paths))

	// Process page by page, following next links.
	idx := 0
	for idx < len(paths) {
		end := idx + pageSize
		if end > len(paths) {
			end = len(paths)
		}

		for _, p := range paths[idx:end] {
			collected[p] = files[p]
		}

		// Advance to the next page (follow the next link).
		idx = end
	}

	return collected
}

func (f *fakeFilesClient) AllFiles(_, versionID string) (map[string]filesapi.FileMeta, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.allFilesCalls++

	// Get the files for the requested version from server state.
	files, ok := f.versions[versionID]
	if !ok {
		return make(map[string]filesapi.FileMeta), nil
	}

	// Copy so the caller cannot mutate our state.
	result := make(map[string]filesapi.FileMeta, len(files))
	for k, v := range files {
		result[k] = v
	}

	// Fault injection: return a wrong checksum for a chosen path.
	f.applyWrongChecksum(result)
	f.applyWrongChecksumForVersion(versionID, result)

	// Simulate pagination: split into pages and follow next links, mirroring
	// the real client. A bug here (not following next links) would cause
	// AllFiles to return only the first page, which the pagination test catches.
	result = paginateFiles(result, f.allFilesPageSize)

	return result, nil
}

// DownloadFile serves the exact bytes recorded for path at versionID,
// streaming them to w the way the real client streams the response body.
// The string return is unused by the sync engine (the production client
// returns "" for inline downloads); n is the number of bytes written.
//
// The fault hooks apply before the content lookup, so a faulted path fails
// whether or not the version has recorded content, and every call is
// counted whether it succeeds or fails.
func (f *fakeFilesClient) DownloadFile(_, versionID, path string, w io.Writer) (string, int64, error) {
	data, err := f.downloadPayload(versionID, path)
	if err != nil {
		return "", 0, err
	}

	n, err := w.Write(data)
	if err != nil {
		return "", int64(n), fmt.Errorf("fakeFilesClient.DownloadFile: write %s: %w", path, err)
	}

	return "", int64(n), nil
}

// downloadPayload gathers the bytes to serve for one download call. It takes
// the mutex so the counter, fault decision, and content snapshot are atomic
// with respect to concurrent uploads creating new versions, and returns a
// copy so the caller's write happens without holding the lock (mirroring the
// read-body-outside-the-mutex convention in UploadToStage).
func (f *fakeFilesClient) downloadPayload(versionID, path string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.downloadFileCalls++

	if path == f.failDownloadPath {
		return nil, fmt.Errorf("fakeFilesClient.DownloadFile: injected failure for %s", path)
	}

	contents, ok := f.versionContents[versionID]
	if !ok {
		// A version can exist with metadata only (hash-only withVersion
		// seed); report that honestly instead of serving empty bytes,
		// which would hash like the empty string and silently pass
		// checksum assertions.
		if _, versionExists := f.versions[versionID]; versionExists {
			return nil, fmt.Errorf(
				"fakeFilesClient.DownloadFile: version %s has no recorded content (seeded via withVersion; use withVersionContent to make it downloadable)", versionID)
		}

		return nil, fmt.Errorf("fakeFilesClient.DownloadFile: version %s not found", versionID)
	}

	data, ok := contents[path]
	if !ok {
		return nil, fmt.Errorf("fakeFilesClient.DownloadFile: path %s not in version %s", path, versionID)
	}

	if path == f.corruptDownloadPath {
		data = corruptBytes(data)
	}

	// Copy under the lock: the recorded content could otherwise be replaced
	// by a concurrent version write between the snapshot and the write.
	snapshot := make([]byte, len(data))
	copy(snapshot, data)

	return snapshot, nil
}

// corruptBytes returns a same-length copy of data with every byte flipped,
// guaranteeing a different SHA-256 while keeping the advertised size intact.
// Empty input is its own corruption (flipping nothing changes nothing), so
// corrupt-download tests must hook a path with non-empty recorded content.
func corruptBytes(data []byte) []byte {
	out := make([]byte, len(data))

	for i, b := range data {
		out[i] = b ^ 0xFF
	}

	return out
}

func (f *fakeFilesClient) DeleteFiles(catalogID string, paths []string) (*filesapi.DeleteFilesResp, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.deletedPaths = append(f.deletedPaths, paths...)

	if len(paths) == 0 {
		return &filesapi.DeleteFilesResp{}, nil
	}

	// Create a new version with the deleted paths removed from the latest version.
	latest, ok := f.latestVersion[catalogID]
	if !ok {
		return &filesapi.DeleteFilesResp{}, nil
	}

	newVersion, newContents := f.versionAfterDeletes(latest, paths)

	newVerID := fmt.Sprintf("%s-del-%d", f.versionID, len(f.versions))

	if f.versions == nil {
		f.versions = make(map[string]map[string]filesapi.FileMeta)
	}

	if f.versionContents == nil {
		f.versionContents = make(map[string]map[string][]byte)
	}

	if f.latestVersion == nil {
		f.latestVersion = make(map[string]string)
	}

	f.versions[newVerID] = newVersion
	f.versionContents[newVerID] = newContents
	f.latestVersion[catalogID] = newVerID

	return &filesapi.DeleteFilesResp{
		CatalogID:        catalogID,
		CatalogVersionID: newVerID,
		NumFiles:         len(newVersion),
	}, nil
}

// versionAfterDeletes builds the post-delete version's metadata and recorded
// content: the latest version's state minus the deleted paths, so surviving
// files keep their checksums and stay downloadable.
func (f *fakeFilesClient) versionAfterDeletes(latest string, paths []string) (map[string]filesapi.FileMeta, map[string][]byte) {
	newVersion := make(map[string]filesapi.FileMeta)

	if files, ok := f.versions[latest]; ok {
		for k, v := range files {
			newVersion[k] = v
		}
	}

	newContents := make(map[string][]byte)

	if contents, ok := f.versionContents[latest]; ok {
		for k, v := range contents {
			newContents[k] = v
		}
	}

	for _, p := range paths {
		delete(newVersion, p)
		delete(newContents, p)
	}

	return newVersion, newContents
}

func (f *fakeFilesClient) ListVersions(_ string, _ int) ([]filesapi.CatalogVersion, error) {
	return nil, errors.New("fakeFilesClient: ListVersions not expected")
}

// extractZipEntries reads a zip archive from raw bytes and returns a map of
// path → the entry's uncompressed content. This models what the server does
// when it extracts an uploaded archive: the extracted bytes are what the
// server holds, so both the recorded checksums and later downloads must
// derive from exactly these bytes.
func extractZipEntries(data []byte) (map[string][]byte, error) {
	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}

	files := make(map[string][]byte)

	for _, zf := range zipReader.File {
		content, err := readZipEntry(zf)
		if err != nil {
			return nil, err
		}

		files[fileops.NormalizePath(zf.Name)] = content
	}

	return files, nil
}

// fileMetaOf derives the server's FileMeta (SHA-256 hex + size) from file
// content, the single derivation every version-recording path goes through.
func fileMetaOf(content []byte) filesapi.FileMeta {
	h := sha256.Sum256(content)

	return filesapi.FileMeta{
		Hash: hex.EncodeToString(h[:]),
		Size: int64(len(content)),
	}
}

// readZipEntry reads and closes a single zip file entry, returning its
// uncompressed content.
func readZipEntry(zf *zip.File) ([]byte, error) {
	rc, err := zf.Open()
	if err != nil {
		return nil, fmt.Errorf("open zip entry %s: %w", zf.Name, err)
	}

	defer func() { _ = rc.Close() }()

	content, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("read zip entry %s: %w", zf.Name, err)
	}

	return content, nil
}
