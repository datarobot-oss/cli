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

package idargs

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const boundID = "68b0c1d2e3f4a5b6c7d8e9f0"

const boundManifest = `workloadId: ` + boundID + `
name: my-app
artifact:
  name: my-app-artifact
`

// project makes a directory holding a manifest, with the manifest search
// bounded to it so a stray .datarobot.yaml on the real filesystem cannot
// answer for it. t.TempDir() sits outside $HOME, where Locate's stop condition
// would never fire and the walk would run to the root.
func project(t *testing.T, content string) string {
	t.Helper()

	home := t.TempDir()
	testutil.SetTestHomeDir(t, home)

	dir := filepath.Join(home, "proj")
	require.NoError(t, os.MkdirAll(dir, 0o750))

	if content != "" {
		require.NoError(t, os.WriteFile(filepath.Join(dir, manifest.FileName), []byte(content), 0o600))
	}

	return dir
}

func TestResolve(t *testing.T) {
	t.Run("explicit id wins over a bound manifest", func(t *testing.T) {
		dir := project(t, boundManifest)

		ref, err := resolve([]string{"typed-id"}, dir)
		require.NoError(t, err)
		assert.Equal(t, "typed-id", ref.ID)
		assert.False(t, ref.FromManifest())
	})

	t.Run("explicit id needs no manifest at all", func(t *testing.T) {
		// Pins that an explicit id skips the search entirely: a foreign or
		// broken manifest overhead must not fail a command that was told which
		// workload it means.
		ref, err := resolve([]string{"typed-id"}, project(t, ""))
		require.NoError(t, err)
		assert.Equal(t, "typed-id", ref.ID)
	})

	t.Run("a blank id is refused, not a fallback", func(t *testing.T) {
		// "" is what an unset shell variable expands to and is the same string
		// as no argument at all, so only arity separates them.
		for _, blank := range []string{"", "   "} {
			_, err := resolve([]string{blank}, project(t, boundManifest))
			require.Error(t, err, "id %q", blank)
			assert.Contains(t, err.Error(), "id is empty")
		}
	})

	t.Run("a typed id is trimmed", func(t *testing.T) {
		ref, err := resolve([]string{"  typed-id  "}, project(t, boundManifest))
		require.NoError(t, err)
		assert.Equal(t, "typed-id", ref.ID)
	})

	t.Run("reads the manifest in dir", func(t *testing.T) {
		dir := project(t, boundManifest)

		ref, err := resolve(nil, dir)
		require.NoError(t, err)
		assert.Equal(t, boundID, ref.ID)
		assert.True(t, ref.FromManifest())
		assert.Equal(t, filepath.Join(dir, manifest.FileName), ref.Path)
	})

	t.Run("reads a manifest in a parent directory", func(t *testing.T) {
		dir := project(t, boundManifest)
		nested := filepath.Join(dir, "svc", "internal")
		require.NoError(t, os.MkdirAll(nested, 0o750))

		ref, err := resolve(nil, nested)
		require.NoError(t, err)
		assert.Equal(t, boundID, ref.ID)
	})

	t.Run("no manifest names both remedies", func(t *testing.T) {
		_, err := resolve(nil, project(t, ""))
		require.Error(t, err)
		require.ErrorIs(t, err, manifest.ErrNotFound)
		assert.Contains(t, err.Error(), "Pass a workload id")
	})

	t.Run("an unbound manifest says to deploy, not to create a file", func(t *testing.T) {
		_, err := resolve(nil, project(t, "name: my-app\n"))
		require.Error(t, err)
		require.NotErrorIs(t, err, manifest.ErrNotFound)
		assert.Contains(t, err.Error(), "names no workload yet")
	})
}

func TestRefWrap(t *testing.T) {
	notFound := &drapi.HTTPError{StatusCode: http.StatusNotFound, URL: "u"}

	t.Run("names the manifest behind a 404", func(t *testing.T) {
		ref := Ref{ID: boundID, Source: WorkloadIDSourceManifest, Path: "/p/.datarobot.yaml"}

		err := ref.Wrap(notFound)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "/p/.datarobot.yaml")
		assert.Contains(t, err.Error(), "not found")

		// The HTTP error is deliberately not chained: its "404 Not Found (url:
		// ...)" tail is longer than the sentence explaining it, and nothing
		// reads the status back off a command error.
		assert.NotContains(t, err.Error(), "url:")
	})

	t.Run("a 403 does not claim the workload is missing", func(t *testing.T) {
		ref := Ref{ID: boundID, Source: WorkloadIDSourceManifest, Path: "/p/.datarobot.yaml"}

		err := ref.Wrap(&drapi.HTTPError{StatusCode: http.StatusForbidden, URL: "u"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no access")
	})

	t.Run("leaves a typed id alone", func(t *testing.T) {
		ref := Ref{ID: boundID, Source: WorkloadIDSourceExplicit}
		assert.Equal(t, notFound, ref.Wrap(notFound))
	})

	t.Run("passes other errors through", func(t *testing.T) {
		other := errors.New("boom")
		ref := Ref{ID: boundID, Source: WorkloadIDSourceManifest, Path: "/p"}
		assert.Equal(t, other, ref.Wrap(other))
	})
}
