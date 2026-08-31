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

package up

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/datarobot/cli/internal/drapi/filesapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A bound, source-built workload. workloadId makes it a bind rather than a
// create, and source: generated makes it a build-from-code the pull applies to.
const boundGeneratedManifest = `name: my-app
workloadId: wl-1
importance: low
artifact:
  name: my-app-artifact
  spec:
    type: service
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 8080
            imageBuildConfig:
              dockerfile:
                source: generated
                executionEnvironmentId: ee-1
              entrypoint: ["uvicorn", "app:app"]
runtime:
  containerGroups:
    - name: default
      replicaCount: 1
      containers:
        - name: primary
          resourceAllocation: {cpu: 0.5, memory: 512MB}
`

// fakeFiles serves a fixed file set. Only the two methods seed uses are
// implemented; the embedded interface satisfies the rest and panics if the
// code under test reaches for one, which would be a test worth failing.
type fakeFiles struct {
	filesapi.Client
	files     map[string]filesapi.FileMeta
	content   map[string]string
	failPaths map[string]bool

	// The reused sync downloader pulls in parallel, so pulled is guarded.
	mu     sync.Mutex
	pulled []string
}

func (f *fakeFiles) AllFiles(string, string) (map[string]filesapi.FileMeta, error) {
	return f.files, nil
}

func (f *fakeFiles) DownloadFile(_, _, path string, w io.Writer) (string, int64, error) {
	f.mu.Lock()
	f.pulled = append(f.pulled, path)
	f.mu.Unlock()

	// The bytes are written before the failure so the test exercises the
	// partial-file-on-disk path, not a create that never wrote anything.
	n, _ := io.WriteString(w, f.content[path])

	if f.failPaths[path] {
		return "", int64(n), errors.New("connection reset mid-stream")
	}

	return "", int64(n), nil
}

// loadedFrom builds a Loaded over dir from a manifest string.
func loadedForSeed(t *testing.T, dir, manifestYAML string) Loaded {
	t.Helper()

	m, err := manifest.Parse([]byte(manifestYAML), dir)
	require.NoError(t, err)

	return Loaded{Path: filepath.Join(dir, manifest.FileName), ProjectDir: dir, Manifest: m}
}

// artifactWithCode is an artifact whose primary container is built from a code
// catalog, which is what ExtractCodeRef reads.
func artifactWithCode(catalog, version string) *workload.Artifact {
	primary := true

	return &workload.Artifact{
		ID: "art-1",
		Spec: workload.Spec{
			ContainerGroups: []workload.ContainerGroup{{
				Containers: []workload.Container{{
					Primary: &primary,
					ImageBuildConfig: &workload.ImageBuildConfig{
						CodeRef: &workload.CodeRef{
							Datarobot: &workload.DatarobotCodeRef{
								CatalogID:        catalog,
								CatalogVersionID: version,
							},
						},
					},
				}},
			}},
		},
	}
}

// stubFiles installs a fake files client for the test's lifetime.
func stubFiles(t *testing.T, fake *fakeFiles) {
	t.Helper()

	force(t, &filesClientFn, func() filesapi.Client { return fake })
}

// A bind into an empty directory pulls the workload's current code, so the
// deploy that follows builds from the real thing instead of an empty tree. The
// .drignore already sitting there (an earlier run wrote it) does not count as
// content.
func TestSeed_PullsCodeIntoAnEmptyBoundProject(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".drignore"), []byte("__pycache__\n"), 0o644))

	force(t, &projectLinkedFn, func(string) bool { return false })
	force(t, &getArtifactFn, func(string) (*workload.Artifact, error) {
		return artifactWithCode("cat-1", "ver-1"), nil
	})
	stubFiles(t, &fakeFiles{
		files: map[string]filesapi.FileMeta{
			"app.py":         {},
			"pyproject.toml": {},
			".drignore":      {},
		},
		content: map[string]string{
			"app.py":         "print('hi')\n",
			"pyproject.toml": "[project]\nname = \"x\"\n",
			".drignore":      "__pycache__\n",
		},
	})

	var stderr bytes.Buffer

	err := seedFromLiveArtifact(loadedForSeed(t, dir, boundGeneratedManifest), Live{ArtifactID: "art-1"},
		Options{Stderr: &stderr})
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(dir, "app.py"))
	assert.FileExists(t, filepath.Join(dir, "pyproject.toml"))
	assert.Contains(t, stderr.String(), "Pulled 3 file(s)")

	got, err := os.ReadFile(filepath.Join(dir, "app.py"))
	require.NoError(t, err)
	assert.Equal(t, "print('hi')\n", string(got))
}

