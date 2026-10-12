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
	"errors"
	"testing"

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Two of the three answers cost nothing: a copy carries its image, and a create
// cannot have one. Only a leftover is a real question, and the platform's
// record of what its image was built from answers it. A read that fails is not
// an answer: "no image" would spend a build on a guess, "has one" would promote
// an artifact with nothing to run.
func TestImageInHand(t *testing.T) {
	leftover := version{ID: "art-abandoned"}
	failed := errors.New("gateway timeout")

	cases := []struct {
		name       string
		made       version
		builtFrom  string
		docErr     error
		historyErr error
		reads      bool
		lists      bool
		want       bool
	}{
		{
			name: "a carried image answers itself", want: true,
			made: version{ID: "art-2", Fresh: true, HasCode: true, ImageURI: "registry/app:sha"},
		},
		{name: "so does a version created moments ago", made: version{ID: "art-2", Fresh: true}},
		{
			name: "a leftover built from the code it points at has one", made: leftover, builtFrom: "ver2",
			reads: true, want: true,
		},
		{
			name: "a leftover built from code it has since moved past does not", made: leftover, builtFrom: "ver1",
			reads: true, lists: true,
		},
		{name: "a leftover never built does not", made: leftover, reads: true, lists: true},
		{name: "a document that cannot be read is an error", made: leftover, docErr: failed, reads: true},
		{
			name: "a history that cannot be read is an error", made: leftover, historyErr: failed,
			reads: true, lists: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			read, listed := false, false

			force(t, &getArtifactDocFn, func(string) (workload.Document, error) {
				read = true

				return builtDoc("ver2", c.builtFrom), c.docErr
			})
			force(t, &listBuildsFn, func(string, int) ([]workload.Build, error) {
				listed = true

				return []workload.Build{}, c.historyErr
			})

			got, builds, err := imageInHand(c.made)
			assert.Equal(t, c.reads, read, "only a leftover is worth a round trip")
			assert.Equal(t, c.lists, listed, "the history is read only for a build to attach to")

			if c.docErr != nil || c.historyErr != nil {
				require.Error(t, err)
				assert.Contains(t, err.Error(), c.made.ID)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, c.want, got)
			assert.Equal(t, c.lists, builds != nil, "the history comes back only when fetched")
		})
	}
}

// builtDoc is an artifact whose primary container points at codeVersion and
// carries the platform's record of a completed build of builtFrom. An empty
// builtFrom leaves the record out, as the platform does when the code moves.
func builtDoc(codeVersion, builtFrom string) workload.Document {
	container := map[string]any{"primary": true}
	stampBuild(container, codeVersion, builtFrom)

	return workload.Document{"spec": map[string]any{
		"containerGroups": []any{map[string]any{"containers": []any{container}}},
	}}
}

// stampBuild writes builtDoc's code reference and build record onto a decoded
// container.
func stampBuild(container map[string]any, codeVersion, builtFrom string) {
	ref := func(version string) map[string]any {
		return map[string]any{"datarobot": map[string]any{"catalogId": "cat1", "catalogVersionId": version}}
	}

	config, _ := container["imageBuildConfig"].(map[string]any)
	if config == nil {
		config = map[string]any{}
		container["imageBuildConfig"] = config
	}

	config["codeRef"] = ref(codeVersion)

	if builtFrom != "" {
		container["build"] = map[string]any{
			"status":    workload.BuildStatusCompleted,
			"builtFrom": map[string]any{"codeRef": ref(builtFrom)},
		}
	}
}

