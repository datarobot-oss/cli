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

	// Configurable AllFiles page size. 0 = return all at once (no pagination
	// simulation). When > 0, AllFiles internally splits files into pages and
	// follows next links, mirroring the real client's pagination behavior.
	allFilesPageSize int

	// Fault-injection hooks (all opt-in; zero values are no-ops).
	dropPathFromApply  string // omit this path from the resulting version after apply
	wrongChecksumPath  string // return wrong checksum for this path in AllFiles
	wrongChecksumValue string // the wrong checksum to substitute
	failNthUpload      int    // fail the Nth UploadToStage call (1-indexed; 0 = disabled)
	failApplyStage     bool   // ApplyStage returns error after staging succeeded
	numFilesOverride   *int   // if non-nil, ApplyStage returns this numFiles; otherwise real count
}

// --- Builder methods for test setup (chainable, mutex-safe) ---

// withVersion pre-populates a version in the fake's server state. Useful for
// testing REPLACE-merge semantics without going through the upload flow.
func (f *fakeFilesClient) withVersion(catalogID, versionID string, files map[string]filesapi.FileMeta) *fakeFilesClient {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.versions == nil {
		f.versions = make(map[string]map[string]filesapi.FileMeta)
	}

	if f.latestVersion == nil {
		f.latestVersion = make(map[string]string)
	}

	f.versions[versionID] = files
	f.latestVersion[catalogID] = versionID

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

	// Initialize a fresh staging area for this stage.
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

	// Fault injection: drop a path from the resulting version.
	if f.dropPathFromApply != "" {
		delete(newVersion, f.dropPathFromApply)
	}

	if f.versions == nil {
		f.versions = make(map[string]map[string]filesapi.FileMeta)
	}

	if f.latestVersion == nil {
		f.latestVersion = make(map[string]string)
	}

	f.versions[f.versionID] = newVersion
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

	zipFiles, err := extractZipFiles(data)
	if err != nil {
		return nil, err
	}

	if f.versions == nil {
		f.versions = make(map[string]map[string]filesapi.FileMeta)
	}

	if f.latestVersion == nil {
		f.latestVersion = make(map[string]string)
	}

	f.versions[f.versionID] = zipFiles
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

	zipFiles, err := extractZipFiles(data)
	if err != nil {
		return nil, err
	}

	// REPLACE merge: start with the latest version's files, then merge.
	newVersion := make(map[string]filesapi.FileMeta)

	if latest, ok := f.latestVersion[catalogID]; ok {
		if files, ok := f.versions[latest]; ok {
			for k, v := range files {
				newVersion[k] = v
			}
		}
	}

	for k, v := range zipFiles {
		newVersion[k] = v
	}

	if f.versions == nil {
		f.versions = make(map[string]map[string]filesapi.FileMeta)
	}

	if f.latestVersion == nil {
		f.latestVersion = make(map[string]string)
	}

	f.versions[f.versionID] = newVersion
	f.latestVersion[catalogID] = f.versionID

	return &filesapi.FromFileResp{
		CatalogID:        catalogID,
		CatalogVersionID: f.versionID,
	}, nil
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

	// Simulate pagination: split into pages and follow next links, mirroring
	// the real client. A bug here (not following next links) would cause
	// AllFiles to return only the first page, which the pagination test catches.
	result = paginateFiles(result, f.allFilesPageSize)

	return result, nil
}

func (f *fakeFilesClient) DownloadFile(_, _, _ string, _ io.Writer) (string, int64, error) {
	return "", 0, errors.New("fakeFilesClient: DownloadFile not expected")
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

	currentFiles, ok := f.versions[latest]
	if !ok {
		return &filesapi.DeleteFilesResp{}, nil
	}

	newVersion := make(map[string]filesapi.FileMeta, len(currentFiles))
	for k, v := range currentFiles {
		newVersion[k] = v
	}

	for _, p := range paths {
		delete(newVersion, p)
	}

	newVerID := fmt.Sprintf("%s-del-%d", f.versionID, len(f.versions))
	if f.versions == nil {
		f.versions = make(map[string]map[string]filesapi.FileMeta)
	}

	if f.latestVersion == nil {
		f.latestVersion = make(map[string]string)
	}

	f.versions[newVerID] = newVersion
	f.latestVersion[catalogID] = newVerID

	return &filesapi.DeleteFilesResp{
		CatalogID:        catalogID,
		CatalogVersionID: newVerID,
		NumFiles:         len(newVersion),
	}, nil
}

func (f *fakeFilesClient) ListVersions(_ string, _ int) ([]filesapi.CatalogVersion, error) {
	return nil, errors.New("fakeFilesClient: ListVersions not expected")
}

// extractZipFiles reads a zip archive from raw bytes and returns a map of
// path → FileMeta with the SHA-256 hash and size of each entry's content.
// This models what the server does when it extracts an uploaded archive.
func extractZipFiles(data []byte) (map[string]filesapi.FileMeta, error) {
	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}

	files := make(map[string]filesapi.FileMeta)

	for _, zf := range zipReader.File {
		content, err := readZipEntry(zf)
		if err != nil {
			return nil, err
		}

		h := sha256.Sum256(content)
		path := fileops.NormalizePath(zf.Name)
		files[path] = filesapi.FileMeta{
			Hash: hex.EncodeToString(h[:]),
			Size: int64(len(content)),
		}
	}

	return files, nil
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
