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
// cannot have one. Only a leftover is a real question, and a history that
// cannot be read is not an answer: "no image" would spend a build on a guess,
// "has one" would promote an artifact with nothing to run.
func TestImageInHand(t *testing.T) {
	leftover := version{ID: "art-abandoned"}

	cases := []struct {
		name    string
		made    version
		builds  []workload.Build
		readErr error
		asks    bool
		want    bool
	}{
		{
			name: "a carried image answers itself", want: true,
			made: version{ID: "art-2", Fresh: true, HasCode: true, ImageURI: "registry/app:sha"},
		},
		{name: "so does a version created moments ago", made: version{ID: "art-2", Fresh: true}},
		{
			name: "a leftover with a completed build has one", made: leftover, asks: true, want: true,
			builds: []workload.Build{{Status: workload.BuildStatusCompleted}},
		},
		{name: "a leftover with none does not", made: leftover, asks: true},
		{
			name: "a history that cannot be read is an error", made: leftover, asks: true,
			readErr: errors.New("gateway timeout"),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			asked := false

			force(t, &listBuildsFn, func(string, int) ([]workload.Build, error) {
				asked = true

				return c.builds, c.readErr
			})

			got, builds, err := imageInHand(c.made)
			assert.Equal(t, c.asks, asked, "only a leftover is worth a round trip")

			if c.readErr != nil {
				require.Error(t, err)
				assert.Contains(t, err.Error(), c.made.ID)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, c.want, got)
			assert.Equal(t, c.builds != nil, builds != nil, "the history comes back only when fetched")
		})
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