// `dr artifact code sync` moves the code an artifact points at without touching
// the working tree, and nothing on the platform records what built an image.
// Everything it cannot establish is stale, the only answer safe to be wrong
// about: the alternative deploys an image nothing vouches for.
func TestImageStale(t *testing.T) {
	cases := []struct {
		name    string
		linked  bool
		loadErr error
		built   *string
		live    string
		want    bool
	}{
		{name: "the recorded build made the code the artifact points at", linked: true, built: ptr("ver1"), live: "ver1"},
		{name: "the code has moved on since that build", linked: true, built: ptr("ver0"), live: "ver1", want: true},
		{name: "no build recorded, so nothing vouches for the image", linked: true, live: "ver1", want: true},
		{name: "the running version points at no code at all", linked: true, built: ptr("ver1"), want: true},
		{name: "the project is not linked, so there is no record to read", built: ptr("ver1"), live: "ver1", want: true},
		{
			name: "the record cannot be read", linked: true, built: ptr("ver1"), live: "ver1",
			loadErr: errors.New("permission denied"), want: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			force(t, &projectLinkedFn, func(string) bool { return c.linked })
			force(t, &loadProjectFn, func(string) (wapi.Config, error) {
				return wapi.Config{ArtifactID: "art-1", LastBuiltVersionID: c.built}, c.loadErr
			})

			assert.Equal(t, c.want, imageStale("/project", Live{CodeVersionID: c.live}))
		})
	}
}

// A roll links the project to the new version before it pushes, so a run that
// dies after the push leaves the last push ahead of what the workload runs.
// Unlike imageStale, what cannot be established is no: yes rolls the workload.
func TestUnserved(t *testing.T) {
	cases := []struct {
		name    string
		linked  bool
		loadErr error
		synced  *string
		live    string
		want    bool
	}{
		{name: "the workload runs the code last pushed", linked: true, synced: ptr("ver1"), live: "ver1"},
		{name: "the last push never reached the workload", linked: true, synced: ptr("ver2"), live: "ver1", want: true},
		{name: "nothing has been pushed from here", linked: true, live: "ver1"},
		{name: "the running version points at no code", linked: true, synced: ptr("ver2")},
		{name: "the project is not linked", synced: ptr("ver2"), live: "ver1"},
		{
			name: "the record cannot be read", linked: true, synced: ptr("ver2"), live: "ver1",
			loadErr: errors.New("permission denied"),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			force(t, &projectLinkedFn, func(string) bool { return c.linked })
			force(t, &loadProjectFn, func(string) (wapi.Config, error) {
				return wapi.Config{ArtifactID: "art-2", LastSyncedVersionID: c.synced}, c.loadErr
			})

			assert.Equal(t, c.want, unserved("/project", Live{CodeVersionID: c.live}))
		})
	}
}

// The record is what the next deploy compares against. A write that fails costs
// that deploy a build, so it must not fail this one, and the two states with
// nothing to record must not write: an id no build was made from reads as
// licence to skip one.
func TestRecordBuiltVersion(t *testing.T) {
	synced := "ver-2"

	cases := []struct {
		name     string
		linked   bool
		synced   *string
		writeErr error
		want     string
	}{
		{name: "the version synced is the version built", linked: true, synced: &synced, want: "ver-2"},
		{
			name: "a write that fails is still attempted", linked: true, synced: &synced,
			writeErr: errors.New("read-only"), want: "ver-2",
		},
		{name: "the project is not linked, so there is nowhere to write"},
		{name: "nothing has been synced, so no version was built", linked: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var saved *wapi.Config

			force(t, &projectLinkedFn, func(string) bool { return c.linked })
			force(t, &loadProjectFn, func(string) (wapi.Config, error) {
				return wapi.Config{ArtifactID: "art-1", LastSyncedVersionID: c.synced}, nil
			})
			force(t, &saveProjectFn, func(_ string, cfg wapi.Config) error {
				saved = &cfg

				return c.writeErr
			})

			require.NotPanics(t, func() { recordBuiltVersion("/project") })

			if c.want == "" {
				assert.Nil(t, saved, "nothing to record is nothing to write")

				return
			}

			require.NotNil(t, saved)
			require.NotNil(t, saved.LastBuiltVersionID)
			assert.Equal(t, c.want, *saved.LastBuiltVersionID)
		})
	}
}

func ptr(v string) *string { return &v }
