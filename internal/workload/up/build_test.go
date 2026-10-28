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
	"io"
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

// The project is linked to art-2 throughout. A roll marks the version it links
// pending, so a run that dies after the push leaves the mark, the link and the
// last push ahead of the workload. Everything else that leaves the link behind
// the workload carries no mark, and rolling on it would undo someone else's
// deploy. Unlike imageStale, what cannot be established is no.
func TestUnserved(t *testing.T) {
	cases := []struct {
		name     string
		linked   bool
		loadErr  error
		pending  *string
		synced   *string
		liveArt  string
		liveCode string
		want     bool
	}{
		{
			name: "a deploy from here died before its rollout", linked: true, pending: ptr("art-2"),
			synced: ptr("ver2"), liveArt: "art-1", liveCode: "ver1", want: true,
		},
		{
			name: "the workload runs what this checkout last deployed", linked: true,
			synced: ptr("ver2"), liveArt: "art-2", liveCode: "ver2",
		},
		{
			name: "another checkout, a CI job or a rollback moved the workload on", linked: true,
			synced: ptr("ver2"), liveArt: "art-3", liveCode: "ver3",
		},
		{
			name: "a sync elsewhere moved the code of the version serving", linked: true, pending: ptr("art-2"),
			synced: ptr("ver2"), liveArt: "art-2", liveCode: "ver3",
		},
		{
			name: "the pending version holds the code already running", linked: true, pending: ptr("art-2"),
			synced: ptr("ver1"), liveArt: "art-1", liveCode: "ver1",
		},
		{
			name: "the mark names a version the project has since left", linked: true, pending: ptr("art-9"),
			synced: ptr("ver2"), liveArt: "art-1", liveCode: "ver1",
		},
		{
			name: "nothing has been pushed from here", linked: true, pending: ptr("art-2"),
			liveArt: "art-1", liveCode: "ver1",
		},
		{
			name: "the running version points at no code", linked: true, pending: ptr("art-2"),
			synced: ptr("ver2"), liveArt: "art-1",
		},
		{name: "the project is not linked", pending: ptr("art-2"), synced: ptr("ver2"), liveArt: "art-1", liveCode: "ver1"},
		{
			name: "the record cannot be read", linked: true, pending: ptr("art-2"), synced: ptr("ver2"),
			liveArt: "art-1", liveCode: "ver1", loadErr: errors.New("permission denied"),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			force(t, &projectLinkedFn, func(string) bool { return c.linked })
			force(t, &loadProjectFn, func(string) (wapi.Config, error) {
				return wapi.Config{
					ArtifactID: "art-2", LastSyncedVersionID: c.synced, PendingRolloutArtifactID: c.pending,
				}, c.loadErr
			})

			got := unserved("/project", Live{ArtifactID: c.liveArt, CodeVersionID: c.liveCode})
			assert.Equal(t, c.want, got)
		})
	}
}

// A version a roll links to stays pending until a run sees it serve, whether
// the roll re-pointed a linked project or linked it for the first time.
func TestLinkProject_MarksTheVersionPending(t *testing.T) {
	made := version{ID: "art-2", Fresh: true}

	t.Run("relinked", func(t *testing.T) {
		var saved wapi.Config

		force(t, &projectLinkedFn, func(string) bool { return true })
		force(t, &loadProjectFn, func(string) (wapi.Config, error) { return wapi.Config{ArtifactID: "art-1"}, nil })
		force(t, &saveProjectFn, func(_ string, cfg wapi.Config) error {
			saved = cfg

			return nil
		})

		require.NoError(t, linkProject("/project", made, newReporter(io.Discard, false)))
		assert.Equal(t, "art-2", saved.ArtifactID)
		assert.Equal(t, ptr("art-2"), saved.PendingRolloutArtifactID, "in the same write as the link")
	})

	t.Run("an earlier attempt picked up again", func(t *testing.T) {
		var saved []wapi.Config

		force(t, &projectLinkedFn, func(string) bool { return true })
		force(t, &saveProjectFn, func(_ string, cfg wapi.Config) error {
			saved = append(saved, cfg)

			return nil
		})

		leftover := version{ID: "art-2"}

		for _, pending := range []*string{nil, ptr("art-2")} {
			force(t, &loadProjectFn, func(string) (wapi.Config, error) {
				return wapi.Config{ArtifactID: "art-2", PendingRolloutArtifactID: pending}, nil
			})

			require.NoError(t, linkProject("/project", leftover, newReporter(io.Discard, false)))
		}

		require.Len(t, saved, 1, "marked once, and not again while the mark is there")
		assert.Equal(t, ptr("art-2"), saved[0].PendingRolloutArtifactID)
	})

	t.Run("linked for the first time", func(t *testing.T) {
		var opts wapi.InitOptions

		force(t, &projectLinkedFn, func(string) bool { return false })
		force(t, &initProjectFn, func(_ string, o wapi.InitOptions) error {
			opts = o

			return nil
		})

		require.NoError(t, linkProject("/project", made, newReporter(io.Discard, false)))
		assert.Equal(t, wapi.InitOptions{ArtifactID: "art-2", RolloutPending: true}, opts)
	})
}

// The mark goes once the workload runs the version it names, and only then.
// Cleared early, a deploy that dies in its rollout is never finished; left in
// place, a later run reads someone else's newer deploy as this checkout's
// unfinished one.
func TestClearServedRollout(t *testing.T) {
	cases := []struct {
		name    string
		linked  bool
		pending *string
		serving string
		clears  bool
	}{
		{name: "the pending version is serving", linked: true, pending: ptr("art-2"), serving: "art-2", clears: true},
		{name: "the workload is still on another version", linked: true, pending: ptr("art-2"), serving: "art-1"},
		{name: "nothing is pending", linked: true, serving: "art-2"},
		{name: "no version was seen serving", linked: true, pending: ptr("art-2")},
		{name: "the project is not linked", pending: ptr("art-2"), serving: "art-2"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var saved *wapi.Config

			force(t, &projectLinkedFn, func(string) bool { return c.linked })
			force(t, &loadProjectFn, func(string) (wapi.Config, error) {
				return wapi.Config{ArtifactID: "art-2", PendingRolloutArtifactID: c.pending}, nil
			})
			force(t, &saveProjectFn, func(_ string, cfg wapi.Config) error {
				saved = &cfg

				return nil
			})

			clearServedRollout("/project", c.serving)

			if !c.clears {
				assert.Nil(t, saved, "nothing to write")

				return
			}

			require.NotNil(t, saved)
			assert.Nil(t, saved.PendingRolloutArtifactID)
			assert.Equal(t, "art-2", saved.ArtifactID, "the link stays where it is")
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
