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
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	MaxFileSizeBytes   int64 = 5 * 1024 * 1024 * 1024 // 5 GiB
	HashChunkSizeBytes       = 64 * 1024
)

var ErrFileTooLarge = errors.New("file exceeds MaxFileSizeBytes")

func HashFile(path string) (string, int64, error) {
	return hashFile(path, MaxFileSizeBytes)
}

func hashFile(path string, maxBytes int64) (string, int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", 0, fmt.Errorf("stat %s: %w", path, err)
	}

	if info.Size() > maxBytes {
		return "", info.Size(), fmt.Errorf("%w: %s (%d bytes)", ErrFileTooLarge, path, info.Size())
	}

	f, err := os.Open(path)
	if err != nil {
		return "", info.Size(), fmt.Errorf("open %s: %w", path, err)
	}

	defer func() { _ = f.Close() }()

	h := sha256.New()
	buf := make([]byte, HashChunkSizeBytes)

	if _, err := io.CopyBuffer(h, f, buf); err != nil {
		return "", info.Size(), fmt.Errorf("hash %s: %w", path, err)
	}

	return hex.EncodeToString(h.Sum(nil)), info.Size(), nil
}

func HashReader(r io.Reader) (string, int64, error) {
	return hashReader(r, MaxFileSizeBytes)
}

func hashReader(r io.Reader, maxBytes int64) (string, int64, error) {
	h := sha256.New()
	buf := make([]byte, HashChunkSizeBytes)

	// Read maxBytes+1 so we can detect overflow rather than silently truncating
	// (io.LimitReader returns EOF at its cap with no error).
	n, err := io.CopyBuffer(h, io.LimitReader(r, maxBytes+1), buf)
	if err != nil {
		return "", n, fmt.Errorf("hash reader: %w", err)
	}

	if n > maxBytes {
		return "", n, fmt.Errorf("%w: stream (%d+ bytes)", ErrFileTooLarge, n)
	}

	return hex.EncodeToString(h.Sum(nil)), n, nil
}
