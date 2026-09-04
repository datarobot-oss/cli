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
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// downloadingFilesClient serves fixed content for DownloadFile; everything
// else is inherited (and panics if hit) from fakeFilesClient.
type downloadingFilesClient struct {
	fakeFilesClient

	body []byte
}

func (f *downloadingFilesClient) DownloadFile(_, _, _ string, w io.Writer) (string, int64, error) {
	n, err := w.Write(f.body)
	return "", int64(n), err
}

func TestDownloadOne_RestoresRemoteMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not meaningful on Windows")
	}

	dir := t.TempDir()
	body := []byte("#!/bin/sh\n")

	e := &Engine{
		projectDir: dir,
		files:      &downloadingFilesClient{body: body},
	}

	fa := FileAction{Path: "entrypoint.sh", RemoteMode: 0o755}

	require.NoError(t, downloadOne(e, "cid", "vid", fa))

	info, err := os.Stat(filepath.Join(dir, "entrypoint.sh"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
}

func TestDownloadOne_UnknownModeLeavesDefault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not meaningful on Windows")
	}

	dir := t.TempDir()

	e := &Engine{
		projectDir: dir,
		files:      &downloadingFilesClient{body: []byte("hi")},
	}

	fa := FileAction{Path: "README.md"}

	require.NoError(t, downloadOne(e, "cid", "vid", fa))

	info, err := os.Stat(filepath.Join(dir, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm(), "mode 0 leaves the os.Create default untouched")
}
