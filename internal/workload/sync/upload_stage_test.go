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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadOneToStage_ForwardsLocalMode(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "entrypoint.sh"), []byte("#!/bin/sh\n"), 0o755))

	fake := &fakeFilesClient{}
	e := &Engine{projectDir: dir, files: fake}

	fa := FileAction{Path: "entrypoint.sh", LocalSize: 10, LocalMode: 0o755}

	require.NoError(t, uploadOneToStage(e, "cid-1", "st-1", fa))

	assert.Equal(t, uint32(0o755), fake.uploadedModes["entrypoint.sh"])
}
