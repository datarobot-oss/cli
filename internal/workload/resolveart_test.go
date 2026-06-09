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

package workload

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validResolveConfig = `{
	"artifactId": "art-from-wapi-000000000",
	"catalogId": null,
	"lastSyncedVersionId": null,
	"createdAt": "2026-04-01T08:00:00Z",
	"cliVersion": "v0.0.0-test"
}`

func writeWAPIConfig(t *testing.T, dir, body string) {
	t.Helper()

	wapiDir := filepath.Join(dir, ".wapi")
	require.NoError(t, os.MkdirAll(wapiDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(wapiDir, "config.json"), []byte(body), 0o600))
}

func TestResolveArtifactID_ExplicitWinsOverWAPI(t *testing.T) {
	dir := t.TempDir()

	writeWAPIConfig(t, dir, validResolveConfig)
	t.Chdir(dir)

	id, source, err := ResolveArtifactID("art-explicit")
	require.NoError(t, err)
	assert.Equal(t, "art-explicit", id)
	assert.Equal(t, ArtifactIDSourceExplicit, source)
}

func TestResolveArtifactID_FromWAPIWhenExplicitEmpty(t *testing.T) {
	dir := t.TempDir()

	writeWAPIConfig(t, dir, validResolveConfig)
	t.Chdir(dir)

	id, source, err := ResolveArtifactID("")
	require.NoError(t, err)
	assert.Equal(t, "art-from-wapi-000000000", id)
	assert.Equal(t, ArtifactIDSourceWAPI, source)
}

func TestResolveArtifactID_NotInitializedHasUserHint(t *testing.T) {
	dir := t.TempDir()

	t.Chdir(dir)

	_, _, err := ResolveArtifactID("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no .wapi project")
	assert.Contains(t, err.Error(), "dr workload code init")
}

func TestResolveArtifactID_CorruptConfigPropagates(t *testing.T) {
	dir := t.TempDir()

	writeWAPIConfig(t, dir, "{ this is not json")
	t.Chdir(dir)

	_, _, err := ResolveArtifactID("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".wapi/config.json")
}