// A download that fails mid-stream must not leave the partial file behind: on
// a retry a truncated root marker would make the directory look like a real
// project, seeding would be skipped, and the deploy would build from the
// corrupt tree.
func TestSeed_RemovesAPartialFileWhenADownloadFails(t *testing.T) {
	dir := t.TempDir()

	force(t, &projectLinkedFn, func(string) bool { return false })
	force(t, &getArtifactFn, func(string) (*workload.Artifact, error) {
		return artifactWithCode("cat-1", "ver-1"), nil
	})
	stubFiles(t, &fakeFiles{
		files:     map[string]filesapi.FileMeta{"pyproject.toml": {}},
		content:   map[string]string{"pyproject.toml": "[project]\nname = \"x\"\n"},
		failPaths: map[string]bool{"pyproject.toml": true},
	})

	err := seedFromLiveArtifact(loadedForSeed(t, dir, boundGeneratedManifest), Live{ArtifactID: "art-1"},
		Options{Stderr: io.Discard})
	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(dir, "pyproject.toml"),
		"a failed download must not leave a partial file a retry mistakes for a real project")
}

// A download whose bytes do not match the catalog's recorded size is a silent
// truncation the writer did not report; it fails and leaves nothing behind.
func TestSeed_RejectsASizeMismatch(t *testing.T) {
	dir := t.TempDir()

	force(t, &projectLinkedFn, func(string) bool { return false })
	force(t, &getArtifactFn, func(string) (*workload.Artifact, error) {
		return artifactWithCode("cat-1", "ver-1"), nil
	})
	stubFiles(t, &fakeFiles{
		files:   map[string]filesapi.FileMeta{"app.py": {Size: 999}},
		content: map[string]string{"app.py": "print('hi')\n"},
	})

	err := seedFromLiveArtifact(loadedForSeed(t, dir, boundGeneratedManifest), Live{ArtifactID: "art-1"},
		Options{Stderr: io.Discard})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected 999")
	assert.NoFileExists(t, filepath.Join(dir, "app.py"))
}

// Two catalog keys that differ only by case collapse to one file on a
// case-insensitive filesystem, so the pull is refused before any file is
// written rather than silently dropping one.
func TestSeed_RefusesCaseCollidingCode(t *testing.T) {
	dir := t.TempDir()

	force(t, &projectLinkedFn, func(string) bool { return false })
	force(t, &getArtifactFn, func(string) (*workload.Artifact, error) {
		return artifactWithCode("cat-1", "ver-1"), nil
	})

	fake := &fakeFiles{files: map[string]filesapi.FileMeta{"App.py": {}, "app.py": {}}}
	stubFiles(t, fake)

	err := seedFromLiveArtifact(loadedForSeed(t, dir, boundGeneratedManifest), Live{ArtifactID: "art-1"},
		Options{Stderr: io.Discard})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "case-only path collisions")
	assert.Empty(t, fake.pulled, "nothing is downloaded when the code cannot be laid down safely")
}

// A recognised-but-unlinked project needs no seeding, and must not be blocked
// by a files-API call it never needed: the live artifact is not read at all.
func TestSeed_DoesNotReadTheArtifactForARecognisedProject(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\n"), 0o644))

	force(t, &projectLinkedFn, func(string) bool { return false })
	force(t, &getArtifactFn, func(string) (*workload.Artifact, error) {
		t.Fatal("a recognised project needs no pull, so its code is never read")

		return nil, nil
	})

	fake := &fakeFiles{}
	stubFiles(t, fake)

	err := seedFromLiveArtifact(loadedForSeed(t, dir, boundGeneratedManifest), Live{ArtifactID: "art-1"},
		Options{Stderr: io.Discard})
	require.NoError(t, err)
	assert.Empty(t, fake.pulled)
}

