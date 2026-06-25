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

package pipeline

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/outputformat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- CondaValue JSON codec ---

func TestCondaValue_MarshalJSON_PlainList(t *testing.T) {
	c := CondaValue{Deps: []string{"scipy", "numpy"}}

	b, err := json.Marshal(c)
	require.NoError(t, err)
	assert.JSONEq(t, `["scipy","numpy"]`, string(b))
}

func TestCondaValue_MarshalJSON_WithChannels(t *testing.T) {
	c := CondaValue{
		Channels: []string{"conda-forge", "defaults"},
		Deps:     []string{"numpy=1.21.*"},
	}

	b, err := json.Marshal(c)
	require.NoError(t, err)

	var got CondaSpec

	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, []string{"conda-forge", "defaults"}, got.Channels)
	assert.Equal(t, []string{"numpy=1.21.*"}, got.Dependencies)
}

func TestCondaValue_UnmarshalJSON_PlainList(t *testing.T) {
	var c CondaValue

	require.NoError(t, json.Unmarshal([]byte(`["scipy","numpy"]`), &c))
	assert.Equal(t, []string{"scipy", "numpy"}, c.Deps)
	assert.Empty(t, c.Channels)
}

func TestCondaValue_UnmarshalJSON_CondaSpec(t *testing.T) {
	var c CondaValue

	raw := `{"channels":["conda-forge"],"dependencies":["numpy=1.21.*","xarray=0.15.1"]}`
	require.NoError(t, json.Unmarshal([]byte(raw), &c))
	assert.Equal(t, []string{"conda-forge"}, c.Channels)
	assert.Equal(t, []string{"numpy=1.21.*", "xarray=0.15.1"}, c.Deps)
}

// --- formatCondaCell ---

func TestFormatCondaCell_PlainDeps(t *testing.T) {
	c := &CondaValue{Deps: []string{"scipy", "numpy"}}

	assert.Equal(t, "scipy,numpy", formatCondaCell(c))
}

func TestFormatCondaCell_ChannelsOnly(t *testing.T) {
	c := &CondaValue{Channels: []string{"conda-forge"}}

	assert.Equal(t, "[conda-forge]", formatCondaCell(c))
}

func TestFormatCondaCell_ChannelsAndDeps(t *testing.T) {
	c := &CondaValue{
		Channels: []string{"conda-forge", "defaults"},
		Deps:     []string{"numpy=1.21.*"},
	}

	assert.Equal(t, "[conda-forge,defaults] numpy=1.21.*", formatCondaCell(c))
}

// --- image human output includes conda channels ---

func TestPrintImageHuman_ShowsCondaChannels(t *testing.T) {
	baseImage := "python:3.12"
	img := Image{
		ImageID:       "img-1",
		Name:          "ml-base",
		LatestVersion: 1,
		CreatedAt:     time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		Versions: []ImageVersion{
			{
				Version: 1,
				Status:  ImageStatusReady,
				Definition: ImageDefinition{
					Name:      "ml-base",
					Pip:       []string{"torch"},
					Conda:     &CondaValue{Channels: []string{"conda-forge"}, Deps: []string{"scipy"}},
					BaseImage: &baseImage,
					Nvidia:    true,
				},
				CreatedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}

	out := captureStdout(t, func() { PrintImageHuman(img) })

	assert.Contains(t, out, "[conda-forge]", "channels must appear in human output")
	assert.Contains(t, out, "scipy", "dependencies must appear in human output")
}

func TestPrintImageHuman_HidesCondaWhenEmpty(t *testing.T) {
	img := Image{
		ImageID:       "img-2",
		Name:          "pip-only",
		LatestVersion: 1,
		CreatedAt:     time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		Versions: []ImageVersion{
			{
				Version: 1,
				Status:  ImageStatusReady,
				Definition: ImageDefinition{
					Name: "pip-only",
					Pip:  []string{"numpy"},
				},
				CreatedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}

	out := captureStdout(t, func() { PrintImageHuman(img) })

	assert.Contains(t, out, emptyValuePlaceholder, "conda cell should show placeholder when no conda packages")
}

// --- imageJSON preserves conda channels ---

func TestPrintImageJSON_CondaChannelsPreserved(t *testing.T) {
	img := Image{
		ImageID:       "img-1",
		Name:          "ml-base",
		LatestVersion: 1,
		CreatedAt:     time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		Versions: []ImageVersion{
			{
				Version: 1,
				Status:  ImageStatusReady,
				Definition: ImageDefinition{
					Name:  "ml-base",
					Conda: &CondaValue{Channels: []string{"conda-forge"}, Deps: []string{"scipy"}},
				},
				CreatedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}

	out := captureStdout(t, func() {
		require.NoError(t, RenderImage(outputformat.OutputFormatJSON, img))
	})

	var parsed map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &parsed))
	versions := parsed["versions"].([]any)
	conda := versions[0].(map[string]any)["conda"].(map[string]any)
	assert.Equal(t, []any{"conda-forge"}, conda["channels"])
	assert.Equal(t, []any{"scipy"}, conda["dependencies"])
}
