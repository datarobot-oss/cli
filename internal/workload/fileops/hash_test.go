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

package fileops

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashFile(t *testing.T) {
	dir := t.TempDir()

	t.Run("KnownContent", func(t *testing.T) {
		body := []byte("hello world\n")
		path := filepath.Join(dir, "a.txt")
		require.NoError(t, os.WriteFile(path, body, 0o644))

		want := sha256.Sum256(body)

		gotHash, gotSize, err := HashFile(path)
		require.NoError(t, err)
		assert.Equal(t, hex.EncodeToString(want[:]), gotHash)
		assert.Equal(t, int64(len(body)), gotSize)
	})

	t.Run("EmptyFile", func(t *testing.T) {
		path := filepath.Join(dir, "empty.txt")
		require.NoError(t, os.WriteFile(path, nil, 0o644))

		gotHash, gotSize, err := HashFile(path)
		require.NoError(t, err)
		// Known SHA-256 of empty input.
		assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", gotHash)
		assert.Equal(t, int64(0), gotSize)
	})

	t.Run("Missing", func(t *testing.T) {
		_, _, err := HashFile(filepath.Join(dir, "nope.txt"))
		require.Error(t, err)
	})

	t.Run("LargerThanChunkBoundary", func(t *testing.T) {
		// Forces multiple buffer refills through io.CopyBuffer.
		body := make([]byte, HashChunkSizeBytes*3+17)
		for i := range body {
			body[i] = byte(i % 251)
		}

		path := filepath.Join(dir, "big.bin")
		require.NoError(t, os.WriteFile(path, body, 0o644))

		want := sha256.Sum256(body)

		gotHash, gotSize, err := HashFile(path)
		require.NoError(t, err)
		assert.Equal(t, hex.EncodeToString(want[:]), gotHash)
		assert.Equal(t, int64(len(body)), gotSize)
	})
}

func TestHashFile_ExceedsMaxSize(t *testing.T) {
	// Avoid writing a 5 GiB fixture by calling the unexported hashFile
	// directly with a tiny ceiling; production HashFile always uses
	// MaxFileSizeBytes.
	t.Run("WrapsErrFileTooLarge", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "x.bin")
		require.NoError(t, os.WriteFile(path, []byte("0123456789"), 0o644))

		_, _, err := hashFile(path, 4)
		require.ErrorIs(t, err, ErrFileTooLarge)
		assert.Contains(t, err.Error(), "x.bin")
	})
}

func TestHashReader(t *testing.T) {
	body := []byte("abc")
	want := sha256.Sum256(body)

	gotHash, gotSize, err := HashReader(strings.NewReader(string(body)))
	require.NoError(t, err)
	assert.Equal(t, hex.EncodeToString(want[:]), gotHash)
	assert.Equal(t, int64(len(body)), gotSize)
}