// A directory with files but no recognised project, not linked to the
// workload, is refused rather than rolled: deploying it would replace the
// workload's code with whatever this is.
func TestSeed_RefusesANonEmptyUnlinkedDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("scratch"), 0o644))

	force(t, &projectLinkedFn, func(string) bool { return false })
	force(t, &getArtifactFn, func(string) (*workload.Artifact, error) {
		return artifactWithCode("cat-1", "ver-1"), nil
	})

	fake := &fakeFiles{files: map[string]filesapi.FileMeta{"app.py": {}}}
	stubFiles(t, fake)

	err := seedFromLiveArtifact(loadedForSeed(t, dir, boundGeneratedManifest), Live{ArtifactID: "art-1"},
		Options{Stderr: io.Discard})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to deploy")
	assert.Empty(t, fake.pulled, "nothing is downloaded when the run is refused")
}

// A directory that already looks like a project is the normal case: the user
// has the code, and nothing is pulled over it.
func TestSeed_SkipsWhenTheDirIsAlreadyAProject(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\n"), 0o644))

	force(t, &projectLinkedFn, func(string) bool { return false })
	force(t, &getArtifactFn, func(string) (*workload.Artifact, error) {
		return artifactWithCode("cat-1", "ver-1"), nil
	})

	fake := &fakeFiles{files: map[string]filesapi.FileMeta{"app.py": {}}}
	stubFiles(t, fake)

	err := seedFromLiveArtifact(loadedForSeed(t, dir, boundGeneratedManifest), Live{ArtifactID: "art-1"},
		Options{Stderr: io.Discard})
	require.NoError(t, err)
	assert.Empty(t, fake.pulled, "a real project is deployed as-is, not overwritten")
}

// A published-image workload has no code catalog, so the artifact is never even
// read for one.
func TestSeed_SkipsAPublishedImageWorkload(t *testing.T) {
	dir := t.TempDir()

	force(t, &projectLinkedFn, func(string) bool { return false })
	force(t, &getArtifactFn, func(string) (*workload.Artifact, error) {
		t.Fatal("a published-image workload has no code to read")

		return nil, nil
	})

	err := seedFromLiveArtifact(loadedForSeed(t, dir, unboundImageManifest), Live{ArtifactID: "art-1"},
		Options{Stderr: io.Discard})
	require.NoError(t, err)
}

// An artifact carrying only the ignore file has no code to pull, so an empty
// directory is left empty and the build guard elsewhere speaks to it.
func TestSeed_SkipsWhenTheArtifactHasNoRealCode(t *testing.T) {
	dir := t.TempDir()

	force(t, &projectLinkedFn, func(string) bool { return false })
	force(t, &getArtifactFn, func(string) (*workload.Artifact, error) {
		return artifactWithCode("cat-1", "ver-1"), nil
	})

	fake := &fakeFiles{files: map[string]filesapi.FileMeta{".drignore": {}}}
	stubFiles(t, fake)

	err := seedFromLiveArtifact(loadedForSeed(t, dir, boundGeneratedManifest), Live{ArtifactID: "art-1"},
		Options{Stderr: io.Discard})
	require.NoError(t, err)
	assert.Empty(t, fake.pulled)
}

// A project already linked here is past first-bind; its incremental sync knows
// its own base, and seeding would be wrong.
func TestSeed_SkipsAnAlreadyLinkedProject(t *testing.T) {
	dir := t.TempDir()

	force(t, &projectLinkedFn, func(string) bool { return true })
	force(t, &getArtifactFn, func(string) (*workload.Artifact, error) {
		t.Fatal("a linked project is not re-seeded")

		return nil, nil
	})

	err := seedFromLiveArtifact(loadedForSeed(t, dir, boundGeneratedManifest), Live{ArtifactID: "art-1"},
		Options{Stderr: io.Discard})
	require.NoError(t, err)
}
